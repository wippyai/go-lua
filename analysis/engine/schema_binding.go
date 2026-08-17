package engine

// This file is the isolated Factor vertical of the callback-free Schema API.
// It is deliberately not wired into a declaration execution path. Schema
// remains the sole owner of cold structural identity; this file only admits
// Link-local typed Factor algebra against exact Factor slots.

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type schemaBindingPhase uint8

const (
	schemaBindingOpen schemaBindingPhase = iota + 1
	schemaBindingSealed
	schemaBindingPoisoned
)

// Keep this non-zero-sized. Go permits pointers to distinct zero-sized
// allocations to compare equal, which would collapse independent Binding
// receipt authorities.
type schemaBindingAuthority struct{ marker byte }

// HotFactorSpec is the Link-local portion of FactorSpec. Semantic is absent:
// the exact FactorSlot already carries the cold proof.
type HotFactorSpec[K ~uint32 | ~uint64, V any] struct {
	KeyEnd      uint64
	Lattice     lattice.Lattice[V]
	Default     V
	AdmitAt     func(K, V) bool
	Fingerprint func(V) uint64
	WidenRank   Measure[K, V]
	NarrowRank  Measure[K, V]
}

// SchemaBinding is a copyable handle to one shared lifecycle. It cannot
// publish until every Factor, Rule, Query, and activation-family cell is
// complete and fenced to this exact Schema.
type SchemaBinding struct{ state *schemaBindingState }

type schemaBindingState struct {
	mu         sync.Mutex
	schema     *Schema
	phase      schemaBindingPhase
	authority  *schemaBindingAuthority
	factors    []schemaFactorBinding
	rules      []schemaBindingCell
	queries    []schemaBindingCell
	activation []schemaBindingCell
	roleSlots  map[RuleSlotCapability]composition.Key
	// linkBootstrapTransports is the sole ordered transport authorization for
	// the Link-global bootstrap seam. The engine retains opaque capabilities,
	// never domain role names; the program owner registers the exact pair once
	// before this Binding seals.
	linkBootstrapTransports    [2]RuleSlotCapability
	linkBootstrapTransportPair bool
	// pendingRules reserves a canonical Rule ordinal while a hot transaction
	// assembles its heterogeneous read receipts.  The reservation is shared
	// by copied transaction handles and prevents two closures racing to publish
	// different implementations for one cold Rule cell.
	pendingRules map[uint64]*schemaRuleBindingToken
}

type schemaBindingCell interface{ schemaBindingSchema() *Schema }

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
	state          *schemaBindingState
	rule           *RuleSlot[V, O]
	write          SchemaWriteSlot[V]
	output         *schemaFactorBindingCell[K, V]
	carry          *schemaRuleCarryBinding[K, V, O]
	reads          []schemaRuleReadBinding
	operandContent func(O) (O, [32]byte, bool)
	admission      RuleAdmission[V, O]
	transfer       func(Access[V, O]) bool
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
}

// schemaSelectedReadSelector is the SchemaBinding-owned selector geometry
// consumed by stagedReadRuntime. It re-reads predecessor membership from the
// immutable Schema and carries the exact Rule fence; it never stores a cold
// dependency slice or a copied Rule row.
type schemaSelectedReadSelector struct {
	fence schemaRuleReceiptFence
	read  uint64
}

func (selector *schemaSelectedReadSelector) selectorReadIndex() int {
	if selector == nil || selector.read > uint64(^uint(0)>>1) {
		return -1
	}
	return int(selector.read)
}

func (selector *schemaSelectedReadSelector) selectorDeclaresRead(index int) bool {
	if selector == nil || index < 0 {
		return false
	}
	for dependency := uint64(0); ; dependency++ {
		value, ok := selector.fence.schema.ruleReadDependencyAt(selector.fence.rule, selector.read, dependency)
		if !ok {
			return false
		}
		if value == uint64(index) {
			return true
		}
	}
}

func (selector *schemaSelectedReadSelector) selectorDependencyCount() int {
	if selector == nil || !selector.fence.valid() {
		return 0
	}
	shape, ok := selector.fence.schema.ruleReadShapeAt(selector.fence.rule, selector.read)
	if !ok || shape.DependencyCount > uint64(^uint(0)>>1) {
		return 0
	}
	return int(shape.DependencyCount)
}

type schemaSelectedRuleReadBinding[K ~uint32 | ~uint64, V any, Tag selectionTag] struct {
	origin *schemaRuleReadOrigin
	factor *schemaFactorBindingCell[K, V]
	read   Read[Selection[Tag, OrderedCells[V]]]
	locate func(SelectorContext) bool
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
	state     *schemaBindingState
	cell      *schemaRuleBindingCellImpl[K, V, O]
	output    *schemaFactorBindingCell[K, V]
	ordinal   uint64
	rule      *RuleSlot[V, O]
	write     SchemaWriteSlot[V]
	carry     *schemaRuleCarryBinding[K, V, O]
	reads     []schemaRuleReadBinding
	operand   func(O) (O, [32]byte, bool)
	admission RuleAdmission[V, O]
	transfer  func(Access[V, O]) bool
	token     *schemaRuleBindingToken
	committed bool
	aborted   bool
}

func factorRefOrdinal[V any](ref FactorRef[V], schema *Schema) (uint64, bool) {
	if schema == nil || ref.cell == nil || ref.cell.schema != schema || !schema.Available() {
		return 0, false
	}
	return ref.cell.ordinal, true
}

// BindSelectedRouteRule owns one route Rule binding transaction: it opens the
// transaction, hands it to bind for read attachment, and terminalizes on
// bind's answer. A rejected bind or a rejected commit aborts, so the shared
// Binding always reaches its terminal state and no caller pairs commit with
// abort by hand.
func BindSelectedRouteRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	tx, opened := beginSelectedRouteRuleBinding[K](binding, slot, carry, write, output, spec, carrySpec)
	return terminalizeSelectedRouteRuleBinding(tx, opened, bind)
}

// BindSelectedRule is BindSelectedRouteRule's non-routed sibling: the Rule's
// output write is exact rather than routed.
func BindSelectedRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O], bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	tx, opened := beginSelectedRuleBinding[K](binding, slot, carry, write, output, spec, carrySpec)
	return terminalizeSelectedRouteRuleBinding(tx, opened, bind)
}

// BindSelectedExactRule is the carry-free sibling for Rules whose output write
// is exact and whose cold geometry declares no carry.
func BindSelectedExactRule[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], bind func(*SelectedRouteRuleBindingTransaction[K, V, O]) bool) bool {
	tx, opened := beginSelectedExactRuleBinding[K](binding, slot, write, output, spec)
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
func beginSelectedRouteRuleBinding[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O]) (*SelectedRouteRuleBindingTransaction[K, V, O], bool) {
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
	return &SelectedRouteRuleBindingTransaction[K, V, O]{shared: &selectedRouteTxnState[K, V, O]{state: state, cell: cell, output: outputCell, ordinal: ruleOrdinal, rule: slot, write: write, carry: &schemaRuleCarryBinding[K, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply}, operand: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, token: token}}, true
}

