// schema_rule_binding.go binds Rules: hot specs, carry, read origin and the selected-route binding transaction.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// HotRuleSpec is the Link-local half of a Factor-output Rule. The RuleSlot,
// write slot, and output Factor cell supply all structural identity; callers
// cannot restate semantic keys or output shape here.
type HotRuleSpec[V, O any] struct {
	OperandContent func(O) (O, [32]byte, bool)
	Admission      RuleAdmission[V, O]
	Transfer       func(Access[V, O]) bool
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
// not receive a copy of these callbacks: the cell-issued receipt below is the
// sole path back to this exact implementation.
type ruleHotImplementation[K ~uint32 | ~uint64, V, O any] struct {
	state           *schemaBindingState
	rule            *RuleSlot[V, O]
	write           SchemaWriteSlot[V]
	output          *schemaFactorBindingCell[K, V]
	carry           *schemaRuleCarryBinding[K, V, O]
	reads           []schemaRuleReadBinding
	operandContent  func(O) (O, [32]byte, bool)
	operandResolver func(OperandCoords) (O, bool)
	admission       RuleAdmission[V, O]
	transfer        func(Access[V, O]) bool
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
	if carry == nil || state == nil || ruleCell == nil || output == nil || carry.state != state || carry.cell != ruleCell || carry.ordinal != ruleOrdinal || carry.slot.cell == nil || carry.slot.cell.schema != state.schema || carry.slot.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.slot.cell.ordinal)) != 0 || carry.factor != output || output.schema != state.schema || output.impl == nil || output.impl.state != state {
		return false
	}
	shape, ok := state.schema.ruleShapeAt(ruleOrdinal)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	if !ok || !carryOK || shape.CarryCount != 1 || carryShape.Factor != shape.Output || carryShape.Input >= shape.Inputs || carry.factor.ordinal >= uint64(len(state.factors)) || state.factors[carry.factor.ordinal] != carry.factor || state.schema.factorSemanticAt(carry.factor.ordinal) != shape.Output {
		return false
	}
	return (carryShape.Transform.Available() && carry.apply != nil) || (!carryShape.Transform.Available() && carry.apply == nil)
}

func (carry *schemaRuleCarryBinding[K, V, O]) shape() (composition.RuleCarryShape, bool) {
	if carry == nil || carry.state == nil || carry.cell == nil {
		return composition.RuleCarryShape{}, false
	}
	return carry.state.schema.ruleCarryShapeAt(carry.ordinal, 0)
}

// schemaRuleReadOrigin is the non-generic owner proof retained by a typed
// Read. It names one exact canonical read cell but no equation surface or
// carrier Unit. The latter are introduced only by the graph-owned member.
type schemaRuleReadOrigin struct {
	state           *schemaBindingState
	cell            schemaRuleBindingCell
	ruleOrdinal     uint64
	readOrdinal     uint64
	input           uint64
	factor          uint64
	kind            composition.ReadKind
	semantic        composition.Key
	formOrdinal     uint64
	dependencyCount uint64
}

func (origin *schemaRuleReadOrigin) matches(proof *ruleRuntimeProof, ordinal uint64) bool {
	if origin == nil || proof == nil || ordinal != origin.readOrdinal || origin.state == nil || origin.cell == nil || origin.state.phase != schemaBindingSealed || origin.state.authority == nil || proof.state != origin.state || proof.schema != origin.state.schema || proof.bindingAuthority != origin.state.authority || proof.ordinal != origin.ruleOrdinal || origin.ruleOrdinal >= uint64(len(origin.state.rules)) || origin.state.rules[origin.ruleOrdinal] != origin.cell || origin.cell.schemaBindingSchema() != origin.state.schema || !origin.cell.schemaRuleProofMatches(proof) {
		return false
	}
	shape, ok := origin.state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	if !ok || shape.Kind != origin.kind || shape.Input != origin.input || shape.Factor != origin.state.schema.factorSemanticAt(origin.factor) {
		return false
	}
	if origin.kind == composition.ReadSelect {
		selectedRead := proof.selectedReadAt(origin.readOrdinal)
		return selectedRead != nil && selectedRead.Valid() && selectedRead.fence.rule == origin.ruleOrdinal && selectedRead.read == origin.readOrdinal && selectedRead.factor == origin.factor && selectedRead.dependencyCount == origin.dependencyCount && shape.Semantic == shape.Factor
	}
	if origin.kind == composition.ReadSummary {
		if shape.DependencyCount != 0 {
			return false
		}
		form, formOK := origin.state.schema.factorFormShapeAt(origin.factor, origin.formOrdinal)
		return formOK && summaryReadRowKind(form.Kind) && form.Semantic == origin.semantic && shape.Semantic == origin.semantic && shape.Normalizer == origin.semantic
	}
	return origin.kind == composition.ReadExact && !origin.semantic.Available() && shape.DependencyCount == 0
}

