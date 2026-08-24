// schema_rule_binding.go binds Rules: hot specs, carry, compiled read rows and direct sealed cells.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// HotRuleSpec is the owner-local half of a Factor-output Rule. The RuleSlot,
// write slot, and output Factor cell supply all structural identity; callers
// cannot restate semantic keys or output shape here.
type HotRuleSpec[V, O any] struct {
	OperandContent func(O) (O, [32]byte, bool)
	// OperandResolver is the owner-issued coordinate lookup for this Rule.
	// It is captured while the binding is open and becomes immutable with the
	// sealed Rule cell.  No post-seal installation path exists.
	OperandResolver func(OperandCoords) (O, bool)
	Fold            func(Frame[V, O]) RuleResult[V]
}

// HotCarrySpec is the executable half of one declared whole-Factor carry.
// The cold SchemaCarrySlot supplies the input, Factor, and optional transform
// identity; only the typed transform closure crosses the hot boundary.
// Apply is nil for an ordinary identity carry and required for a transformed
// carry. No semantic key is accepted from the hot caller.
type HotCarrySpec[V, O any] struct {
	Apply func(O, V) (V, bool)
}

// ruleHotImplementation is retained only by the sealed Rule cell. Callers do
// not receive a copy of these callbacks: the sealed cell binding is the
// sole path back to this exact implementation.
type ruleHotImplementation[K ~uint32 | ~uint64, V, O any] struct {
	state  *schemaBindingState
	rule   *RuleSlot[V, O]
	write  SchemaWriteSlot[V]
	output *schemaFactorBindingCell[K, V]
	carry  *schemaRuleCarryBinding[K, V, O]
	reads  []schemaRuleReadBinding
	// The fields below are populated exactly once by SchemaBinding.Seal after
	// every open Rule validation has succeeded.  They are the sealed Rule's
	// direct geometry; the draft Rule/write/carry handles above are then
	// cleared and cannot participate in runtime admission.
	ruleSemantic    composition.Key
	operandFamily   composition.Key
	writeMode       directRuleWriteMode
	routeRead       uint64
	carryPresent    bool
	carryInput      uint64
	carryTransform  composition.Key
	carryApply      func(O, V) (V, bool)
	operandContent  func(O) (O, [32]byte, bool)
	operandResolver func(OperandCoords) (O, bool)
	fold            func(Frame[V, O]) RuleResult[V]
	projectWrite    func(O) (uint64, bool)
}

// schemaRuleCarryBinding is the typed cell for the one supported carry lane.
// It retains the opaque sealed token and exact output Factor cell; all cold
// shape checks continue to query Schema by canonical ordinal.
type schemaRuleCarryBinding[K ~uint32 | ~uint64, V, O any] struct {
	state   *schemaBindingState
	cell    schemaRuleBindingCell
	ordinal uint64
	slot    SchemaCarrySlot[V]
	factor  *schemaFactorBindingCell[K, V]
	apply   func(O, V) (V, bool)
}

func (carry *schemaRuleCarryBinding[K, V, O]) complete(state *schemaBindingState, ruleCell schemaRuleBindingCell, ruleOrdinal uint64, output *schemaFactorBindingCell[K, V]) bool {
	if carry == nil || state == nil || ruleCell == nil || output == nil || carry.state != state || carry.cell != ruleCell || carry.ordinal != ruleOrdinal || carry.slot.cell == nil || carry.slot.cell.schema != state.schema || carry.slot.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.slot.cell.ordinal)) != 0 || carry.factor != output || output.schema != state.schema || output.impl == nil || output.state != state {
		return false
	}
	shape, ok := state.schema.ruleShapeAt(ruleOrdinal)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	if !ok || !carryOK || shape.CarryCount != 1 || carryShape.Factor != shape.Output || carryShape.Input >= shape.Inputs || !schemaRuleInputFitsInt(carryShape.Input) || carry.factor.ordinal >= uint64(len(state.factors)) || state.factors[carry.factor.ordinal] != carry.factor || state.schema.factorSemanticAt(carry.factor.ordinal) != shape.Output {
		return false
	}
	return (carryShape.Transform.Available() && carry.apply != nil) || (!carryShape.Transform.Available() && carry.apply == nil)
}

