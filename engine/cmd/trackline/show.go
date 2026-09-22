package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/walkthrough"
)

// cmdShow renders a recorded session as a story rather than a log.
//
//	trackline show [--root DIR] [--task "..."] [--json FILE]
func cmdShow(args []string) error {
	root := ""
	task := ""
	out := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--task", "-task":
			if i+1 < len(args) {
				i++
				task = args[i]
			}
		case "--json":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		}
	}
	if root == "" {
		root, _ = os.Getwd()
	}

	s, err := walkthrough.Build(filepath.Join(root, ".trackline", "findings.jsonl"), root, task)
	if err != nil {
		return fmt.Errorf("no recorded session here: %w", err)
	}

	if out != "" {
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %d step(s) to %s\n", len(s.Steps), out)
		return nil
	}

	render(s)
	return nil
}

const (
	dim    = "\x1b[2m"
	bold   = "\x1b[1m"
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
	green  = "\x1b[32m"
	reset  = "\x1b[0m"
)

func render(s walkthrough.Session) {
	if s.Task != "" {
		fmt.Printf("%sasked for%s  %s\n", dim, reset, s.Task)
	}
	fmt.Printf("%smode%s       %s\n\n", dim, reset, s.Mode)

	for _, st := range s.Steps {
		marker, colour := " ", dim
		switch st.Outcome {
		case "blocked":
			marker, colour = "■", red
		case "finding":
			marker, colour = "▲", yellow
		case "unseen":
			marker, colour = "?", dim
		case "clean":
			marker, colour = "·", green
		}

		what := strings.Join(st.Paths, ", ")
		if what == "" {
			what = st.Command
		}
		if what == "" {
			what = "—"
		}
		fmt.Printf("  %s%s%s %2d. %-12s %s\n", colour, marker, reset, st.N, st.Tool, what)

		for _, f := range st.Findings {
			fmt.Printf("        %s%s%s  %s\n", colour, strings.ToUpper(f.Severity), reset, f.Summary)
			for _, e := range f.Evidence {
				fmt.Printf("          %s%-6s%s %s\n", dim, e.Kind, reset, e.Value)
			}
			if f.Suggestion != "" {
				fmt.Printf("          %s→ %s%s\n", dim, f.Suggestion, reset)
			}
		}
		if len(st.Unseen) > 0 && len(st.Findings) == 0 {
			fmt.Printf("        %snot seen by %s%s\n", dim, strings.Join(st.Unseen, ", "), reset)
		}
	}

	fmt.Printf("\n%s%d actions, %d finding(s), %d blocked, %d action(s) nobody could see%s\n",
		bold, s.Summary.Actions, s.Summary.Findings, s.Summary.Blocked, s.Summary.Unseen, reset)
	if s.Summary.Unseen > 0 {
		fmt.Printf("%san action nobody could look at is not an action that was fine.%s\n", dim, reset)
	}
}
