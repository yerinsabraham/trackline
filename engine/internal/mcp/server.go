// Package mcp lets an agent ask trackline before it acts.
//
// It reaches agents trackline has no hook for, and it is weaker than a hook in
// one way that has to be said plainly wherever this is described: the agent
// chooses whether to ask. A hook sees every action whether the agent likes it
// or not. An MCP tool is consulted, and an agent that does not consult it is
// not watched. So this widens reach; it does not replace enforcement.
//
// The protocol is JSON-RPC 2.0, one message per line on stdin and stdout. It is
// written here directly rather than through an SDK: the surface used is four
// methods, and the engine carries no dependencies it can avoid.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/runner"
	"github.com/yerinsabraham/trackline/engine/internal/shell"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Versions this server has been written against, newest first. A client
// asking for one of these gets it; anything else gets the newest, and the
// client decides whether it can continue.
var versions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Server answers one client over one stream.
type Server struct {
	Root    string
	Version string
	Now     func() time.Time
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// UnlikelyRoot reports a root that is almost certainly not a project, which is
// where a client that ignores the working directory will start the server.
func UnlikelyRoot(root string) bool {
	clean := filepath.Clean(root)
	home, _ := os.UserHomeDir()
	return clean == "/" || clean == filepath.VolumeName(clean)+`\` || (home != "" && clean == filepath.Clean(home))
}

// Serve reads requests until the input closes.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	enc := json.NewEncoder(out)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		// A notification has no id and must never be answered.
		if len(req.ID) == 0 {
			continue
		}
		result, rerr := s.handle(req)
		resp := response{JSONRPC: "2.0", ID: req.ID}
		if rerr != nil {
			resp.Error = rerr
		} else {
			resp.Result = result
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) handle(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		v := versions[0]
		for _, known := range versions {
			if p.ProtocolVersion == known {
				v = known
			}
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "trackline", "version": s.Version},
			"instructions": "Before writing, editing or deleting a file, running a command, " +
				"or installing a package, call check_action and follow its answer. " +
				"Call get_rules at the start of a task and again when unsure.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		switch p.Name {
		case "check_action":
			return s.checkAction(p.Arguments), nil
		case "get_rules":
			return s.getRules(), nil
		}
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("unknown tool %q", p.Name)}
	}
	return nil, &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
}

var tools = []map[string]any{
	{
		"name": "check_action",
		"description": "Ask trackline whether an action is allowed before taking it. " +
			"Returns proceed, stop, or ask (stop and put the question to the user). " +
			"Follow the answer. Pass task, in the user's own words, so the scope " +
			"check has something to compare against.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string",
					"enum": []string{"write_file", "edit_file", "delete_file", "run_command", "install_package"}},
				"path":     map[string]any{"type": "string", "description": "file the action touches"},
				"command":  map[string]any{"type": "string", "description": "for run_command"},
				"content":  map[string]any{"type": "string", "description": "the new content, for writes"},
				"packages": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"task":     map[string]any{"type": "string", "description": "what the user asked for, in their words"},
			},
			"required": []string{"action"},
		},
	},
	{
		"name":        "get_rules",
		"description": "The project's rules: protected paths, how each check acts, and the rules files trackline read.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
}

type checkArgs struct {
	Action   string   `json:"action"`
	Path     string   `json:"path"`
	Command  string   `json:"command"`
	Content  string   `json:"content"`
	Packages []string `json:"packages"`
	Task     string   `json:"task"`
}

func (s *Server) checkAction(raw json.RawMessage) map[string]any {
	var a checkArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("arguments were not readable: " + err.Error())
	}

	act, err := s.toAction(a)
	if err != nil {
		return toolError(err.Error())
	}

	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	// A fixed session and no turn. Approvals made for one request cannot be
	// matched to an MCP question, so only project-wide approvals apply here.
	ev := event.Event{
		SessionID: "mcp",
		At:        now,
		Host:      event.HostMCP,
		Phase:     event.PhasePreTool,
		CWD:       s.Root,
		Action:    act,
	}

	var in intent.Intent
	unavailable := ""
	if strings.TrimSpace(a.Task) != "" {
		// Self-reported. The agent says what it was asked, and could be wrong
		// or could shade it; a hook reads the transcript instead. Still better
		// than no intent, which leaves scope with nothing to compare against.
		in.Add("", now, a.Task)
	} else {
		unavailable = "the agent did not say what it was asked to do, so scope has nothing to compare against"
	}

	d := runner.Check(ev, in, unavailable, runner.Options{Root: s.Root, Now: now})
	return render(d, s.Root)
}

func (s *Server) toAction(a checkArgs) (event.Action, error) {
	act := event.Action{ToolName: "mcp:" + a.Action}
	path := func() error {
		if a.Path == "" {
			return fmt.Errorf("%s needs a path", a.Action)
		}
		p := a.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.Root, p)
		}
		act.Paths = []string{filepath.Clean(p)}
		return nil
	}

	switch a.Action {
	case "write_file", "edit_file":
		act.Type = event.ActionWriteFile
		if a.Action == "edit_file" {
			act.Type = event.ActionEditFile
		}
		if err := path(); err != nil {
			return act, err
		}
		act.SetBody(a.Content)
	case "delete_file":
		act.Type = event.ActionDeleteFile
		if err := path(); err != nil {
			return act, err
		}
	case "run_command":
		if a.Command == "" {
			return act, fmt.Errorf("run_command needs a command")
		}
		act.Type = event.ActionRunCommand
		act.Command = a.Command
		eff := shell.Parse(a.Command)
		for _, p := range append(eff.Writes, eff.Deletes...) {
			if !filepath.IsAbs(p) {
				p = filepath.Join(s.Root, p)
			}
			act.Paths = append(act.Paths, filepath.Clean(p))
		}
		act.Installs = eff.Installs
		act.PathsUnknown = !eff.Understood
	case "install_package":
		if len(a.Packages) == 0 {
			return act, fmt.Errorf("install_package needs packages")
		}
		act.Type = event.ActionRunCommand
		act.Command = "install " + strings.Join(a.Packages, " ")
		act.Installs = a.Packages
	default:
		return act, fmt.Errorf("unknown action %q; use write_file, edit_file, delete_file, run_command or install_package", a.Action)
	}
	return act, nil
}

// render turns a decision into the tool result.
//
// The text is what a model reads, so it leads with the instruction. The
// structured copy is for clients that act on it programmatically.
func render(d runner.Decision, root string) map[string]any {
	decision := "proceed"
	var text strings.Builder

	switch {
	case d.Block && d.Ask:
		decision = "ask"
		text.WriteString(d.Message)
	case d.Block:
		decision = "stop"
		text.WriteString("Do not do this.\n\n")
		text.WriteString(d.Message)
	default:
		text.WriteString("Proceed.")
	}

	var findings []map[string]string
	var unmeasured []string
	for _, res := range d.Report.Results {
		switch res.Outcome {
		case verdict.OutcomeFinding:
			for _, v := range res.Verdicts {
				findings = append(findings, map[string]string{"check": res.Signal, "summary": v.Summary})
			}
		case verdict.OutcomeCannotMeasure:
			unmeasured = append(unmeasured, res.Signal)
		}
	}

	// In warn mode a finding does not stop anything, but the agent should
	// still hear it: that is the reminder working.
	if decision == "proceed" && len(findings) > 0 {
		text.WriteString(" trackline noted:\n")
		for _, f := range findings {
			fmt.Fprintf(&text, "- %s: %s\n", f["check"], f["summary"])
		}
	}
	// Named, so "proceed" is never read as "every check passed".
	if len(unmeasured) > 0 {
		fmt.Fprintf(&text, "\nNot checked, because trackline could not see enough: %s.", strings.Join(unmeasured, ", "))
	}
	// Said every time. An MCP client decides where the server starts, some
	// start it far from the project, and a check against the wrong folder
	// finds none of the project's config or rules and gives no sign of it.
	// Caught when a test client dropped --root and the answers looked fine.
	fmt.Fprintf(&text, "\n(checked against the project at %s)", root)

	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": strings.TrimSpace(text.String())}},
		"structuredContent": map[string]any{
			"decision":   decision,
			"root":       root,
			"findings":   findings,
			"unmeasured": unmeasured,
		},
	}
}

func (s *Server) getRules() map[string]any {
	cfg, cfgErr := config.Load(s.Root)
	rules, _ := config.LoadRules(s.Root, cfg)

	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\n", s.Root)
	fmt.Fprintf(&b, "Default mode: %s\n", cfg.Mode)
	for check, m := range cfg.Modes {
		fmt.Fprintf(&b, "  %s: %s\n", check, m)
	}
	fmt.Fprintf(&b, "\nNever write to: %s\n", strings.Join(cfg.OffLimits, ", "))
	if cfgErr != nil {
		fmt.Fprintf(&b, "\nThe trackline config could not be read (%v); defaults apply.\n", cfgErr)
	}
	if len(rules) == 0 {
		fmt.Fprintf(&b, "\nNo rules files found (looked for %s).\n", strings.Join(cfg.RuleFiles, ", "))
	} else {
		b.WriteString("\nProject rules:\n")
		// Capped: a rules file past this length is mostly ignored by the model
		// anyway, and the answer should stay readable.
		const limit = 100
		for i, r := range rules {
			if i == limit {
				fmt.Fprintf(&b, "... and %d more\n", len(rules)-limit)
				break
			}
			fmt.Fprintf(&b, "- %s (%s)\n", r.Text, r.Source)
		}
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": strings.TrimSpace(b.String())}},
	}
}

// toolError reports a problem with the call itself. It is a tool result, not a
// protocol error, so the model sees it and can correct its arguments.
func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}
