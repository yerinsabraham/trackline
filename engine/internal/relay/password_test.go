package relay

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

// The site makes the same key and MAC in the browser (site/lib/remote.ts,
// passwordMAC). If either side changes alone, every password job is refused,
// so both are pinned to this value.
func TestPasswordMACVector(t *testing.T) {
	key := passwordKey("dev_example", "c2FsdHNhbHRzYWx0c2FsdA", "correct horse battery")
	got := hex.EncodeToString(PasswordMAC(key, []byte(`{"v":1}`)))
	const want = "ad9aefb78a51d0f5028569e2a9a02aaf63c64d9ae89666a2a2d2aa9caeda690d"
	if got != want {
		t.Fatalf("PasswordMAC = %s", got)
	}
}

func TestShortPasswordsAreRefused(t *testing.T) {
	for _, pw := range []string{"", "short", "elevenchars", "            "} {
		if _, err := NewPassword("dev", pw, time.Now()); !errors.Is(err, ErrShortPassword) {
			t.Errorf("%q: err = %v", pw, err)
		}
	}
	p, err := NewPassword("dev", "twelve chars", time.Now())
	if err != nil || p.Key == "" || p.Salt == "" || len(p.ID) < 10 || p.ID[:3] != PasswordPrefix {
		t.Fatalf("p = %+v, err = %v", p, err)
	}
	q, _ := NewPassword("dev", "twelve chars", time.Now())
	if q.Salt == p.Salt || q.Key == p.Key || q.ID == p.ID {
		t.Fatal("the same password twice made the same key")
	}
}
