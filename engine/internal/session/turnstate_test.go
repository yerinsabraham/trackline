package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/session"
)

func ev(sess, turn, path string) event.Event {
	return event.Event{
		SessionID: sess, TurnID: turn,
		Action: event.Action{Type: event.ActionWriteFile, ToolName: "Write",
			Paths: []string{path}, Body: "some content"},
	}
}

func TestTurnStateRemembersWithinATurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn.json")

	for _, p := range []string{"/w/a.ts", "/w/b.ts", "/w/a.ts"} {
		c := &session.TurnCounter{Path: path} // a fresh process each time
		if err := c.Append(ev("s", "t1", p)); err != nil {
			t.Fatal(err)
		}
	}

	c := &session.TurnCounter{Path: path}
	files, err := c.FilesInTurn("s", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("files = %v, want the two distinct paths", files)
	}
	acts, _ := c.ActionsInTurn("s", "t1")
	if len(acts) != 3 {
		t.Errorf("actions = %d, want all three attempts", len(acts))
	}
}

// The whole reason this file exists instead of replaying the recording.
func TestTurnStateResetsWhenTheTurnChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn.json")
	for i, p := range []string{"/w/a.ts", "/w/b.ts", "/w/c.ts"} {
		c := &session.TurnCounter{Path: path}
		turn := "t1"
		if i == 2 {
			turn = "t2" // the user asked something new
		}
		c.Append(ev("s", turn, p))
	}

	c := &session.TurnCounter{Path: path}
	files, _ := c.FilesInTurn("s", "t2")
	if len(files) != 1 || files[0] != "/w/c.ts" {
		t.Errorf("files = %v; a new request starts a new count", files)
	}

	// The old turn is gone, which is correct: only the current one is kept.
	old, _ := c.FilesInTurn("s", "t1")
	if len(old) != 0 {
		t.Errorf("old turn = %v, want empty", old)
	}
}

// Latency must not grow with the length of the session. Reading the whole
// recording made every call pay for the whole session, which measured 10.8ms
// empty and 18.7ms at 600 events.
func TestTurnStateDoesNotGrowWithTheSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn.json")

	// A long session: many turns, a few actions each.
	for turn := 0; turn < 200; turn++ {
		for i := 0; i < 3; i++ {
			c := &session.TurnCounter{Path: path}
			c.Append(ev("s", string(rune('a'+turn%26))+string(rune('0'+turn/26)), "/w/f.ts"))
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Three trimmed events, not six hundred.
	if info.Size() > 4096 {
		t.Errorf("turn state is %d bytes after 600 actions; it must hold only the current turn", info.Size())
	}
}

// Bodies are the largest part of an event and no check reading this state needs
// them.
func TestTurnStateDropsBodies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn.json")
	e := ev("s", "t1", "/w/a.ts")
	e.Action.Body = "a very large file body"
	e.Action.PriorBody = "the previous contents"

	(&session.TurnCounter{Path: path}).Append(e)

	b, _ := os.ReadFile(path)
	for _, s := range []string{"a very large file body", "the previous contents"} {
		if string(b) != "" && contains(string(b), s) {
			t.Errorf("turn state kept a body: %q", s)
		}
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

// A corrupt state file must not fail an action. It is self-healing.
func TestCorruptTurnStateIsSurvivable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn.json")
	os.WriteFile(path, []byte("{not json"), 0o600)

	c := &session.TurnCounter{Path: path}
	if _, err := c.FilesInTurn("s", "t1"); err != nil {
		t.Errorf("a corrupt state file must not fail the pass: %v", err)
	}
	if err := c.Append(ev("s", "t1", "/w/a.ts")); err != nil {
		t.Fatal(err)
	}
	files, _ := (&session.TurnCounter{Path: path}).FilesInTurn("s", "t1")
	if len(files) != 1 {
		t.Errorf("files = %v; the next write should repair the file", files)
	}
}
