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
func BindRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O]) bool {
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
	cell.impl = &ruleHotImplementation[K, V, O]{state: state, rule: slot, write: write, output: outputCell, operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer}
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
func BindRuleWithCarry[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O]) bool {
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
		operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer,
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
func BindRuleWithExactReadAndCarry[OK ~uint32 | ~uint64, V, O any, RK ~uint32 | ~uint64, RV any](binding *SchemaBinding, slot *RuleSlot[V, O], readSlot SchemaReadSlot[RV], readFactor *FactorSlot[RV], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O]) (Read[OrderedCells[RV]], bool) {
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
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 0, input: readShape.Input, factor: readFactorOrdinal, kind: composition.ReadExact}
	read := Read[OrderedCells[RV]]{origin: origin, index: 0, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	readBinding := &schemaExactRuleReadBinding[RK, RV]{origin: origin, factor: readCell, read: read}
	cell.impl = &ruleHotImplementation[OK, V, O]{
		state: state, rule: slot, write: write, output: outputCell,
		carry:          &schemaRuleCarryBinding[OK, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply},
		reads:          []schemaRuleReadBinding{readBinding},
		operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer,
	}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	state.rules[ruleOrdinal] = cell
	return read, true
}

// BindRuleWithExactAndSummaryReadAndCarry binds one exact read, one summary
// read, one ordinary carry, and one exact write. The two read receipts remain
// anchored to their declared Factor cells; no topology or callback snapshot is
// copied into the runtime implementation.
func BindRuleWithExactAndSummaryReadAndCarry[OK ~uint32 | ~uint64, V, O any, EK ~uint32 | ~uint64, EV any, SK ~uint32 | ~uint64, SV, S any](binding *SchemaBinding, slot *RuleSlot[V, O], exactSlot SchemaReadSlot[EV], exactFactor *FactorSlot[EV], summarySlot SchemaReadSlot[SV], summaryFactor *FactorSlot[SV], summaryForm SchemaReadForm[SV], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O]) (Read[OrderedCells[EV]], Read[S], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || exactSlot.cell == nil || exactSlot.cell.schema != state.schema || exactFactor == nil || exactFactor.cell == nil || exactFactor.cell.schema != state.schema || summarySlot.cell == nil || summarySlot.cell.schema != state.schema || summaryFactor == nil || summaryFactor.cell == nil || summaryFactor.cell.schema != state.schema || summaryForm.cell == nil || summaryForm.cell.schema != state.schema || !summaryReadFormKind(summaryForm.cell.kind) || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	exactPacked, summaryPacked, carryPacked, writePacked := exactSlot.cell.ordinal, summarySlot.cell.ordinal, carry.cell.ordinal, write.cell.ordinal
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || exactPacked>>32 != ruleOrdinal || uint64(uint32(exactPacked)) != 0 || summaryPacked>>32 != ruleOrdinal || uint64(uint32(summaryPacked)) != 1 || carryPacked>>32 != ruleOrdinal || uint64(uint32(carryPacked)) != 0 || writePacked>>32 != ruleOrdinal || uint64(uint32(writePacked)) != 0 {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	exactShape, exactOK := state.schema.ruleReadShapeAt(ruleOrdinal, 0)
	summaryShape, summaryOK := state.schema.ruleReadShapeAt(ruleOrdinal, 1)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !exactOK || !summaryOK || !carryOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.ReadCount != 2 || shape.CarryCount != 1 || shape.WriteCount != 1 || exactShape.Kind != composition.ReadExact || exactShape.Input >= shape.Inputs || exactShape.DependencyCount != 0 || summaryShape.Kind != composition.ReadSummary || summaryShape.Input >= shape.Inputs || summaryShape.DependencyCount != 0 || carryShape.Input >= shape.Inputs || carryShape.Factor != shape.Output || carryShape.Transform.Available() != (carrySpec.Apply != nil) || writeShape.Kind != composition.WriteExact || writeShape.Factor != shape.Output || writeShape.Route != 0 {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	outputOrdinal, outputOK := output.Ordinal()
	exactFactorOrdinal, exactFactorOK := exactFactor.Ordinal()
	summaryFactorOrdinal, summaryFactorOK := summaryFactor.Ordinal()
	if !outputOK || !exactFactorOK || !summaryFactorOK || outputOrdinal >= uint64(len(state.factors)) || exactFactorOrdinal >= uint64(len(state.factors)) || summaryFactorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output || state.schema.factorSemanticAt(exactFactorOrdinal) != exactShape.Factor || state.schema.factorSemanticAt(summaryFactorOrdinal) != summaryShape.Factor {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	outputCell, outputTyped := state.factors[outputOrdinal].(*schemaFactorBindingCell[OK, V])
	exactCell, exactTyped := state.factors[exactFactorOrdinal].(*schemaFactorBindingCell[EK, EV])
	summaryCell, summaryTyped := state.factors[summaryFactorOrdinal].(*schemaFactorBindingCell[SK, SV])
	if !outputTyped || !exactTyped || !summaryTyped || outputCell == nil || exactCell == nil || summaryCell == nil || outputCell.impl == nil || exactCell.impl == nil || summaryCell.impl == nil || outputCell.impl.algebra == nil || exactCell.impl.algebra == nil || summaryCell.impl.algebra == nil || outputCell.impl.state != state || exactCell.impl.state != state || summaryCell.impl.state != state {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	formFactor, formOrdinal := summaryForm.cell.ordinal>>32, uint64(uint32(summaryForm.cell.ordinal))
	if formFactor != summaryFactorOrdinal || formOrdinal >= uint64(len(summaryCell.forms)) {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	summary, summaryTyped := summaryCell.forms[formOrdinal].(*schemaSummaryReadCell[SK, SV, S])
	shapeForm, shapeFormOK := state.schema.factorFormShapeAt(summaryFactorOrdinal, formOrdinal)
	if !summaryTyped || summary == nil || summary.schema != state.schema || summary.factor != summaryCell || summary.form.cell != summaryForm.cell || summary.normalize == nil || summary.equal == nil || summary.fingerprint == nil || !shapeFormOK || !summaryReadRowKind(shapeForm.Kind) || shapeForm.Semantic != summaryShape.Semantic {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	cell := &schemaRuleBindingCellImpl[OK, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	exactOrigin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 0, input: exactShape.Input, factor: exactFactorOrdinal, kind: composition.ReadExact}
	exactRead := Read[OrderedCells[EV]]{origin: exactOrigin, index: 0, resolve: resolveTypedRead[EV, OrderedCells[EV]]}
	readOrigin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 1, input: summaryShape.Input, factor: summaryFactorOrdinal, kind: composition.ReadSummary, semantic: summaryShape.Semantic, formOrdinal: formOrdinal}
	read := Read[S]{origin: readOrigin, index: 1, resolve: resolveTypedRead[SV, S]}
	cell.impl = &ruleHotImplementation[OK, V, O]{state: state, rule: slot, write: write, output: outputCell,
		carry: &schemaRuleCarryBinding[OK, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply},
		reads: []schemaRuleReadBinding{
			&schemaExactRuleReadBinding[EK, EV]{origin: exactOrigin, factor: exactCell, read: exactRead},
			&schemaSummaryRuleReadBinding[SK, SV, S]{origin: readOrigin, factor: summaryCell, form: summary, read: read},
		}, operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[OrderedCells[EV]]{}, Read[S]{}, false
	}
	state.rules[ruleOrdinal] = cell
	return exactRead, read, true
}

// BindRuleWithSelectedReadAndRouteWrite binds the smallest receipt-native
// staged Rule lane: ordered exact predecessors, one selected exact read, and
// the one route write consuming that Selection. Predecessor slots are passed
// as opaque capabilities; their canonical ordinals and Factor cells are
// rechecked against Schema and never copied into a cold Rule representation.
func BindRuleWithSelectedReadAndRouteWrite[OK ~uint32 | ~uint64, V, O any, RK ~uint32 | ~uint64, RV any, Tag selectionTag](binding *SchemaBinding, slot *RuleSlot[V, O], predecessors []SchemaReadSlot[OrderedCells[RV]], predecessorFactors []*FactorSlot[RV], selected SchemaReadSlot[Selection[Tag, OrderedCells[RV]]], selectedFactor *FactorSlot[RV], write SchemaWriteSlot[V], output *FactorSlot[V], locate func(SelectorContext, Read[OrderedCells[RV]]) bool, spec HotRuleSpec[V, O]) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || selected.cell == nil || selected.cell.schema != state.schema || selectedFactor == nil || selectedFactor.cell == nil || selectedFactor.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || locate == nil || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil || len(predecessors) == 0 || len(predecessors) != len(predecessorFactors) {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	selectedPacked := selected.cell.ordinal
	writePacked := write.cell.ordinal
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || selectedPacked>>32 != ruleOrdinal || writePacked>>32 != ruleOrdinal || uint64(uint32(writePacked)) != 0 {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.ReadCount != uint64(len(predecessors)+1) || shape.CarryCount != 0 || shape.WriteCount != 1 || writeShape.Kind != composition.WriteRoute || writeShape.Route != uint64(uint32(selectedPacked))+1 || writeShape.Factor != shape.Output {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	selectedShape, selectedOK := state.schema.ruleReadShapeAt(ruleOrdinal, uint64(len(predecessors)))
	selectedOrdinalOK := uint64(uint32(selectedPacked)) == uint64(len(predecessors))
	if !selectedOK || !selectedOrdinalOK || selectedShape.Kind != composition.ReadSelect || selectedShape.Factor != shape.Output || selectedShape.Semantic != selectedShape.Factor || selectedShape.Normalizer.Available() || selectedShape.DependencyCount != uint64(len(predecessors)) {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	for index := range predecessors {
		packed := predecessors[index].cell
		factor := predecessorFactors[index]
		readShape, readOK := state.schema.ruleReadShapeAt(ruleOrdinal, uint64(index))
		factorOrdinal, factorOK := factor.Ordinal()
		if packed == nil || packed.schema != state.schema || packed.ordinal>>32 != ruleOrdinal || uint64(uint32(packed.ordinal)) != uint64(index) || factor == nil || factor.cell == nil || factor.cell.schema != state.schema || !readOK || !factorOK || readShape.Kind != composition.ReadExact || readShape.DependencyCount != 0 || factorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(factorOrdinal) != readShape.Factor {
			state.poisonLocked()
			return Read[Selection[Tag, OrderedCells[RV]]]{}, false
		}
		dependency, dependencyOK := state.schema.ruleReadDependencyAt(ruleOrdinal, uint64(len(predecessors)), uint64(index))
		if !dependencyOK || dependency != uint64(index) {
			state.poisonLocked()
			return Read[Selection[Tag, OrderedCells[RV]]]{}, false
		}
	}
	selectedFactorOrdinal, selectedFactorOK := selectedFactor.Ordinal()
	outputOrdinal, outputOK := output.Ordinal()
	if !selectedFactorOK || !outputOK || selectedFactorOrdinal >= uint64(len(state.factors)) || outputOrdinal >= uint64(len(state.factors)) || selectedFactorOrdinal != outputOrdinal || state.schema.factorSemanticAt(selectedFactorOrdinal) != selectedShape.Factor || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	outputCell, outputTyped := state.factors[outputOrdinal].(*schemaFactorBindingCell[OK, V])
	selectedCell, selectedTyped := state.factors[selectedFactorOrdinal].(*schemaFactorBindingCell[RK, RV])
	if !outputTyped || !selectedTyped || outputCell == nil || selectedCell == nil || outputCell.impl == nil || selectedCell.impl == nil || outputCell.impl.algebra == nil || selectedCell.impl.algebra == nil || outputCell.impl.state != state || selectedCell.impl.state != state {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	cell := &schemaRuleBindingCellImpl[OK, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	reads := make([]schemaRuleReadBinding, 0, len(predecessors)+1)
	var predecessorRead Read[OrderedCells[RV]]
	for index := range predecessors {
		readShape, _ := state.schema.ruleReadShapeAt(ruleOrdinal, uint64(index))
		factorOrdinal, _ := predecessorFactors[index].Ordinal()
		origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: uint64(index), input: readShape.Input, factor: factorOrdinal, kind: composition.ReadExact}
		read := Read[OrderedCells[RV]]{origin: origin, index: index, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
		if index == 0 {
			predecessorRead = read
		}
		factorCell, factorOK := state.factors[factorOrdinal].(*schemaFactorBindingCell[RK, RV])
		if !factorOK || factorCell == nil {
			state.poisonLocked()
			return Read[Selection[Tag, OrderedCells[RV]]]{}, false
		}
		reads = append(reads, &schemaExactRuleReadBinding[RK, RV]{origin: origin, factor: factorCell, read: read})
	}
	selectedOrigin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: uint64(len(predecessors)), input: selectedShape.Input, factor: selectedFactorOrdinal, kind: composition.ReadSelect, dependencyCount: selectedShape.DependencyCount}
	selectedRead := Read[Selection[Tag, OrderedCells[RV]]]{origin: selectedOrigin, index: len(predecessors), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	reads = append(reads, &schemaSelectedRuleReadBinding[RK, RV, Tag]{origin: selectedOrigin, factor: selectedCell, read: selectedRead, locate: func(context SelectorContext) bool {
		return locate(context, predecessorRead)
	}})
	cell.impl = &ruleHotImplementation[OK, V, O]{state: state, rule: slot, write: write, output: outputCell, reads: reads, operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	state.rules[ruleOrdinal] = cell
	return selectedRead, true
}

// BindRuleWithSummaryRead admits one typed summary read together with one
// exact strong output write. The summary callbacks must already be installed
// on the exact Factor form cell by BindSummaryReadForFactor; this API only
// consumes that cell-issued receipt and never stores a second callback copy.
func BindRuleWithSummaryRead[OK ~uint32 | ~uint64, V, O any, RK ~uint32 | ~uint64, RV, S any](binding *SchemaBinding, slot *RuleSlot[V, O], readSlot SchemaReadSlot[RV], readFactor *FactorSlot[RV], form SchemaReadForm[RV], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O]) (Read[S], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[S]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor == nil || readFactor.cell == nil || readFactor.cell.schema != state.schema || form.cell == nil || form.cell.schema != state.schema || !summaryReadFormKind(form.cell.kind) || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return Read[S]{}, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	readPacked := readSlot.cell.ordinal
	writePacked := write.cell.ordinal
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || readPacked>>32 != ruleOrdinal || uint64(uint32(readPacked)) != 0 || writePacked>>32 != ruleOrdinal || uint64(uint32(writePacked)) != 0 {
		state.poisonLocked()
		return Read[S]{}, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	readShape, readShapeOK := state.schema.ruleReadShapeAt(ruleOrdinal, 0)
	writeShape, writeShapeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.ReadCount != 1 || shape.CarryCount != 0 || shape.WriteCount != 1 || !readShapeOK || readShape.Kind != composition.ReadSummary || readShape.Input >= shape.Inputs || readShape.DependencyCount != 0 || !writeShapeOK || writeShape.Kind != composition.WriteExact || writeShape.Factor != shape.Output || writeShape.Route != 0 {
		state.poisonLocked()
		return Read[S]{}, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return Read[S]{}, false
	}
	outputOrdinal, outputOK := output.Ordinal()
	readFactorOrdinal, readFactorOK := readFactor.Ordinal()
	formFactor, formOrdinal := form.cell.ordinal>>32, uint64(uint32(form.cell.ordinal))
	if !outputOK || !readFactorOK || outputOrdinal >= uint64(len(state.factors)) || readFactorOrdinal >= uint64(len(state.factors)) || formFactor != readFactorOrdinal || state.schema.factorSemanticAt(outputOrdinal) != shape.Output || state.schema.factorSemanticAt(readFactorOrdinal) != readShape.Factor || readShape.Semantic == (composition.Key{}) || readShape.Normalizer != readShape.Semantic {
		state.poisonLocked()
		return Read[S]{}, false
	}
	outputCell, outputTyped := state.factors[outputOrdinal].(*schemaFactorBindingCell[OK, V])
	readCell, readTyped := state.factors[readFactorOrdinal].(*schemaFactorBindingCell[RK, RV])
	if !outputTyped || !readTyped || outputCell == nil || readCell == nil || outputCell.impl == nil || readCell.impl == nil || outputCell.impl.algebra == nil || readCell.impl.algebra == nil || outputCell.impl.state != state || readCell.impl.state != state {
		state.poisonLocked()
		return Read[S]{}, false
	}
	if formOrdinal >= uint64(len(readCell.forms)) {
		state.poisonLocked()
		return Read[S]{}, false
	}
	summaryForm, summaryTyped := readCell.forms[formOrdinal].(*schemaSummaryReadCell[RK, RV, S])
	if !summaryTyped || summaryForm == nil || summaryForm.schema != state.schema || summaryForm.factor != readCell || summaryForm.form.cell != form.cell || !summaryReadFormKind(summaryForm.form.cell.kind) || summaryForm.form.cell.ordinal != form.cell.ordinal || summaryForm.normalize == nil || summaryForm.equal == nil || summaryForm.fingerprint == nil || summaryForm.form.cell.ordinal != formFactor<<32|formOrdinal {
		state.poisonLocked()
		return Read[S]{}, false
	}
	shapeForm, shapeFormOK := state.schema.factorFormShapeAt(readFactorOrdinal, formOrdinal)
	if !shapeFormOK || !summaryReadRowKind(shapeForm.Kind) || shapeForm.Semantic != readShape.Semantic {
		state.poisonLocked()
		return Read[S]{}, false
	}
	cell := &schemaRuleBindingCellImpl[OK, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 0, input: readShape.Input, factor: readFactorOrdinal, kind: composition.ReadSummary, semantic: readShape.Semantic, formOrdinal: formOrdinal}
	read := Read[S]{origin: origin, index: 0, resolve: resolveTypedRead[RV, S]}
	readBinding := &schemaSummaryRuleReadBinding[RK, RV, S]{origin: origin, factor: readCell, form: summaryForm, read: read}
	cell.impl = &ruleHotImplementation[OK, V, O]{state: state, rule: slot, write: write, output: outputCell, reads: []schemaRuleReadBinding{readBinding}, operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[S]{}, false
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
	if !ok || proof == nil || !output.receipt.valid() {
		return nil, false
	}
	receipt := ruleRuntimeReceipt[K, V, O]{state: state, authority: state.authority, cell: cell, proof: proof, output: output.receipt, issued: true}
	if !receipt.valid() {
		return nil, false
	}
	return &RuleImplementation[K, V, O]{receipt: receipt}, true
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
