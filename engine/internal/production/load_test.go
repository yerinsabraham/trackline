package production_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/otlp"
	"github.com/yerinsabraham/trackline/engine/internal/production"
)

// Production is not a laptop. The captured stream, cloned into many
// conversations with fresh trace ids, run through the receiver.
//
//	TRACKLINE_LOAD=20000 go test ./internal/production -run Load -v
//
// Skipped by default: it is a measurement, not a check that should gate CI.
func TestLoad(t *testing.T) {
	n := 0
	fmt.Sscan(os.Getenv("TRACKLINE_LOAD"), &n)
	if n == 0 {
		t.Skip("set TRACKLINE_LOAD to the number of conversations")
	}

	files, _ := filepath.Glob(filepath.Join("..", "otlp", "testdata", "*.pb"))
	sort.Strings(files)
	var raw [][]byte
	var base []otel.Span
	for _, f := range files {
		b, _ := os.ReadFile(f)
		raw = append(raw, b)
		s, _ := otlp.Decode(b, "")
		base = append(base, s...)
	}
	byTrace := map[string][]otel.Span{}
	var traces []string
	for _, s := range base {
		if byTrace[s.TraceID] == nil {
			traces = append(traces, s.TraceID)
		}
		byTrace[s.TraceID] = append(byTrace[s.TraceID], s)
	}

	// Decoding, on the real bytes.
	start := time.Now()
	var bytes, decoded int
	for time.Since(start) < time.Second {
		for _, b := range raw {
			s, _ := otlp.Decode(b, "")
			bytes += len(b)
			decoded += len(s)
		}
	}
	el := time.Since(start)
	t.Logf("decode: %.0f spans/s, %.1f MB/s", float64(decoded)/el.Seconds(), float64(bytes)/1e6/el.Seconds())

	for _, sample := range []float64{1, 0.1} {
		srv := production.NewServer(policy())
		srv.Sample = sample
		checked, flagged := 0, 0
		srv.Emit = func(r production.Result) {
			checked++
			if len(r.Findings()) > 0 || len(r.Incidents) > 0 {
				flagged++
			}
		}
		var before runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)

		start := time.Now()
		batch := make([]otel.Span, 0, 512)
		for i := 0; i < n; i++ {
			for _, s := range byTrace[traces[i%len(traces)]] {
				s.TraceID = fmt.Sprintf("%032x", i)
				batch = append(batch, s)
			}
			if len(batch) > 400 {
				srv.Accept(batch)
				batch = batch[:0]
			}
		}
		srv.Accept(batch)
		srv.Flush(true)
		el := time.Since(start)

		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		t.Logf("sample %.0f%%: %d of %d conversations checked, %d flagged, %.0f conversations/s, %.0f MB allocated",
			sample*100, checked, n, flagged, float64(n)/el.Seconds(), float64(after.TotalAlloc-before.TotalAlloc)/1e6)
		if sample == 1 && checked != n {
			t.Errorf("checked %d of %d with no sampling", checked, n)
		}
		if sample < 1 {
			frac := float64(checked) / float64(n)
			if frac < sample*0.8 || frac > sample*1.2 {
				t.Errorf("sampling %.2f kept %.3f of conversations", sample, frac)
			}
		}
	}
}
