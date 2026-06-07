package flow

import "github.com/wippyai/go-lua/types/constraint"

// ProvenanceRouteKind identifies the semantic edge used to reach a source value.
type ProvenanceRouteKind uint8

const (
	ProvenanceRouteIdentityAlias ProvenanceRouteKind = iota + 1
	ProvenanceRouteIndexedIterator
	ProvenanceRouteKeyedIterator
	ProvenanceRouteAppendElementField
)

// ProvenanceRoute is the structured-path view of a point-local provenance edge.
// Flow owns storage normalization; domain/provenance owns route interpretation.
type ProvenanceRoute struct {
	Kind           ProvenanceRouteKind
	Source         constraint.Path
	Remainder      []constraint.Segment
	VarIndex       int
	SourceField    []constraint.Segment
	FieldRemainder []constraint.Segment
}

// sourceRoute is the canonical address view of a provenance source plus the
// suffix consumed below the originating value.
type sourceRoute struct {
	source    StableAddress
	remainder []constraint.Segment
}

func sourceRouteOf(source StableAddress, remainder []constraint.Segment) (sourceRoute, bool) {
	if source.Key() == "" {
		return sourceRoute{}, false
	}
	return sourceRoute{
		source:    source,
		remainder: cloneAddressSegments(remainder),
	}, true
}

func (r sourceRoute) appendedSource() (StableAddress, bool) {
	return r.source.Append(r.remainder)
}
