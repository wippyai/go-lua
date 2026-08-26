package tuple

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Cell is one ordered, typed payload carried by a transient Tuple. Its
// address, scope, and lineage are intentionally tuple-level facts: the
// evaluator has already conjoined scope and provenance before it constructs
// a tuple, so a fold cannot observe per-cell masks or synthesize a second
// support protocol.
type Cell struct {
	column   model.ColumnID
	typeID   model.TypeID
	value    binding.ValueToken
	presence model.Presence
	// source is the ordinal of the owner-issued row in Tuple.sources that
	// owns this cell. It is structural tuple provenance, not a nominal
	// relation lookup: self-joins may have more than one row of one relation.
	source uint32
}

func (cell Cell) Column() model.ColumnID    { return cell.column }
func (cell Cell) Type() model.TypeID        { return cell.typeID }
func (cell Cell) Value() binding.ValueToken { return cell.value }
func (cell Cell) Presence() model.Presence  { return cell.presence }
func (cell Cell) Source() uint32            { return cell.source }

func (cell Cell) available(fence binding.Fence) bool {
	if !cell.column.Available() || !cell.typeID.Available() || !cell.presence.Available() || cell.presence.Is(model.Refused) {
		return false
	}
	if cell.value.Available() {
		return cell.value.ValidFor(fence) && cell.value.Type() == cell.typeID
	}
	return !cell.presence.Is(model.Present) && !cell.presence.Is(model.AuthenticatedOpaque)
}

// Tuple is one immutable evaluator frame. Sources preserve authored operand
// order, including repeated RowIDs for legal self-joins; callers must never
// collapse them into a set. Cells similarly preserve declared column order.
// Scope and lineage are already-conjoined facts, never mutable accumulators.
type Tuple struct {
	fence   binding.Fence
	scope   witness.Scope
	lineage model.LineageRef
	sources []model.RowID
	cells   []Cell
	sealed  bool
}

// newTuple is the private tuple certificate constructor. Public callers do
// not supply a bag of individually valid fields: Input and tuple-owned
// combinators derive scope, lineage, and source provenance from sealed
// authorities before assembling the immutable frame here.
func newTuple(mounted witness.Mounted, scope witness.Scope, lineage model.LineageRef, sources []model.RowID, cells []Cell) (Tuple, bool) {
	return newTupleMode(mounted, scope, lineage, sources, cells)
}

func newTupleMode(mounted witness.Mounted, scope witness.Scope, lineage model.LineageRef, sources []model.RowID, cells []Cell) (Tuple, bool) {
	if !mounted.Available() || !scope.ValidFor(mounted.RuntimeFence()) || sources == nil || len(sources) == 0 || cells == nil {
		return Tuple{}, false
	}
	// Scope.ValidFor authenticates only the runtime fence. Redeem membership
	// in this mount's private scope arena before retaining the token so an
	// equal-fence scope from a sibling mount cannot cross the tuple boundary.
	if _, scopeOK := mounted.ScopeToken(scope); !scopeOK {
		return Tuple{}, false
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return Tuple{}, false
	}
	if !lineage.Available() || !lineageAuthority.Validate(lineage) {
		return Tuple{}, false
	}
	copySources := append([]model.RowID(nil), sources...)
	for _, source := range copySources {
		if !source.Available() {
			return Tuple{}, false
		}
		// RowID is owner-issued, while the mounted directory is the exact
		// relation-local membership authority for this evaluator. This keeps
		// a copied token from another mounted world out of the frame.
		if _, sourceOK := mounted.RowIndex(source.Relation(), source); !sourceOK {
			return Tuple{}, false
		}
	}
	// Preserve an authenticated empty vector as non-nil. Relation Input may
	// legitimately carry only its owner-issued RowID and no payload columns;
	// cloning that sealed empty extent through a nil destination would turn a
	// valid zero-width tuple into an unavailable one.
	copyCells := append([]Cell{}, cells...)
	seenCells := make(map[struct {
		source uint32
		column model.ColumnID
	}]struct{}, len(copyCells))
	for _, cell := range copyCells {
		if !cell.available(mounted.RuntimeFence()) {
			return Tuple{}, false
		}
		if int(cell.Source()) >= len(copySources) {
			return Tuple{}, false
		}
		key := struct {
			source uint32
			column model.ColumnID
		}{source: cell.Source(), column: cell.Column()}
		if _, duplicate := seenCells[key]; duplicate {
			return Tuple{}, false
		}
		seenCells[key] = struct{}{}
	}
	result := Tuple{
		fence:   mounted.RuntimeFence(),
		scope:   scope,
		lineage: lineage,
		sources: copySources,
		cells:   copyCells,
		sealed:  true,
	}
	return result, result.ValidForMode(mounted)
}

