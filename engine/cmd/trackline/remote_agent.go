package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/remote"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/install"
	"github.com/yerinsabraham/trackline/engine/internal/relay"
	"github.com/yerinsabraham/trackline/engine/internal/relay/agent"
)

// Running a prompt: start the agent in the project, send what it does to
// the phone as it happens, and stop it when asked or when it runs too long.

var (
	// jobLimit is the longest one remote prompt may run. A task that needs
	// longer is better sent in parts that can each be read on a phone.
	jobLimit = 30 * time.Minute
	// flushEvery is how often what the agent did is sent on.
	flushEvery = time.Second
	// heartbeat is how often the laptop asks about stop while the agent is
	// silent: thinking can take minutes without a single event.
	heartbeat = 2 * time.Second
	// permissionWait is long enough not to fire on normal quick reads, but
	// short enough that a phone does not look frozen while macOS waits for a
	// local Files and Folders prompt.
	permissionWait = 8 * time.Second
)

var agentNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "cursor": "Cursor"}

// needsTrust are agents that run a project's hook only after the person
// trusts it there.
var needsTrust = map[string]install.Host{"codex": install.Codex}

// findAgents looks for each agent on this shell's PATH, which is the one the
// person uses; the runner, started by launchd, would not find them itself.
func findAgents() map[string]string {
	found := map[string]string{}
	for name, bin := range agent.Binaries {
		if p, err := exec.LookPath(bin); err == nil {
			if abs, err := filepath.Abs(p); err == nil {
				found[name] = abs
			}
		}
	}
	return found
}

func agentList(found map[string]string) string {
	var names []string
	for name, bin := range found {
		if _, err := agent.Command(name, bin, "", ""); err == nil {
			names = append(names, agentNames[name])
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "none found"
	}
	return strings.Join(names, ", ")
}

// runs holds what is running: one prompt per project at a time, so two
// agents never edit the same files at once.
type runs struct {
	mu     sync.Mutex
	cancel map[string]context.CancelCauseFunc
	wg     sync.WaitGroup
}

func newRuns() *runs { return &runs{cancel: map[string]context.CancelCauseFunc{}} }

func (r *runs) start(project string, cancel context.CancelCauseFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, busy := r.cancel[project]; busy {
		return false
	}
	r.cancel[project] = cancel
	r.wg.Add(1)
	return true
}

func (r *runs) finish(project string) {
	r.mu.Lock()
	delete(r.cancel, project)
	r.mu.Unlock()
	r.wg.Done()
}

// stopAll stops every agent and waits, briefly, for each to report how it
// ended.
func (r *runs) stopAll(why error) {
	r.mu.Lock()
	for _, c := range r.cancel {
		c(why)
	}
	r.mu.Unlock()
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
	}
}

var (
	errStopped = errors.New("stopped from the phone")
	errTimeout = fmt.Errorf("stopped after %s", jobLimit)
)

