package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// SendSafety emits zero-copy send admission judgments from solved ownership
// facts. Unknown is intentionally non-fatal: runtime copy/promotion remains the
// correct fallback.
type SendSafety struct{}

func (SendSafety) Name() string {
	return "send.isolation"
}

func (SendSafety) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachCall(func(call readmodel.CallSite) bool {
		for _, report := range call.SendSafety {
			out = append(out, sendSafetyJudgment(ctx, functionKey, report))
		}
		return true
	})
	return out
}

func sendSafetyJudgment(ctx Context, functionKey string, report readmodel.SendSafety) judgment.Judgment {
	arg := report.Argument
	verdict := judgment.VerdictUnknown
	trust := judgment.EvidenceTrustUnknown
	if report.Verdict == readmodel.SendSafetyProvenIsolated || report.Verdict == readmodel.SendSafetyProvenImmutable {
		verdict = judgment.VerdictProven
		trust = judgment.EvidenceTrustProven
	}

	evidence := judgment.EvidenceChain{{
		Kind:  judgment.EvidenceAbstractFact,
		Trust: trust,
		Origin: judgment.OriginRef{
			Point: report.Point,
			Key:   fmt.Sprintf("%s:arg:%d:target:%s", judgment.OriginSendSafety, arg.Index, report.Target.Key()),
		},
		Detail: judgment.EvidenceDetail{
			Kind:         judgment.EvidenceDetailSendSafetyFact,
			Message:      report.Reason,
			SubjectLabel: sendSafetyEvidenceLabel(report),
		},
		Span: spanFromReadModel(ctx.SourceFile, arg.Span),
	}}
	switch report.Verdict {
	case readmodel.SendSafetyProvenIsolated:
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: report.Point,
				Key:   fmt.Sprintf("%s:arg:%d", judgment.OriginSendIsolationProof, arg.Index),
			},
			Detail: judgment.EvidenceDetail{
				Kind:    judgment.EvidenceDetailSendSafetyProof,
				Message: "direct literal birth site has no retained graph identity",
			},
			Span: spanFromReadModel(ctx.SourceFile, sendSafetyBirthSpanOrArg(report)),
		})
	case readmodel.SendSafetyProvenImmutable:
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: report.Point,
				Key:   fmt.Sprintf("%s:arg:%d", judgment.OriginSendImmutableProof, arg.Index),
			},
			Detail: judgment.EvidenceDetail{
				Kind:    judgment.EvidenceDetailSendSafetyProof,
				Message: "exact identity is frozen before send",
			},
			Span: spanFromReadModel(ctx.SourceFile, sendSafetyBirthSpanOrArg(report)),
		})
	case readmodel.SendSafetyUnknown:
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceMissingProof,
			Trust: judgment.EvidenceTrustUnknown,
			Origin: judgment.OriginRef{
				Point: report.Point,
				Key:   fmt.Sprintf("send:arg:%d:missing-zero-copy-proof", arg.Index),
			},
			Detail: judgment.EvidenceDetail{
				Kind:    judgment.EvidenceDetailSendSafetyBlocker,
				Message: report.Reason,
			},
		})
	}

	return judgment.Judgment{
		Code:  judgment.CodeSendIsolation,
		Point: report.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectCallArgument,
			fmt.Sprintf("call:%d:arg:%d:send:%s", report.Point, arg.Index, report.Target.Key()),
		).WithLabel(callArgumentSubjectLabel(arg)),
		Actual:   judgment.NewValueRef(arg.ValueHash, arg.TypeWithPresence).WithLabel(report.Verdict.String()),
		Verdict:  verdict,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, arg.Span)},
	}
}

func sendSafetyBirthSpanOrArg(report readmodel.SendSafety) readmodel.SourceSpan {
	if report.HasBirthSpan {
		return report.BirthSpan
	}
	return report.Argument.Span
}

func sendSafetyEvidenceLabel(report readmodel.SendSafety) string {
	label := report.Verdict.String()
	if report.HasIdentity {
		label += " identity=" + report.Identity.String()
	}
	if report.HasPlacement {
		label += " placement=" + report.Placement.String()
	}
	if report.Frozen {
		label += " frozen"
	}
	return label
}
