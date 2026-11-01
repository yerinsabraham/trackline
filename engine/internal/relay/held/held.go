// Package held passes a phone message to a chat that is open on this computer.
//
// A second process on an open Claude Code chat reaches its file but never its
// screen, so the message has to come from inside. The runner leaves it here,
// and a background hook that Claude Code wakes the chat for hands it over.
// Measured on 2.1.278: an idle chat woke within two seconds and answered.
//
// That hook runs from the end of one turn until the start of the next, so a
// chat that is working has none, and takes the message when its turn ends.
package held

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Life is how long a message waits: a phone message's signed life.
const Life = 12 * time.Hour

// fresh is how recently a waiting hook must have been seen. It looks every two
// seconds.
const fresh = 10 * time.Second

// name keeps a session id, which came from a hook, out of a path.
func name(session string) string {
	sum := sha256.Sum256([]byte(session))
	return hex.EncodeToString(sum[:16])
}

func path(dir, session, suffix string) string {
	return filepath.Join(dir, "held", name(session)+suffix)
}

// Put leaves a message for a chat.
func Put(dir, session, text string) error {
	p := path(dir, session, ".txt")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Take removes a message and returns it. The rename makes it once: of two hooks
// looking, one gets the file.
func Take(dir, session string, now time.Time) (string, bool) {
	p := path(dir, session, ".txt")
	taken := p + ".taken"
	if os.Rename(p, taken) != nil {
		return "", false
	}
	defer os.Remove(taken)
	fi, err := os.Stat(taken)
	if err != nil || now.Sub(fi.ModTime()) > Life {
		return "", false
	}
	b, err := os.ReadFile(taken)
	return string(b), err == nil && len(b) > 0
}

// Seen records that a hook is waiting for this chat.
func Seen(dir, session string) {
	p := path(dir, session, ".waiting")
	now := time.Now()
	if os.Chtimes(p, now, now) != nil {
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, nil, 0o600)
	}
}

// Waiting reports whether a hook is waiting for this chat right now.
func Waiting(dir, session string, now time.Time) bool {
	fi, err := os.Stat(path(dir, session, ".waiting"))
	return err == nil && now.Sub(fi.ModTime()) < fresh
}

// Started records that a new turn began, which ends the last turn's hook.
func Started(dir, session string) {
	p := path(dir, session, ".turn")
	now := time.Now()
	if os.Chtimes(p, now, now) != nil {
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, nil, 0o600)
	}
}

// StartedSince reports whether a new turn began after t.
func StartedSince(dir, session string, t time.Time) bool {
	fi, err := os.Stat(path(dir, session, ".turn"))
	return err == nil && fi.ModTime().After(t)
}
