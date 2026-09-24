package otlp_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/otlp"
)

// The fixtures are what the official Python OTLP/HTTP exporter sent, unmodified,
// from an agent instrumented with opentelemetry-instrumentation-openai-v2. The
// JSON is the same request in the OTLP/JSON mapping.

func read(t *testing.T, name, ct string) []otel.Span {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	spans, err := otlp.Decode(b, ct)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return spans
}

func TestRealProtobufExport(t *testing.T) {
	spans := read(t, "000.pb", "application/x-protobuf")
	if len(spans) != 54 {
		t.Fatalf("got %d spans, want the 54 the exporter sent", len(spans))
	}
	kinds := map[string]int{}
	for _, s := range spans {
		kinds[s.Attributes["gen_ai.operation.name"]]++
		if s.TraceID == "" || s.SpanID == "" || s.Start.IsZero() || s.End.Before(s.Start) {
			t.Fatalf("span %q is missing identity or timing: %+v", s.Name, s)
		}
		if s.Service != "northwind-support" {
			t.Fatalf("service = %q; it comes from the resource, not the span", s.Service)
		}
	}
	for _, k := range []string{"invoke_agent", "chat", "execute_tool"} {
		if kinds[k] == 0 {
			t.Errorf("no %s spans decoded: %v", k, kinds)
		}
	}
}

// Every attribute that production checks read, in the shape they read it.
func TestAttributesThatMatter(t *testing.T) {
	var chat, tool, root otel.Span
	for _, s := range read(t, "000.pb", "") {
		switch s.Attributes["gen_ai.operation.name"] {
		case "chat":
			chat = s
		case "execute_tool":
			tool = s
		case "invoke_agent":
			root = s
		}
	}
	if chat.Attributes["gen_ai.input.messages"] == "" || chat.Attributes["gen_ai.output.messages"] == "" {
		t.Error("message content must survive decoding")
	}
	// An array attribute. Rendered as JSON, so a check can still find it.
	if got := chat.Attributes["gen_ai.response.finish_reasons"]; got != `["tool_calls"]` && got != `["stop"]` {
		t.Errorf("finish_reasons = %q", got)
	}
	if chat.Attributes["gen_ai.usage.input_tokens"] == "" {
		t.Error("an int attribute must decode")
	}
	if tool.Attributes["gen_ai.tool.name"] == "" || tool.ParentSpanID == "" {
		t.Errorf("tool span = %+v", tool)
	}
	if root.Attributes["gen_ai.conversation.id"] == "" || root.ParentSpanID != "" {
		t.Errorf("root span = %+v", root)
	}
}

func TestFailedToolCallsCarryErrorStatus(t *testing.T) {
	var errors int
	for _, f := range []string{"000.pb", "001.pb", "002.pb", "003.pb", "004.pb", "005.pb", "006.pb"} {
		for _, s := range read(t, f, "") {
			if s.Status == "error" {
				errors++
				if s.StatusMessage == "" {
					t.Errorf("error span %q has no message", s.Name)
				}
			}
		}
	}
	// Six conversations each hit one failing tool call.
	if errors != 6 {
		t.Errorf("error spans = %d, want 6", errors)
	}
}

// The same request in both encodings must decode identically, or the answer
// would depend on how a deployment happens to configure its exporter.
func TestJSONAndProtobufAgree(t *testing.T) {
	pb := read(t, "000.pb", "application/x-protobuf")
	js := read(t, "000.json", "application/json")
	key := func(ss []otel.Span) {
		sort.Slice(ss, func(i, j int) bool { return ss[i].SpanID < ss[j].SpanID })
	}
	key(pb)
	key(js)
	if len(pb) != len(js) {
		t.Fatalf("protobuf %d spans, json %d", len(pb), len(js))
	}
	for i := range pb {
		if !reflect.DeepEqual(pb[i], js[i]) {
			t.Fatalf("span %d differs:\nproto %+v\njson  %+v", i, pb[i], js[i])
		}
	}
}

func TestTruncatedInputIsAnError(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join("testdata", "000.pb"))
	if _, err := otlp.Decode(b[:len(b)/2], ""); err == nil {
		t.Error("half a request must be an error, never a shorter list of spans")
	}
}
