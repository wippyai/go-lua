package pass_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestDirectCallArityReportsImportedSignatureTooFewAndTooMany(t *testing.T) {
	result := checkFunction(t, `function f()
    need_string()
    need_string("ok", "extra")
end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.DirectCallArity{}).Run(obligationpass.Context{
		FunctionKey: "fixture:arity",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 2 {
		t.Fatalf("judgments = %d, want two arity judgments: %#v", len(got), got)
	}
	assertArityJudgment(t, got[0], judgment.EvidenceDetailArityTooFew, 1, 0)
	assertArityJudgment(t, got[1], judgment.EvidenceDetailArityTooMany, 1, 2)
	if len(got[1].Spans) != 2 || got[1].Spans[1].StartLine == 0 {
		t.Fatalf("too-many spans = %#v, want call span plus extra-argument span", got[1].Spans)
	}
}

func TestDirectCallArityReportsLocalFunctionThroughSummaryContract(t *testing.T) {
	result := checkFileRoot(t, `local function add(a: number, b: number): number
    return a + b
end
add(1)
add(1, 2, 3)`)

	got := obligationpass.New(obligationpass.DirectCallArity{}).Run(obligationpass.Context{
		FunctionKey: "fixture:local-arity",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 2 {
		t.Fatalf("judgments = %d, want two local arity judgments: %#v", len(got), got)
	}
	assertArityJudgment(t, got[0], judgment.EvidenceDetailArityTooFew, 2, 1)
	assertArityJudgment(t, got[1], judgment.EvidenceDetailArityTooMany, 2, 3)
}

func assertArityJudgment(t *testing.T, got judgment.Judgment, kind judgment.EvidenceDetailKind, expected, actual int) {
	t.Helper()
	if got.Code != judgment.CodeCallArity {
		t.Fatalf("code = %q, want %q", got.Code, judgment.CodeCallArity)
	}
	if got.Subject.Kind != judgment.SubjectCallExpression {
		t.Fatalf("subject kind = %v, want call expression", got.Subject.Kind)
	}
	if got.Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got.Verdict)
	}
	detail := arityDetail(t, got)
	if detail.Kind != kind || detail.ExpectedCount != expected || detail.ActualCount != actual {
		t.Fatalf("detail = %#v, want %v expected=%d actual=%d", detail, kind, expected, actual)
	}
}

func arityDetail(t *testing.T, got judgment.Judgment) judgment.EvidenceDetail {
	t.Helper()
	for _, evidence := range got.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof {
			return evidence.Detail
		}
	}
	t.Fatalf("evidence = %#v, want missing-proof arity detail", got.Evidence)
	return judgment.EvidenceDetail{}
}

func checkFileRoot(t *testing.T, src string) *body.Result {
	t.Helper()
	result := testutil.CheckFile(src, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	return result
}