// beginSelectedRuleBinding starts the non-routed selected-read transaction.
// It is the sibling of beginSelectedRouteRuleBinding for Rules whose output
// write is exact.  Keeping the transaction type shared is intentional: the
// sealed cell, heterogeneous read receipts, and atomic commit path are the
// same; only the cold write geometry differs.
func beginSelectedRuleBinding[K ~uint32 | ~uint64, V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], carry SchemaCarrySlot[V], write SchemaWriteSlot[V], output FactorRef[V], spec HotRuleSpec[V, O], carrySpec HotCarrySpec[V, O]) (*SelectedRouteRuleBindingTransaction[K, V, O], bool) {
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
	return &SelectedRouteRuleBindingTransaction[K, V, O]{shared: &selectedRouteTxnState[K, V, O]{state: state, cell: cell, output: outputCell, ordinal: ruleOrdinal, rule: slot, write: write, carry: &schemaRuleCarryBinding[K, V, O]{state: state, cell: cell, ordinal: ruleOrdinal, slot: carry, factor: outputCell, apply: carrySpec.Apply}, operand: spec.OperandContent, admission: spec.Admission, transfer: spec.Transfer, token: token}}, true
}

func AddSelectedRouteExactRead[K ~uint32 | ~uint64, V, O any, RV any](tx *SelectedRouteRuleBindingTransaction[K, V, O], slot SchemaReadSlot[RV], factor FactorRef[RV]) (Read[OrderedCells[RV]], bool) {
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
	shared.reads = append(shared.reads, &schemaOpaqueExactRuleReadBinding[RV]{origin: origin, factor: factorCell, read: read})
	return read, true
}

