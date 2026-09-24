package payload_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// The fixtures are real sessions from Claude Code, Codex and Cursor, recorded in
// the Phase 4 evaluation, with local paths rewritten to /work/lab and /Users/dev.
// Each event keeps everything the hook saw: the raw payload, command text,
// written content, Cursor's user_email. The contract is that none of it leaves.

const root = "/work/lab"
const home = "/Users/dev"

type recorded struct {
	Event   event.Event      `json:"event"`
	Results []verdict.Result `json:"results"`
	Mode    string           `json:"mode"`
	Blocked bool             `json:"blocked"`
}

func fixtures(t *testing.T) map[string][]recorded {
	t.Helper()
	files, _ := filepath.Glob("testdata/*.json")
	if len(files) < 4 {
		t.Fatalf("found %d fixtures, want the four recorded sessions", len(files))
	}
	out := map[string][]recorded{}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var rs []recorded
		if err := json.Unmarshal(b, &rs); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = rs
	}
	return out
}

func build(t *testing.T, r recorded, share bool, i int) payload.Event {
	t.Helper()
	ev, err := payload.Build(r.Event, r.Results, payload.Options{
		Root: root, Home: home, ProjectID: "proj_test", ShareRequest: share,
		Mode: r.Mode, Blocked: r.Blocked, ID: fmt.Sprintf("ev-%d", i),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return ev
}

// ── the schema, read from the file both ends are held to ────────────────────

func schema(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "contract", "ingest-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// validate checks a value against the subset of JSON Schema the contract uses:
// type, const, enum, closed objects, required, arrays and string lengths. Small
// enough to read, so nobody has to trust a dependency to know what "valid" means.
func validate(root, s map[string]any, v any, at string) []string {
	if ref, ok := s["$ref"].(string); ok {
		def := root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		return validate(root, def, v, at)
	}
	var errs []string
	if c, ok := s["const"]; ok && fmt.Sprint(c) != fmt.Sprint(v) {
		errs = append(errs, fmt.Sprintf("%s: %v is not %v", at, v, c))
	}
	if e, ok := s["enum"].([]any); ok {
		found := false
		for _, x := range e {
			if x == v {
				found = true
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("%s: %v not in %v", at, v, e))
		}
	}
	switch x := v.(type) {
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for k, val := range x {
			ps, ok := props[k].(map[string]any)
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: field %q is not in the contract", at, k))
				continue
			}
			errs = append(errs, validate(root, ps, val, at+"."+k)...)
		}
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				if _, ok := x[r.(string)]; !ok {
					errs = append(errs, fmt.Sprintf("%s: missing %q", at, r))
				}
			}
		}
	case []any:
		if m, ok := s["maxItems"].(float64); ok && len(x) > int(m) {
			errs = append(errs, fmt.Sprintf("%s: %d items, limit %v", at, len(x), m))
		}
		items, _ := s["items"].(map[string]any)
		for i, it := range x {
			errs = append(errs, validate(root, items, it, fmt.Sprintf("%s[%d]", at, i))...)
		}
	case string:
		if m, ok := s["maxLength"].(float64); ok && len(x) > int(m) {
			errs = append(errs, fmt.Sprintf("%s: %d bytes, limit %v", at, len(x), m))
		}
	}
	return errs
}

func asJSON(t *testing.T, v any) any {
	t.Helper()
	b, _ := json.Marshal(v)
	var out any
	json.Unmarshal(b, &out)
	return out
}

// ── the contract ─────────────────────────────────────────────────────────────

func TestEveryRecordedEventBuildsToTheContract(t *testing.T) {
	s := schema(t)
	n := 0
	for name, rs := range fixtures(t) {
		var batch payload.Batch
		batch.V = payload.Version
		for i, r := range rs {
			batch.Events = append(batch.Events, build(t, r, true, i))
		}
		for _, e := range validate(s, s, asJSON(t, batch), name) {
			t.Error(e)
		}
		n += len(rs)
	}
	if n < 20 {
		t.Errorf("checked %d events; the fixtures should hold at least 20", n)
	}
}

// strings collects every string value in a JSON document.
func strs(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, i := range x {
			strs(i, out)
		}
	case map[string]any:
		for _, i := range x {
			strs(i, out)
		}
	}
}

