// Package repetition notices an agent doing the same thing over and over.
//
// The build plan called this "repeated failure", and the name had to change to
// match what is actually observable. A PreToolUse hook runs *before* the tool,
// so it cannot know whether the last attempt failed. What it can see is that
// the same action is being attempted again, and again.
//
// Repetition is a proxy for being stuck, not proof of it. An agent rewriting
// one file five times in a single turn is usually failing to get something
// right and trying variations; occasionally it is doing legitimate incremental
// work. So this warns, names the action, and leaves the judgement to the human.
//
// Naming it for what it detects rather than what it implies matters. A check
// called "repeated failure" that has never seen a failure would be claiming
// more than it knows.
package repetition

import (
	"fmt"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "repetition"

// Threshold is how many attempts at the identical action before it is worth
// mentioning.
//
// Three is a retry. Four is a pattern. Set lower and this fires on the ordinary
// write-test-fix rhythm of real work.
const Threshold = 4

// History supplies the actions already taken in this turn.
type History interface {
	// ActionsInTurn returns the events already recorded under turnID, in order,
	// excluding the current one.
	ActionsInTurn(sessionID, turnID string) ([]event.Event, error)
}

// Signal reports an action repeated within one request.
type Signal struct {
	History History
}

// New builds the check.
func New(h History) Signal { return Signal{History: h} }

func (Signal) Name() string { return Name }

func (s Signal) Check(in signal.Input) verdict.Result {
	if in.Event.TurnID == "" {
		return verdict.NotApplicable(Name, "the host did not group this action under a turn, so repeats cannot be counted")
	}
	if s.History == nil {
		return verdict.CannotMeasure(Name, "no record of earlier actions is available, so repeats could not be counted")
	}

	key, ok := fingerprint(in.Event)
	if !ok {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"%s does not describe itself precisely enough to tell one attempt from another",
			describeTool(in.Event)))
	}

	prior, err := s.History.ActionsInTurn(in.Event.SessionID, in.Event.TurnID)
	if err != nil {
		return verdict.CannotMeasure(Name, fmt.Sprintf("earlier actions could not be read: %v", err))
	}

	count := 1 // this attempt
	for _, ev := range prior {
		if k, ok := fingerprint(ev); ok && k == key {
			count++
		}
	}

	if count < Threshold {
		return verdict.Clean(Name)
	}

	return verdict.Finding(Name, verdict.Verdict{
		Severity: verdict.SeverityWarn,
		Summary: fmt.Sprintf("the same action has now been attempted %d times in this request",
			count),
		Evidence: []verdict.Evidence{
			{Kind: verdict.EvidenceCount, Value: fmt.Sprintf("%d attempts", count),
				Note: "how many times"},
			{Kind: evidenceKind(in.Event), Value: describeAction(in.Event),
				Note: "the action being repeated"},
		},
		Suggestion: "if this is not converging, stop and say what is blocking rather than trying again",
	})
}

// fingerprint identifies an action precisely enough that two attempts at the
// same thing match and two different attempts do not.
//
// ok is false when the action does not describe itself well enough to compare,
// which is the honest answer for a tool whose input the adapter cannot see.
func fingerprint(e event.Event) (string, bool) {
	a := e.Action
	switch a.Type {
	case event.ActionRunCommand:
		if strings.TrimSpace(a.Command) == "" {
			return "", false
		}
		return "cmd:" + strings.Join(strings.Fields(a.Command), " "), true
	case event.ActionWriteFile, event.ActionEditFile, event.ActionDeleteFile, event.ActionReadFile:
		if a.PathsUnknown || len(a.Paths) == 0 {
			return "", false
		}
		return string(a.Type) + ":" + strings.Join(a.Paths, "|"), true
	case event.ActionCallTool:
		if a.ToolNameUnknown || a.ToolName == "" {
			return "", false
		}
		return "tool:" + a.ToolName, true
	default:
		return "", false
	}
}

func evidenceKind(e event.Event) verdict.EvidenceKind {
	if e.Action.Type == event.ActionRunCommand {
		return verdict.EvidenceCommand
	}
	return verdict.EvidenceFile
}

func describeAction(e event.Event) string {
	if e.Action.Type == event.ActionRunCommand {
		return e.Action.Command
	}
	return strings.Join(e.Action.Paths, ", ")
}

func describeTool(e event.Event) string {
	if e.Action.ToolName != "" {
		return e.Action.ToolName
	}
	return string(e.Action.Type)
}