// AddSelectedRouteSummaryRead appends one summary predecessor while keeping
// the source Factor's coordinate type inside its exact owner cell. The shared
// Rule transaction retains only a typed value/summary receipt and cannot
// guess or expose the sibling Factor's K instantiation.
func AddSelectedRouteSummaryRead[K ~uint32 | ~uint64, V, O, RV, S any](tx *SelectedRouteRuleBindingTransaction[K, V, O], slot SchemaReadSlot[RV], factor FactorRef[RV], form SchemaReadForm[RV]) (Read[S], bool) {
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
	shared.reads = append(shared.reads, &schemaOpaqueSummaryRuleReadBinding[RV, S]{origin: origin, form: formCell, read: read})
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
	shared.cell.impl = &ruleHotImplementation[K, V, O]{state: shared.state, rule: shared.rule, write: shared.write, output: shared.output, carry: shared.carry, reads: shared.reads, operandContent: shared.operand, admission: shared.admission, transfer: shared.transfer}
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

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.locate == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.origin.kind != composition.ReadSelect || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || binding.factor.impl == nil || binding.factor.impl.algebra == nil || binding.factor.impl.state != state {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSelect && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == shape.Factor && shape.Normalizer == (composition.Key{}) && shape.DependencyCount == binding.origin.dependencyCount && binding.origin.dependencyCount != 0
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.origin.matches(proof, binding.origin.readOrdinal) || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	selectedRead := proof.selectedReadAt(binding.origin.readOrdinal)
	if selectedRead == nil || !selectedRead.Valid() {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	target, staged := runtime.(stagedFactor[V])
	if !present || !typed || !staged || factor == nil || !factor.receipt.valid() || factor.receipt.state != binding.origin.state || factor.receipt.authority != binding.origin.state.authority || factor.receipt.ordinal != binding.origin.factor || factor.receipt.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSelect || row.Input != binding.origin.input || row.Factor != factorKey || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorKey || surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	selector := &schemaSelectedReadSelector{fence: selectedRead.fence, read: binding.origin.readOrdinal}
	if !selector.fence.valid() || selector.selectorDependencyCount() != int(binding.origin.dependencyCount) || selector.selectorDependencyCount() == 0 {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	locate := binding.locate
	return bound.appendReadRuntime(&stagedReadRuntime[V, OrderedCells[V], Tag]{input: int(binding.origin.input), selector: selector, target: target, locate: locate, normalize: normalize})
}

type schemaExactRuleReadBinding[K ~uint32 | ~uint64, V any] struct {
	origin *schemaRuleReadOrigin
	factor *schemaFactorBindingCell[K, V]
	read   Read[OrderedCells[V]]
}

// These heterogeneous variants retain the typed Read while delegating all
// carrier-coordinate work to the exact Factor cell issuer.
type schemaOpaqueExactRuleReadBinding[V any] struct {
	origin *schemaRuleReadOrigin
	factor schemaFactorBinding
	read   Read[OrderedCells[V]]
}

type schemaOpaqueSelectedRuleReadBinding[V any, Tag selectionTag] struct {
	origin *schemaRuleReadOrigin
	factor schemaFactorBinding
	read   Read[Selection[Tag, OrderedCells[V]]]
	locate func(SelectorContext) bool
}

type schemaOpaqueSummaryRuleReadBinding[V, S any] struct {
	origin *schemaRuleReadOrigin
	form   schemaOpaqueSummaryRuleReadForm[V, S]
	read   Read[S]
}

type schemaOpaqueOperandSelectedRuleReadBinding[RV, O any, Tag selectionTag] struct {
	origin        *schemaRuleReadOrigin
	factor        schemaFactorBinding
	read          Read[Selection[Tag, OrderedCells[RV]]]
	locateOperand func(SelectorContext, O) bool
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || !binding.factor.schemaFactorReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadExact && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.DependencyCount == 0
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || bound == nil || factors == nil {
		return false
	}
	if _, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof }); !ok {
		return false
	}
	return binding.factor.schemaFactorBindExactRead(bound, member, factors, binding.origin)
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.form == nil || state == nil || cell == nil ||
		binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal ||
		binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) ||
		binding.read.resolve == nil || !binding.form.schemaSummaryRuleReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSummary && shape.Input == binding.origin.input &&
		shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == binding.origin.semantic &&
		shape.Normalizer == binding.origin.semantic && shape.DependencyCount == 0
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || binding.form == nil || bound == nil || factors == nil ||
		!binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	return binding.form.schemaSummaryRuleReadBind(bound, member, factors, binding.origin)
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.locate == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || !binding.factor.schemaFactorReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSelect && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == shape.Factor && shape.DependencyCount == binding.origin.dependencyCount && binding.origin.dependencyCount != 0
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || bound == nil || factors == nil || binding.locate == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) || !binding.origin.matches(proof, binding.origin.readOrdinal) {
		return false
	}
	selectedRead := proof.selectedReadAt(binding.origin.readOrdinal)
	if selectedRead == nil || !selectedRead.Valid() {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	targetProvider, targetOK := runtime.(stagedTargetProvider[V])
	if !present || !targetOK || targetProvider.stagedFactorTarget() == nil {
		return false
	}
	selector := &schemaSelectedReadSelector{fence: selectedRead.fence, read: binding.origin.readOrdinal}
	if !selector.fence.valid() || selector.selectorDependencyCount() != int(binding.origin.dependencyCount) || selector.selectorDependencyCount() == 0 {
		return false
	}
	return bound.appendReadRuntime(&stagedReadRuntime[V, OrderedCells[V], Tag]{input: int(binding.origin.input), selector: selector, target: targetProvider.stagedFactorTarget(), locate: binding.locate, normalize: func(value OrderedCells[V]) OrderedCells[V] { return value }})
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.locateOperand == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || !binding.factor.schemaFactorReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSelect && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == shape.Factor && shape.DependencyCount == binding.origin.dependencyCount && binding.origin.dependencyCount != 0
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || bound == nil || factors == nil || binding.locateOperand == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	operandOwner, operandOK := bound.(interface{ ruleOperand() O })
	if !ok || !operandOK {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) || !binding.origin.matches(proof, binding.origin.readOrdinal) {
		return false
	}
	selectedRead := proof.selectedReadAt(binding.origin.readOrdinal)
	if selectedRead == nil || !selectedRead.Valid() {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	targetProvider, targetOK := runtime.(stagedTargetProvider[RV])
	if !present || !targetOK || targetProvider.stagedFactorTarget() == nil || !targetProvider.stagedFactorReceiptMatches(binding.origin) {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(binding.origin.readOrdinal))
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	factorSemantic := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSelect || row.Input != binding.origin.input || row.Factor != factorSemantic || row.Semantic != factorSemantic || row.Normalizer.Available() || row.DependencyCount != binding.origin.dependencyCount || surface.Factor != factorSemantic || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorSemantic || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	selector := &schemaSelectedReadSelector{fence: selectedRead.fence, read: binding.origin.readOrdinal}
	if !selector.fence.valid() || selector.selectorDependencyCount() != int(binding.origin.dependencyCount) || selector.selectorDependencyCount() == 0 {
		return false
	}
	operand := operandOwner.ruleOperand()
	locate := func(context SelectorContext) bool { return binding.locateOperand(context, operand) }
	return bound.appendReadRuntime(&stagedReadRuntime[RV, OrderedCells[RV], Tag]{input: int(binding.origin.input), selector: selector, target: targetProvider.stagedFactorTarget(), locate: locate, normalize: func(value OrderedCells[RV]) OrderedCells[RV] { return value }})
}

type schemaSummaryRuleReadBinding[K ~uint32 | ~uint64, V, S any] struct {
	origin *schemaRuleReadOrigin
	factor *schemaFactorBindingCell[K, V]
	form   *schemaSummaryReadCell[K, V, S]
	read   Read[S]
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.form == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.origin.kind != composition.ReadSummary || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || binding.factor.impl == nil || binding.factor.impl.algebra == nil || binding.factor.impl.state != state || binding.factor.ordinal != binding.origin.factor || binding.form.schema != state.schema || binding.form.factor != binding.factor || binding.form.form.cell == nil || binding.form.form.cell.schema != state.schema || !summaryReadFormKind(binding.form.form.cell.kind) || !binding.form.schemaFactorFormComplete() {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSummary && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == binding.origin.semantic && shape.Normalizer == binding.origin.semantic && shape.DependencyCount == 0
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.origin.matches(proof, binding.origin.readOrdinal) || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || !factor.receipt.valid() || factor.receipt.state != binding.origin.state || factor.receipt.authority != binding.origin.state.authority || factor.receipt.ordinal != binding.origin.factor || factor.receipt.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSummary || row.Input != binding.origin.input || row.Factor != factorKey || row.Semantic != binding.origin.semantic || row.Normalizer != binding.origin.semantic || row.DependencyCount != 0 || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSummary || surface.Semantic != binding.origin.semantic || surface.Normalizer != binding.origin.semantic || surface.Mode != equation.TargetModeNone {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	unitRow, rowPresent := factor.reads[surface]
	proofSummary, proofOK := factor.summaryReadReceiptProof(surface, uint64(uint32(binding.form.ordinal)), binding.origin.semantic)
	if !unitOK || !rowPresent || unitRow.kind != carrier.SummaryUnit || !proofOK {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: int(binding.origin.input), binding: factor.binding, unit: unit, summary: proofSummary, normalize: binding.form.normalize, equal: binding.form.equal, fingerprint: binding.form.fingerprint})
}

func (binding *schemaExactRuleReadBinding[K, V]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || binding.factor.impl == nil || binding.factor.impl.algebra == nil || binding.factor.impl.state != state || binding.factor.ordinal != binding.origin.factor {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadExact && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.DependencyCount == 0
}

func (binding *schemaExactRuleReadBinding[K, V]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.origin.matches(proof, binding.origin.readOrdinal) || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || !factor.receipt.valid() || factor.receipt.state != binding.origin.state || factor.receipt.authority != binding.origin.state.authority || factor.receipt.ordinal != binding.origin.factor || factor.receipt.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadExact || row.Input != binding.origin.input || row.Factor != factorKey || row.DependencyCount != 0 || surface.Factor != factorKey || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	readProof, proofOK := newRuleReadReceiptProof(factor.receipt, surface)
	if !unitOK || !proofOK {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, binding.factor.impl.algebra.Equal)
	}
	fingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, binding.factor.impl.algebra.Fingerprint)
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, OrderedCells[V]]{input: int(binding.origin.input), binding: factor.binding, unit: unit, proof: readProof, normalize: normalize, equal: equal, fingerprint: fingerprint})
}

// ruleRuntimeReceipt is an opaque, cell-issued capability. It binds one exact
// hot implementation and output Factor receipt to the already-issued cold
// Rule proof. A state+ordinal pair is deliberately insufficient.
type ruleRuntimeReceipt[K ~uint32 | ~uint64, V, O any] struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	cell      *schemaRuleBindingCellImpl[K, V, O]
	proof     *ruleRuntimeProof
	output    factorRuntimeReceipt
	issued    bool
}

