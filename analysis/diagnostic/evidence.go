package diagnostic

import (
	"fmt"
	"strings"
)

// TrustKind classifies whether evidence is analytical proof or a weaker fact.
type TrustKind int

const (
	TrustProven TrustKind = iota
	TrustClaimed
	TrustRefuted
	TrustUnknown
)

func (k TrustKind) String() string {
	switch k {
	case TrustProven:
		return "proven"
	case TrustClaimed:
		return "claimed"
	case TrustRefuted:
		return "refuted"
	case TrustUnknown:
		return "unknown"
	default:
		return "trust(unknown)"
	}
}

// EvidenceKind classifies the source of one diagnostic explanation item.
type EvidenceKind int

const (
	EvidenceAbstractFact EvidenceKind = iota
	EvidenceUserAssertion
	EvidenceMissingProof
	EvidencePrecisionBoundary
)

func (k EvidenceKind) String() string {
	switch k {
	case EvidenceAbstractFact:
		return "abstract fact"
	case EvidenceUserAssertion:
		return "user assertion"
	case EvidenceMissingProof:
		return "missing proof"
	case EvidencePrecisionBoundary:
		return "unvalidated value"
	default:
		return "evidence(unknown)"
	}
}

// EvidenceReason captures the producer-side reason for an evidence item.
// It is intentionally separate from Message so diagnostic assembly can stay
// structured even when user-facing wording changes.
type EvidenceReason int

const (
	EvidenceReasonUnspecified EvidenceReason = iota
	EvidenceReasonBoundaryValidationMissing
	EvidenceReasonIndexReadValidationMissing
	EvidenceReasonExplicitBoundaryValidation
	EvidenceReasonUserTypeAssertion
	EvidenceReasonUserAssertedAny
	EvidenceReasonUserAssertedNonNil
)

func (r EvidenceReason) String() string {
	switch r {
	case EvidenceReasonUnspecified:
		return "unspecified"
	case EvidenceReasonBoundaryValidationMissing:
		return "boundary validation missing"
	case EvidenceReasonIndexReadValidationMissing:
		return "index read validation missing"
	case EvidenceReasonExplicitBoundaryValidation:
		return "explicit boundary validation"
	case EvidenceReasonUserTypeAssertion:
		return "user type assertion"
	case EvidenceReasonUserAssertedAny:
		return "user asserted any"
	case EvidenceReasonUserAssertedNonNil:
		return "user asserted non-nil"
	default:
		return "reason(unknown)"
	}
}

// Evidence is one fact used to explain why a diagnostic was produced.
type Evidence struct {
	Kind   EvidenceKind
	Trust  TrustKind
	Reason EvidenceReason
	// File overrides the diagnostic's primary file for this evidence item.
	// Leave empty only for evidence whose span is in the primary diagnostic file.
	File    string
	Span    Span
	Message string
}

// Explanation is a copy-safe diagnostic explanation value.
type Explanation struct {
	evidence []Evidence
}

// NewExplanation builds an explanation from evidence items.
func NewExplanation(items ...Evidence) Explanation {
	return Explanation{evidence: append([]Evidence(nil), items...)}
}

// Evidence returns a defensive copy of explanation items.
func (e Explanation) Evidence() []Evidence {
	return append([]Evidence(nil), e.evidence...)
}

// String renders explanation items deterministically.
func (e Explanation) String() string {
	if len(e.evidence) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.evidence))
	for _, item := range e.evidence {
		parts = append(parts, formatEvidence(item))
	}
	return strings.Join(parts, "\n")
}

func formatEvidence(item Evidence) string {
	message := item.Message
	if message == "" {
		message = fallbackEvidenceMessage(item)
	}
	if !item.Span.Valid() {
		if item.File != "" {
			return fmt.Sprintf("%s: %s", item.File, message)
		}
		return message
	}
	if item.File != "" {
		return fmt.Sprintf("%s:%d:%d: %s", item.File, item.Span.StartLine, item.Span.StartCol, message)
	}
	return fmt.Sprintf("%d:%d: %s", item.Span.StartLine, item.Span.StartCol, message)
}

func fallbackEvidenceMessage(item Evidence) string {
	switch item.Kind {
	case EvidenceAbstractFact:
		switch item.Trust {
		case TrustProven:
			return "analysis proved this fact"
		case TrustRefuted:
			return "analysis refuted this fact"
		default:
			return "analysis recorded this fact"
		}
	case EvidenceUserAssertion:
		return "user-provided assertion needs proof"
	case EvidenceMissingProof:
		return "required proof was not found"
	case EvidencePrecisionBoundary:
		return "value needs validation before use"
	default:
		return "diagnostic evidence is unavailable"
	}
}
