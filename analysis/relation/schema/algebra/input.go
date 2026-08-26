package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// InputProjection records whether an Input consumes the whole authored row or
// one exact, ordered source projection. The distinction is part of the
// expression identity: an explicit whole-row source and an exact projection
// containing the same columns must never alias.
type InputProjection uint8

const (
	InputProjectionInvalid InputProjection = iota
	InputProjectionAllColumns
	InputProjectionExactColumns
)

func (projection InputProjection) Available() bool {
	return projection == InputProjectionAllColumns || projection == InputProjectionExactColumns
}

// Input names one sealed logical relation occurrence. An AllColumns Input is
// retained for direct algebra authors; compiler-produced plans use the exact
// projection constructor so repeated occurrences of one relation can carry
// different source vectors without a relation-keyed side table.
type Input struct {
	relation   model.RelationID
	projection InputProjection
	columns    []model.ColumnID
}

// NewInput constructs an explicit whole-row input expression. Relation
// availability and declaration membership are checker responsibilities.
func NewInput(relation model.RelationID) Input {
	return Input{relation: relation, projection: InputProjectionAllColumns}
}

// NewInputColumns constructs an occurrence-local exact source projection. The
// vector is non-empty, ordered, duplicate-free, and relation-owned; type
// compatibility remains a checker responsibility because the algebra layer
// does not own the declaration catalogue.
func NewInputColumns(relation model.RelationID, columns []model.ColumnID) (Input, bool) {
	if !relation.Available() || len(columns) == 0 {
		return Input{}, false
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		if !column.Available() || column.Relation() != relation {
			return Input{}, false
		}
		if _, duplicate := seen[column]; duplicate {
			return Input{}, false
		}
		seen[column] = struct{}{}
	}
	return Input{relation: relation, projection: InputProjectionExactColumns, columns: cloneColumns(columns)}, true
}

// Relation returns the stable relation reference.
func (input Input) Relation() model.RelationID { return input.relation }

// Projection returns the sealed source projection mode.
func (input Input) Projection() InputProjection { return input.projection }

// AllColumns reports whether this source explicitly consumes the complete
// authored relation row.  It is intentionally a predicate rather than a
// mutable view of the projection vector.
func (input Input) AllColumns() bool { return input.IsAllColumns() }

// ExactColumns returns the immutable ordered source vector.  The bool is
// false for the explicit AllColumns form (and for any unavailable/malformed
// Input value); callers never receive the Input's backing storage.
func (input Input) ExactColumns() ([]model.ColumnID, bool) {
	if !input.IsExactColumns() {
		return nil, false
	}
	return cloneColumns(input.columns), true
}

// IsAllColumns reports whether this explicit algebra source consumes the full
// authored row.
func (input Input) IsAllColumns() bool {
	return input.projection == InputProjectionAllColumns && len(input.columns) == 0
}

// IsExactColumns reports whether this source carries a non-empty exact vector.
func (input Input) IsExactColumns() bool {
	return input.projection == InputProjectionExactColumns && len(input.columns) != 0
}

// Columns returns the exact ordered projection. It is nil for AllColumns.
func (input Input) Columns() []model.ColumnID { return cloneColumns(input.columns) }

// Available reports whether the expression's source contract is structurally
// sealed. Declaration membership and typed compatibility remain checker laws.
func (input Input) Available() bool {
	if !input.relation.Available() {
		return false
	}
	if input.IsAllColumns() {
		return true
	}
	if !input.IsExactColumns() {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(input.columns))
	for _, column := range input.columns {
		if !column.Available() || column.Relation() != input.relation {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

// Kind implements Expression.
func (input Input) Kind() Kind { return KindInput }

// Digest returns the deterministic structural identity.
func (input Input) Digest() identity.ContentID {
	parts := appendRelation(nil, input.relation)
	parts = append(parts, byte(input.projection))
	parts = appendColumns(parts, input.columns)
	return derive("analysis/relation/schema/algebra/input/v2", parts)
}

func (input Input) expression() {}
