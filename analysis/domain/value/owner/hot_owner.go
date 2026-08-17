package owner

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
)

// HotOwner is Value's Link-local Factor implementation. The cold Factor
// fragment remains its only structural input; schema supplies the concrete
// lattice, admission, coordinate census, and widening law for this binding.
// The engine implementation and binding are retained privately so future
// Rule binders can use the exact same authority without a second adapter.
type HotOwner struct {
	binding  *engine.SchemaBinding
	fragment *SchemaFragment
	schema   *value.Schema
}

// RuleImplementation is a Value-owned pending receipt issuer. It retains the
// caller's typed Rule slot without exposing Value's private engine coordinate
// type or permitting the caller to restate the output Factor.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[value.Value, O]
}

func (issuer *RuleImplementation[O]) MountedCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.MountedCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

func (issuer *RuleImplementation[O]) LinkCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.LinkCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

// SummaryReceipt is the one constructor-issued Value summary witness retained
// by a hot operand. Its Ref vector remains private to Value owner; consumers
// can only validate the receipt against their exact HotOwner and derivation.
type SummaryReceipt struct {
	owner  *HotOwner
	refs   *engine.ClosedRefs[coordinate]
	width  uint32
	digest [32]byte
}

// ValidForSchema is the narrow source-admission fence used when a Closed
// operand retains this receipt. It exposes neither refs nor coordinates.
func (receipt SummaryReceipt) ValidForSchema(schema *value.Schema) bool {
	return receipt.owner != nil && receipt.owner.schema == schema && receipt.refs != nil && receipt.width != 0
}

// IssuedBy is the exact HotOwner authority fence used to replay an already
// admitted operand without issuing a second summary vector.
func (receipt SummaryReceipt) IssuedBy(owner *HotOwner) bool {
	return owner != nil && receipt.owner == owner && receipt.ValidForSchema(owner.schema)
}

// Width is the immutable number of coordinates captured by this receipt.
func (receipt SummaryReceipt) Width() int { return int(receipt.width) }

// MatchesCoordinates is a cold/admission fence for the exact Closed vector.
// It rejects equal-width coordinate splices before the receipt enters a hot
// operand; the hot checker never calls this vector comparison.
func (receipt SummaryReceipt) MatchesCoordinates(schema *value.Schema, locations []value.Coordinate) bool {
	if !receipt.ValidForSchema(schema) || len(locations) != int(receipt.width) {
		return false
	}
	keys := make([]uint64, len(locations))
	for index, location := range locations {
		raw, ok := schema.CoordinateIndex(location)
		if !ok {
			return false
		}
		keys[index] = uint64(raw)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return engine.SummaryVectorDigest(keys) == receipt.digest
}

// SelectedRuleBinding is Value's owner-native heterogeneous selected-read
// transaction.  The output remains Value-owned while predecessor receipts
// may come from sibling Factors.
type SelectedRuleBinding[O any] struct {
	owner  *HotOwner
	tx     *engine.SelectedRouteRuleBindingTransaction[coordinate, value.Value, O]
	issuer *RuleImplementation[O]
}

// BindSelectedRule owns Value's heterogeneous selected-read transaction. bind
// attaches the reads and its answer terminalizes the transaction, so a
// rejected assembly always reaches the shared Binding's terminal poison.
func BindSelectedRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], carry engine.SchemaCarrySlot[value.Value], write engine.SchemaWriteSlot[value.Value], output engine.FactorRef[value.Value], spec engine.HotRuleSpec[value.Value, O], carrySpec engine.HotCarrySpec[value.Value, O], bind func(*SelectedRuleBinding[O]) bool) bool {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil || output != owner.fragment.Ref() || bind == nil {
		return false
	}
	return engine.BindSelectedRule[coordinate](owner.binding, slot, carry, write, output, spec, carrySpec, func(tx *engine.SelectedRouteRuleBindingTransaction[coordinate, value.Value, O]) bool {
		return bind(&SelectedRuleBinding[O]{owner: owner, tx: tx, issuer: &RuleImplementation[O]{owner: owner, slot: slot}})
	})
}

