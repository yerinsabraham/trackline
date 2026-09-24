package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/judge"
	"github.com/yerinsabraham/trackline/engine/internal/otlp"
	"github.com/yerinsabraham/trackline/engine/internal/production"
)

// cmdTraces checks exported production traces.
//
//	trackline traces [--root DIR] [--json] FILE|DIR...
//
// Files are OTLP export requests: .json in the OTLP/JSON mapping, anything
// else protobuf. A directory is read in name order, which is arrival order for
// anything that numbers its captures.
func cmdTraces(args []string) error {
	root, _ := os.Getwd()
	asJSON := false
	judgeBin := ""
	var inputs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--json":
			asJSON = true
		case "--judge":
			if i+1 < len(args) {
				i++
				judgeBin = args[i]
			}
		default:
			inputs = append(inputs, args[i])
		}
	}
	if len(inputs) == 0 {
		return fmt.Errorf("usage: trackline traces [--root DIR] [--json] FILE|DIR...")
	}

	var files []string
	for _, in := range inputs {
		st, err := os.Stat(in)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			files = append(files, in)
			continue
		}
		entries, _ := os.ReadDir(in)
		var names []string
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, filepath.Join(in, e.Name()))
			}
		}
		sort.Strings(names)
		files = append(files, names...)
	}

	var spans []otel.Span
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		ct := "application/x-protobuf"
		if strings.HasSuffix(f, ".json") {
			ct = "application/json"
		}
		s, err := otlp.Decode(b, ct)
		if err != nil {
			// One unreadable file is named and stops the run. Carrying on would
			// report on a stream with a hole in it as if it were whole.
			return fmt.Errorf("%s: %w", f, err)
		}
		spans = append(spans, s...)
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	results := production.Process(production.Assemble(spans), cfg, production.NewMonitor())

	var verdicts map[string]judge.Answer
	if judgeBin != "" {
		p, err := provider(config.JudgeConfig{Provider: "cli", Binary: judgeBin})
		if err != nil {
			return err
		}
		verdicts = judgeConversations(p, results)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		for _, r := range results {
			line := map[string]any{"result": r}
			if a, ok := verdicts[r.Conversation.TraceID]; ok {
				line["judge"] = a
			}
			if err := enc.Encode(line); err != nil {
				return err
			}
		}
		return nil
	}
	printTraceResults(results, cfg, verdicts, judgeBin != "")
	return nil
}

// judgeConversations asks whether each run's tool calls served its request.
//
// The judge sees the request and the names of the tools called, never their
// arguments. Arguments carry customer data, account numbers and amounts, and
// the question does not need them: whether send_marketing_email serves "what
// is my balance" is answered by the name. The same line the coding judge draws
// by never sending file contents.
func judgeConversations(p judge.Provider, results []production.Result) map[string]judge.Answer {
	j := judge.New(p)
	out := map[string]judge.Answer{}
	for _, r := range results {
		c := r.Conversation
		if !c.ContentAvailable || len(c.Requests) == 0 {
			continue
		}
		var actions []string
		for _, a := range c.Actions {
			actions = append(actions, "called tool "+a.Action.ToolName)
		}
		a, err := j.Ask(context.Background(), judge.Turn{Request: strings.Join(c.Requests, "\n"), Actions: actions})
		if err != nil {
			a = judge.Answer{Verdict: "error", Reason: err.Error()}
		}
		out[c.TraceID] = a
	}
	return out
}

func printTraceResults(results []production.Result, cfg config.Config, verdicts map[string]judge.Answer, judged bool) {
	flagged, incidents, unmeasured, unrelated := 0, 0, 0, 0
	for _, r := range results {
		fs := r.Findings()
		a, hasVerdict := verdicts[r.Conversation.TraceID]
		offTask := hasVerdict && (a.Verdict == judge.Unrelated || a.Verdict == "error")
		if len(fs) == 0 && len(r.Incidents) == 0 && len(r.Unmeasured) == 0 && !offTask {
			continue
		}
		fmt.Printf("%s\n", r.Conversation.ID)
		if len(r.Conversation.Requests) > 0 {
			fmt.Printf("  asked: %s\n", truncate(r.Conversation.Requests[0], 100))
		}
		for _, v := range fs {
			flagged++
			fmt.Printf("  POLICY    %s\n", v.Summary)
		}
		for _, i := range r.Incidents {
			incidents++
			fmt.Printf("  INCIDENT  %s: %s\n", i.Kind, i.Summary)
		}
		for _, u := range r.Unmeasured {
			unmeasured++
			fmt.Printf("  UNCHECKED %s\n", u.Reason)
		}
		if offTask {
			if a.Verdict == judge.Unrelated {
				unrelated++
			}
			fmt.Printf("  JUDGE     %s: %s\n", a.Verdict, a.Reason)
		}
		fmt.Println()
	}
	fmt.Printf("%d conversations, %d policy findings, %d incidents, %d not checkable",
		len(results), flagged, incidents, unmeasured)
	if judged {
		fmt.Printf(", %d judged off-task", unrelated)
	}
	fmt.Println()
	if cfg.Tools.Empty() {
		fmt.Println("\nno tool policy is configured, so tool calls were not checked against one.\n" +
			`add one to .trackline.json: {"tools": {"never": [...], "requireApproval": {"tool": "approval_tool"}}}`)
	}
	if !judged {
		fmt.Println("\nA permitted tool used for something the user did not ask for is not caught by")
		fmt.Println("policy or monitors. --judge codex (or claude, cursor-agent) asks a model, sending")
		fmt.Println("it each request and the names of the tools called, never their arguments.")
	}
}
