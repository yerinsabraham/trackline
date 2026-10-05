package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
)

// output runs fn with stdout captured. Tests have no terminal, which is the
// case that matters here: an agent running the command.
func output(t *testing.T, fn func()) string {
	t.Helper()
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var b bytes.Buffer
	io.Copy(&b, r)
	return b.String()
}

var noAsk = func(string, bool) bool { panic("asked without a terminal") }

// Run by an agent, sharing is never switched on by default: measured, the
// install guide's agent ran connect and the old default of yes opted the
// person in without asking them.
func TestAnAgentNeverOptsThePersonIn(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	var share bool
	out := output(t, func() { share, _ = shareChoice("/new", false, false, false, noAsk) })
	if share {
		t.Fatal("shared with nobody having said so")
	}
	if !strings.Contains(out, "trackline connect --share") {
		t.Fatalf("does not say how to turn it on: %q", out)
	}
	// --yes accepts defaults; it is not an answer to the privacy question.
	if share, _ = shareChoice("/new", false, false, true, noAsk); share {
		t.Fatal("--yes shared")
	}
}

// The answer already given for the other projects carries over.
func TestTheEarlierChoiceCarriesOver(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	account.ConnectProject("/a", true)
	var share bool
	out := output(t, func() { share, _ = shareChoice("/new", false, false, false, noAsk) })
	if !share || !strings.Contains(out, "as for your other projects") {
		t.Fatalf("share=%v out=%q", share, out)
	}
}

func TestSharingFlagsAnswerIt(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	account.ConnectProject("/a", true)
	if share, _ := shareChoice("/new", true, false, false, noAsk); share {
		t.Fatal("--no-requests shared")
	}
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	if share, _ := shareChoice("/new", false, true, false, noAsk); !share {
		t.Fatal("--share did not share")
	}
	if _, err := shareChoice("/new", true, true, false, noAsk); err == nil {
		t.Fatal("contradictory flags were accepted")
	}
}

// init ends by saying how to put the project on the dashboard, naming the
// account when this computer is already linked to one.
func TestInitPointsAtConnect(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	out := output(t, func() { offerConnect(t.TempDir()) })
	if !strings.Contains(out, "trackline connect") {
		t.Fatalf("unlinked: %q", out)
	}
	account.SaveCredentials(account.Credentials{API: "http://x", Token: "t", Email: "ada@example.com"})
	out = output(t, func() { offerConnect(t.TempDir()) })
	if !strings.Contains(out, "linked to ada@example.com") || !strings.Contains(out, "trackline connect") {
		t.Fatalf("linked: %q", out)
	}
}
