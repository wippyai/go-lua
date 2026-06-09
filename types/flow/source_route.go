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

// EvidenceLabel describes the route as read-only diagnostic evidence. Flow owns
// the route vocabulary, so callers do not need to switch on provenance kinds.
func (r ProvenanceRoute) EvidenceLabel(target constraint.Path) string {
	source := r.Source.String()
	if source == "" {
		source = "unknown source"
	}
	switch r.Kind {
	case ProvenanceRouteIdentityAlias:
		return target.String() + " aliases " + source
	case ProvenanceRouteIndexedIterator:
		return target.String() + " comes from indexed iterator source " + source
	case ProvenanceRouteKeyedIterator:
		return target.String() + " comes from keyed iterator source " + source
	case ProvenanceRouteAppendElementField:
		return target.String() + " comes from appended element source " + source
	default:
		return target.String() + " comes from " + source
	}
}

// AppendElementFieldRouteQueryForPath converts a field path into the query used
// to explain append-element field provenance.
func AppendElementFieldRouteQueryForPath(path constraint.Path) (AppendElementFieldRouteQuery, bool) {
	if path.IsEmpty() || path.Symbol == 0 || len(path.Segments) == 0 {
		return AppendElementFieldRouteQuery{}, false
	}
	array := path
	array.Segments = append([]constraint.Segment(nil), path.Segments[:len(path.Segments)-1]...)
	field := append([]constraint.Segment(nil), path.Segments[len(path.Segments)-1:]...)
	return AppendElementFieldRouteQuery{
		ArrayPath: array,
		Field:     field,
	}, true
}

// sourceRoute is the canonical address view of a provenance source plus the
// suffix consumed below the originating value.
type sourceRoute struct {
	kind      ProvenanceRouteKind
	source    StableAddress
	remainder []constraint.Segment
	varIndex  int
}

// addressSuffixKey is the comparable identity for one address plus a structured
// suffix. It is used for route de-duplication without string-concatenating
// address and segment encodings at call sites.
type addressSuffixKey struct {
	address constraint.PathKey
	suffix  constraint.PathKey
}

func sourceRouteOfKind(kind ProvenanceRouteKind, source StableAddress, remainder []constraint.Segment, varIndex int) (sourceRoute, bool) {
	if kind == 0 || source.Key() == "" {
		return sourceRoute{}, false
	}
	return sourceRoute{
		kind:      kind,
		source:    source,
		remainder: cloneAddressSegments(remainder),
		varIndex:  varIndex,
	}, true
}

func addressSuffixIdentity(address StableAddress, suffix []constraint.Segment) (addressSuffixKey, bool) {
	key := address.Key()
	if key == "" {
		return addressSuffixKey{}, false
	}
	return addressSuffixKey{
		address: key,
		suffix:  constraint.PathKey(constraint.FormatSegments(suffix)),
	}, true
}

func (r sourceRoute) appendedSource() (StableAddress, bool) {
	return r.source.Append(r.remainder)
}
