package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// CallCallee emits obligations for invoking call targets.
type CallCallee struct{}

func (CallCallee) Name() string {
	return "call.callee"
}

func (CallCallee) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}

	var out []judgment.Judgment
	ctx.Reader.ForEachCall(func(call readmodel.CallSite) bool {
		if call.Callee.Kind == readmodel.CallCalleeReportNone {
			return true
		}
		out = append(out, callCalleeJudgment(ctx, functionKey, call))
		return true
	})
	return out
}

func callCalleeJudgment(ctx Context, functionKey string, call readmodel.CallSite) judgment.Judgment {
	report := call.Callee
	detail := judgment.CalleeNotCallableEvidenceDetail()
	if report.MemberAccess {
		detail = judgment.MemberCalleeNotCallableEvidenceDetail()
	}
	verdict := judgment.VerdictRefuted
	missingTrust := judgment.EvidenceTrustRefuted
	switch report.Kind {
	case readmodel.CallCalleeReportMayBeNil:
		detail = judgment.CalleeMayBeNilEvidenceDetail(report.Callable)
		if report.MemberAccess {
			detail = judgment.MemberCalleeMayBeNilEvidenceDetail(report.Callable)
		}
		verdict = judgment.VerdictUnknown
		missingTrust = judgment.EvidenceTrustUnknown
	case readmodel.CallCalleeReportMissingMember:
		detail = judgment.MemberMissingEvidenceDetail(report.MemberName)
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