func (tx *SelectedRuleBinding[O]) Implementation() (*RuleImplementation[O], bool) {
	if tx == nil || tx.owner == nil || tx.issuer == nil {
		return nil, false
	}
	return tx.issuer, true
}

func AddSelectedRuleExactRead[O any, RV any](tx *SelectedRuleBinding[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV]) (engine.Read[engine.OrderedCells[RV]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.AddSelectedRouteExactRead(tx.tx, slot, factor)
}

func AddSelectedRuleRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](tx *SelectedRuleBinding[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil || locate == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.AddSelectedRouteRead[coordinate, value.Value, O, RV, Tag](tx.tx, slot, factor, locate)
}

func AddSelectedRuleOperandRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](tx *SelectedRuleBinding[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil || locate == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.AddSelectedRouteOperandRead[coordinate, value.Value, O, RV, Tag](tx.tx, slot, factor, locate)
}

// BindHot admits Value's concrete algebra into the exact SchemaBinding. It
// binds the Value summary form immediately because later Rule/query binders
// must consume the same Factor-owned summary implementation.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, schema *value.Schema) (*HotOwner, bool) {
	if binding == nil || fragment == nil || schema == nil || !schema.LinkOwner().Available() || !bindingOpen(binding) {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, schema: schema}
	spec, specOK := owner.FactorSpec()
	if !specOK || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[coordinate, value.Value](binding, fragment.slot, fragment.summaryRead) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[coordinate, value.Value](binding, fragment.slot, fragment.foldRead) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Value's exact Factor algebra for this binding: the same value
// BindHot hands to the engine. A declaration surface projects this record
// instead of restating the lattice, admission, or widening law, so the two
// cannot drift.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, value.Value], bool) {
	if owner == nil || owner.schema == nil {
		return engine.HotFactorSpec[coordinate, value.Value]{}, false
	}
	keyEnd := owner.schema.CoordinateCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[coordinate, value.Value]{}, false
	}
	return engine.HotFactorSpec[coordinate, value.Value]{
		KeyEnd:      uint64(keyEnd),
		Lattice:     owner.schema.Domain(),
		Default:     owner.schema.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.schema.Fingerprint,
		WidenRank: engine.Measure[coordinate, value.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, true
}

// Schema returns the Link-local Value schema used to admit this binding.
func (owner *HotOwner) Schema() *value.Schema {
	if owner == nil {
		return nil
	}
	return owner.schema
}

// MatchesBinding proves that this owner belongs to the exact shared hot
// transaction. Equal Schema contents from another Binding are not sufficient.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

// FactorRef returns this owner's sealed Factor surface without exposing its
// private carrier coordinate or engine FactorSlot.
func (owner *HotOwner) FactorRef() engine.FactorRef[value.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[value.Value]{}
	}
	return owner.fragment.Ref()
}

