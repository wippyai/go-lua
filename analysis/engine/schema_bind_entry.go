// schema_bind_entry.go exposes the public Bind entry points and the sealed implementation accessors.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
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
	cell := &schemaFactorBindingCell[K, V]{state: state, ordinal: ordinal, schema: state.schema, forms: make([]schemaFactorFormBinding, formCount)}
	cell.impl = &FactorImplementation[K, V]{row: cell, ordinal: ordinal, algebra: algebra}
	cell.exactRead = &schemaFactorFormCell[K, V]{schema: state.schema, ordinal: ordinal, kind: SchemaFormReadExact, factor: cell, algebra: algebra}
	cell.exactWrite = &schemaFactorFormCell[K, V]{schema: state.schema, ordinal: ordinal, kind: SchemaFormWriteExact, factor: cell, algebra: algebra}
	state.factors[ordinal] = cell
	return true
}

// BindRelationOwner installs the one generated member-relation owner for a
// bound Factor/axis while the SchemaBinding is open. The owner is deliberately
// neutral: it can only resolve the Plan's relation/projection ordinals and
// never receives a Rule callback, operand value, or runtime handle.
func BindRelationOwner[V any](binding *SchemaBinding, slot *FactorSlot[V], owner memberrelation.Owner) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.schema == nil || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || owner == nil {
		if state.phase == schemaBindingOpen {
			state.poisonLocked()
		}
		return false
	}
	ordinal, ok := slot.Ordinal()
	if !ok || ordinal >= uint64(len(state.factors)) || state.factors[ordinal] == nil || state.factors[ordinal].schemaFactorRelationOwner() != nil {
		state.poisonLocked()
		return false
	}
	factor := state.factors[ordinal]
	return factor.setSchemaFactorRelationOwner(owner)
}

