package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// The A form's cold geometry, decided from the declaration.
//
// A structural activation rule genuinely has two reads: one exact read at the
// trigger coordinate, and one selected read over the candidate relation whose
// members are the branches it chooses among. That is what the sealed execution
// form table classifies the form by - execution.DeclaredForm derives
// FormActivation from a structural publication with a selected read and a
// transport vector - and it is what the generated A form declares.
//
// The two hand-declared entries below are the other shape. Neither declares the
// candidate selection: the fold walks the branches from a column its own
// package sealed at bind - domain/call/activation's route table - which is the
// candidate relation restated by hand rather than declared as the join it is.
// So one form has two cold shapes, and the hand one is the shape that changes.
//
// It changes when the generated activation lane can execute. The read side of
// that change is what the reconciliation actually costs: an activation member
// with a selected read needs the anchored selected-read surface at activation
// issuance, and the candidate relation needs a member-relation owner on the
// axis the trigger is indexed by. Until then these entries stay, and they stay
// named as the shape that is not the form's own.

// HotActivationSpec is the Link-local implementation half of one cold
// SchemaActivationRuleSlot. The slot supplies semantic shape and activation
// family; this value supplies only the typed fold callback.
// The callback is retained behind the sealed cell and is never exposed as a
// mutable rule registry.
type HotActivationSpec struct {
	Fold func(ActivationFrame) ActivationResult
}

type activationHotImplementation struct {
	state *schemaBindingState
	rule  *SchemaActivationRuleSlot
	reads []schemaRuleReadBinding
	fold  func(ActivationFrame) ActivationResult
}

type schemaActivationRuleBindingCell struct {
	state   *schemaBindingState
	schema  *Schema
	ordinal uint64
	impl    *activationHotImplementation
}

// schemaActivationFamilyBindingCell is the immutable binding marker for one
// cold activation family. A family has no callback; its exact canonical slot
// is nevertheless inventoried so Binding.Seal cannot publish a Rule without
// the corresponding family authority.
type schemaActivationFamilyBindingCell struct {
	schema  *Schema
	ordinal uint64
}

func (cell *schemaActivationFamilyBindingCell) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaActivationFamilyBindingCell) activationComplete(schema *Schema, ordinal uint64) bool {
	return cell != nil && cell.schema == schema && cell.ordinal == ordinal && schema != nil && ordinal < schema.activationCount()
}

func (cell *schemaActivationRuleBindingCell) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaActivationRuleBindingCell) schemaRuleOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaActivationRuleBindingCell) schemaRuleBindingState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (cell *schemaActivationRuleBindingCell) schemaRuleReadAt(index uint64) *schemaRuleReadRow {
	if cell == nil || cell.impl == nil || index >= uint64(len(cell.impl.reads)) || cell.impl.reads[index] == nil {
		return nil
	}
	return cell.impl.reads[index].readRow()
}

func (cell *schemaActivationRuleBindingCell) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.state.schema != cell.schema || cell.impl == nil || cell.impl.state != cell.state || cell.impl.rule == nil || cell.impl.rule.cell == nil || cell.impl.rule.cell.schema != cell.schema || cell.impl.fold == nil {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || shape.OutputKind != composition.StructuralOutput || shape.CarryCount != 0 || shape.WriteCount != 0 || shape.ActivationCount != 1 || uint64(len(cell.impl.reads)) != shape.ReadCount {
		return false
	}
	ruleOrdinal, ruleOK := cell.impl.rule.Ordinal()
	if !ruleOK || ruleOrdinal != cell.ordinal {
		return false
	}
	for _, read := range cell.impl.reads {
		if read == nil || !read.complete(cell.state, cell, cell.ordinal) {
			return false
		}
	}
	return true
}

