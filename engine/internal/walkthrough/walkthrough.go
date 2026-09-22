// Package walkthrough turns a recorded session into something a person can read.
//
// It exists because a findings log is evidence, not a story. The log says what
// each check concluded about each action; a walkthrough says what happened: the
// agent was asked for one thing, did several, and here is the moment one of
// them stopped matching the request.
//
// The same structure drives the terminal viewer and the page on the web, so
// there is one description of a session rather than two that drift apart.
package walkthrough

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Session is a whole recorded session, ready to render.
type Session struct {
	// Task is what the human asked for, as they wrote it.
	Task string `json:"task"`
	// Mode is how trackline was configured while this ran.
	Mode string `json:"mode"`
	// Steps are the actions the agent took, in order.
	Steps []Step `json:"steps"`
	// Summary is the counts, so a reader knows the shape before reading.
	Summary Summary `json:"summary"`
}

// Step is one action and what was concluded about it.
type Step struct {
	N       int      `json:"n"`
	Tool    string   `json:"tool"`
	Paths   []string `json:"paths,omitempty"`
	Command string   `json:"command,omitempty"`
	// Outcome is the worst thing any check said: clean, unseen, finding,
	// blocked.
	Outcome  string    `json:"outcome"`
	Findings []Finding `json:"findings,omitempty"`
	// Unseen names checks that could not judge this action at all. Kept
	// because an action nobody could look at is not an action that was fine.
	Unseen []string `json:"unseen,omitempty"`
}

// Finding is one thing a check objected to.
type Finding struct {
	Check      string     `json:"check"`
	Severity   string     `json:"severity"`
	Summary    string     `json:"summary"`
	Evidence   []Evidence `json:"evidence"`
	Suggestion string     `json:"suggestion,omitempty"`
}

// Evidence is one checkable fact behind a finding.
type Evidence struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

// Summary is the shape of a session at a glance.
type Summary struct {
	Actions  int `json:"actions"`
	Findings int `json:"findings"`
	Blocked  int `json:"blocked"`
	Unseen   int `json:"unseen"`
}

// logEntry mirrors what the hook writes, kept here rather than shared so the
// on-disk format can change without silently altering a published page.
type logEntry struct {
	At      time.Time        `json:"at"`
	Tool    string           `json:"tool"`
	Paths   []string         `json:"paths"`
	Mode    string           `json:"mode"`
	Blocked bool             `json:"blocked"`
	Results []verdict.Result `json:"results"`
}

// Build reads a findings log into a session.
//
// root is stripped from paths: an absolute path from someone's machine is
// noise in a terminal and a small privacy leak on a web page.
func Build(logPath, root, task string) (Session, error) {
	b, err := os.ReadFile(logPath)
	if err != nil {
		return Session{}, err
	}

	s := Session{Task: task}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e logEntry
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		s.Steps = append(s.Steps, step(e, root, len(s.Steps)+1))
		if s.Mode == "" {
			s.Mode = e.Mode
		}
	}

	for _, st := range s.Steps {
		s.Summary.Actions++
		s.Summary.Findings += len(st.Findings)
		s.Summary.Unseen += len(st.Unseen)
		if st.Outcome == "blocked" {
			s.Summary.Blocked++
		}
	}
	return s, nil
}

func step(e logEntry, root string, n int) Step {
	st := Step{N: n, Tool: e.Tool}
	for _, p := range e.Paths {
		st.Paths = append(st.Paths, relative(root, p))
	}

	for _, r := range e.Results {
		switch r.Outcome {
		case verdict.OutcomeCannotMeasure:
			st.Unseen = append(st.Unseen, r.Signal)
		case verdict.OutcomeFinding:
			for _, v := range r.Verdicts {
				f := Finding{
					Check: r.Signal, Severity: string(v.Severity),
					Summary: v.Summary, Suggestion: v.Suggestion,
				}
				for _, ev := range v.Evidence {
					f.Evidence = append(f.Evidence, Evidence{
						Kind: string(ev.Kind), Value: relative(root, ev.Value), Note: ev.Note,
					})
				}
				st.Findings = append(st.Findings, f)
			}
		}
	}

	switch {
	case e.Blocked:
		st.Outcome = "blocked"
	case len(st.Findings) > 0:
		st.Outcome = "finding"
	case len(st.Unseen) > 0 && len(st.Findings) == 0:
		// Reported as its own state rather than folded into clean. A check that
		// could not see an action did not find it acceptable.
		st.Outcome = "unseen"
	default:
		st.Outcome = "clean"
	}
	return st
}

func relative(root, p string) string {
	if root == "" || !strings.HasPrefix(p, root) {
		return p
	}
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}