func (receipt ruleRuntimeReceipt[K, V, O]) valid() bool {
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.cell == nil || receipt.proof == nil || !receipt.proof.valid() || !receipt.output.valid() || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.state.schema == nil || receipt.cell.state != receipt.state || receipt.cell.schema != receipt.state.schema || receipt.cell.impl == nil || receipt.cell.impl.state != receipt.state || receipt.cell.ordinal != receipt.proof.ordinal || receipt.proof.state != receipt.state || receipt.proof.bindingAuthority != receipt.authority || receipt.output.state != receipt.state || receipt.output.authority != receipt.authority || receipt.output.schema != receipt.state.schema || receipt.proof.output != receipt.output.semantic {
		return false
	}
	if receipt.proof.ordinal >= uint64(len(receipt.state.rules)) || receipt.state.rules[receipt.proof.ordinal] != receipt.cell {
		return false
	}
	return receipt.cell.schemaRuleComplete() && receipt.cell.schemaRuleProofMatches(receipt.proof)
}

// RuleImplementation is the typed public handle for the one currently
// supported receipt lane. It intentionally contains only an opaque receipt;
// callbacks and structural draft handles remain cell-owned.
type RuleImplementation[K ~uint32 | ~uint64, V, O any] struct {
	receipt ruleRuntimeReceipt[K, V, O]
}

type schemaRuleBindingCell interface {
	schemaBindingCell
	schemaRuleOrdinal() uint64
	schemaRuleComplete() bool
	schemaRuleProofMatches(*ruleRuntimeProof) bool
}

func NewSchemaBinding(schema *Schema) *SchemaBinding {
	if schema == nil || !schema.Available() {
		return nil
	}
	factors, rules, queries, activations, ok := schema.shapeCount()
	if !ok {
		return nil
	}
	return &SchemaBinding{state: &schemaBindingState{
		schema: schema, phase: schemaBindingOpen, authority: &schemaBindingAuthority{},
		factors:      make([]schemaFactorBinding, factors),
		rules:        make([]schemaBindingCell, rules),
		queries:      make([]schemaBindingCell, queries),
		activation:   make([]schemaBindingCell, activations),
		roleSlots:    make(map[RuleSlotCapability]composition.Key),
		pendingRules: make(map[uint64]*schemaRuleBindingToken),
	}}
}

func bindingState(binding *SchemaBinding) *schemaBindingState {
	if binding == nil {
		return nil
	}
	return binding.state
}

func (binding *SchemaBinding) Schema() *Schema {
	state := bindingState(binding)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil {
		return nil
	}
	return state.schema
}

func (binding *SchemaBinding) Sealed() bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == schemaBindingSealed && state.authority != nil
}

func (binding *SchemaBinding) Poisoned() bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == schemaBindingPoisoned
}

func (state *schemaBindingState) poisonLocked() {
	if state == nil || state.phase == schemaBindingSealed {
		return
	}
	state.phase = schemaBindingPoisoned
	state.factors = nil
	state.rules = nil
	state.queries = nil
	state.activation = nil
	state.authority = nil
}

// Seal validates the Factor vertical, receipt-native Rule/activation lanes,
// and the currently supported exact-Factor Query lane. Activation families
// are inventoried separately from their Rule cells and must be complete before
// one Binding authority is published.
func (binding *SchemaBinding) Seal() bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.schema == nil || state.authority == nil || len(state.activation) != int(state.schema.activationCount()) {
		state.poisonLocked()
		return false
	}
	if len(state.factors) != schemaFactorCount(state.schema) {
		state.poisonLocked()
		return false
	}
	for ordinal, cell := range state.factors {
		if cell == nil || cell.schemaFactorSchema() != state.schema || cell.schemaFactorOrdinal() != uint64(ordinal) || !cell.schemaFactorComplete() {
			state.poisonLocked()
			return false
		}
	}
	for ordinal, cell := range state.rules {
		rule, ok := cell.(schemaRuleBindingCell)
		if !ok || rule == nil || rule.schemaBindingSchema() != state.schema || rule.schemaRuleOrdinal() != uint64(ordinal) || !rule.schemaRuleComplete() {
			state.poisonLocked()
			return false
		}
	}
	for ordinal, cell := range state.activation {
		family, ok := cell.(*schemaActivationFamilyBindingCell)
		if !ok || family == nil || cell.schemaBindingSchema() != state.schema || !family.activationComplete(state.schema, uint64(ordinal)) {
			state.poisonLocked()
			return false
		}
	}
	for ordinal, cell := range state.queries {
		query, ok := cell.(schemaQueryBindingCell)
		if !ok || query == nil || cell.schemaBindingSchema() != state.schema || query.schemaQueryState() != state || query.schemaQueryOrdinal() != uint64(ordinal) || !query.complete() {
			state.poisonLocked()
			return false
		}
	}
	if len(state.roleSlots) != 0 {
		if !completeCapabilityDirectory(state) {
			state.poisonLocked()
			return false
		}
	}
	if state.linkBootstrapTransportPair && !completeLinkBootstrapTransportPairLocked(state) {
		state.poisonLocked()
		return false
	}
	state.phase = schemaBindingSealed
	return true
}

func completeLinkBootstrapTransportPairLocked(state *schemaBindingState) bool {
	if state == nil || state.schema == nil || state.authority == nil || !state.linkBootstrapTransportPair {
		return false
	}
	seenOutputs := make(map[composition.Key]struct{}, len(state.linkBootstrapTransports))
	for _, capability := range state.linkBootstrapTransports {
		semantic, registered := state.roleSlots[capability]
		shape, shapeOK := state.schema.ruleShapeAt(capability.ordinal)
		if !registered || !capability.link() || capability.state != state || capability.authority != state.authority || semantic != state.schema.ruleSemanticAt(capability.ordinal) || !shapeOK || shape.OutputKind != composition.FactorOutput || !shape.Output.Available() {
			return false
		}
		if _, duplicate := seenOutputs[shape.Output]; duplicate {
			return false
		}
		seenOutputs[shape.Output] = struct{}{}
	}
	return true
}

func schemaFactorCount(schema *Schema) int {
	if schema == nil {
		return 0
	}
	factors, _, _, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return factors
}

type schemaFactorBinding interface {
	boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool)
	schemaFactorOrdinal() uint64
	schemaFactorSchema() *Schema
	schemaFactorComplete() bool
	schemaFactorRuntimeBinding(*runtimeBinding) (runtimeFactor, bool)
	schemaFactorReadComplete(*schemaBindingState, *schemaRuleReadOrigin) bool
	schemaFactorBindExactRead(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor, *schemaRuleReadOrigin) bool
	schemaFactorFormAt(uint64) schemaFactorFormBinding
}

