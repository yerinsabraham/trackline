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
	"github.com/yerinsabraham/trackline/engine/internal/relay/agent"
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
	// stopAfter says stop once this many results are in, so a test can let
	// running agents finish first.
	stopAfter int
	results   map[string]remote.Result
	events    map[string][]agent.Event
	cancel    map[string]bool
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
		send(remote.Next{Stop: f.stop || f.stopAfter > 0 && len(f.results) >= f.stopAfter})
	case r.Method == "POST" && strings.HasSuffix(p, "/events"):
		var body struct {
			From   int
			Events []agent.Event
		}
		json.NewDecoder(r.Body).Decode(&body)
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/remote/jobs/"), "/events")
		if f.events == nil {
			f.events = map[string][]agent.Event{}
		}
		if body.From == len(f.events[id]) {
			f.events[id] = append(f.events[id], body.Events...)
		}
		send(map[string]bool{"cancel": f.cancel[id]})
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

func TestMacPermissionPromptHintsProtectedFolders(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{`cat ~/Downloads/cv.pdf`, "Downloads"},
		{`/Users/ada/Desktop/report.pdf`, "Desktop"},
		{`read $HOME/Documents/tax.txt`, "Documents"},
		{`/Users/ada/Library/Mobile Documents/com~apple~CloudDocs/CV.pdf`, "iCloud Drive"},
		{`ls /Volumes/Passport/archive`, "an external or network volume"},
		{`cat ./downloads-helper.ts`, ""},
	}
	for _, c := range cases {
		if got := macPermissionArea(c.text); got != c.want {
			t.Errorf("%q: got %q, want %q", c.text, got, c.want)
		}
	}

	hint, ok := permissionPromptEvent(agent.Event{Kind: "tool", Tool: "Read", Text: "~/Downloads/cv.pdf"})
	if !ok || hint.Kind != "permission" || !strings.Contains(hint.Text, "Approve it on the Mac") {
		t.Fatalf("no useful hint: ok=%v %+v", ok, hint)
	}
	if _, ok := permissionPromptEvent(agent.Event{Kind: "say", Text: "~/Downloads/cv.pdf"}); ok {
		t.Fatal("ordinary agent text must not become a permission prompt")
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

// fakeClaude is a stand-in for Claude Code: it records how it was started,
// and answers in Claude's stream-json. Told "sleep", it works until stopped.
const fakeClaude = `#!/bin/sh
prompt=$(cat)
printf '%s' "$prompt" > "$FAKE_OUT/prompt"
echo "$@" > "$FAKE_OUT/argv"
echo "$TRACKLINE_REMOTE_JOB" > "$FAKE_OUT/job"
pwd -P > "$FAKE_OUT/pwd"
if [ "$prompt" = "think" ]; then sleep 60; fi
printf '{"type":"system","subtype":"init","session_id":"sess-1"}\n'
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}\n'
if [ "$prompt" = "sleep" ]; then sleep 60; fi
printf '{"type":"result","subtype":"success","result":"all done","is_error":false}\n'
`

// promptEnv is remoteEnv with a fake claude on PATH, trackline wired into the
// project for Claude Code, and a phone paired.
func promptEnv(t *testing.T, f *fakeAccount) (root, out string, phone *relaytest.Phone, proj string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs a POSIX shell")
	}
	phone = relaytest.New("iPhone")
	f.answer = func(code string) relay.PairAnswer { return phone.Pair("dev_laptop", code) }
	root = remoteEnv(t, f)
	bin, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeClaude), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_OUT", out)
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"command":"/bin/trackline-hook"}]}]}}`), 0o644)
	flushEvery = 10 * time.Millisecond
	if err := captureCode(t, f, func() error { return remoteEnable([]string{"--root", root, "--no-start"}) }); err != nil {
		t.Fatal(err)
	}
	_, p, _ := account.ProjectFor(root)
	return root, out, phone, p.ID
}

func prompt(phone *relaytest.Phone, id, project, text string) remote.Delivery {
	now := time.Now()
	return remote.Delivery{ID: id, Envelope: phone.Send(relay.Job{V: 1, ID: id, Machine: "dev_laptop", Project: project,
		Kind: "prompt", Agent: "claude", Text: text, IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}
}

func runUntilStopped(t *testing.T) {
	t.Helper()
	done := make(chan error)
	go func() { done <- remoteRun() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the runner did not stop")
	}
}

func read(t *testing.T, path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

func TestRemotePromptRunsTheAgentWatched(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	// Text that would be a flag on a command line, to show it never is one.
	text := "--dangerously-skip-permissions tidy the README"
	f.queue = []remote.Delivery{prompt(phone, "job_prompt", proj, text)}
	f.stopAfter = 1
	runUntilStopped(t)

	res := f.results["job_prompt"]
	if res.Status != "done" || res.Output != "all done" || res.Session != "sess-1" {
		t.Fatalf("result: %+v", res)
	}
	if read(t, filepath.Join(out, "prompt")) != text {
		t.Error("the instruction did not arrive on stdin")
	}
	if strings.Contains(read(t, filepath.Join(out, "argv")), "tidy") {
		t.Error("the instruction reached the command line")
	}
	if read(t, filepath.Join(out, "job")) != "job_prompt" {
		t.Error("the agent was not marked as remote, so trackline would not block")
	}
	want, _ := filepath.EvalSymlinks(root)
	if read(t, filepath.Join(out, "pwd")) != want {
		t.Errorf("ran in %s, not the project", read(t, filepath.Join(out, "pwd")))
	}
	kinds := []string{}
	for _, e := range f.events["job_prompt"] {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "session,say,done" {
		t.Errorf("streamed %v", kinds)
	}
}

func TestStopFromThePhoneStopsTheAgent(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}, cancel: map[string]bool{"job_long": true}}
	_, _, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{prompt(phone, "job_long", proj, "sleep")}
	f.stopAfter = 1
	started := time.Now()
	runUntilStopped(t)
	if r := f.results["job_long"]; r.Status != "failed" || r.Code != "stopped" {
		t.Fatalf("result: %+v", r)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("the agent was not stopped promptly")
	}
}

func TestOnePromptAtATimePerProject(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, _, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{prompt(phone, "job_first", proj, "sleep"), prompt(phone, "job_second", proj, "hello")}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_second"]; r.Status != "refused" || r.Code != "busy" {
		t.Fatalf("second: %+v", r)
	}
}

func TestAPromptIsRefusedWhereTracklineIsNotWatching(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	os.Remove(filepath.Join(root, ".claude", "settings.json"))
	f.queue = []remote.Delivery{prompt(phone, "job_unwatched", proj, "hello")}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_unwatched"]; r.Status != "refused" || r.Code != "not-watched" {
		t.Fatalf("result: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(out, "prompt")); err == nil {
		t.Fatal("the agent ran unwatched")
	}
}

// Codex runs a project's hook only once trusted there. Wired but never heard
// from, it would run unwatched, so it does not run.
func TestCodexIsRefusedUntilItsHookHasFired(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, _, phone, proj := promptEnv(t, f)
	os.MkdirAll(filepath.Join(root, ".codex"), 0o755)
	os.WriteFile(filepath.Join(root, ".codex", "hooks.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"command":"/bin/trackline-hook"}]}]}}`), 0o644)
	d := prompt(phone, "job_codex", proj, "hello")
	now := time.Now()
	d.Envelope = phone.Send(relay.Job{V: 1, ID: "job_codex", Machine: "dev_laptop", Project: proj, Kind: "prompt", Agent: "codex",
		Text: "hello", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})
	f.queue = []remote.Delivery{d}
	f.stopAfter = 1
	runUntilStopped(t)
	r := f.results["job_codex"]
	if r.Status != "refused" || r.Code != "not-watched" || !strings.Contains(r.Reason, "trust") {
		t.Fatalf("result: %+v", r)
	}
}

