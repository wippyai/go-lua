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

var judgmentDiagnosticRenderers = map[judgment.RenderKey]judgmentDiagnosticRenderer{
	judgment.RenderAdvice:                   renderAdviceJudgmentWithPolicy,
	judgment.RenderAssignment:               renderAssignmentJudgmentWithPolicy,
	judgment.RenderCallArity:                renderCallArityJudgmentWithPolicy,
	judgment.RenderCallCallee:               renderCallCalleeJudgmentWithPolicy,
	judgment.RenderChannelLifecycle:         renderChannelLifecycleJudgmentWithPolicy,
	judgment.RenderChannelSelect:            renderChannelSelectJudgmentWithPolicy,
	judgment.RenderConcatOperand:            renderConcatOperandJudgmentWithPolicy,
	judgment.RenderDeadAssignment:           renderDeadAssignmentJudgmentWithPolicy,
	judgment.RenderDirectCallArgument:       renderDirectCallArgumentJudgmentWithPolicy,
	judgment.RenderDiscriminatedUnion:       renderDiscriminatedUnionJudgmentWithPolicy,
	judgment.RenderFrozenTable:              renderFrozenTableJudgmentWithPolicy,
	judgment.RenderLifecycle:                renderLifecycleJudgmentWithPolicy,
	judgment.RenderMemberRead:               renderMemberReadJudgmentWithPolicy,
	judgment.RenderNonNilAssertion:          renderNonNilAssertionJudgmentWithPolicy,
	judgment.RenderNumericFor:               renderNumericForJudgmentWithPolicy,
	judgment.RenderOptional:                 renderOptionalJudgmentWithPolicy,
	judgment.RenderOptionalAssignmentTarget: renderOptionalAssignmentTargetJudgmentWithPolicy,
	judgment.RenderRedundantCondition:       renderRedundantConditionJudgmentWithPolicy,
	judgment.RenderRegistration:             renderRegistrationJudgmentWithPolicy,
	judgment.RenderResultShape:              renderResultShapeJudgmentWithPolicy,
	judgment.RenderReturn:                   renderReturnJudgmentWithPolicy,
	judgment.RenderSendIsolation:            renderSendIsolationJudgmentWithPolicy,
	judgment.RenderTableDispatch:            renderTableDispatchJudgmentWithPolicy,
	judgment.RenderTypestateInvalid:         renderTypestateInvalidTransitionJudgmentWithPolicy,
	judgment.RenderTypestateRequirement:     renderTypestateRequirementJudgmentWithPolicy,
	judgment.RenderUnresolvedType:           renderUnresolvedTypeJudgmentWithPolicy,
	judgment.RenderUnresolvedValue:          renderUnresolvedValueJudgmentWithPolicy,
	judgment.RenderUnusedLocal:              renderUnusedLocalJudgmentWithPolicy,
}

func renderJudgmentDiagnostics(items []judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return newJudgmentRenderContext().render(items, policy, mode)
}

// RenderJudgments renders semantic judgments without running obligation
// producers. Judgment policy controls disposition, while
// diagnostic policy controls opt-in families, suppression, and severity
// overrides.
func RenderJudgments(items []judgment.Judgment, config Config) []diagnostic.Diagnostic {
	if len(items) == 0 {
		return nil
	}
	judgmentPolicy := config.Judgment.Normalized()
	ctx := newJudgmentRenderContext()
	out := make([]diagnostic.Diagnostic, 0, len(items))
	for _, item := range items {
		spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
		if !ok {
			continue
		}
		render, ok := judgmentDiagnosticRenderers[spec.Render]
		if !ok {
			continue
		}
		d, ok := render(ctx, item, judgmentPolicy.Policy, judgmentPolicy.Strictness)
		if !ok || !config.Policy.Enabled(d.Code, spec.DiagnosticDefault == judgment.DiagnosticDefaultEnabled) {
			continue
		}
		d, ok = config.Policy.ApplyOne(d)
		if !ok {
			continue
		}
		if d.Position.File == "" && len(item.Spans) != 0 {
			d.Position.File = item.Spans[0].File
		}
		out = append(out, d)
	}
	out = diagnostic.Deduplicate(out)
	diagnostic.Sort(out)
	out = applyDiagnosticPrecedence(out, defaultDiagnosticPrecedenceRules())
	out = diagnostic.CoalesceSamePrimary(out)
	return out
}

