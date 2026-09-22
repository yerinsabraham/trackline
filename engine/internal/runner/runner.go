// Package runner is the hook's one pass: read an event, work out what was
// asked, run the checks, decide what to do.
//
// Separate from cmd/hook so it can be tested without a process.
package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/claudecode"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/codex"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/engine"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/dependency"
	"github.com/yerinsabraham/trackline/engine/internal/signal/offlimits"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Host names accepted on the command line.
const (
	HostAuto   = "auto"
	HostClaude = "claude"
	HostCodex  = "codex"
)

// Decision is what the hook should do about an event.
type Decision struct {
	// Block is true only when the grounds exist *and* the configured mode
	// permits stopping. Phase 2 ships warn-only, so this stays false.
	Block bool

	// Message is what to hand back to the agent when blocking.
	Message string

	// Report is everything the checks concluded, for the findings log.
	Report engine.Report

	// Mode is what the configuration said to do.
	Mode config.Mode
}

// Detect works out which host sent a payload.
//
// The two are distinguishable without being told: Claude Code sends prompt_id
// and Codex sends turn_id. A flag still wins, because a host that later changes
// its payload should be overridable without waiting for a release.
func Detect(raw []byte) string {
	var probe struct {
		PromptID string `json:"prompt_id"`
		TurnID   string `json:"turn_id"`
		Model    string `json:"model"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return HostClaude
	}
	switch {
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
	default:
		ev, err = claudecode.Parse(raw, opts.Now)
	}
	if err != nil {
		return Decision{}, fmt.Errorf("could not read the %s payload: %w", host, err)
	}

	root := opts.Root
	if root == "" {
		root = ev.CWD
	}

	// A configuration that cannot be read is worth saying out loud, but it must
	// never stop the user working. Defaults carry on.
	cfg, cfgErr := config.Load(root)
	rules, _ := config.LoadRules(root, cfg)

	var in intent.Intent
	if ev.TranscriptPath != "" {
		_ = (&intent.Reader{Path: ev.TranscriptPath}).Read(&in)
	}

	e := engine.New(build(cfg)...)
	rep := e.Run(ev, in, rules)

	d := Decision{Report: rep, Mode: cfg.Mode}
	if cfgErr != nil {
		d.Report.Results = append(d.Report.Results,
			verdict.CannotMeasure("config", cfgErr.Error()))
	}

	// Blocking needs two things: grounds, and permission. Phase 2 ships
	// warn-only, so the second is absent by default and nothing is stopped.
	if rep.Blocked() {
		if mode := worstMode(cfg, rep); mode == config.ModeAuto {
			d.Block = true
			d.Message = blockMessage(rep)
		}
	}
	return d, nil
}

// build assembles the configured checks.
func build(cfg config.Config) []signal.Signal {
	var out []signal.Signal
	if !cfg.IsDisabled(offlimits.Name) {
		out = append(out, offlimits.New(cfg.OffLimits))
	}
	if !cfg.IsDisabled(dependency.Name) {
		out = append(out, dependency.New())
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