type schemaRuleReadBinding interface {
	complete(*schemaBindingState, schemaRuleBindingCell, uint64) bool
	bind(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor) bool
	projectLocal(any) (uint64, bool)
	exactAdmitFactor() schemaFactorBinding
}

// SelectedRouteRuleBindingTransaction is the shared-state hot route Rule
// construction cut. It is deliberately named for its current route/carry
// shape; exact-only transactions use their own narrower constructor.
// It keeps the typed output cell private while package-level generic attach
// functions add heterogeneous read receipts in canonical cold ordinal order.
// No read is published until CommitRuleBinding validates the complete Schema.
type SelectedRouteRuleBindingTransaction[K ~uint32 | ~uint64, V, O any] struct {
	shared *selectedRouteTxnState[K, V, O]
}

type schemaRuleBindingToken struct{ marker byte }

// selectedRouteTxnState is the sole mutable transaction authority. Public
// transaction handles are intentionally copyable views of this shared state;
// no copied handle can diverge its read list, reservation, or terminal state.
type selectedRouteTxnState[K ~uint32 | ~uint64, V, O any] struct {
	state        *schemaBindingState
	cell         *schemaRuleBindingCellImpl[K, V, O]
	output       *schemaFactorBindingCell[K, V]
	ordinal      uint64
	rule         *RuleSlot[V, O]
	write        SchemaWriteSlot[V]
	carry        *schemaRuleCarryBinding[K, V, O]
	reads        []schemaRuleReadBinding
	operand      func(O) (O, [32]byte, bool)
	admission    RuleAdmission[V, O]
	transfer     func(Access[V, O]) bool
	projectWrite func(O) (uint64, bool)
	token        *schemaRuleBindingToken
	committed    bool
	aborted      bool
}

// BindSelectedRouteRule owns one route Rule binding transaction: it opens the
// transaction, hands it to bind for read attachment, and terminalizes on
// bind's answer. A rejected bind or a rejected commit aborts, so the shared
// Binding always reaches its terminal state and no caller pairs commit with
// abort by hand.
func BindSelectedRouteRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool), bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	tx, opened := beginSelectedRouteRuleBinding[K](binding, slot, carry, write, output, spec, carrySpec, projectWrite)
	return terminalizeSelectedRouteRuleBinding(tx, opened, bind)
}

// BindSelectedRule is BindSelectedRouteRule's non-routed sibling: the Rule's
// output write is exact rather than routed.
func BindSelectedRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool), bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	tx, opened := beginSelectedRuleBinding[K](binding, slot, carry, write, output, spec, carrySpec, projectWrite)
	return terminalizeSelectedRouteRuleBinding(tx, opened, bind)
}

type directRuleWriteMode uint8

const (
	directRuleWriteExact directRuleWriteMode = iota + 1
	directRuleWriteRoute
)

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
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil || (writeMode != directRuleWriteExact && writeMode != directRuleWriteRoute) || carryRequired && (carry.cell == nil || carry.cell.schema != state.schema) {
		state.poisonLocked()
		return false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || state.pendingRules[ruleOrdinal] != nil || write.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(write.cell.ordinal)) != 0 || carryRequired && (carry.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.cell.ordinal)) != 0) {
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
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return false
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
		operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer,
		projectWrite: projectWrite,
	}
	state.rules[ruleOrdinal] = cell
	return true
}

// BindSelectedRuleDirect installs the final exact-write/one-carry Rule cell
// at its declared ordinal. No pending transaction state is created.
func BindSelectedRuleDirect[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	return bindSelectedRuleDirectCell[K](binding, slot, carry, write, output, spec, carrySpec, projectWrite, true, directRuleWriteExact)
}

