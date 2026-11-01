package held

import (
	"os"
	"testing"
	"time"
)

func TestTakeHandsAMessageOverOnce(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Take(dir, "s", time.Now()); ok {
		t.Fatal("nothing was left, so nothing can be taken")
	}
	if err := Put(dir, "s", "hello"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Take(dir, "other", time.Now()); ok {
		t.Fatal("a message must reach only its own chat")
	}
	got, ok := Take(dir, "s", time.Now())
	if !ok || got != "hello" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := Take(dir, "s", time.Now()); ok {
		t.Fatal("a message must be handed over once")
	}
}

func TestAMessagePastItsLifeIsDropped(t *testing.T) {
	dir := t.TempDir()
	Put(dir, "s", "old")
	if _, ok := Take(dir, "s", time.Now().Add(Life+time.Minute)); ok {
		t.Fatal("a message older than its signed life must not arrive")
	}
}

func TestSessionNeverBecomesAPath(t *testing.T) {
	dir := t.TempDir()
	if err := Put(dir, "../../escape", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/../../escape.txt"); err == nil {
		t.Fatal("a session id must not choose where a file lands")
	}
}

func TestWaitingGoesStale(t *testing.T) {
	dir := t.TempDir()
	if Waiting(dir, "s", time.Now()) {
		t.Fatal("nothing has waited yet")
	}
	Seen(dir, "s")
	if !Waiting(dir, "s", time.Now()) {
		t.Fatal("a hook just looked")
	}
	if Waiting(dir, "s", time.Now().Add(time.Minute)) {
		t.Fatal("a hook unseen for a minute is gone")
	}
}
