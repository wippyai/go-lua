package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ColumnSlot selects one exact typed cell occurrence from a child row.  The
// nominal ColumnID is retained beside the ordinal so the checker and mount
// can prove the ordinal was not substituted or reinterpreted.
type ColumnSlot struct {
	column model.ColumnID
	cell   uint32
}

// NewColumnSlot constructs a positional output cell declaration.  Bounds,
// duplicate columns, and child membership are checked by the enclosing
// ColumnProject expression.
func NewColumnSlot(column model.ColumnID, cell uint32) ColumnSlot {
	return ColumnSlot{column: column, cell: cell}
}

// Column returns the expected nominal output column.
func (slot ColumnSlot) Column() model.ColumnID { return slot.column }

// Cell returns the exact child cell occurrence to retain.
func (slot ColumnSlot) Cell() uint32 { return slot.cell }

func (slot ColumnSlot) digestBytes() []byte {
	parts := appendColumn(nil, slot.column)
	return appendUint32(parts, slot.cell)
}

// ColumnProjectContract is a closed typed row projection.  It deliberately
// has no target relation, key, callback, or nominal lookup surface: it keeps
// only declared positions from one already-authenticated child tuple.
type ColumnProjectContract struct {
	slots []ColumnSlot
}

// NewColumnProjectContract defensively copies authored slots.  The checker
// validates them against the typed child shape.
func NewColumnProjectContract(slots []ColumnSlot) ColumnProjectContract {
	return ColumnProjectContract{slots: cloneColumnSlots(slots)}
}

// Slots returns the selected output cells in authored order.
func (contract ColumnProjectContract) Slots() []ColumnSlot { return cloneColumnSlots(contract.slots) }

func (contract ColumnProjectContract) digestBytes() []byte {
	parts := appendLength(nil, len(contract.slots))
	for _, slot := range contract.slots {
		parts = appendBytes(parts, slot.digestBytes())
	}
	return parts
}

// ColumnProject retains exactly the contract's ordered child cells.  It is
// the generic carrier needed when an authored alternative supplies a
// destination row/key authority plus only a subset of writable semantic
// columns to a vertical Merge.
type ColumnProject struct {
	child    Expression
	contract ColumnProjectContract
}

// NewColumnProject constructs a closed positional projection without running
// semantic checks.
func NewColumnProject(child Expression, contract ColumnProjectContract) ColumnProject {
	return ColumnProject{child: child, contract: contract}
}

// Child returns the projected relational child.
func (project ColumnProject) Child() Expression { return project.child }

// Contract returns the immutable positional projection contract.
func (project ColumnProject) Contract() ColumnProjectContract { return project.contract }

// Kind implements Expression.
func (project ColumnProject) Kind() Kind { return KindColumnProject }

// Digest returns the deterministic structural identity.
func (project ColumnProject) Digest() identity.ContentID {
	parts := appendExpr(nil, project.child)
	return derive("analysis/relation/schema/algebra/column-project/v1", append(parts, project.contract.digestBytes()...))
}

func (project ColumnProject) expression() {}
