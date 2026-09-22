// Package intent models what the user actually asked for.
//
// This is the hard half of the product, and Phase 0 is why it looks like this.
//
// The obvious design is "the latest user message is the task". Measured against
// a real session, that fails badly: the most recent human turn in the session
// that produced these findings was the single word "proceed". It carries no
// scope, no files and no constraints. A check comparing actions against
// "proceed" would either fire on everything or on nothing.
//
// Intent accumulates. It is rebuilt from many turns rather than read from one,
// and it has to survive turns that carry no content at all.
//
// This package deliberately holds no policy. It records every human turn and
// marks which ones carry substance; deciding what to compare an action against
// belongs to the checks, not to the model they read.
package intent

import (
	"strings"
	"time"
)

// Turn is one thing a human said.
type Turn struct {
	// ID is the host's prompt or turn identifier, so actions can be grouped
	// under the request that caused them.
	ID   string    `json:"id,omitempty"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`

	// Substantive is false for a turn that only tells the agent to keep going.
	// See IsSubstantive: it is a documented heuristic, not a fact about the
	// text, and it is kept as a flag rather than used to discard the turn.
	Substantive bool `json:"substantive"`
}

// Intent is every human turn in a session, oldest first.
type Intent struct {
	SessionID string `json:"sessionId,omitempty"`
	Turns     []Turn `json:"turns,omitempty"`
}

// continuations are turns whose entire content is "keep going".
//
// The list is short on purpose. A word only counts when it is the whole message:
// "proceed" is a continuation, "proceed with the refactor but skip the tests"
// is an instruction. Anything not listed is treated as substantive, because the
// cost of ignoring a real instruction is higher than the cost of treating a
// filler word as one.
var continuations = map[string]bool{
	"proceed": true, "go": true, "go ahead": true, "continue": true,
	"yes": true, "y": true, "ok": true, "okay": true, "sure": true,
	"do it": true, "keep going": true, "next": true, "please continue": true,
	"carry on": true, "sounds good": true, "lgtm": true, "approved": true,
}

// IsSubstantive reports whether a turn carries an instruction of its own.
//
// Heuristic, and the accuracy of it directly drives the false-alarm rate, so it
// is exported and tested rather than buried. When in doubt it answers true: an
// intent that is too broad produces a quiet tool, while an intent that wrongly
// discards a real instruction produces a tool that fires on correct work.
func IsSubstantive(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.Trim(t, ".!,;:")
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		return false
	}
	return !continuations[t]
}

// Add appends a turn, classifying it.
func (in *Intent) Add(id string, at time.Time, text string) {
	in.Turns = append(in.Turns, Turn{
		ID:          id,
		At:          at,
		Text:        text,
		Substantive: IsSubstantive(text),
	})
}

// Anchor is the most recent turn that carried an instruction.
//
// This is what a scope check should compare against, rather than the literal
// last message. When the user says "proceed", the anchor stays on whatever they
// actually asked for.
//
// ok is false when nothing substantive has been said yet, which is a real state
// early in a session and must not be papered over with an empty turn.
func (in Intent) Anchor() (Turn, bool) {
	for i := len(in.Turns) - 1; i >= 0; i-- {
		if in.Turns[i].Substantive {
			return in.Turns[i], true
		}
	}
	return Turn{}, false
}

// Latest is the most recent turn of any kind, substantive or not.
func (in Intent) Latest() (Turn, bool) {
	if len(in.Turns) == 0 {
		return Turn{}, false
	}
	return in.Turns[len(in.Turns)-1], true
}

// Recent returns up to n of the most recent substantive turns, oldest first.
//
// Intent often spans several turns: a task, then a correction, then a
// constraint. A check that reads only the anchor misses the correction.
func (in Intent) Recent(n int) []Turn {
	if n <= 0 {
		return nil
	}
	var out []Turn
	for i := len(in.Turns) - 1; i >= 0 && len(out) < n; i-- {
		if in.Turns[i].Substantive {
			out = append(out, in.Turns[i])
		}
	}
	// Collected newest first; hand them back in the order they were said.
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// TurnByID finds the turn an action was performed under.
func (in Intent) TurnByID(id string) (Turn, bool) {
	for _, t := range in.Turns {
		if t.ID != "" && t.ID == id {
			return t, true
		}
	}
	return Turn{}, false
}