type FactorImplementation[K ~uint32 | ~uint64, V any] struct {
	state      *schemaBindingState
	algebra    *factbinding.Algebra[K, V]
	descriptor factorRuntimeDescriptor
	receipt    factorRuntimeReceipt
}

type factorFormReceipt struct {
	ordinal  uint64
	kind     SchemaFormKind
	semantic composition.Key
}

// factorRuntimeReceipt is the sealed, private Factor implementation proof
// consumed by the carrier binder. The state and authority pointers fence an
// equal-but-foreign SchemaBinding; the scalar rows are copied only as an
// immutable receipt, never as a second schema or Factor registry.
type factorRuntimeReceipt struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	ordinal   uint64
	semantic  composition.Key
	keyEnd    uint64
	algebra   anyFactorAlgebra
	forms     []factorFormReceipt
	issued    bool
}

func (receipt factorRuntimeReceipt) valid() bool {
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.state.authority != receipt.authority || receipt.state.phase != schemaBindingSealed || receipt.schema == nil || receipt.state.schema != receipt.schema || !receipt.schema.Available() || !receipt.semantic.Available() || receipt.algebra == nil || receipt.algebra.KeyEnd() != receipt.keyEnd {
		return false
	}
	if receipt.ordinal >= receipt.schema.factorCount() || receipt.ordinal >= uint64(len(receipt.state.factors)) || receipt.schema.factorSemanticAt(receipt.ordinal) != receipt.semantic {
		return false
	}
	cell, cellOK := receipt.state.factors[receipt.ordinal].(interface {
		schemaFactorAlgebra() anyFactorAlgebra
		schemaFactorBindingState() *schemaBindingState
	})
	if !cellOK || cell.schemaFactorBindingState() != receipt.state || cell.schemaFactorAlgebra() != receipt.algebra {
		return false
	}
	return true
}

func (receipt factorRuntimeReceipt) validForms() bool {
	if !receipt.valid() {
		return false
	}
	formCount, ok := receipt.schema.factorFormCount(receipt.ordinal)
	if !ok || len(receipt.forms) != formCount {
		return false
	}
	for index, form := range receipt.forms {
		shape, shapeOK := receipt.schema.factorFormShapeAt(receipt.ordinal, uint64(index))
		if !shapeOK || form.ordinal != uint64(index) || form.kind == SchemaFormInvalid {
			return false
		}
		want := composition.Key{}
		if summaryReadRowKind(shape.Kind) {
			want = shape.Semantic
		}
		if form.kind != schemaFormKind(shape.Kind) || form.semantic != want {
			return false
		}
	}
	return true
}

func (receipt factorRuntimeReceipt) formAt(ordinal uint64, kind SchemaFormKind, semantic composition.Key) (factorFormReceipt, bool) {
	if !receipt.valid() || ordinal >= uint64(len(receipt.forms)) {
		return factorFormReceipt{}, false
	}
	form := receipt.forms[ordinal]
	return form, form.ordinal == ordinal && form.kind == kind && form.semantic == semantic
}

func schemaFormKind(kind composition.FactorFormKind) SchemaFormKind {
	switch kind {
	case composition.FactorSummaryRead:
		return SchemaFormReadSummary
	case composition.FactorDistributiveSummaryRead:
		return SchemaFormReadDistributiveSummary
	default:
		return SchemaFormInvalid
	}
}

// factorRuntimeDescriptor is the narrow immutable Factor proof consumed by
// the carrier binder. It contains no declaration callback or
// copied cold row; all shape queries go through the exact Schema slot.
type factorRuntimeDescriptor struct {
	schema   *Schema
	binding  *SchemaBinding
	state    *schemaBindingState
	ordinal  uint64
	semantic composition.Key
	keyEnd   uint64
	algebra  anyFactorAlgebra
}

// schemaRuleReceiptFence is the shared owner proof for the isolated
// selected/selector/route Rule vertical. It carries identity only; all shape
// validation re-reads scalar projections from the exact Schema.
type schemaRuleReceiptFence struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	rule      uint64
	cell      schemaRuleBindingCell
}

func (fence schemaRuleReceiptFence) valid() bool {
	return fence.state != nil && fence.authority != nil && fence.state.authority == fence.authority && fence.state.phase == schemaBindingSealed && fence.state.schema == fence.schema && fence.schema != nil && fence.schema.Available() && fence.rule < fence.schema.ruleCount() && fence.rule < uint64(len(fence.state.rules)) && fence.cell != nil && fence.state.rules[fence.rule] == fence.cell && fence.cell.schemaBindingSchema() == fence.schema && fence.cell.schemaRuleOrdinal() == fence.rule
}

// SchemaSelectedReadReceipt is opaque selected-read geometry evidence. It is
// intentionally not a runtime callback or a copied Rule row.
type SchemaSelectedReadReceipt struct {
	fence           schemaRuleReceiptFence
	read            uint64
	factor          uint64
	dependencyCount uint64
	issued          bool
}

// SchemaRouteWriteReceipt is opaque route-write geometry evidence. Route is
// always tied to the one selected-read predecessor named by the Schema row.
type SchemaRouteWriteReceipt struct {
	fence  schemaRuleReceiptFence
	write  uint64
	read   uint64
	factor uint64
	issued bool
}

func (receipt SchemaSelectedReadReceipt) Valid() bool {
	if !receipt.issued || !receipt.fence.valid() {
		return false
	}
	rule, ruleOK := receipt.fence.schema.ruleShapeAt(receipt.fence.rule)
	shape, shapeOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, receipt.read)
	factor, factorOK := receipt.fence.schema.factorOrdinalOf(shape.Factor)
	return ruleOK && shapeOK && factorOK && receipt.read < rule.ReadCount && shape.Kind == composition.ReadSelect && shape.Semantic == shape.Factor && !shape.Normalizer.Available() && shape.DependencyCount != 0 && receipt.factor == factor && receipt.dependencyCount == shape.DependencyCount
}

func (receipt SchemaRouteWriteReceipt) Valid() bool {
	if !receipt.issued || !receipt.fence.valid() {
		return false
	}
	rule, ruleOK := receipt.fence.schema.ruleShapeAt(receipt.fence.rule)
	shape, shapeOK := receipt.fence.schema.ruleWriteShapeAt(receipt.fence.rule, receipt.write)
	read, readOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, receipt.read)
	factor, factorOK := receipt.fence.schema.factorOrdinalOf(shape.Factor)
	readFactor, readFactorOK := receipt.fence.schema.factorOrdinalOf(read.Factor)
	return ruleOK && shapeOK && readOK && factorOK && readFactorOK && receipt.write < rule.WriteCount && receipt.read < rule.ReadCount && rule.WriteCount == 1 && shape.Kind == composition.WriteRoute && shape.Route == receipt.read+1 && read.Kind == composition.ReadSelect && read.Semantic == read.Factor && !read.Normalizer.Available() && read.DependencyCount != 0 && receipt.factor == factor && factor == readFactor
}

