package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/remote"
	"github.com/yerinsabraham/trackline/engine/internal/relay"
	"github.com/yerinsabraham/trackline/engine/internal/relay/relaytest"
)

// fakeAccount plays the trackline API for one machine: it relays a pairing
// answer from a phone, hands out queued jobs one per wait, and records what
// the runner reports.
type fakeAccount struct {
	mu       sync.Mutex
	answer   func(code string) relay.PairAnswer
	code     string // what the laptop printed, read off its screen
	paired   *bool
	projects map[string]string
	queue    []remote.Delivery
	stop     bool
	results  map[string]remote.Result
}

func (f *fakeAccount) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("authorization") != "Bearer machine-token" {
		w.WriteHeader(401)
		return
	}
	send := func(v any) { json.NewEncoder(w).Encode(v) }
	p := r.URL.Path
	switch {
	case r.Method == "POST" && p == "/remote/pairings":
		send(remote.Pairing{ID: "pair_1", ExpiresIn: 60})
	case r.Method == "GET" && p == "/remote/pairings/pair_1":
		if f.code == "" {
			send(remote.PairingState{Status: "waiting"})
			return
		}
		a := f.answer(f.code)
		send(remote.PairingState{Status: "answered", Answer: &a})
	case r.Method == "POST" && p == "/remote/pairings/pair_1/result":
		var body struct{ OK bool }
		json.NewDecoder(r.Body).Decode(&body)
		f.paired = &body.OK
	case r.Method == "PUT" && strings.HasPrefix(p, "/remote/projects/"):
		var body struct{ Name string }
		json.NewDecoder(r.Body).Decode(&body)
		f.projects[strings.TrimPrefix(p, "/remote/projects/")] = body.Name
	case r.Method == "GET" && p == "/remote/jobs/next":
		if len(f.queue) > 0 {
			d := f.queue[0]
			f.queue = f.queue[1:]
			send(remote.Next{Job: &d})
			return
		}
		send(remote.Next{Stop: f.stop})
	case r.Method == "POST" && strings.HasSuffix(p, "/result"):
		var res remote.Result
		json.NewDecoder(r.Body).Decode(&res)
		f.results[strings.TrimSuffix(strings.TrimPrefix(p, "/remote/jobs/"), "/result")] = res
	default:
		w.WriteHeader(404)
	}
}

// remoteEnv is a connected machine with one connected project, its config in
// a temporary folder and launchctl replaced.
func remoteEnv(t *testing.T, f *fakeAccount) (root string) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	t.Setenv("TRACKLINE_NO_NOTIFY", "1")
	t.Setenv("TRACKLINE_ORIGIN", "")
	t.Setenv("HOME", t.TempDir())
	launchctl = func(...string) error { return nil }
	pairPoll, pollWait = 10*time.Millisecond, time.Second

	if err := account.SaveCredentials(account.Credentials{API: srv.URL, Token: "machine-token", DeviceID: "dev_laptop", DeviceName: "laptop"}); err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	if _, err := account.ConnectProject(root, false); err != nil {
		t.Fatal(err)
	}
	return root
}

// The phone "reads" the code as the laptop prints it: the only way a code
// gets from the laptop to the phone is a person's eyes.
func captureCode(t *testing.T, f *fakeAccount, run func() error) error {
	t.Helper()
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	done := make(chan error)
	go func() { done <- run() }()
	go func() {
		buf := make([]byte, 4096)
		var seen strings.Builder
		for {
			n, err := r.Read(buf)
			seen.Write(buf[:n])
			for _, line := range strings.Split(seen.String(), "\n") {
				line = strings.TrimSpace(line)
				if len(line) == relay.CodeLength+1 && line[relay.CodeLength/2] == '-' {
					f.mu.Lock()
					f.code = line
					f.mu.Unlock()
				}
			}
			if err != nil {
				return
			}
		}
	}()
	err := <-done
	w.Close()
	os.Stdout = stdout
	return err
}

