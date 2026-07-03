package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type judgmentDiagnosticRenderer func(judgment.Judgment, judgment.Policy, judgment.StrictnessMode) (diagnostic.Diagnostic, bool)

var judgmentDiagnosticRenderers = map[judgment.Code]judgmentDiagnosticRenderer{
	judgment.CodeCallArgType:      renderDirectCallArgumentJudgmentWithPolicy,
	judgment.CodeCallArity:        renderDirectCallArityJudgmentWithPolicy,
	judgment.CodeCallCallee:       renderDirectCallCalleeJudgmentWithPolicy,
	judgment.CodeAssignment:       renderAssignmentJudgmentWithPolicy,
	judgment.CodeAssignmentTarget: renderOptionalAssignmentTargetJudgmentWithPolicy,
}

func renderJudgmentDiagnostics(items []judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
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
		if d, ok := render(item, policy, mode); ok {
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
