// Package override records decisions a human has already made.
//
// This is what makes ask-first workable rather than a loop. Neither host lets a
// PreToolUse hook prompt a person directly: "ask" is not a permitted decision,
// only allow or deny. So asking means denying with an explanation and letting
// the agent relay the question. Without somewhere to record the answer, the
// next attempt is denied identically and the user is asked the same thing
// forever.
//
// An override is deliberately narrow. Approving one write to one path does not
// approve the next, and nothing here can switch a check off wholesale: that is
// what configuration is for, and it should be a decision someone makes in the
// open rather than by clicking through a prompt.
package override

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Grant is one decision a human made.
type Grant struct {
	// Signal is the check being overridden.
	Signal string `json:"signal"`
	// Target is what was approved: a path, a package, a command.
	Target string `json:"target"`
	// Scope limits how far the approval reaches.
	Scope Scope `json:"scope"`
	// Session and Turn bound a one-off approval.
	Session string `json:"session,omitempty"`
	Turn    string `json:"turn,omitempty"`
	// Reason is what the human said, kept so the decision can be reviewed.
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

// Scope is how long an approval lasts.
type Scope string

const (
	// ScopeOnce lasts for the request it was granted in. The safest, and the
	// default: a person approving one thing has approved one thing.
	ScopeOnce Scope = "once"
	// ScopeProject lasts until removed. For a decision that is genuinely
	// permanent, like a path the project has decided is fine to write.
	ScopeProject Scope = "project"
)

// Store holds grants for a project.
type Store struct {
	Path string

	loaded bool
	grants []Grant
}

// NewStore points at the usual location under a project root.
func NewStore(root string) *Store {
	return &Store{Path: filepath.Join(root, ".trackline", "overrides.json")}
}

func (s *Store) load() []Grant {
	if s.loaded {
		return s.grants
	}
	s.loaded = true
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil
	}
	// A corrupt file means no overrides, which fails toward asking rather than
	// toward silently allowing. That is the right direction for this file.
	_ = json.Unmarshal(b, &s.grants)
	return s.grants
}

// Allows reports whether a human has already approved this, and why.
func (s *Store) Allows(signal, target, session, turn string) (Grant, bool) {
	for _, g := range s.load() {
		if g.Signal != signal || !matches(g.Target, target) {
			continue
		}
		switch g.Scope {
		case ScopeProject:
			return g, true
		case ScopeOnce:
			// A one-off holds only within the request it was granted in.
			if g.Session == session && g.Turn == turn {
				return g, true
			}
		}
	}
	return Grant{}, false
}

// matches compares an approved target with the one now in question.
//
// Exact, apart from a trailing "/**" meaning everything beneath a directory.
// Prefix matching by default would quietly approve far more than a person
// meant: approving "src/auth" would approve "src/authority" too.
func matches(approved, actual string) bool {
	if approved == actual {
		return true
	}
	if strings.HasSuffix(approved, "/**") {
		dir := strings.TrimSuffix(approved, "/**")
		return strings.HasPrefix(actual, dir+"/")
	}
	return false
}

// Add records a decision.
func (s *Store) Add(g Grant) error {
	g.At = time.Now().UTC()
	grants := append(s.load(), g)
	s.grants = grants

	b, err := json.MarshalIndent(grants, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.Path, append(b, '\n'), 0o600)
}

// List returns every grant, for review.
func (s *Store) List() []Grant { return s.load() }

// Remove drops grants matching a signal and target, and reports how many went.
func (s *Store) Remove(signal, target string) (int, error) {
	var kept []Grant
	removed := 0
	for _, g := range s.load() {
		if g.Signal == signal && (target == "" || g.Target == target) {
			removed++
			continue
		}
		kept = append(kept, g)
	}
	if removed == 0 {
		return 0, nil
	}
	s.grants = kept

	b, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return 0, err
	}
	return removed, os.WriteFile(s.Path, append(b, '\n'), 0o600)
}

// Describe renders a grant for a human reading the list back.
func (g Grant) Describe() string {
	scope := "for this request only"
	if g.Scope == ScopeProject {
		scope = "for this project"
	}
	out := fmt.Sprintf("%s on %s, %s", g.Signal, g.Target, scope)
	if g.Reason != "" {
		out += " — " + g.Reason
	}
	return out
}
