package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: playlint <playlist.m3u> [more playlists...]")
		os.Exit(2)
	}

	exitCode := 0
	for _, path := range os.Args[1:] {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 2
			continue
		}

		findings, err := Lint(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 2
			continue
		}

		for _, finding := range findings {
			fmt.Printf("%s:%d: %s %s: %s\n", path, finding.Line, finding.Severity, finding.Rule, finding.Message)
			if finding.Severity == SeverityError && exitCode == 0 {
				exitCode = 1
			}
		}
	}

	os.Exit(exitCode)
}
