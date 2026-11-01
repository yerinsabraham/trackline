package intent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACompactionSummaryIsNotWhatThePersonAsked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	// The order Claude Code really writes: the message, then the summary, in
	// the same prompt and marked as human.
	body := `{"type":"user","turnOrigin":"human","origin":{"kind":"human"},"promptId":"p1","timestamp":"2026-11-01T14:32:20Z","message":{"content":"make sure everything aligns before you push"}}
{"type":"user","turnOrigin":"human","isCompactSummary":true,"isVisibleInTranscriptOnly":true,"promptId":"p1","timestamp":"2026-11-01T14:18:44Z","message":{"content":"This session is being continued from a previous conversation"}}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var in Intent
	if err := (&Reader{Path: path}).Read(&in); err != nil {
		t.Fatal(err)
	}
	got, ok := in.Latest()
	if !ok || got.Text != "make sure everything aligns before you push" {
		t.Fatalf("the summary replaced the request: %q", got.Text)
	}
	if a, ok := in.Anchor(); !ok || strings.Contains(a.Text, "continued from") {
		t.Fatalf("checks must not measure against the summary: %q", a.Text)
	}
}

func TestLastSaid(t *testing.T) {
	human := `{"type":"user","turnOrigin":"human","message":{"content":"do it"}}`
	said := `{"type":"assistant","message":{"content":[{"type":"text","text":"Checking the menu."},{"type":"tool_use","name":"Bash"}]}}`
	tool := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`
	result := `{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}`
	codexAsked := `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"text","text":"do it"}]}}}`
	codexSaid := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Looking now."}]}}`
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"nothing":           {nil, ""},
		"nothing yet":       {[]string{human}, ""},
		"before a step":     {[]string{human, said}, "Checking the menu."},
		"steps after it":    {[]string{human, said, result, tool, result}, "Checking the menu."},
		"a new message":     {[]string{human, said, human}, ""},
		"a cut first line":  {[]string{`"text":"half a line`, said}, "Checking the menu."},
		"codex":             {[]string{codexAsked, codexSaid}, "Looking now."},
		"codex new message": {[]string{codexSaid, codexAsked}, ""},
	} {
		if got := LastSaid([]byte(strings.Join(c.lines, "\n"))); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestATailReadsOnlyTheEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	old := `{"type":"user","turnOrigin":"human","promptId":"old","message":{"content":"an old request"}}` + "\n"
	recent := `{"type":"user","turnOrigin":"human","promptId":"new","message":{"content":"the latest request"}}` + "\n"
	padding := strings.Repeat(`{"type":"assistant","message":{"content":"working"}}`+"\n", 400)
	if err := os.WriteFile(path, []byte(old+padding+recent), 0o600); err != nil {
		t.Fatal(err)
	}
	var in Intent
	r := &Reader{Path: path, Tail: 1024}
	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if !r.Cut() || len(in.Turns) != 1 || in.Turns[0].ID != "new" {
		t.Fatalf("cut %v, turns %+v", r.Cut(), in.Turns)
	}
}

// Quick must see every line a request can arrive on, on every host, or a check
// would quietly measure against an older one.
func TestQuickFindsTheSameRequestsOnEveryHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	body := strings.Join([]string{
		`{"type":"user","turnOrigin":"human","origin":{"kind":"human"},"promptId":"1","message":{"content":"interactive"}}`,
		`{"type":"user","turnOrigin":"sdk","promptId":"2","message":{"content":"headless"}}`,
		`{"type":"user","origin":{"kind":"human"},"promptId":"3","message":{"content":"origin only"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"noise"}]}}`,
		`{"timestamp":"2026-11-01T12:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"c1"}}`,
		`{"timestamp":"2026-11-01T12:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"codex one"}}`,
		`{"timestamp":"2026-11-01T12:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"c2"}}`,
		`{"timestamp":"2026-11-01T12:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"text","text":"codex two"}]}}}`,
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>cursor</user_query>"}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	read := func(quick bool) []Turn {
		var in Intent
		if err := (&Reader{Path: path, Quick: quick}).Read(&in); err != nil {
			t.Fatal(err)
		}
		return in.Turns
	}
	full, quick := read(false), read(true)
	if len(full) != 6 || len(quick) != len(full) {
		t.Fatalf("full %d, quick %d", len(full), len(quick))
	}
	for i := range full {
		if full[i].Text != quick[i].Text || full[i].ID != quick[i].ID {
			t.Errorf("turn %d: full %+v, quick %+v", i, full[i], quick[i])
		}
	}
}
