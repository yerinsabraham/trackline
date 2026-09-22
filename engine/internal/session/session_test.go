package session_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/session"
)

func TestRoundTripPreservesEverythingAnAdapterProduced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.jsonl")
	r := session.Recorder{Path: path}

	in := []event.Event{
		{
			ID: "u1", SessionID: "s", TurnID: "t1",
			At:   time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC),
			Host: event.HostClaudeCode, Phase: event.PhasePreTool,
			CWD: "/work", TranscriptPath: "/tmp/t.jsonl",
			Action: event.Action{
				Type: event.ActionWriteFile, ToolName: "Write",
				Paths: []string{"/work/a.ts"},
			},
		},
		{
			ID: "u2", SessionID: "s", TurnID: "t1",
			Host: event.HostCodex, Phase: event.PhasePreTool,
			Action: event.Action{
				Type: event.ActionRunCommand, ToolName: "shell",
				Command: "rm -rf build", PathsUnknown: true,
			},
		},
	}
	for _, ev := range in {
		if err := r.Append(ev); err != nil {
			t.Fatal(err)
		}
	}

	got, err := session.Replay(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(in) {
		t.Fatalf("got %d events, want %d", len(got), len(in))
	}
	if got[0].Action.Paths[0] != "/work/a.ts" || got[0].TurnID != "t1" {
		t.Errorf("first event lost detail: %+v", got[0])
	}
	// The field that must survive above all others: not knowing.
	if !got[1].Action.PathsUnknown {
		t.Error("PathsUnknown did not survive the round trip; a replay would report the command as touching nothing")
	}
	if got[1].Action.Command != "rm -rf build" {
		t.Errorf("command = %q", got[1].Action.Command)
	}
}

// A recording is the evidence a finding rests on. Silently skipping a corrupt
// line would mean replaying a different session from the one that happened.
func TestCorruptRecordingIsReportedNotSkipped(t *testing.T) {
	_, err := session.ReplayFrom(strings.NewReader(
		`{"id":"u1","host":"claude-code"}` + "\n" + `{not json` + "\n"))
	if err == nil {
		t.Fatal("a corrupt recording must error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should name the line: %v", err)
	}
}

func TestEmptyRecordingIsNotAnError(t *testing.T) {
	got, err := session.ReplayFrom(strings.NewReader(""))
	if err != nil {
		t.Fatalf("an empty recording is a real state, not a failure: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d events", len(got))
	}
}