// schemaRuleReadRow is the one immutable compiled geometry row for a typed
// Rule read. It replaces the former Origin, which repeatedly reopened Schema
// to recover the same shape. The existing sealed Rule/activation cell is the
// owner; no Schema, binding state, draft slot, or form handle is retained.
// The only retained form value is the existing sealed summary-form cell needed
// by a summary normalizer; no Schema or mutable construction handle crosses
// into the row. A returned Read and its existing ruleHotImplementation.reads
// entry share this exact pointer.
type schemaRuleReadRow struct {
	owner          schemaRuleBindingCell
	ownerOrdinal   uint64
	readOrdinal    uint64
	input          uint64
	kind           composition.ReadKind
	factorOrdinal  uint64
	factor         composition.Key
	semantic       composition.Key
	normalizer     composition.Key
	dependencies   []uint64
	summaryForm    schemaFactorFormBinding
	summaryOrdinal uint64
}

func (row *schemaRuleReadRow) ownerState() *schemaBindingState {
	if row == nil || row.owner == nil {
		return nil
	}
	return row.owner.schemaRuleBindingState()
}

func (row *schemaRuleReadRow) live(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return row != nil && state != nil && cell != nil && row.owner == cell && row.ownerOrdinal == ordinal && schemaRuleInputFitsInt(row.readOrdinal) && cell.schemaRuleOrdinal() == ordinal && cell.schemaRuleBindingState() == state
}

func (row *schemaRuleReadRow) sealed() bool {
	state := row.ownerState()
	if state == nil || !row.live(state, row.owner, row.ownerOrdinal) || state.phase != schemaBindingSealed || state.authority == nil || row.ownerOrdinal >= uint64(len(state.rules)) {
		return false
	}
	return state.rules[row.ownerOrdinal] == row.owner
}

// The compiled row is also the staged selector seam. This avoids a second
// selector wrapper containing a duplicate read/dependency vector.
func (row *schemaRuleReadRow) selectorReadIndex() int {
	if row == nil || !schemaRuleInputFitsInt(row.readOrdinal) {
		return -1
	}
	return int(row.readOrdinal)
}

func (row *schemaRuleReadRow) selectorDeclaresRead(index int) bool {
	if row == nil || index < 0 {
		return false
	}
	for _, dependency := range row.dependencies {
		if dependency == uint64(index) {
			return true
		}
	}
	return false
}

func (row *schemaRuleReadRow) selectorDependencyCount() int {
	if row == nil {
		return 0
	}
	return len(row.dependencies)
}

func compileSchemaRuleReadRow(state *schemaBindingState, owner schemaRuleBindingCell, ruleOrdinal, readOrdinal uint64, summaryForm schemaFactorFormBinding, summaryOrdinal uint64) (*schemaRuleReadRow, bool) {
	if state == nil || state.phase != schemaBindingOpen || state.schema == nil || owner == nil || owner.schemaBindingSchema() != state.schema || owner.schemaRuleOrdinal() != ruleOrdinal || !schemaRuleInputFitsInt(readOrdinal) {
		return nil, false
	}
	shape, shapeOK := state.schema.ruleReadShapeAt(ruleOrdinal, readOrdinal)
	factorOrdinal, factorOK := state.schema.factorOrdinalOf(shape.Factor)
	ruleShape, ruleOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !shapeOK || !factorOK || !ruleOK || factorOrdinal >= uint64(len(state.factors)) || state.factors[factorOrdinal] == nil || shape.Input >= ruleShape.Inputs || !schemaRuleInputFitsInt(shape.Input) || !schemaRuleInputFitsInt(shape.DependencyCount) {
		return nil, false
	}
	row := &schemaRuleReadRow{
		owner: owner, ownerOrdinal: ruleOrdinal, readOrdinal: readOrdinal,
		input: shape.Input, kind: shape.Kind, factorOrdinal: factorOrdinal,
		factor: shape.Factor, semantic: shape.Semantic, normalizer: shape.Normalizer,
	}
	switch shape.Kind {
	case composition.ReadExact:
		if shape.Semantic.Available() || shape.Normalizer.Available() || shape.DependencyCount != 0 || summaryForm != nil {
			return nil, false
		}
	case composition.ReadSelect:
		if shape.Semantic != shape.Factor || shape.Normalizer.Available() || summaryForm != nil || !validReadDependencies(state.schema, ruleOrdinal, readOrdinal, shape.DependencyCount) {
			return nil, false
		}
		row.dependencies = make([]uint64, int(shape.DependencyCount))
		for index := range row.dependencies {
			dependency, dependencyOK := state.schema.ruleReadDependencyAt(ruleOrdinal, readOrdinal, uint64(index))
			if !dependencyOK {
				return nil, false
			}
			row.dependencies[index] = dependency
		}
	case composition.ReadSummary:
		if !shape.Semantic.Available() || shape.Normalizer != shape.Semantic || shape.DependencyCount != 0 || summaryForm == nil {
			return nil, false
		}
		formShape, formOK := state.schema.factorFormShapeAt(factorOrdinal, summaryOrdinal)
		if !formOK || !summaryReadRowKind(formShape.Kind) || formShape.Semantic != shape.Semantic || summaryForm.schemaBindingSchema() != state.schema {
			return nil, false
		}
		row.summaryForm, row.summaryOrdinal = summaryForm, summaryOrdinal
	default:
		return nil, false
	}
	return row, true
}

