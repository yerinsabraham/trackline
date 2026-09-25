// Package runner is the hook's one pass: read an event, work out what was
// asked, run the checks, decide what to do.
//
// Separate from cmd/hook so it can be tested without a process.
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/claudecode"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/codex"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/cursor"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/engine"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/override"
	"github.com/yerinsabraham/trackline/engine/internal/session"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/dependency"
	"github.com/yerinsabraham/trackline/engine/internal/signal/diffsize"
	"github.com/yerinsabraham/trackline/engine/internal/signal/offlimits"
	"github.com/yerinsabraham/trackline/engine/internal/signal/repetition"
	"github.com/yerinsabraham/trackline/engine/internal/signal/scope"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Host names accepted on the command line.
const (
	HostAuto   = "auto"
	HostClaude = "claude"
	HostCodex  = "codex"
	HostCursor = "cursor"
)

// Decision is what the hook should do about an event.
type Decision struct {
	// Block is true only when the grounds exist *and* the configured mode
	// permits stopping. Phase 2 ships warn-only, so this stays false.
	Block bool

	// Message is what to hand back to the agent when blocking.
	Message string

	// Ask is true when the block is a request for a human decision rather
	// than a refusal.
	Ask bool

	// Report is everything the checks concluded, for the findings log.
	Report engine.Report

	// Overridden are findings a human had already approved. Recorded rather
	// than dropped: a decision someone made is worth being able to review.
	Overridden []verdict.Verdict

	// Mode is what the configuration said to do.
	Mode config.Mode
}

// Detect works out which host sent a payload.
//
// The hosts are distinguishable without being told: Claude Code sends
// prompt_id, Codex sends turn_id, and Cursor sends cursor_version on every
// event. A flag still wins, because a host that later changes its payload
// should be overridable without waiting for a release.
//
// Cursor is checked first because it also runs Claude-format hooks from
// .claude/settings.json, so a payload arriving through Claude's wiring may
// still be Cursor's.
func Detect(raw []byte) string {
	var probe struct {
		PromptID      string `json:"prompt_id"`
		TurnID        string `json:"turn_id"`
		Model         string `json:"model"`
		CursorVersion string `json:"cursor_version"`
	}
	if json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}), &probe) != nil {
		return HostClaude
	}
	switch {
	case probe.CursorVersion != "":
		return HostCursor
	case probe.TurnID != "":
		return HostCodex
	case probe.PromptID != "":
		return HostClaude
	case probe.Model != "":
		// Codex reports the model on every hook event; Claude Code does not.
		return HostCodex
	default:
		return HostClaude
	}
}

// Options configure one pass.
type Options struct {
	Host string
	Root string
	Now  time.Time
}

// Run does the whole pass for one payload.
func Run(raw []byte, opts Options) (Decision, error) {
	host := opts.Host
	if host == "" || host == HostAuto {
		host = Detect(raw)
	}

	var ev event.Event
	var err error
	switch host {
	case HostCodex:
		ev, err = codex.Parse(raw, opts.Now)
	case HostCursor:
		ev, err = cursor.Parse(raw, opts.Now)
	default:
		ev, err = claudecode.Parse(raw, opts.Now)
	}
	if err != nil {
		return Decision{}, fmt.Errorf("could not read the %s payload: %w", host, err)
	}

	var in intent.Intent
	var intentUnavailable string
	if ev.TranscriptPath != "" {
		reader := &intent.Reader{Path: ev.TranscriptPath}
		if err := reader.Read(&in); err != nil {
			intentUnavailable = fmt.Sprintf("the session transcript could not be read: %v", err)
		} else if len(in.Turns) == 0 {
			// A conversation we read but recognised nothing in is not a
			// conversation with nothing in it.
			if entries, assistant := reader.ReadAnything(); (entries > 0 && assistant > 0) || reader.Unrecognised() {
				intentUnavailable = "the session transcript was read but no request could be recognised in it, " +
					"so what the user asked for is unknown"
			}
		}
	} else {
		intentUnavailable = "this host did not say where the session transcript is, " +
			"so what the user asked for is unknown"
	}

	return decide(ev, in, intentUnavailable, opts, true), nil
}

// Check judges an action that has not happened and may never happen.
//
// It is what the MCP server calls when an agent asks "may I?". The answer is
// the one the hook would give, through the same checks, config and approvals.
// Nothing is recorded: a question is not an action, and counting it would
// have repetition and diff-size judge the agent for asking.
func Check(ev event.Event, in intent.Intent, intentUnavailable string, opts Options) Decision {
	return decide(ev, in, intentUnavailable, opts, false)
}

