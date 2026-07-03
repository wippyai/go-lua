package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// DirectCallArity emits call argument-count obligations from solved state.
type DirectCallArity struct{}

func (DirectCallArity) Name() string {
	return "direct_call.arity"
}

func (DirectCallArity) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}

	var out []judgment.Judgment
	ctx.Reader.ForEachCall(func(call readmodel.CallSite) bool {
		if call.Arity.Kind == readmodel.CallArityReportNone {
			return true
		}
		out = append(out, directCallArityJudgment(ctx, functionKey, call))
		return true
	})
	return out
}

func directCallArityJudgment(ctx Context, functionKey string, call readmodel.CallSite) judgment.Judgment {
	report := call.Arity
	detail := judgment.ArityTooFewEvidenceDetail(report.ExpectedCount, report.ActualCount)
	if report.Kind == readmodel.CallArityReportTooMany {
		detail = judgment.ArityTooManyEvidenceDetail(report.ExpectedCount, report.ActualCount)
	}
	spans := []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, report.CallSpan)}
	if report.Kind == readmodel.CallArityReportTooMany && report.ExtraSpan.StartLine != 0 {
		spans = append(spans, spanFromReadModel(ctx.SourceFile, report.ExtraSpan))
	}
	return judgment.Judgment{
		Code:  judgment.CodeCallArity,
		Point: call.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectCallExpression,
			fmt.Sprintf("call:%d:arity", call.Point),
		).WithLabel(report.CallableName),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: call.Point,
					Key:   "arity:actual",
				},
				Detail: detail,
				Span:   spanFromReadModel(ctx.SourceFile, report.CallSpan),
			},
			{
				Kind:  judgment.EvidenceUserAssertion,
				Trust: judgment.EvidenceTrustClaimed,
				Origin: judgment.OriginRef{
					Point: call.Point,
					Key:   "arity:expected",
				},
				Detail: detail,
				Span:   spanFromReadModel(ctx.SourceFile, report.DeclarationSpan),
			},
			{
				Kind:  judgment.EvidenceMissingProof,
				Trust: judgment.EvidenceTrustRefuted,
				Origin: judgment.OriginRef{
					Point: call.Point,
					Key:   "arity:proof",
				},
				Detail: detail,
				Span:   spanFromReadModel(ctx.SourceFile, report.ExtraSpan),
			},
		},
		Spans: spans,
	}
}
