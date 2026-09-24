package production_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/production"
)

func summarise(rs []production.Result) map[string]string {
	out := map[string]string{}
	for _, r := range rs {
		var parts []string
		for _, v := range r.Findings() {
			parts = append(parts, v.Summary)
		}
		for _, i := range r.Incidents {
			parts = append(parts, i.Kind)
		}
		out[r.Conversation.ID] = strings.Join(parts, "|") + " actions=" + itoa(len(r.Conversation.Actions))
	}
	return out
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// The captured export requests, posted in order the way the exporter sent
// them, must produce exactly what reading the same files offline produces.
func TestLiveReceiverMatchesOffline(t *testing.T) {
	var mu sync.Mutex
	var live []production.Result
	srv := production.NewServer(policy())
	srv.Emit = func(r production.Result) { mu.Lock(); live = append(live, r); mu.Unlock() }
	ts := httptest.NewServer(srv)
	defer ts.Close()

	files, _ := filepath.Glob(filepath.Join("..", "otlp", "testdata", "*.pb"))
	sort.Strings(files)
	for _, f := range files {
		b, _ := os.ReadFile(f)
		resp, err := http.Post(ts.URL+"/v1/traces", "application/x-protobuf", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s: status %d: %s", f, resp.StatusCode, body)
		}
		resp.Body.Close()
	}
	srv.Flush(true)

	offline := production.Process(stream(t), policy(), production.NewMonitor())
	got, want := summarise(live), summarise(offline)
	if len(got) != 45 {
		t.Fatalf("live produced %d conversations, want 45", len(got))
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: live %q, offline %q", id, got[id], w)
		}
	}
}

// A request groups spans by scope, not time, so a root can precede its own
// children in one request. The trace must not be split.
func TestRootBeforeChildrenInOneRequest(t *testing.T) {
	var got []production.Result
	srv := production.NewServer(policy())
	srv.Emit = func(r production.Result) { got = append(got, r) }
	srv.Accept([]otel.Span{
		{TraceID: "t", SpanID: "root", Name: "invoke_agent", Attributes: map[string]string{"gen_ai.operation.name": "invoke_agent", "gen_ai.conversation.id": "c1"}},
		{TraceID: "t", SpanID: "tool", ParentSpanID: "root", Attributes: map[string]string{"gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": "fintech_change_limit"}},
	})
	srv.Flush(true)
	if len(got) != 1 || len(got[0].Conversation.Actions) != 1 || len(got[0].Findings()) != 1 {
		t.Fatalf("got %d results; the trace must arrive whole and its unapproved call be found: %+v", len(got), got)
	}
}

// Sampling keeps or drops whole conversations. Half a conversation would make
// the approval check fire on a call whose approval was simply not sampled.
func TestSamplingKeepsConversationsWhole(t *testing.T) {
	full := summarise(production.Process(stream(t), policy(), production.NewMonitor()))

	var got []production.Result
	srv := production.NewServer(policy())
	srv.Sample = 0.5
	srv.Emit = func(r production.Result) { got = append(got, r) }
	files, _ := filepath.Glob(filepath.Join("..", "otlp", "testdata", "*.pb"))
	sort.Strings(files)
	for _, f := range files {
		b, _ := os.ReadFile(f)
		req := httptest.NewRequest("POST", "/v1/traces", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/x-protobuf")
		srv.ServeHTTP(httptest.NewRecorder(), req)
	}
	srv.Flush(true)
	if len(got) == 0 || len(got) == 45 {
		t.Fatalf("sampled %d of 45; want some but not all", len(got))
	}
	for _, r := range got {
		want := full[r.Conversation.ID]
		// Drift incidents depend on which runs formed the baseline, so only the
		// action count and policy findings must match exactly.
		if !strings.HasSuffix(want, " actions="+itoa(len(r.Conversation.Actions))) {
			t.Errorf("%s: sampled with %d actions, full stream %q", r.Conversation.ID, len(r.Conversation.Actions), want)
		}
		for _, v := range r.Findings() {
			if !strings.Contains(want, v.Summary) {
				t.Errorf("%s: finding %q not in the unsampled result", r.Conversation.ID, v.Summary)
			}
		}
	}
}

// A trace whose root never arrives is still checked, once it goes quiet.
func TestAnOrphanedTraceIsCheckedNotLost(t *testing.T) {
	var got []production.Result
	srv := production.NewServer(policy())
	srv.Emit = func(r production.Result) { got = append(got, r) }
	srv.Accept([]otel.Span{{TraceID: "t", SpanID: "tool", ParentSpanID: "missing-root",
		Attributes: map[string]string{"gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": "fintech_change_limit"}}})
	srv.Flush(false)
	if len(got) != 0 {
		t.Fatal("a trace still inside its idle window must wait")
	}
	srv.Flush(true)
	if len(got) != 1 || len(got[0].Findings()) != 1 {
		t.Fatalf("got %+v; an orphaned trace must be checked, not dropped", got)
	}
}

func TestBadExportIsRejectedLoudly(t *testing.T) {
	srv := production.NewServer(policy())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/traces", strings.NewReader("\xff\xff\xff"))
	srv.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("status %d; an undecodable export must be a 400 the exporter will log, not a silent 200", rec.Code)
	}
}

func TestWebhookSendsSlackShapedText(t *testing.T) {
	var got map[string]any
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
	}))
	defer hook.Close()

	srv := production.NewServer(policy())
	srv.Alert = production.Webhook(hook.URL)
	srv.Accept([]otel.Span{
		{TraceID: "t", SpanID: "root", Attributes: map[string]string{"gen_ai.operation.name": "invoke_agent", "gen_ai.conversation.id": "c9"}},
		{TraceID: "t", SpanID: "x", ParentSpanID: "root", Attributes: map[string]string{"gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": "fintech_change_limit"}},
	})
	text, _ := got["text"].(string)
	if !strings.Contains(text, "c9") || !strings.Contains(text, "fintech_change_limit without request_human_approval") {
		t.Errorf("text = %q; a chat tool reads only this field", text)
	}
	if got["trackline"] == nil {
		t.Error("the full result should ride alongside for anything that wants it")
	}
}
