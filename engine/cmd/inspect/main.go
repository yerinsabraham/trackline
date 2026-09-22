// Command inspect replays a recorded session and prints what the engine saw.
//
// This is a debugging tool, not a product surface. It exists because when a
// verdict is wrong, the only way to find out why is to look at the events and
// the intent the engine was working from. It is deliberately plain text.
//
// Usage:
//
//	inspect -rec session.jsonl [-transcript transcript.jsonl]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/engine"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/session"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// scopeStub is Phase 1 scaffolding, not a real check.
//
// It exists to prove events and intent reach a signal and that a verdict comes
// back with evidence attached. The real scope check, with a false-positive rate
// measured over a week of actual work, is Phase 2.
type scopeStub struct{}

func (scopeStub) Name() string { return "scope-stub" }

func (scopeStub) Check(in signal.Input) verdict.Result {
	anchor, ok := in.Intent.Anchor()
	if !ok {
		return verdict.NotApplicable("scope-stub", "nothing substantive has been asked yet")
	}
	if in.Event.Action.PathsUnknown {
		return verdict.CannotMeasure("scope-stub",
			fmt.Sprintf("%s does not report which files it touches", in.Event.Action.ToolName))
	}
	if !in.Event.TouchesFiles() {
		return verdict.NotApplicable("scope-stub", "this action touches no files")
	}

	// Deliberately naive: does any word of the request appear in the path.
	words := strings.FieldsFunc(strings.ToLower(anchor.Text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	for _, p := range in.Event.Action.Paths {
		lower := strings.ToLower(p)
		hit := false
		for _, w := range words {
			if len(w) > 3 && strings.Contains(lower, w) {
				hit = true
				break
			}
		}
		if !hit {
			return verdict.Finding("scope-stub", verdict.Verdict{
				Severity: verdict.SeverityWarn,
				Summary:  "edited a file the request never mentioned",
				Evidence: []verdict.Evidence{
					{Kind: verdict.EvidenceTurn, Value: anchor.Text, Note: "what was asked"},
					{Kind: verdict.EvidenceFile, Value: p, Note: "what was touched"},
				},
			})
		}
	}
	return verdict.Clean("scope-stub")
}

func main() {
	rec := flag.String("rec", "", "path to a recorded session (JSONL)")
	tr := flag.String("transcript", "", "path to the host transcript, for intent")
	flag.Parse()

	if *rec == "" {
		fmt.Fprintln(os.Stderr, "usage: inspect -rec session.jsonl [-transcript transcript.jsonl]")
		os.Exit(2)
	}

	events, err := session.Replay(*rec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "replay:", err)
		os.Exit(1)
	}

	var in intent.Intent
	path := *tr
	if path == "" && len(events) > 0 {
		path = events[0].TranscriptPath
	}
	if path != "" {
		r := &intent.Reader{Path: path}
		if err := r.Read(&in); err != nil {
			fmt.Fprintf(os.Stderr, "note: could not read transcript (%v); running without intent\n\n", err)
		}
	}

	printIntent(in)
	printTimeline(events, in)
}

func printIntent(in intent.Intent) {
	fmt.Printf("INTENT  %d human turn(s)\n", len(in.Turns))
	if anchor, ok := in.Anchor(); ok {
		fmt.Printf("  anchor : %s\n", oneLine(anchor.Text, 76))
	} else {
		fmt.Println("  anchor : none — nothing substantive asked yet")
	}
	if latest, ok := in.Latest(); ok && !latest.Substantive {
		fmt.Printf("  latest : %s  (carries no instruction)\n", oneLine(latest.Text, 60))
	}
	fmt.Println()
}

func printTimeline(events []event.Event, in intent.Intent) {
	e := engine.New(scopeStub{})

	fmt.Printf("TIMELINE  %d event(s)\n", len(events))
	var findings, unmeasured int

	for i, ev := range events {
		rep := e.Run(ev, in, nil)

		where := "-"
		switch {
		case ev.Action.PathsUnknown:
			where = "(files not visible)"
		case len(ev.Action.Paths) > 0:
			where = filepath.Base(ev.Action.Paths[0])
			if n := len(ev.Action.Paths); n > 1 {
				where += fmt.Sprintf(" +%d", n-1)
			}
		}

		fmt.Printf("\n  %2d. %-11s %-13s %-22s %s\n",
			i+1, ev.Host, ev.Action.Type, ev.Action.ToolName, where)

		for _, res := range rep.Results {
			switch res.Outcome {
			case verdict.OutcomeFinding:
				for _, v := range res.Verdicts {
					findings++
					fmt.Printf("      %s  %s\n", strings.ToUpper(string(v.Severity)), v.Summary)
					for _, evd := range v.Evidence {
						fmt.Printf("        %-8s %s\n", evd.Kind, oneLine(evd.Value, 64))
					}
				}
			case verdict.OutcomeCannotMeasure:
				unmeasured++
				fmt.Printf("      not measured: %s\n", res.Reason)
			case verdict.OutcomeNotApplicable:
				fmt.Printf("      n/a: %s\n", res.Reason)
			}
		}
	}

	fmt.Printf("\n%d finding(s), %d not measured\n", findings, unmeasured)
	if unmeasured > 0 {
		fmt.Println("not measured is not clean: those actions were never checked.")
	}
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
