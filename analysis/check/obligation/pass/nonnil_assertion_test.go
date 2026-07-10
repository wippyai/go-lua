package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestNonNilAssertionsRefutesFlowNarrowedNilOperand(t *testing.T) {
	checked := testutil.CheckFile(`local function f(x: string?): string
	if x == nil then
		return x!
	end
	return x
end`, "test.lua")

	got := nonNilAssertionJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("nonnil judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeNonNilAssertion {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeNonNilAssertion)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.Label != "x" || got[0].Actual.Label != "x" {
		t.Fatalf("labels = subject %q actual %q, want x/x", got[0].Subject.Label, got[0].Actual.Label)
	}
}

func TestNonNilAssertionsSkipsOptionalOperand(t *testing.T) {
	checked := testutil.CheckFile(`local function f(x: string?): string
	return x!
end`, "test.lua")

	got := nonNilAssertionJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("nonnil judgments = %d, want 0: %#v", len(got), got)
	}
}

func nonNilAssertionJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.NonNilAssertions{}).Run(obligationpass.Context{
			FunctionKey: "fixture:nonnil",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}
