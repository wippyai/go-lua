package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestFrozenTableMutationsRefutesAssignmentAfterFreeze(t *testing.T) {
	checked := testutil.CheckFile(`local cfg = { name = "prod" }
table.freeze(cfg)
cfg.name = "staging"`, "test.lua", testutil.WithStdlib())

	got := frozenTableJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("frozen-table judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeFrozenTable {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeFrozenTable)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.Label != "cfg" {
		t.Fatalf("subject label = %q, want cfg", got[0].Subject.Label)
	}
	if !judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailFrozenTableAssignment) {
		t.Fatalf("evidence = %#v, want assignment mutation detail", got[0].Evidence)
	}
}

func TestFrozenTableMutationsRefutesMutatingCallAfterFreeze(t *testing.T) {
	checked := testutil.CheckFile(`local items = { "a" }
table.freeze(items)
table.insert(items, "b")`, "test.lua", testutil.WithStdlib())

	got := frozenTableJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("frozen-table judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Subject.Label != "items" {
		t.Fatalf("subject label = %q, want items", got[0].Subject.Label)
	}
	if !judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailFrozenTableCall) {
		t.Fatalf("evidence = %#v, want mutating-call detail", got[0].Evidence)
	}
}

func frozenTableJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.FrozenTableMutations{}).Run(obligationpass.Context{
			FunctionKey: "fixture:frozen-table",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}

func judgmentHasEvidenceDetail(item judgment.Judgment, kind judgment.EvidenceDetailKind) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == kind {
			return true
		}
	}
	return false
}
