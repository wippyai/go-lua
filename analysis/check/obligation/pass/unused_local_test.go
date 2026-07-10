package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestUnusedLocalsRefutesReachableUnreadLocal(t *testing.T) {
	checked := testutil.CheckFile(`local unused = 1`, "test.lua")

	got := unusedLocalJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("unused-local judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeUnusedLocal {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeUnusedLocal)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.Label != "unused" {
		t.Fatalf("subject label = %q, want unused", got[0].Subject.Label)
	}
	if len(got[0].Spans) != 1 || got[0].Spans[0].StartLine != 1 || got[0].Spans[0].StartCol != 7 {
		t.Fatalf("spans = %#v, want local-name span", got[0].Spans)
	}
	if !judgmentHasEvidence(got[0], judgment.EvidenceAbstractFact) {
		t.Fatalf("evidence = %#v, want abstract no-read fact", got[0].Evidence)
	}
}

func TestUnusedLocalsSkipsReadAndCapturedLocals(t *testing.T) {
	checked := testutil.CheckFile(`
local used = 1
local captured = 2
local fn = function()
    return captured
end
return used, fn
`, "test.lua")

	got := unusedLocalJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("unused-local judgments = %#v, want none", got)
	}
}

func unusedLocalJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.UnusedLocals{}).Run(obligationpass.Context{
			FunctionKey: "fixture:unused-local",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}
