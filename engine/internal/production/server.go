package production

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/otlp"
)

// Server receives OTLP/HTTP trace exports and checks each conversation as it
// completes.
//
// A conversation is complete when its root span arrives: exporters batch by
// time, so a run's spans come in several requests, and the root, which ends
// last, comes last. A trace whose root never arrives (a crashed process, a
// dropped batch) is checked after Idle, so nothing waits forever and nothing
// is silently discarded.
type Server struct {
	Config config.Config
	// Sample is the fraction of conversations checked, 0 < Sample <= 1.
	// Chosen by trace id, so a conversation is kept or dropped whole: sampling
	// spans would cut runs in half and make every policy check unreliable.
	Sample float64
	Idle   time.Duration
	// Emit receives every result. Alert, if set, receives only the ones with
	// something in them.
	Emit  func(Result)
	Alert func(Result)

	mu      sync.Mutex
	pending map[string]*pendingTrace
	monitor *Monitor
	now     func() time.Time
}

type pendingTrace struct {
	spans []otel.Span
	last  time.Time
}

// NewServer builds a server with the default monitor thresholds.
func NewServer(cfg config.Config) *Server {
	return &Server{Config: cfg, Sample: 1, Idle: 30 * time.Second,
		pending: map[string]*pendingTrace{}, monitor: NewMonitor(), now: time.Now}
}

// ServeHTTP handles POST /v1/traces.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/traces" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	spans, err := otlp.Decode(body, r.Header.Get("Content-Type"))
	if err != nil {
		// A 400 makes the exporter log it. Accepting and dropping would make a
		// broken pipeline look like a quiet one.
		http.Error(w, "could not decode trace export: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.Accept(spans)

	// An empty ExportTraceServiceResponse, in the encoding asked for.
	if strings.Contains(r.Header.Get("Content-Type"), "json") {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

// Accept takes decoded spans, and checks every conversation they complete.
func (s *Server) Accept(spans []otel.Span) {
	s.mu.Lock()
	// Two passes. A request groups spans by instrumentation scope, not by
	// time, so a root can come before its own model-call spans in the same
	// request; closing the trace on sight of the root would split it.
	var roots []string
	for _, sp := range spans {
		if !s.sampled(sp.TraceID) {
			continue
		}
		p := s.pending[sp.TraceID]
		if p == nil {
			p = &pendingTrace{}
			s.pending[sp.TraceID] = p
		}
		p.spans = append(p.spans, sp)
		p.last = s.now()
		if sp.ParentSpanID == "" {
			roots = append(roots, sp.TraceID)
		}
	}
	var done [][]otel.Span
	for _, id := range roots {
		if p, ok := s.pending[id]; ok {
			done = append(done, p.spans)
			delete(s.pending, id)
		}
	}
	s.mu.Unlock()
	for _, d := range done {
		s.check(d)
	}
}

// Flush checks every trace that has been quiet for longer than Idle, or every
// pending trace when all is true, as at shutdown.
func (s *Server) Flush(all bool) {
	s.mu.Lock()
	var done [][]otel.Span
	for id, p := range s.pending {
		if all || s.now().Sub(p.last) > s.Idle {
			done = append(done, p.spans)
			delete(s.pending, id)
		}
	}
	s.mu.Unlock()
	for _, d := range done {
		s.check(d)
	}
}

func (s *Server) check(spans []otel.Span) {
	for _, c := range Assemble(spans) {
		// One monitor, one conversation at a time: an outage is a sequence.
		s.mu.Lock()
		r := Evaluate(c, s.Config)
		r.Incidents = s.monitor.Observe(c)
		s.mu.Unlock()
		if s.Emit != nil {
			s.Emit(r)
		}
		if s.Alert != nil && (len(r.Findings()) > 0 || len(r.Incidents) > 0) {
			s.Alert(r)
		}
	}
}

func (s *Server) sampled(traceID string) bool {
	if s.Sample >= 1 {
		return true
	}
	h := fnv.New32a()
	h.Write([]byte(traceID))
	return float64(h.Sum32())/float64(^uint32(0)) < s.Sample
}

// Webhook posts a result to url as a Slack-style {"text": ...} message, with
// the full result alongside for anything that wants the detail. Slack, and
// most chat tools that copied its incoming-webhook shape, read the text field
// and ignore the rest, so one alert works in all of them without per-tool
// code.
func Webhook(url string) func(Result) {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(r Result) {
		body, _ := json.Marshal(map[string]any{"text": Summary(r), "trackline": r})
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
	}
}

// Summary is one conversation's result in a line or two a person can read.
func Summary(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "trackline: %s", r.Conversation.ID)
	if r.Conversation.Service != "" {
		fmt.Fprintf(&b, " (%s)", r.Conversation.Service)
	}
	for _, v := range r.Findings() {
		fmt.Fprintf(&b, "\n• policy: %s", v.Summary)
	}
	for _, i := range r.Incidents {
		fmt.Fprintf(&b, "\n• %s: %s", i.Kind, i.Summary)
	}
	return b.String()
}
