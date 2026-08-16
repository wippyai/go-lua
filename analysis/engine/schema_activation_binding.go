package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// HotActivationSpec is the Link-local implementation half of one cold
// SchemaActivationRuleSlot. The slot supplies semantic shape and activation
// family; this value supplies only the typed admission and evaluator callback.
// The callback is retained behind the sealed receipt and is never exposed as a
// mutable rule registry.
type HotActivationSpec struct {
	Admission RuleAdmission[ActivationResult, ruleUnit]
	Run       func(Activation) bool
}

type activationHotImplementation struct {
	state     *schemaBindingState
	rule      *SchemaActivationRuleSlot
	reads     []schemaRuleReadBinding
	admission RuleAdmission[ActivationResult, ruleUnit]
	run       func(Activation) bool
}

type schemaActivationRuleBindingCell struct {
	state   *schemaBindingState
	schema  *Schema
	ordinal uint64
	impl    *activationHotImplementation
}

// schemaActivationFamilyBindingCell is the immutable receipt marker for one
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

func (cell *schemaActivationFamilyBindingCell) activationOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
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

func (cell *schemaActivationRuleBindingCell) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.state.schema != cell.schema || cell.impl == nil || cell.impl.state != cell.state || cell.impl.rule == nil || cell.impl.rule.cell == nil || cell.impl.rule.cell.schema != cell.schema || cell.impl.admission.valid() == false || cell.impl.run == nil {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || shape.OutputKind != composition.StructuralOutput || shape.CarryCount != 0 || shape.WriteCount != 0 || shape.ActivationCount != 1 || uint64(len(cell.impl.reads)) != shape.ReadCount {
		return false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || cell.impl.admission.kind != coldAdmission.kind || cell.impl.admission.identity != coldAdmission.identity {
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

func (cell *schemaActivationRuleBindingCell) schemaRuleProofMatches(proof *ruleRuntimeProof) bool {
	if cell == nil || proof == nil || cell.state == nil || cell.impl == nil || cell.schema != proof.schema || cell.ordinal != proof.ordinal || cell.state != proof.state || cell.state.authority != proof.bindingAuthority {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || shape.OutputKind != composition.StructuralOutput || shape.CarryCount != proof.carries || shape.WriteCount != proof.writes || shape.Inputs != proof.inputs || shape.ReadCount != proof.reads || shape.ActivationCount != 1 || proof.output.Available() {
		return false
	}
	admission, admitted := coldRuleAdmission(shape.Admission)
	return admitted && admission == proof.admission && cell.impl.admission.kind == admission.kind && cell.impl.admission.identity == admission.identity && cell.schema.ruleSemanticAt(cell.ordinal) == proof.semantic
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
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || !spec.Admission.valid() || spec.Run == nil {
		state.poisonLocked()
		return false
	}
	ruleOrdinal, ok := slot.Ordinal()
	if !ok || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !shapeOK || shape.OutputKind != composition.StructuralOutput || shape.ReadCount != 0 || shape.CarryCount != 0 || shape.WriteCount != 0 || shape.ActivationCount != 1 || !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return false
	}
	if !bindActivationFamilyLocked(state, shape.ActivationFamily) {
		state.poisonLocked()
		return false
	}
	cell := &schemaActivationRuleBindingCell{state: state, schema: state.schema, ordinal: ruleOrdinal}
	cell.impl = &activationHotImplementation{state: state, rule: slot, admission: spec.Admission, run: spec.Run}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return false
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindActivationRuleWithExactRead binds the first typed activation read lane.
// The returned Read is receipt-owned and can be resolved only by a future
// receipt-native activation execution carrying the matching Rule proof.
func BindActivationRuleWithExactRead[RK ~uint32 | ~uint64, RV any](binding *SchemaBinding, slot *SchemaActivationRuleSlot, readSlot SchemaReadSlot[RV], readFactor *FactorSlot[RV], spec HotActivationSpec) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor == nil || readFactor.cell == nil || readFactor.cell.schema != state.schema || !spec.Admission.valid() || spec.Run == nil {
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
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !shapeOK || shape.OutputKind != composition.StructuralOutput || shape.ReadCount != 1 || shape.CarryCount != 0 || shape.WriteCount != 0 || shape.ActivationCount != 1 || !readOK || readShape.Kind != composition.ReadExact || readShape.Input >= shape.Inputs || readShape.DependencyCount != 0 || !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
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
	if !factorTyped || factorCell == nil || factorCell.impl == nil || factorCell.impl.algebra == nil || factorCell.impl.state != state {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	cell := &schemaActivationRuleBindingCell{state: state, schema: state.schema, ordinal: ruleOrdinal}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 0, input: readShape.Input, factor: ruleFactorOrdinal, kind: composition.ReadExact}
	read := Read[OrderedCells[RV]]{origin: origin, index: 0, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	readBinding := &schemaExactRuleReadBinding[RK, RV]{origin: origin, factor: factorCell, read: read}
	cell.impl = &activationHotImplementation{state: state, rule: slot, reads: []schemaRuleReadBinding{readBinding}, admission: spec.Admission, run: spec.Run}
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

// ActivationRuleImplementation is an opaque sealed receipt. Its callback,
// reads, schema cell, and authority remain private to the engine runtime.
type ActivationRuleImplementation struct {
	receipt activationRuleRuntimeReceipt
}

type activationRuleRuntimeReceipt struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	cell      *schemaActivationRuleBindingCell
	proof     *ruleRuntimeProof
	issued    bool
}

func (receipt activationRuleRuntimeReceipt) valid() bool {
	return receipt.issued && receipt.state != nil && receipt.authority != nil && receipt.cell != nil && receipt.proof != nil && receipt.state.phase == schemaBindingSealed && receipt.state.authority == receipt.authority && receipt.cell.state == receipt.state && receipt.cell.schema == receipt.state.schema && receipt.proof.state == receipt.state && receipt.proof.bindingAuthority == receipt.authority && receipt.proof.ordinal == receipt.cell.ordinal && receipt.proof.valid() && receipt.cell.schemaRuleComplete() && receipt.cell.schemaRuleProofMatches(receipt.proof)
}

// ActivationRuleImplementationAt issues a fresh receipt only from the exact
// sealed SchemaBinding and slot. Equal Schema IDs backed by another Binding
// cannot pass the state/authority fence.
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
	if !ok || cell == nil || !cell.schemaRuleComplete() {
		return nil, false
	}
	proof, ok := newSchemaRuleRuntimeProof(state, state.authority, ordinal)
	if !ok {
		return nil, false
	}
	receipt := activationRuleRuntimeReceipt{state: state, authority: state.authority, cell: cell, proof: proof, issued: true}
	if !receipt.valid() {
		return nil, false
	}
	return &ActivationRuleImplementation{receipt: receipt}, true
}
