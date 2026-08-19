// schema_bind_entry.go exposes the public Bind entry points and the sealed implementation accessors.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
)

func BindSummaryReadForFactor[K ~uint32 | ~uint64, V, S any](binding *SchemaBinding, factorSlot *FactorSlot[V], form SchemaReadForm[V], normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || factorSlot == nil || factorSlot.Schema() != state.schema || form.cell == nil || form.cell.schema != state.schema || !summaryReadFormKind(form.cell.kind) || normalize == nil || equal == nil || fingerprint == nil {
		state.poisonLocked()
		return false
	}
	factorOrdinal, ok := factorSlot.Ordinal()
	formFactor, formOrdinal := form.cell.ordinal>>32, uint64(uint32(form.cell.ordinal))
	if !ok || factorOrdinal != formFactor || factorOrdinal >= uint64(len(state.factors)) {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.factorFormShapeAt(factorOrdinal, formOrdinal)
	if !shapeOK || !summaryReadRowKind(shape.Kind) {
		state.poisonLocked()
		return false
	}
	factor, ok := state.factors[factorOrdinal].(*schemaFactorBindingCell[K, V])
	if !ok || factor == nil || formOrdinal >= uint64(len(factor.forms)) || factor.forms[formOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	factor.forms[formOrdinal] = &schemaSummaryReadCell[K, V, S]{schema: state.schema, ordinal: form.cell.ordinal, factor: factor, algebra: factor.impl.algebra, form: form, normalize: normalize, equal: equal, fingerprint: fingerprint}
	return true
}

// BindIdentitySummaryReadForFactor binds the canonical identity summary form
// using the exact Factor algebra already admitted into this SchemaBinding.
// The record equality/fingerprint and identity normalizer remain engine-owned;
// callers cannot install a second summary law for an existing identity form.
func BindIdentitySummaryReadForFactor[K ~uint32 | ~uint64, V any](binding *SchemaBinding, factorSlot *FactorSlot[V], form SchemaReadForm[V]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	if state.phase != schemaBindingOpen || factorSlot == nil || factorSlot.Schema() != state.schema {
		state.mu.Unlock()
		return false
	}
	ordinal, ok := factorSlot.Ordinal()
	if !ok || ordinal >= uint64(len(state.factors)) {
		state.mu.Unlock()
		return false
	}
	factor, typed := state.factors[ordinal].(*schemaFactorBindingCell[K, V])
	if !typed || factor == nil || factor.impl == nil || factor.impl.algebra == nil {
		state.mu.Unlock()
		return false
	}
	algebra := factor.impl.algebra
	state.mu.Unlock()
	return BindSummaryReadForFactor[K, V](binding, factorSlot, form,
		func(value OrderedCells[V]) OrderedCells[V] { return value },
		func(left, right OrderedCells[V]) bool {
			return equalOrderedCellRecords(left.record, right.record, algebra.Equal)
		},
		func(value OrderedCells[V]) uint64 {
			return fingerprintOrderedCellRecord(value.record, algebra.Fingerprint)
		},
	)
}

func BindFactor[K ~uint32 | ~uint64, V any](binding *SchemaBinding, slot *FactorSlot[V], spec HotFactorSpec[K, V]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.Schema() != state.schema {
		state.poisonLocked()
		return false
	}
	ordinal, ok := slot.Ordinal()
	if !ok || ordinal >= uint64(len(state.factors)) || state.factors[ordinal] != nil {
		state.poisonLocked()
		return false
	}
	algebra, admitted := factbinding.Admit(spec.KeyEnd, spec.Default, spec.Lattice, spec.AdmitAt, spec.Fingerprint, factbinding.Measure[K, V]{Width: spec.WidenRank.Width, At: spec.WidenRank.At}, factbinding.Measure[K, V]{Width: spec.NarrowRank.Width, At: spec.NarrowRank.At})
	if !admitted || algebra == nil {
		state.poisonLocked()
		return false
	}
	formCount, ok := state.schema.factorFormCount(ordinal)
	if !ok {
		state.poisonLocked()
		return false
	}
	cell := &schemaFactorBindingCell[K, V]{ordinal: ordinal, schema: state.schema, impl: &FactorImplementation[K, V]{state: state, algebra: algebra, descriptor: factorRuntimeDescriptor{schema: state.schema, state: state, ordinal: ordinal, semantic: state.schema.factorSemanticAt(ordinal), keyEnd: spec.KeyEnd, algebra: algebra}}, forms: make([]schemaFactorFormBinding, formCount)}
	cell.exactRead = &schemaFactorFormCell[K, V]{schema: state.schema, ordinal: ordinal, kind: SchemaFormReadExact, factor: cell, algebra: algebra}
	cell.exactWrite = &schemaFactorFormCell[K, V]{schema: state.schema, ordinal: ordinal, kind: SchemaFormWriteExact, factor: cell, algebra: algebra}
	state.factors[ordinal] = cell
	return true
}

// BindRule admits the sole receipt-native Rule lane currently implemented.
// The output argument is a FactorSlot because FactorImplementationAt is only
// issued after the complete Binding seals; the slot resolves to that exact
// typed Factor cell during Seal and runtime receipt construction.
func BindRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return false
	}
	ruleOrdinal, ok := slot.Ordinal()
	if !ok || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !shapeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs != 0 || shape.ReadCount != 0 || shape.CarryCount != 0 || shape.WriteCount != 1 {
		state.poisonLocked()
		return false
	}
	writeOrdinal := write.cell.ordinal
	if writeOrdinal>>32 != ruleOrdinal || uint64(uint32(writeOrdinal)) != 0 {
		state.poisonLocked()
		return false
	}
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !writeOK || writeShape.Kind != composition.WriteExact || writeShape.Factor != shape.Output || writeShape.Route != 0 {
		state.poisonLocked()
		return false
	}
	outputOrdinal, outputOK := output.Ordinal()
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.schema != state.schema || outputCell.ordinal != outputOrdinal || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	cell.impl = &ruleHotImplementation[K, V, O]{state: state, rule: slot, write: write, output: outputCell, operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, projectWrite: projectWrite}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return false
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindRuleWithCarry admits the receipt-native one-write/one-carry lane. The
// carry token is the sole source of input, Factor, and transform identity;
// HotCarrySpec supplies only the typed executable transform when the sealed
// token declares one.
func BindRuleWithCarry[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return false
	}
	ruleOrdinal, ok := slot.Ordinal()
	if !ok || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || carry.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.cell.ordinal)) != 0 {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	if !shapeOK || !carryOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.ReadCount != 0 || shape.CarryCount != 1 || shape.WriteCount != 1 || carryShape.Input >= shape.Inputs || carryShape.Factor != shape.Output || (carryShape.Transform.Available() != (carrySpec.Apply != nil)) {
		state.poisonLocked()
		return false
	}
	writeOrdinal := write.cell.ordinal
	if writeOrdinal>>32 != ruleOrdinal || uint64(uint32(writeOrdinal)) != 0 {
		state.poisonLocked()
		return false
	}
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !writeOK || writeShape.Kind != composition.WriteExact || writeShape.Factor != shape.Output || writeShape.Route != 0 {
		state.poisonLocked()
		return false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return false
	}
	outputOrdinal, outputOK := output.Ordinal()
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.schema != state.schema || outputCell.ordinal != outputOrdinal || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	cell.impl = &ruleHotImplementation[K, V, O]{
		state: state, rule: slot, write: write, output: outputCell,
		carry:          &schemaRuleCarryBinding[K, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply},
		operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, projectWrite: projectWrite,
	}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return false
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindRuleWithExactReadAndCarry binds the one exact-read/one-carry/one-exact
// write receipt lane. The sealed read and carry capabilities remain the sole
// source of their input, Factor, and transform identities; this method adds no
// parallel structural representation.
func BindRuleWithExactReadAndCarry[OK ~uint32 | ~uint64, V, O any, RK ~uint32 | ~uint64, RV any](binding *SchemaBinding, slot *RuleSlot[V, O], readSlot SchemaReadSlot[RV], readFactor *FactorSlot[RV], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor == nil || readFactor.cell == nil || readFactor.cell.schema != state.schema || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	readPacked := readSlot.cell.ordinal
	carryPacked := carry.cell.ordinal
	writePacked := write.cell.ordinal
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || readPacked>>32 != ruleOrdinal || uint64(uint32(readPacked)) != 0 || carryPacked>>32 != ruleOrdinal || uint64(uint32(carryPacked)) != 0 || writePacked>>32 != ruleOrdinal || uint64(uint32(writePacked)) != 0 {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	readShape, readOK := state.schema.ruleReadShapeAt(ruleOrdinal, 0)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !readOK || !carryOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.ReadCount != 1 || shape.CarryCount != 1 || shape.WriteCount != 1 || readShape.Kind != composition.ReadExact || readShape.Input >= shape.Inputs || readShape.DependencyCount != 0 || carryShape.Input >= shape.Inputs || carryShape.Factor != shape.Output || carryShape.Transform.Available() != (carrySpec.Apply != nil) || writeShape.Kind != composition.WriteExact || writeShape.Factor != shape.Output || writeShape.Route != 0 {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	outputOrdinal, outputOK := output.Ordinal()
	readFactorOrdinal, readFactorOK := readFactor.Ordinal()
	if !outputOK || !readFactorOK || outputOrdinal >= uint64(len(state.factors)) || readFactorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output || state.schema.factorSemanticAt(readFactorOrdinal) != readShape.Factor {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	outputCell, outputTyped := state.factors[outputOrdinal].(*schemaFactorBindingCell[OK, V])
	readCell, readTyped := state.factors[readFactorOrdinal].(*schemaFactorBindingCell[RK, RV])
	if !outputTyped || !readTyped || outputCell == nil || readCell == nil || outputCell.impl == nil || readCell.impl == nil || outputCell.impl.algebra == nil || readCell.impl.algebra == nil || outputCell.impl.state != state || readCell.impl.state != state {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	cell := &schemaRuleBindingCellImpl[OK, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 0}
	read := Read[OrderedCells[RV]]{origin: origin, index: 0, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	readBinding := &schemaExactRuleReadBinding[RK, RV]{origin: origin, factor: readCell, read: read, projector: projectExactLocal(projectRead)}
	cell.impl = &ruleHotImplementation[OK, V, O]{
		state: state, rule: slot, write: write, output: outputCell,
		carry:          &schemaRuleCarryBinding[OK, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply},
		reads:          []schemaRuleReadBinding{readBinding},
		operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer,
		projectWrite: projectWrite,
	}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	state.rules[ruleOrdinal] = cell
	return read, true
}

// RuleImplementationAt issues a receipt-native typed Rule implementation
// only after the shared Binding has sealed. The returned value is a snapshot;
// its authority is rechecked when it is attached to an execution member.
func RuleImplementationAt[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (*RuleImplementation[K, V, O], bool) {
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
	cell, ok := state.rules[ordinal].(*schemaRuleBindingCellImpl[K, V, O])
	if !ok || cell == nil || !cell.schemaRuleComplete() {
		return nil, false
	}
	output, ok := cell.impl.output.sealedImplementation(state, state.authority)
	if !ok {
		return nil, false
	}
	proof, ok := newSchemaRuleRuntimeProof(state, state.authority, ordinal)
	if !ok || proof == nil || !output.binding.valid() {
		return nil, false
	}
	receipt := ruleRuntimeBinding[K, V, O]{state: state, authority: state.authority, cell: cell, proof: proof, output: output.binding, issued: true}
	if !receipt.valid() {
		return nil, false
	}
	return &RuleImplementation[K, V, O]{binding: receipt}, true
}

func FactorImplementationAt[K ~uint32 | ~uint64, V any](binding *SchemaBinding, slot *FactorSlot[V]) (*FactorImplementation[K, V], bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || slot == nil || slot.Schema() != state.schema {
		return nil, false
	}
	ordinal, ok := slot.Ordinal()
	if !ok || ordinal >= uint64(len(state.factors)) {
		return nil, false
	}
	cell, ok := state.factors[ordinal].(*schemaFactorBindingCell[K, V])
	if !ok || cell.schema != state.schema || cell.ordinal != ordinal || cell.impl == nil {
		return nil, false
	}
	// Return a fresh immutable implementation snapshot. The shared cell is
	// never mutated after Seal, so concurrent callers cannot observe a receipt
	// or descriptor being rewritten underneath a live runtime binder.
	return cell.sealedImplementation(state, state.authority)
}
