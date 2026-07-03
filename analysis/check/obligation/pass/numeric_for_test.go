package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestNumericForOperandsRefutesStringInit(t *testing.T) {
	checked := testutil.CheckFile(`for i = "one", 10 do
end`, "test.lua")

	got := numericForOperandJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("numeric-for judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeNumericForOperand {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeNumericForOperand)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Expected.Type == nil || got[0].Expected.Type.String() != "number" {
		t.Fatalf("expected = %v, want number", got[0].Expected.Type)
	}
	if got[0].Expected.Label != "initial value" {
		t.Fatalf("expected label = %q, want initial value", got[0].Expected.Label)
	}
}

func TestNumericForOperandsSkipPartlyNumericUnion(t *testing.T) {
	checked := testutil.CheckFile(`function f(value: number | string)
	for i = value, 10 do
	end
end`, "test.lua")

	got := numericForOperandJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("numeric-for judgments = %d, want 0: %#v", len(got), got)
	}
}

func TestNumericForOperandsCarriesExplicitAnyCastEvidence(t *testing.T) {
	checked := testutil.CheckFile(`for i = ("one" :: any), 10 do
end`, "test.lua")

	got := numericForOperandJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("numeric-for judgments = %d, want 1: %#v", len(got), got)
	}
	if !judgmentHasEvidence(got[0], judgment.EvidenceUserAssertion) ||
		!judgmentHasEvidence(got[0], judgment.EvidencePrecisionBoundary) ||
		!judgmentHasEvidence(got[0], judgment.EvidenceMissingProof) {
		t.Fatalf("evidence = %#v, want explicit-any assertion, precision boundary, and missing proof", got[0].Evidence)
	}
}

func numericForOperandJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.NumericForOperands{}).Run(obligationpass.Context{
			FunctionKey: "fixture:numeric-for",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}

func judgmentHasEvidence(item judgment.Judgment, kind judgment.EvidenceKind) bool {
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind {
			return true
		}
	}
	return false
}