func (owner *HotOwner) implementationAt() (*engine.FactorImplementation[coordinate, value.Value], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil {
		return nil, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, value.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

// BindExactWriteRule binds one Value-output Rule through this owner's exact
// Factor slot. The child package supplies its own typed Rule slot, write
// slot, operand admission, and transfer implementation; it cannot supply a
// different output Factor or recover Value's coordinate type.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], write engine.SchemaWriteSlot[value.Value], spec engine.HotRuleSpec[value.Value, O]) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[coordinate](owner.binding, slot, write, owner.fragment.slot, spec) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindCarryRule binds one transformed-carry Rule through this owner's exact
// Factor slot. The child supplies only its typed operand transform and
// transfer; the SchemaCarrySlot remains the sole structural authority.
func BindCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], carry engine.SchemaCarrySlot[value.Value], write engine.SchemaWriteSlot[value.Value], spec engine.HotRuleSpec[value.Value, O], carrySpec engine.HotCarrySpec[value.Value, O]) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRuleWithCarry[coordinate](owner.binding, slot, carry, write, owner.fragment.slot, spec, carrySpec) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindExactReadAndCarryRule binds Value's one exact-read/one carry/one exact
// write lane through the same sealed Factor authority. The child receives the
// typed read capability required by its transfer and evidence checker, but it
// cannot recover Value's private coordinate or Factor slot.
func BindExactReadAndCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], read engine.SchemaReadSlot[value.Value], carry engine.SchemaCarrySlot[value.Value], write engine.SchemaWriteSlot[value.Value], spec engine.HotRuleSpec[value.Value, O], carrySpec engine.HotCarrySpec[value.Value, O]) (*RuleImplementation[O], engine.Read[engine.OrderedCells[value.Value]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[value.Value]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithExactReadAndCarry[coordinate, value.Value, O, coordinate, value.Value](owner.binding, slot, read, owner.fragment.slot, carry, write, owner.fragment.slot, spec, carrySpec)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[value.Value]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, runtimeRead, true
}

// BindExactQuery binds a typed exact query to this owner's Factor slot while
// keeping the Factor coordinate and slot authority private to Value owner.
func BindExactQuery[R any](owner *HotOwner, query *engine.QuerySlot[R], spec engine.HotExactQuerySpec[value.Value, R]) bool {
	if owner == nil || owner.binding == nil || owner.fragment == nil || query == nil {
		return false
	}
	return engine.BindExactQuery(owner.binding, query, owner.fragment.slot, spec)
}

// BindSummaryQuery binds Value's catalog summary Query to the exact
// owner-issued summary form. The form is explicit so a Link cannot silently
// substitute another normalizer with the same Factor and Query family.
func BindSummaryQuery[R any](owner *HotOwner, query *engine.QuerySlot[R], form engine.SchemaReadForm[value.Value], spec engine.HotSummaryQuerySpec[value.Value, R]) bool {
	if owner == nil || owner.binding == nil || owner.fragment == nil || query == nil {
		return false
	}
	return engine.BindSummaryQuery(owner.binding, query, owner.fragment.slot, form, spec)
}

// ResolveRuleImplementation issues the engine receipt only after the shared
// binding has sealed. The private coordinate remains inferred by callers and
// cannot be named or manufactured by child domains.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, value.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.RuleImplementationAt[coordinate, value.Value, O](issuer.owner.binding, issuer.slot)
}

// ResolveRuleImplementationFor is the explicit owner fence used by child
// binders that retain more than one equal-Schema HotOwner. A receipt issued
// by another binding is rejected before engine resolution.
func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, value.Value, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

// Ref issues the exact Value key capability after binding publication. The
// caller supplies a Value schema coordinate, never a dense engine coordinate.
func (owner *HotOwner) Ref(location value.Coordinate) (engine.Ref[coordinate], bool) {
	implementation, ok := owner.implementationAt()
	if !ok || owner.schema == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.schema.CoordinateIndex(location)
	if !ok || uint64(index) >= uint64(owner.schema.CoordinateCount()) {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

// SelectRoute emits one exact Value route without exposing the owner's
// private carrier coordinate to a child Rule locator.
func (owner *HotOwner) SelectRoute(context engine.SelectorContext, location value.Coordinate, tag uint64) bool {
	ref, ok := owner.Ref(location)
	return ok && engine.SelectRoute(context, ref, tag)
}

// SelectRouteTyped preserves the child's exact semantic route-tag type while
// keeping Value's carrier coordinate private.
func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, context engine.SelectorContext, location value.Coordinate, tag Tag) bool {
	ref, ok := owner.Ref(location)
	return ok && engine.SelectRoute(context, ref, tag)
}

// TargetMatches proves a staged target belongs to this exact Value owner.
func (owner *HotOwner) TargetMatches(target engine.RuleTarget, location value.Coordinate) bool {
	ref, ok := owner.Ref(location)
	return ok && engine.TargetMatchesRef(target, ref)
}

// ReadMatches authenticates one exact read against this owner's sealed
// coordinate authority without exporting the private Ref type.
func ReadMatches[V, O, S any](owner *HotOwner, derivation engine.RuleDerivation[V, O], read engine.Read[S], location value.Coordinate) bool {
	ref, ok := owner.Ref(location)
	return ok && engine.DerivationReadMatchesRef(derivation, read, ref)
}

// SelectionMatches authenticates one exact selected route and preserves the
// caller's semantic tag type end-to-end.
func SelectionMatches[V, O, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, derivation engine.RuleDerivation[V, O], disposition engine.RuleDisposition[V], read engine.Read[engine.Selection[Tag, S]], ordinal int, location value.Coordinate) bool {
	ref, ok := owner.Ref(location)
	return ok && engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, read, ordinal, ref)
}

