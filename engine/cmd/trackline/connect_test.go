package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

// Without a terminal an earlier no is repeated, but an earlier yes is not:
// consent to upload messages is per project, and a bare connect in a work
// repo must not start uploading because a personal project does.
func TestOnlyAnEarlierNoCarriesOverWithoutATerminal(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	account.ConnectProject("/a", false)
	var share bool
	out := output(t, func() { share, _ = shareChoice("/new", false, false, false, noAsk) })
	if share || !strings.Contains(out, "as for your other projects") {
		t.Fatalf("earlier no: share=%v out=%q", share, out)
	}

	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	account.ConnectProject("/a", true)
	out = output(t, func() { share, _ = shareChoice("/new", false, false, false, noAsk) })
	if share {
		t.Fatal("an earlier yes opted a new project in without the person")
	}
	if !strings.Contains(out, "trackline connect --share") {
		t.Fatalf("does not say how to match: %q", out)
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

// /dev/null is a character device, and what an agent's commands often get as
// input; it must not count as a person at a terminal.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	if isTerminal(f.Fd()) {
		t.Fatal("/dev/null taken for a terminal")
	}
}

// fakeLink is an account that approves a computer once told to.
type fakeLink struct {
	approved bool
	starts   int
}

func (f *fakeLink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	send := func(v any) { json.NewEncoder(w).Encode(v) }
	switch {
	case strings.HasSuffix(r.URL.Path, "/devices/code"):
		f.starts++
		send(map[string]any{"deviceCode": "dc1", "userCode": "ABCD-EFGH", "verificationUriComplete": "http://site/connect?code=ABCD-EFGH", "expiresIn": 600, "interval": 1})
	case strings.HasSuffix(r.URL.Path, "/devices/token"):
		if f.approved {
			send(map[string]any{"status": "approved", "token": "tl_dev_x", "device": map[string]string{"id": "dev1", "name": "mac"}})
		} else {
			send(map[string]any{"status": "pending"})
		}
	case strings.HasSuffix(r.URL.Path, "/devices/me"):
		send(map[string]any{"user": map[string]string{"name": "Ada", "email": "ada@example.com"}})
	default:
		send(map[string]any{})
	}
}

// Run by an agent on a computer not linked yet: connect prints the link and
// stops, instead of waiting longer than an agent's command may run. Run
// again after the person approves, it finishes with the same code.
func TestAnApprovalOutlivesTheAgentsCommand(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	f := &fakeLink{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	root := t.TempDir()
	o := connectOptions{api: srv.URL, noRequests: true, noBrowser: true}

	var err error
	out := output(t, func() { err = connect(root, o) })
	if err != nil || !strings.Contains(out, "ABCD-EFGH") || !strings.Contains(out, "run the same trackline connect again") {
		t.Fatalf("err=%v out=%q", err, out)
	}
	if _, _, ok := account.ProjectFor(root); ok {
		t.Fatal("connected before the computer was approved")
	}

	f.approved = true
	out = output(t, func() { err = connect(root, o) })
	if err != nil || !strings.Contains(out, "is connected to Ada") {
		t.Fatalf("err=%v out=%q", err, out)
	}
	if f.starts != 1 {
		t.Fatalf("asked the person to approve %d codes, want 1", f.starts)
	}
	if _, _, ok := account.ProjectFor(root); !ok {
		t.Fatal("not connected after approval")
	}
}
