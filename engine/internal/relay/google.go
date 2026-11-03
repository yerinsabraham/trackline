package relay

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// Confirming with Google, for a phone with no passkey paired here: the phone
// signs in with Google, asking for an ID token whose nonce is the job's
// challenge, and that token goes where a passkey's signature would. Google
// signs it, so this laptop checks it with Google's keys and the server that
// carries it still cannot write one. What it rests on is the person's Google
// account, which is what they sign in with anyway.
//
// The Google account it must be is recorded here by a person at this laptop
// (trackline remote enable), never taken from the server later: a server that
// could change it could name its own Google account and confirm its own jobs.

const (
	// GoogleKey marks a job confirmed with Google.
	GoogleKey = "google"
	// AccountKey marks a job the account confirmed, with a code sent to the
	// person's email. Only the server can check that code, so this laptop
	// takes the server's word, and only when a person here allowed it.
	AccountKey = "account"
)

// GoogleClientID is trackline's own Google sign-in client. A token issued to
// any other site is refused, so one made for another app cannot be replayed
// here. TRACKLINE_GOOGLE_CLIENT_ID replaces it for a test deployment.
var GoogleClientID = "266379412699-5t0tvcark8sfd16pp1u83oenq2ads23v.apps.googleusercontent.com"

func init() {
	if id := os.Getenv("TRACKLINE_GOOGLE_CLIENT_ID"); id != "" {
		GoogleClientID = id
	}
}

// GoogleID is the Google account that may confirm jobs here.
type GoogleID struct {
	Sub   string    `json:"sub"`
	Email string    `json:"email,omitempty"`
	SetAt time.Time `json:"setAt"`
}

// GoogleKeys returns Google's signing key with this id. The runner sets it to
// a fetch from Google; the hook never does, as it links no network code, and
// it never needs to: only the runner checks a job.
var GoogleKeys = func(kid string) (*rsa.PublicKey, error) {
	return nil, errors.New("Google's keys are not available in this process")
}

// VerifyGoogle checks that token is a Google ID token for trackline, for the
// Google account sub, whose nonce is want, and that was made around now.
func VerifyGoogle(token, sub string, want []byte, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("not a Google ID token")
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	hb, err := decodeB64(parts[0])
	if err != nil || json.Unmarshal(hb, &head) != nil || head.Alg != "RS256" {
		return errors.New("not a Google ID token")
	}
	sig, err := decodeB64(parts[2])
	if err != nil {
		return errors.New("not a Google ID token")
	}
	pub, err := GoogleKeys(head.Kid)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
		return errors.New("Google's signature does not match")
	}
	var c struct {
		Iss   string `json:"iss"`
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Nonce string `json:"nonce"`
		Iat   int64  `json:"iat"`
	}
	cb, err := decodeB64(parts[1])
	if err != nil || json.Unmarshal(cb, &c) != nil {
		return errors.New("not a Google ID token")
	}
	switch {
	case c.Iss != "accounts.google.com" && c.Iss != "https://accounts.google.com":
		return errors.New("not issued by Google")
	case c.Aud != GoogleClientID:
		return errors.New("issued to another app")
	case c.Sub != sub:
		return errors.New("a different Google account from the one allowed here")
	case c.Nonce != b64.EncodeToString(want):
		return errors.New("made for a different job")
	}
	// Made for this job, so not a sign-in from long ago. The job's own expiry
	// is checked after; a prompt may wait hours for a laptop asleep.
	iat := time.Unix(c.Iat, 0)
	if iat.After(now.Add(Skew)) || now.Sub(iat) > WaitLife+Skew {
		return errors.New("confirmed at a time this job could not have been sent")
	}
	return nil
}
