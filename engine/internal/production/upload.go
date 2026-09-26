package production

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Upload is a result as it may leave for the account: what trackline
// concluded, never what was said. Requests, rules, replies and tool arguments
// are in Result and deliberately not read here.
func Upload(r Result) payload.ProdConversation {
	c := r.Conversation
	service := c.Service
	if service == "" {
		service = "unnamed service"
	}
	out := payload.ProdConversation{
		ID:               payload.ProdID(service, c.TraceID),
		At:               payload.ProdTime(c.Start),
		Service:          payload.Clean(service, 100),
		Conversation:     payload.Clean(c.ID, 128),
		Tools:            []payload.ProdTool{},
		Findings:         []payload.ProdFinding{},
		Incidents:        []payload.ProdIncident{},
		ModelCalls:       c.ModelCalls,
		Tokens:           c.Tokens,
		ModelLatencyMs:   c.ModelLatency.Milliseconds(),
		ContentAvailable: c.ContentAvailable,
	}

	calls, errs := map[string]int{}, map[string]int{}
	for _, a := range c.Actions {
		calls[a.Action.ToolName]++
	}
	for _, name := range c.ToolErrors {
		errs[name]++
	}
	names := make([]string, 0, len(calls))
	for n := range calls {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if len(out.Tools) == 50 {
			break
		}
		label := n
		if label == "" {
			label = "unknown tool"
		}
		out.Tools = append(out.Tools, payload.ProdTool{Name: payload.Clean(label, 100), Calls: calls[n], Errors: errs[n]})
	}

	for _, v := range r.Findings() {
		if len(out.Findings) == 50 {
			break
		}
		out.Findings = append(out.Findings, payload.ProdFinding{
			Check: v.Signal, Severity: string(v.Severity),
			Summary: payload.Clean(v.Summary, 300), Tool: payload.Clean(v.Target, 100),
		})
	}
	for _, i := range r.Incidents {
		if len(out.Incidents) == 20 {
			break
		}
		out.Incidents = append(out.Incidents, payload.ProdIncident{Kind: i.Kind, Summary: payload.Clean(i.Summary, 300)})
	}
	// An unchecked conversation must not look like a clean one.
	for _, a := range r.Actions {
		for _, res := range a.Results {
			if res.Outcome == verdict.OutcomeCannotMeasure {
				out.Unmeasured = addUnmeasured(out.Unmeasured, res)
			}
		}
	}
	for _, res := range r.Unmeasured {
		out.Unmeasured = addUnmeasured(out.Unmeasured, res)
	}
	return out
}

func addUnmeasured(list []payload.ProdUnmeasured, res verdict.Result) []payload.ProdUnmeasured {
	for _, u := range list {
		if u.Check == res.Signal {
			return list
		}
	}
	if len(list) == 20 {
		return list
	}
	return append(list, payload.ProdUnmeasured{Check: payload.Clean(res.Signal, 40), Reason: payload.Clean(res.Reason, 300)})
}

// Sender posts a batch to the account.
type Sender interface {
	Production(payload.ProdBatch) error
}

// Uploader sends results in the background, in batches, and keeps trying
// through an outage. The receiver never waits on it: an agent's traces keep
// being checked whether or not the account can be reached.
type Uploader struct {
	Send Sender
	// Max is how many conversations wait at most; the oldest go first. A
	// service that is down for a day should not fill the memory of the
	// process watching it.
	Max int

	mu      sync.Mutex
	queue   []payload.ProdConversation
	dropped int
	sent    int
	lastErr error
	wake    chan struct{}
}

// NewUploader builds an uploader. Run starts it.
func NewUploader(s Sender) *Uploader {
	return &Uploader{Send: s, Max: 5000, wake: make(chan struct{}, 1)}
}

// Add queues one result.
func (u *Uploader) Add(r Result) {
	u.mu.Lock()
	u.queue = append(u.queue, Upload(r))
	if over := len(u.queue) - u.Max; over > 0 {
		u.queue = u.queue[over:]
		u.dropped += over
	}
	full := len(u.queue) >= batchSize
	u.mu.Unlock()
	if full {
		select {
		case u.wake <- struct{}{}:
		default:
		}
	}
}

const batchSize = 100

// Run sends until stop is closed, then sends what is left once.
func (u *Uploader) Run(stop <-chan struct{}, every time.Duration) {
	backoff := every
	t := time.NewTimer(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			u.Flush()
			return
		case <-u.wake:
		case <-t.C:
		}
		if err := u.Flush(); err != nil {
			backoff = min(backoff*2, 5*time.Minute)
		} else {
			backoff = every
		}
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(backoff)
	}
}

// Flush sends everything queued, a batch at a time, and stops at the first
// failure so nothing is lost or sent out of order.
func (u *Uploader) Flush() error {
	for {
		u.mu.Lock()
		n := min(len(u.queue), batchSize)
		batch := append([]payload.ProdConversation(nil), u.queue[:n]...)
		u.mu.Unlock()
		if n == 0 {
			return nil
		}
		err := u.Send.Production(payload.ProdBatch{V: 1, Conversations: batch})
		// Refused as malformed is not going to succeed on a retry, and must
		// not hold up everything behind it. Anything else is worth retrying.
		var st interface{ HTTPStatus() int }
		refused := err != nil && errors.As(err, &st) && st.HTTPStatus() >= 400 && st.HTTPStatus() < 500 && st.HTTPStatus() != 401 && st.HTTPStatus() != 429
		if err != nil && !refused {
			u.mu.Lock()
			u.lastErr = err
			u.mu.Unlock()
			return err
		}
		done := make(map[string]bool, n)
		for _, c := range batch {
			done[c.ID] = true
		}
		u.mu.Lock()
		// By id, not position: Add may have dropped the oldest while this
		// batch was in flight.
		kept := u.queue[:0]
		for _, c := range u.queue {
			if !done[c.ID] {
				kept = append(kept, c)
			}
		}
		u.queue = kept
		if refused {
			u.dropped += n
			u.lastErr = err
		} else {
			u.sent += n
			u.lastErr = nil
		}
		u.mu.Unlock()
	}
}

// Stats says how the upload is going.
func (u *Uploader) Stats() (sent, waiting, dropped int, lastErr error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.sent, len(u.queue), u.dropped, u.lastErr
}
