package install_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/install"
)

const bin = "/opt/trackline/bin/trackline-hook"

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
	return doc
}

// Cursor's schema is not Claude's: a required version, and a flat list of
// hooks per event. Writing Claude's shape here would fail silently.
func TestCursorFreshInstallUsesCursorsSchema(t *testing.T) {
	root := t.TempDir()
	res, err := install.Install(install.Cursor, root, bin)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.Path != filepath.Join(root, ".cursor", "hooks.json") {
		t.Errorf("result = %+v", res)
	}
	doc := readJSON(t, res.Path)
	if doc["version"] != float64(1) {
		t.Errorf("version = %v, want 1", doc["version"])
	}
	hooks := doc["hooks"].(map[string]any)
	pre := hooks["preToolUse"].([]any)
	if len(pre) != 1 {
		t.Fatalf("preToolUse = %v", pre)
	}
	h := pre[0].(map[string]any)
	if h["command"] != bin {
		t.Errorf("command = %v", h["command"])
	}
	if _, nested := h["hooks"]; nested {
		t.Error("Cursor hooks are flat; a nested Claude-style group would never run")
	}
	if fc, set := h["failClosed"]; set && fc == true {
		t.Error("failClosed must stay off: a broken watcher must not stop the user working")
	}
	// Shell commands fire both events. Hooking both judges each one twice.
	if _, ok := hooks["beforeShellExecution"]; ok {
		t.Error("only preToolUse should be wired")
	}
}

func TestCursorInstallLeavesExistingHooksAlone(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".cursor", "hooks.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"version":1,"hooks":{
		"preToolUse":[{"command":"./audit.sh"}],
		"afterFileEdit":[{"command":"./format.sh"}]}}`), 0o644)

	if _, err := install.Install(install.Cursor, root, bin); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, path)["hooks"].(map[string]any)
	if pre := hooks["preToolUse"].([]any); len(pre) != 2 || pre[0].(map[string]any)["command"] != "./audit.sh" {
		t.Errorf("the user's own preToolUse hook must survive, first: %v", pre)
	}
	if _, ok := hooks["afterFileEdit"]; !ok {
		t.Error("an unrelated event the user configured was dropped")
	}
}

func TestCursorInstallTwiceChangesNothing(t *testing.T) {
	root := t.TempDir()
	install.Install(install.Cursor, root, bin)
	res, err := install.Install(install.Cursor, root, bin)
	if err != nil {
		t.Fatal(err)
	}
	if !res.AlreadyPresent || res.Changed {
		t.Errorf("second install = %+v, want already present and unchanged", res)
	}
}

func TestCursorUnknownVersionIsRefused(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".cursor", "hooks.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	original := []byte(`{"version":2,"hooks":{}}`)
	os.WriteFile(path, original, 0o644)

	if _, err := install.Install(install.Cursor, root, bin); err == nil {
		t.Fatal("a schema version this code has never seen must be refused, not guessed at")
	}
	if b, _ := os.ReadFile(path); string(b) != string(original) {
		t.Error("a refused install must not touch the file")
	}
}
