// Package main implements playlint, a linter for M3U/M3U8 audio playlists.
package main

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Severity classifies how serious a Finding is.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Finding is a single problem reported by the linter, anchored to a line
// number in the source playlist so a reader can jump straight to it.
type Finding struct {
	Line     int
	Severity Severity
	Rule     string
	Message  string
}

// knownExtensions holds file extensions (without the leading dot, lower
// case) that we recognize as audio. Anything else on a local path is
// flagged so a reader can catch a stray .jpg or a typo'd extension.
var knownExtensions = map[string]bool{
	"mp3":  true,
	"m4a":  true,
	"m4b":  true,
	"aac":  true,
	"flac": true,
	"wav":  true,
	"wave": true,
	"ogg":  true,
	"oga":  true,
	"opus": true,
	"wma":  true,
	"aiff": true,
	"aif":  true,
	"ape":  true,
	"wv":   true,
}

// Lint reads an M3U/M3U8 playlist and returns every finding, sorted by
// line number.
func Lint(r io.Reader) ([]Finding, error) {
	var findings []Finding

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	firstLine := true
	sawHeader := false
	pendingExtinf := 0 // line number of an #EXTINF not yet matched to a path, 0 if none
	seen := map[string]int{}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if firstLine {
			// A playlist saved by some Windows tools starts with a UTF-8
			// BOM; strip it so the header check below still matches.
			line = strings.TrimPrefix(line, "﻿")
			firstLine = false
		}
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if lineNum == 1 && trimmed == "#EXTM3U" {
			sawHeader = true
			continue
		}

		if rest, ok := strings.CutPrefix(trimmed, "#EXTINF:"); ok {
			if pendingExtinf != 0 {
				findings = append(findings, Finding{
					Line:     pendingExtinf,
					Severity: SeverityError,
					Rule:     "dangling-extinf",
					Message:  "#EXTINF entry is not followed by a track path",
				})
			}
			if f, ok := checkExtinf(rest, lineNum); ok {
				findings = append(findings, f)
			}
			pendingExtinf = lineNum
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			// Some other directive or comment; leave it alone.
			continue
		}

		findings = append(findings, checkPath(line, trimmed, lineNum, seen)...)
		pendingExtinf = 0
	}

	if err := scanner.Err(); err != nil {
		return findings, err
	}

	if pendingExtinf != 0 {
		findings = append(findings, Finding{
			Line:     pendingExtinf,
			Severity: SeverityError,
			Rule:     "dangling-extinf",
			Message:  "#EXTINF entry is not followed by a track path",
		})
	}

	if !sawHeader {
		findings = append(findings, Finding{
			Line:     1,
			Severity: SeverityWarning,
			Rule:     "missing-header",
			Message:  "playlist does not start with #EXTM3U",
		})
	}

	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Line < findings[j].Line })

	return findings, nil
}

// Fix removes duplicate track entries from a playlist, keeping the first
// occurrence of each path and its metadata lines (#EXTINF and similar) and
// dropping the metadata and path of every later repeat. It returns the
// fixed content, the number of duplicate entries removed, and preserves
// the original line-ending style and trailing-newline presence so a diff
// against the source shows only the removed lines.
func Fix(r io.Reader) (string, int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", 0, err
	}
	content := string(data)

	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}

	trailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}

	var out []string
	var pendingMeta []string
	seen := map[string]bool{}
	removed := 0

	for i, line := range lines {
		if i == 0 {
			line = strings.TrimPrefix(line, "﻿")
		}
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			out = append(out, line)
			continue
		}

		if i == 0 && trimmed == "#EXTM3U" {
			out = append(out, line)
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			pendingMeta = append(pendingMeta, line)
			continue
		}

		if seen[trimmed] {
			pendingMeta = nil
			removed++
			continue
		}
		seen[trimmed] = true
		out = append(out, pendingMeta...)
		out = append(out, line)
		pendingMeta = nil
	}
	out = append(out, pendingMeta...)

	fixed := strings.Join(out, nl)
	if trailingNewline && len(out) > 0 {
		fixed += nl
	}

	return fixed, removed, nil
}

// checkExtinf validates the duration field of an #EXTINF line. A duration
// of -1 is a documented sentinel for "unknown length" (live streams) and
// is not an error; anything else negative or non-numeric is.
func checkExtinf(rest string, lineNum int) (Finding, bool) {
	durationPart, _, _ := strings.Cut(rest, ",")
	durationPart = strings.TrimSpace(durationPart)

	dur, err := strconv.ParseFloat(durationPart, 64)
	if err != nil {
		return Finding{
			Line:     lineNum,
			Severity: SeverityError,
			Rule:     "bad-duration",
			Message:  fmt.Sprintf("duration %q is not a number", durationPart),
		}, true
	}
	if dur < 0 && dur != -1 {
		return Finding{
			Line:     lineNum,
			Severity: SeverityError,
			Rule:     "bad-duration",
			Message:  fmt.Sprintf("duration %v is negative", dur),
		}, true
	}
	return Finding{}, false
}

// checkPath validates a track entry: a URL or a filesystem path pointing
// at the audio file itself. raw is the line as it appeared in the file
// (used for whitespace checks); path is the already-trimmed entry used
// for lookups and comparisons.
func checkPath(raw, path string, lineNum int, seen map[string]int) []Finding {
	var findings []Finding

	if strings.TrimRight(raw, " \t") != raw {
		findings = append(findings, Finding{
			Line:     lineNum,
			Severity: SeverityWarning,
			Rule:     "trailing-whitespace",
			Message:  "line has trailing whitespace, which some players treat as part of the path",
		})
	}

	if firstLine, ok := seen[path]; ok {
		findings = append(findings, Finding{
			Line:     lineNum,
			Severity: SeverityWarning,
			Rule:     "duplicate-track",
			Message:  fmt.Sprintf("%q was already listed at line %d", path, firstLine),
		})
	} else {
		seen[path] = lineNum
	}

	lower := strings.ToLower(path)
	isURL := strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
	if !isURL {
		if strings.Contains(path, "\\") {
			findings = append(findings, Finding{
				Line:     lineNum,
				Severity: SeverityWarning,
				Rule:     "backslash-path",
				Message:  fmt.Sprintf("%q uses backslashes; players on non-Windows platforms won't resolve them", path),
			})
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if !knownExtensions[ext] {
			findings = append(findings, Finding{
				Line:     lineNum,
				Severity: SeverityWarning,
				Rule:     "unknown-extension",
				Message:  fmt.Sprintf("%q does not look like an audio file", path),
			})
		}
	}

	return findings
}