type schemaRuleReadBinding interface {
	complete(*schemaBindingState, schemaRuleBindingCell, uint64) bool
	bind(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor) bool
	readRow() *schemaRuleReadRow
	projectLocal(any) (uint64, bool)
	exactAdmitFactor() schemaFactorBinding
}

type directRuleWriteMode uint8

const (
	directRuleWriteExact directRuleWriteMode = iota + 1
	directRuleWriteRoute
	// directRuleWriteStructural is the publication disposition of a rule that
	// writes no fact. Its output is the activation row set its candidate
	// branches mount into the construct topology, so it names no write shape
	// and no destination: there is no Factor cell for one to address.
	directRuleWriteStructural
)

// schemaRuleInputFitsInt is the single admission bound for a declared Rule
// input that will cross into the runtime's int-indexed read/carry lanes. The
// bound is checked while the cold shape and draft handles are still present;
// sealed cells retain only the already-safe scalar.
func schemaRuleInputFitsInt(input uint64) bool {
	return input <= uint64(^uint(0)>>1)
}

// bindSelectedRuleDirectCell installs the final Rule cell for one direct
// ordinal lane. Its cold shape selects the carry/write geometry; every read
// slot is populated later at its packed ordinal and SchemaBinding.Seal checks
// the completed inventory.
func bindSelectedRuleDirectCell[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool), carryRequired bool, writeMode directRuleWriteMode) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || spec.OperandResolver == nil || spec.Fold == nil || (writeMode != directRuleWriteExact && writeMode != directRuleWriteRoute) || writeMode == directRuleWriteExact && projectWrite == nil || carryRequired && (carry.cell == nil || carry.cell.schema != state.schema) {
		state.poisonLocked()
		return false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || write.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(write.cell.ordinal)) != 0 || carryRequired && (carry.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.cell.ordinal)) != 0) {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	carryCount := uint64(0)
	if carryRequired {
		carryCount = 1
	}
	if !shapeOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.WriteCount != 1 || shape.CarryCount != carryCount || writeShape.Factor != shape.Output || shape.ReadCount > uint64(^uint(0)>>1) {
		state.poisonLocked()
		return false
	}
	if carryRequired {
		carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
		if !carryOK || carryShape.Input >= shape.Inputs || carryShape.Factor != shape.Output || (carryShape.Transform.Available() != (carrySpec.Apply != nil)) {
			state.poisonLocked()
			return false
		}
	}
	switch writeMode {
	case directRuleWriteExact:
		if writeShape.Kind != composition.WriteExact || writeShape.Route != 0 {
			state.poisonLocked()
			return false
		}
	case directRuleWriteRoute:
		if writeShape.Kind != composition.WriteRoute || writeShape.Route == 0 || writeShape.Route > shape.ReadCount {
			state.poisonLocked()
			return false
		}
		routeRead, routeReadOK := state.schema.ruleReadShapeAt(ruleOrdinal, writeShape.Route-1)
		if !routeReadOK || routeRead.Kind != composition.ReadSelect || routeRead.Factor != shape.Output || routeRead.Semantic != routeRead.Factor || routeRead.Normalizer.Available() || routeRead.DependencyCount == 0 {
			state.poisonLocked()
			return false
		}
	}
	outputOrdinal, outputOK := factorRefOrdinal(output, state.schema)
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.ordinal != outputOrdinal || outputCell.schema != state.schema || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	var carryBinding *schemaRuleCarryBinding[K, V, O]
	if carryRequired {
		carryBinding = &schemaRuleCarryBinding[K, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply}
	}
	cell.impl = &ruleHotImplementation[K, V, O]{
		state: state, rule: slot, write: write, output: outputCell,
		carry:          carryBinding,
		reads:          make([]schemaRuleReadBinding, int(shape.ReadCount)),
		operandContent: spec.OperandContent, operandResolver: spec.OperandResolver, fold: spec.Fold,
		projectWrite: projectWrite,
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindSelectedRuleDirect installs the final exact-write/one-carry Rule cell
// at its declared ordinal.
func BindSelectedRuleDirect[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	return bindSelectedRuleDirectCell[K](binding, slot, carry, write, output, spec, carrySpec, projectWrite, true, directRuleWriteExact)
}

// BindSelectedExactRuleDirect installs the final exact-write/no-carry Rule
// cell at its declared ordinal.
func BindSelectedExactRuleDirect[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	return bindSelectedRuleDirectCell[K](binding, slot, SchemaCarrySlot[V]{}, write, output, spec, HotCarrySpec[V, O]{}, projectWrite, false, directRuleWriteExact)
}

// BindSelectedRouteRuleDirect installs the final routed-write/one-carry Rule
// cell at its declared ordinal. Its route read is validated from the cold
// write row.
func BindSelectedRouteRuleDirect[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	return bindSelectedRuleDirectCell[K](binding, slot, carry, write, output, spec, carrySpec, projectWrite, true, directRuleWriteRoute)
}

func directRuleReadCell[K ~uint32 | ~uint64, V, O, RV any](state *schemaBindingState, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV]) (*schemaRuleBindingCellImpl[K, V, O], uint64, schemaFactorBinding, bool) {
	if state == nil || state.phase != schemaBindingOpen || rule == nil || rule.cell == nil || rule.cell.schema != state.schema || slot.cell == nil || slot.cell.schema != state.schema || factor.cell == nil || factor.cell.schema != state.schema {
		if state != nil {
			state.poisonLocked()
		}
		return nil, 0, nil, false
	}
	ruleOrdinal, ruleOK := rule.Ordinal()
	packed := slot.cell.ordinal
	readOrdinal := uint64(uint32(packed))
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || packed>>32 != ruleOrdinal {
		state.poisonLocked()
		return nil, 0, nil, false
	}
	cell, cellOK := state.rules[ruleOrdinal].(*schemaRuleBindingCellImpl[K, V, O])
	boundOrdinal, boundOK := uint64(0), false
	if cellOK && cell != nil && cell.impl != nil && cell.impl.rule != nil {
		boundOrdinal, boundOK = cell.impl.rule.Ordinal()
	}
	ruleShape, ruleShapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !cellOK || cell == nil || cell.state != state || cell.schema != state.schema || cell.ordinal != ruleOrdinal || cell.impl == nil || !boundOK || boundOrdinal != ruleOrdinal || !ruleShapeOK || uint64(len(cell.impl.reads)) != ruleShape.ReadCount || readOrdinal >= uint64(len(cell.impl.reads)) || cell.impl.reads[int(readOrdinal)] != nil {
		state.poisonLocked()
		return nil, 0, nil, false
	}
	factorOrdinal, factorOK := factorRefOrdinal(factor, state.schema)
	if !factorOK || factorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(factorOrdinal) == (composition.Key{}) {
		state.poisonNamed(readForeignOwnerRefusal)
		return nil, 0, nil, false
	}
	factorCell := state.factors[factorOrdinal]
	if factorCell == nil {
		state.poisonNamed(readForeignOwnerRefusal)
		return nil, 0, nil, false
	}
	return cell, readOrdinal, factorCell, true
}

// BindSelectedRuleDirectExactRead installs one exact typed Read into the
// already-installed direct Rule cell at the read slot's packed cold ordinal.
// The slot ordinal, not call order, chooses the immutable read position.
func BindSelectedRuleDirectExactRead[K ~uint32 | ~uint64, V, O, RV any](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], project func(O) (uint64, bool)) (Read[OrderedCells[RV]], bool) {
	return BindSelectedRuleDirectExactReadUnderContract[K, V, O, RV](binding, rule, slot, factor, project, ReadContract{})
}

