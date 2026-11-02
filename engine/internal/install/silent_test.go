package install

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSilentNamesAgentsSetUpButNeverSeen(t *testing.T) {
	root := t.TempDir()
	for _, h := range []Host{Claude, Codex} {
		if _, err := Install(h, root, "/opt/trackline/trackline-hook"); err != nil {
			t.Fatal(err)
		}
	}
	if got := Silent(root); !reflect.DeepEqual(got, []Host{Claude, Codex}) {
		t.Fatalf("before anything ran: %v", got)
	}

	// Claude Code has reported; Codex, untrusted, has not.
	os.MkdirAll(filepath.Join(root, ".trackline"), 0o755)
	os.WriteFile(filepath.Join(root, ".trackline", "findings.jsonl"), []byte(`{"host":"claude-code","tool":"Write"}`+"\n"), 0o644)
	if got := Silent(root); !reflect.DeepEqual(got, []Host{Codex}) {
		t.Errorf("after Claude Code ran: %v, want only Codex", got)
	}
}

// A project set up by an older release must say so, and one just set up must
// not: a notice that is always on gets ignored like one that never is.
func TestBehindNamesOnlyAgentsAnInitWouldChange(t *testing.T) {
	root := t.TempDir()
	const hook = "/opt/trackline/bin/trackline-hook"
	for _, h := range []Host{Claude, Codex} {
		if _, err := Install(h, root, hook); err != nil {
			t.Fatal(err)
		}
	}
	if got := Behind(root, hook); len(got) != 0 {
		t.Fatalf("just installed, Behind = %v", got)
	}

	// 0.13.0 wrote only PreToolUse and Stop.
	old := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"` + hook + `"}]}],"Stop":[{"hooks":[{"type":"command","command":"` + hook + `"}]}]}}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Behind(root, hook); !reflect.DeepEqual(got, []string{"claude-code"}) {
		t.Errorf("older Claude setup, Behind = %v", got)
	}
	// A trackline installed somewhere else since.
	if got := Behind(root, "/usr/local/lib/trackline-hook"); !reflect.DeepEqual(got, []string{"claude-code", "codex"}) {
		t.Errorf("moved install, Behind = %v", got)
	}
	if got := Behind(t.TempDir(), hook); got != nil {
		t.Errorf("nothing set up, Behind = %v", got)
	}
	if got := Behind(root, ""); got != nil {
		t.Errorf("no hook binary known, Behind = %v", got)
	}
}
