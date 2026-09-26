// Package outside refuses, in a session started from the phone, a write to a
// file outside the project.
//
// A remote task runs in one project, and each agent is confined to it by its
// own means: Claude Code's permission mode, Codex's sandbox, Cursor's sandbox
// for shell commands. Cursor's file edit tool is not confined. Measured: a
// model mistyped the project's path and Cursor's editor wrote a file into a
// folder it created elsewhere on the laptop. With nobody at the keyboard that
// cannot be left to the agent, so trackline refuses it for every agent,
// whatever the host does.
//
// Only in a phone session. At the desk, writing outside the project is the
// person's own business, and scope already says when it drifts.
package outside

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "outside-project"

// Signal refuses writes outside Root.
type Signal struct{ Root string }

// New builds the check for the project at root.
func New(root string) Signal { return Signal{Root: root} }

func (Signal) Name() string { return Name }

func (s Signal) Check(in signal.Input) verdict.Result {
	a := in.Event.Action
	switch a.Type {
	case event.ActionWriteFile, event.ActionEditFile, event.ActionDeleteFile:
	default:
		return verdict.NotApplicable(Name, "this action does not write a file")
	}
	if s.Root == "" {
		return verdict.CannotMeasure(Name, "the project's folder is not known")
	}
	if a.PathsUnknown {
		return verdict.CannotMeasure(Name, "the files this action writes are not known")
	}
	root := resolve(s.Root)
	for _, p := range a.Paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.Root, p)
		}
		if within(root, resolve(p)) {
			continue
		}
		return verdict.Finding(Name, verdict.Verdict{
			Severity: verdict.SeverityBlock,
			Target:   p,
			Summary:  fmt.Sprintf("writing outside the project: %s", p),
			Evidence: []verdict.Evidence{
				{Kind: verdict.EvidenceFile, Value: p, Note: "the file being written"},
				{Kind: verdict.EvidenceRule, Value: s.Root, Note: "the project this task runs in"},
			},
			Suggestion: "write inside " + s.Root + "; check the path for a typo",
		})
	}
	return verdict.Clean(Name)
}

// resolve follows symlinks as far as the path exists, so /tmp and
// /private/tmp, or a project reached through a link, compare as the same
// place, and a new file is judged by the folder it would land in.
func resolve(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{r}, rest...)...)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
		cur = parent
	}
}

func within(root, p string) bool {
	r, err := filepath.Rel(root, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator))
}
