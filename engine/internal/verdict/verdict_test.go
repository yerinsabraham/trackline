package verdict_test

import (
	"errors"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

func TestInterruptingVerdictNeedsEvidence(t *testing.T) {
	for _, sev := range []verdict.Severity{verdict.SeverityWarn, verdict.SeverityBlock} {
		v := verdict.Verdict{Signal: "s", Severity: sev, Summary: "something happened"}
		if err := v.Validate(); !errors.Is(err, verdict.ErrNoEvidence) {
			t.Errorf("severity %s with no evidence: err = %v, want ErrNoEvidence", sev, err)
		}
	}
}

// Info never reaches the user unasked, so it is allowed to be thin.
func TestInfoVerdictMayOmitEvidence(t *testing.T) {
	v := verdict.Verdict{Signal: "s", Severity: verdict.SeverityInfo, Summary: "noted"}
	if err := v.Validate(); err != nil {
		t.Errorf("info verdict rejected: %v", err)
	}
}

func TestVerdictNeedsASummary(t *testing.T) {
	v := verdict.Verdict{Signal: "s", Severity: verdict.SeverityWarn,
		Evidence: []verdict.Evidence{{Kind: verdict.EvidenceFile, Value: "/a"}}}
	if err := v.Validate(); !errors.Is(err, verdict.ErrNoSummary) {
		t.Errorf("err = %v, want ErrNoSummary", err)
	}
}

// The riskViolationRate lesson in type form: an outcome that means "no answer"
// must explain itself, and must never smuggle findings alongside.
func TestNonAnswersMustExplainThemselves(t *testing.T) {
	for _, o := range []verdict.Outcome{verdict.OutcomeNotApplicable, verdict.OutcomeCannotMeasure} {
		r := verdict.Result{Signal: "s", Outcome: o}
		if err := r.Validate(); !errors.Is(err, verdict.ErrNoReason) {
			t.Errorf("outcome %s with no reason: err = %v, want ErrNoReason", o, err)
		}
	}
}

func TestOutcomesCannotContradictTheirContents(t *testing.T) {
	good := verdict.Verdict{Signal: "s", Severity: verdict.SeverityWarn, Summary: "x",
		Evidence: []verdict.Evidence{{Kind: verdict.EvidenceFile, Value: "/a"}}}

	cases := []struct {
		name string
		r    verdict.Result
	}{
		{"clean carrying a finding",
			verdict.Result{Signal: "s", Outcome: verdict.OutcomeClean, Verdicts: []verdict.Verdict{good}}},
		{"finding carrying nothing",
			verdict.Result{Signal: "s", Outcome: verdict.OutcomeFinding}},
		{"cannot-measure carrying a finding",
			verdict.Result{Signal: "s", Outcome: verdict.OutcomeCannotMeasure, Reason: "r", Verdicts: []verdict.Verdict{good}}},
		{"unknown outcome",
			verdict.Result{Signal: "s", Outcome: "made-up"}},
	}
	for _, c := range cases {
		if err := c.r.Validate(); err == nil {
			t.Errorf("%s: accepted, want rejected", c.name)
		}
	}
}

func TestConstructorsProduceValidResults(t *testing.T) {
	good := verdict.Verdict{Severity: verdict.SeverityBlock, Summary: "x",
		Evidence: []verdict.Evidence{{Kind: verdict.EvidenceRule, Value: "never touch .env"}}}

	for _, r := range []verdict.Result{
		verdict.Clean("s"),
		verdict.NotApplicable("s", "nothing of this kind here"),
		verdict.CannotMeasure("s", "the data was not available"),
		verdict.Finding("s", good),
	} {
		if err := r.Validate(); err != nil {
			t.Errorf("%s: %v", r.Outcome, err)
		}
	}
}

// Finding stamps the signal name onto verdicts that omit it, so no verdict can
// reach a user without saying which check produced it.
func TestFindingStampsTheSignalName(t *testing.T) {
	r := verdict.Finding("scope", verdict.Verdict{
		Severity: verdict.SeverityWarn, Summary: "x",
		Evidence: []verdict.Evidence{{Kind: verdict.EvidenceFile, Value: "/a"}},
	})
	if r.Verdicts[0].Signal != "scope" {
		t.Errorf("signal = %q, want it filled in", r.Verdicts[0].Signal)
	}
}

func TestOnlyWarnAndBlockInterrupt(t *testing.T) {
	if verdict.SeverityInfo.Interrupts() {
		t.Error("info must never interrupt")
	}
	if !verdict.SeverityWarn.Interrupts() || !verdict.SeverityBlock.Interrupts() {
		t.Error("warn and block must interrupt")
	}
}
