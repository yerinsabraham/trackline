package outbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
)

// server stands in for the backend: it keeps what it accepts, deduplicated on
// the event id the way the real one is, and fails on command.
type server struct {
	mu       sync.Mutex
	stored   []string
	seen     map[string]bool
	batches  []int
	failWith func(call int, b payload.Batch) error
	calls    int
}

type httpErr int

func (e httpErr) Error() string   { return fmt.Sprintf("status %d", int(e)) }
func (e httpErr) HTTPStatus() int { return int(e) }

func (s *server) Ingest(b payload.Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	// An outbox that never empties would resend forever. Fail fast rather
	// than grow until the machine runs out of memory, which one did.
	if s.calls > 5000 {
		panic("the outbox kept resending: nothing was removed after a success")
	}
	if s.failWith != nil {
		if err := s.failWith(s.calls, b); err != nil {
			return err
		}
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	s.batches = append(s.batches, len(b.Events))
	for _, e := range b.Events {
		if !s.seen[e.ID] {
			s.seen[e.ID] = true
			s.stored = append(s.stored, e.ID)
		}
	}
	return nil
}

func queue(t *testing.T, b Box, n int) []string {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("ev_%04d", i)
		if err := b.Put(payload.Event{ID: id, Host: "claude-code"}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func same(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
}

func TestSendsEverythingInOrderAndEmptiesTheOutbox(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	ids := queue(t, b, 250)
	s := &server{}
	res, err := b.Drain(s)
	if err != nil || res.Sent != 250 {
		t.Fatalf("sent %d, err %v", res.Sent, err)
	}
	if !same(s.stored, ids) {
		t.Error("events arrived out of order")
	}
	if left, _ := b.Pending(); len(left) != 0 {
		t.Errorf("%d left in the outbox", len(left))
	}
	for _, n := range s.batches {
		if n > 100 {
			t.Errorf("a batch of %d, over the contract's 100", n)
		}
	}
	if _, err := os.Stat(b.lock()); !os.IsNotExist(err) {
		t.Error("the lock was not released")
	}
}

// Offline: nothing is lost, the next attempt is held back rather than retried
// on every tool call, and once the network is back everything goes, in order,
// once.
func TestOfflineEventsWaitAndArriveLaterOnce(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	ids := queue(t, b, 30)
	offline := true
	s := &server{failWith: func(int, payload.Batch) error {
		if offline {
			return errors.New("dial tcp: no route to host")
		}
		return nil
	}}
	res, err := b.Drain(s)
	if err == nil || !res.Waiting || res.Sent != 0 {
		t.Fatalf("offline drain: %+v %v", res, err)
	}
	if left, _ := b.Pending(); len(left) != 30 {
		t.Fatalf("offline lost events: %d left of 30", len(left))
	}
	if b.NeedsSender(time.Now()) {
		t.Error("the hook would start a sender straight after a failure")
	}

	// A second failure doubles the wait.
	past := time.Now().Add(-time.Hour)
	os.Chtimes(b.backoff(), past, past)
	b.Drain(s)
	if d := b.backoffDelay(); d != 2*firstBackoff {
		t.Errorf("backoff after two failures %v, want %v", d, 2*firstBackoff)
	}

	offline = false
	os.Chtimes(b.backoff(), past, past)
	if !b.NeedsSender(time.Now()) {
		t.Fatal("the hook would not start a sender once the wait is over")
	}
	if res, err := b.Drain(s); err != nil || res.Sent != 30 {
		t.Fatalf("back online: %+v %v", res, err)
	}
	if !same(s.stored, ids) {
		t.Error("events after an outage arrived out of order or more than once")
	}
	if _, err := os.Stat(b.backoff()); !os.IsNotExist(err) {
		t.Error("the backoff outlived a success")
	}
}

// The worst moment to die: the server has stored the batch, and the machine
// never hears so. The files are still there, so they go again, and the
// server's dedup on the event id means they count once.
func TestKilledMidBatchLosesNothingAndDoublesNothing(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	ids := queue(t, b, 150)
	s := &server{}
	cut := true
	s.failWith = func(call int, batch payload.Batch) error {
		if call == 2 && cut {
			cut = false
			// Stored, then the connection drops before the answer.
			for _, e := range batch.Events {
				if !s.seen[e.ID] {
					s.seen[e.ID] = true
					s.stored = append(s.stored, e.ID)
				}
			}
			return errors.New("connection reset by peer")
		}
		return nil
	}
	if _, err := b.Drain(s); err == nil {
		t.Fatal("the cut connection was not reported")
	}
	os.Remove(b.backoff())
	if _, err := b.Drain(s); err != nil {
		t.Fatal(err)
	}
	if !same(s.stored, ids) {
		t.Errorf("stored %d, want each of %d exactly once and in order", len(s.stored), len(ids))
	}
}

// A process killed while holding the lock must not stop sending for good.
func TestAStaleLockIsTakenOverAndAFreshOneRespected(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	queue(t, b, 3)
	os.WriteFile(b.lock(), []byte("12345"), 0o600)

	s := &server{}
	if res, _ := b.Drain(s); res.Sent != 0 {
		t.Error("sent while another sync held a fresh lock")
	}
	if b.NeedsSender(time.Now()) {
		t.Error("the hook would start a second sender beside a running one")
	}

	old := time.Now().Add(-staleLock - time.Second)
	os.Chtimes(b.lock(), old, old)
	if !b.NeedsSender(time.Now()) {
		t.Error("the hook would never replace a dead sender")
	}
	if res, _ := b.Drain(s); res.Sent != 3 {
		t.Errorf("sent %d after taking over a stale lock, want 3", res.Sent)
	}
}

// A revoked machine has nobody to send to. Its queue goes, and nothing is
// retried.
func TestARevokedMachineDropsItsQueue(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	queue(t, b, 5)
	s := &server{failWith: func(int, payload.Batch) error { return httpErr(401) }}
	res, err := b.Drain(s)
	if err != nil || !res.Revoked {
		t.Fatalf("%+v %v", res, err)
	}
	if left, _ := b.Pending(); len(left) != 0 {
		t.Error("a revoked machine kept its queue")
	}
	if s.calls != 1 {
		t.Errorf("%d attempts after a 401, want 1", s.calls)
	}
}

// A batch the server reads and refuses would be refused forever. It is set
// aside, and everything behind it still goes.
func TestARefusedBatchIsSetAsideAndTheRestStillGo(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	ids := queue(t, b, 120)
	s := &server{failWith: func(call int, _ payload.Batch) error {
		if call == 1 {
			return httpErr(400)
		}
		return nil
	}}
	res, err := b.Drain(s)
	if err != nil || res.Rejected != 100 || res.Sent != 20 {
		t.Fatalf("%+v %v", res, err)
	}
	if !same(s.stored, ids[100:]) {
		t.Error("the events behind a refused batch were not sent")
	}
	if entries, _ := os.ReadDir(filepath.Join(b.Dir, "rejected")); len(entries) != 100 {
		t.Errorf("%d set aside, want 100", len(entries))
	}
}

// Events big enough that the byte limit, not the count of 100, is what
// splits the batches. With small events the byte limit is never reached, and
// a test of it proves nothing.
func TestBatchesStayUnderTheBodyLimit(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	big := strings.Repeat("x", 20000)
	for i := 0; i < 100; i++ {
		b.Put(payload.Event{ID: fmt.Sprintf("ev_%04d", i), Request: big, Host: "codex"})
	}
	s := &server{}
	if res, _ := b.Drain(s); res.Sent != 100 {
		t.Fatalf("sent %d of 100", res.Sent)
	}
	for _, n := range s.batches {
		if n*20000 > maxBatchBytes {
			t.Errorf("a batch of %d events of 20KB is over the %d byte limit", n, maxBatchBytes)
		}
	}
}

// Parallel tool calls run parallel hooks. Every event lands whole.
func TestParallelHooksNeverInterleave(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b.Put(payload.Event{ID: fmt.Sprintf("ev_%02d", i), Request: strings.Repeat("r", 4000)})
		}(i)
	}
	wg.Wait()
	s := &server{}
	res, _ := b.Drain(s)
	if res.Sent != 64 || len(s.stored) != 64 {
		t.Errorf("sent %d, stored %d, want 64", res.Sent, len(s.stored))
	}
}

// Found between a sync's last look and its exit: sent by that sync, or by a
// new one the hook starts. Never left for the next tool call.
func TestAnEventQueuedAsTheSenderFinishesIsNotStranded(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	queue(t, b, 1)
	s := &server{}
	late := false
	s.failWith = func(int, payload.Batch) error {
		if !late {
			late = true
			// Arrives while this batch is in flight.
			b.Put(payload.Event{ID: "ev_late"})
		}
		return nil
	}
	b.Drain(s)
	if !s.seen["ev_late"] {
		t.Error("an event queued during the last batch was left behind")
	}
}

func TestIDsAreUniqueAndFitTheContract(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if seen[id] || len(id) > 64 {
			t.Fatalf("id %q repeated or too long", id)
		}
		seen[id] = true
	}
}

// While a sender gathers, it holds the lock, so hooks queue into its batch
// instead of starting senders of their own.
func TestASenderHoldsTheLockWhileItGathers(t *testing.T) {
	b := Box{Dir: t.TempDir()}
	queue(t, b, 1)
	done := make(chan struct{})
	s := &server{}
	go func() { b.DrainAfter(s, 300*time.Millisecond); close(done) }()
	time.Sleep(50 * time.Millisecond)
	if b.NeedsSender(time.Now()) {
		t.Error("a hook would start a second sender while the first gathers")
	}
	b.Put(payload.Event{ID: "ev_during"})
	<-done
	if len(s.batches) != 1 || s.batches[0] != 2 {
		t.Errorf("batches %v, want one batch of 2", s.batches)
	}
}
