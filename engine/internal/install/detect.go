package install

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Detect lists the agents that look installed on this machine, in a fixed
// order. An agent counts if its command is on the PATH or its settings folder
// exists in the home directory: people run Claude Code and Codex from an
// editor extension as often as from a terminal, and then the command may be
// missing while the folder is not.
//
// Found in real use: `trackline init` wired Claude Code only, the user worked
// in Codex, and nothing was recorded or said. Wiring what is installed, and
// saying what was wired, closes that.
// applications is where macOS keeps the Cursor app. A variable so a test does
// not depend on what the machine running it has installed.
var applications = "/Applications"

func Detect(home string, lookPath func(string) (string, error)) []Host {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	has := func(commands []string, paths []string) bool {
		for _, c := range commands {
			if _, err := lookPath(c); err == nil {
				return true
			}
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
		return false
	}
	var found []Host
	if has([]string{"claude"}, []string{filepath.Join(home, ".claude")}) {
		found = append(found, Claude)
	}
	if has([]string{"codex"}, []string{filepath.Join(home, ".codex")}) {
		found = append(found, Codex)
	}
	if has([]string{"cursor-agent", "cursor"}, []string{filepath.Join(home, ".cursor"), filepath.Join(applications, "Cursor.app")}) {
		found = append(found, Cursor)
	}
	return found
}
