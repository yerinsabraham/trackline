package production

import (
	"fmt"
	"time"
)

// Incident is an operational failure visible only across runs, or across the
// calls of one run: an outage, a loop, a slowdown. Not misalignment. The agent
// may be doing exactly what it was asked and still be failing its users.
type Incident struct {
	Kind         string `json:"kind"` // tool-errors, loop, latency, tokens
	Conversation string `json:"conversation"`
	Summary      string `json:"summary"`
}

// Thresholds are the defaults. They are round numbers chosen before any trace
// was seen, not tuned to the data they are tested on, and are fields so an
// operator with a real baseline can replace them.
type Thresholds struct {
	// LoopCalls is how many identical calls in one run make a loop.
	LoopCalls int
	// ErrorWindow and ErrorMin: a tool is failing when at least ErrorMin of
	// its last ErrorWindow calls failed, and at least half of them.
	ErrorWindow, ErrorMin int
	// DriftFactor is how many times the baseline median a run must be to
	// count as drift, once Baseline runs have been seen.
	DriftFactor float64
	Baseline    int
	// LatencyFloor stops a fast service tripping on noise: a run must also be
	// slower than this in absolute terms.
	LatencyFloor time.Duration
}

// DefaultThresholds is what a Monitor starts with.
var DefaultThresholds = Thresholds{
	LoopCalls: 3, ErrorWindow: 10, ErrorMin: 3,
	DriftFactor: 5, Baseline: 10, LatencyFloor: time.Second,
}

// Monitor watches conversations in the order they arrive.
type Monitor struct {
	T Thresholds

	calls    map[string][]bool // per tool, recent outcomes, true = failed
	alerting map[string]bool   // per tool, an outage already reported
	latency  []time.Duration
	tokens   []int
}

// NewMonitor starts a monitor with the default thresholds.
func NewMonitor() *Monitor {
	return &Monitor{T: DefaultThresholds, calls: map[string][]bool{}, alerting: map[string]bool{}}
}

// Observe takes one conversation and returns anything it reveals.
func (m *Monitor) Observe(c Conversation) []Incident {
	var out []Incident
	out = append(out, m.loops(c)...)
	out = append(out, m.errors(c)...)
	out = append(out, m.drift(c)...)
	return out
}

func (m *Monitor) loops(c Conversation) []Incident {
	counts := map[string]int{}
	tools := map[string]string{}
	var order []string
	for _, ev := range c.Actions {
		if ev.Action.ToolNameUnknown {
			continue
		}
		k := argsKey(ev)
		if counts[k] == 0 {
			order = append(order, k)
			tools[k] = ev.Action.ToolName
		}
		counts[k]++
	}
	var out []Incident
	for _, k := range order {
		if counts[k] >= m.T.LoopCalls {
			out = append(out, Incident{Kind: "loop", Conversation: c.ID,
				Summary: fmt.Sprintf("called %s %d times with the same arguments in one run", tools[k], counts[k])})
		}
	}
	return out
}

// errors reports a tool once when it starts failing, and not again until it
// has recovered. An outage is one incident, not one per conversation it hits.
func (m *Monitor) errors(c Conversation) []Incident {
	failed := map[string]int{}
	for _, t := range c.ToolErrors {
		failed[t]++
	}
	var out []Incident
	seen := map[string]bool{}
	for _, ev := range c.Actions {
		t := ev.Action.ToolName
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		calls := 0
		for _, e := range c.Actions {
			if e.Action.ToolName == t {
				calls++
			}
		}
		for i := 0; i < calls; i++ {
			m.calls[t] = append(m.calls[t], i < failed[t])
		}
		if len(m.calls[t]) > m.T.ErrorWindow {
			m.calls[t] = m.calls[t][len(m.calls[t])-m.T.ErrorWindow:]
		}
		n := 0
		for _, f := range m.calls[t] {
			if f {
				n++
			}
		}
		switch {
		case n >= m.T.ErrorMin && n*2 >= len(m.calls[t]) && !m.alerting[t]:
			m.alerting[t] = true
			out = append(out, Incident{Kind: "tool-errors", Conversation: c.ID,
				Summary: fmt.Sprintf("%s failed %d of its last %d calls", t, n, len(m.calls[t]))})
		case failed[t] == 0:
			m.alerting[t] = false
		}
	}
	return out
}

// drift compares a run with the runs before it. A drifted run is kept out of
// the baseline, so a slowdown does not become the new normal while it lasts.
func (m *Monitor) drift(c Conversation) []Incident {
	var out []Incident
	if c.ModelLatency > 0 {
		base := medianDur(m.latency)
		if len(m.latency) >= m.T.Baseline && c.ModelLatency > m.T.LatencyFloor &&
			float64(c.ModelLatency) > m.T.DriftFactor*float64(base) {
			out = append(out, Incident{Kind: "latency", Conversation: c.ID,
				Summary: fmt.Sprintf("model calls took %s, against a usual %s", c.ModelLatency.Round(time.Millisecond), base.Round(time.Millisecond))})
		} else {
			m.latency = keep(m.latency, c.ModelLatency, 50)
		}
	}
	if c.Tokens > 0 {
		base := medianInt(m.tokens)
		if len(m.tokens) >= m.T.Baseline && float64(c.Tokens) > m.T.DriftFactor*float64(base) {
			out = append(out, Incident{Kind: "tokens", Conversation: c.ID,
				Summary: fmt.Sprintf("used %d tokens, against a usual %d", c.Tokens, base)})
		} else {
			m.tokens = keepInt(m.tokens, c.Tokens, 50)
		}
	}
	return out
}

func keep(s []time.Duration, v time.Duration, n int) []time.Duration {
	s = append(s, v)
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

func keepInt(s []int, v, n int) []int {
	s = append(s, v)
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

func medianDur(s []time.Duration) time.Duration { return median(s) }

func medianInt(s []int) int {
	if len(s) == 0 {
		return 0
	}
	c := append([]int(nil), s...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j] < c[j-1]; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
	return c[len(c)/2]
}
