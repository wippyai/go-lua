package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestDeadAssignmentsRefutesOverwriteBeforeRead(t *testing.T) {
	checked := testutil.CheckFile(`
local value = 1
value = 2
return value
`, "test.lua")

	got := deadAssignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("dead-assignment judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeDeadAssignment {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeDeadAssignment)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.Label != "value" {
		t.Fatalf("subject label = %q, want value", got[0].Subject.Label)
	}
	if !judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailDeadAssignmentOverwrite) {
		t.Fatalf("evidence = %#v, want overwrite detail", got[0].Evidence)
	}
}

func TestDeadAssignmentsIncludesExitFrontierEvidence(t *testing.T) {
	checked := testutil.CheckFile(`
local value = 1
if test then
	return
end
value = 2
return value
`, "test.lua")

	got := deadAssignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("dead-assignment judgments = %d, want 1: %#v", len(got), got)
	}
	if !judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailDeadAssignmentOverwrite) ||
		!judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailDeadAssignmentExit) {
		t.Fatalf("evidence = %#v, want overwrite and exit details", got[0].Evidence)
	}
}

func deadAssignmentJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.DeadAssignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:dead-assignment",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}
