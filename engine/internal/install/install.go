// Package install wires the hook into an agent's configuration.
//
// Two things make this harder than writing a file.
//
// A misconfigured hook does not warn. It simply never runs, and everything
// looks fine. Two attempts during Phase 0 silently did nothing because the
// config schema was guessed from the other agent's shape. So installing is not
// finished until the hook has been observed to fire.
//
// And the file being edited usually already exists and belongs to the user.
// Their settings are not ours to replace, so an install merges and leaves
// everything it did not add untouched.
package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Host is an agent trackline can attach to.
type Host string

const (
	Claude Host = "claude"
	Codex  Host = "codex"
)

// Result describes what an install did, so it can be reported honestly rather
// than as a uniform success.
type Result struct {
	Host    Host
	Path    string
	Created bool
	Changed bool
	// AlreadyPresent is true when a trackline hook was already configured.
	AlreadyPresent bool
}

// Plan says where a host keeps its hook configuration.
func Plan(host Host, root string) (string, error) {
	switch host {
	case Claude:
		return filepath.Join(root, ".claude", "settings.json"), nil
	case Codex:
		// Project-local Codex hooks require the .codex layer to be trusted, and
		// trust is hash-based, so the user must review it once via /hooks.
		return filepath.Join(root, ".codex", "hooks.json"), nil
	default:
		return "", fmt.Errorf("unknown host %q", host)
	}
}

// Install adds a PreToolUse hook running binary, without disturbing anything
// else in the file.
func Install(host Host, root, binary string) (Result, error) {
	path, err := Plan(host, root)
	if err != nil {
		return Result{}, err
	}
	res := Result{Host: host, Path: path}

	doc := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		res.Created = true
	case err != nil:
		return res, err
	default:
		if err := json.Unmarshal(b, &doc); err != nil {
			// Refusing is the only safe move. Overwriting a file we cannot
			// read would destroy settings the user cares about.
			return res, fmt.Errorf("%s exists but is not valid JSON; fix or move it first: %w", path, err)
		}
	}

	added, err := addHook(host, doc, binary)
	if err != nil {
		return res, err
	}
	if !added {
		res.AlreadyPresent = true
		return res, nil
	}
	res.Changed = true

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return res, err
	}
	return res, os.WriteFile(path, append(out, '\n'), 0o644)
}

// The two hosts use the same event name and the same block semantics, and
// differ only in where the hook list is rooted. Codex nests it under a "hooks"
// key; Claude Code puts "hooks" at the top level. Getting this wrong is silent,
// which is why Verify exists.
func addHook(host Host, doc map[string]any, binary string) (bool, error) {
	container := doc
	if host == Codex {
		inner, _ := doc["hooks"].(map[string]any)
		if inner == nil {
			inner = map[string]any{}
			doc["hooks"] = inner
		}
		container = inner
	} else {
		inner, _ := doc["hooks"].(map[string]any)
		if inner == nil {
			inner = map[string]any{}
			doc["hooks"] = inner
		}
		container = inner
	}

	entries, _ := container["PreToolUse"].([]any)
	for _, e := range entries {
		m, _ := e.(map[string]any)
		hooks, _ := m["hooks"].([]any)
		for _, h := range hooks {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); containsBinary(cmd, binary) {
				return false, nil
			}
		}
	}

	entry := map[string]any{
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": binary,
			"timeout": 15,
		}},
	}
	// Claude Code matches on tool name; Codex applies to all tools when no
	// matcher is given.
	if host == Claude {
		entry["matcher"] = "Write|Edit|MultiEdit|NotebookEdit|Bash"
	}

	container["PreToolUse"] = append(entries, entry)
	return true, nil
}

func containsBinary(cmd, binary string) bool {
	return cmd != "" && binary != "" && len(cmd) >= len(binary) &&
		(cmd == binary || indexOf(cmd, binary) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