// NewSummaryRefs starts one Factor-issued summary vector. It is sealed by
// CloseSummaryRefs and can only be appended through this exact HotOwner.
func (owner *HotOwner) NewSummaryRefs() *engine.ClosedRefs[coordinate] {
	implementation, ok := owner.implementationAt()
	if !ok {
		return nil
	}
	return implementation.NewClosedRefs()
}

func (owner *HotOwner) AppendSummaryCoordinate(refs *engine.ClosedRefs[coordinate], location value.Coordinate) bool {
	ref, ok := owner.Ref(location)
	return ok && refs != nil && refs.Append(ref)
}

func (owner *HotOwner) CloseSummaryRefs(refs *engine.ClosedRefs[coordinate]) bool {
	return owner != nil && refs != nil && refs.Close()
}

// IssueSummaryReceipt seals one exact coordinate vector once during operand
// admission. The hot checker later consumes only this opaque witness.
func (owner *HotOwner) IssueSummaryReceipt(locations []value.Coordinate) (SummaryReceipt, bool) {
	if owner == nil || owner.schema == nil || len(locations) == 0 || uint64(len(locations)) > uint64(^uint32(0)) {
		return SummaryReceipt{}, false
	}
	refs := owner.NewSummaryRefs()
	if refs == nil {
		return SummaryReceipt{}, false
	}
	for _, location := range locations {
		if !owner.AppendSummaryCoordinate(refs, location) {
			return SummaryReceipt{}, false
		}
	}
	if !owner.CloseSummaryRefs(refs) {
		return SummaryReceipt{}, false
	}
	keys := make([]uint64, len(locations))
	for index, location := range locations {
		raw, ok := owner.schema.CoordinateIndex(location)
		if !ok {
			return SummaryReceipt{}, false
		}
		keys[index] = uint64(raw)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return SummaryReceipt{owner: owner, refs: refs, width: uint32(len(locations)), digest: engine.SummaryVectorDigest(keys)}, true
}

// MatchSummaryReceipt authenticates a previously issued receipt against the
// sealed engine summary proof. This method never rebuilds coordinates or
// allocates ClosedRefs; the engine owns any canonical-vector comparison.
func MatchSummaryReceipt[V, O, S any](owner *HotOwner, receipt SummaryReceipt, derivation engine.RuleDerivation[V, O], read engine.Read[S]) bool {
	return owner != nil && receipt.owner == owner && receipt.ValidForSchema(owner.schema) && engine.DerivationReadMatchesSummaryRefs(derivation, read, receipt.refs)
}

// AddSummaryReceiptRead bridges a Value-owned summary witness into a mounted
// RuleSourceTransaction without exposing Value's private coordinate type or
// its ClosedRefs vector. The caller supplies the sealed Rule-owned summary
// read proof (normally obtained from its own RuleImplementation); the engine
// validates the exact rule/read/factor semantic before receiving refs.
func AddSummaryReceiptRead(owner *HotOwner, transaction *engine.RuleSourceTransaction, receipt SummaryReceipt, read engine.SchemaSummaryReadReceipt) bool {
	return owner != nil && transaction != nil && receipt.IssuedBy(owner) && read.Valid() && engine.AddSummaryRead(transaction, read, receipt.refs)
}

func (owner *HotOwner) admits(index coordinate, fact value.Value) bool {
	if owner == nil || owner.schema == nil {
		return false
	}
	location, ok := owner.schema.CoordinateAt(int(index))
	return ok && owner.schema.AdmitsCoordinate(location, fact)
}

func (owner *HotOwner) widenRank(index coordinate, fact value.Value, component int) uint64 {
	if owner == nil || owner.schema == nil || component != 0 {
		return 0
	}
	_, ok := owner.schema.CoordinateAt(int(index))
	if !ok {
		return 0
	}
	return owner.schema.WidenMeasure(fact)
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