func decide(ev event.Event, in intent.Intent, intentUnavailable string, opts Options, record bool) Decision {
	root := opts.Root
	if root == "" {
		root = ev.CWD
	}

	// A configuration that cannot be read is worth saying out loud, but it must
	// never stop the user working. Defaults carry on.
	cfg, cfgErr := config.Load(root)
	if os.Getenv(config.RemoteEnv) != "" {
		cfg = cfg.ForRemote()
	}
	rules, rulesErr := config.LoadRules(root, cfg)

	// Two separate stores, on purpose. The recording is the full session, for
	// replay and inspection, and nothing on the hot path reads it. The turn
	// state holds only the current request, and is what the checks read, so
	// their cost does not grow with the length of the session.
	recording := filepath.Join(root, ".trackline", "events.jsonl")
	turnState := &session.TurnCounter{Path: filepath.Join(root, ".trackline", "turn.json")}

	// A whole-file write carries no prior state, so a check cannot tell an
	// addition from a change. Measured: an agent asked to add a library
	// rewrote package.json entirely, and dependency-added honestly reported it
	// could not tell what was new.
	//
	// The hook runs *before* the tool, so whatever is on disk right now is the
	// prior state. Reading it here rather than inside a check keeps the rule
	// that checks touch nothing outside their input, and keeps replays
	// deterministic: the content is captured into the event.
	fillPriorBody(&ev)

	e := engine.New(build(cfg, root, turnState)...)
	rep := e.Run(ev, in, rules, intentUnavailable)

	if t, ok := in.Latest(); ok {
		ev.Request = t.Text
		if len(ev.Request) > requestLimit {
			ev.Request = ev.Request[:requestLimit]
		}
		// The report is what the hook uploads from, and it was built before
		// the request was known.
		rep.Event.Request = ev.Request
	}

	// Recorded after the checks run, so a check counting earlier writes does
	// not count the action it is currently judging twice.
	if record && root != "" {
		_ = session.Recorder{Path: recording}.Append(ev)
		_ = turnState.Append(ev)
	}

	// Approvals are applied before anything is decided. A person who has
	// already said yes should not be asked again, and being asked repeatedly
	// about something already settled is how a tool gets switched off.
	grants := override.NewStore(root)
	d := Decision{Mode: cfg.Mode}
	rep, d.Overridden = applyOverrides(rep, grants, ev)
	d.Report = rep
	if cfgErr != nil {
		d.Report.Results = append(d.Report.Results,
			verdict.CannotMeasure("config", cfgErr.Error()))
	}
	if rulesErr != nil {
		d.Report.Results = append(d.Report.Results,
			verdict.CannotMeasure("rules", "a rules file could not be read, so its rules were not applied: "+rulesErr.Error()))
	}

	// Acting needs two things: grounds, and permission. They are separate on
	// purpose, so warn-only is enforced by the structure rather than by
	// everyone remembering.
	switch worstMode(cfg, rep) {
	case config.ModeAuto:
		if rep.Blocked() {
			d.Block = true
			d.Message = blockMessage(rep)
		}
	case config.ModeAsk:
		if len(rep.Findings()) > 0 {
			d.Block = true
			d.Ask = true
			d.Message = askMessage(rep)
		}
	}
	return d
}

// requestLimit bounds the request copied into each recorded event. Every
// action carries one, and a pasted log should not multiply through the file.
const requestLimit = 4000

// fillPriorBody reads what a file currently holds, for a write that replaces it
// whole.
//
// Deliberately narrow: only for writes, only when the host gave no prior text,
// only for a single known path, and only up to the usual body limit. Reading
// more than that on every tool call would cost latency for content no check
// asked for.
func fillPriorBody(ev *event.Event) {
	a := &ev.Action
	if a.Type != event.ActionWriteFile || a.PriorBody != "" || a.PathsUnknown || len(a.Paths) != 1 {
		return
	}
	info, err := os.Stat(a.Paths[0])
	if err != nil || info.IsDir() || info.Size() > event.BodyLimit {
		// A missing file is a new file, which has no prior state and needs
		// none. A file too large to read is left alone, and the check will say
		// it could not measure.
		return
	}
	b, err := os.ReadFile(a.Paths[0])
	if err != nil {
		return
	}
	a.PriorBody, _ = event.TrimBody(string(b))
}

