package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestConcatOperandsRefutesOptionalOperand(t *testing.T) {
	checked := testutil.CheckFile(`local maybe: string? = nil
local label: string = "prefix:" .. maybe`, "test.lua")

	got := concatOperandJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("concat judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeConcatOperand {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeConcatOperand)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.Label != "maybe" || got[0].Actual.Label != "maybe" {
		t.Fatalf("labels = subject %q actual %q, want maybe/maybe", got[0].Subject.Label, got[0].Actual.Label)
	}
	if !judgmentHasEvidence(got[0], judgment.EvidenceAbstractFact) {
		t.Fatalf("evidence = %#v, want operand fact", got[0].Evidence)
	}
}

func concatOperandJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.ConcatOperands{}).Run(obligationpass.Context{
			FunctionKey: "fixture:concat",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}