// R2's exit, end to end on the laptop: pair a passkey through the account,
// then receive a signed test job, refuse a forged one, refuse one for a
// folder not enabled, refuse a replay, and stop when the site says stop.
func TestRemoteRunnerEndToEnd(t *testing.T) {
	phone := relaytest.New("iPhone")
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	f.answer = func(code string) relay.PairAnswer { return phone.Pair("dev_laptop", code) }
	root := remoteEnv(t, f)

	if err := captureCode(t, f, func() error { return remoteEnable([]string{"--root", root, "--no-start"}) }); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if f.paired == nil || !*f.paired {
		t.Fatal("the laptop did not report the pairing as trusted")
	}
	_, proj, _ := account.ProjectFor(root)
	if f.projects[proj.ID] != filepath.Base(root) {
		t.Fatalf("the account was not told the project is enabled: %v", f.projects)
	}

	now := time.Now()
	good := relay.Job{V: 1, ID: "job_ok", Machine: "dev_laptop", Project: proj.ID, Kind: "test",
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	forged := good
	forged.ID = "job_forged"
	server := relaytest.New("server")
	server.ID = phone.ID // claims to be the paired passkey
	elsewhere := good
	elsewhere.ID, elsewhere.Project = "job_elsewhere", "proj_not_enabled"

	okEnv := phone.Send(good)
	f.queue = []remote.Delivery{
		{ID: "job_ok", Envelope: okEnv},
		{ID: "job_forged", Envelope: server.Send(forged)},
		{ID: "job_elsewhere", Envelope: phone.Send(elsewhere)},
		{ID: "job_again", Envelope: okEnv},
	}
	f.stop = true

	done := make(chan error)
	go func() { done <- remoteRun() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the runner did not stop when the site said stop")
	}

	want := map[string]string{
		"job_ok":        "done",
		"job_forged":    "refused bad-signature",
		"job_elsewhere": "refused project-not-enabled",
		"job_again":     "refused replayed",
	}
	for id, w := range want {
		r := f.results[id]
		got := strings.TrimSpace(r.Status + " " + r.Code)
		if got != w {
			t.Errorf("%s: got %q, want %q (%s)", id, got, w, r.Reason)
		}
	}
}

func TestPairingRefusesAnAnswerOverAnotherCode(t *testing.T) {
	phone := relaytest.New("iPhone")
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	// The server never saw the code, so it guesses.
	f.answer = func(string) relay.PairAnswer { return phone.Pair("dev_laptop", "AAAA-AAAA") }
	root := remoteEnv(t, f)

	err := captureCode(t, f, func() error { return remoteEnable([]string{"--root", root, "--no-start"}) })
	if err == nil {
		t.Fatal("enabled with a pairing over the wrong code")
	}
	if f.paired == nil || *f.paired {
		t.Fatal("the refusal was not reported to the account")
	}
	s, _ := relay.Load()
	if len(s.Keys) != 0 || len(s.Projects) != 0 {
		t.Fatalf("something was trusted anyway: %+v", s)
	}
}

func TestEnableRefusesAProjectThatIsNotConnected(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	remoteEnv(t, f)
	if err := remoteEnable([]string{"--root", t.TempDir(), "--no-start"}); err == nil || !strings.Contains(err.Error(), "trackline connect") {
		t.Fatalf("got %v", err)
	}
}

func TestDisableAllForgetsEverything(t *testing.T) {
	phone := relaytest.New("iPhone")
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	f.answer = func(code string) relay.PairAnswer { return phone.Pair("dev_laptop", code) }
	root := remoteEnv(t, f)
	if err := captureCode(t, f, func() error { return remoteEnable([]string{"--root", root, "--no-start"}) }); err != nil {
		t.Fatal(err)
	}
	if err := remoteDisable([]string{"--all"}); err != nil {
		t.Fatal(err)
	}
	s, _ := relay.Load()
	if len(s.Keys) != 0 || len(s.Projects) != 0 {
		t.Fatalf("still trusted: %+v", s)
	}
}

func TestLaunchPlistIsValid(t *testing.T) {
	plist := launchPlist("/usr/local/bin/trackline", "/tmp/a&b/remote.log", map[string]string{"TRACKLINE_API": "http://localhost:3000/trackline/v1"})
	for _, want := range []string{"<string>remote</string>", "<string>run</string>", "SuccessfulExit", "a&amp;b", "TRACKLINE_API"} {
		if !strings.Contains(plist, want) {
			t.Errorf("missing %s", want)
		}
	}
	lint, err := exec.LookPath("plutil")
	if err != nil {
		t.Skip("plutil is macOS only")
	}
	p := filepath.Join(t.TempDir(), "x.plist")
	os.WriteFile(p, []byte(plist), 0o644)
	if out, err := exec.Command(lint, "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
}