// applyOverrides removes findings a human has already approved.
func applyOverrides(rep engine.Report, grants *override.Store, ev event.Event) (engine.Report, []verdict.Verdict) {
	var removed []verdict.Verdict
	out := rep
	out.Results = nil

	for _, res := range rep.Results {
		if res.Outcome != verdict.OutcomeFinding {
			out.Results = append(out.Results, res)
			continue
		}
		var kept []verdict.Verdict
		for _, v := range res.Verdicts {
			if _, ok := grants.Allows(res.Signal, v.Target, ev.SessionID, ev.TurnID); ok {
				removed = append(removed, v)
				continue
			}
			kept = append(kept, v)
		}
		switch {
		case len(kept) == 0:
			// Everything here was approved. Clean is the right word: the check
			// ran, and what it found has been settled.
			out.Results = append(out.Results, verdict.Clean(res.Signal))
		default:
			res.Verdicts = kept
			out.Results = append(out.Results, res)
		}
	}
	return out, removed
}

// askMessage is what the agent is told when a finding needs a human decision.
//
// Neither host lets a hook prompt a person: "ask" is not a permitted decision,
// only allow or deny. So asking means denying and telling the agent to put the
// question to whoever it is working with.
//
// The message has to name the exact command that records the answer, or the
// agent tries again, is denied identically, and the person is asked the same
// thing forever.
func askMessage(rep engine.Report) string {
	var b strings.Builder
	b.WriteString("PAUSED by trackline. This needs a decision from the person you are working with.\n")

	for _, v := range rep.Findings() {
		fmt.Fprintf(&b, "\n%s\n", v.Summary)
		for _, e := range v.Evidence {
			fmt.Fprintf(&b, "  %s: %s\n", e.Kind, e.Value)
		}
		fmt.Fprintf(&b, "\nAsk them whether to go ahead. If they say yes, run:\n")
		fmt.Fprintf(&b, "    trackline allow %s %q\n", v.Signal, v.Target)
		b.WriteString("and then try again. Do not work around this, and do not decide it yourself.\n")
	}
	return b.String()
}

// build assembles the configured checks.
//
// counter is shared: diff-size and repetition both need the session recording,
// and letting each read it separately cost measurable latency on every tool
// call.
func build(cfg config.Config, root string, counter *session.TurnCounter) []signal.Signal {
	var out []signal.Signal
	if !cfg.IsDisabled(offlimits.Name) {
		out = append(out, offlimits.New(cfg.OffLimits))
	}
	if !cfg.IsDisabled(dependency.Name) {
		out = append(out, dependency.New())
	}
	if !cfg.IsDisabled(scope.Name) {
		out = append(out, scope.New(root))
	}
	if !cfg.IsDisabled(diffsize.Name) {
		out = append(out, diffsize.New(counter))
	}
	if !cfg.IsDisabled(repetition.Name) {
		out = append(out, repetition.New(counter))
	}
	return out
}

// worstMode returns the most permissive-to-act mode among the signals that
// actually found something, so a check configured to block is not held back by
// another configured to warn.
func worstMode(cfg config.Config, rep engine.Report) config.Mode {
	mode := config.ModeWarn
	for _, res := range rep.Results {
		if res.Outcome != verdict.OutcomeFinding {
			continue
		}
		switch cfg.ModeFor(res.Signal) {
		case config.ModeAuto:
			return config.ModeAuto
		case config.ModeAsk:
			mode = config.ModeAsk
		}
	}
	return mode
}

// blockMessage is what the agent is told.
//
// Phase 0 measured 11 corrections out of 11 when the message named a concrete
// alternative, and whether a bare refusal works as well is untested. Until that
// is known, the suggestion is treated as load-bearing and always included.
func blockMessage(rep engine.Report) string {
	var b strings.Builder
	b.WriteString("BLOCKED by trackline.\n")
	for _, v := range rep.Findings() {
		if v.Severity != verdict.SeverityBlock {
			continue
		}
		fmt.Fprintf(&b, "\n%s\n", v.Summary)
		for _, e := range v.Evidence {
			fmt.Fprintf(&b, "  %s: %s\n", e.Kind, e.Value)
		}
		if v.Suggestion != "" {
			fmt.Fprintf(&b, "  do this instead: %s\n", v.Suggestion)
		}
	}
	return b.String()
}