// BindActivationRule binds a zero-read activation Rule. Exact reads use
// BindActivationRuleWithExactRead below; both APIs consume only the exact
// SchemaActivationRuleSlot and never reconstruct a cold row.
func BindActivationRule(binding *SchemaBinding, slot *SchemaActivationRuleSlot, spec HotActivationSpec) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || spec.Fold == nil {
		state.poisonLocked()
		return false
	}
	ruleOrdinal, ok := slot.Ordinal()
	if !ok || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !shapeOK || shape.OutputKind != composition.StructuralOutput || shape.ReadCount != 0 || shape.CarryCount != 0 || shape.WriteCount != 0 || shape.ActivationCount != 1 {
		state.poisonLocked()
		return false
	}
	if !bindActivationFamilyLocked(state, shape.ActivationFamily) {
		state.poisonLocked()
		return false
	}
	cell := &schemaActivationRuleBindingCell{state: state, schema: state.schema, ordinal: ruleOrdinal}
	cell.impl = &activationHotImplementation{state: state, rule: slot, fold: spec.Fold}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return false
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindActivationRuleWithExactRead binds the first typed activation read lane.
// The returned Read is cell-owned and can be resolved only by the matching
// sealed activation execution.
func BindActivationRuleWithExactRead[RK ~uint32 | ~uint64, RV any](binding *SchemaBinding, slot *SchemaActivationRuleSlot, readSlot SchemaReadSlot[RV], readFactor *FactorSlot[RV], spec HotActivationSpec) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor == nil || readFactor.cell == nil || readFactor.cell.schema != state.schema || spec.Fold == nil {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	readPacked := readSlot.cell.ordinal
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || readPacked>>32 != ruleOrdinal || uint64(uint32(readPacked)) != 0 {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	readShape, readOK := state.schema.ruleReadShapeAt(ruleOrdinal, 0)
	if !shapeOK || shape.OutputKind != composition.StructuralOutput || shape.ReadCount != 1 || shape.CarryCount != 0 || shape.WriteCount != 0 || shape.ActivationCount != 1 || !readOK || readShape.Kind != composition.ReadExact || readShape.Input >= shape.Inputs || readShape.DependencyCount != 0 {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	if !bindActivationFamilyLocked(state, shape.ActivationFamily) {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	ruleFactorOrdinal, factorOK := readFactor.Ordinal()
	if !factorOK || ruleFactorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(ruleFactorOrdinal) != readShape.Factor {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	factorCell, factorTyped := state.factors[ruleFactorOrdinal].(*schemaFactorBindingCell[RK, RV])
	if !factorTyped || factorCell == nil || factorCell.impl == nil || factorCell.impl.algebra == nil || factorCell.state != state {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	cell := &schemaActivationRuleBindingCell{state: state, schema: state.schema, ordinal: ruleOrdinal}
	row, rowOK := compileSchemaRuleReadRow(state, cell, ruleOrdinal, 0, nil, 0)
	if !rowOK || row.factorOrdinal != ruleFactorOrdinal || !factorCell.schemaFactorReadComplete(state, row) {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	read := Read[OrderedCells[RV]]{row: row, index: 0, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	readBinding := &schemaExactRuleReadBinding[RK, RV]{row: row, factor: factorCell, read: read}
	cell.impl = &activationHotImplementation{state: state, rule: slot, reads: []schemaRuleReadBinding{readBinding}, fold: spec.Fold}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	state.rules[ruleOrdinal] = cell
	return read, true
}

func bindActivationFamilyLocked(state *schemaBindingState, key composition.Key) bool {
	if state == nil || state.schema == nil || !key.Available() {
		return false
	}
	ordinal, ok := state.schema.activationOrdinalOf(key)
	if !ok || ordinal >= uint64(len(state.activation)) {
		return false
	}
	if state.activation[ordinal] == nil {
		state.activation[ordinal] = &schemaActivationFamilyBindingCell{schema: state.schema, ordinal: ordinal}
	}
	family, ok := state.activation[ordinal].(*schemaActivationFamilyBindingCell)
	return ok && family.activationComplete(state.schema, ordinal)
}

// ActivationRuleImplementation is the sealed typed activation row. The cell
// owns the callback and exact Binding state; ordinal is retained alongside it
// so every consumer addresses the same canonical row without a copied proof or
// authority tuple.
type ActivationRuleImplementation struct {
	cell    *schemaActivationRuleBindingCell
	ordinal uint64
}

// sealedActivationCell is the lightweight execution fence for an issued
// activation row. Full cold-shape validation belongs to the open bind and
// construction paths; the implementation itself retains only the typed cell
// and its canonical ordinal.
func (implementation *ActivationRuleImplementation) sealedActivationCell() (*schemaActivationRuleBindingCell, bool) {
	if implementation == nil || implementation.cell == nil || implementation.ordinal != implementation.cell.ordinal {
		return nil, false
	}
	cell := implementation.cell
	if cell.state == nil || cell.schema == nil || cell.state.schema != cell.schema || cell.state.phase != schemaBindingSealed || cell.state.authority == nil {
		return nil, false
	}
	return cell, true
}

// ActivationRuleImplementationAt issues a typed implementation only from the
// exact sealed SchemaBinding and canonical slot row. The returned cell carries
// that Binding state into the later foreign-plane fences.
func ActivationRuleImplementationAt(binding *SchemaBinding, slot *SchemaActivationRuleSlot) (*ActivationRuleImplementation, bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || slot == nil || slot.cell == nil || slot.cell.schema != state.schema {
		return nil, false
	}
	ordinal, ok := slot.Ordinal()
	if !ok || ordinal >= uint64(len(state.rules)) {
		return nil, false
	}
	cell, ok := state.rules[ordinal].(*schemaActivationRuleBindingCell)
	if !ok || cell == nil || cell.state != state || cell.schema != state.schema || !cell.schemaRuleComplete() {
		return nil, false
	}
	return &ActivationRuleImplementation{cell: cell, ordinal: ordinal}, true
}
