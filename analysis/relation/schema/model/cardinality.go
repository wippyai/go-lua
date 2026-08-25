package model

// CardinalityKind is the closed delivery-size vocabulary for one semantic
// output.  BoundedMany is always paired with a positive logical bound.
type CardinalityKind uint8

const (
	// InvalidCardinality is the unavailable zero value.
	InvalidCardinality CardinalityKind = iota
	// ExactlyOne requires one output row.
	ExactlyOne
	// Optional allows zero or one output row.
	Optional
	// BoundedMany allows at most Bound rows.
	BoundedMany
)

// String returns the canonical cardinality label.
func (kind CardinalityKind) String() string {
	switch kind {
	case ExactlyOne:
		return "ExactlyOne"
	case Optional:
		return "Optional"
	case BoundedMany:
		return "BoundedMany"
	default:
		return "InvalidCardinality"
	}
}

// Cardinality is an immutable logical output-size contract.  Bound is set
// only for BoundedMany and is never a physical row ordinal.
type Cardinality struct {
	kind  CardinalityKind
	bound uint32
}

// NewCardinality validates a cardinality kind and bound.
func NewCardinality(kind CardinalityKind, bound uint32) (Cardinality, bool) {
	switch kind {
	case ExactlyOne, Optional:
		if bound != 0 {
			return Cardinality{}, false
		}
		return Cardinality{kind: kind}, true
	case BoundedMany:
		if bound == 0 {
			return Cardinality{}, false
		}
		return Cardinality{kind: kind, bound: bound}, true
	default:
		return Cardinality{}, false
	}
}

// Kind returns the cardinality kind.
func (cardinality Cardinality) Kind() CardinalityKind { return cardinality.kind }

// Bound returns the positive bound for BoundedMany.
func (cardinality Cardinality) Bound() (uint32, bool) {
	if !cardinality.Available() || cardinality.kind != BoundedMany {
		return 0, false
	}
	return cardinality.bound, true
}

// Available reports whether cardinality is a complete logical contract.
func (cardinality Cardinality) Available() bool {
	switch cardinality.kind {
	case ExactlyOne, Optional:
		return cardinality.bound == 0
	case BoundedMany:
		return cardinality.bound != 0
	default:
		return false
	}
}

// DenominatorRef identifies the authenticated logical relation/key universe
// that bounds a completion or expansion.  The key must belong to relation;
// no local ordinal or physical arrangement is carried.
type DenominatorRef struct {
	relation RelationID
	key      KeyID
}

// NewDenominatorRef constructs a relation/key reference only when both
// identities are valid and the key is owned by relation.
func NewDenominatorRef(relation RelationID, key KeyID) (DenominatorRef, bool) {
	if !relation.Available() || !key.Available() || key.Relation() != relation {
		return DenominatorRef{}, false
	}
	return DenominatorRef{relation: relation, key: key}, true
}

// Available reports whether ref passed construction.
func (ref DenominatorRef) Available() bool {
	return ref.relation.Available() && ref.key.Available() && ref.key.Relation() == ref.relation
}

// Relation returns the denominator relation.
func (ref DenominatorRef) Relation() RelationID { return ref.relation }

// Key returns the denominator key.
func (ref DenominatorRef) Key() KeyID { return ref.key }

// Owner returns the relation owner that authenticated the reference.
func (ref DenominatorRef) Owner() OwnerID { return ref.relation.Owner() }