func issueSchemaSelectedReadReceiptFence(fence schemaRuleReceiptFence, ok bool, read uint64) (SchemaSelectedReadReceipt, bool) {
	if !ok {
		return SchemaSelectedReadReceipt{}, false
	}
	shape, shapeOK := fence.schema.ruleReadShapeAt(fence.rule, read)
	if !shapeOK || shape.Kind != composition.ReadSelect || shape.Semantic != shape.Factor || shape.Normalizer.Available() || shape.DependencyCount == 0 {
		return SchemaSelectedReadReceipt{}, false
	}
	factor, factorOK := fence.schema.factorOrdinalOf(shape.Factor)
	if !factorOK || !validReadDependencies(fence.schema, fence.rule, read, shape.DependencyCount) {
		return SchemaSelectedReadReceipt{}, false
	}
	return SchemaSelectedReadReceipt{fence: fence, read: read, factor: factor, dependencyCount: shape.DependencyCount, issued: true}, true
}

func issueSchemaRouteWriteReceiptFence(fence schemaRuleReceiptFence, ok bool, write uint64) (SchemaRouteWriteReceipt, bool) {
	if !ok {
		return SchemaRouteWriteReceipt{}, false
	}
	shape, shapeOK := fence.schema.ruleWriteShapeAt(fence.rule, write)
	ruleShape, ruleOK := fence.schema.ruleShapeAt(fence.rule)
	if !shapeOK || !ruleOK || shape.Kind != composition.WriteRoute || shape.Route == 0 || ruleShape.WriteCount != 1 || shape.Route > ruleShape.ReadCount {
		return SchemaRouteWriteReceipt{}, false
	}
	read := shape.Route - 1
	readShape, readOK := fence.schema.ruleReadShapeAt(fence.rule, read)
	factor, factorOK := fence.schema.factorOrdinalOf(shape.Factor)
	readFactor, readFactorOK := fence.schema.factorOrdinalOf(readShape.Factor)
	if !readOK || !readFactorOK || !factorOK || readShape.Kind != composition.ReadSelect || readShape.Semantic != readShape.Factor || readShape.Normalizer.Available() || factor != readFactor || readShape.DependencyCount == 0 || !validReadDependencies(fence.schema, fence.rule, read, readShape.DependencyCount) {
		return SchemaRouteWriteReceipt{}, false
	}
	return SchemaRouteWriteReceipt{fence: fence, write: write, read: read, factor: factor, issued: true}, true
}

func validReadDependencies(schema *Schema, rule, read, count uint64) bool {
	var previous uint64
	for index := uint64(0); index < count; index++ {
		dependency, ok := schema.ruleReadDependencyAt(rule, read, index)
		if !ok || dependency >= read || index > 0 && dependency <= previous {
			return false
		}
		previous = dependency
		shape, shapeOK := schema.ruleReadShapeAt(rule, dependency)
		if !shapeOK || (shape.Kind != composition.ReadExact && shape.Kind != composition.ReadSelect) {
			return false
		}
		if shape.Kind == composition.ReadSelect && (shape.Semantic != shape.Factor || shape.Normalizer.Available() || shape.DependencyCount == 0 || !validReadDependencies(schema, rule, dependency, shape.DependencyCount)) {
			return false
		}
	}
	return true
}

type anyFactorAlgebra interface {
	KeyEnd() uint64
}

func (descriptor factorRuntimeDescriptor) valid() bool {
	return descriptor.schema != nil && descriptor.schema.Available() && descriptor.semantic.Available() && descriptor.algebra != nil && descriptor.algebra.KeyEnd() == descriptor.keyEnd && (descriptor.state == nil || descriptor.state.schema == descriptor.schema && descriptor.state.phase == schemaBindingSealed && descriptor.state.authority != nil)
}

type schemaFactorFormBinding interface {
	schemaBindingSchema() *Schema
	schemaFactorFormComplete() bool
}