// BindSelectedExactRuleDirect installs the final exact-write/no-carry Rule
// cell at its declared ordinal. No pending transaction state is created.
func BindSelectedExactRuleDirect[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], projectWrite func(O) (uint64, bool)) bool {
	return bindSelectedRuleDirectCell[K](binding, slot, SchemaCarrySlot[V]{}, write, output, spec, HotCarrySpec[V, O]{}, projectWrite, false, directRuleWriteExact)
}

// BindSelectedRouteRuleDirect installs the final routed-write/one-carry Rule
// cell at its declared ordinal. Its route read is validated from the cold
// write row; no pending transaction state is created.
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
		state.poisonLocked()
		return nil, 0, nil, false
	}
	factorCell := state.factors[factorOrdinal]
	if factorCell == nil {
		state.poisonLocked()
		return nil, 0, nil, false
	}
	return cell, readOrdinal, factorCell, true
}

// BindSelectedRuleDirectExactRead installs one exact typed Read into the
// already-installed direct Rule cell at the read slot's packed cold ordinal.
// The slot ordinal, not call order, chooses the immutable read position.
func BindSelectedRuleDirectExactRead[K ~uint32 | ~uint64, V, O, RV any](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], project func(O) (uint64, bool)) (Read[OrderedCells[RV]], bool) {
	state := bindingState(binding)
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
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: cell.ordinal, readOrdinal: readOrdinal, input: shape.Input, factor: factorOrdinal, kind: composition.ReadExact}
	read := Read[OrderedCells[RV]]{origin: origin, index: int(readOrdinal), resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	if !factorCell.schemaFactorReadComplete(state, origin) {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueExactRuleReadBinding[RV]{
		origin: origin, factor: factorCell, read: read,
		projector: projectExactLocal(project),
	}
	return read, true
}

// BindSelectedRuleDirectSelectedRead installs one static-selector Read into
// the direct Rule cell at its packed cold ordinal. Its dependency vector and
// selected Factor geometry are revalidated from the sealed Schema.
func BindSelectedRuleDirectSelectedRead[K ~uint32 | ~uint64, V, O, RV any, Tag selectionTag](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext) bool) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	state := bindingState(binding)
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
	if !shapeOK || shape.Kind != composition.ReadSelect || shape.DependencyCount == 0 || !validReadDependencies(state.schema, cell.ordinal, readOrdinal, shape.DependencyCount) || !factorOK || state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: cell.ordinal, readOrdinal: readOrdinal, input: shape.Input, factor: factorOrdinal, kind: composition.ReadSelect, dependencyCount: shape.DependencyCount}
	read := Read[Selection[Tag, OrderedCells[RV]]]{origin: origin, index: int(readOrdinal), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	if !factorCell.schemaFactorReadComplete(state, origin) {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueSelectedRuleReadBinding[RV, Tag]{origin: origin, factor: factorCell, read: read, locate: locate}
	return read, true
}

// BindSelectedRuleDirectOperandRead installs one operand-dependent selector
// Read into the direct Rule cell at its packed cold ordinal. The operand is
// resolved only later by the canonical bound Rule during graph attachment.
func BindSelectedRuleDirectOperandRead[K ~uint32 | ~uint64, V, O, RV any, Tag selectionTag](binding *SchemaBinding, rule *RuleSlot[V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext, O) bool) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	state := bindingState(binding)
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
	if !shapeOK || shape.Kind != composition.ReadSelect || shape.DependencyCount == 0 || !validReadDependencies(state.schema, cell.ordinal, readOrdinal, shape.DependencyCount) || !factorOK || state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: cell.ordinal, readOrdinal: readOrdinal, input: shape.Input, factor: factorOrdinal, kind: composition.ReadSelect, dependencyCount: shape.DependencyCount}
	read := Read[Selection[Tag, OrderedCells[RV]]]{origin: origin, index: int(readOrdinal), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	if !factorCell.schemaFactorReadComplete(state, origin) {
		state.poisonLocked()
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	cell.impl.reads[int(readOrdinal)] = &schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]{origin: origin, factor: factorCell, read: read, locateOperand: locate}
	return read, true
}

