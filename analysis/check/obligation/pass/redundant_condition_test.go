package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestRedundantConditionsRefutesRepeatedGuard(t *testing.T) {
	checked := testutil.CheckFile(`
local function f(flag: boolean): ()
    if flag then
        if flag then
            return
        end
    end
end
`, "test.lua")

	got := redundantConditionJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("redundant-condition judgments = %d, want 1: %#v", len(got), got)
	}
	item := got[0]
	if item.Code != judgment.CodeRedundantCondition {
		t.Fatalf("code = %q, want %q", item.Code, judgment.CodeRedundantCondition)
	}
	if item.Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", item.Verdict)
	}
	if !judgmentHasEvidenceDetail(item, judgment.EvidenceDetailRedundantConditionCheck) ||
		!judgmentHasEvidenceDetail(item, judgment.EvidenceDetailRedundantConditionProof) ||
		!judgmentHasEvidenceDetail(item, judgment.EvidenceDetailRedundantConditionStability) {
		t.Fatalf("evidence = %#v, want check, proof, and stability details", item.Evidence)
	}
	if len(item.Spans) != 2 || item.Spans[0].DisplayFile() != "test.lua" || item.Spans[1].DisplayFile() != "test.lua" {
		t.Fatalf("spans = %#v, want current and proof spans in test.lua", item.Spans)
	}
}

func redundantConditionJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.RedundantConditions{}).Run(obligationpass.Context{
			FunctionKey: "fixture:redundant-condition",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}