// The heart of it: nothing the hook saw that is not on the "sent" list leaves.
func TestNothingOffTheListLeaves(t *testing.T) {
	for name, rs := range fixtures(t) {
		for i, r := range rs {
			out := asJSON(t, build(t, r, true, i))
			var sent []string
			strs(out, &sent)
			all := strings.Join(sent, "\n")

			// Command text. Checked on substantial fragments, so a command
			// that happens to be a single common word is not a false alarm.
			if cmd := r.Event.Action.Command; len(cmd) >= 12 && strings.Contains(all, cmd) {
				t.Errorf("%s[%d]: command text left the machine: %.60q", name, i, cmd)
			}
			// Written content.
			if body := r.Event.Action.Body; len(body) >= 20 && strings.Contains(all, body[:20]) {
				t.Errorf("%s[%d]: file content left the machine", name, i)
			}
			if body := r.Event.Action.PriorBody; len(body) >= 20 && strings.Contains(all, body[:20]) {
				t.Errorf("%s[%d]: prior file content left the machine", name, i)
			}
			// Evidence values quote the action; only summaries travel.
			for _, res := range r.Results {
				for _, v := range res.Verdicts {
					for _, ev := range v.Evidence {
						if len(ev.Value) >= 30 && strings.Contains(all, ev.Value) && ev.Value != v.Summary {
							t.Errorf("%s[%d]: an evidence value left the machine: %.60q", name, i, ev.Value)
						}
					}
				}
			}
			// Where the machine is, and whose it is.
			for _, bad := range []string{"/work/lab", home, "/tmp/", "/private/", "user@example.com", `"raw"`} {
				if strings.Contains(all, bad) {
					t.Errorf("%s[%d]: %q appears in the upload", name, i, bad)
				}
			}
		}
	}
}

func TestRequestTextOnlyWhenShared(t *testing.T) {
	for name, rs := range fixtures(t) {
		for i, r := range rs {
			if got := build(t, r, false, i).Request; got != "" {
				t.Errorf("%s[%d]: request sent without consent: %q", name, i, got)
			}
		}
	}
}

