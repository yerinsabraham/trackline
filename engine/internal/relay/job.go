// Package relay is the laptop's side of remote agents: what it trusts, and
// the checks a job must pass before anything runs.
//
// The server relays jobs; it is not trusted to decide them. A job is run only
// if a passkey this laptop paired with signed it, it is for this machine and a
// project enabled here, it is fresh, and it has not been seen before. Every
// refusal says which of those failed, so a forged job is reported as forged.
package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Job is what the person asked for, exactly as their device signed it.
//
// There is deliberately no field for a mode, flags or a folder. The runner
// chooses the agent's flags and runs in the enabled project's own folder, so
// a job cannot ask for full access or another directory: a job that tries
// carries an unknown field and is refused whole.
type Job struct {
	V       int    `json:"v"`
	ID      string `json:"id"`
	Machine string `json:"machine"`
	// Project is the random id `trackline connect` gave the project. The
	// server never learns the path, so it cannot name one.
	Project   string `json:"project"`
	Kind      string `json:"kind"`
	Agent     string `json:"agent,omitempty"`
	Text      string `json:"text"`
	IssuedAt  int64  `json:"issuedAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

// Envelope is a job as the server delivers it: the signed bytes, which key
// signed them, and the signature.
type Envelope struct {
	Job string `json:"job"`
	Key string `json:"key"`
	Assertion
}

const (
	// MaxLife bounds how long a job may say it is valid for. A job captured
	// in transit is useless after this.
	MaxLife = 5 * time.Minute
	// Skew allows for the phone's clock and the laptop's disagreeing.
	Skew = 2 * time.Minute
	// Idle is how long a project may go without a job before it has to be
	// enabled again on the laptop.
	Idle = 30 * 24 * time.Hour
	// MaxText is far more than any instruction a person types on a phone.
	MaxText = 32 << 10
)

// Kinds a job may be. R2 runs only the test job; prompts arrive with R3.
var kinds = map[string]bool{"test": true}

// Agents a job may name. Anything else is refused before it gets near a
// command line.
var agents = map[string]bool{"claude": true, "codex": true, "cursor": true}

// Refusal is why a job was not run. Code is stable, for the server's audit log
// and the site; Reason is for a person.
type Refusal struct {
	Code   string
	Reason string
}

func (r *Refusal) Error() string { return r.Reason }

func refuse(code, format string, a ...any) *Refusal {
	return &Refusal{Code: code, Reason: fmt.Sprintf(format, a...)}
}

// Accepted is a job that passed, with the folder it belongs to.
type Accepted struct {
	Job  Job
	Root string
}

// Check decides whether this laptop runs a job. It records the job as seen,
// so the same signed job is never accepted twice, even if the server sends it
// again.
func Check(s *State, env Envelope, self string, now time.Time) (Accepted, error) {
	raw, err := decodeB64(env.Job)
	if err != nil || len(raw) == 0 || len(raw) > MaxText+4096 {
		return Accepted{}, refuse("bad-envelope", "the job could not be read")
	}
	key, ok := s.key(env.Key)
	if !ok {
		return Accepted{}, refuse("unknown-key", "signed by a passkey this laptop has not paired with")
	}
	pub, err := ParseKey(key.PublicKey)
	if err != nil {
		return Accepted{}, refuse("unknown-key", "the paired passkey could not be read: %v", err)
	}
	if err := verifyAssertion(pub, s.rp(), env.Assertion, JobChallenge(raw)); err != nil {
		return Accepted{}, refuse("bad-signature", "forged or altered: %v", err)
	}

	// Only now is the content trusted enough to read.
	var j Job
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil || dec.More() {
		return Accepted{}, refuse("unsupported", "the job asks for something this runner does not do")
	}
	if j.V != 1 || j.ID == "" {
		return Accepted{}, refuse("unsupported", "a job format this runner does not know")
	}
	if j.Machine != self {
		return Accepted{}, refuse("wrong-machine", "the job is for another machine")
	}

	issued, expires := time.UnixMilli(j.IssuedAt), time.UnixMilli(j.ExpiresAt)
	switch {
	case expires.Sub(issued) > MaxLife || !expires.After(issued):
		return Accepted{}, refuse("too-long-lived", "the job claims to be valid for longer than %s", MaxLife)
	case issued.After(now.Add(Skew)):
		return Accepted{}, refuse("not-yet-valid", "the job is dated in the future; check this laptop's clock")
	case now.After(expires.Add(Skew)):
		return Accepted{}, refuse("expired", "the job expired before it arrived")
	}
	if _, seen := s.Seen[j.ID]; seen {
		return Accepted{}, refuse("replayed", "this job was already received once")
	}
	// Seen from here on, whatever happens next: a refused job stays refused
	// rather than becoming acceptable after a project is enabled.
	s.see(j.ID, expires.Add(Skew), now)

	if !kinds[j.Kind] {
		return Accepted{}, refuse("unsupported", "this runner does not do %q jobs yet", j.Kind)
	}
	if j.Agent != "" && !agents[j.Agent] {
		return Accepted{}, refuse("unsupported", "%q is not an agent this runner starts", j.Agent)
	}
	if len(j.Text) > MaxText || strings.ContainsRune(j.Text, 0) {
		return Accepted{}, refuse("unsupported", "the instruction is too long or not text")
	}

	p, ok := s.Projects[j.Project]
	if !ok {
		return Accepted{}, refuse("project-not-enabled", "remote is not enabled for that project on this laptop")
	}
	last := p.EnabledAt
	if p.LastJobAt.After(last) {
		last = p.LastJobAt
	}
	if now.Sub(last) > Idle {
		delete(s.Projects, j.Project)
		return Accepted{}, refuse("project-idle", "unused for %d days, so remote was turned off for %s; run trackline remote enable there to turn it on", int(Idle.Hours()/24), p.Root)
	}
	if fi, err := os.Stat(p.Root); err != nil || !fi.IsDir() {
		return Accepted{}, refuse("project-missing", "the project folder is no longer there")
	}
	p.LastJobAt = now
	s.Projects[j.Project] = p
	return Accepted{Job: j, Root: p.Root}, nil
}