// BindSelectedExactRule is the carry-free sibling for Rules whose output write
// is exact and whose cold geometry declares no carry.
func BindSelectedExactRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], projectWrite func(O) (uint64, bool), bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	tx, opened := beginSelectedExactRuleBinding[K](binding, slot, write, output, spec, projectWrite)
	return terminalizeSelectedRouteRuleBinding(tx, opened, bind)
}

// terminalizeSelectedRouteRuleBinding is the sole commit-XOR-abort decision for
// an opened Rule transaction.
func terminalizeSelectedRouteRuleBinding[K ~uint32 | ~uint64, V, O any](tx *SelectedRouteRuleBindingTransaction[K, V, O], opened bool, bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	if !opened || tx == nil || bind == nil {
		return false
	}
	if !bind(tx) || !commitSelectedRouteRuleBinding(tx) {
		abortSelectedRouteRuleBinding(tx)
		return false
	}
	return true
}

// beginSelectedRouteRuleBinding starts one receipt-native route Rule transaction. The exact
// output, carry, and write slots are checked here; reads are attached through
// AddRuleExactRead/AddRuleSelectedRead and committed once.
func beginSelectedRouteRuleBinding[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool)) (*SelectedRouteRuleBindingTransaction[K, V, O], bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return nil, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || state.pendingRules[ruleOrdinal] != nil || carry.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.cell.ordinal)) != 0 || write.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(write.cell.ordinal)) != 0 {
		state.poisonLocked()
		return nil, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !carryOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.CarryCount != 1 || shape.WriteCount != 1 || carryShape.Input >= shape.Inputs || carryShape.Factor != shape.Output || (carryShape.Transform.Available() != (carrySpec.Apply != nil)) || writeShape.Factor != shape.Output || writeShape.Kind != composition.WriteRoute || writeShape.Route == 0 {
		state.poisonLocked()
		return nil, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return nil, false
	}
	outputOrdinal, outputOK := factorRefOrdinal(output, state.schema)
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return nil, false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.schema != state.schema || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return nil, false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	token := &schemaRuleBindingToken{}
	if state.pendingRules == nil {
		state.pendingRules = make(map[uint64]*schemaRuleBindingToken)
	}
	state.pendingRules[ruleOrdinal] = token
	return &SelectedRouteRuleBindingTransaction[K, V, O]{shared: &selectedRouteTxnState[K, V, O]{state: state, cell: cell, output: outputCell, ordinal: ruleOrdinal, rule: slot, write: write, carry: &schemaRuleCarryBinding[K, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply}, operand: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, projectWrite: projectWrite, token: token}}, true
}

// beginSelectedRuleBinding starts the non-routed selected-read transaction.
// It is the sibling of beginSelectedRouteRuleBinding for Rules whose output
// write is exact.  Keeping the transaction type shared is intentional: the
// sealed cell, heterogeneous read receipts, and atomic commit path are the
// same; only the cold write geometry differs.
func beginSelectedRuleBinding[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], projectWrite func(O) (uint64, bool)) (*SelectedRouteRuleBindingTransaction[K, V, O], bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || carry.cell == nil || carry.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return nil, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || state.pendingRules[ruleOrdinal] != nil || carry.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(carry.cell.ordinal)) != 0 || write.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(write.cell.ordinal)) != 0 {
		state.poisonLocked()
		return nil, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	carryShape, carryOK := state.schema.ruleCarryShapeAt(ruleOrdinal, 0)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !carryOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.CarryCount != 1 || shape.WriteCount != 1 || carryShape.Input >= shape.Inputs || carryShape.Factor != shape.Output || (carryShape.Transform.Available() != (carrySpec.Apply != nil)) || writeShape.Factor != shape.Output || writeShape.Kind != composition.WriteExact || writeShape.Route != 0 {
		state.poisonLocked()
		return nil, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return nil, false
	}
	outputOrdinal, outputOK := factorRefOrdinal(output, state.schema)
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return nil, false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.schema != state.schema || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return nil, false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	token := &schemaRuleBindingToken{}
	if state.pendingRules == nil {
		state.pendingRules = make(map[uint64]*schemaRuleBindingToken)
	}
	state.pendingRules[ruleOrdinal] = token
	return &SelectedRouteRuleBindingTransaction[K, V, O]{shared: &selectedRouteTxnState[K, V, O]{state: state, cell: cell, output: outputCell, ordinal: ruleOrdinal, rule: slot, write: write, carry: &schemaRuleCarryBinding[K, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply}, operand: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, projectWrite: projectWrite, token: token}}, true
}

