package adapter_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/claudecode"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/event"
)

// The spans below are real output from the official
// opentelemetry-instrumentation-openai-v2 package, captured under three
// configurations. See docs/experiments/03-otel-traces.md.
func span(t *testing.T, name string) otel.Span {
	t.Helper()
	var s otel.Span
	if err := json.Unmarshal(load(t, name), &s); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	return s
}

// The configuration most operators are in without knowing it: content capture
// off. The trace proves a tool was called and refuses to say which.
func TestDefaultTraceIsHonestAboutWhatItCannotSee(t *testing.T) {
	got := otel.Parse(span(t, "otel-default.json"), now)

	if got.ContentAvailable {
		t.Fatal("a default trace carries no message content")
	}
	if got.Reason == "" {
		t.Error("an unavailable-content trace must explain itself")
	}
	if len(got.UserMessages) != 0 || len(got.SystemRules) != 0 {
		t.Error("no content means no request and no rules")
	}

	if len(got.Events) != 1 {
		t.Fatalf("got %d events; finish_reasons said a tool was called, so one event must be recorded", len(got.Events))
	}
	a := got.Events[0].Action
	if a.Type != event.ActionCallTool {
		t.Errorf("action = %q, want call-tool", a.Type)
	}
	if !a.ToolNameUnknown {
		t.Error("the tool name is genuinely unknown here and must be marked so")
	}
	if got.Events[0].Observable() {
		t.Error("an action whose tool is unknown is not observable; a check must return cannot-measure, not clean")
	}
}

// Fully opted in: everything alignment needs.
func TestOptedInTraceCarriesRuleRequestAndAction(t *testing.T) {
	got := otel.Parse(span(t, "otel-opted-in.json"), now)

	if !got.ContentAvailable {
		t.Fatal("content capture was on for this span")
	}
	if len(got.SystemRules) != 1 || !strings.Contains(got.SystemRules[0], "NEVER change a customer's credit limit") {
		t.Errorf("system rule not recovered: %q", got.SystemRules)
	}
	if len(got.UserMessages) != 1 || !strings.Contains(got.UserMessages[0], "charged twice") {
		t.Errorf("user request not recovered: %q", got.UserMessages)
	}
	if len(got.Events) != 1 {
		t.Fatalf("got %d events, want the one tool call", len(got.Events))
	}
	a := got.Events[0].Action
	if a.ToolName != "fintech_change_limit" {
		t.Errorf("tool = %q", a.ToolName)
	}
	if a.ToolNameUnknown {
		t.Error("the tool name is right there; it must not be marked unknown")
	}
	// Everything needed to call this misaligned: a rule forbidding it, a
	// request about something else, and the action itself.
	if !got.Events[0].Observable() {
		t.Error("a fully opted-in tool call is observable")
	}
}

// The result that matters commercially: an operator can scrub PII and still be
// checkable.
func TestRedactedTraceStillSupportsAlignment(t *testing.T) {
	got := otel.Parse(span(t, "otel-redacted.json"), now)

	if !got.ContentAvailable {
		t.Fatal("redacted content is still content")
	}
	if len(got.SystemRules) != 1 || !strings.Contains(got.SystemRules[0], "NEVER change") {
		t.Error("redaction must not remove the rule")
	}
	if len(got.UserMessages) != 1 || !strings.Contains(got.UserMessages[0], "charged twice") {
		t.Error("redaction must not remove the shape of the request")
	}
	if strings.Contains(got.UserMessages[0], "5512") {
		t.Error("the order number should have been scrubbed by the operator")
	}
	if len(got.Events) != 1 || got.Events[0].Action.ToolName != "fintech_change_limit" {
		t.Fatalf("the tool call must survive redaction: %+v", got.Events)
	}
}

