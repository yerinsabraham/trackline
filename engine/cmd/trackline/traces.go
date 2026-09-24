package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/config"
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

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		for _, r := range results {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	}
	printTraceResults(results, cfg)
	return nil
}

func printTraceResults(results []production.Result, cfg config.Config) {
	flagged, incidents, unmeasured := 0, 0, 0
	for _, r := range results {
		fs := r.Findings()
		if len(fs) == 0 && len(r.Incidents) == 0 && len(r.Unmeasured) == 0 {
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
		fmt.Println()
	}
	fmt.Printf("%d conversations, %d policy findings, %d incidents, %d not checkable\n",
		len(results), flagged, incidents, unmeasured)
	if cfg.Tools.Empty() {
		fmt.Println("\nno tool policy is configured, so tool calls were not checked against one.\n" +
			`add one to .trackline.json: {"tools": {"never": [...], "requireApproval": {"tool": "approval_tool"}}}`)
	}
	fmt.Println("\nA permitted tool used for something the user did not ask for is not caught here;")
	fmt.Println("that needs a judge reading the request, which trace review does not run yet.")
}
