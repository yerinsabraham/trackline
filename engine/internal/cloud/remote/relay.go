package remote

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// RemoteProject is one project the account is told takes remote jobs.
type RemoteProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SetRemoteProjects tells the account about many projects at once, so turning
// on a laptop with twenty is one security email rather than twenty. Unlike
// SetRemoteProject it never lifts a stop made from the site: this runs on its
// own, and only a person at the laptop may undo that.
func (c Client) SetRemoteProjects(ps []RemoteProject) error {
	if len(ps) == 0 {
		return nil
	}
	return c.do("PUT", "/remote/projects", map[string]any{"projects": ps}, nil)
}

func (c Client) UnsetRemoteProject(id string) error {
	return c.do("DELETE", "/remote/projects/"+url.PathEscape(id), nil, nil)
}

// SetPassword tells the account a send password is set here: its id and salt
// only, which a device needs to make the key. The key itself never leaves.
func (c Client) SetPassword(id, salt string, rounds int) error {
	return c.do("PUT", "/remote/password", map[string]any{"id": id, "salt": salt, "rounds": rounds}, nil)
}

func (c Client) UnsetPassword() error { return c.do("DELETE", "/remote/password", nil, nil) }

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
	// Acted says the agent did or said anything, so a refusal it reports cannot
	// be read as nothing having happened. It stays on the laptop.
	Acted bool `json:"-"`
}

// AskAgent files a handoff. job is the run asking: the account takes the sender
// from its own record of that job. It answers with the handoff's number.
func (c Client) AskAgent(job, to, task string) (string, error) {
	var out struct {
		Number int `json:"number"`
	}
	err := c.do("POST", "/remote/handoffs", map[string]any{"job": job, "to": to, "task": task}, &out)
	return strconv.Itoa(out.Number), err
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

// resultBackoff is the longest wait between tries.
var resultBackoff = 8 * time.Second

// JobResult reports how a job ended, a few times over: it is the one message a
// chat cannot do without, and it is sent as a lid closes or a connection drops.
func (c Client) JobResult(id string, r Result) error {
	var err error
	for wait := time.Second; wait <= resultBackoff; wait *= 2 {
		if err = c.do("POST", "/remote/jobs/"+url.PathEscape(id)+"/result", r, nil); err == nil {
			return nil
		}
		// Refused, not lost: asking again would get the same answer.
		var refused *Error
		if errors.As(err, &refused) && refused.Status < 500 {
			return err
		}
		time.Sleep(wait)
	}
	return err
}

// JobImage fetches one image attached to a job, by the hash the job signed.
// Checking the bytes against that hash is the caller's job: this only
// fetches, with a size cap so a hostile server cannot fill the disk.
func (c Client) JobImage(id, sha256 string, max int) ([]byte, error) {
	req, err := http.NewRequest("GET", c.Base+"/remote/jobs/"+url.PathEscape(id)+"/images/"+url.PathEscape(sha256), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+c.Token)
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", c.Base, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, &Error{Status: res.StatusCode, Message: res.Status}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("the image is larger than it was signed as")
	}
	return b, nil
}
