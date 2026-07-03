package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// DirectCallCallee emits obligations for invoking direct call targets.
type DirectCallCallee struct{}

func (DirectCallCallee) Name() string {
	return "direct_call.callee"
}

func (DirectCallCallee) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}

	var out []judgment.Judgment
	ctx.Reader.ForEachCall(func(call readmodel.CallSite) bool {
		if call.Callee.Kind == readmodel.CallCalleeReportNone {
			return true
		}
		out = append(out, directCallCalleeJudgment(ctx, functionKey, call))
		return true
	})
	return out
}

func directCallCalleeJudgment(ctx Context, functionKey string, call readmodel.CallSite) judgment.Judgment {
	report := call.Callee
	detail := judgment.CalleeNotCallableEvidenceDetail()
	verdict := judgment.VerdictRefuted
	missingTrust := judgment.EvidenceTrustRefuted
	if report.Kind == readmodel.CallCalleeReportMayBeNil {
		detail = judgment.CalleeMayBeNilEvidenceDetail(report.Callable)
		verdict = judgment.VerdictUnknown
		missingTrust = judgment.EvidenceTrustUnknown
	}
	span := spanFromReadModel(ctx.SourceFile, report.Span)
	return judgment.Judgment{
		Code:  judgment.CodeCallCallee,
		Point: call.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectCallExpression,
			fmt.Sprintf("call:%d:callee", call.Point),
		).WithLabel(report.CallableName),
		Actual:  judgment.NewValueRef(0, report.Type),
		Verdict: verdict,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: call.Point,
					Key:   "callee:actual",
				},
				Detail: detail,
				Span:   span,
			},
			{
				Kind:  judgment.EvidenceUserAssertion,
				Trust: judgment.EvidenceTrustClaimed,
				Origin: judgment.OriginRef{
					Point: call.Point,
					Key:   "callee:callable",
				},
				Detail: detail,
				Span:   span,
			},
			{
				Kind:  judgment.EvidenceMissingProof,
				Trust: missingTrust,
				Origin: judgment.OriginRef{
					Point: call.Point,
					Key:   "callee:proof",
				},
				Detail: detail,
				Span:   span,
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
