// Package outbox holds what a connected machine has yet to upload, and sends
// it.
//
// The hook writes; `trackline sync` sends. They never wait on each other: the
// hook runs before every tool call and must not pay for a network, so all it
// does is drop one file here and, if nobody is sending, start someone.
//
// One file per event, written to a temporary name and renamed into place. A
// rename is atomic, so parallel hooks never interleave and a half-written event
// is never read. Names start with the time, so sorting them is sending in order.
// A file is deleted only after the server has accepted it, and every event
// carries an id the server deduplicates on, so a sync killed at any moment
// loses nothing and doubles nothing.
package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
)

const (
	// A running sync refreshes the lock after every batch. One older than this
	// belongs to a sync that died.
	staleLock = 2 * time.Minute
	// Offline for weeks should not fill a disk. The oldest go first.
	maxPending = 20000
	// The server's body limit is 1MB; this leaves room.
	maxBatchBytes = 512 << 10
	// Waits after a failure: 30s, then doubling, never more than 15 minutes.
	firstBackoff = 30 * time.Second
	maxBackoff   = 15 * time.Minute
)

// Box is an outbox in a directory, normally the account directory.
type Box struct{ Dir string }

func (b Box) pending() string { return filepath.Join(b.Dir, "outbox") }
func (b Box) lock() string    { return filepath.Join(b.Dir, "sync.lock") }
func (b Box) backoff() string { return filepath.Join(b.Dir, "sync.backoff") }

// NewID is a fresh event id.
func NewID() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return "ev_" + hex.EncodeToString(buf)
}

