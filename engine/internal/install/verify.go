package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Verifying an install, in two steps that answer different questions.
//
// SelfTest answers "does the binary work", by running it against a synthetic
// event and checking it behaves. That can be done immediately.
//
// Observed answers "has the host ever actually invoked it", which is the
// question that matters and which no amount of inspecting configuration can
// answer. A hook can be perfectly configured and never run, and that failure is
// silent. The only proof is a trace left behind by a real invocation.

// SelfTest runs the binary against a synthetic protected write and checks it
// responds correctly.
//
// It uses a temporary project so the user's own findings log is not polluted
// with a test entry that never happened.
func SelfTest(binary string) error {
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("the hook binary is not at %s: %w", binary, err)
	}

	dir, err := os.MkdirTemp("", "trackline-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Auto mode, so a correct binary must block and say why.
	if err := os.WriteFile(filepath.Join(dir, ".trackline.json"),
		[]byte(`{"modes":{"off-limits":"auto"}}`), 0o600); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "selftest",
		"prompt_id":       "selftest",
		"tool_name":       "Write",
		"cwd":             dir,
		"tool_input":      map[string]any{"file_path": ".env", "content": "x"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "-root", dir)
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()

	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return fmt.Errorf("the hook binary would not run: %w", err)
	}

	if code != 2 {
		return fmt.Errorf("the hook did not block a write to .env (exit %d); it is not working", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("BLOCKED")) {
		return fmt.Errorf("the hook blocked but said nothing useful: %q", stderr.String())
	}
	return nil
}

// Observed reports whether the hook has ever actually been invoked in root.
//
// This is the only honest confirmation that the wiring works. A configuration
// file that looks right proves nothing: a misconfigured hook is silent, and
// two attempts during development silently did nothing because the schema had
// been guessed from the wrong host.
func Observed(root string) (bool, time.Time, error) {
	info, err := os.Stat(filepath.Join(root, ".trackline", "findings.jsonl"))
	if os.IsNotExist(err) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	return true, info.ModTime(), nil
}