// BindRuleFamily installs the execution family one rule authors for its own
// sealed ordinal, while the SchemaBinding is open.
//
// A rule's family is the rule's knowledge. The schemas, contracts and derived
// plans its fold needs are in scope exactly here, at the rule's own bind, and
// nowhere else: the axis owner the rule writes to is constructed from that
// axis's schema alone and could not supply them without acquiring foreign
// schemas it has no business holding. So the rule installs its own family, and
// the claim is fenced by the sealed rule ordinal it is made against.
//
// The installer is retained as sealed data, exactly as a relation owner's
// source columns are. It is asked once, when the Factor it belongs to builds
// its form table, and never during a solve.
// The seam is one function for both declaration lanes. A hand-declared rule
// and a Program-declared one differ in how their geometry was authored, not in
// what a family claim is, so RuleFamilyTarget is the only thing this entry
// knows about the claimant.
// The output Factor is named by reference, not by its owner slot. An axis
// owner hands its rules a FactorRef precisely so they cannot declare against
// its Factor, and a family claim needs no more than the reference names.
func BindRuleFamily[K ~uint32 | ~uint64, V any](binding *SchemaBinding, slot RuleFamilyTarget, output FactorRef[V], installer execution.RuleFamilyInstaller[K, V]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	ruleCell := ruleFamilyTargetCell(slot)
	if state.phase != schemaBindingOpen || state.schema == nil || ruleCell == nil ||
		ruleCell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || installer == nil {
		if state.phase == schemaBindingOpen {
			state.poisonLocked()
		}
		return false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	factorOrdinal, factorOK := output.ordinal()
	if !ruleOK || !factorOK || ruleOrdinal >= uint64(len(state.rules)) || factorOrdinal >= uint64(len(state.factors)) || state.factors[factorOrdinal] == nil {
		state.poisonLocked()
		return false
	}
	// The claim is fenced by the Factor the rule publishes at. A family
	// installed against any other Factor is typed in a key and fact the rule
	// never publishes, and the claim would simply never be resolved: the Factor
	// it names does not own the rule's rows. Refusing it here names the mistake
	// where it is made.
	//
	// Which Factor that is comes from the rule's own declared geometry, and the
	// two output kinds state it differently. A fact-writing rule names its
	// output Factor in the cold row, and the claim is fenced by that semantic.
	// A structural rule writes no fact and names no output Factor at all: its
	// output is the activation row set its candidate branches mount into the
	// construct topology. What its rows still have is the axis they are indexed
	// by, which is the axis whose typed plane they are built on, so that axis
	// is what fences the claim. One seam, two declared geometries.
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !shapeOK {
		state.poisonLocked()
		return false
	}
	switch shape.OutputKind {
	case composition.FactorOutput:
		if shape.Output != state.factors[factorOrdinal].schemaFactorSemanticKey() {
			state.poisonLocked()
			return false
		}
	case composition.StructuralOutput:
		if !structuralRuleFamilyAxis(state, ruleOrdinal, shape, factorOrdinal) {
			state.poisonLocked()
			return false
		}
	default:
		state.poisonLocked()
		return false
	}
	// The installer must be typed in that Factor's own key and fact. A family
	// typed at another coordinate is one this Factor's binding could never
	// resolve, and resolving it is a type assertion made at Program seal - so
	// an unnamed refusal there reads as a Factor that will not bind rather than
	// as the declaration that mistyped it. Two nominal coordinates over one
	// width are the case this actually catches.
	if !state.factors[factorOrdinal].schemaFactorAdmitsRuleFamily(installer) {
		state.poisonNamed("rule-family/foreign-coordinate")
		return false
	}
	// One rule ordinal has one family. A second claim is two authorities for
	// one rule's execution, which no order between them could resolve.
	if _, claimed := state.ruleFamilies[ruleOrdinal]; claimed {
		state.poisonLocked()
		return false
	}
	if state.ruleFamilies == nil {
		state.ruleFamilies = map[uint64]ruleFamilyClaim{}
	}
	state.ruleFamilies[ruleOrdinal] = ruleFamilyClaim{factor: factorOrdinal, installer: installer}
	return true
}

// BindRule binds the sole direct Rule lane currently implemented.
// The output argument is a FactorSlot because FactorImplementationAt is only
// issued after the complete Binding seals; the slot resolves to that exact
// typed Factor cell during Seal and runtime member construction.
func BindRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || spec.OperandResolver == nil || spec.Fold == nil || projectWrite == nil {
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
	cell.impl = &ruleHotImplementation[K, V, O]{state: state, rule: slot, write: write, output: outputCell, operandContent: spec.OperandContent, operandResolver: spec.OperandResolver, fold: spec.Fold, projectWrite: projectWrite}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindRuleWithCarry binds the direct one-write/one-carry lane. The
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
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || spec.OperandResolver == nil || spec.Fold == nil || projectWrite == nil {
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
		operandContent: spec.OperandContent, operandResolver: spec.OperandResolver, fold: spec.Fold, projectWrite: projectWrite,
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindRuleWithExactReadAndCarry binds the one exact-read/one-carry/one-exact
// write lane. The sealed read and carry capabilities remain the sole
// source of their input, Factor, and transform identities; this method adds no
// parallel structural representation.
func BindRuleWithExactReadAndCarry[OK ~uint32 | ~uint64, V, O any, RK ~uint32 | ~uint64, RV any](binding *SchemaBinding, slot *RuleSlot[V, O], readSlot SchemaReadSlot[RV], readFactor *FactorSlot[RV], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output *FactorSlot[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor == nil || readFactor.cell == nil || readFactor.cell.schema != state.schema || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output == nil || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || spec.OperandResolver == nil || spec.Fold == nil || projectWrite == nil {
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
	outputOrdinal, outputOK := output.Ordinal()
	readFactorOrdinal, readFactorOK := readFactor.Ordinal()
	if !outputOK || !readFactorOK || outputOrdinal >= uint64(len(state.factors)) || readFactorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output || state.schema.factorSemanticAt(readFactorOrdinal) != readShape.Factor {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	outputCell, outputTyped := state.factors[outputOrdinal].(*schemaFactorBindingCell[OK, V])
	readCell, readTyped := state.factors[readFactorOrdinal].(*schemaFactorBindingCell[RK, RV])
	if !outputTyped || !readTyped || outputCell == nil || readCell == nil || outputCell.impl == nil || readCell.impl == nil || outputCell.impl.algebra == nil || readCell.impl.algebra == nil || outputCell.state != state || readCell.state != state {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	cell := &schemaRuleBindingCellImpl[OK, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	row, rowOK := compileSchemaRuleReadRow(state, cell, ruleOrdinal, 0, nil, 0)
	if !rowOK || row.factorOrdinal != readFactorOrdinal || !readCell.schemaFactorReadComplete(state, row) {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	read := Read[OrderedCells[RV]]{row: row, index: 0, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	readBinding := &schemaExactRuleReadBinding[RK, RV]{row: row, factor: readCell, read: read, projector: projectExactLocal(projectRead)}
	cell.impl = &ruleHotImplementation[OK, V, O]{
		state: state, rule: slot, write: write, output: outputCell,
		carry:          &schemaRuleCarryBinding[OK, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply},
		reads:          []schemaRuleReadBinding{readBinding},
		operandContent: spec.OperandContent, operandResolver: spec.OperandResolver, fold: spec.Fold,
		projectWrite: projectWrite,
	}
	state.rules[ruleOrdinal] = cell
	return read, true
}

// RuleImplementationAt issues the typed sealed Rule row only after the shared
// Binding has sealed. Its cell and ordinal are checked again when attached to
// an execution member.
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
	if !ok || cell == nil || !cell.sealedRuleComplete() {
		return nil, false
	}
	output, outputOK := cell.impl.output.sealedImplementation(state, state.authority)
	if !outputOK || output == nil || !factorRowAvailable(output.row) {
		return nil, false
	}
	implementation := &RuleImplementation[K, V, O]{cell: cell, ordinal: ordinal, output: output.row}
	if _, valid := implementation.sealedRuleCell(); !valid {
		return nil, false
	}
	return implementation, true
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
	// never mutated after Seal, so concurrent callers cannot observe a
	// descriptor being rewritten underneath a live runtime binder.
	return cell.sealedImplementation(state, state.authority)
}

// structuralRuleFamilyAxis reports whether one structural rule's family claim
// names the axis its rows are indexed by.
//
// A structural rule has no Output semantic for the claim to be fenced by, so
// the fence is the output axis its sealed descriptor carries - the same axis
// the execution ladder routes its rows to. The descriptor exists only once the
// rule itself is bound, which is where a structural claim is made: the rule
// installs its own family at its own bind, on the ordinal it just took.
//
// The activation family is required alongside it. A structural row whose cold
// capability is a prune rather than an activation publishes no candidate
// branch set, so a family claiming to author its execution would author rows
// nothing mounts.
func structuralRuleFamilyAxis(state *schemaBindingState, ruleOrdinal uint64, shape composition.RuleShape, factorOrdinal uint64) bool {
	if state == nil || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() || shape.Output.Available() || shape.WriteCount != 0 {
		return false
	}
	cell, generated := state.rules[ruleOrdinal].(*generatedRuleBindingCell)
	if !generated || cell == nil || cell.generated == nil || !cell.generated.available() {
		return false
	}
	return uint64(cell.generated.program.OutputFactor()) == factorOrdinal
}

// ruleFamilyTargetCell resolves the declaration cell of a family claim target.
// A nil interface and a typed nil pointer are the same absent claimant here,
// so the entry above states one refusal rather than two.
func ruleFamilyTargetCell(slot RuleFamilyTarget) *schemaTokenCell {
	if slot == nil {
		return nil
	}
	return slot.ruleFamilyCell()
}
