package pass_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDirectCallCalleeReportsNotCallableAndMayBeNil(t *testing.T) {
	result := checkFileRoot(t, `local x: number = 42
x()
local maybe: (() -> string)? = nil
maybe()`)

	got := obligationpass.New(obligationpass.DirectCallCallee{}).Run(obligationpass.Context{
		FunctionKey: "fixture:callee",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 2 {
		t.Fatalf("judgments = %d, want two callee judgments: %#v", len(got), got)
	}
	assertCalleeJudgment(t, got[0], judgment.EvidenceDetailCalleeNotCallable, "x", typ.Number, judgment.VerdictRefuted)
	assertCalleeJudgment(t, got[1], judgment.EvidenceDetailCalleeMayBeNil, "maybe", nil, judgment.VerdictUnknown)
	if !strings.HasPrefix(got[0].Subject.StableKey(), "fixture:callee|call|call:") || !strings.HasSuffix(got[0].Subject.StableKey(), ":callee") {
		t.Fatalf("first stable key = %q, want callee call subject", got[0].Subject.StableKey())
	}
	if got[0].Subject.StableKey() == got[1].Subject.StableKey() {
		t.Fatalf("stable keys are not distinct: %q", got[0].Subject.StableKey())
	}
}

func TestDirectCallCalleeSkipsCallableTarget(t *testing.T) {
	result := checkFileRoot(t, `local function ok(): string
    return "ok"
end
ok()`)

	got := obligationpass.New(obligationpass.DirectCallCallee{}).Run(obligationpass.Context{
		FunctionKey: "fixture:callable",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want callable target accepted", got)
	}
}

func assertCalleeJudgment(t *testing.T, got judgment.Judgment, kind judgment.EvidenceDetailKind, label string, actual typ.Type, verdict judgment.Verdict) {
	t.Helper()
	if got.Code != judgment.CodeCallCallee {
		t.Fatalf("code = %q, want %q", got.Code, judgment.CodeCallCallee)
	}
	if got.Subject.Kind != judgment.SubjectCallExpression || got.Subject.Label != label {
		t.Fatalf("subject = %#v, want call expression label %q", got.Subject, label)
	}
	if got.Verdict != verdict {
		t.Fatalf("verdict = %v, want %v", got.Verdict, verdict)
	}
	if len(got.Spans) != 1 || got.Spans[0].StartLine == 0 {
		t.Fatalf("spans = %#v, want primary callee span", got.Spans)
	}
	if actual != nil && !typ.TypeEquals(got.Actual.ProjectedType, actual) {
		t.Fatalf("actual type = %v, want %v", got.Actual.ProjectedType, actual)
	}
	for _, evidence := range got.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof && evidence.Detail.Kind == kind {
			return
		}
	}
	t.Fatalf("evidence = %#v, want missing-proof detail %v", got.Evidence, kind)
}
