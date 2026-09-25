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