// BindSelectedRuleDirectExactReadUnderContract is BindSelectedRuleDirectExactRead
// under an explicit read-boundary contract. An exact read has one coordinate
// and no alternative set, so only the sparse clause is admitted here.
func BindSelectedRuleDirectExactReadUnderContract[K ~uint32 | ~uint64, V, O, RV any](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], project func(O) (uint64, bool), contract ReadContract) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
	if !contract.exactValid() {
		if state != nil {
			state.mu.Lock()
			state.poisonNamed(readContractRefusal)
			state.mu.Unlock()
		}
		return Read[OrderedCells[RV]]{}, false
	}
	if state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	cell, readOrdinal, factorCell, ok := directRuleReadCell[K](state, rule, slot, factor)
	if !ok {
		return Read[OrderedCells[RV]]{}, false
	}
	shape, shapeOK := state.schema.ruleReadShapeAt(cell.ordinal, readOrdinal)
	factorOrdinal, factorOK := factorRefOrdinal(factor, state.schema)
	if !shapeOK || shape.Kind != composition.ReadExact || shape.DependencyCount != 0 || !factorOK || state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		state.poisonNamed(readForeignOwnerRefusal)
		return Read[OrderedCells[RV]]{}, false
	}
	row, rowOK := compileSchemaRuleReadRow(state, cell, cell.ordinal, readOrdinal, nil, 0)
	if !rowOK || row.factorOrdinal != factorOrdinal || !factorCell.schemaFactorReadComplete(state, row) {
		state.poisonNamed(readForeignOwnerRefusal)
		return Read[OrderedCells[RV]]{}, false
	}
	read := Read[OrderedCells[RV]]{row: row, index: int(readOrdinal), resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueExactRuleReadBinding[RV]{
		row: row, factor: factorCell, read: read,
		projector: projectExactLocal(project), contract: contract,
	}
	return read, true
}

