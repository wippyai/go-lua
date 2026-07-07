package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRenderReturnJudgmentConcreteMismatch(t *testing.T) {
	item := judgment.Judgment{
		Code:  judgment.CodeReturn,
		Point: cfg.Point(7),
		Subject: judgment.NewSubjectRef("test.lua", judgment.SubjectReturnValue, "return:7:0").
			WithLabel("returned value 1"),
		Actual:   judgment.NewValueRef(1, typ.String).WithLabel(`returned value 1 ("bad")`),
		Expected: judgment.NewTypeRef(typ.Number).WithLabel("returned value 1"),
		Verdict:  judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Span:  judgment.SpanRef{StartLine: 1, StartCol: 29, EndLine: 1, EndCol: 34},
			},
			{
				Kind:  judgment.EvidenceUserAssertion,
				Trust: judgment.EvidenceTrustClaimed,
				Span:  judgment.SpanRef{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 21},
			},
		},
		Spans: []judgment.SpanRef{{StartLine: 1, StartCol: 29, EndLine: 1, EndCol: 34}},
	}

	d, ok := renderReturnJudgmentWithPolicy(newJudgmentRenderContext(), item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderReturnJudgmentWithPolicy returned false")
	}
	if d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want return contract error", d)
	}
	if !strings.Contains(d.Message, `returned value 1 ("bad") is string, not number`) {
		t.Fatalf("message = %q, want concrete return mismatch", d.Message)
	}
	if len(d.Labels) != 2 || d.Labels[0].Message != labelReturnedValue || d.Labels[1].Message != labelDeclaredReturn {
		t.Fatalf("labels = %#v, want returned value and declared return type", d.Labels)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), `returned value 1 ("bad") has type string`) ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "returned value 1 must satisfy declared return type number") {
		t.Fatalf("evidence = %#v, want actual and declared-return evidence", d.Explanation.Evidence())
	}
}

func TestRenderReturnJudgmentNilGuardIsDrivenByEvidence(t *testing.T) {
	item := judgment.Judgment{
		Code:  judgment.CodeReturn,
		Point: cfg.Point(7),
		Subject: judgment.NewSubjectRef("test.lua", judgment.SubjectReturnValue, "return:7:0").
			WithLabel("returned value 1"),
		Actual:   judgment.NewValueRef(1, typ.String).WithLabel("value"),
		Expected: judgment.NewTypeRef(typ.String).WithLabel("returned value 1"),
		Verdict:  judgment.VerdictUnknown,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Span:  judgment.SpanRef{StartLine: 2, StartCol: 9, EndLine: 2, EndCol: 14},
			},
			{
				Kind:  judgment.EvidenceUserAssertion,
				Trust: judgment.EvidenceTrustClaimed,
				Span:  judgment.SpanRef{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 21},
			},
			{
				Kind:   judgment.EvidenceMissingProof,
				Trust:  judgment.EvidenceTrustUnknown,
				Detail: judgment.MayBeNilEvidenceDetail(),
				Span:   judgment.SpanRef{StartLine: 2, StartCol: 9, EndLine: 2, EndCol: 14},
			},
		},
		Spans: []judgment.SpanRef{{StartLine: 2, StartCol: 9, EndLine: 2, EndCol: 14}},
	}

	d, ok := renderReturnJudgmentWithPolicy(newJudgmentRenderContext(), item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderReturnJudgmentWithPolicy returned false")
	}
	if !strings.Contains(d.Message, "cannot return value as returned value 1 because it may be nil") {
		t.Fatalf("message = %q, want nil-specific return message from structured evidence", d.Message)
	}
	if !strings.Contains(d.Help, "Guard `value` with a nil check") {
		t.Fatalf("help = %q, want nil-specific return help from structured evidence", d.Help)
	}
}

func TestProduceReturnJudgmentDiagnosticsFromResult(t *testing.T) {
	fn := mustFunctionExpr(t, `function f(): number return "bad" end`)
	result, err := program.RunFunction(fn, program.Config{
		Check: body.Config{
			Registry: standard.Registry(),
		},
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	diags := produceReturnJudgmentDiagnostics(result.RootResult(), "main.lua")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one return judgment diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want return contract error", d)
	}
	if !strings.Contains(d.Message, `returned value 1 ("bad")`) ||
		!strings.Contains(d.Message, `"bad"`) ||
		!strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q, want literal string-to-number return mismatch", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), `returned value 1 ("bad") has literal value "bad"`) ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "returned value 1 must satisfy declared return type number") {
		t.Fatalf("evidence = %#v, want return evidence", d.Explanation.Evidence())
	}
}
