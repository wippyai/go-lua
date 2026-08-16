package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

type returnPresentation struct {
	Subject  string
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) Return(item judgment.Judgment, label string, sourceName string, got, want typ.Type, primary diagnostic.Span) returnPresentation {
	proof := item.ReturnProof()
	subject := returnJudgmentSubject(label, sourceName)
	declSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceUserAssertion)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceAbstractFact),
			Span:    diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceAbstractFact),
			Message: display.SourceTypeEvidence(subject, got),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceUserAssertion),
			Span:    declSpan,
			Message: display.ReturnDeclaredTypeEvidence(label, want),
		},
	}
	evidence = append(evidence, returnJudgmentExtraEvidence(item, proof, subject, got, primary)...)
	if proof.MayBeNil {
		evidence = append(evidence, nilabilityProvenanceEvidence(item, sourceName, got, primary)...)
	}
	return returnPresentation{
		Subject:  subject,
		Message:  returnJudgmentMessage(subject, sourceName, label, got, want, proof),
		Help:     returnJudgmentHelp(sourceName, got, proof),
		Evidence: evidence,
		Labels: []diagnostic.Label{
			sourceLabel(primary, labelReturnedValue),
			sourceLabel(declSpan, labelDeclaredReturn),
		},
	}
}

func returnJudgmentSubject(label, sourceName string) string {
	if sourceName == "" {
		return label
	}
	if sourceName == label {
		return label
	}
	if strings.HasPrefix(sourceName, label+" (") {
		return sourceName
	}
	return fmt.Sprintf("%s (%s)", label, sourceName)
}

func returnJudgmentMessage(subject, sourceName, label string, got, want typ.Type, proof judgment.ReturnProofSummary) string {
	if proof.PrecisionBoundary && !proof.MayBeNil {
		return fmt.Sprintf("%s comes from any/unknown; no proof shows it satisfies declared return type %s", subject, display.Type(want))
	}
	if proof.MayBeNil && (!typ.Nil.Equals(got) || proof.IndexedRead) {
		if sourceName != "" && sourceName != label {
			return fmt.Sprintf("cannot return %s as %s because it may be nil", sourceName, label)
		}
		return fmt.Sprintf("cannot return %s because it may be nil", subject)
	}
	gotText := display.Type(got)
	wantText := display.Type(want)
	if gotText == wantText {
		return fmt.Sprintf("cannot prove %s satisfies declared return type %s", subject, wantText)
	}
	if subject == "" {
		subject = label
	}
	return fmt.Sprintf("%s is %s, not %s", subject, gotText, wantText)
}

func returnJudgmentHelp(sourceName string, got typ.Type, proof judgment.ReturnProofSummary) string {
	if sourceName != "" && sourceName != unknownSourceName && proof.MayBeNil && (!typ.Nil.Equals(got) || proof.IndexedRead) {
		return fmt.Sprintf("Guard `%s` with a nil check, return a default value, or change the return type to accept nil.", sourceName)
	}
	return "Return a value compatible with the declared return type, or change the return annotation if the returned value is valid."
}

func returnJudgmentExtraEvidence(item judgment.Judgment, proof judgment.ReturnProofSummary, subject string, got typ.Type, primary diagnostic.Span) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.Evidence {
		switch evidence.Kind {
		case judgment.EvidenceUserAssertion:
			if evidence.Detail.Kind == judgment.EvidenceDetailUserAssertedAny {
				out = append(out, diagnostic.Evidence{
					Kind:    diagnostic.EvidenceUserAssertion,
					Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed),
					Reason:  diagnostic.EvidenceReasonUserAssertedAny,
					Cause:   diagnosticCauseFromJudgmentEvidence(evidence),
					Span:    diagnosticSpanFromJudgment(evidence.Span),
					Message: display.UserAssertedAnyEvidence(),
				})
			}
		case judgment.EvidencePrecisionBoundary:
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidencePrecisionBoundary,
				Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustUnknown),
				Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
				Cause:   diagnosticCauseFromJudgmentEvidence(evidence),
				Span:    diagnosticJudgmentEvidenceSpanOr(evidence, primary),
				Message: display.ReturnExplicitBoundaryProofMessage(subject),
			})
		}
	}
	if !typ.IsAny(got) && !typ.IsUnknown(got) && !proof.PrecisionBoundary && !proof.MayBeNil {
		return out
	}
	missingReason := diagnostic.EvidenceReasonBoundaryValidationMissing
	missingMessage := display.ReturnMissingProofMessage(subject)
	if proof.IndexedRead {
		missingReason = diagnostic.EvidenceReasonIndexReadValidationMissing
		missingMessage = display.ReturnIndexedReadProofMessage(subject)
	}
	out = append(out, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   missingProofTrustFromJudgment(item.Verdict),
		Reason:  missingReason,
		Cause:   diagnostic.EvidenceCause{Kind: diagnostic.EvidenceCauseMissingProof},
		Span:    primary,
		Message: missingMessage,
	})
	return out
}
