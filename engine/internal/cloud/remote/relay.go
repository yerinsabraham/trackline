package remote

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/relay"
	"github.com/yerinsabraham/trackline/engine/internal/relay/agent"
)

// The runner's side of remote jobs. The machine credential can receive jobs
// and report on them; nothing here can send one.

// Pairing is a pairing the laptop started and is waiting on.
type Pairing struct {
	ID        string `json:"id"`
	ExpiresIn int    `json:"expiresIn"`
}

// PairingState is the answer to "has the phone answered yet".
type PairingState struct {
	Status string            `json:"status"` // waiting | answered | expired
	Answer *relay.PairAnswer `json:"answer,omitempty"`
}

func (c Client) StartPairing() (Pairing, error) {
	var out Pairing
	return out, c.do("POST", "/remote/pairings", map[string]string{}, &out)
}

func (c Client) PairingState(id string) (PairingState, error) {
	var out PairingState
	return out, c.do("GET", "/remote/pairings/"+url.PathEscape(id), nil, &out)
}

// PairingResult tells the account whether the laptop trusted the answer, so
// the phone can say so, and so a refused one is in the audit log.
func (c Client) PairingResult(id string, ok bool, reason string) error {
	return c.do("POST", "/remote/pairings/"+url.PathEscape(id)+"/result", map[string]any{"ok": ok, "reason": reason}, nil)
}

// SetRemoteProject tells the account a project takes remote jobs, by its
// random id and folder name only, so the site can offer it.
func (c Client) SetRemoteProject(id, name string) error {
	return c.do("PUT", "/remote/projects/"+url.PathEscape(id), map[string]string{"name": name}, nil)
}

func (c Client) UnsetRemoteProject(id string) error {
	return c.do("DELETE", "/remote/projects/"+url.PathEscape(id), nil, nil)
}

// UnsetAllRemote is the laptop's kill switch, told to the account.
func (c Client) UnsetAllRemote() error { return c.do("DELETE", "/remote/projects", nil, nil) }

// Delivery is one job, with the server's id for reporting back.
type Delivery struct {
	ID       string         `json:"id"`
	Envelope relay.Envelope `json:"envelope"`
}

// Next is what a wait for work returns.
type Next struct {
	Job *Delivery `json:"job,omitempty"`
	// Stop is the kill switch on the site. The runner stops until remote is
	// enabled again on the laptop.
	Stop bool `json:"stop,omitempty"`
}

// NextJob waits up to wait for a job. The server holds the request open
// rather than the laptop opening a port, so nothing on the internet can
// reach the laptop directly.
func (c Client) NextJob(wait time.Duration) (Next, error) {
	var out Next
	long := c
	// A connection that went quiet during sleep is abandoned rather than
	// waited on forever.
	long.HTTP = &http.Client{Timeout: wait + 15*time.Second}
	return out, long.do("GET", fmt.Sprintf("/remote/jobs/next?wait=%d", int(wait.Seconds())), nil, &out)
}

// Result is what the runner reports about a job.
type Result struct {
	Status string `json:"status"` // done | refused | failed
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
	Output string `json:"output,omitempty"`
	// Session is the agent's own session id, which joins the job to the
	// session trackline watched, and is what R4 resumes.
	Session string `json:"session,omitempty"`
}

// JobEvents sends what a running agent did, numbered from from so a batch
// sent twice after a lost reply is stored once. The answer says whether the
// person pressed stop.
func (c Client) JobEvents(id string, from int, events []agent.Event) (cancel bool, err error) {
	var out struct {
		Cancel bool `json:"cancel"`
	}
	err = c.do("POST", "/remote/jobs/"+url.PathEscape(id)+"/events", map[string]any{"from": from, "events": events}, &out)
	return out.Cancel, err
}

func (c Client) JobResult(id string, r Result) error {
	return c.do("POST", "/remote/jobs/"+url.PathEscape(id)+"/result", r, nil)
}
