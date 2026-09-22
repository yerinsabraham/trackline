package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/judge"
	"github.com/yerinsabraham/trackline/engine/internal/session"
)

// cmdReview asks a model whether each turn's work served its request.
//
// Separate from the hook on purpose. A model call takes seconds and the hook
// has milliseconds, so this runs after the fact, over a session that has
// already been recorded. It changes nothing and blocks nothing: it is the one
// check that can be wrong in a way no test catches, so it observes until it has
// earned more than that.
//
//	trackline review [--root DIR] [--provider cli|http] [--binary claude]
func cmdReview(args []string) error {
	f := parse(args)
	override := ""
	binary := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 < len(args) {
				i++
				override = args[i]
			}
		case "--binary":
			if i+1 < len(args) {
				i++
				binary = args[i]
			}
		}
	}

	cfg, err := config.Load(f.root)
	if err != nil {
		return err
	}
	if override != "" {
		cfg.Judge.Provider = override
	}
	if binary != "" {
		cfg.Judge.Binary = binary
	}

	p, err := provider(cfg.Judge)
	if err != nil {
		return err
	}

	events, err := session.Replay(filepath.Join(f.root, ".trackline", "events.jsonl"))
	if err != nil {
		return fmt.Errorf("no recorded session here: %w", err)
	}
	if len(events) == 0 {
		fmt.Println("nothing has been recorded yet.")
		return nil
	}

	turns, skipped := group(events, f.root)
	if len(turns) == 0 {
		fmt.Println("no turns could be reconstructed from the recording.")
		if skipped > 0 {
			fmt.Printf("%d turn(s) had no recorded request, so there was nothing to judge them against.\n", skipped)
		}
		return nil
	}

	fmt.Printf("asking %s about %d turn(s)\n\n", p.Name(), len(turns))

	j := judge.New(p)
	counts := map[judge.Verdict]int{}
	failed := 0

	for _, t := range turns {
		a, err := j.Ask(context.Background(), t.turn)
		if err != nil {
			failed++
			fmt.Printf("  %s could not be judged: %v\n\n", t.label, err)
			continue
		}
		counts[a.Verdict]++

		fmt.Printf("  %-9s %s\n", strings.ToUpper(string(a.Verdict)), t.label)
		fmt.Printf("            %s\n", a.Reason)
		if a.Unrelated != "" {
			fmt.Printf("            does not fit: %s\n", a.Unrelated)
		}
		fmt.Println()
	}

	fmt.Printf("%d serves, %d unrelated, %d unclear",
		counts[judge.Serves], counts[judge.Unrelated], counts[judge.Unclear])
	if failed > 0 {
		fmt.Printf(", %d could not be judged", failed)
	}
	fmt.Println()

	if skipped > 0 {
		// Not folded into the counts above: a turn nobody could read is not a
		// turn that was fine.
		fmt.Printf("\n%d turn(s) were not reviewed at all, because no request was recorded for them.\n", skipped)
	}

	if counts[judge.Unclear] > counts[judge.Serves]+counts[judge.Unrelated] {
		fmt.Println("\nmostly unclear usually means the requests were vague, not that the agent misbehaved.")
	}
	fmt.Println("\nThis changes nothing and blocks nothing. It is one model's opinion,")
	fmt.Println("recorded so it can be checked against what actually happened.")
	return nil
}

func provider(c config.JudgeConfig) (judge.Provider, error) {
	switch c.Provider {
	case "cli":
		bin := c.Binary
		if bin == "" {
			bin = "claude"
		}
		return judge.CLI{Binary: bin, Timeout: 120 * time.Second}, nil
	case "http":
		if c.Model == "" {
			return nil, fmt.Errorf("the http judge needs a model in .trackline.json")
		}
		key := ""
		if c.APIKeyEnv != "" {
			key = os.Getenv(c.APIKeyEnv)
		}
		return judge.HTTP{BaseURL: c.BaseURL, Model: c.Model, APIKey: key}, nil
	case "":
		return nil, fmt.Errorf(`no judge is configured.

It is off by default, because it is the one check that costs money and the one
that can be wrong in a way no test catches.

To use the coding-agent CLI you already have, which needs no key:
    trackline review --provider cli --binary claude

Or set it permanently in .trackline.json:
    {"judge": {"provider": "cli", "binary": "claude"}}

Or point it at any OpenAI-compatible endpoint, including a local one:
    {"judge": {"provider": "http", "baseUrl": "http://localhost:11434/v1",
               "model": "llama3.1", "apiKeyEnv": "OPENAI_API_KEY"}}`)
	default:
		return nil, fmt.Errorf("unknown judge provider %q", c.Provider)
	}
}

type groupedTurn struct {
	label string
	turn  judge.Turn
}

// group reconstructs each turn: what was asked, and what was done under it.
func group(events []event.Event, root string) ([]groupedTurn, int) {
	// A recording outlives any one agent session: a project watched over a
	// week holds many, each with its own transcript. Reading only the first
	// meant every turn but the earliest had no request attached and was
	// silently dropped, which looked exactly like a session with one turn in
	// it.
	var in intent.Intent
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.TranscriptPath == "" || seen[ev.TranscriptPath] {
			continue
		}
		seen[ev.TranscriptPath] = true
		_ = (&intent.Reader{Path: ev.TranscriptPath}).Read(&in)
	}

	order := []string{}
	byTurn := map[string][]event.Event{}
	for _, ev := range events {
		if ev.TurnID == "" {
			continue
		}
		if _, seen := byTurn[ev.TurnID]; !seen {
			order = append(order, ev.TurnID)
		}
		byTurn[ev.TurnID] = append(byTurn[ev.TurnID], ev)
	}

	var out []groupedTurn
	skipped := 0
	for _, id := range order {
		evs := byTurn[id]
		request := ""
		if turn, ok := in.TurnByID(id); ok {
			request = turn.Text
		}
		// A turn whose request was never recorded cannot be judged: there is
		// nothing to compare the work against. Counted rather than dropped, so
		// a reader can tell a short session from a session mostly unread.
		if strings.TrimSpace(request) == "" {
			skipped++
			continue
		}

		var actions []string
		for _, ev := range evs {
			actions = append(actions, describe(ev, root))
		}
		out = append(out, groupedTurn{
			label: fmt.Sprintf("%q", truncate(request, 64)),
			turn:  judge.Turn{Request: request, Actions: actions},
		})
	}
	return out, skipped
}

// describe renders one action as a line the judge can read.
//
// Content is deliberately left out. The judge is asked whether the work fits
// the request, and sending file contents would both cost far more and invite it
// to comment on code quality, which is the thing it is explicitly not for.
func describe(ev event.Event, root string) string {
	a := ev.Action
	var what []string
	for _, p := range a.Paths {
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
			what = append(what, rel)
		} else {
			what = append(what, p)
		}
	}

	switch {
	case a.Type == event.ActionRunCommand && a.Command != "":
		return fmt.Sprintf("ran: %s", truncate(a.Command, 120))
	case len(a.Installs) > 0:
		return fmt.Sprintf("installed %s", strings.Join(a.Installs, ", "))
	case len(what) > 0:
		return fmt.Sprintf("%s %s", a.Type, strings.Join(what, ", "))
	default:
		return fmt.Sprintf("%s (%s)", a.Type, a.ToolName)
	}
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