// BindSelectedRuleDirectSelectedRead installs one static-selector Read into
// the direct Rule cell at its packed cold ordinal. Its dependency vector and
// selected Factor geometry are revalidated from the sealed Schema.
func BindSelectedRuleDirectSelectedRead[K ~uint32 | ~uint64, V, O, RV any, Tag selectionTag](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext) bool) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	return BindSelectedRuleDirectSelectedReadUnderContract[K, V, O, RV, Tag](binding, rule, slot, factor, locate, ReadContract{})
}

// BindSelectedRuleDirectSelectedReadUnderContract is
// BindSelectedRuleDirectSelectedRead under an explicit read-boundary contract.
func BindSelectedRuleDirectSelectedReadUnderContract[K ~uint32 | ~uint64, V, O, RV any, Tag selectionTag](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext) bool, contract ReadContract) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	state := bindingState(binding)
	if !contract.valid() {
		if state != nil {
			state.mu.Lock()
			state.poisonNamed(readContractRefusal)
			state.mu.Unlock()
		}
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	if state == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if locate == nil {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	cell, readOrdinal, factorCell, ok := directRuleReadCell[K](state, rule, slot, factor)
	if !ok {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shape, shapeOK := state.schema.ruleReadShapeAt(cell.ordinal, readOrdinal)
	factorOrdinal, factorOK := factorRefOrdinal(factor, state.schema)
	if !shapeOK || shape.Kind != composition.ReadSelect || !validReadDependencies(state.schema, cell.ordinal, readOrdinal, shape.DependencyCount) || !factorOK || state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		state.poisonNamed(readForeignOwnerRefusal)
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	row, rowOK := compileSchemaRuleReadRow(state, cell, cell.ordinal, readOrdinal, nil, 0)
	if !rowOK || row.factorOrdinal != factorOrdinal || !factorCell.schemaFactorReadComplete(state, row) {
		state.poisonNamed(readForeignOwnerRefusal)
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	read := Read[Selection[Tag, OrderedCells[RV]]]{row: row, index: int(readOrdinal), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueSelectedRuleReadBinding[RV, Tag]{row: row, factor: factorCell, read: read, locate: locate, contract: contract}
	return read, true
}

// BindSelectedRuleDirectOperandRead installs one operand-dependent selector
// Read into the direct Rule cell at its packed cold ordinal. The operand is
// resolved only later by the canonical bound Rule during graph attachment.
func BindSelectedRuleDirectOperandRead[K ~uint32 | ~uint64, V, O, RV any, Tag selectionTag](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext, O) bool) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	return BindSelectedRuleDirectOperandReadUnderContract[K, V, O, RV, Tag](binding, rule, slot, factor, locate, ReadContract{})
}

// BindSelectedRuleDirectOperandReadUnderContract is
// BindSelectedRuleDirectOperandRead under an explicit read-boundary contract.
// It is the declaration a rule makes once so its locator and its Fold both
// receive the members in the declared order, with the Factor's default at every
// unwritten coordinate and the declared disposition of an opaque alternative.
func BindSelectedRuleDirectOperandReadUnderContract[K ~uint32 | ~uint64, V, O, RV any, Tag selectionTag](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext, O) bool, contract ReadContract) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	state := bindingState(binding)
	if !contract.valid() {
		if state != nil {
			state.mu.Lock()
			state.poisonNamed(readContractRefusal)
			state.mu.Unlock()
		}
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	if state == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if locate == nil {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	cell, readOrdinal, factorCell, ok := directRuleReadCell[K](state, rule, slot, factor)
	if !ok {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shape, shapeOK := state.schema.ruleReadShapeAt(cell.ordinal, readOrdinal)
	factorOrdinal, factorOK := factorRefOrdinal(factor, state.schema)
	if !shapeOK || shape.Kind != composition.ReadSelect || !validReadDependencies(state.schema, cell.ordinal, readOrdinal, shape.DependencyCount) || !factorOK || state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		state.poisonNamed(readForeignOwnerRefusal)
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	row, rowOK := compileSchemaRuleReadRow(state, cell, cell.ordinal, readOrdinal, nil, 0)
	if !rowOK || row.factorOrdinal != factorOrdinal || !factorCell.schemaFactorReadComplete(state, row) {
		state.poisonNamed(readForeignOwnerRefusal)
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	read := Read[Selection[Tag, OrderedCells[RV]]]{row: row, index: int(readOrdinal), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]{row: row, factor: factorCell, read: read, locateOperand: locate, contract: contract}
	return read, true
}

// BindSelectedRuleDirectSummaryRead installs one typed summary Read into the
// direct Rule cell at its packed cold ordinal. The sealed Rule row supplies
// the summary semantic and zero-dependency geometry; the Factor form supplies
// the typed normalizer and closed summary row. No read is appended.
func BindSelectedRuleDirectSummaryRead[K ~uint32 | ~uint64, V, O, RV, S any](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], form SchemaReadForm[RV]) (Read[S], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[S]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if form.cell == nil || form.cell.schema != state.schema || !summaryReadFormKind(form.cell.kind) {
		state.poisonLocked()
		return Read[S]{}, false
	}
	cell, readOrdinal, factorCell, ok := directRuleReadCell[K](state, rule, slot, factor)
	if !ok {
		return Read[S]{}, false
	}
	shape, shapeOK := state.schema.ruleReadShapeAt(cell.ordinal, readOrdinal)
	factorOrdinal, factorOK := factorRefOrdinal(factor, state.schema)
	formFactor, formOrdinal := form.cell.ordinal>>32, uint64(uint32(form.cell.ordinal))
	formShape, formShapeOK := state.schema.factorFormShapeAt(factorOrdinal, formOrdinal)
	if !shapeOK || shape.Kind != composition.ReadSummary || shape.DependencyCount != 0 || !shape.Semantic.Available() || shape.Semantic != shape.Normalizer || !factorOK || formFactor != factorOrdinal || !formShapeOK || !summaryReadRowKind(formShape.Kind) || formShape.Semantic != shape.Semantic || state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		state.poisonLocked()
		return Read[S]{}, false
	}
	formCell, formOK := factorCell.schemaFactorFormAt(formOrdinal).(schemaOpaqueSummaryRuleReadForm[RV, S])
	row, rowOK := compileSchemaRuleReadRow(state, cell, cell.ordinal, readOrdinal, formCell, formOrdinal)
	if !formOK || !rowOK || row.factorOrdinal != factorOrdinal || !formCell.schemaSummaryRuleReadComplete(state, row) {
		state.poisonLocked()
		return Read[S]{}, false
	}
	read := Read[S]{row: row, index: int(readOrdinal), resolve: resolveTypedRead[RV, S]}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueSummaryRuleReadBinding[RV, S]{row: row, form: formCell, read: read}
	return read, true
}

// BindRuleWithOpaqueExactRead binds an exact-read Rule while retaining the
// predecessor Factor behind its owner-issued ref. The output owner chooses K;
// the predecessor coordinate is deliberately never guessed at this boundary.
func BindRuleWithOpaqueExactRead[OK ~uint32 | ~uint64, V, O, RV any](binding *SchemaBinding, slot *RuleSlot[V, O], readSlot SchemaReadSlot[RV], readFactor FactorRef[RV], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
	if state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor.cell == nil || readFactor.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || spec.OperandResolver == nil || spec.Fold == nil || projectWrite == nil {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	readPacked, writePacked := readSlot.cell.ordinal, write.cell.ordinal
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || readPacked>>32 != ruleOrdinal || uint64(uint32(readPacked)) != 0 || writePacked>>32 != ruleOrdinal || uint64(uint32(writePacked)) != 0 {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	readShape, readOK := state.schema.ruleReadShapeAt(ruleOrdinal, 0)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.ReadCount != 1 || shape.CarryCount != 0 || shape.WriteCount != 1 || !readOK || readShape.Kind != composition.ReadExact || readShape.Input >= shape.Inputs || readShape.DependencyCount != 0 || !writeOK || writeShape.Kind != composition.WriteExact || writeShape.Factor != shape.Output || writeShape.Route != 0 {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	outputOrdinal, outputOK := factorRefOrdinal(output, state.schema)
	readFactorOrdinal, readFactorOK := factorRefOrdinal(readFactor, state.schema)
	if !outputOK || !readFactorOK || outputOrdinal >= uint64(len(state.factors)) || readFactorOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output || state.schema.factorSemanticAt(readFactorOrdinal) != readShape.Factor {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	outputCell, outputTyped := state.factors[outputOrdinal].(*schemaFactorBindingCell[OK, V])
	readCell := state.factors[readFactorOrdinal]
	if !outputTyped || outputCell == nil || readCell == nil || outputCell.impl == nil || readCell.schemaFactorSchema() != state.schema || outputCell.impl.algebra == nil || outputCell.state != state {
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
	readBinding := &schemaOpaqueExactRuleReadBinding[RV]{row: row, factor: readCell, read: read, projector: projectExactLocal(projectRead)}
	cell.impl = &ruleHotImplementation[OK, V, O]{state: state, rule: slot, write: write, output: outputCell, reads: []schemaRuleReadBinding{readBinding}, operandContent: spec.OperandContent, operandResolver: spec.OperandResolver, fold: spec.Fold, projectWrite: projectWrite}
	state.rules[ruleOrdinal] = cell
	return read, true
}
