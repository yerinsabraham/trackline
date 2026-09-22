package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/engine"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

type stub struct {
	name string
	fn   func(signal.Input) verdict.Result
}

func (s stub) Name() string                         { return s.name }
func (s stub) Check(in signal.Input) verdict.Result { return s.fn(in) }

func ev() event.Event {
	return event.Event{
		ID: "u1", SessionID: "s", TurnID: "t", Host: event.HostClaudeCode,
		Phase: event.PhasePreTool,
		Action: event.Action{
			Type: event.ActionWriteFile, ToolName: "Write",
			Paths: []string{"/work/payments/charge.ts"},
		},
	}
}

func TestRunCollectsEverySignal(t *testing.T) {
	e := engine.New(
		stub{"a", func(signal.Input) verdict.Result { return verdict.Clean("a") }},
		stub{"b", func(signal.Input) verdict.Result { return verdict.NotApplicable("b", "no scope stated") }},
	)
	rep := e.Run(ev(), intent.Intent{}, nil)
	if len(rep.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(rep.Results))
	}
	if rep.Blocked() {
		t.Error("nothing found, nothing should be blocked")
	}
}

// A signal that panics must not take the process down, and must not be silently
// dropped either. On Codex a crashed hook is a silently disabled hook, so one
// bad check cannot be allowed to switch off the rest.
func TestPanickingSignalBecomesCannotMeasure(t *testing.T) {
	e := engine.New(
		stub{"boom", func(signal.Input) verdict.Result { panic("nil map or something") }},
		stub{"fine", func(signal.Input) verdict.Result { return verdict.Clean("fine") }},
	)
	rep := e.Run(ev(), intent.Intent{}, nil)

	if len(rep.Results) != 2 {
		t.Fatalf("got %d results; the healthy signal must still run", len(rep.Results))
	}
	un := rep.Unmeasured()
	if len(un) != 1 || un[0].Signal != "boom" {
		t.Fatalf("unmeasured = %+v; a panic must surface as cannot-measure", un)
	}
	if !strings.Contains(un[0].Reason, "panicked") {
		t.Errorf("reason = %q; it should say what happened", un[0].Reason)
	}
}

// The riskViolationRate lesson, enforced in the type system this time: a check
// that could not run must never be indistinguishable from one that found
// nothing.
func TestCannotMeasureIsNotClean(t *testing.T) {
	e := engine.New(stub{"s", func(signal.Input) verdict.Result {
		return verdict.CannotMeasure("s", "no rules file found and scope was never stated")
	}})
	rep := e.Run(ev(), intent.Intent{}, nil)

	if len(rep.Findings()) != 0 {
		t.Error("cannot-measure is not a finding")
	}
	if len(rep.Unmeasured()) != 1 {
		t.Fatal("cannot-measure must be reported, not dropped")
	}
	if rep.Results[0].Outcome == verdict.OutcomeClean {
		t.Error("cannot-measure must never read as clean")
	}
}

// A verdict that cannot name what it saw is not shippable. A signal producing
// one is buggy, and the honest report of a buggy check is that it measured
// nothing.
func TestInterruptingVerdictWithoutEvidenceIsRejected(t *testing.T) {
	e := engine.New(stub{"vague", func(signal.Input) verdict.Result {
		return verdict.Finding("vague", verdict.Verdict{
			Severity: verdict.SeverityWarn,
			Summary:  "the agent seems off track",
			// no evidence
		})
	}})
	rep := e.Run(ev(), intent.Intent{}, nil)

	if len(rep.Findings()) != 0 {
		t.Error("a warn verdict with no evidence must not reach the user")
	}
	if len(rep.Unmeasured()) != 1 {
		t.Fatalf("results = %+v; an invalid verdict becomes cannot-measure", rep.Results)
	}
}

func TestEvidenceBackedFindingSurvives(t *testing.T) {
	e := engine.New(stub{"scope", func(in signal.Input) verdict.Result {
		anchor, ok := in.Intent.Anchor()
		if !ok {
			return verdict.NotApplicable("scope", "nothing has been asked yet")
		}
		return verdict.Finding("scope", verdict.Verdict{
			Severity: verdict.SeverityBlock,
			Summary:  "edited a file outside the stated scope",
			Evidence: []verdict.Evidence{
				{Kind: verdict.EvidenceTurn, Value: anchor.Text, Note: "what was asked"},
				{Kind: verdict.EvidenceFile, Value: in.Event.Action.Paths[0], Note: "what was edited"},
			},
			Suggestion: "leave payments alone and work in the auth module",
		})
	}})

	var in intent.Intent
	in.Add("t", time.Now(), "Fix the login bug in the auth module")
	in.Add("t2", time.Now(), "proceed")

	rep := e.Run(ev(), in, nil)
	f := rep.Findings()
	if len(f) != 1 {
		t.Fatalf("got %d findings, want 1", len(f))
	}
	if !rep.Blocked() {
		t.Error("a block-severity finding establishes grounds to block")
	}
	// The anchor, not the latest turn: "proceed" must not become the evidence.
	if f[0].Evidence[0].Value != "Fix the login bug in the auth module" {
		t.Errorf("evidence cites %q; it must cite the instruction, not the continuation", f[0].Evidence[0].Value)
	}
}