// An agent thinking says nothing for a long time. Stop must still reach it:
// measured with real Claude Code, a stop pressed mid-thought took a minute
// when the laptop only asked alongside new events.
func TestStopReachesASilentAgent(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}, cancel: map[string]bool{"job_think": true}}
	_, _, phone, proj := promptEnv(t, f)
	heartbeat = 50 * time.Millisecond
	f.queue = []remote.Delivery{prompt(phone, "job_think", proj, "think")}
	f.stopAfter = 1
	started := time.Now()
	runUntilStopped(t)
	if r := f.results["job_think"]; r.Code != "stopped" {
		t.Fatalf("result: %+v", r)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatalf("stop took %s", time.Since(started))
	}
}

// The reply box on the phone continues the agent's own session.
func TestARemotePromptContinuesItsSession(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	now := time.Now()
	f.queue = []remote.Delivery{{ID: "job_again", Envelope: phone.Send(relay.Job{V: 1, ID: "job_again", Machine: "dev_laptop", Project: proj,
		Kind: "prompt", Agent: "claude", Text: "and add a test", Session: "sess-1", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_again"]; r.Status != "done" {
		t.Fatalf("result: %+v", r)
	}
	if argv := read(t, filepath.Join(out, "argv")); !strings.Contains(argv, "--resume sess-1") {
		t.Fatalf("not resumed: %s", argv)
	}
}
