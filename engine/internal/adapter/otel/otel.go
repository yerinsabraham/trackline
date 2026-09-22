// Package otel normalises an OpenTelemetry GenAI span into events.
//
// This is the surface the whole "one engine, two surfaces" bet rests on, and
// Phase 0 established that it is also the riskiest. A production span is not
// like a hook event in one decisive way: the attributes carrying what the agent
// was told and what it did are marked Opt-In in the specification and are off
// unless an operator deliberately enables them.
//
// Measured against the real instrumentation:
//
//   - Default settings give eight metadata attributes. You can see that a tool
//     was called, from gen_ai.response.finish_reasons, but not which one.
//   - Fully opted in, gen_ai.input.messages and gen_ai.output.messages carry the
//     system rule, the user request and the tool call with its arguments.
//   - Opted in with PII redaction, alignment still works: the rule and the tool
//     name survive while identifiers and argument values are scrubbed.
//
// See docs/experiments/03-otel-traces.md. The adapter's job is to make that
// gradient explicit rather than let a thin trace read as a clean one.
package otel

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
)

// Attribute names, from the GenAI semantic conventions. They are still marked
// Development in the specification and have already moved repositories, so they
// are isolated here rather than spread through the engine.
const (
	attrOperation    = "gen_ai.operation.name"
	attrInputMsgs    = "gen_ai.input.messages"
	attrOutputMsgs   = "gen_ai.output.messages"
	attrSystemInstr  = "gen_ai.system_instructions"
	attrFinishReason = "gen_ai.response.finish_reasons"
	attrRequestModel = "gen_ai.request.model"
	attrConversation = "gen_ai.conversation.id"
)

// Span is the minimum both an OTLP payload and an SDK export reduce to.
//
// Deliberately not the full OTLP type: the engine needs a name, attributes and
// identity, and taking the whole protobuf would couple the core to a wire
// format that is still changing.
type Span struct {
	Name       string            `json:"name"`
	TraceID    string            `json:"traceId,omitempty"`
	SpanID     string            `json:"spanId,omitempty"`
	Attributes map[string]string `json:"attributes"`
}

// Parsed is everything one span yields.
//
// A span carries the action, the request and the rules together, where a hook
// event carries only the action and points at a transcript for the rest.
type Parsed struct {
	// Events are the actions the agent took in this span.
	Events []event.Event

	// UserMessages are the human turns stated in this span's input.
	UserMessages []string

	// SystemRules are constraints stated in the system prompt. Experiment 0.5
	// found the system message inside gen_ai.input.messages as a role:"system"
	// entry rather than in gen_ai.system_instructions, so both are read.
	SystemRules []string

	// ContentAvailable is false when nothing readable came back. Everything
	// above will be empty, and that is a configuration or data state, never a
	// clean result.
	ContentAvailable bool

	// ContentCorrupt distinguishes the two ways content can be missing, which
	// need different answers from an operator.
	//
	// Absent means they never opted in: a setting to change. Corrupt means the
	// attribute is present and unreadable, which in practice means a redaction
	// pass ran a regex over the serialised JSON and replaced a number with a
	// bare token, leaving `"new_limit":[REDACTED]`. That is not valid JSON, so
	// a redaction meant to hide one value destroys every other. Telling them
	// "enable content capture" when it is already enabled would send them the
	// wrong way.
	ContentCorrupt bool

	// Reason explains a false ContentAvailable, in words fit to show a user.
	Reason string
}

type message struct {
	Role  string `json:"role"`
	Parts []struct {
		Type      string          `json:"type"`
		Content   string          `json:"content"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		ID        string          `json:"id"`
	} `json:"parts"`
}

// Parse normalises one span.
func Parse(s Span, now time.Time) Parsed {
	sessionID := s.Attributes[attrConversation]
	if sessionID == "" {
		// No conversation id even when fully opted in, so trace context is the
		// only available grouping.
		sessionID = s.TraceID
	}

	base := event.Event{
		SessionID: sessionID,
		TurnID:    s.SpanID,
		At:        now,
		Host:      event.HostOTel,
		// A span is a record of something that already happened, so nothing
		// here can be blocked. A check firing on it may warn, never claim to
		// have prevented anything.
		Phase: event.PhasePostTool,
	}

	out := Parsed{}

	sysMsgs, _ := parseMessages(s.Attributes[attrSystemInstr])
	for _, m := range sysMsgs {
		out.SystemRules = append(out.SystemRules, textOf(m))
	}

	inputs, inputBad := parseMessages(s.Attributes[attrInputMsgs])
	for _, m := range inputs {
		switch m.Role {
		case "user":
			if txt := textOf(m); txt != "" {
				out.UserMessages = append(out.UserMessages, txt)
			}
		case "system", "developer":
			if txt := textOf(m); txt != "" {
				out.SystemRules = append(out.SystemRules, txt)
			}
		}
	}

	outputs, outputBad := parseMessages(s.Attributes[attrOutputMsgs])
	for _, m := range outputs {
		for _, p := range m.Parts {
			if p.Type != "tool_call" {
				continue
			}
			ev := base
			ev.ID = p.ID
			ev.Action = event.Action{
				Type:     event.ActionCallTool,
				ToolName: p.Name,
				// A production tool call says nothing about files. That is
				// unknown, not empty.
				PathsUnknown: true,
			}
			out.Events = append(out.Events, ev)
		}
	}

	out.ContentAvailable = len(inputs) > 0 || len(outputs) > 0
	out.ContentCorrupt = inputBad || outputBad

	if !out.ContentAvailable {
		if out.ContentCorrupt {
			out.Reason = "the trace carries message content that could not be parsed. " +
				"A redaction pass run over the serialised JSON will do this: replacing a " +
				"number with a bare token leaves invalid JSON and the whole message list " +
				"becomes unreadable. Redact inside string values instead"
		} else {
			out.Reason = "the trace carries no message content; gen_ai.input.messages and " +
				"gen_ai.output.messages are Opt-In and this operator has not enabled them"
		}

		// The one thing a default trace does reveal: that a tool was called.
		// Recording it as an event with the name unknown is what stops a check
		// reporting clean on an action it never saw.
		if strings.Contains(s.Attributes[attrFinishReason], "tool_call") {
			ev := base
			ev.Action = event.Action{
				Type:            event.ActionCallTool,
				ToolNameUnknown: true,
				PathsUnknown:    true,
			}
			out.Events = append(out.Events, ev)
		}
	}

	return out
}

// parseMessages reads one of the message-carrying attributes.
//
// The attribute is a JSON string. Anything unparseable is treated as absent
// rather than partially read: a truncated message list would silently drop the
// tail, and the tail is where a later tool call would be.
//
// corrupt separates "the attribute was not there" from "it was there and would
// not parse", because those need different advice.
func parseMessages(raw string) (msgs []message, corrupt bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	if json.Unmarshal([]byte(raw), &msgs) != nil {
		return nil, true
	}
	return msgs, false
}

func textOf(m message) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Type == "text" && p.Content != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Content)
		}
	}
	return b.String()
}
