package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/remote"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/override"
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
	// images the server hands out, by hash: honest or not.
	images map[string][]byte
	// googleSub is the account's Google sign-in, if any, and confirm what the
	// laptop said about Google and email codes.
	googleSub string
	confirm   map[string]bool
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
	case r.Method == "GET" && strings.Contains(p, "/images/"):
		b, ok := f.images[p[strings.LastIndex(p, "/")+1:]]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Write(b)
	case r.Method == "GET" && p == "/devices/me":
		me := map[string]any{"device": map[string]string{"id": "dev_laptop"}, "user": map[string]string{"email": "person@example.com"}}
		if f.googleSub != "" {
			me["google"] = map[string]string{"sub": f.googleSub, "email": "person@example.com"}
		}
		send(me)
	case r.Method == "PUT" && p == "/remote/confirm":
		json.NewDecoder(r.Body).Decode(&f.confirm)
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
	was := interactive
	interactive = func() bool { return true }
	t.Cleanup(func() { interactive = was })
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

// Signed in with Google, a person turns remote on without pairing anything:
// the laptop records which Google account, said on screen, and tells the
// account so the phone offers Google.
func TestEnableWithGoogleNeedsNoPasskey(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}, googleSub: "g-123"}
	root := remoteEnv(t, f)
	stdin := os.Stdin
	t.Cleanup(func() { os.Stdin = stdin })
	r, w, _ := os.Pipe()
	w.WriteString("n\n")
	w.Close()
	os.Stdin = r
	if err := remoteEnable([]string{"--root", root, "--no-start"}); err != nil {
		t.Fatal(err)
	}
	s, _ := relay.Load()
	if s.Google == nil || s.Google.Sub != "g-123" || len(s.Keys) != 0 {
		t.Fatalf("Google = %+v, keys = %d", s.Google, len(s.Keys))
	}
	if s.AccountCodes || !s.AccountAsked {
		t.Fatalf("email codes answered no, yet: %v asked %v", s.AccountCodes, s.AccountAsked)
	}
	if f.paired != nil {
		t.Fatal("a pairing was started though Google confirms")
	}
	if !f.confirm["google"] || f.confirm["email"] {
		t.Fatalf("the account was told %v", f.confirm)
	}
	if len(s.Projects) == 0 {
		t.Fatal("no project turned on")
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
if [ "$1" = "agents" ]; then
	if [ -n "$FAKE_LIST_FAILS" ]; then exit 1; fi
	printf '[{"kind":"interactive","sessionId":"%s"}]' "$FAKE_OPEN"
	exit 0
fi
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
	// sess-1 is a session trackline's hook has seen here, so it may be continued.
	os.MkdirAll(filepath.Join(root, ".trackline"), 0o755)
	os.WriteFile(filepath.Join(root, ".trackline", "events.jsonl"), []byte(`{"id":"a","sessionId":"sess-1"}`+"\n"), 0o644)
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

func reply(phone *relaytest.Phone, id, project string) remote.Delivery {
	now := time.Now()
	return remote.Delivery{ID: id, Envelope: phone.Send(relay.Job{V: 1, ID: id, Machine: "dev_laptop", Project: project,
		Kind: "prompt", Agent: "claude", Session: "sess-1", Text: "carry on", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}
}

func TestReplyToAnOpenClaudeChatIsHeld(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	t.Setenv("FAKE_OPEN", "sess-1")
	f.queue = []remote.Delivery{reply(phone, "job_open", proj)}
	f.stopAfter = 1
	runUntilStopped(t)

	if res := f.results["job_open"]; res.Status != "refused" || res.Code != "in-use" {
		t.Fatalf("result: %+v", res)
	}
	if read(t, filepath.Join(out, "prompt")) != "" {
		t.Error("a second process ran on an open chat")
	}
}

func TestReplyIsHeldWhenItCannotTellTheChatIsClosed(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	t.Setenv("FAKE_LIST_FAILS", "1")
	f.queue = []remote.Delivery{reply(phone, "job_unknown", proj)}
	f.stopAfter = 1
	runUntilStopped(t)

	if res := f.results["job_unknown"]; res.Code != "in-use" {
		t.Fatalf("result: %+v", res)
	}
	if read(t, filepath.Join(out, "prompt")) != "" {
		t.Error("unknown must count as open")
	}
}

func TestReplyToAClosedClaudeChatResumesIt(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	t.Setenv("FAKE_OPEN", "another-chat")
	f.queue = []remote.Delivery{reply(phone, "job_closed", proj)}
	f.stopAfter = 1
	runUntilStopped(t)

	if res := f.results["job_closed"]; res.Status != "done" {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(read(t, filepath.Join(out, "argv")), "--resume sess-1") {
		t.Error("the chat was not continued")
	}
}

// fakeCodex is Codex holding a chat: resume is refused, as measured on 0.166.
func fakeCodex(t *testing.T) (binary, out string) {
	t.Helper()
	bin, out := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = queue ]; then for a; do echo \"$a\"; done > " + out + "/queued; exit 0; fi\n" +
		"echo 'thread abc already has an active writer' >&2; exit 1\n"
	os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755)
	return filepath.Join(bin, "codex"), out
}

func TestJoinOpenChat(t *testing.T) {
	bin, out := fakeCodex(t)
	j := relay.Job{Kind: "prompt", Session: "sess-1", Text: "--carry on"}

	if res := joinOpenChat(context.Background(), j, t.TempDir(), bin, "", false); res.Code != "in-use" {
		t.Fatalf("not switched on, got %+v", res)
	}
	if read(t, filepath.Join(out, "queued")) != "" {
		t.Fatal("queued without the person's say-so at the laptop")
	}

	withImage := j
	withImage.Images = []relay.Image{{Type: "image/png"}}
	decision := j
	decision.Kind = "allow"
	for _, held := range []relay.Job{withImage, decision} {
		if res := joinOpenChat(context.Background(), held, t.TempDir(), bin, "", true); res.Code != "in-use" {
			t.Fatalf("%+v was not held: %+v", held, res)
		}
	}

	res := joinOpenChat(context.Background(), j, t.TempDir(), bin, "", true)
	if res.Status != "done" || res.Code != "queued" || res.Session != "sess-1" {
		t.Fatalf("result: %+v", res)
	}
	if got := read(t, filepath.Join(out, "queued")); !strings.Contains(got, "--message=--carry on") {
		t.Fatalf("queued: %s", got)
	}
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
// trackline's settings live in the project, where a remote agent writes. A
// change since remote was enabled could be the agent switching off its own
// checks, so nothing runs until the person accepts it on the laptop.
// What R6 found, and the review of 2026-10-31 proved again when this was
// relaxed: a remote agent must not be able to rewrite what watches it, and a
// process it leaves behind can do that between jobs. Any change refuses.
func TestAPromptIsRefusedWhenTracklineSettingsChanged(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	os.WriteFile(filepath.Join(root, ".trackline.json"), []byte(`{"disabled":["off-limits"]}`), 0o644)
	f.queue = []remote.Delivery{prompt(phone, "job_changed", proj, "hello")}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_changed"]; r.Status != "refused" || r.Code != "changed" {
		t.Fatalf("result: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(out, "prompt")); err == nil {
		t.Fatal("the agent ran under changed settings")
	}
}

func TestEnableNeedsAPersonAtATerminal(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root := remoteEnv(t, f)
	interactive = func() bool { return false }
	if err := remoteEnable([]string{"--root", root, "--no-start"}); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("got %v", err)
	}
}

// Only a person at a terminal says which agents the runner starts.
func TestNoTerminalLeavesTheAgentsAlone(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	remoteEnv(t, f)
	relay.Update(func(s *relay.State) error {
		s.All, s.Keys, s.Agents = true, []relay.Key{{ID: "k"}}, map[string]string{"claude": "/safe/claude"}
		return nil
	})
	interactive = func() bool { return false }
	client, _, err := remoteClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := enrollAll(client, true); err != nil {
		t.Fatal(err)
	}
	if s, _ := relay.Load(); s.Agents["claude"] != "/safe/claude" {
		t.Fatalf("agents rewritten without a person: %v", s.Agents)
	}
}

func TestAnAgentInsideAProjectIsNotRecorded(t *testing.T) {
	connected := map[string]account.Project{"/work/app": {ID: "a"}}
	kept := outsideProjects(map[string]string{"claude": "/work/app/node_modules/.bin/claude", "cursor": "/work/app/..bin/cursor", "codex": "/usr/local/bin/codex"}, connected)
	if len(kept) != 1 || kept["codex"] == "" {
		t.Fatalf("kept %v", kept)
	}
	if got := pathOutsideProjects("/work/app/node_modules/.bin:/usr/bin", connected); got != "/usr/bin" {
		t.Fatalf("path %q", got)
	}
}

// A session id comes from the server. One that never ran in this project is
// not continued here, or another project's conversation would come with it.
func TestASessionFromElsewhereIsNotContinued(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	d := decisionJob(phone, "job_foreign", proj, "deny")
	now := time.Now()
	d.Envelope = phone.Send(relay.Job{V: 1, ID: "job_foreign", Machine: "dev_laptop", Project: proj, Kind: "prompt", Agent: "claude",
		Text: "hello", Session: "sess-other", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})
	f.queue = []remote.Delivery{d}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_foreign"]; r.Status != "refused" || r.Code != "unknown-session" {
		t.Fatalf("result: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(out, "prompt")); err == nil {
		t.Fatal("the agent resumed a session from elsewhere")
	}
}

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

// A test or development config folder has its own login agent, so turning
// remote off there never touches the real one. It once did.
func TestAnotherConfigFolderNeverTouchesTheRealLoginAgent(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", "")
	real := launchLabel()
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	if launchLabel() == real || !strings.HasPrefix(launchLabel(), real+".") {
		t.Fatalf("real %q, test %q", real, launchLabel())
	}
	var booted []string
	launchctl = func(args ...string) error { booted = append(booted, strings.Join(args, " ")); return nil }
	stopAtLogin()
	for _, b := range booted {
		if strings.HasSuffix(b, "/"+real) {
			t.Fatalf("stopped the real agent: %s", b)
		}
	}
}

func decisionJob(phone *relaytest.Phone, id, project, kind string) remote.Delivery {
	now := time.Now()
	return remote.Delivery{ID: id, Envelope: phone.Send(relay.Job{V: 1, ID: id, Machine: "dev_laptop", Project: project,
		Kind: kind, Agent: "claude", Session: "sess-1", Check: "off-limits", Target: ".env",
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}
}

// Allow once records the approval where the hook finds it, then continues
// the session telling the agent to go ahead, in words written here.
func TestAllowOnceRecordsTheApprovalAndResumes(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{decisionJob(phone, "job_allow", proj, "allow")}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_allow"]; r.Status != "done" {
		t.Fatalf("result: %+v", r)
	}
	grants, _ := relay.GrantsPath(proj)
	if _, ok := (&override.Store{Path: grants, Root: root}).Allows("off-limits", filepath.Join(root, ".env"), "sess-1", "any"); !ok {
		t.Fatal("the approval was not recorded")
	}
	// Not in the project, where the agent could have written it itself.
	if len(override.NewStore(root).List()) != 0 {
		t.Fatal("the approval was written into the project")
	}
	if !strings.Contains(read(t, filepath.Join(out, "argv")), "--resume sess-1") || !strings.Contains(read(t, filepath.Join(out, "prompt")), "Retry the same action now") {
		t.Fatalf("argv %q prompt %q", read(t, filepath.Join(out, "argv")), read(t, filepath.Join(out, "prompt")))
	}
}

func TestKeepBlockedRecordsNothing(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{decisionJob(phone, "job_deny", proj, "deny")}
	f.stopAfter = 1
	runUntilStopped(t)
	grants, _ := relay.GrantsPath(proj)
	if len(override.NewStore(root).List()) != 0 || len((&override.Store{Path: grants}).List()) != 0 {
		t.Fatal("keeping it blocked recorded an approval")
	}
	if !strings.Contains(read(t, filepath.Join(out, "prompt")), "keep this blocked") {
		t.Fatalf("prompt %q", read(t, filepath.Join(out, "prompt")))
	}
}

func TestAnInstallInAGuardedFolderIsWarnedAbout(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if protectedInstall("/Users/a/Documents/trackline-dev/trackline") == "" {
		t.Error("an install in Documents went unmentioned")
	}
	if protectedInstall("/usr/local/lib/node_modules/trackline/bin/trackline") != "" {
		t.Error("an ordinary install was warned about")
	}
}

// A tiny valid PNG: the signature and enough after it to be a file.
var png = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 40)...)

func imageJob(phone *relaytest.Phone, id, project string, b []byte) remote.Delivery {
	sum := sha256.Sum256(b)
	now := time.Now()
	return remote.Delivery{ID: id, Envelope: phone.Send(relay.Job{V: 1, ID: id, Machine: "dev_laptop", Project: project,
		Kind: "prompt", Agent: "claude", Text: "what is in the picture?",
		Images:   []relay.Image{{Type: "image/png", Size: len(b), SHA256: hex.EncodeToString(sum[:])}},
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}
}

// An image attached on the phone reaches the agent: saved in the project,
// out of git's way, and named in the prompt.
func TestAnAttachedImageReachesTheAgent(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	sum := sha256.Sum256(png)
	f.images = map[string][]byte{hex.EncodeToString(sum[:]): png}
	f.queue = []remote.Delivery{imageJob(phone, "job_withanimage000001", proj, png)}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_withanimage000001"]; r.Status != "done" {
		t.Fatalf("result: %+v", r)
	}
	saved := filepath.Join(root, ".trackline", "attachments", "job_withanimage000001", "1.png")
	if b, err := os.ReadFile(saved); err != nil || !bytes.Equal(b, png) {
		t.Fatalf("image not saved: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".trackline", "attachments", ".gitignore")); string(b) != "*\n" {
		t.Error("attachments are not kept out of git")
	}
	if p := read(t, filepath.Join(out, "prompt")); !strings.Contains(p, ".trackline/attachments/job_withanimage000001/1.png") || !strings.Contains(p, "what is in the picture?") {
		t.Fatalf("prompt: %q", p)
	}
}

// The server hands over a different picture than the one signed for: the
// laptop takes nothing and runs nothing.
func TestASwappedImageIsRefused(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	sum := sha256.Sum256(png)
	other := append(append([]byte{}, png[:8]...), bytes.Repeat([]byte{2}, 40)...)
	f.images = map[string][]byte{hex.EncodeToString(sum[:]): other}
	f.queue = []remote.Delivery{imageJob(phone, "job_swappedimage000001", proj, png)}
	f.stopAfter = 1
	runUntilStopped(t)
	if r := f.results["job_swappedimage000001"]; r.Status != "refused" || r.Code != "images" {
		t.Fatalf("result: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(out, "prompt")); err == nil {
		t.Fatal("the agent ran with an image nobody signed for")
	}
	if _, err := os.Stat(filepath.Join(root, ".trackline", "attachments", "job_swappedimage000001", "1.png")); err == nil {
		t.Fatal("the swapped image was kept")
	}
}

func handoffJob(phone *relaytest.Phone, id, project string) remote.Delivery {
	now := time.Now()
	return remote.Delivery{ID: id, Envelope: phone.Send(relay.Job{V: 1, ID: id, Machine: "dev_laptop", Project: project,
		Kind: "handoff", Agent: "claude", Text: "Review the login change", Handoff: "hf_abcdefgh12", From: "sess-1", FromAgent: "claude",
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}
}

func TestAHandoffRunsMarkedAndKeepsItsResult(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{handoffJob(phone, "job_handoff", proj)}
	f.stopAfter = 1
	runUntilStopped(t)

	if res := f.results["job_handoff"]; res.Status != "done" || res.Output != "all done" {
		t.Fatalf("result: %+v", res)
	}
	if got := read(t, filepath.Join(out, "prompt")); !strings.HasPrefix(got, "Another agent asked for this") || !strings.HasSuffix(got, "Review the login change") {
		t.Errorf("told: %q", got)
	}
	if strings.Contains(read(t, filepath.Join(out, "argv")), "ask-tool") {
		t.Error("a receiver must not get the tool, or two agents can loop")
	}
	s, _ := relay.Load()
	if h := s.Handoffs["hf_abcdefgh12"]; !h.Finished || h.Result != "all done" || !h.Reported {
		t.Fatalf("kept: %+v", h)
	}
}

func TestAPromptGetsTheToolAndADecisionDoesNot(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{prompt(phone, "job_tool", proj, "hello")}
	f.stopAfter = 1
	runUntilStopped(t)
	if got := read(t, filepath.Join(out, "argv")); !strings.Contains(got, "ask-tool") || !strings.Contains(got, "job_tool") {
		t.Fatalf("argv: %s", got)
	}
}

func TestAHandoffDeliveredTwiceRunsOnce(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	f.queue = []remote.Delivery{handoffJob(phone, "job_first", proj)}
	f.stopAfter = 1
	runUntilStopped(t)
	os.Remove(filepath.Join(out, "prompt"))

	// A new signature for the same handoff: a different job, the same work.
	f.queue = []remote.Delivery{handoffJob(phone, "job_second", proj)}
	f.stopAfter = 1
	runUntilStopped(t)
	if res := f.results["job_second"]; res.Code != "handoff-repeat" {
		t.Fatalf("result: %+v", res)
	}
	if read(t, filepath.Join(out, "prompt")) != "" {
		t.Fatal("the task ran twice")
	}
}

func TestAResultTheAccountNeverGotIsSentAgainNotRunAgain(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	promptEnv(t, f)
	relay.Update(func(s *relay.State) error {
		s.ClaimHandoff("hf_done", "job_lost_report", "p", time.Now())
		h := s.Handoffs["hf_done"]
		h.Finished, h.Result = true, "the result"
		s.Handoffs["hf_done"] = h
		s.ClaimHandoff("hf_gone", "job_lost_run", "p", time.Now().Add(-time.Hour))
		h = s.Handoffs["hf_gone"]
		h.At = time.Now().Add(-time.Hour)
		s.Handoffs["hf_gone"] = h
		return nil
	})
	client, _, err := remoteClient()
	if err != nil {
		t.Fatal(err)
	}
	resendHandoffs(client, newRuns())
	if res := f.results["job_lost_report"]; res.Status != "done" || res.Output != "the result" {
		t.Fatalf("report: %+v", res)
	}
	if res := f.results["job_lost_run"]; res.Status != "failed" || res.Code != "lost" {
		t.Fatalf("lost: %+v", res)
	}
}

func TestAHandoffThatChangesAgentFilesFailsWhateverRouteItUsed(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	// As a script would: nothing the checks see, only the file changing.
	bin := filepath.Dir(firstOnPath(t, "claude"))
	script := strings.Replace(fakeClaude, "pwd -P", "echo sneaked > "+root+"/Claude.md; pwd -P", 1)
	os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755)
	now := time.Now()
	f.queue = []remote.Delivery{{ID: "job_sneak", Envelope: phone.Send(relay.Job{V: 1, ID: "job_sneak", Machine: "dev_laptop", Project: proj,
		Kind: "handoff", Agent: "claude", Text: "Review it", Handoff: "hf_sneak_12345", From: "sess-1", FromAgent: "claude", Return: true,
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}}
	f.stopAfter = 1
	runUntilStopped(t)

	res := f.results["job_sneak"]
	if res.Status != "failed" || res.Code != "changed-agent-files" || !strings.Contains(res.Reason, "Claude.md") {
		t.Fatalf("result: %+v", res)
	}
	// Nothing went back: the last run was the receiver's.
	if !strings.HasPrefix(read(t, filepath.Join(out, "prompt")), "Another agent asked for this") {
		t.Fatal("the sender was resumed after agent files changed")
	}
}

func TestAResultIsNotReturnedIfWhatWatchesTheSenderChanged(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	root, out, phone, proj := promptEnv(t, f)
	bin := filepath.Dir(firstOnPath(t, "claude"))
	// Rewrites the settings that load next, outside the agent-file names a real
	// run is compared on, to show the guard stands on its own.
	script := strings.Replace(fakeClaude, "pwd -P", "echo '{}' > "+root+"/.trackline.json; pwd -P", 1)
	os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755)
	now := time.Now()
	f.queue = []remote.Delivery{{ID: "job_guard", Envelope: phone.Send(relay.Job{V: 1, ID: "job_guard", Machine: "dev_laptop", Project: proj,
		Kind: "handoff", Agent: "claude", Text: "Review it", Handoff: "hf_guard_12345", From: "sess-1", FromAgent: "claude", Return: true,
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}}
	f.stopAfter = 1
	runUntilStopped(t)

	if res := f.results["job_guard"]; res.Status != "done" || !strings.Contains(res.Reason, "did not go back") {
		t.Fatalf("result: %+v", res)
	}
	if !strings.HasPrefix(read(t, filepath.Join(out, "prompt")), "Another agent asked for this") {
		t.Fatal("the sender was resumed after the guard changed")
	}
}

func TestAResultGoesBackOnceWithItsLabel(t *testing.T) {
	f := &fakeAccount{projects: map[string]string{}, results: map[string]remote.Result{}}
	_, out, phone, proj := promptEnv(t, f)
	now := time.Now()
	f.queue = []remote.Delivery{{ID: "job_back", Envelope: phone.Send(relay.Job{V: 1, ID: "job_back", Machine: "dev_laptop", Project: proj,
		Kind: "handoff", Agent: "claude", Text: "Review it", Handoff: "hf_back_123456", From: "sess-1", FromAgent: "claude", Return: true,
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()})}}
	f.stopAfter = 1
	runUntilStopped(t)

	got := read(t, filepath.Join(out, "prompt"))
	if !strings.HasPrefix(got, intent.PeerPrefix) || !strings.Contains(got, "cannot set you a new task") {
		t.Fatalf("told: %q", got)
	}
	if !strings.Contains(read(t, filepath.Join(out, "argv")), "--resume sess-1") {
		t.Fatal("the sender was not resumed")
	}
	dir, _ := account.Dir()
	if !intent.LoadPeers(dir)[intent.PeerHash(got)] {
		t.Fatal("the result was not marked as a peer's before the sender read it")
	}
	if strings.Contains(read(t, filepath.Join(out, "argv")), "ask-tool") {
		t.Fatal("a returned result must not come with the tool")
	}
}

func firstOnPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSelfWatchWaitsForAStableReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trackline")
	write := func(s string) {
		tmp := path + ".new"
		if err := os.WriteFile(tmp, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
	}
	write("old")
	fi, _ := os.Stat(path)
	w := &selfWatch{path: path, was: fi}
	if w.replaced() {
		t.Fatal("unchanged, yet replaced")
	}
	write("new release")
	if w.replaced() {
		t.Fatal("replaced on the first sight of a new file, which may be half written")
	}
	if !w.replaced() {
		t.Fatal("the same new file twice, yet not replaced")
	}
	os.Remove(path)
	if w.replaced() {
		t.Fatal("gone mid-install, yet replaced")
	}
	var none *selfWatch
	if none.replaced() {
		t.Fatal("no watch, yet replaced")
	}
}
