package main

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/relay"
)

// The runner checks jobs confirmed with Google against Google's own keys. The
// fetch lives here, not in the relay package, because the hook links that
// package and must stay free of network code.
func init() { relay.GoogleKeys = fetchGoogleKeys }

var b64 = base64.RawURLEncoding

const googleCerts = "https://www.googleapis.com/oauth2/v3/certs"

var googleCache struct {
	sync.Mutex
	keys map[string]*rsa.PublicKey
	at   time.Time
}

// fetchGoogleKeys keeps Google's keys for an hour. Google rotates them over
// days, and a token signed with a key not seen yet triggers one fresh fetch.
func fetchGoogleKeys(kid string) (*rsa.PublicKey, error) {
	googleCache.Lock()
	defer googleCache.Unlock()
	if k := googleCache.keys[kid]; k != nil && time.Since(googleCache.at) < time.Hour {
		return k, nil
	}
	c := http.Client{Timeout: 10 * time.Second}
	res, err := c.Get(googleCerts)
	if err != nil {
		return nil, fmt.Errorf("Google's keys could not be fetched: %w", err)
	}
	defer res.Body.Close()
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&set) != nil {
		return nil, errors.New("Google's keys could not be read")
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, err1 := b64.DecodeString(k.N)
		e, err2 := b64.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	googleCache.keys, googleCache.at = keys, time.Now()
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, errors.New("signed with a key Google does not list")
}
