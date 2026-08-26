package typing

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// shape is the checker-only typed row shape. A derived Join can have no
// nominal relation until a Project gives it one, so relation is optional;
// columns and their TypeIDs remain fully nominal and checked.
type shape struct {
	relation model.RelationID
	columns  []columnType
	// sources is the ordered row-occurrence layout of the tuple this shape
	// describes. One Join may retain two source rows of the same relation, so
	// this cannot be recovered from a relation set.
	sources []model.RelationID
	keys    []model.KeyID
	// rowKeys are owner-authenticated row identities retained independently
	// of the delivered cell vector. A narrow projection may remove the key
	// cells while preserving which owner row supplied the remaining payload.
	// Ordinary tuple reduction must still use keys; proposal-alternative Merge
	// may use rowKeys because Apply owns the destination proposal lease.
	rowKeys []model.KeyID
	// proposal is true only for an Apply result and the closed operators that
	// preserve its application sidecar (currently ColumnProject and Merge).
	proposal bool
	// rangeBound is true only when the expression's evaluator is required to
	// preserve one exact ordered range boundary. A nominal relation key alone
	// is not this proof: Input/Join/Merge may expose the same key while still
	// delivering an incidental flattened row stream.
	rangeBound bool
	// completeDenominator is present only when the currently preserved range
	// is a Complete range. It lets the cold checker distinguish a complete
	// closed-world delivery from an arbitrary grouped/bounded range and prove
	// that its declared order is the denominator order the physical operator
	// must redeem.
	completeDenominator model.DenominatorRef
}

type columnType struct {
	ID   model.ColumnID
	Type model.TypeID
	// Source identifies the row occurrence in shape.sources that owns this
	// cell. It is carried to the runtime by tuple.Cell, not reconstructed by a
	// nominal ColumnID lookup.
	Source     uint32
	Occurrence readOccurrence
}

// readOccurrence is checker-local identity for one relation read inside one
// sealed expression root.  ColumnID remains the schema owner's nominal
// identity; this qualifier only prevents the checker from collapsing two
// occurrences of the same relation into one output family.  The ordinal is
// assigned while walking the immutable expression tree in pre-order, never
// from a diagnostic path or a physical address.
type readOccurrence struct {
	root    model.ExpressionID
	ordinal uint32
}

func (value readOccurrence) available() bool {
	return value.root.Available()
}

func (value readOccurrence) equal(other readOccurrence) bool {
	return value.root == other.root && value.ordinal == other.ordinal
}

type outputIdentity struct {
	Relation model.RelationID
	Column   model.ColumnID
}

func (value shape) valid() bool { return len(value.columns) > 0 }

func (value shape) column(id model.ColumnID) (columnType, bool) {
	for _, column := range value.columns {
		if column.ID == id {
			return column, true
		}
	}
	return columnType{}, false
}

func (value shape) cell(index uint32) (columnType, bool) {
	if int(index) >= len(value.columns) {
		return columnType{}, false
	}
	return value.columns[index], true
}

func (value shape) source(index uint32) (model.RelationID, bool) {
	if int(index) >= len(value.sources) {
		return model.RelationID{}, false
	}
	return value.sources[index], true
}

func (value shape) hasKey(id model.KeyID) bool {
	for _, key := range value.keys {
		if key == id {
			return true
		}
	}
	return false
}

func (value shape) hasRowKey(id model.KeyID) bool {
	for _, key := range value.rowKeys {
		if key == id {
			return true
		}
	}
	return false
}

func sameShape(left, right shape) bool {
	if left.relation != right.relation || len(left.columns) != len(right.columns) || len(left.sources) != len(right.sources) {
		return false
	}
	for index := range left.sources {
		if left.sources[index] != right.sources[index] {
			return false
		}
	}
	for index := range left.columns {
		// Occurrence is a checker-side provenance qualifier. Merge inputs
		// still have one nominal typed row shape even when their expressions
		// reached it through different read occurrences.
		if left.columns[index].ID != right.columns[index].ID || left.columns[index].Type != right.columns[index].Type || left.columns[index].Source != right.columns[index].Source {
			return false
		}
	}
	return true
}

func uniqueColumnCount(columns []columnType) int {
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		seen[column.ID] = struct{}{}
	}
	return len(seen)
}
