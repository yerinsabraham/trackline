package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectFindsAgentsByCommandOrSettingsFolder(t *testing.T) {
	home := t.TempDir()
	applications = t.TempDir()
	onPath := map[string]bool{}
	look := func(c string) (string, error) {
		if onPath[c] {
			return "/usr/local/bin/" + c, nil
		}
		return "", errors.New("not found")
	}

	if got := Detect(home, look); len(got) != 0 {
		t.Fatalf("nothing installed, found %v", got)
	}

	// Codex used only from an editor extension: no command, but its folder.
	os.Mkdir(filepath.Join(home, ".codex"), 0o755)
	onPath["claude"] = true
	onPath["cursor-agent"] = true
	want := []Host{Claude, Codex, Cursor}
	if got := Detect(home, look); !reflect.DeepEqual(got, want) {
		t.Errorf("found %v, want %v", got, want)
	}
}
