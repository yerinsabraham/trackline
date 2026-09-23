package mcp_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/mcp"
)

// exchange sends requests the way a client would, one JSON object per line,
// and returns the responses in order.
func exchange(t *testing.T, root string, lines ...string) []map[string]any {
	t.Helper()
	s := &mcp.Server{Root: root, Version: "test", Now: func() time.Time {
		return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	}}
	var out bytes.Buffer
	if err := s.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var r map[string]any
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("server wrote invalid JSON: %v\n%s", err, out.String())
		}
		resps = append(resps, r)
	}
	return resps
}

func call(id int, tool string, args string) string {
	return `{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"tools/call","params":{"name":"` +
		tool + `","arguments":` + args + `}}`
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func structured(t *testing.T, r map[string]any) map[string]any {
	t.Helper()
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", r)
	}
	sc, ok := res["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("no structuredContent: %v", res)
	}
	return sc
}

func text(t *testing.T, r map[string]any) string {
	t.Helper()
	res := r["result"].(map[string]any)
	return res["content"].([]any)[0].(map[string]any)["text"].(string)
}

func TestHandshakeAndToolList(t *testing.T) {
	resps := exchange(t, t.TempDir(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	// The notification must not be answered.
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2: %v", len(resps), resps)
	}
	init := resps[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v; a version the server knows must be echoed", init["protocolVersion"])
	}
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	if !names["check_action"] || !names["get_rules"] {
		t.Errorf("tools = %v", names)
	}
}

func TestUnknownProtocolVersionGetsTheNewest(t *testing.T) {
	resps := exchange(t, t.TempDir(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := resps[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-11-25" {
		t.Errorf("protocolVersion = %v", v)
	}
}

// The same protected write the hook blocks. Same checks, same config, same
// answer, reached through a question instead of an interception.
func TestProtectedWriteIsStopInAutoMode(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".trackline.json"), []byte(`{"modes":{"off-limits":"auto"}}`), 0o600)

	resps := exchange(t, root, call(1, "check_action", `{"action":"write_file","path":".env","content":"KEY=1"}`))
	sc := structured(t, resps[0])
	if sc["decision"] != "stop" {
		t.Errorf("decision = %v, want stop", sc["decision"])
	}
	msg := text(t, resps[0])
	if !strings.HasPrefix(msg, "Do not do this.") || !strings.Contains(msg, ".env") {
		t.Errorf("the model must be told plainly not to, and why:\n%s", msg)
	}
}

// Warn mode never stops anything, but the agent still hears the finding. That
// is the reminder working without interrupting.
func TestWarnModeProceedsButSaysWhatItNoticed(t *testing.T) {
	root := t.TempDir()
	resps := exchange(t, root, call(1, "check_action", `{"action":"write_file","path":".env","content":"KEY=1"}`))
	sc := structured(t, resps[0])
	if sc["decision"] != "proceed" {
		t.Errorf("decision = %v; warn mode must never stop", sc["decision"])
	}
	if f, _ := sc["findings"].([]any); len(f) == 0 {
		t.Error("the finding must still be reported")
	}
	if !strings.Contains(text(t, resps[0]), "trackline noted") {
		t.Errorf("text must carry the finding:\n%s", text(t, resps[0]))
	}
}

func TestAskModeIsAskNotStop(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".trackline.json"), []byte(`{"modes":{"off-limits":"ask"}}`), 0o600)
	resps := exchange(t, root, call(1, "check_action", `{"action":"write_file","path":".env"}`))
	if d := structured(t, resps[0])["decision"]; d != "ask" {
		t.Errorf("decision = %v, want ask", d)
	}
}

// An ordinary action is proceed. And proceed must never read as "every check
// passed" when some could not look.
func TestProceedNamesWhatWasNotChecked(t *testing.T) {
	root := t.TempDir()
	resps := exchange(t, root, call(1, "check_action", `{"action":"run_command","command":"./deploy.sh"}`))
	sc := structured(t, resps[0])
	if sc["decision"] != "proceed" {
		t.Errorf("decision = %v", sc["decision"])
	}
	if u, _ := sc["unmeasured"].([]any); len(u) == 0 {
		t.Error("an unreadable command must be reported as unchecked, not as clean")
	}
	if !strings.Contains(text(t, resps[0]), "Not checked") {
		t.Errorf("the model must be told what was not checked:\n%s", text(t, resps[0]))
	}
}

// A question is not an action. Recording it would have repetition and
// diff-size judge the agent for asking.
func TestAskingRecordsNothing(t *testing.T) {
	root := t.TempDir()
	exchange(t, root,
		call(1, "check_action", `{"action":"write_file","path":"a.txt","content":"x"}`),
		call(2, "check_action", `{"action":"write_file","path":"a.txt","content":"x"}`))
	for _, name := range []string{"events.jsonl", "turn.json"} {
		if _, err := os.Stat(filepath.Join(root, ".trackline", name)); err == nil {
			t.Errorf("%s was written; asking must not be recorded as doing", name)
		}
	}
}

func TestBadArgumentsAreAToolErrorTheModelCanRead(t *testing.T) {
	resps := exchange(t, t.TempDir(),
		call(1, "check_action", `{"action":"write_file"}`),
		call(2, "check_action", `{"action":"teleport"}`))
	for i, r := range resps {
		res, ok := r["result"].(map[string]any)
		if !ok || res["isError"] != true {
			t.Errorf("response %d: want a tool result with isError, so the model can fix its call: %v", i, r)
		}
	}
}

func TestProtocolErrors(t *testing.T) {
	resps := exchange(t, t.TempDir(),
		`not json`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	want := []float64{-32700, -32601, -32602}
	if len(resps) != 3 {
		t.Fatalf("got %d responses: %v", len(resps), resps)
	}
	for i, r := range resps {
		e, ok := r["error"].(map[string]any)
		if !ok || e["code"] != want[i] {
			t.Errorf("response %d = %v, want error %v", i, r, want[i])
		}
	}
}

func TestGetRulesReturnsTheProjectsRules(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Rules\n\n- Never commit secrets.\n"), 0o600)
	resps := exchange(t, root, call(1, "get_rules", `{}`))
	got := text(t, resps[0])
	for _, want := range []string{"Default mode: warn", ".env", "Never commit secrets"} {
		if !strings.Contains(got, want) {
			t.Errorf("get_rules is missing %q:\n%s", want, got)
		}
	}
}

// A server started in the wrong folder checks the wrong project and finds none
// of its config. Every answer names the folder, so that is visible.
func TestEveryAnswerNamesTheProject(t *testing.T) {
	root := t.TempDir()
	resps := exchange(t, root,
		call(1, "check_action", `{"action":"edit_file","path":"a.txt"}`),
		call(2, "get_rules", `{}`))
	for i, r := range resps {
		if !strings.Contains(text(t, r), root) {
			t.Errorf("response %d does not say which project it checked:\n%s", i, text(t, r))
		}
	}
	if structured(t, resps[0])["root"] != root {
		t.Error("structured result must carry the root")
	}
}

func TestUnlikelyRoots(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, r := range []string{"/", home} {
		if !mcp.UnlikelyRoot(r) {
			t.Errorf("%q should be flagged: no project lives there", r)
		}
	}
	if mcp.UnlikelyRoot(t.TempDir()) {
		t.Error("an ordinary directory must not be flagged")
	}
}
