package algebra

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// MergeContract declares the key used to combine alternative derivations.
//
// Merge intentionally carries no reducer identity or callback.  The output
// relation's column schemas are the sole authority for value types, and the
// corresponding typed binding owns Join/LessOrEqual for each TypeID.  Widen
// is a plan recurrence policy, not a property of a merge expression.  Keeping
// those concerns out of this contract prevents a second semantic registry
// from disagreeing with the column declarations.
type MergeContract struct {
	key model.KeyID
}

// NewMergeContract constructs a merge declaration without checking key
// compatibility.  The checker later resolves the output relation and its
// column TypeIDs from the surrounding schema.
func NewMergeContract(key model.KeyID) MergeContract {
	return MergeContract{key: key}
}

// Key returns the merge key.
func (contract MergeContract) Key() model.KeyID { return contract.key }

func (contract MergeContract) digestBytes() []byte {
	return appendKey(nil, contract.key)
}

// GroupContract declares grouping key and canonical delivered cardinality. A
// reduction is an explicit Apply expression rather than a second operation
// identity.
type GroupContract struct {
	key         model.KeyID
	cardinality model.Cardinality
}

// NewGroupContract constructs a group declaration without checker rules.
func NewGroupContract(key model.KeyID, cardinality model.Cardinality) GroupContract {
	return GroupContract{key: key, cardinality: cardinality}
}

// Key returns the grouping key.
func (contract GroupContract) Key() model.KeyID { return contract.key }

// Cardinality returns the canonical delivered cardinality.
func (contract GroupContract) Cardinality() model.Cardinality { return contract.cardinality }

func (contract GroupContract) digestBytes() []byte {
	parts := appendKey(nil, contract.key)
	return appendCardinality(parts, contract.cardinality)
}
