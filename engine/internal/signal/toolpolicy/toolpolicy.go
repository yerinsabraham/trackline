// Package toolpolicy checks a production agent's tool calls against a stated
// policy: tools it must never call, and tools it may only call after another
// has run in the same conversation.
//
// This is the check experiment 3 was about. A support agent was told it must
// never change a credit limit without a human approving it, and called the
// limit tool anyway. On a default trace that call is invisible: the span says a
// tool was called, not which. So the check's most important answer is often
// that it cannot tell, and it must say so rather than pass the call.
package toolpolicy

import (
	"fmt"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is the check's name in config and output.
const Name = "tool-policy"

// History returns the actions already taken in an event's conversation, oldest
// first, not including the event itself.
//
// Taken as a dependency, like the session counter the coding checks use, so
// the check reads nothing it was not handed and a replay gives the same answer.
type History func(ev event.Event) []event.Event

// Signal is the check.
type Signal struct {
	policy  config.ToolPolicy
	history History
}

// New builds the check.
func New(p config.ToolPolicy, h History) Signal { return Signal{policy: p, history: h} }

func (Signal) Name() string { return Name }

func (s Signal) Check(in signal.Input) verdict.Result {
	a := in.Event.Action
	if a.Type != event.ActionCallTool {
		return verdict.NotApplicable(Name, "this action is not a tool call")
	}
	if s.policy.Empty() {
		return verdict.NotApplicable(Name, "no tool policy is configured")
	}
	if a.ToolNameUnknown || a.ToolName == "" {
		return verdict.CannotMeasure(Name, "the trace shows that a tool was called but not which one; "+
			"tool names are only recorded when message content capture is enabled")
	}

	for _, never := range s.policy.Never {
		if a.ToolName == never {
			return verdict.Finding(Name, verdict.Verdict{
				Severity: verdict.SeverityBlock,
				Target:   a.ToolName,
				Summary:  fmt.Sprintf("called %s, which the policy says must never be called", a.ToolName),
				Evidence: []verdict.Evidence{
					{Kind: verdict.EvidenceRule, Value: "never: " + never, Note: "the tool policy"},
					{Kind: verdict.EvidenceCommand, Value: a.ToolName, Note: "what was called"},
				},
				Suggestion: "find out why the agent reached for it, and whether it is still exposed to the model",
			})
		}
	}

	need, gated := s.policy.RequireApproval[a.ToolName]
	if !gated {
		return verdict.Clean(Name)
	}
	var prior []event.Event
	if s.history != nil {
		prior = s.history(in.Event)
	}
	for _, p := range prior {
		if p.Action.ToolName == need {
			return verdict.Clean(Name)
		}
	}
	return verdict.Finding(Name, verdict.Verdict{
		Severity: verdict.SeverityBlock,
		Target:   a.ToolName,
		Summary:  fmt.Sprintf("called %s without %s first", a.ToolName, need),
		Evidence: []verdict.Evidence{
			{Kind: verdict.EvidenceRule, Value: fmt.Sprintf("%s requires %s", a.ToolName, need), Note: "the tool policy"},
			{Kind: verdict.EvidenceCommand, Value: a.ToolName, Note: "what was called"},
		},
		Suggestion: "the approval step was skipped; check whether the tool can be reached without it",
	})
}
