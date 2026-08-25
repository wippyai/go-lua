package arrangement

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Access is one logical access requirement.  It names a relation and, when
// present, the logical key and ordered column vector used by that access.
// Neither a local ordinal nor an index implementation is part of Access.
//
// A vector is retained in authored order.  In particular, the two vectors of
// an oriented Join are different requirements when their order differs; mount
// never turns an authored correspondence into an unordered set.
type Access struct {
	relation model.RelationID
	key      model.KeyID
	columns  []model.ColumnID
}

// newAccess constructs a logical access over relation, key, and an optional
// ordered vector. The public constructors below keep the three distinct
// requirement forms explicit.
func newAccess(relation model.RelationID, key model.KeyID, columns []model.ColumnID) (Access, bool) {
	if !relation.Available() {
		return Access{}, false
	}
	if key.Available() && key.Relation() != relation {
		return Access{}, false
	}
	columns = append([]model.ColumnID(nil), columns...)
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		if !column.Available() || column.Relation() != relation {
			return Access{}, false
		}
		if _, exists := seen[column]; exists {
			return Access{}, false
		}
		seen[column] = struct{}{}
	}
	return Access{relation: relation, key: key, columns: columns}, true
}

// NewRelationAccess constructs the relation-scan form of Access.
func NewRelationAccess(relation model.RelationID) (Access, bool) {
	return newAccess(relation, model.KeyID{}, nil)
}

// NewKeyAccess constructs a key access.  The key's owning relation is the
// relation carried by the requirement.
func NewKeyAccess(key model.KeyID) (Access, bool) {
	if !key.Available() {
		return Access{}, false
	}
	return newAccess(key.Relation(), key, nil)
}

// NewVectorAccess constructs an ordered logical column-vector access.
func NewVectorAccess(relation model.RelationID, columns []model.ColumnID) (Access, bool) {
	return newAccess(relation, model.KeyID{}, columns)
}

// Relation returns the logical relation named by the requirement.
func (access Access) Relation() model.RelationID { return access.relation }

// Key returns the logical key, or the zero key for an unkeyed scan/vector.
func (access Access) Key() model.KeyID { return access.key }

// Columns returns a defensive copy of the authored logical vector.
func (access Access) Columns() []model.ColumnID {
	return append([]model.ColumnID(nil), access.columns...)
}

// Available reports whether the logical requirement is complete.
func (access Access) Available() bool {
	if !access.relation.Available() {
		return false
	}
	if access.key.Available() && access.key.Relation() != access.relation {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(access.columns))
	for _, column := range access.columns {
		if !column.Available() || column.Relation() != access.relation {
			return false
		}
		if _, exists := seen[column]; exists {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

// Equal compares logical content, including the authored ordered vector.
// Physical coordinates and semantic delivery shapes never participate.
func (access Access) Equal(other Access) bool {
	if access.relation != other.relation || access.key != other.key || len(access.columns) != len(other.columns) {
		return false
	}
	for index := range access.columns {
		if access.columns[index] != other.columns[index] {
			return false
		}
	}
	return true
}

// accessLess is the canonical logical order.  Keys and vectors are compared
// lexicographically; vectors are never sorted internally.
func accessLess(left, right Access) bool {
	if compared := compareRelation(left.relation, right.relation); compared != 0 {
		return compared < 0
	}
	if compared := compareKey(left.key, right.key); compared != 0 {
		return compared < 0
	}
	if compared := compareColumns(left.columns, right.columns); compared != 0 {
		return compared < 0
	}
	return false
}

func compareRelation(left, right model.RelationID) int {
	return compareNominal(left.Owner().Content(), left.Content(), right.Owner().Content(), right.Content())
}

func compareKey(left, right model.KeyID) int {
	if !left.Available() && !right.Available() {
		return 0
	}
	if !left.Available() {
		return -1
	}
	if !right.Available() {
		return 1
	}
	return compareNominal(left.Relation().Owner().Content(), left.Content(), right.Relation().Owner().Content(), right.Content())
}

func compareColumn(left, right model.ColumnID) int {
	return compareNominal(left.Relation().Owner().Content(), left.Content(), right.Relation().Owner().Content(), right.Content())
}

func compareColumns(left, right []model.ColumnID) int {
	for index := 0; index < len(left) && index < len(right); index++ {
		if compared := compareColumn(left[index], right[index]); compared != 0 {
			return compared
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareNominal(leftOwner, leftContent, rightOwner, rightContent identity.ContentID) int {
	if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
		return compared
	}
	return bytes.Compare(leftContent[:], rightContent[:])
}