// Put queues one event.
func (b Box) Put(e payload.Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	dir := b.pending()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Zero-padded nanoseconds, so a name sort is a time sort.
	name := fmt.Sprintf("%020d-%s.json", time.Now().UnixNano(), e.ID)
	tmp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

// PutReply queues an agent's reply. Replies share the queue with events, so
// they leave in the order things happened.
func (b Box) PutReply(r payload.Reply) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	dir := b.pending()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%s%s", time.Now().UnixNano(), r.ID, replySuffix)
	tmp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

const replySuffix = ".reply.json"

// Pending lists queued files, oldest first.
func (b Box) Pending() ([]string, error) {
	entries, err := os.ReadDir(b.pending())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".json") && !strings.HasPrefix(n, ".") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

// NeedsSender says whether the hook should start a sync: nobody is sending,
// and the last failure is not still being waited out. Two stats, no reads of
// any size, because the hook pays for this on every tool call.
func (b Box) NeedsSender(now time.Time) bool {
	if fi, err := os.Stat(b.lock()); err == nil && now.Sub(fi.ModTime()) < staleLock {
		return false
	}
	if until, ok := b.backoffUntil(); ok && now.Before(until) {
		return false
	}
	return true
}

func (b Box) backoffUntil() (time.Time, bool) {
	fi, err := os.Stat(b.backoff())
	if err != nil {
		return time.Time{}, false
	}
	return fi.ModTime().Add(b.backoffDelay()), true
}

func (b Box) backoffDelay() time.Duration {
	raw, err := os.ReadFile(b.backoff())
	if err != nil {
		return firstBackoff
	}
	d, err := time.ParseDuration(strings.TrimSpace(string(raw)))
	if err != nil || d < firstBackoff {
		return firstBackoff
	}
	return d
}

func (b Box) acquire(now time.Time) bool {
	if err := os.MkdirAll(b.Dir, 0o700); err != nil {
		return false
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(b.lock(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprint(f, os.Getpid())
			f.Close()
			return true
		}
		fi, serr := os.Stat(b.lock())
		if serr != nil || now.Sub(fi.ModTime()) < staleLock {
			return false
		}
		os.Remove(b.lock())
	}
	return false
}

func (b Box) touch()   { now := time.Now(); os.Chtimes(b.lock(), now, now) }
func (b Box) release() { os.Remove(b.lock()) }

// Sender is where batches go. remote.Client satisfies it.
type Sender interface {
	Ingest(payload.Batch) error
}

// A Sender's error says what the server answered by having an HTTPStatus
// method. An error without one means the server was not reached.
type statusError interface{ HTTPStatus() int }

func status(err error) int {
	var se statusError
	if errors.As(err, &se) {
		return se.HTTPStatus()
	}
	return 0
}

// Result is what one Drain did.
type Result struct {
	Sent, Rejected, Dropped int
	// Revoked means the server no longer knows this machine. Its queue was
	// discarded: nobody is there to receive it.
	Revoked bool
	// Waiting means sending stopped on a failure worth retrying, and the next
	// attempt is held back until the backoff passes.
	Waiting bool
}

// Drain sends everything queued, then returns. If another sync holds the
// lock, it returns at once and sends nothing.
func (b Box) Drain(s Sender) (Result, error) { return b.DrainAfter(s, 0) }

// DrainAfter waits before sending, holding the lock while it waits, so the
// hooks that run meanwhile queue into this send rather than each starting a
// sender of their own. Measured without the lock held: 210 events went up in
// 120 requests.
func (b Box) DrainAfter(s Sender, wait time.Duration) (Result, error) {
	var res Result
	if !b.acquire(time.Now()) {
		return res, nil
	}
	time.Sleep(wait)
	for {
		names, err := b.Pending()
		if err != nil {
			b.release()
			return res, err
		}
		if len(names) == 0 {
			// Released before a last look, never after. A hook that queues
			// between the two either is seen by that look, or finds no lock
			// and starts a sync of its own. Either way nothing waits for the
			// next tool call.
			b.release()
			if names, _ = b.Pending(); len(names) == 0 || !b.acquire(time.Now()) {
				return res, nil
			}
			continue
		}
		if over := len(names) - maxPending; over > 0 {
			for _, n := range names[:over] {
				os.Remove(filepath.Join(b.pending(), n))
			}
			res.Dropped += over
			names = names[over:]
		}

		batch, used := b.read(names)
		if len(used) == 0 {
			// Nothing in reach was readable. Corrupt files were removed; one
			// that cannot even be opened is left for the next sync rather
			// than spun on here.
			b.release()
			return res, nil
		}
		err = s.Ingest(batch)
		code := status(err)
		switch {
		case err == nil:
			b.remove(used)
			res.Sent += len(used)
			os.Remove(b.backoff())
			b.touch()
		case code == 401:
			os.RemoveAll(b.pending())
			res.Revoked = true
			b.release()
			return res, nil
		case code == 400 || code == 413 || code == 422:
			// The server read the batch and will never take it. Retrying
			// forever would stop everything behind it, so it is set aside
			// where a person can look.
			b.setAside(used)
			res.Rejected += len(used)
			b.touch()
		default:
			b.wait()
			res.Waiting = true
			b.release()
			return res, err
		}
	}
}

func (b Box) read(names []string) (payload.Batch, []string) {
	batch := payload.Batch{V: payload.Version}
	var used []string
	size := 0
	for _, n := range names {
		if len(batch.Events) == 100 || len(batch.Replies) == 100 {
			break
		}
		path := filepath.Join(b.pending(), n)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if size+len(raw) > maxBatchBytes && len(used) > 0 {
			break
		}
		if strings.HasSuffix(n, replySuffix) {
			var r payload.Reply
			if json.Unmarshal(raw, &r) != nil {
				os.Remove(path)
				continue
			}
			batch.Replies = append(batch.Replies, r)
		} else {
			var e payload.Event
			if json.Unmarshal(raw, &e) != nil {
				os.Remove(path)
				continue
			}
			batch.Events = append(batch.Events, e)
		}
		size += len(raw)
		used = append(used, n)
	}
	if batch.Events == nil {
		batch.Events = []payload.Event{}
	}
	return batch, used
}

func (b Box) remove(names []string) {
	for _, n := range names {
		os.Remove(filepath.Join(b.pending(), n))
	}
}

func (b Box) setAside(names []string) {
	dir := filepath.Join(b.Dir, "rejected")
	os.MkdirAll(dir, 0o700)
	for _, n := range names {
		os.Rename(filepath.Join(b.pending(), n), filepath.Join(dir, n))
	}
}

func (b Box) wait() {
	next := firstBackoff
	if _, err := os.Stat(b.backoff()); err == nil {
		next = b.backoffDelay() * 2
	}
	if next > maxBackoff {
		next = maxBackoff
	}
	os.WriteFile(b.backoff(), []byte(next.String()), 0o600)
	now := time.Now()
	os.Chtimes(b.backoff(), now, now)
}

// Counts reports what is queued and set aside, for `trackline account`.
func (b Box) Counts() (pending, rejected int) {
	names, _ := b.Pending()
	entries, _ := os.ReadDir(filepath.Join(b.Dir, "rejected"))
	return len(names), len(entries)
}

// WaitingUntil is when the next attempt may happen, if one is being held back.
func (b Box) WaitingUntil() (time.Time, bool) {
	until, ok := b.backoffUntil()
	if !ok || time.Now().After(until) {
		return time.Time{}, false
	}
	return until, true
}
