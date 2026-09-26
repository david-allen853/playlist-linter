package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// fileResult holds one file's outcome for JSON output: what got fixed (if
// -fix was passed) and every finding from linting the result.
type fileResult struct {
	File     string    `json:"file"`
	Removed  int       `json:"removed,omitempty"`
	Findings []Finding `json:"findings"`
}

func main() {
	fix := flag.Bool("fix", false, "remove duplicate track entries from each playlist in place")
	jsonOutput := flag.Bool("json", false, "print findings as a JSON array instead of plain text")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: playlint [-fix] [-json] <playlist.m3u> [more playlists...]")
	}
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	exitCode := 0
	results := make([]fileResult, 0, len(paths))

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 2
			continue
		}

		result := fileResult{File: path}

		if *fix {
			info, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
				exitCode = 2
				continue
			}

			fixed, removed, err := Fix(strings.NewReader(string(content)))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
				exitCode = 2
				continue
			}
			if removed > 0 {
				if err := os.WriteFile(path, []byte(fixed), info.Mode()); err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
					exitCode = 2
					continue
				}
				if !*jsonOutput {
					fmt.Printf("%s: removed %d duplicate track(s)\n", path, removed)
				}
				result.Removed = removed
				content = []byte(fixed)
			}
		}

		findings, err := Lint(strings.NewReader(string(content)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 2
			continue
		}

		for _, finding := range findings {
			if finding.Severity == SeverityError && exitCode == 0 {
				exitCode = 1
			}
		}

		if *jsonOutput {
			result.Findings = findings
			if result.Findings == nil {
				result.Findings = []Finding{}
			}
			results = append(results, result)
		} else {
			for _, finding := range findings {
				fmt.Printf("%s:%d: %s %s: %s\n", path, finding.Line, finding.Severity, finding.Rule, finding.Message)
			}
		}
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(os.Stderr, "playlint: %v\n", err)
			exitCode = 2
		}
	}

	os.Exit(exitCode)
}