func at(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func TestPathsAreRelativeAndOutsidePathsAreDropped(t *testing.T) {
	ev := event.Event{
		Host: event.HostClaudeCode, SessionID: "s", At: at("2026-09-24T10:00:00Z"),
		Action: event.Action{Type: event.ActionWriteFile, Paths: []string{
			"/work/lab/src/app.ts", "src/rel.ts", "/etc/passwd", "/work/lab/../other/x", "/work/labmates/y",
		}},
	}
	out, err := payload.Build(ev, nil, payload.Options{Root: root, Home: home, ProjectID: "p", Mode: "warn", ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/app.ts", "src/rel.ts"}
	if strings.Join(out.Action.Paths, ",") != strings.Join(want, ",") {
		t.Errorf("paths = %v, want %v: anything outside the project is dropped, including a sibling that shares the name's prefix", out.Action.Paths, want)
	}
	if out.Project.Name != "lab" {
		t.Errorf("project name = %q; only the folder's name is sent", out.Project.Name)
	}
}

// The off-limits check writes absolute paths into its summary. A username is
// in there.
func TestFindingTextCarriesNoAbsolutePath(t *testing.T) {
	ev := event.Event{Host: event.HostCursor, SessionID: "s", At: at("2026-09-24T10:00:00Z"),
		Action: event.Action{Type: event.ActionWriteFile, Paths: []string{"/work/lab/.env"}}}
	res := []verdict.Result{{Signal: "off-limits", Outcome: verdict.OutcomeFinding, Verdicts: []verdict.Verdict{{
		Severity: verdict.SeverityBlock, Summary: "writing to a protected path: /work/lab/.env",
		Target: "/work/lab/.env", Suggestion: "see /Users/dev/notes.md",
		Evidence: []verdict.Evidence{{Kind: verdict.EvidenceFile, Value: "/work/lab/.env"}},
	}}}}
	out, _ := payload.Build(ev, res, payload.Options{Root: root, Home: home, ProjectID: "p", Mode: "auto", ID: "1"})
	f := out.Results[0].Findings[0]
	if f.Summary != "writing to a protected path: .env" || f.Target != ".env" || f.Suggestion != "see ~/notes.md" {
		t.Errorf("finding = %+v", f)
	}
}

func TestTokenShapesAreRedacted(t *testing.T) {
	secrets := []string{
		"ghp_" + strings.Repeat("a", 36),
		"github_pat_" + strings.Repeat("b", 40),
		"sk-ant-" + strings.Repeat("c", 40),
		"sk-proj-" + strings.Repeat("d", 40),
		"AKIA" + strings.Repeat("E", 16),
		"xoxb-" + strings.Repeat("1", 20),
		"AIza" + strings.Repeat("f", 35),
		"npm_" + strings.Repeat("g", 36),
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.abcdefghijkl",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----",
	}
	for _, s := range secrets {
		ev := event.Event{Host: event.HostClaudeCode, SessionID: "s", At: at("2026-09-24T10:00:00Z"),
			Request: "use this key " + s + " please", Action: event.Action{Type: event.ActionOther}}
		out, _ := payload.Build(ev, nil, payload.Options{Root: root, Home: home, ProjectID: "p", ShareRequest: true, Mode: "warn", ID: "1"})
		if strings.Contains(out.Request, s[:12]) || !strings.Contains(out.Request, "[redacted]") {
			t.Errorf("request = %q; %.12s… was not redacted", out.Request, s)
		}
	}
	// And it does not eat ordinary text.
	ev := event.Event{Host: event.HostClaudeCode, SessionID: "s", At: at("2026-09-24T10:00:00Z"),
		Request: "fix the sk- prefix check in auth.ts", Action: event.Action{Type: event.ActionOther}}
	out, _ := payload.Build(ev, nil, payload.Options{Root: root, Home: home, ProjectID: "p", ShareRequest: true, Mode: "warn", ID: "1"})
	if out.Request != "fix the sk- prefix check in auth.ts" {
		t.Errorf("ordinary request mangled: %q", out.Request)
	}
}

func TestLimitsHoldOnOversizedInput(t *testing.T) {
	long := strings.Repeat("é", 5000) // two bytes each: cutting must not split one
	var paths []string
	for i := 0; i < 200; i++ {
		paths = append(paths, fmt.Sprintf("/work/lab/f%d", i))
	}
	ev := event.Event{Host: event.HostCodex, SessionID: long, TurnID: long, At: at("2026-09-24T10:00:00Z"),
		Request: long, Action: event.Action{Type: event.ActionWriteFile, ToolName: long, Paths: paths}}
	var res []verdict.Result
	for i := 0; i < 30; i++ {
		res = append(res, verdict.Result{Signal: "scope", Outcome: verdict.OutcomeCannotMeasure, Reason: long})
	}
	out, _ := payload.Build(ev, res, payload.Options{Root: root, Home: home, ProjectID: long, ShareRequest: true, Mode: "warn", ID: long})
	s := schema(t)
	for _, e := range validate(s, s, asJSON(t, payload.Batch{V: 1, Events: []payload.Event{out}}), "oversized") {
		t.Error(e)
	}
	if !json.Valid([]byte(fmt.Sprintf("%q", out.Request))) || strings.ContainsRune(out.Request, '�') {
		t.Error("cutting split a character")
	}
}

func TestUnknownChecksAreLeftOutNotSent(t *testing.T) {
	ev := event.Event{Host: event.HostClaudeCode, SessionID: "s", At: at("2026-09-24T10:00:00Z"), Action: event.Action{Type: event.ActionOther}}
	out, _ := payload.Build(ev, []verdict.Result{{Signal: "some-future-check", Outcome: verdict.OutcomeClean}, {Signal: "scope", Outcome: verdict.OutcomeClean}},
		payload.Options{Root: root, Home: home, ProjectID: "p", Mode: "warn", ID: "1"})
	if len(out.Results) != 1 || out.Results[0].Check != "scope" {
		t.Errorf("results = %+v", out.Results)
	}
}

func TestProductionAndMCPEventsDoNotUpload(t *testing.T) {
	for _, h := range []event.Host{event.HostOTel, event.HostMCP} {
		if _, err := payload.Build(event.Event{Host: h}, nil, payload.Options{Root: root, ProjectID: "p", Mode: "warn", ID: "1"}); err == nil {
			t.Errorf("%s: production and MCP have their own contract, not this one", h)
		}
	}
}

// The Go types and the schema file must name the same fields. A field added to
// one without the other fails here.
func TestTypesMatchTheSchemaFile(t *testing.T) {
	s := schema(t)
	defs := s["$defs"].(map[string]any)
	names := func(props map[string]any) string {
		var k []string
		for n := range props {
			k = append(k, n)
		}
		sort.Strings(k)
		return strings.Join(k, ",")
	}
	ev := defs["event"].(map[string]any)
	cases := map[string]struct {
		schema map[string]any
		v      any
	}{
		"event":   {ev["properties"].(map[string]any), payload.Event{}},
		"project": {ev["properties"].(map[string]any)["project"].(map[string]any)["properties"].(map[string]any), payload.Project{}},
		"action":  {ev["properties"].(map[string]any)["action"].(map[string]any)["properties"].(map[string]any), payload.Action{}},
		"result":  {defs["result"].(map[string]any)["properties"].(map[string]any), payload.Result{}},
		"finding": {defs["result"].(map[string]any)["properties"].(map[string]any)["findings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), payload.Finding{}},
	}
	for name, c := range cases {
		if got, want := jsonFields(c.v), names(c.schema); got != want {
			t.Errorf("%s: Go type has [%s], schema has [%s]", name, got, want)
		}
	}
}

// jsonFields lists a type's JSON field names from its struct tags. Read from
// the type itself rather than from marshalled output: marshalling hides an
// empty omitempty field, and a first version of this test, which filled in the
// optional fields it knew about, missed a newly added one entirely.
func jsonFields(v any) string {
	t := reflect.TypeOf(v)
	var k []string
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			k = append(k, name)
		}
	}
	sort.Strings(k)
	return strings.Join(k, ",")
}
