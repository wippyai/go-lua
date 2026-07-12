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
	functionKey := contextFunctionKey(ctx)
	var out []judgment.Judgment
	ctx.Reader.ForEachCall(func(call readmodel.CallSite) bool {
		for _, report := range call.SendSafety {
			if !report.SchemaValid() {
				continue
			}
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

	evidence := judgment.EvidenceChain{
		abstractEvidence(
			report.Point,
			fmt.Sprintf("%s:arg:%d:target:%s", judgment.OriginSendSafety, arg.Index, report.Target.Key()),
			trust,
			judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailSendSafetyFact,
				Cause:        judgment.EvidenceCause{Kind: judgment.EvidenceCauseFlowAssign},
				Message:      report.Reason,
				SubjectLabel: sendSafetyEvidenceLabel(report),
			},
			spanFromReadModel(ctx.SourceFile, arg.Span),
		),
	}
	switch report.Verdict {
	case readmodel.SendSafetyProvenIsolated, readmodel.SendSafetyProvenImmutable:
		origin := judgment.OriginSendIsolationProof
		cause := judgment.EvidenceCauseBirth
		message := "direct literal birth site has no retained graph identity"
		if report.Verdict == readmodel.SendSafetyProvenImmutable {
			origin = judgment.OriginSendImmutableProof
			cause = judgment.EvidenceCauseGuard
			message = "exact identity is frozen before send"
		}
		evidence = append(evidence, provenAbstractEvidence(
			report.Point,
			fmt.Sprintf("%s:arg:%d", origin, arg.Index),
			judgment.EvidenceDetail{
				Cause:   judgment.EvidenceCause{Kind: cause},
				Kind:    judgment.EvidenceDetailSendSafetyProof,
				Message: message,
			},
			spanFromReadModel(ctx.SourceFile, sendSafetyBirthSpanOrArg(report)),
		))
	case readmodel.SendSafetyUnknown:
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceMissingProof,
			Trust: judgment.EvidenceTrustUnknown,
			Origin: judgment.OriginRef{
				Point: report.Point,
				Key:   fmt.Sprintf("send:arg:%d:missing-zero-copy-proof", arg.Index),
			},
			Detail: judgment.EvidenceDetail{
				Cause:   judgment.EvidenceCause{Kind: judgment.EvidenceCauseMissingProof},
				Kind:    judgment.EvidenceDetailSendSafetyBlocker,
				Message: report.Reason,
			},
		})
	}

	return judgment.Judgment{
		Code:  judgment.CodeSendIsolation,
		Point: report.Point,
		Subject: subjectRef(functionKey, judgment.SubjectCallArgument,
			fmt.Sprintf("call:%d:arg:%d:send:%s", report.Point, arg.Index, report.Target.Key()),
			callArgumentSubjectLabel(arg)),
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