// Available is deliberately O(1). Input and the tuple-owned
// combinators prove every source/cell once before setting sealed; the hot
// evaluator path must not rescan a width-sized frame for every accessor.
// ValidFor restores the mounted lineage authority at an evaluator boundary.
func (tuple Tuple) Available() bool {
	return tuple.sealed && tuple.fence.Available() && tuple.scope.ValidFor(tuple.fence) && tuple.sources != nil && tuple.cells != nil && tuple.lineage.Available()
}

// ValidFor restores the mounted lineage authority at the one boundary where
// a tuple crosses an evaluator/operator call. It never accepts a foreign
// scope, value, or lineage arena.
func (tuple Tuple) ValidFor(mounted witness.Mounted) bool {
	return tuple.ValidForMode(mounted)
}

func (tuple Tuple) ValidForMode(mounted witness.Mounted) bool {
	if !tuple.Available() || !mounted.Available() || !tuple.fence.Same(mounted.RuntimeFence()) || !tuple.scope.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return false
	}
	return tuple.lineage.Available() && lineageAuthority.Validate(tuple.lineage)
}

func (tuple Tuple) Scope() witness.Scope {
	if !tuple.Available() {
		return witness.Scope{}
	}
	return tuple.scope
}

func (tuple Tuple) Lineage() model.LineageRef {
	if !tuple.Available() {
		return model.LineageRef{}
	}
	return tuple.lineage
}

func (tuple Tuple) SourceLen() int {
	if !tuple.Available() {
		return 0
	}
	return len(tuple.sources)
}

func (tuple Tuple) SourceAt(index int) (model.RowID, bool) {
	if !tuple.Available() || index < 0 || index >= len(tuple.sources) {
		return model.RowID{}, false
	}
	return tuple.sources[index], true
}

func (tuple Tuple) Sources() []model.RowID {
	if !tuple.Available() {
		return nil
	}
	return append([]model.RowID(nil), tuple.sources...)
}

// SourceFor resolves one unique owner-issued source row for a relation. A
// self-join with two rows of the same relation is deliberately ambiguous and
// refuses rather than choosing a positional row behind the semantic ABI's
// back. A future proven self-join delivery must declare an explicit role.
func (tuple Tuple) SourceFor(relation model.RelationID) (model.RowID, bool) {
	if !tuple.Available() || !relation.Available() {
		return model.RowID{}, false
	}
	var result model.RowID
	found := false
	for _, source := range tuple.sources {
		if source.Relation() != relation {
			continue
		}
		if found {
			return model.RowID{}, false
		}
		result, found = source, true
	}
	return result, found
}

func (tuple Tuple) Len() int {
	if !tuple.Available() {
		return 0
	}
	return len(tuple.cells)
}

func (tuple Tuple) At(index int) (Cell, bool) {
	if !tuple.Available() || index < 0 || index >= len(tuple.cells) {
		return Cell{}, false
	}
	return tuple.cells[index], true
}

func (tuple Tuple) Cells() []Cell {
	if !tuple.Available() {
		return nil
	}
	return append([]Cell(nil), tuple.cells...)
}

// Same compares the complete immutable frame in authored order. It is a
// structural replay comparison for Batch laws; semantic equality remains the
// responsibility of the owning relational operator.
func (tuple Tuple) Same(other Tuple) bool {
	if !tuple.Available() || !other.Available() || !tuple.fence.Same(other.fence) || !tuple.scope.Same(other.scope) || tuple.lineage != other.lineage || len(tuple.sources) != len(other.sources) || len(tuple.cells) != len(other.cells) {
		return false
	}
	for index, source := range tuple.sources {
		if source != other.sources[index] {
			return false
		}
	}
	for index, cell := range tuple.cells {
		otherCell := other.cells[index]
		if cell.column != otherCell.column || cell.typeID != otherCell.typeID || cell.presence != otherCell.presence || cell.source != otherCell.source || cell.value.Available() != otherCell.value.Available() {
			return false
		}
		if cell.value.Available() && !cell.value.Same(otherCell.value) {
			return false
		}
	}
	return true
}

// CellFor resolves one unique logical column in this frame. Ambiguity is a
// hard refusal: repeated columns from a self-join need a declared positional
// role, never an arbitrary first-match convention.
func (tuple Tuple) CellFor(column model.ColumnID) (Cell, bool) {
	if !tuple.Available() || !column.Available() {
		return Cell{}, false
	}
	var result Cell
	found := false
	for _, cell := range tuple.cells {
		if cell.column != column {
			continue
		}
		if found {
			return Cell{}, false
		}
		result, found = cell, true
	}
	return result, found
}