func AddSelectedRouteExactRead[K ~uint32 | ~uint64, V, O any, RV any](tx *SelectedRouteRuleBindingTransaction[K, V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], project func(O) (uint64, bool)) (Read[OrderedCells[RV]], bool) {
	if tx == nil || tx.shared == nil || tx.shared.state == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	shared := tx.shared
	shared.state.mu.Lock()
	defer shared.state.mu.Unlock()
	if shared.committed || shared.aborted || shared.token == nil || shared.state.pendingRules[shared.ordinal] != shared.token || shared.cell == nil || slot.cell == nil || slot.cell.schema != shared.state.schema || factor.cell == nil || factor.cell.schema != shared.state.schema || len(shared.reads) >= int(^uint(0)>>1) {
		return Read[OrderedCells[RV]]{}, false
	}
	index := uint64(len(shared.reads))
	packed := slot.cell.ordinal
	if packed>>32 != shared.ordinal || uint64(uint32(packed)) != index {
		return Read[OrderedCells[RV]]{}, false
	}
	shape, ok := shared.state.schema.ruleReadShapeAt(shared.ordinal, index)
	factorOrdinal, factorOK := factorRefOrdinal(factor, shared.state.schema)
	if !ok || !factorOK || shape.Kind != composition.ReadExact || shape.DependencyCount != 0 || factorOrdinal >= uint64(len(shared.state.factors)) || shared.state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		return Read[OrderedCells[RV]]{}, false
	}
	factorCell := shared.state.factors[factorOrdinal]
	if factorCell == nil {
		return Read[OrderedCells[RV]]{}, false
	}
	origin := &schemaRuleReadOrigin{state: shared.state, cell: shared.cell, ruleOrdinal: shared.ordinal, readOrdinal: index, input: shape.Input, factor: factorOrdinal, kind: composition.ReadExact}
	read := Read[OrderedCells[RV]]{origin: origin, index: int(index), resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	if !factorCell.schemaFactorReadComplete(shared.state, origin) {
		return Read[OrderedCells[RV]]{}, false
	}
	shared.reads = append(shared.reads, &schemaOpaqueExactRuleReadBinding[RV]{
		origin: origin, factor: factorCell, read: read,
		projector: projectExactLocal(project),
	})
	return read, true
}

// AddSelectedRouteSummaryRead appends one summary predecessor while keeping
// the source Factor's coordinate type inside its exact owner cell. The shared
// Rule transaction retains only a typed value/summary receipt and cannot
// guess or expose the sibling Factor's K instantiation.
func AddSelectedRouteSummaryRead[K ~uint32 | ~uint64, V, O, RV, S any](tx *SelectedRouteRuleBindingTransaction[K, V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], form SchemaReadForm[RV], admit any) (Read[S], bool) {
	if tx == nil || tx.shared == nil || tx.shared.state == nil {
		return Read[S]{}, false
	}
	shared := tx.shared
	shared.state.mu.Lock()
	defer shared.state.mu.Unlock()
	if shared.committed || shared.aborted || shared.token == nil || shared.state.pendingRules[shared.ordinal] != shared.token ||
		shared.cell == nil || slot.cell == nil || slot.cell.schema != shared.state.schema || factor.cell == nil ||
		factor.cell.schema != shared.state.schema || form.cell == nil || form.cell.schema != shared.state.schema ||
		!summaryReadFormKind(form.cell.kind) || len(shared.reads) >= int(^uint(0)>>1) {
		return Read[S]{}, false
	}
	index := uint64(len(shared.reads))
	packed := slot.cell.ordinal
	if packed>>32 != shared.ordinal || uint64(uint32(packed)) != index {
		return Read[S]{}, false
	}
	shape, shapeOK := shared.state.schema.ruleReadShapeAt(shared.ordinal, index)
	factorOrdinal, factorOK := factorRefOrdinal(factor, shared.state.schema)
	formFactor, formOrdinal := form.cell.ordinal>>32, uint64(uint32(form.cell.ordinal))
	if !shapeOK || !factorOK || shape.Kind != composition.ReadSummary || shape.DependencyCount != 0 ||
		!shape.Semantic.Available() || shape.Semantic != shape.Normalizer || factorOrdinal >= uint64(len(shared.state.factors)) ||
		formFactor != factorOrdinal || shared.state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		return Read[S]{}, false
	}
	factorCell := shared.state.factors[factorOrdinal]
	if factorCell == nil {
		return Read[S]{}, false
	}
	formCell, formOK := factorCell.schemaFactorFormAt(formOrdinal).(schemaOpaqueSummaryRuleReadForm[RV, S])
	origin := &schemaRuleReadOrigin{state: shared.state, cell: shared.cell, ruleOrdinal: shared.ordinal, readOrdinal: index, input: shape.Input, factor: factorOrdinal, kind: composition.ReadSummary, semantic: shape.Semantic, formOrdinal: formOrdinal}
	read := Read[S]{origin: origin, index: int(index), resolve: resolveTypedRead[RV, S]}
	if !formOK || !formCell.schemaSummaryRuleReadComplete(shared.state, origin) {
		return Read[S]{}, false
	}
	shared.reads = append(shared.reads, &schemaOpaqueSummaryRuleReadBinding[RV, S]{origin: origin, form: formCell, read: read, admit: admit})
	return read, true
}

