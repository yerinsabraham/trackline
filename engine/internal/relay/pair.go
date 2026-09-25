package relay

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Pairing is how the laptop learns which passkeys may send it jobs, without
// taking the server's word for it.
//
// The laptop shows a code. The person types it on their phone, and the phone
// signs the code with a passkey. The laptop checks that signature against the
// key it was given: only someone who could see this laptop's screen could
// have signed that code, so a key substituted on the way (by a compromised
// server) fails. It is the same idea as pairing a Bluetooth keyboard.
//
// The laptop takes the first answer only. A server trying keys of its own
// gets one guess at a code with 40 bits in it.

// codeAlphabet is Crockford's base32: no I, L, O or U, so the code survives
// being read off one screen and typed on another.
const codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// CodeLength characters of 5 bits each: 40 bits.
const CodeLength = 8

// NewCode makes a pairing code.
func NewCode() (string, error) {
	b := make([]byte, CodeLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = codeAlphabet[b[i]&31]
	}
	return string(b), nil
}

// ShowCode splits a code in two for reading aloud or across a room.
func ShowCode(code string) string {
	return code[:CodeLength/2] + "-" + code[CodeLength/2:]
}

// NormalizeCode reads a code as a person may type it: lower case, with a
// hyphen or spaces, and O for 0 or I and L for 1. The site does the same
// before signing.
func NormalizeCode(s string) string {
	s = strings.ToUpper(s)
	s = strings.NewReplacer("-", "", " ", "", "O", "0", "I", "1", "L", "1").Replace(s)
	return s
}

// PairAnswer is what the phone sends back through the server.
type PairAnswer struct {
	Key struct {
		ID        string `json:"id"`
		PublicKey string `json:"publicKey"`
		Name      string `json:"name"`
	} `json:"key"`
	Assertion
}

// VerifyPair checks that the answer was signed by its key over this laptop's
// code, and returns the key to trust.
func VerifyPair(rp RP, deviceID, code string, a PairAnswer, now time.Time) (Key, error) {
	if a.Key.ID == "" {
		return Key{}, errors.New("the answer names no passkey")
	}
	pub, err := ParseKey(a.Key.PublicKey)
	if err != nil {
		return Key{}, err
	}
	if err := verifyAssertion(pub, rp, a.Assertion, PairChallenge(deviceID, code)); err != nil {
		return Key{}, fmt.Errorf("the phone did not sign this laptop's code: %w", err)
	}
	name := strings.TrimSpace(a.Key.Name)
	if name == "" {
		name = "a passkey"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	return Key{ID: a.Key.ID, PublicKey: a.Key.PublicKey, Name: name, PairedAt: now}, nil
}
