// Package workaround notices an agent reaching something trackline stopped by
// another route.
//
// A blocked agent usually corrects itself (docs/experiments/01). Sometimes it
// avoids the forbidden file and routes the same change through something
// else: a script that writes it, a build step, a package.json hook, a config
// that points at it. Each of those actions is allowed on its face, and the
// check that blocked the first attempt sees nothing, because the file itself
// is never named in a path. What gives it away is the target turning up in
// what the agent writes or runs next.
//
// So this check remembers what was blocked in the session and reads later
// writes and commands for it. It reports the blocked call and the new route
// side by side, so the person sees the corrective path and not only the
// refusal (asked for by a reader). It warns: an agent explaining in a comment
// why it did not touch .env mentions .env too.
//
// Files only, for now: a blocked file is named the same way wherever it
// reappears. A blocked package would need its own matching.
package workaround

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/session"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "workaround"

// Blocks is what was stopped earlier in a session.
type Blocks interface {
	BlocksInSession(sessionID string) ([]session.Block, error)
}

// Signal reports a blocked target reached another way.
type Signal struct {
	Blocks Blocks
	// Root is the project, for naming a blocked file the way the agent would.
	Root string
}

// New builds the check.
func New(b Blocks, root string) Signal { return Signal{Blocks: b, Root: root} }

func (Signal) Name() string { return Name }

// watched are the checks whose blocks name a file.
var watched = map[string]bool{"off-limits": true}

func (s Signal) Check(in signal.Input) verdict.Result {
	a := in.Event.Action
	switch a.Type {
	case event.ActionWriteFile, event.ActionEditFile, event.ActionRunCommand:
	default:
		return verdict.NotApplicable(Name, "this action neither writes nor runs anything")
	}
	if s.Blocks == nil || in.Event.SessionID == "" {
		return verdict.NotApplicable(Name, "there is no record of earlier blocks in this session")
	}
	blocks, err := s.Blocks.BlocksInSession(in.Event.SessionID)
	if err != nil {
		return verdict.CannotMeasure(Name, fmt.Sprintf("earlier blocks could not be read: %v", err))
	}

	text := a.Command + "\n" + a.Body
	for _, b := range blocks {
		if !watched[b.Signal] {
			continue
		}
		names := s.names(b.Target)
		// Naming the blocked file directly is the same attempt again, and
		// the check that blocked it will answer it.
		if touches(a.Paths, names) {
			continue
		}
		line, ok := mention(text, names)
		if !ok {
			continue
		}
		route := describe(in.Event)
		return verdict.Finding(Name, verdict.Verdict{
			Severity: verdict.SeverityWarn,
			Target:   b.Target,
			Summary:  fmt.Sprintf("after %s was blocked, %s names it", names[0], route),
			Evidence: []verdict.Evidence{
				{Kind: verdict.EvidenceRule, Value: b.What, Note: "what was blocked (" + b.Signal + ")"},
				{Kind: evidenceFor(a.Type), Value: route, Note: "the route taken after"},
				{Kind: verdict.EvidenceCode, Value: line, Note: "where it names the blocked file"},
			},
			Suggestion: "check this reaches " + names[0] + " only in a way the person would agree to; a block is not a detour sign",
		})
	}
	return verdict.Clean(Name)
}

// names are the ways a blocked file can be written: relative to the project,
// then by its name alone.
func (s Signal) names(target string) []string {
	rel := target
	if s.Root != "" && filepath.IsAbs(target) {
		if r, err := filepath.Rel(s.Root, target); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	rel = filepath.ToSlash(rel)
	out := []string{rel}
	if base := filepath.Base(rel); base != rel {
		out = append(out, base)
	}
	return out
}

func touches(paths, names []string) bool {
	for _, p := range paths {
		p = filepath.ToSlash(p)
		for _, n := range names {
			if p == n || strings.HasSuffix(p, "/"+n) {
				return true
			}
		}
	}
	return false
}

// mention finds a name standing on its own in text, and returns its line:
// ".env" in `cp x .env` or `open(".env")`, but not in ".env.example".
func mention(text string, names []string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		for _, n := range names {
			if len(n) < 3 {
				continue
			}
			for i := 0; ; {
				j := strings.Index(line[i:], n)
				if j < 0 {
					break
				}
				at := i + j
				end := at + len(n)
				if (at == 0 || !nameChar(line[at-1])) && (end == len(line) || !nameChar(line[end]) || line[end] == '.' && (end+1 == len(line) || !nameChar(line[end+1]))) {
					l := strings.TrimSpace(line)
					if len(l) > 160 {
						l = l[:160] + "…"
					}
					return l, true
				}
				i = at + 1
			}
		}
	}
	return "", false
}

func nameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.'
}

func describe(e event.Event) string {
	a := e.Action
	if a.Type == event.ActionRunCommand {
		c := a.Command
		if len(c) > 120 {
			c = c[:120] + "…"
		}
		return "the command `" + c + "`"
	}
	if len(a.Paths) > 0 {
		return "a write to " + filepath.Base(a.Paths[0])
	}
	return "a write"
}

func evidenceFor(t event.ActionType) verdict.EvidenceKind {
	if t == event.ActionRunCommand {
		return verdict.EvidenceCommand
	}
	return verdict.EvidenceFile
}
