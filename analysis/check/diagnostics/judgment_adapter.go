package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type judgmentRenderContext struct {
	proof ProofContext
}

func newJudgmentRenderContext() judgmentRenderContext {
	return judgmentRenderContext{proof: NewProofContext()}
}

type judgmentDiagnosticRenderer func(judgmentRenderContext, judgment.Judgment, judgment.Policy, judgment.StrictnessMode) (diagnostic.Diagnostic, bool)

var judgmentDiagnosticRenderers = map[judgment.Code]judgmentDiagnosticRenderer{
	judgment.CodeCallArgType:             renderDirectCallArgumentJudgmentWithPolicy,
	judgment.CodeCallArity:               renderCallArityJudgmentWithPolicy,
	judgment.CodeCallCallee:              renderCallCalleeJudgmentWithPolicy,
	judgment.CodeAssignment:              renderAssignmentJudgmentWithPolicy,
	judgment.CodeAssignmentTarget:        renderOptionalAssignmentTargetJudgmentWithPolicy,
	judgment.CodeReturn:                  renderReturnJudgmentWithPolicy,
	judgment.CodeNonNilAssertion:         renderNonNilAssertionJudgmentWithPolicy,
	judgment.CodeNumericForOperand:       renderNumericForJudgmentWithPolicy,
	judgment.CodeFrozenTable:             renderFrozenTableJudgmentWithPolicy,
	judgment.CodeLifecycle:               renderLifecycleJudgmentWithPolicy,
	judgment.CodeUnusedLocal:             renderUnusedLocalJudgmentWithPolicy,
	judgment.CodeDeadAssignment:          renderDeadAssignmentJudgmentWithPolicy,
	judgment.CodeChannelSelect:           renderChannelSelectJudgmentWithPolicy,
	judgment.CodeDiscriminatedUnion:      renderDiscriminatedUnionJudgmentWithPolicy,
	judgment.CodeOptional:                renderOptionalJudgmentWithPolicy,
	judgment.CodeResultShape:             renderResultShapeJudgmentWithPolicy,
	judgment.CodeRegistration:            renderRegistrationJudgmentWithPolicy,
	judgment.CodeTableDispatch:           renderTableDispatchJudgmentWithPolicy,
	judgment.CodeUnresolvedValue:         renderUnresolvedValueJudgmentWithPolicy,
	judgment.CodeUnresolvedType:          renderUnresolvedTypeJudgmentWithPolicy,
	judgment.CodeRedundantCondition:      renderRedundantConditionJudgmentWithPolicy,
	judgment.CodeMemberRead:              renderMemberReadJudgmentWithPolicy,
	judgment.CodeConcatOperand:           renderConcatOperandJudgmentWithPolicy,
	judgment.CodeSendIsolation:           renderSendIsolationJudgmentWithPolicy,
	judgment.CodeAdviceRedundantClaim:    renderAdviceRedundantClaimJudgmentWithPolicy,
	judgment.CodeAdviceAlwaysTrueGuard:   renderAdviceAlwaysTrueGuardJudgmentWithPolicy,
	judgment.CodeAdviceInvariantLoopRead: renderAdviceInvariantLoopReadJudgmentWithPolicy,
}

func renderJudgmentDiagnostics(items []judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return newJudgmentRenderContext().render(items, policy, mode)
}

func (ctx judgmentRenderContext) render(items []judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	if len(items) == 0 {
		return nil
	}
	policy = normalizedJudgmentPolicy(policy)
	out := make([]diagnostic.Diagnostic, 0, len(items))
	for _, item := range items {
		render, ok := judgmentDiagnosticRenderers[item.Code]
		if !ok {
			continue
		}
		if d, ok := render(ctx, item, policy, mode); ok {
			out = append(out, d)
		}
	}
	return out
}

func diagnosticSeverityForJudgment(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Severity, bool) {
	level, ok := normalizedJudgmentPolicy(policy).LevelFor(item, mode)
	if !ok {
		return diagnostic.SeverityHint, false
	}
	switch level {
	case judgment.LevelDisabled:
		return diagnostic.SeverityHint, false
	case judgment.LevelError:
		return diagnostic.SeverityError, true
	case judgment.LevelWarning:
		return diagnostic.SeverityWarning, true
	case judgment.LevelHint:
		return diagnostic.SeverityHint, true
	default:
		return diagnostic.SeverityHint, false
	}
}

func normalizedJudgmentPolicy(policy judgment.Policy) judgment.Policy {
	if policy.IsZero() {
		return judgment.DefaultPolicy()
	}
	return policy
}

func diagnosticSpanFromJudgment(span judgment.SpanRef) diagnostic.Span {
	return diagnostic.Span{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func diagnosticSpanFromJudgmentEvidence(evidence judgment.Evidence) (diagnostic.Span, bool) {
	if evidence.Span.StartLine == 0 {
		return diagnostic.Span{}, false
	}
	return diagnosticSpanFromJudgment(evidence.Span), true
}

func diagnosticEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) (diagnostic.Span, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != kind {
			continue
		}
		if span, ok := diagnosticSpanFromJudgmentEvidence(evidence); ok {
			return span, true
		}
	}
	return diagnostic.Span{}, false
}

func diagnosticEvidenceSpanOr(item judgment.Judgment, kind judgment.EvidenceKind, fallback diagnostic.Span) diagnostic.Span {
	if span, ok := diagnosticEvidenceSpan(item, kind); ok {
		return span
	}
	return fallback
}

func diagnosticEvidenceSpanOrPrimary(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	if len(item.Spans) == 0 {
		return diagnostic.Span{}
	}
	return diagnosticEvidenceSpanOr(item, kind, diagnosticSpanFromJudgment(item.Spans[0]))
}

func diagnosticJudgmentEvidenceSpanOr(evidence judgment.Evidence, fallback diagnostic.Span) diagnostic.Span {
	if span, ok := diagnosticSpanFromJudgmentEvidence(evidence); ok {
		return span
	}
	return fallback
}

func diagnosticTrustFromJudgmentEvidence(item judgment.Judgment, kind judgment.EvidenceKind, fallback diagnostic.TrustKind) diagnostic.TrustKind {
	if trust, ok := item.EvidenceTrustFor(kind); ok {
		return diagnosticTrustFromJudgmentTrust(trust, fallback)
	}
	return fallback
}

func diagnosticTrustFromJudgmentTrust(trust judgment.EvidenceTrust, fallback diagnostic.TrustKind) diagnostic.TrustKind {
	switch trust {
	case judgment.EvidenceTrustProven:
		return diagnostic.TrustProven
	case judgment.EvidenceTrustClaimed:
		return diagnostic.TrustClaimed
	case judgment.EvidenceTrustRefuted:
		return diagnostic.TrustRefuted
	case judgment.EvidenceTrustUnknown:
		return diagnostic.TrustUnknown
	default:
		return fallback
	}
}

func missingProofTrustFromJudgment(verdict judgment.Verdict) diagnostic.TrustKind {
	if verdict == judgment.VerdictRefuted {
		return diagnostic.TrustRefuted
	}
	return diagnostic.TrustUnknown
}
