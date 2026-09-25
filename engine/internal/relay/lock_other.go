//go:build !unix

package relay

import (
	"errors"
	"os"
	"time"
)

// lock without flock: create the file exclusively, and treat one older than
// a minute as left behind by a process that died holding it. Nothing holds
// the lock for more than a file write.
func lock(path string) (func(), error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > time.Minute {
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, errors.New("the remote settings are locked by another trackline process")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
