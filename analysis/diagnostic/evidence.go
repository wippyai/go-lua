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
		return "precision boundary"
	default:
		return "evidence(unknown)"
	}
}

// Evidence is one fact used to explain why a diagnostic was produced.
type Evidence struct {
	Kind    EvidenceKind
	Trust   TrustKind
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
		message = fmt.Sprintf("%s evidence is %s", item.Kind, item.Trust)
	}
	if !item.Span.Valid() {
		return message
	}
	return fmt.Sprintf("%d:%d: %s", item.Span.StartLine, item.Span.StartCol, message)
}