// EvidenceForJudgment projects the canonical diagnostic evidence for one
// already-solved judgment. It deliberately bypasses diagnostic visibility
// policy: proof consumers need the cause chain even for an opt-in diagnostic.
// It neither runs obligation producers nor performs checker analysis.
func EvidenceForJudgment(item judgment.Judgment) []diagnostic.Evidence {
	spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
	if !ok {
		return nil
	}
	render, ok := judgmentDiagnosticRenderers[spec.Render]
	if !ok {
		return nil
	}
	diagnosticItem, ok := render(newJudgmentRenderContext(), item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		return nil
	}
	return diagnosticItem.Explanation.Evidence()
}

func (ctx judgmentRenderContext) render(items []judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	if len(items) == 0 {
		return nil
	}
	policy = normalizedJudgmentPolicy(policy)
	out := make([]diagnostic.Diagnostic, 0, len(items))
	for _, item := range items {
		spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
		if !ok {
			continue
		}
		render, ok := judgmentDiagnosticRenderers[spec.Render]
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

func diagnosticCodeForJudgment(item judgment.Judgment) diagnostic.Code {
	spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
	if !ok {
		return ""
	}
	return primaryDiagnosticCode(spec)
}

func diagnosticCodeForJudgmentAt(item judgment.Judgment, index int) diagnostic.Code {
	spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
	if !ok || index < 0 || index >= len(spec.DiagnosticCodes) || spec.DiagnosticCodes[index] == "" {
		return ""
	}
	return diagnostic.Code(spec.DiagnosticCodes[index])
}

func diagnosticCodeDeclaredForJudgment(item judgment.Judgment, code diagnostic.Code) bool {
	spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
	if !ok {
		return false
	}
	for _, declared := range spec.DiagnosticCodes {
		if diagnostic.Code(declared) == code {
			return true
		}
	}
	return false
}

func primaryDiagnosticCode(spec judgment.CodeSpec) diagnostic.Code {
	if len(spec.DiagnosticCodes) != 1 || spec.DiagnosticCodes[0] == "" {
		return ""
	}
	return diagnostic.Code(spec.DiagnosticCodes[0])
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

func diagnosticCauseFromJudgmentCause(cause judgment.EvidenceCause) diagnostic.EvidenceCause {
	switch cause.Kind {
	case judgment.EvidenceCauseBirth:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseBirth}
	case judgment.EvidenceCauseFlowAssign:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseFlowAssign}
	case judgment.EvidenceCauseFlowCall:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseFlowCall}
	case judgment.EvidenceCauseClaim:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseClaim}
	case judgment.EvidenceCauseGuard:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseGuard}
	case judgment.EvidenceCauseWiden:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseWiden}
	case judgment.EvidenceCauseJoin:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseJoin}
	case judgment.EvidenceCauseUse:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseUse}
	case judgment.EvidenceCauseDeclaration:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseDeclaration}
	case judgment.EvidenceCauseMissingProof:
		return diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseMissingProof}
	default:
		return diagnostic.EvidenceCause{}
	}
}

func diagnosticCauseFromJudgmentEvidence(evidence judgment.Evidence) diagnostic.EvidenceCause {
	return diagnosticCauseFromJudgmentCause(evidence.Detail.Cause)
}

func diagnosticCauseFromJudgmentDetail(detail judgment.EvidenceDetail) diagnostic.EvidenceCause {
	return diagnosticCauseFromJudgmentCause(detail.Cause)
}

func diagnosticCauseForJudgmentEvidenceKind(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.EvidenceCause {
	if evidence, ok := item.FirstEvidence(kind); ok {
		return diagnosticCauseFromJudgmentEvidence(evidence)
	}
	return diagnostic.EvidenceCause{}
}

func missingProofTrustFromJudgment(verdict judgment.Verdict) diagnostic.TrustKind {
	if verdict == judgment.VerdictRefuted {
		return diagnostic.TrustRefuted
	}
	return diagnostic.TrustUnknown
}
