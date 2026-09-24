package hosts

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateSite = flag.Bool("update-site", false, "rewrite site/data/hosts.json from this package")

// The website's per-agent pages are built from a copy of this package's table.
// A copy can drift, and a site that claims more than the engine does is the
// exact failure this package exists to prevent, so the copy is checked here.
//
//	go test ./internal/hosts -update-site
func TestSiteCopyIsCurrent(t *testing.T) {
	type entry struct {
		Key                string   `json:"key"`
		Name               string   `json:"name"`
		Blocks             bool     `json:"blocks"`
		ReasonReachesAgent bool     `json:"reasonReachesAgent"`
		Intent             string   `json:"intent"`
		Evidence           string   `json:"evidence"`
		Limits             []string `json:"limits"`
	}
	var out []entry
	for _, n := range Names() {
		c, _ := For(n)
		out = append(out, entry{n, c.Name, c.Blocks, c.ReasonReachesAgent, c.Intent, c.Evidence, c.Limits})
	}
	want, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')

	path := filepath.Join("..", "..", "..", "site", "data", "hosts.json")
	if *updateSite {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the site's copy: %v (run: go test ./internal/hosts -update-site)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("site/data/hosts.json is out of date with this package (run: go test ./internal/hosts -update-site)")
	}
}
