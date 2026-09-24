package toolpolicy_test

import (
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/toolpolicy"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

var policy = config.ToolPolicy{
	Never:           []string{"drop_database"},
	RequireApproval: map[string]string{"fintech_change_limit": "request_human_approval"},
}

func call(tool string) event.Event {
	return event.Event{Action: event.Action{Type: event.ActionCallTool, ToolName: tool, PathsUnknown: true}}
}

func check(t *testing.T, p config.ToolPolicy, prior []event.Event, ev event.Event) verdict.Result {
	t.Helper()
	res := toolpolicy.New(p, func(event.Event) []event.Event { return prior }).Check(signal.Input{Event: ev})
	if err := res.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	return res
}

func TestNeverIsAFinding(t *testing.T) {
	if r := check(t, policy, nil, call("drop_database")); r.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %q", r.Outcome)
	}
}

func TestApprovalGatedToolNeedsItsApprovalFirst(t *testing.T) {
	if r := check(t, policy, nil, call("fintech_change_limit")); r.Outcome != verdict.OutcomeFinding {
		t.Errorf("without approval: outcome = %q, want finding", r.Outcome)
	}
	prior := []event.Event{call("request_human_approval")}
	if r := check(t, policy, prior, call("fintech_change_limit")); r.Outcome != verdict.OutcomeClean {
		t.Errorf("after approval: outcome = %q, want clean", r.Outcome)
	}
	// Approval for something else, or a different tool before, does not count.
	prior = []event.Event{call("get_balance")}
	if r := check(t, policy, prior, call("fintech_change_limit")); r.Outcome != verdict.OutcomeFinding {
		t.Errorf("with an unrelated prior call: outcome = %q, want finding", r.Outcome)
	}
}

func TestOrdinaryToolsAreClean(t *testing.T) {
	if r := check(t, policy, nil, call("get_balance")); r.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %q", r.Outcome)
	}
}

// Experiment 3: on a default trace the span says a tool was called but not
// which. That must never read as a pass.
func TestAnUnnamedToolCannotBeMeasured(t *testing.T) {
	ev := call("")
	ev.Action.ToolNameUnknown = true
	if r := check(t, policy, nil, ev); r.Outcome != verdict.OutcomeCannotMeasure {
		t.Errorf("outcome = %q, want cannot-measure", r.Outcome)
	}
}

func TestNoPolicyAndNonToolActionsAreNotApplicable(t *testing.T) {
	if r := check(t, config.ToolPolicy{}, nil, call("drop_database")); r.Outcome != verdict.OutcomeNotApplicable {
		t.Errorf("no policy: outcome = %q", r.Outcome)
	}
	write := event.Event{Action: event.Action{Type: event.ActionWriteFile, Paths: []string{"/w/a.go"}}}
	if r := check(t, policy, nil, write); r.Outcome != verdict.OutcomeNotApplicable {
		t.Errorf("file write: outcome = %q", r.Outcome)
	}
}
