package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	fix := flag.Bool("fix", false, "remove duplicate track entries from each playlist in place")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: playlint [-fix] <playlist.m3u> [more playlists...]")
	}
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	exitCode := 0
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 2
			continue
		}

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
				fmt.Printf("%s: removed %d duplicate track(s)\n", path, removed)
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
			fmt.Printf("%s:%d: %s %s: %s\n", path, finding.Line, finding.Severity, finding.Rule, finding.Message)
			if finding.Severity == SeverityError && exitCode == 0 {
				exitCode = 1
			}
		}
	}

	os.Exit(exitCode)
}
