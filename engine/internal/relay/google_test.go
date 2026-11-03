package relay_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/relay"
)

var googleKey, _ = rsa.GenerateKey(rand.Reader, 2048)

// googleToken is an ID token as Google would sign it, with over replacing claims.
func googleToken(t *testing.T, key *rsa.PrivateKey, raw []byte, over map[string]any) string {
	t.Helper()
	enc := base64.RawURLEncoding
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1"})
	claims := map[string]any{
		"iss": "https://accounts.google.com", "aud": relay.GoogleClientID, "sub": "g-123",
		"nonce": enc.EncodeToString(relay.JobChallenge(raw)), "iat": now.Unix(), "email": "person@example.com",
	}
	for k, v := range over {
		claims[k] = v
	}
	body, _ := json.Marshal(claims)
	signed := enc.EncodeToString(head) + "." + enc.EncodeToString(body)
	sum := sha256.Sum256([]byte(signed))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	return signed + "." + enc.EncodeToString(sig)
}

func googleEnvelope(t *testing.T, j relay.Job, key *rsa.PrivateKey, over map[string]any) relay.Envelope {
	raw, _ := json.Marshal(j)
	return relay.Envelope{
		Job: base64.RawURLEncoding.EncodeToString(raw), Key: relay.GoogleKey,
		Assertion: relay.Assertion{Signature: googleToken(t, key, raw, over)},
	}
}

func TestConfirmedWithGoogle(t *testing.T) {
	was := relay.GoogleKeys
	relay.GoogleKeys = func(kid string) (*rsa.PublicKey, error) {
		if kid != "k1" {
			return nil, errors.New("unknown key")
		}
		return &googleKey.PublicKey, nil
	}
	t.Cleanup(func() { relay.GoogleKeys = was })
	other, _ := rsa.GenerateKey(rand.Reader, 2048)

	f := setup(t)
	if c := code(relaySend(f, googleEnvelope(t, job(), googleKey, nil))); c != "unknown-key" {
		t.Fatalf("Google not allowed here, yet: %s", c)
	}
	f.state.Google = &relay.GoogleID{Sub: "g-123"}
	n := 0
	id := func(j *relay.Job) { n++; j.ID = "job_g" + string(rune('a'+n)) }
	if c := code(relaySend(f, googleEnvelope(t, job(id), googleKey, nil))); c != "accepted" {
		t.Fatalf("a good Google confirmation: %s", c)
	}
	for name, env := range map[string]relay.Envelope{
		"signed by someone else":   googleEnvelope(t, job(id), other, nil),
		"another Google account":   googleEnvelope(t, job(id), googleKey, map[string]any{"sub": "g-999"}),
		"another app's token":      googleEnvelope(t, job(id), googleKey, map[string]any{"aud": "someone-else.apps.googleusercontent.com"}),
		"not from Google":          googleEnvelope(t, job(id), googleKey, map[string]any{"iss": "https://evil.example"}),
		"made for a different job": googleEnvelope(t, job(id), googleKey, map[string]any{"nonce": "c29tZXRoaW5nIGVsc2U"}),
		"a sign-in from a day ago": googleEnvelope(t, job(id), googleKey, map[string]any{"iat": now.Add(-24 * time.Hour).Unix()}),
		"not a token":              {Job: googleEnvelope(t, job(id), googleKey, nil).Job, Key: relay.GoogleKey, Assertion: relay.Assertion{Signature: "nope"}},
	} {
		if c := code(relaySend(f, env)); c != "bad-signature" {
			t.Errorf("%s: %s", name, c)
		}
	}
	// A token for one job cannot carry another, even one just like it.
	env := googleEnvelope(t, job(id), googleKey, nil)
	altered, _ := json.Marshal(job(id, func(j *relay.Job) { j.Text = "rm -rf ~" }))
	env.Job = base64.RawURLEncoding.EncodeToString(altered)
	if c := code(relaySend(f, env)); c != "bad-signature" {
		t.Errorf("altered job: %s", c)
	}
}

// The account's word is taken only where a person here allowed it.
func TestConfirmedByTheAccount(t *testing.T) {
	f := setup(t)
	raw, _ := json.Marshal(job())
	env := relay.Envelope{Job: base64.RawURLEncoding.EncodeToString(raw), Key: relay.AccountKey}
	if c := code(relaySend(f, env)); c != "unknown-key" {
		t.Fatalf("email codes not allowed, yet: %s", c)
	}
	f.state.AccountCodes = true
	if c := code(relaySend(f, env)); c != "accepted" {
		t.Fatalf("email codes allowed: %s", c)
	}
}
