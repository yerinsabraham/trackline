package agent_test

import (
	"bufio"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/relay/agent"
)

// The testdata is real output from the installed agents, run with the
// command lines Command builds, paths replaced with /work/app.
func parse(t *testing.T, name, file string) []agent.Event {
	t.Helper()
	f, err := os.Open("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := agent.NewParser(name, "/work/app")
	var out []agent.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		out = append(out, p.Line(sc.Bytes())...)
	}
	return out
}

func TestClaudeWritingAFile(t *testing.T) {
	got := parse(t, "claude", "claude-write.jsonl")
	want := []agent.Event{
		{Kind: "session", Text: "62df7713-920a-4e4d-a277-d47449944319"},
		{Kind: "tool", Tool: "Write", Text: "hello.txt"},
		{Kind: "say", Text: "done"},
		{Kind: "done", Text: "done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

// A command nobody could approve is refused, and the phone is told which.
func TestClaudeRefusedACommand(t *testing.T) {
	got := parse(t, "claude", "claude-denied.jsonl")
	i := slices.IndexFunc(got, func(e agent.Event) bool { return e.Kind == "denied" })
	if i < 0 || got[i].Tool != "Bash" || !strings.Contains(got[i].Text, "print(6*7)") {
		t.Fatalf("the refusal was not reported: %+v", got)
	}
	if got[len(got)-1].Kind != "done" {
		t.Fatalf("no final answer: %+v", got)
	}
}

func TestCodexWritingAFile(t *testing.T) {
	got := parse(t, "codex", "codex-write.jsonl")
	want := []agent.Event{
		{Kind: "session", Text: "01a0d91e-d8b2-70a1-b1b3-cffb3a417b85"},
		{Kind: "say", Text: "I’m creating the requested file now."},
		{Kind: "tool", Tool: "edit", Text: "codex.txt"},
		{Kind: "say", Text: "done"},
		{Kind: "done", Text: "done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParsersIgnoreWhatTheyCannotRead(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		p := agent.NewParser(name, "/work/app")
		for _, line := range []string{"", "not json", `{"type":"something new"}`, `[1,2]`} {
			if ev := p.Line([]byte(line)); len(ev) != 0 {
				t.Errorf("%s: %q gave %+v", name, line, ev)
			}
		}
	}
}

// No command line may carry a way around the agent's permissions, and none
// carries the prompt: it goes on stdin, where "--dangerously-skip-permissions"
// is only text.
func TestCommandsStaySafe(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		argv, err := agent.Command(name, "/bin/"+name, "/work/app", "")
		if err != nil {
			t.Fatal(err)
		}
		line := strings.Join(argv, " ")
		for _, bad := range []string{"dangerously", "bypass", "danger-full-access", "--yolo", "--force", "--restricted", "user,"} {
			if strings.Contains(line, bad) {
				t.Errorf("%s: %s", name, line)
			}
		}
	}
	claude, _ := agent.Command("claude", "claude", "/work/app", "")
	for _, need := range []string{"acceptEdits", "--permission-prompts none", "--setting-sources project,local"} {
		if !strings.Contains(strings.Join(claude, " "), need) {
			t.Errorf("claude is missing %s", need)
		}
	}
	codex, _ := agent.Command("codex", "codex", "/work/app", "")
	if !strings.Contains(strings.Join(codex, " "), "--sandbox workspace-write") || codex[len(codex)-1] != "-" {
		t.Errorf("codex: %v", codex)
	}
	if _, err := agent.Command("cursor", "cursor-agent", "/work/app", ""); err != agent.ErrNotYet {
		t.Errorf("cursor: %v", err)
	}
}

func TestLongTextIsClippedOnACharacter(t *testing.T) {
	long := strings.Repeat("é", agent.MaxText)
	p := agent.NewParser("codex", "/work/app")
	ev := p.Line([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"` + long + `"}}`))
	if len(ev) != 1 || len(ev[0].Text) > agent.MaxText+len("…") || !strings.HasSuffix(ev[0].Text, "…") {
		t.Fatalf("len %d", len(ev[0].Text))
	}
	if !utf8Valid(ev[0].Text) {
		t.Fatal("cut through a character")
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "�") == s }

// Continuing a session keeps every safeguard of a new one: the same
// permissions for Claude Code, the same sandbox for Codex.
func TestResumingKeepsTheSafeguards(t *testing.T) {
	claude, _ := agent.Command("claude", "claude", "/work/app", "sess-1")
	line := strings.Join(claude, " ")
	for _, need := range []string{"--resume sess-1", "acceptEdits", "--permission-prompts none", "--setting-sources project,local"} {
		if !strings.Contains(line, need) {
			t.Errorf("claude resume is missing %s: %s", need, line)
		}
	}
	codex, _ := agent.Command("codex", "codex", "/work/app", "sess-1")
	line = strings.Join(codex, " ")
	for _, need := range []string{"exec resume", `sandbox_mode="workspace-write"`, `approval_policy="never"`, "sess-1 -"} {
		if !strings.Contains(line, need) {
			t.Errorf("codex resume is missing %s: %s", need, line)
		}
	}
	if strings.Contains(line, "danger") || strings.Contains(line, "bypass") {
		t.Errorf("codex resume: %s", line)
	}
}