func AddSelectedRouteRead[K ~uint32 | ~uint64, V, O any, RV any, Tag selectionTag](tx *SelectedRouteRuleBindingTransaction[K, V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext) bool) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	if tx == nil || tx.shared == nil || tx.shared.state == nil || locate == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shared := tx.shared
	shared.state.mu.Lock()
	defer shared.state.mu.Unlock()
	if shared.committed || shared.aborted || shared.token == nil || shared.state.pendingRules[shared.ordinal] != shared.token || shared.cell == nil || slot.cell == nil || slot.cell.schema != shared.state.schema || factor.cell == nil || factor.cell.schema != shared.state.schema {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	index := uint64(len(shared.reads))
	packed := slot.cell.ordinal
	if packed>>32 != shared.ordinal || uint64(uint32(packed)) != index {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shape, ok := shared.state.schema.ruleReadShapeAt(shared.ordinal, index)
	factorOrdinal, factorOK := factorRefOrdinal(factor, shared.state.schema)
	if !ok || !factorOK || shape.Kind != composition.ReadSelect || shape.DependencyCount == 0 || !validReadDependencies(shared.state.schema, shared.ordinal, index, shape.DependencyCount) || factorOrdinal >= uint64(len(shared.state.factors)) || shared.state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	factorCell := shared.state.factors[factorOrdinal]
	if factorCell == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	origin := &schemaRuleReadOrigin{state: shared.state, cell: shared.cell, ruleOrdinal: shared.ordinal, readOrdinal: index, input: shape.Input, factor: factorOrdinal, kind: composition.ReadSelect, dependencyCount: shape.DependencyCount}
	read := Read[Selection[Tag, OrderedCells[RV]]]{origin: origin, index: int(index), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	if !factorCell.schemaFactorReadComplete(shared.state, origin) {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shared.reads = append(shared.reads, &schemaOpaqueSelectedRuleReadBinding[RV, Tag]{origin: origin, factor: factorCell, read: read, locate: locate})
	return read, true
}

// AddSelectedRouteOperandRead is the operand-aware selected lane. The
// operand is supplied only later by the canonical boundRule during graph
// attachment; this transaction stores the typed locator, never an operand
// snapshot or an erased callback.
func AddSelectedRouteOperandRead[K ~uint32 | ~uint64, V, O any, RV any, Tag selectionTag](tx *SelectedRouteRuleBindingTransaction[K, V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], locate func(SelectorContext, O) bool) (Read[Selection[Tag, OrderedCells[RV]]], bool) {
	if tx == nil || tx.shared == nil || tx.shared.state == nil || locate == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shared := tx.shared
	shared.state.mu.Lock()
	defer shared.state.mu.Unlock()
	if shared.committed || shared.aborted || shared.token == nil || shared.state.pendingRules[shared.ordinal] != shared.token || shared.cell == nil || slot.cell == nil || slot.cell.schema != shared.state.schema || factor.cell == nil || factor.cell.schema != shared.state.schema {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	index := uint64(len(shared.reads))
	packed := slot.cell.ordinal
	if packed>>32 != shared.ordinal || uint64(uint32(packed)) != index {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shape, ok := shared.state.schema.ruleReadShapeAt(shared.ordinal, index)
	factorOrdinal, factorOK := factorRefOrdinal(factor, shared.state.schema)
	if !ok || !factorOK || shape.Kind != composition.ReadSelect || shape.DependencyCount == 0 || !validReadDependencies(shared.state.schema, shared.ordinal, index, shape.DependencyCount) || factorOrdinal >= uint64(len(shared.state.factors)) || shared.state.schema.factorSemanticAt(factorOrdinal) != shape.Factor {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	factorCell := shared.state.factors[factorOrdinal]
	if factorCell == nil {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	origin := &schemaRuleReadOrigin{state: shared.state, cell: shared.cell, ruleOrdinal: shared.ordinal, readOrdinal: index, input: shape.Input, factor: factorOrdinal, kind: composition.ReadSelect, dependencyCount: shape.DependencyCount}
	read := Read[Selection[Tag, OrderedCells[RV]]]{origin: origin, index: int(index), resolve: resolveTypedSelection[RV, OrderedCells[RV], Tag]}
	if !factorCell.schemaFactorReadComplete(shared.state, origin) {
		return Read[Selection[Tag, OrderedCells[RV]]]{}, false
	}
	shared.reads = append(shared.reads, &schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]{origin: origin, factor: factorCell, read: read, locateOperand: locate})
	return read, true
}

// commitSelectedRouteRuleBinding atomically publishes the one completed Rule
// cell after every exact/selected receipt has arrived in canonical ordinal
// order.
func commitSelectedRouteRuleBinding[K ~uint32 | ~uint64, V, O any](tx *SelectedRouteRuleBindingTransaction[K, V, O]) bool {
	if tx == nil || tx.shared == nil || tx.shared.state == nil {
		return false
	}
	shared := tx.shared
	shared.state.mu.Lock()
	defer shared.state.mu.Unlock()
	if shared.committed || shared.aborted || shared.token == nil || shared.state.pendingRules[shared.ordinal] != shared.token || shared.cell == nil || shared.state.phase != schemaBindingOpen || shared.state.rules[shared.ordinal] != nil {
		return false
	}
	shape, shapeOK := shared.state.schema.ruleShapeAt(shared.ordinal)
	if !shapeOK || uint64(len(shared.reads)) != shape.ReadCount {
		delete(shared.state.pendingRules, shared.ordinal)
		shared.aborted = true
		shared.state.poisonLocked()
		return false
	}
	shared.cell.impl = &ruleHotImplementation[K, V, O]{state: shared.state, rule: shared.rule, write: shared.write, output: shared.output, carry: shared.carry, reads: shared.reads, operandContent: shared.operand, admission: shared.admission, transfer: shared.transfer, projectWrite: shared.projectWrite}
	if !shared.cell.schemaRuleComplete() {
		delete(shared.state.pendingRules, shared.ordinal)
		shared.aborted = true
		shared.state.poisonLocked()
		return false
	}
	shared.state.rules[shared.ordinal] = shared.cell
	delete(shared.state.pendingRules, shared.ordinal)
	shared.committed = true
	return true
}

// abortSelectedRouteRuleBinding terminally poisons the shared Binding and
// releases the ordinal reservation. There is intentionally no reusable
// rollback path: a failed hot transaction cannot be replaced by a second
// closure while other cells may still hold its receipts.
func abortSelectedRouteRuleBinding[K ~uint32 | ~uint64, V, O any](tx *SelectedRouteRuleBindingTransaction[K, V, O]) bool {
	if tx == nil || tx.shared == nil || tx.shared.state == nil {
		return false
	}
	shared := tx.shared
	shared.state.mu.Lock()
	defer shared.state.mu.Unlock()
	if shared.committed || shared.aborted || shared.token == nil || shared.state.pendingRules[shared.ordinal] != shared.token || shared.state.phase != schemaBindingOpen {
		return false
	}
	delete(shared.state.pendingRules, shared.ordinal)
	shared.aborted = true
	shared.state.poisonLocked()
	return true
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
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || readSlot.cell == nil || readSlot.cell.schema != state.schema || readFactor.cell == nil || readFactor.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
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
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
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
	if !outputTyped || outputCell == nil || readCell == nil || outputCell.impl == nil || readCell.schemaFactorSchema() != state.schema || outputCell.impl.algebra == nil || !readCell.schemaFactorReadComplete(state, &schemaRuleReadOrigin{state: state, ruleOrdinal: ruleOrdinal, readOrdinal: 0, input: readShape.Input, factor: readFactorOrdinal, kind: composition.ReadExact}) || outputCell.impl.state != state {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	cell := &schemaRuleBindingCellImpl[OK, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	origin := &schemaRuleReadOrigin{state: state, cell: cell, ruleOrdinal: ruleOrdinal, readOrdinal: 0, input: readShape.Input, factor: readFactorOrdinal, kind: composition.ReadExact}
	read := Read[OrderedCells[RV]]{origin: origin, index: 0, resolve: resolveTypedRead[RV, OrderedCells[RV]]}
	readBinding := &schemaOpaqueExactRuleReadBinding[RV]{origin: origin, factor: readCell, read: read, projector: projectExactLocal(projectRead)}
	cell.impl = &ruleHotImplementation[OK, V, O]{state: state, rule: slot, write: write, output: outputCell, reads: []schemaRuleReadBinding{readBinding}, operandContent: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, projectWrite: projectWrite}
	if !cell.schemaRuleComplete() {
		state.poisonLocked()
		return Read[OrderedCells[RV]]{}, false
	}
	state.rules[ruleOrdinal] = cell
	return read, true
}

// beginSelectedExactRuleBinding starts the no-carry heterogeneous selected
// read transaction for one exact-write Rule. It shares the one-shot read and
// commit authority with the routed/carry transaction, but requires the cold
// Rule to prove CarryCount=0. There is no optional-carry mode: the two shapes
// have disjoint constructors and validation laws.
func beginSelectedExactRuleBinding[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], projectWrite func(O) (uint64, bool)) (*SelectedRouteRuleBindingTransaction[K, V, O], bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || write.cell == nil || write.cell.schema != state.schema || output.cell == nil || output.cell.schema != state.schema || spec.OperandContent == nil || !spec.Admission.valid() || spec.Transfer == nil {
		state.poisonLocked()
		return nil, false
	}
	ruleOrdinal, ruleOK := slot.Ordinal()
	if !ruleOK || ruleOrdinal >= uint64(len(state.rules)) || state.rules[ruleOrdinal] != nil || state.pendingRules[ruleOrdinal] != nil || write.cell.ordinal>>32 != ruleOrdinal || uint64(uint32(write.cell.ordinal)) != 0 {
		state.poisonLocked()
		return nil, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ruleOrdinal, 0)
	if !shapeOK || !writeOK || shape.OutputKind != composition.FactorOutput || shape.Inputs == 0 || shape.CarryCount != 0 || shape.WriteCount != 1 || writeShape.Factor != shape.Output || writeShape.Kind != composition.WriteExact || writeShape.Route != 0 {
		state.poisonLocked()
		return nil, false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || spec.Admission.kind != coldAdmission.kind || spec.Admission.identity != coldAdmission.identity {
		state.poisonLocked()
		return nil, false
	}
	outputOrdinal, outputOK := factorRefOrdinal(output, state.schema)
	if !outputOK || outputOrdinal >= uint64(len(state.factors)) || state.schema.factorSemanticAt(outputOrdinal) != shape.Output {
		state.poisonLocked()
		return nil, false
	}
	outputCell, outputOK := state.factors[outputOrdinal].(*schemaFactorBindingCell[K, V])
	if !outputOK || outputCell == nil || outputCell.schema != state.schema || outputCell.impl == nil || outputCell.impl.algebra == nil {
		state.poisonLocked()
		return nil, false
	}
	cell := &schemaRuleBindingCellImpl[K, V, O]{state: state, schema: state.schema, ordinal: ruleOrdinal}
	token := &schemaRuleBindingToken{}
	if state.pendingRules == nil {
		state.pendingRules = make(map[uint64]*schemaRuleBindingToken)
	}
	state.pendingRules[ruleOrdinal] = token
	return &SelectedRouteRuleBindingTransaction[K, V, O]{shared: &selectedRouteTxnState[K, V, O]{
		state: state, cell: cell, output: outputCell, ordinal: ruleOrdinal, rule: slot, write: write,
		operand: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, projectWrite: projectWrite, token: token,
	}}, true
}
