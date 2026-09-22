// Package session records what an agent did and replays it.
//
// Record and replay is how this project tests anything. The CI side already
// works this way, for the same reasons: a replay is deterministic, costs
// nothing, needs no API key, and a change in behaviour shows up as a readable
// diff rather than as a number nobody re-ran.
//
// A recording is JSONL, one normalised event per line, appended as the session
// runs. Append-only matters: a hook writing a recording must never lose earlier
// events because a later one crashed.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yerinsabraham/trackline/engine/internal/event"
)

// Recorder appends normalised events to a file.
type Recorder struct {
	Path string
}

// Append writes one event. It opens and closes per call rather than holding a
// handle, because the writer is a short-lived hook process, and a half-written
// line from a killed process is worse than the cost of an open.
func (r Recorder) Append(ev event.Event) error {
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	// One write, so a line is either fully present or absent. A partial line
	// would make the whole recording unreplayable from that point.
	_, err = f.Write(append(b, '\n'))
	return err
}

// Replay reads a recording back.
//
// A line that will not parse is reported rather than skipped: a recording is
// the evidence a finding rests on, and quietly dropping part of it would mean
// replaying a different session from the one that happened.
func Replay(path string) ([]event.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReplayFrom(f)
}

// ReplayFrom reads a recording from any reader.
func ReplayFrom(rd io.Reader) ([]event.Event, error) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	var out []event.Event
	line := 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("recording line %d is not a valid event: %w", line, err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Counting what has already happened in the current turn.
//
// The hook is a short-lived process: it starts, judges one action and exits. A
// check that needs to know what happened earlier in the same request therefore
// cannot hold state in memory.
//
// The obvious implementation reads the session recording back on every call,
// and it was measured and rejected. Latency grew with the recording: 10.8ms
// empty, 15.1ms at 300 events, 18.7ms at 600. Every call pays for the whole
// session, so a long session costs O(n²) in total and gets slower the longer
// someone works. That is the wrong shape for something that runs before every
// action.
//
// So the hook keeps a small state file holding only the current turn, reset
// whenever the turn changes. Reads are bounded by how much an agent does in one
// request, which is tens of actions, not the whole session. The full recording
// is still written, and is still what `inspect` replays; nothing on the hot
// path reads it.

// TurnState is what has happened so far under one user turn.
type TurnState struct {
	SessionID string        `json:"sessionId"`
	TurnID    string        `json:"turnId"`
	Events    []event.Event `json:"events"`
}

// TurnCounter reads and updates the per-turn state file.
type TurnCounter struct {
	Path string

	loaded bool
	state  TurnState
	err    error
}

// load reads the state file once per process.
//
// A missing file, or one for a different turn, both mean the same thing:
// nothing has happened in this turn yet.
func (c *TurnCounter) load(sessionID, turnID string) (TurnState, error) {
	if !c.loaded {
		c.loaded = true
		b, err := os.ReadFile(c.Path)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			c.err = err
		default:
			if err := json.Unmarshal(b, &c.state); err != nil {
				// A corrupt state file is not worth failing an action over, and
				// it is self-healing: the next write replaces it.
				c.state = TurnState{}
			}
		}
	}
	if c.err != nil {
		return TurnState{}, c.err
	}
	if c.state.SessionID != sessionID || c.state.TurnID != turnID {
		return TurnState{}, nil
	}
	return c.state, nil
}

// FilesInTurn returns the distinct paths written under this turn.
func (c *TurnCounter) FilesInTurn(sessionID, turnID string) ([]string, error) {
	st, err := c.load(sessionID, turnID)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []string
	for _, ev := range st.Events {
		switch ev.Action.Type {
		case event.ActionWriteFile, event.ActionEditFile, event.ActionDeleteFile:
		default:
			continue
		}
		for _, p := range ev.Action.Paths {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// ActionsInTurn returns every action recorded under this turn, in order.
//
// Where FilesInTurn answers "how much has changed", this answers "what has been
// tried", which is what a repetition check needs.
func (c *TurnCounter) ActionsInTurn(sessionID, turnID string) ([]event.Event, error) {
	st, err := c.load(sessionID, turnID)
	if err != nil {
		return nil, err
	}
	return st.Events, nil
}

// Append adds an event to the turn state, starting a new turn when the id
// changes.
//
// Bodies are dropped. They are the largest part of an event and no check that
// reads this state needs them, so keeping them would make every read cost more
// for nothing.
func (c *TurnCounter) Append(ev event.Event) error {
	if c.Path == "" {
		return nil
	}
	st, err := c.load(ev.SessionID, ev.TurnID)
	if err != nil {
		st = TurnState{}
	}
	st.SessionID, st.TurnID = ev.SessionID, ev.TurnID

	ev.Action.Body = ""
	ev.Action.PriorBody = ""
	ev.Raw = nil
	st.Events = append(st.Events, ev)

	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	// Written whole each time rather than appended, so the file is always a
	// complete state and never a half-updated one.
	return os.WriteFile(c.Path, b, 0o600)
}
