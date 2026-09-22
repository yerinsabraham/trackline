// Package diffsize notices a change far larger than the request implied.
//
// "Fix this typo" producing fourteen edited files is drift even when every
// individual edit is defensible. The signal is not any one write, it is the
// accumulation, which means this check needs memory: it counts distinct files
// written under one user turn.
//
// It is heuristic and it fires late by design. A threshold low enough to catch
// small scope creep would fire on every legitimate refactor, and a refactor is
// exactly the work people hand an agent. So it stays quiet until the count is
// hard to explain, and it only ever warns.
package diffsize

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "diff-size"

// Thresholds, in distinct files written under one turn.
//
// Two numbers rather than one, because how large a change should be depends on
// how large a change was asked for. "Fix the typo" and "migrate the auth
// system" deserve different patience, and the request says which it is.
const (
	// SmallAskLimit applies when the request sounds narrow.
	SmallAskLimit = 5
	// DefaultLimit applies otherwise.
	DefaultLimit = 15
)

// Counter remembers which files were written under which turn.
//
// The hook is a short-lived process, so this state has to survive between
// invocations. The runner supplies an implementation backed by the session
// recording; tests supply one in memory.
type Counter interface {
	// FilesInTurn returns distinct paths already written under turnID,
	// excluding the current event.
	FilesInTurn(sessionID, turnID string) ([]string, error)
}

// Signal reports a change much larger than the request implied.
type Signal struct {
	Counter Counter
}

// New builds the check.
func New(c Counter) Signal { return Signal{Counter: c} }

func (Signal) Name() string { return Name }

// narrowAsk marks a request that plainly describes a small change.
var narrowAsk = []string{
	"typo", "rename", "one line", "a line", "small fix", "small change",
	"quick fix", "just fix", "only fix", "single", "minor", "tweak",
	"comment", "formatting", "whitespace", "import",
}

func (s Signal) Check(in signal.Input) verdict.Result {
	if !writes(in.Event.Action.Type) {
		return verdict.NotApplicable(Name, "this action does not write to a file")
	}
	if in.Event.TurnID == "" {
		return verdict.NotApplicable(Name, "the host did not group this action under a turn, so a change cannot be totalled")
	}
	if in.IntentUnavailable != "" {
		return verdict.CannotMeasure(Name, in.IntentUnavailable)
	}
	anchor, ok := in.Intent.Anchor()
	if !ok {
		return verdict.NotApplicable(Name, "nothing substantive has been asked yet")
	}
	if s.Counter == nil {
		return verdict.CannotMeasure(Name, "no record of earlier writes is available, so the size of this change is unknown")
	}

	prior, err := s.Counter.FilesInTurn(in.Event.SessionID, in.Event.TurnID)
	if err != nil {
		return verdict.CannotMeasure(Name,
			fmt.Sprintf("earlier writes in this turn could not be read: %v", err))
	}

	files := map[string]bool{}
	for _, p := range prior {
		files[p] = true
	}
	// An action whose files are invisible still counts as having happened, but
	// its paths cannot join the set. Say so rather than undercounting silently.
	if in.Event.Action.PathsUnknown {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"%s does not report which files it touches, so this change cannot be totalled accurately",
			describeTool(in.Event)))
	}
	for _, p := range in.Event.Action.Paths {
		files[p] = true
	}

	limit, why := limitFor(anchor)
	if len(files) <= limit {
		return verdict.Clean(Name)
	}

	names := make([]string, 0, len(files))
	for p := range files {
		names = append(names, p)
	}
	sort.Strings(names)

	shown := names
	if len(shown) > 6 {
		shown = shown[:6]
	}

	return verdict.Finding(Name, verdict.Verdict{
		Severity: verdict.SeverityWarn,
		Summary: fmt.Sprintf("%d files changed under one request%s",
			len(files), why),
		Evidence: []verdict.Evidence{
			{Kind: verdict.EvidenceTurn, Value: anchor.Text, Note: "what was asked"},
			{Kind: verdict.EvidenceCount, Value: fmt.Sprintf("%d files (limit %d)", len(files), limit),
				Note: "how much has changed"},
			{Kind: verdict.EvidenceFile, Value: strings.Join(shown, ", "), Note: "the files so far"},
		},
		Suggestion: "if the change genuinely needs this much, say so; otherwise stop and narrow it",
	})
}

func limitFor(anchor intent.Turn) (int, string) {
	low := strings.ToLower(anchor.Text)
	for _, phrase := range narrowAsk {
		if strings.Contains(low, phrase) {
			return SmallAskLimit, fmt.Sprintf(", and the request described a small change (%q)", phrase)
		}
	}
	return DefaultLimit, ""
}

func writes(t event.ActionType) bool {
	switch t {
	case event.ActionWriteFile, event.ActionEditFile, event.ActionDeleteFile:
		return true
	default:
		return false
	}
}

func describeTool(e event.Event) string {
	if e.Action.ToolName != "" {
		return e.Action.ToolName
	}
	return string(e.Action.Type)
}
