package install

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// eventHost is how each agent names itself in the findings log.
var eventHost = map[Host]string{Claude: "claude-code", Codex: "codex", Cursor: "cursor"}

// Silent lists the agents whose configuration in root carries the trackline
// hook, yet which have never recorded an action there.
//
// Codex runs a hook only once it is trusted, and asks at most once; in our
// test, a prompt left unanswered was never shown again, and the hook stayed
// off without a word. A configured hook that never fires looks exactly like a
// working one with nothing to report. This is the check that tells them apart.
func Silent(root string) []Host {
	seen := map[string]bool{}
	if f, err := os.Open(filepath.Join(root, ".trackline", "findings.jsonl")); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
		for sc.Scan() {
			var e struct {
				Host string `json:"host"`
			}
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				seen[e.Host] = true
			}
		}
		f.Close()
	}
	var silent []Host
	for _, h := range []Host{Claude, Codex, Cursor} {
		path, _ := Plan(h, root)
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "trackline-hook") {
			continue
		}
		if !seen[eventHost[h]] {
			silent = append(silent, h)
		}
	}
	return silent
}

// Wired lists the agents whose configuration in root carries the trackline
// hook, by the name the contract uses for them.
func Wired(root string) []string {
	var out []string
	for _, h := range []Host{Claude, Codex, Cursor} {
		path, _ := Plan(h, root)
		if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), "trackline-hook") {
			out = append(out, eventHost[h])
		}
	}
	return out
}

// Unsilence says, in one line, what makes a silent agent start reporting.
func Unsilence(h Host) string {
	switch h {
	case Codex:
		return "Codex runs a hook only once you trust it. In the Codex panel: Settings, Hooks, From Projects, then Trust. In a terminal: run codex here and type /hooks."
	case Cursor:
		return "Send a message in Cursor's agent chat in this project."
	default:
		return "Send a message in Claude Code in this project."
	}
}
