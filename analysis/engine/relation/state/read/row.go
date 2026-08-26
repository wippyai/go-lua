package read

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Row is one borrowed extensional relation row. It is intentionally an
// interface with an unexported marker: only a Reader bound to a committed
// aggregate can issue rows, so a sibling package cannot manufacture a
// relation/scope/lineage/cell combination and pass it to an operator.
// Consumers must not retain a row after the callback returns. ID is the
// owner-issued logical identity; Key is only the arrangement-local physical
// coordinate used by the state implementation.
type Row interface {
	Available() bool
	ID() model.RowID
	Key() geometry.Key
	Scope() witness.Scope
	Lineage() model.LineageRef
	Cells() []Cell
	// CellAt borrows one already-owned full-vector cell without copying the
	// row's cell slice. Callers must first redeem the Row through Reader.Owns.
	CellAt(index int) (Cell, bool)
	rowFrom(*reader) bool
}

// Cell is the logical projection of one authenticated state fiber.  Physical
// support partitions are normalized by the Reader before a Cell is issued;
// callers receive only the mounted Scope and cannot inspect or retain a
// support.Mask/ReadPart.  The owner marker keeps a copied cell tied to the
// committed Reader that redeemed it.
type Cell struct {
	owner    *reader
	column   model.ColumnID
	typeID   model.TypeID
	value    binding.ValueToken
	presence model.Presence
	scope    witness.Scope
	lineage  model.LineageRef
}

// Available authenticates the complete logical cell.  A zero or foreign
// owner cannot be turned into a usable cell by a sibling package because all
// issuance fields are private and the owner validates the exact fence.
func (cell Cell) Available() bool {
	if cell.owner == nil || !cell.owner.available() || !cell.column.Available() || !cell.typeID.Available() || cell.column.Relation() != cell.owner.layout.Access().Relation() || !cell.presence.Available() || cell.presence.Is(model.Refused) || !cell.scope.ValidFor(cell.owner.fence) || !cell.lineage.Available() || !cell.owner.lineageAuthority.Validate(cell.lineage) {
		return false
	}
	if cell.value.Available() {
		return cell.value.ValidFor(cell.owner.fence) && cell.value.Type() == cell.typeID
	}
	return !cell.presence.Is(model.Present) && !cell.presence.Is(model.AuthenticatedOpaque)
}

func (cell Cell) Column() model.ColumnID    { return cell.column }
func (cell Cell) Type() model.TypeID        { return cell.typeID }
func (cell Cell) Value() binding.ValueToken { return cell.value }
func (cell Cell) Presence() model.Presence  { return cell.presence }
func (cell Cell) Scope() witness.Scope      { return cell.scope }
func (cell Cell) Lineage() model.LineageRef { return cell.lineage }

type row struct {
	owner   *reader
	id      model.RowID
	key     geometry.Key
	mask    support.Mask
	scope   witness.Scope
	lineage model.LineageRef
	cells   []Cell
}

func (value *row) rowFrom(owner *reader) bool { return value != nil && value.owner == owner }

// CellAt returns one borrowed cell from the reader-owned vector. The owner
// marker prevents a sibling reader from redeeming a row, while the cell's own
// Available check preserves the existing fence/lineage validation. Full-row
// shape is authenticated by Reader.Owns before this accessor is used.
func (value *row) CellAt(index int) (Cell, bool) {
	if value == nil || value.owner == nil || index < 0 || index >= len(value.cells) {
		return Cell{}, false
	}
	cell := value.cells[index]
	if !cell.Available() {
		return Cell{}, false
	}
	return cell, true
}

func (value *row) Available() bool {
	if value == nil || value.owner == nil || !value.id.Available() || value.id.Relation() != value.owner.layout.Access().Relation() || !value.mask.Valid() || value.mask.Manager() != value.owner.manager || !value.scope.ValidFor(value.owner.fence) || !value.lineage.Available() || !value.owner.lineageAuthority.Validate(value.lineage) {
		return false
	}
	normalized, normalizedOK := value.owner.view.Normalize(value.mask)
	if !normalizedOK || !normalized.Same(value.scope) {
		return false
	}
	columns := value.owner.layout.Columns()
	if len(value.cells) != len(columns) {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(value.cells))
	for position, cell := range value.cells {
		// A row is one common physical fiber.  A cell that is merely narrower
		// would make the row a false Cartesian product: the semantic payload
		// and the row address would describe different regions.  Readers issue
		// rows only after common refinement, so equality is the executable law.
		if !cell.Available() || cell.Column() != columns[position] || cell.Column().Relation() != value.id.Relation() || !cell.Scope().Same(value.scope) {
			return false
		}
		if _, duplicate := seen[cell.Column()]; duplicate {
			return false
		}
		seen[cell.Column()] = struct{}{}
		if cell.Value().Available() && cell.Value().Type() != cell.Type() {
			return false
		}
	}
	return true
}

func (value *row) ID() model.RowID {
	if value == nil {
		return model.RowID{}
	}
	return value.id
}

func (value *row) Key() geometry.Key {
	if value == nil {
		return geometry.Key(0)
	}
	return value.key
}

func (value *row) Scope() witness.Scope {
	if value == nil {
		return witness.Scope{}
	}
	return value.scope
}

func (value *row) Lineage() model.LineageRef {
	if value == nil {
		return model.LineageRef{}
	}
	return value.lineage
}

func (value *row) Cells() []Cell {
	if value == nil || !value.Available() {
		return nil
	}
	return append([]Cell(nil), value.cells...)
}

// Same compares a complete row address view. Rows from different readers are
// never interchangeable, even when their public coordinates happen to match.
func Same(left, right Row) bool {
	leftValue, leftOK := left.(*row)
	rightValue, rightOK := right.(*row)
	if !leftOK || !rightOK || !leftValue.Available() || !rightValue.Available() || leftValue.owner != rightValue.owner || leftValue.id != rightValue.id || leftValue.key != rightValue.key || !leftValue.mask.Equal(rightValue.mask) || !leftValue.scope.Same(rightValue.scope) || leftValue.lineage != rightValue.lineage || len(leftValue.cells) != len(rightValue.cells) {
		return false
	}
	for position := range leftValue.cells {
		leftCell, rightCell := leftValue.cells[position], rightValue.cells[position]
		if leftCell.Column() != rightCell.Column() || leftCell.Type() != rightCell.Type() || leftCell.Presence() != rightCell.Presence() || !leftCell.Scope().Same(rightCell.Scope()) || leftCell.Lineage() != rightCell.Lineage() {
			return false
		}
		if leftCell.Value().Available() != rightCell.Value().Available() || leftCell.Value().Available() && !leftCell.Value().Same(rightCell.Value()) {
			return false
		}
	}
	return true
}
