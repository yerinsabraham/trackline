package relay

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// A send password is for a device that holds no passkey paired here: a
// borrowed laptop, a browser without passkeys, a phone without Face ID. It is
// set on this laptop and checked by this laptop, like a passkey, so the server
// that carries a job still cannot write one.
//
// What it gives up is said plainly where it is set: a job signed with it
// passes through the server, and a server that turned on its user could try
// passwords against one offline. Rounds make each guess slow and MinPassword
// makes the space large, and that is the whole defence, which is why it is
// off unless asked for. A guess sent as a job instead is stopped by
// MaxPasswordFails.

const (
	passwordPurpose = "trackline send v1\n"
	// PasswordRounds matches PairRounds: a phone spends under a second, once
	// per visit, and an offline search pays the same for every guess.
	PasswordRounds = 600_000
	// MinPassword is in characters. Twelve of anything is beyond an offline
	// search at these rounds; a short one is not, whatever it contains.
	MinPassword = 12
	// MaxPasswordFails wrong passwords in a row turn it off until it is set
	// again here. A hostile server can use this to turn it off, which costs
	// a person a step at the laptop and nothing else.
	MaxPasswordFails = 5
	// PasswordPrefix marks a job signed with the send password, where a
	// passkey's credential id would otherwise be.
	PasswordPrefix = "pw_"
)

// Password is the send password as this laptop keeps it: the slow key made
// from it, never the password itself.
type Password struct {
	ID       string    `json:"id"`
	Salt     string    `json:"salt"`
	Key      string    `json:"key"`
	SetAt    time.Time `json:"setAt"`
	Fails    int       `json:"fails,omitempty"`
	LockedAt time.Time `json:"lockedAt,omitzero"`
}

// ErrShortPassword is a password an offline search could find.
var ErrShortPassword = errors.New("a send password needs at least 12 characters")

// NewPassword makes the key this laptop checks jobs against. The id and salt
// go to the account, so a device can find them; the key stays here.
func NewPassword(deviceID, password string, now time.Time) (Password, error) {
	if utf8.RuneCountInString(password) < MinPassword || strings.TrimSpace(password) == "" {
		return Password{}, ErrShortPassword
	}
	id, salt := make([]byte, 12), make([]byte, 16)
	rand.Read(id)
	rand.Read(salt)
	p := Password{ID: PasswordPrefix + b64.EncodeToString(id), Salt: b64.EncodeToString(salt), SetAt: now}
	p.Key = b64.EncodeToString(passwordKey(deviceID, p.Salt, password))
	return p, nil
}

// passwordKey is the slow step, done the same way in the browser: PBKDF2 over
// the purpose, this laptop's id and the salt, so a key made for one laptop or
// one setting is useless for another.
func passwordKey(deviceID, salt, password string) []byte {
	k, err := pbkdf2.Key(sha256.New, password, []byte(passwordPurpose+deviceID+"\n"+salt), PasswordRounds, 32)
	if err != nil {
		// Only reachable with invalid parameters, which these are not.
		panic(err)
	}
	return k
}

// PasswordMAC is what a device sends for a job: the job's challenge, keyed
// with the slow key.
func PasswordMAC(key, job []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(JobChallenge(job))
	return m.Sum(nil)
}

// checkPassword is the password half of Check. A wrong one counts towards
// turning it off; a right one starts the count again.
func (s *State) checkPassword(env Envelope, raw []byte, now time.Time) *Refusal {
	p := s.Password
	if p == nil || p.ID != env.Key {
		return refuse("unknown-key", "signed with a send password this laptop does not have; set one here: trackline remote password")
	}
	if !p.LockedAt.IsZero() {
		return refuse("password-locked", "the send password was turned off after %d wrong tries; set it again here: trackline remote password", MaxPasswordFails)
	}
	key, err1 := decodeB64(p.Key)
	got, err2 := decodeB64(env.Signature)
	if err1 != nil || err2 != nil || !hmac.Equal(got, PasswordMAC(key, raw)) {
		p.Fails++
		if p.Fails >= MaxPasswordFails {
			p.LockedAt = now
		}
		return refuse("bad-password", "wrong send password")
	}
	p.Fails = 0
	return nil
}