// The property the plan asked for, across the two surfaces rather than the two
// hook hosts: the same fact normalises to the same event.
//
// An agent calling a tool is the same fact whether a hook reports it locally or
// a span reports it from production. Only Phase differs, because a span is a
// record of something that already happened and cannot be blocked.
func TestLocalAndProductionAgreeOnAToolCall(t *testing.T) {
	hook := []byte(`{
		"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_use_id":"call_1","tool_name":"fintech_change_limit","cwd":"/w",
		"tool_input":{"customer_id":"c_99","new_limit":999999}}`)

	local, err := claudecode.Parse(hook, now)
	if err != nil {
		t.Fatal(err)
	}
	prod := otel.Parse(span(t, "otel-opted-in.json"), now)
	if len(prod.Events) != 1 {
		t.Fatalf("got %d production events", len(prod.Events))
	}
	remote := prod.Events[0]

	if local.Action.ToolName != remote.Action.ToolName {
		t.Errorf("tool name differs: local=%q production=%q", local.Action.ToolName, remote.Action.ToolName)
	}
	if local.ID != remote.ID {
		t.Errorf("call id differs: local=%q production=%q", local.ID, remote.ID)
	}
	// Neither surface can say which files a tool call touched.
	if !local.Action.PathsUnknown || !remote.Action.PathsUnknown {
		t.Error("a tool call reveals no file paths on either surface")
	}
	// The one legitimate difference.
	if !local.CanBlock() {
		t.Error("a local pre-tool event can be blocked")
	}
	if remote.CanBlock() {
		t.Error("a production span records the past and must never claim it can block")
	}
}

func TestMalformedContentIsTreatedAsAbsentNotPartial(t *testing.T) {
	// A truncated message list would silently drop its tail, and the tail is
	// where a later tool call would be.
	s := otel.Span{Name: "chat", Attributes: map[string]string{
		"gen_ai.output.messages": `[{"role":"assistant","parts":[{"type":"tool_call","name":"a"`,
	}}
	got := otel.Parse(s, now)
	if len(got.Events) != 0 {
		t.Errorf("a half-parsed message list must yield nothing, got %+v", got.Events)
	}
	if got.ContentAvailable {
		t.Error("unparseable content is not available content")
	}
}

// Observable depends on the kind of action, and getting that wrong marks
// healthy events unobservable or unseen ones observable. Both are bad in
// opposite directions.
func TestObservableDependsOnActionKind(t *testing.T) {
	cases := []struct {
		name string
		a    event.Action
		want bool
	}{
		{"tool call with a name", event.Action{Type: event.ActionCallTool, ToolName: "x", PathsUnknown: true}, true},
		{"tool call without a name", event.Action{Type: event.ActionCallTool, ToolNameUnknown: true, PathsUnknown: true}, false},
		{"write with a path", event.Action{Type: event.ActionWriteFile, Paths: []string{"/a"}}, true},
		{"write with unknown paths", event.Action{Type: event.ActionWriteFile, PathsUnknown: true}, false},
		{"command with text", event.Action{Type: event.ActionRunCommand, Command: "ls", PathsUnknown: true}, true},
		{"command with no text", event.Action{Type: event.ActionRunCommand, PathsUnknown: true}, false},
		{"unclassified action", event.Action{Type: event.ActionOther, PathsUnknown: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (event.Event{Action: c.a}).Observable(); got != c.want {
				t.Errorf("Observable() = %v, want %v", got, c.want)
			}
		})
	}
}

// A redaction pass run over the serialised JSON replaces a number with a bare
// token, leaving `"new_limit":[REDACTED]`. That is invalid JSON, so a redaction
// meant to hide one value destroys every other. The adapter must say which of
// the two failures happened, because the advice differs.
func TestCorruptContentIsDistinguishedFromAbsentContent(t *testing.T) {
	corrupt := otel.Parse(otel.Span{Name: "chat", Attributes: map[string]string{
		"gen_ai.output.messages": `[{"role":"assistant","parts":[{"arguments":{"new_limit":[REDACTED]},"name":"x","type":"tool_call"}]}]`,
	}}, now)

	if corrupt.ContentAvailable {
		t.Error("unparseable content is not available content")
	}
	if !corrupt.ContentCorrupt {
		t.Fatal("content was present and unreadable; that is corrupt, not absent")
	}
	if !strings.Contains(corrupt.Reason, "Redact inside string values") {
		t.Errorf("reason should point at the real cause, got: %q", corrupt.Reason)
	}

	absent := otel.Parse(span(t, "otel-default.json"), now)
	if absent.ContentCorrupt {
		t.Error("a default trace has no content at all; that is absent, not corrupt")
	}
	if !strings.Contains(absent.Reason, "Opt-In") {
		t.Errorf("reason should point at the setting, got: %q", absent.Reason)
	}
}
