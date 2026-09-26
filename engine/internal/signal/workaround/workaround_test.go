package workaround

import (
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/session"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

type blocks []session.Block

func (b blocks) BlocksInSession(string) ([]session.Block, error) { return b, nil }

func TestMentionMatchesTheNameOnItsOwn(t *testing.T) {
	names := []string{".env"}
	for text, want := range map[string]bool{
		"echo X > .env":                     true,
		`open(".env", "w")`:                 true,
		"cat ./.env":                        true,
		"write it to .env.":                 true,
		"cp .env.example config.local.json": false,
		"x.env":                             false,
		"dotenv":                            false,
		"":                                  false,
	} {
		if _, got := mention(text, names); got != want {
			t.Errorf("mention(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestQuietWithoutEarlierBlocks(t *testing.T) {
	s := New(blocks(nil), "/w")
	in := signal.Input{Event: event.Event{SessionID: "s", Action: event.Action{Type: event.ActionRunCommand, Command: "echo X > .env"}}}
	if r := s.Check(in); r.Outcome == verdict.OutcomeFinding {
		t.Fatal("fired with nothing blocked")
	}
}

func TestOnlyFileBlocksAreFollowed(t *testing.T) {
	s := New(blocks{{Signal: "dependency-added", Target: "lodash"}}, "/w")
	in := signal.Input{Event: event.Event{SessionID: "s", Action: event.Action{Type: event.ActionRunCommand, Command: "echo lodash"}}}
	if r := s.Check(in); r.Outcome == verdict.OutcomeFinding {
		t.Fatal("followed a block that names no file")
	}
}

func TestAReadIsNotAWayRound(t *testing.T) {
	s := New(blocks{{Signal: "off-limits", Target: "/w/.env"}}, "/w")
	in := signal.Input{Event: event.Event{SessionID: "s", Action: event.Action{Type: event.ActionReadFile, Paths: []string{"/w/notes.md"}, Body: ".env"}}}
	if r := s.Check(in); r.Outcome != verdict.OutcomeNotApplicable {
		t.Fatalf("outcome = %s", r.Outcome)
	}
}
