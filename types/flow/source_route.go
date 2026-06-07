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
// Flow owns storage normalization; callers own domain-specific interpretation of
// the route, such as mapping iterator edges to parameter evidence.
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

func canonicalSourceRoute(sourceKey constraint.PathKey, remainder []constraint.Segment) (sourceRoute, bool) {
	source, ok := StableAddressFromCanonicalKey(sourceKey)
	if !ok {
		return sourceRoute{}, false
	}
	return sourceRoute{
		source:    source,
		remainder: cloneAddressSegments(remainder),
	}, true
}

func appendCanonicalSourceRoute(out []sourceRoute, sourceKey constraint.PathKey, remainder []constraint.Segment) []sourceRoute {
	route, ok := canonicalSourceRoute(sourceKey, remainder)
	if !ok {
		return out
	}
	return append(out, route)
}

func (r sourceRoute) appendedSource() (StableAddress, bool) {
	return r.source.Append(r.remainder)
}