type schemaFactorBindingCell[K ~uint32 | ~uint64, V any] struct {
	ordinal    uint64
	schema     *Schema
	impl       *FactorImplementation[K, V]
	exactRead  schemaFactorFormBinding
	exactWrite schemaFactorFormBinding
	forms      []schemaFactorFormBinding
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorAlgebra() anyFactorAlgebra {
	if cell == nil || cell.impl == nil {
		return nil
	}
	return cell.impl.algebra
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorBindingState() *schemaBindingState {
	if cell == nil || cell.impl == nil || cell.impl.state == nil {
		return nil
	}
	return cell.impl.state
}

func (cell *schemaFactorBindingCell[K, V]) boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if cell == nil || cell.impl == nil || cell.impl.state == nil {
		return nil, nil, composition.Key{}, false
	}
	state := cell.impl.state
	authority := state.authority
	if authority == nil || state.phase != schemaBindingSealed || state.schema != cell.schema || cell.ordinal >= uint64(len(state.factors)) || state.factors[cell.ordinal] != cell || cell.impl.algebra == nil {
		return nil, nil, composition.Key{}, false
	}
	semantic := state.schema.factorSemanticAt(cell.ordinal)
	if !semantic.Available() {
		return nil, nil, composition.Key{}, false
	}
	return state, authority, semantic, true
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorRuntimeBinding(runtime *runtimeBinding) (runtimeFactor, bool) {
	if cell == nil || cell.impl == nil || runtime == nil || runtime.mode != runtimeBindingReceipt || runtime.state == nil || runtime.authority == nil {
		return nil, false
	}
	implementation, ok := cell.sealedImplementation(runtime.state, runtime.authority)
	if !ok {
		return nil, false
	}
	return bindFactorFromGraph(implementation, runtime)
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorReadComplete(state *schemaBindingState, origin *schemaRuleReadOrigin) bool {
	return cell != nil && state != nil && origin != nil && cell.schema == state.schema && cell.impl != nil && cell.impl.state == state && cell.impl.algebra != nil && cell.ordinal == origin.factor
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorFormAt(index uint64) schemaFactorFormBinding {
	if cell == nil || index >= uint64(len(cell.forms)) {
		return nil
	}
	return cell.forms[index]
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorBindExactRead(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor, origin *schemaRuleReadOrigin) bool {
	if cell == nil || bound == nil || member.ReadCount() <= int(origin.readOrdinal) || !cell.schemaFactorReadComplete(origin.state, origin) || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !origin.matches(proof, origin.readOrdinal) {
		return false
	}
	factorKey := origin.state.schema.factorSemanticAt(origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || !factor.receipt.valid() || factor.receipt.state != origin.state || factor.receipt.authority != origin.state.authority || factor.receipt.ordinal != origin.factor || factor.receipt.algebra != cell.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(origin.readOrdinal))
	row, rowOK := origin.state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadExact || row.Input != origin.input || row.Factor != factorKey || row.DependencyCount != 0 || surface.Factor != factorKey || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	readProof, proofOK := newRuleReadReceiptProof(factor.receipt, surface)
	if !unitOK || !proofOK {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, cell.impl.algebra.Equal)
	}
	fingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, cell.impl.algebra.Fingerprint)
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, OrderedCells[V]]{input: int(origin.input), binding: factor.binding, unit: unit, proof: readProof, normalize: normalize, equal: equal, fingerprint: fingerprint})
}

func (cell *schemaFactorBindingCell[K, V]) sealedImplementation(state *schemaBindingState, authority *schemaBindingAuthority) (*FactorImplementation[K, V], bool) {
	if cell == nil || cell.impl == nil || state == nil || authority == nil || cell.impl.state != state || state.schema != cell.schema || state.authority != authority || state.phase != schemaBindingSealed || cell.ordinal >= uint64(len(state.factors)) || state.factors[cell.ordinal] != cell || cell.impl.algebra == nil {
		return nil, false
	}
	forms := make([]factorFormReceipt, len(cell.forms))
	for index := range forms {
		shape, shapeOK := state.schema.factorFormShapeAt(cell.ordinal, uint64(index))
		if !shapeOK {
			return nil, false
		}
		forms[index] = factorFormReceipt{ordinal: uint64(index), kind: schemaFormKind(shape.Kind), semantic: shape.Semantic}
	}
	receipt := factorRuntimeReceipt{state: state, authority: authority, schema: state.schema, ordinal: cell.ordinal, semantic: state.schema.factorSemanticAt(cell.ordinal), keyEnd: cell.impl.algebra.KeyEnd(), algebra: cell.impl.algebra, forms: forms, issued: true}
	if !receipt.validForms() {
		return nil, false
	}
	result := *cell.impl
	result.state = state
	result.receipt = receipt
	result.descriptor = factorRuntimeDescriptor{schema: state.schema, state: state, ordinal: cell.ordinal, semantic: receipt.semantic, keyEnd: receipt.keyEnd, algebra: receipt.algebra}
	return &result, true
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorComplete() bool {
	if cell == nil || cell.schema == nil || cell.impl == nil || cell.impl.state == nil || cell.exactRead == nil || cell.exactWrite == nil || !cell.exactRead.schemaFactorFormComplete() || !cell.exactWrite.schemaFactorFormComplete() {
		return false
	}
	count, ok := cell.schema.factorFormCount(cell.ordinal)
	if !ok || len(cell.forms) != count {
		return false
	}
	for _, form := range cell.forms {
		if form == nil || !form.schemaFactorFormComplete() || form.schemaBindingSchema() != cell.schema {
			return false
		}
	}
	return true
}

type schemaFactorFormCell[K ~uint32 | ~uint64, V any] struct {
	schema  *Schema
	ordinal uint64
	kind    SchemaFormKind
	factor  *schemaFactorBindingCell[K, V]
	algebra *factbinding.Algebra[K, V]
}

func (cell *schemaFactorFormCell[K, V]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaFactorFormCell[K, V]) schemaFactorFormComplete() bool {
	if cell == nil || cell.schema == nil || cell.factor == nil || cell.algebra == nil || cell.factor.schema != cell.schema || cell.factor.impl == nil || cell.factor.impl.state == nil {
		return false
	}
	switch cell.kind {
	case SchemaFormReadExact, SchemaFormWriteExact:
		return cell.ordinal == cell.factor.ordinal
	case SchemaFormReadSummary, SchemaFormReadDistributiveSummary:
		if cell.ordinal>>32 != cell.factor.ordinal {
			return false
		}
		shape, ok := cell.schema.factorFormShapeAt(cell.factor.ordinal, uint64(uint32(cell.ordinal)))
		rowKind, optional := factorFormRowKind(cell.kind)
		return ok && optional && shape.Kind == rowKind
	default:
		return false
	}
}

type schemaRuleBindingCellImpl[K ~uint32 | ~uint64, V, O any] struct {
	state   *schemaBindingState
	schema  *Schema
	ordinal uint64
	impl    *ruleHotImplementation[K, V, O]
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.state.schema != cell.schema || cell.impl == nil || cell.impl.state != cell.state || cell.impl.rule == nil || cell.impl.rule.cell == nil || cell.impl.rule.cell.schema != cell.schema || cell.impl.write.cell == nil || cell.impl.write.cell.schema != cell.schema || cell.impl.output == nil || cell.impl.output.schema != cell.schema || cell.impl.output.impl == nil || cell.impl.output.impl.state != cell.state {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || shape.OutputKind != composition.FactorOutput || shape.WriteCount != 1 || uint64(len(cell.impl.reads)) != shape.ReadCount || shape.CarryCount > 1 {
		return false
	}
	if shape.CarryCount == 0 {
		if cell.impl.carry != nil {
			return false
		}
	} else if !cell.impl.carry.complete(cell.state, cell, cell.ordinal, cell.impl.output) {
		return false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || !cell.impl.admission.valid() || cell.impl.admission.kind != coldAdmission.kind || cell.impl.admission.identity != coldAdmission.identity {
		return false
	}
	ruleOrdinal, ruleOK := cell.impl.rule.Ordinal()
	writeOrdinal := cell.impl.write.cell.ordinal
	if !ruleOK || ruleOrdinal != cell.ordinal || writeOrdinal>>32 != cell.ordinal || uint64(uint32(writeOrdinal)) != 0 {
		return false
	}
	write, writeOK := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
	if !writeOK || write.Factor != shape.Output || write.Kind != composition.WriteExact && write.Kind != composition.WriteRoute {
		return false
	}
	if write.Kind == composition.WriteExact && write.Route != 0 || write.Kind == composition.WriteRoute && (write.Route == 0 || write.Route > shape.ReadCount) {
		return false
	}
	if write.Kind == composition.WriteRoute {
		read, readOK := cell.schema.ruleReadShapeAt(cell.ordinal, write.Route-1)
		if !readOK || read.Kind != composition.ReadSelect || read.Factor != shape.Output || read.Semantic != read.Factor || read.Normalizer.Available() || read.DependencyCount == 0 {
			return false
		}
	}
	for _, read := range cell.impl.reads {
		if read == nil || !read.complete(cell.state, cell, cell.ordinal) {
			return false
		}
	}
	outputOrdinal, outputOK := cell.impl.output.ordinalFromSchema()
	return outputOK && outputOrdinal < cell.schema.factorCount() && cell.schema.factorSemanticAt(outputOrdinal) == shape.Output && cell.impl.operandContent != nil && cell.impl.admission.valid() && cell.impl.transfer != nil
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleProofMatches(proof *ruleRuntimeProof) bool {
	if cell == nil || proof == nil || cell.state == nil || cell.impl == nil || cell.schema != proof.schema || cell.ordinal != proof.ordinal || cell.state != proof.state || cell.impl.state != cell.state || cell.state.authority != proof.bindingAuthority || cell.impl.output == nil || cell.impl.output.schema != proof.schema || cell.impl.output.impl == nil || cell.impl.output.impl.state != proof.state || cell.impl.output.impl.algebra == nil {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok {
		return false
	}
	admission, admitted := coldRuleAdmission(shape.Admission)
	return admitted && shape.OutputKind == proof.outputKind && shape.Output == proof.output && shape.Inputs == proof.inputs && shape.ReadCount == proof.reads && shape.CarryCount == proof.carries && shape.WriteCount == proof.writes && shape.OperandFamily == proof.operandFamily && admission == proof.admission && cell.impl.admission.kind == admission.kind && cell.impl.admission.identity == admission.identity
}

func (cell *schemaFactorBindingCell[K, V]) ordinalFromSchema() (uint64, bool) {
	if cell == nil || cell.schema == nil || !cell.schema.Available() {
		return 0, false
	}
	return cell.ordinal, true
}

type schemaSummaryReadCell[K ~uint32 | ~uint64, V, S any] struct {
	schema      *Schema
	ordinal     uint64
	factor      *schemaFactorBindingCell[K, V]
	algebra     *factbinding.Algebra[K, V]
	form        SchemaReadForm[V]
	normalize   func(OrderedCells[V]) S
	equal       func(S, S) bool
	fingerprint func(S) uint64
}

// schemaOpaqueSummaryRuleReadForm is the coordinate-erased summary authority
// consumed by heterogeneous Rule transactions. V and S remain typed while
// the owning Factor cell keeps its private K instantiation.
type schemaOpaqueSummaryRuleReadForm[V, S any] interface {
	schemaFactorFormBinding
	schemaSummaryRuleReadComplete(*schemaBindingState, *schemaRuleReadOrigin) bool
	schemaSummaryRuleReadBind(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor, *schemaRuleReadOrigin) bool
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaFactorFormComplete() bool {
	if cell == nil || cell.schema == nil || cell.factor == nil || cell.algebra == nil || cell.factor.impl == nil || cell.factor.impl.state == nil || cell.form.cell == nil || cell.form.cell.schema != cell.schema || !summaryReadFormKind(cell.form.cell.kind) || cell.normalize == nil || cell.equal == nil || cell.fingerprint == nil {
		return false
	}
	if cell.ordinal>>32 != cell.factor.ordinal {
		return false
	}
	shape, ok := cell.schema.factorFormShapeAt(cell.factor.ordinal, uint64(uint32(cell.ordinal)))
	return ok && summaryReadRowKind(shape.Kind)
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaSummaryRuleReadComplete(state *schemaBindingState, origin *schemaRuleReadOrigin) bool {
	if cell == nil || state == nil || origin == nil || origin.state != state || origin.kind != composition.ReadSummary ||
		cell.factor == nil || cell.factor.impl == nil || cell.factor.impl.algebra == nil || cell.factor.impl.state != state ||
		cell.factor.ordinal != origin.factor || cell.schema != state.schema || cell.form.cell == nil ||
		cell.form.cell.schema != state.schema || !summaryReadFormKind(cell.form.cell.kind) || !cell.schemaFactorFormComplete() {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSummary && shape.Input == origin.input &&
		shape.Factor == state.schema.factorSemanticAt(origin.factor) && shape.Semantic == origin.semantic &&
		shape.Normalizer == origin.semantic && shape.DependencyCount == 0
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaSummaryRuleReadBind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor, origin *schemaRuleReadOrigin) bool {
	if cell == nil || bound == nil || origin == nil || member.ReadCount() <= int(origin.readOrdinal) || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !origin.matches(proof, origin.readOrdinal) || !cell.schemaSummaryRuleReadComplete(origin.state, origin) {
		return false
	}
	factorKey := origin.state.schema.factorSemanticAt(origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || !factor.receipt.valid() || factor.receipt.state != origin.state ||
		factor.receipt.authority != origin.state.authority || factor.receipt.ordinal != origin.factor ||
		factor.receipt.algebra != cell.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(origin.readOrdinal))
	row, rowOK := origin.state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSummary || row.Input != origin.input ||
		row.Factor != factorKey || row.Semantic != origin.semantic || row.Normalizer != origin.semantic || row.DependencyCount != 0 ||
		surface.Factor != factorKey || surface.Form != equation.SurfaceReadSummary || surface.Semantic != origin.semantic ||
		surface.Normalizer != origin.semantic || surface.Mode != equation.TargetModeNone {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	unitRow, rowPresent := factor.reads[surface]
	proofSummary, proofOK := factor.summaryReadReceiptProof(surface, uint64(uint32(cell.ordinal)), origin.semantic)
	if !unitOK || !rowPresent || unitRow.kind != carrier.SummaryUnit || !proofOK {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: int(origin.input), binding: factor.binding, unit: unit, summary: proofSummary, normalize: cell.normalize, equal: cell.equal, fingerprint: cell.fingerprint})
}

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

// Ref issues the callback-free Factor implementation's opaque exact-key
// capability. The Ref carries the shared sealed authority pointer, never a
// copied SchemaBinding handle or a public coordinate accessor.
func (implementation *FactorImplementation[K, V]) Ref(key K) (Ref[K], bool) {
	if implementation == nil || !implementation.receipt.valid() || uint64(key) >= implementation.receipt.keyEnd {
		return Ref[K]{}, false
	}
	receipt := implementation.receipt
	return Ref[K]{compositionID: receipt.schema.ID(), bindingAuthority: receipt.authority, factorKey: receipt.semantic, factorIndex: receipt.ordinal, raw: key}, true
}

func (implementation *FactorImplementation[K, V]) NewClosedRefs() *ClosedRefs[K] {
	if implementation == nil || !implementation.receipt.valid() {
		return nil
	}
	return &ClosedRefs[K]{receipt: implementation.receipt}
}

func (implementation *FactorImplementation[K, V]) OwnsClosedRefs(refs *ClosedRefs[K]) bool {
	return implementation != nil && refs != nil && refs.validIssuer() && implementation.receipt.valid() && refs.receipt.state == implementation.receipt.state && refs.receipt.authority == implementation.receipt.authority && refs.receipt.schema == implementation.receipt.schema && refs.receipt.ordinal == implementation.receipt.ordinal
}