// startPrompt checks the laptop can run this prompt watched, then runs it in
// the background so the runner keeps listening, for a stop among others.
func startPrompt(client remote.Client, d remote.Delivery, got relay.Accepted, r *runs) {
	j := got.Job
	refuse := func(code, reason string) {
		logf("refused job %s: %s (%s)", d.ID, reason, code)
		client.JobResult(d.ID, remote.Result{Status: "refused", Code: code, Reason: reason})
	}
	s, err := relay.Load()
	if err != nil {
		refuse("failed", "the laptop could not read its remote settings")
		return
	}
	name := agentNames[j.Agent]
	// A remote session is watched exactly like a local one, or it does not
	// start.
	if !slices.Contains(install.Wired(got.Root), agent.Hosts[j.Agent]) {
		refuse("not-watched", fmt.Sprintf("trackline is not set up for %s in %s. Run there: trackline init --host %s", name, filepath.Base(got.Root), j.Agent))
		return
	}
	// Codex runs a project's hook only once it is trusted. Wired but never
	// heard from is a hook that is off: measured, a remote Codex run in such a
	// project went unwatched.
	if h, ok := needsTrust[j.Agent]; ok && slices.Contains(install.Silent(got.Root), h) {
		refuse("not-watched", fmt.Sprintf("trackline has never heard from %s in %s, so it would run unwatched. %s", name, filepath.Base(got.Root), install.Unsilence(h)))
		return
	}
	binary := s.Agents[j.Agent]
	if binary == "" {
		refuse("agent-missing", fmt.Sprintf("%s was not found on this laptop. Install it, then run trackline remote enable again", name))
		return
	}
	argv, err := agent.Command(j.Agent, binary, got.Root, j.Session)
	if err != nil {
		refuse("unsupported", fmt.Sprintf("%s: %v", name, err))
		return
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	if !r.start(j.Project, cancel) {
		cancel(nil)
		refuse("busy", "an agent is already working in this project; wait for it or stop it")
		return
	}
	logf("starting %s for job %s in %s", name, d.ID, got.Root)
	notify("trackline: remote prompt", fmt.Sprintf("%s started in %s: %s", name, filepath.Base(got.Root), firstLine(j.Text, 80)))
	go func() {
		defer r.finish(j.Project)
		defer cancel(nil)
		res := runAgent(ctx, cancel, client, d.ID, j, got.Root, argv, s.Path)
		if err := client.JobResult(d.ID, res); err != nil {
			logf("could not report job %s: %v", d.ID, err)
		}
		logf("job %s ended: %s %s", d.ID, res.Status, res.Code)
	}()
}

func firstLine(s string, n int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

type permissionWatch struct {
	start time.Time
	event agent.Event
	sent  bool
}

func macPermissionArea(s string) string {
	t := strings.ToLower(strings.ReplaceAll(s, "\\", "/"))
	switch {
	case strings.Contains(t, "icloud drive") || strings.Contains(t, "/mobile documents/"):
		return "iCloud Drive"
	case namesProtectedFolder(t, "downloads"):
		return "Downloads"
	case namesProtectedFolder(t, "desktop"):
		return "Desktop"
	case namesProtectedFolder(t, "documents"):
		return "Documents"
	case strings.Contains(t, "/volumes/"):
		return "an external or network volume"
	}
	return ""
}

func namesProtectedFolder(s, name string) bool {
	for start := 0; ; {
		i := strings.Index(s[start:], name)
		if i < 0 {
			return false
		}
		i += start
		beforeOK := i == 0 || strings.ContainsRune(` /'"=$~`, rune(s[i-1]))
		after := i + len(name)
		afterOK := after == len(s) || strings.ContainsRune(`/ '"`, rune(s[after]))
		if beforeOK && afterOK {
			return true
		}
		start = i + len(name)
	}
}

func permissionPromptEvent(e agent.Event) (agent.Event, bool) {
	if e.Kind != "tool" {
		return agent.Event{}, false
	}
	area := macPermissionArea(e.Text)
	if area == "" {
		return agent.Event{}, false
	}
	action := strings.TrimSpace(strings.TrimSpace(e.Tool + " " + e.Text))
	if action != "" {
		action = " Last action: " + firstLine(action, 180)
	}
	return agent.Event{
		Kind: "permission",
		Tool: "macOS permission",
		Text: fmt.Sprintf("Your Mac may be waiting for a local macOS permission prompt to access %s. Approve it on the Mac to continue, or stop this job.%s", area, action),
	}, true
}

// agentEnv is the environment an agent runs in: the person's own, with the
// PATH it was found on, and the mark that tells trackline's hook this session
// was started from the phone.
func agentEnv(base []string, path, job string) []string {
	var env []string
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		// CLAUDECODE marks a shell inside Claude Code, where claude declines
		// to start another session.
		if k == "PATH" && path != "" || k == config.RemoteEnv || k == "CLAUDECODE" {
			continue
		}
		env = append(env, kv)
	}
	if path != "" {
		env = append(env, "PATH="+path)
	}
	return append(env, config.RemoteEnv+"="+job)
}

// runAgent runs one prompt to the end and says how it ended.
func runAgent(ctx context.Context, cancel context.CancelCauseFunc, client remote.Client, id string, j relay.Job, root string, argv []string, path string) remote.Result {
	ctx, stopTimer := context.WithTimeoutCause(ctx, jobLimit, errTimeout)
	defer stopTimer()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Env = agentEnv(os.Environ(), path, id)
	// On stdin, never in argv: an instruction that looks like a flag is text.
	cmd.Stdin = strings.NewReader(j.Text)
	var stderr tail
	cmd.Stderr = &stderr
	inGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return remote.Result{Status: "failed", Reason: err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return remote.Result{Status: "failed", Reason: fmt.Sprintf("could not start %s: %v", agentNames[j.Agent], err)}
	}

	lines := make(chan agent.Event, 256)
	go func() {
		defer close(lines)
		p := agent.NewParser(j.Agent, root)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		for sc.Scan() {
			for _, e := range p.Line(sc.Bytes()) {
				lines <- e
			}
		}
	}()

	var res remote.Result
	var pending []agent.Event
	sent := 0
	asked := time.Now()
	var permission *permissionWatch
	flush := func() {
		if permission != nil && !permission.sent && time.Since(permission.start) >= permissionWait {
			pending = append(pending, permission.event)
			permission.sent = true
		}
		if len(pending) == 0 && time.Since(asked) < heartbeat {
			return
		}
		asked = time.Now()
		stop, err := client.JobEvents(id, sent, pending)
		if err != nil {
			// Kept and sent with the next batch. Bounded, so an account that
			// is down for the whole run cannot fill the laptop's memory.
			if len(pending) > 1000 {
				pending = pending[len(pending)-1000:]
			}
			return
		}
		sent += len(pending)
		pending = nil
		if stop {
			cancel(errStopped)
		}
	}
	tick := time.NewTicker(flushEvery)
	defer tick.Stop()
	for open := true; open; {
		select {
		case e, ok := <-lines:
			if !ok {
				open = false
				break
			}
			switch e.Kind {
			case "session":
				res.Session = e.Text
			case "done":
				res.Output = e.Text
			case "error":
				res.Reason = e.Text
			}
			if hint, ok := permissionPromptEvent(e); ok {
				permission = &permissionWatch{start: time.Now(), event: hint}
			} else if e.Kind != "session" && e.Kind != "permission" {
				permission = nil
			}
			pending = append(pending, e)
		case <-tick.C:
			flush()
		}
	}
	err = cmd.Wait()
	flush()

	switch cause := context.Cause(ctx); {
	case errors.Is(cause, errStopped):
		res.Status, res.Code, res.Reason = "failed", "stopped", errStopped.Error()
	case errors.Is(cause, errTimeout):
		res.Status, res.Code, res.Reason = "failed", "timeout", errTimeout.Error()
	case cause != nil:
		res.Status, res.Code, res.Reason = "failed", "stopped", "the laptop stopped remote jobs"
	case err != nil || res.Reason != "":
		res.Status = "failed"
		if res.Reason == "" {
			res.Reason = strings.TrimSpace(fmt.Sprintf("%s exited: %v\n%s", agentNames[j.Agent], err, stderr.String()))
		}
	default:
		res.Status = "done"
	}
	return res
}

// tail keeps the end of what an agent wrote to stderr, which is where it
// says why it failed.
type tail struct{ b []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 4096 {
		t.b = t.b[len(t.b)-4096:]
	}
	return len(p), nil
}

func (t *tail) String() string { return string(t.b) }

// statusAgents is the line `trackline remote status` shows for agents.
func statusAgents(s *relay.State) string {
	return "Agents it can start: " + agentList(s.Agents)
}
