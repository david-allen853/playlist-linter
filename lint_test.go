package main

import (
	"strings"
	"testing"
)

// want describes a finding we expect, without pinning down the exact
// message text (that's free to reword; the line and rule are the contract).
type want struct {
	line     int
	severity Severity
	rule     string
}

func TestLint(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []want
	}{
		{
			name: "well formed playlist has no findings",
			input: "#EXTM3U\n" +
				"#EXTINF:123,Some Song\n" +
				"songs/track1.mp3\n" +
				"#EXTINF:-1,Live Stream\n" +
				"http://stream.example/live\n",
			want: nil,
		},
		{
			name:  "empty file is missing a header",
			input: "",
			want:  []want{{1, SeverityWarning, "missing-header"}},
		},
		{
			name:  "missing header on an otherwise fine playlist",
			input: "songs/track1.mp3\n",
			want:  []want{{1, SeverityWarning, "missing-header"}},
		},
		{
			name:  "a leading BOM does not break header detection",
			input: "﻿#EXTM3U\ntrack.mp3\n",
			want:  nil,
		},
		{
			name:  "CRLF line endings are handled",
			input: "#EXTM3U\r\ntrack.mp3\r\n",
			want:  nil,
		},
		{
			name:  "non-numeric duration",
			input: "#EXTM3U\n#EXTINF:abc,Title\ntrack.mp3\n",
			want:  []want{{2, SeverityError, "bad-duration"}},
		},
		{
			name:  "negative duration other than -1 is invalid",
			input: "#EXTM3U\n#EXTINF:-5,Title\ntrack.mp3\n",
			want:  []want{{2, SeverityError, "bad-duration"}},
		},
		{
			name:  "-1 duration is the documented live-stream sentinel",
			input: "#EXTM3U\n#EXTINF:-1,Live\nhttp://example.com/live\n",
			want:  nil,
		},
		{
			name:  "dangling EXTINF at end of file",
			input: "#EXTM3U\n#EXTINF:120,Title\n",
			want:  []want{{2, SeverityError, "dangling-extinf"}},
		},
		{
			name:  "EXTINF immediately followed by another EXTINF",
			input: "#EXTM3U\n#EXTINF:120,First\n#EXTINF:90,Second\ntrack.mp3\n",
			want:  []want{{2, SeverityError, "dangling-extinf"}},
		},
		{
			name:  "duplicate track path",
			input: "#EXTM3U\ntrack.mp3\nother.mp3\ntrack.mp3\n",
			want:  []want{{4, SeverityWarning, "duplicate-track"}},
		},
		{
			name:  "unknown file extension",
			input: "#EXTM3U\ntrack.xyz\n",
			want:  []want{{2, SeverityWarning, "unknown-extension"}},
		},
		{
			name:  "extension check ignores case",
			input: "#EXTM3U\nTRACK.MP3\n",
			want:  nil,
		},
		{
			name:  "http URLs are not checked for a file extension",
			input: "#EXTM3U\nhttp://example.com/stream\n",
			want:  nil,
		},
		{
			name:  "blank lines between entries are not entries",
			input: "#EXTM3U\n\ntrack.mp3\n\n\nother.mp3\n",
			want:  nil,
		},
		{
			name:  "trailing whitespace on a path line",
			input: "#EXTM3U\ntrack.mp3 \n",
			want:  []want{{2, SeverityWarning, "trailing-whitespace"}},
		},
		{
			name:  "trailing tab on a path line",
			input: "#EXTM3U\ntrack.mp3\t\n",
			want:  []want{{2, SeverityWarning, "trailing-whitespace"}},
		},
		{
			name:  "backslash path",
			input: "#EXTM3U\nsongs\\track.mp3\n",
			want:  []want{{2, SeverityWarning, "backslash-path"}},
		},
		{
			name:  "backslash path is not flagged for URLs",
			input: "#EXTM3U\nhttp://example.com/a\\b\n",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lint(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Lint returned an error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings, want %d\ngot: %+v", len(got), len(tt.want), got)
			}

			for i, w := range tt.want {
				g := got[i]
				if g.Line != w.line || g.Severity != w.severity || g.Rule != w.rule {
					t.Errorf("finding %d: got {line:%d severity:%v rule:%s}, want {line:%d severity:%v rule:%s}",
						i, g.Line, g.Severity, g.Rule, w.line, w.severity, w.rule)
				}
			}
		})
	}
}

func TestFix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		removed int
	}{
		{
			name:    "no duplicates leaves content untouched",
			input:   "#EXTM3U\ntrack.mp3\nother.mp3\n",
			want:    "#EXTM3U\ntrack.mp3\nother.mp3\n",
			removed: 0,
		},
		{
			name:    "duplicate track and its EXTINF are removed",
			input:   "#EXTM3U\n#EXTINF:100,A\ntrack.mp3\n#EXTINF:100,A\ntrack.mp3\n",
			want:    "#EXTM3U\n#EXTINF:100,A\ntrack.mp3\n",
			removed: 1,
		},
		{
			name:    "CRLF line endings are preserved",
			input:   "#EXTM3U\r\ntrack.mp3\r\ntrack.mp3\r\n",
			want:    "#EXTM3U\r\ntrack.mp3\r\n",
			removed: 1,
		},
		{
			name:    "missing trailing newline is preserved",
			input:   "#EXTM3U\ntrack.mp3\ntrack.mp3",
			want:    "#EXTM3U\ntrack.mp3",
			removed: 1,
		},
		{
			name:    "later duplicates of an earlier duplicate are all removed",
			input:   "#EXTM3U\ntrack.mp3\ntrack.mp3\ntrack.mp3\n",
			want:    "#EXTM3U\ntrack.mp3\n",
			removed: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, removed, err := Fix(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Fix returned an error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got fixed content %q, want %q", got, tt.want)
			}
			if removed != tt.removed {
				t.Errorf("got %d removed, want %d", removed, tt.removed)
			}
		})
	}
}
