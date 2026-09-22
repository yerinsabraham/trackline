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
