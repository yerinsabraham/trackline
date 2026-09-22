package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Reading the findings log back.
//
// The breakdown is per signal, not aggregate, and that is the whole point. A
// heuristic that is noisy on its own must not be able to hide inside a good
// average: aggregate quiet is how a check that fires on ordinary work survives
// long enough to get the tool uninstalled.

type entry struct {
	Host    string           `json:"host"`
	Tool    string           `json:"tool"`
	Paths   []string         `json:"paths"`
	Mode    string           `json:"mode"`
	Blocked bool             `json:"blocked"`
	Results []verdict.Result `json:"results"`
}

type tally struct {
	clean, findings, unmeasured, notApplicable int
	examples                                   []string
}

func summarise(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	per := map[string]*tally{}
	actions, blocked := 0, 0

	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		actions++
		if e.Blocked {
			blocked++
		}
		for _, r := range e.Results {
			t := per[r.Signal]
			if t == nil {
				t = &tally{}
				per[r.Signal] = t
			}
			switch r.Outcome {
			case verdict.OutcomeClean:
				t.clean++
			case verdict.OutcomeFinding:
				t.findings++
				for _, v := range r.Verdicts {
					if len(t.examples) < 3 {
						t.examples = append(t.examples, v.Summary)
					}
				}
			case verdict.OutcomeCannotMeasure:
				t.unmeasured++
			case verdict.OutcomeNotApplicable:
				t.notApplicable++
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	fmt.Printf("%d action(s) seen, %d blocked.\n\n", actions, blocked)

	names := make([]string, 0, len(per))
	for n := range per {
		names = append(names, n)
	}
	sort.Strings(names)

	fmt.Printf("  %-18s %8s %8s %12s\n", "check", "fired", "clean", "not measured")
	for _, n := range names {
		t := per[n]
		// Rate over the actions the check could actually judge. Including the
		// ones it declined would flatter every number.
		judged := t.clean + t.findings
		rate := ""
		if judged > 0 {
			rate = fmt.Sprintf("  (%.0f%% of %d judged)", 100*float64(t.findings)/float64(judged), judged)
		}
		fmt.Printf("  %-18s %8d %8d %12d%s\n", n, t.findings, t.clean, t.unmeasured, rate)
	}

	fmt.Print("\nEvery one of those is a question for you: right, wrong, or just annoying.\n" +
		"Wrong and annoying both count against the check.\n")

	for _, n := range names {
		t := per[n]
		if len(t.examples) == 0 {
			continue
		}
		fmt.Printf("\n  %s fired on:\n", n)
		for _, ex := range t.examples {
			fmt.Printf("    %s\n", ex)
		}
	}

	var unmeasured []string
	for _, n := range names {
		if per[n].unmeasured > 0 {
			unmeasured = append(unmeasured, n)
		}
	}
	if len(unmeasured) > 0 {
		fmt.Printf("\nnot measured is not clean: %v could not see some actions at all.\n", unmeasured)
	}
	return nil
}
