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
	"strings"
)

// Host is an agent trackline can attach to.
type Host string

const (
	Claude Host = "claude"
	Codex  Host = "codex"
	Cursor Host = "cursor"
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
	case Cursor:
		// Project-level, so it only sees what the agent does once it is inside
		// the project. In the captured session the agent started in the home
		// directory and searched it before moving in; none of that fired.
		return filepath.Join(root, ".cursor", "hooks.json"), nil
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

// The two hosts use the same event names and the same block semantics, and
// differ only in where the hook list is rooted. Codex nests it under a "hooks"
// key; Claude Code puts "hooks" at the top level. Getting this wrong is silent,
// which is why Verify exists.
//
// Two events: PreToolUse checks each action, and Stop hands over the agent's
// final message for the turn, which is what the dashboard shows as its reply.
// Each is added only if missing, so a project set up before Stop existed gains
// it on the next init without a duplicate PreToolUse.
func addHook(host Host, doc map[string]any, binary string) (bool, error) {
	if host == Cursor {
		return addCursorHook(doc, binary)
	}
	container, _ := doc["hooks"].(map[string]any)
	if container == nil {
		container = map[string]any{}
		doc["hooks"] = container
	}

	changed := false
	for _, ev := range []string{"PreToolUse", "Stop"} {
		entries, _ := container[ev].([]any)
		entries, held, moved := keepOneOurs(entries, binary, true)
		if moved {
			container[ev] = entries
			changed = true
		}
		if held {
			continue
		}
		entry := map[string]any{
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": binary,
				"timeout": 15,
			}},
		}
		// Claude Code matches on tool name; Codex applies to all tools when
		// no matcher is given. Stop has no tool to match.
		if host == Claude && ev == "PreToolUse" {
			entry["matcher"] = "Write|Edit|MultiEdit|NotebookEdit|Bash"
		}
		container[ev] = append(entries, entry)
		changed = true
	}
	return changed, nil
}

// ours recognises trackline's hook by its file name, wherever it lives. A
// check for this exact path missed an older install at another path, so an
// upgrade added a second hook beside the first and every action was judged
// twice (found in real use, 2026-09-25).
func ours(cmd string) bool {
	f := strings.Fields(cmd)
	return len(f) > 0 && strings.TrimSuffix(filepath.Base(f[0]), ".exe") == "trackline-hook"
}

// keepOneOurs leaves exactly one trackline hook in an event's list, pointing
// at binary: an older one is moved to the current path, and any extra is
// removed. grouped is true for Claude Code and Codex, whose entries are
// matcher groups holding hooks; Cursor's are the hooks themselves. It reports
// whether one is present afterwards, and whether anything changed.
func keepOneOurs(entries []any, binary string, grouped bool) (out []any, held, changed bool) {
	for _, e := range entries {
		m, _ := e.(map[string]any)
		if !grouped {
			if cmd, _ := m["command"].(string); ours(cmd) {
				if held {
					changed = true
					continue
				}
				held = true
				if cmd != binary {
					m["command"] = binary
					changed = true
				}
			}
			out = append(out, e)
			continue
		}
		hooks, _ := m["hooks"].([]any)
		var kept []any
		for _, h := range hooks {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); ours(cmd) {
				if held {
					changed = true
					continue
				}
				held = true
				if cmd != binary {
					hm["command"] = binary
					changed = true
				}
			}
			kept = append(kept, h)
		}
		if len(kept) == 0 {
			changed = true
			continue
		}
		m["hooks"] = kept
		out = append(out, e)
	}
	return out, held, changed
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

// addCursorHook wires preToolUse, and afterAgentResponse for the reply. It covers Shell, Write, Read,
// Delete and MCP tools in one event and runs in Cursor's cloud agents, where
// beforeMCPExecution does not. Shell commands also fire beforeShellExecution;
// hooking both would judge every command twice.
//
// Cursor's schema differs from the other two: a required version field, and
// each event holds a flat list of hooks rather than matcher groups.
func addCursorHook(doc map[string]any, binary string) (bool, error) {
	if v, ok := doc["version"]; ok {
		if n, isNum := v.(float64); !isNum || n != 1 {
			// A version this code has never seen may mean a schema it would
			// write wrongly, and a wrong hooks file fails silently.
			return false, fmt.Errorf("unsupported Cursor hooks version %v; expected 1", v)
		}
	} else {
		doc["version"] = 1
	}

	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		doc["hooks"] = hooks
	}

	// afterAgentResponse carries the agent's reply text. It is not fired by
	// the cursor-agent CLI (checked 2026-09-25, where preToolUse is), so the
	// reply arrives from the editor only; unverified there until seen.
	//
	// failClosed is deliberately left at its default, false. If trackline
	// itself breaks, the user keeps working: a watcher that jams the editor
	// gets uninstalled, and then it watches nothing.
	changed := false
	for _, ev := range []string{"preToolUse", "afterAgentResponse"} {
		entries, _ := hooks[ev].([]any)
		entries, held, moved := keepOneOurs(entries, binary, false)
		if moved {
			hooks[ev] = entries
			changed = true
		}
		if held {
			continue
		}
		hooks[ev] = append(entries, map[string]any{
			"command": binary,
			"timeout": 15,
		})
		changed = true
	}
	return changed, nil
}
