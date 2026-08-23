package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/domain/heap"
)

// HotOwner is Heap's exact Link-local Factor implementation. The retained
// fields are private receipts for its issued proof operations; it publishes no
// SchemaBinding, cold slot, mutable algebra snapshot, or generic Factor
// implementation to sibling packages.
type HotOwner struct {
	binding   *engine.SchemaBinding
	fragment  *SchemaFragment
	schema    heap.Schema
	linkOwner link.OwnerCapability
	rank      heap.WidenRank
}

// LinkOwner returns the exact detached Link witness captured at hot bind.
func (owner *HotOwner) LinkOwner() link.OwnerCapability {
	if owner == nil {
		return link.OwnerCapability{}
	}
	return owner.linkOwner
}

// LinkID returns the scalar identity paired with LinkOwner.
func (owner *HotOwner) LinkID() identity.ContentID {
	if owner == nil {
		return identity.ContentID{}
	}
	return owner.linkOwner.ContentID()
}

// RuleImplementation is Heap's opaque pending rule-receipt issuer. It keeps
// the typed Rule slot private to the Heap Factor owner so child rules cannot
// restate Heap's output factor or its private dense Coordinate type.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[heap.Value, O]
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

// BindSelectedRouteRuleDirect installs Heap's routed-write Rule cell at its
// declared schema ordinal. The returned issuer is owner-typed and carries no
// transaction or append-order state.
func BindSelectedRouteRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], carry engine.SchemaCarrySlot[heap.Value], write engine.SchemaWriteSlot[heap.Value], output engine.FactorRef[heap.Value], spec engine.HotRuleSpec[heap.Value, O], carrySpec engine.HotCarrySpec[heap.Value, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil || output != owner.fragment.Ref() {
		return nil, false
	}
	if !engine.BindSelectedRouteRuleDirect[Coordinate](owner.binding, slot, carry, write, output, spec, carrySpec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// AddSelectedRouteRuleDirectExactRead installs an exact predecessor at its
// declared cold ordinal on a directly bound Heap Rule.
func AddSelectedRouteRuleDirectExactRead[O any, RV any](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], project func(O) (uint64, bool)) (engine.Read[engine.OrderedCells[RV]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.BindSelectedRuleDirectExactRead[Coordinate](issuer.owner.binding, issuer.slot, slot, factor, project)
}

// AddSelectedRouteRuleDirectOperandRead installs an operand-dependent
// selector at its declared cold ordinal on a directly bound Heap Rule.
func AddSelectedRouteRuleDirectOperandRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.BindSelectedRuleDirectOperandRead[Coordinate, heap.Value, O, RV, Tag](issuer.owner.binding, issuer.slot, slot, factor, locate)
}

// AddSelectedRouteRuleDirectOperandReadUnderContract installs the same
// operand-dependent selector under an explicit engine read-boundary contract.
// The contract is the rule's one declaration of member order, sparse default
// and opaque disposition; the engine, not the rule, then delivers them.
func AddSelectedRouteRuleDirectOperandReadUnderContract[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool, contract engine.ReadContract) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.BindSelectedRuleDirectOperandReadUnderContract[Coordinate, heap.Value, O, RV, Tag](issuer.owner.binding, issuer.slot, slot, factor, locate, contract)
}

// BindHot attaches Heap's already-sealed algebra to its one exact cold Factor
// fragment. It neither re-declares the Factor shape nor creates a legacy
// Composition owner.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, schema heap.Schema) (*HotOwner, bool) {
	if !bindingOpen(binding) || fragment == nil || !schema.Valid() || !schema.LinkOwner().Available() {
		return nil, false
	}
	rank, ok := heap.NewWidenRank(schema)
	if !ok {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, schema: schema, linkOwner: schema.LinkOwner(), rank: rank}
	spec, specOK := owner.FactorSpec()
	if !specOK || !engine.BindFactor[Coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[Coordinate, heap.Value](binding, fragment.slot, fragment.summaryRead) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Heap's exact Factor algebra for this binding: the same value
// BindHot hands to the engine. A declaration surface projects this record
// instead of restating the lattice, admission, or widening law, so the two
// cannot drift.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[Coordinate, heap.Value], bool) {
	if owner == nil || !owner.schema.Valid() {
		return engine.HotFactorSpec[Coordinate, heap.Value]{}, false
	}
	keyEnd := owner.schema.KeyCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[Coordinate, heap.Value]{}, false
	}
	schema := owner.schema
	return engine.HotFactorSpec[Coordinate, heap.Value]{
		KeyEnd:  uint64(keyEnd),
		Lattice: schema.Domain(),
		Default: schema.Default(),
		AdmitAt: owner.admits,
		Fingerprint: func(value heap.Value) uint64 {
			fingerprint, valid := schema.Fingerprint(value)
			if !valid {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[Coordinate, heap.Value]{Width: owner.rank.Width(), At: owner.widenRank},
	}, true
}

// Schema returns Heap's already-sealed Link-local semantic authority. It is
// an immutable typed proof needed by Heap-owned operand/evidence rules; the
// shared SchemaBinding and Factor slot remain private.
func (owner *HotOwner) Schema() heap.Schema {
	if owner == nil {
		return heap.Schema{}
	}
	return owner.schema
}

// MatchesBinding proves that this owner belongs to the exact shared hot
// transaction. Equal Schema contents from another Binding are not sufficient.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

// FactorRef returns this owner's sealed Factor surface without exposing its
// private carrier Coordinate or engine FactorSlot.
func (owner *HotOwner) FactorRef() engine.FactorRef[heap.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[heap.Value]{}
	}
	return owner.fragment.Ref()
}

// SummaryRead returns Heap's owner-issued complete-vector summary form. The
// form is exposed only through this owner so consumers can pair it with the
// exact hot binding that authenticated its Factor implementation.
func (owner *HotOwner) SummaryRead() engine.SchemaReadForm[heap.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[heap.Value]{}
	}
	return owner.fragment.SummaryRead()
}

// BindExactWriteRule binds a typed Heap-output rule through this exact owner's
// private Factor slot. Child packages supply only their cold Rule/write proofs
// and behavior; they cannot choose another output Factor or Coordinate type.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], write engine.SchemaWriteSlot[heap.Value], spec engine.HotRuleSpec[heap.Value, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[Coordinate](owner.binding, slot, write, owner.fragment.slot, spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindExactReadAndCarryRule binds Heap's one exact-read/one transformed-carry
// output lane. The child receives the typed read receipt needed by its
// transfer and evidence checks, but never Heap's dense Coordinate or Factor
// slot.
func BindExactReadAndCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], read engine.SchemaReadSlot[heap.Value], carry engine.SchemaCarrySlot[heap.Value], write engine.SchemaWriteSlot[heap.Value], spec engine.HotRuleSpec[heap.Value, O], carrySpec engine.HotCarrySpec[heap.Value, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], engine.Read[engine.OrderedCells[heap.Value]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[heap.Value]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithExactReadAndCarry[Coordinate, heap.Value, O, Coordinate, heap.Value](owner.binding, slot, read, owner.fragment.slot, carry, write, owner.fragment.slot, spec, carrySpec, projectRead, projectWrite)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[heap.Value]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, runtimeRead, true
}

// BindExactAndSummaryReadAndCarry binds the one heterogeneous Heap/Value
// read lane needed by closed allocation directly at its declared ordinals.
// FactorRefs keep both owner Coordinate types private while the engine owns
// the shared binding cell; no construction transaction is retained.
func BindExactAndSummaryReadAndCarry[O, EV, SV, S any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], exactSlot engine.SchemaReadSlot[EV], exactFactor engine.FactorRef[EV], summarySlot engine.SchemaReadSlot[SV], summaryFactor engine.FactorRef[SV], summaryForm engine.SchemaReadForm[SV], carry engine.SchemaCarrySlot[heap.Value], write engine.SchemaWriteSlot[heap.Value], spec engine.HotRuleSpec[heap.Value, O], carrySpec engine.HotCarrySpec[heap.Value, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], engine.Read[engine.OrderedCells[EV]], engine.Read[S], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	if !engine.BindSelectedRuleDirect[Coordinate](owner.binding, slot, carry, write, owner.fragment.Ref(), spec, carrySpec, projectWrite) {
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	exactRead, exactOK := engine.BindSelectedRuleDirectExactRead[Coordinate, heap.Value, O, EV](owner.binding, slot, exactSlot, exactFactor, projectRead)
	if !exactOK {
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	summaryRead, summaryOK := engine.BindSelectedRuleDirectSummaryRead[Coordinate, heap.Value, O, SV, S](owner.binding, slot, summarySlot, summaryFactor, summaryForm)
	if !summaryOK {
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, exactRead, summaryRead, true
}

// BindExactQuery binds one typed query to this owner's exact Heap Factor
// surface while keeping the private carrier Coordinate sealed here.
func BindExactQuery[R any](owner *HotOwner, query *engine.QuerySlot[R], spec engine.HotExactQuerySpec[heap.Value, R]) bool {
	if owner == nil || owner.binding == nil || owner.fragment == nil || query == nil {
		return false
	}
	return engine.BindExactQuery(owner.binding, query, owner.fragment.slot, spec)
}

// ResolveRuleImplementation issues the engine receipt only after the shared
// binding seals. Heap's Coordinate remains private to this owner package.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[Coordinate, heap.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[Coordinate, heap.Value, O](issuer.owner.binding, issuer.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

// ResolveRuleImplementationFor rejects a receipt issued by another equal
// Heap binding before resolving the private engine implementation.
func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[Coordinate, heap.Value, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

// Ref issues the exact Heap Coordinate proof for an existing schema Key only
// after the shared binding publishes its immutable Factor implementation.
func (owner *HotOwner) Ref(key heap.Key) (engine.Ref[Coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || !owner.schema.OwnsKey(key) {
		return engine.Ref[Coordinate]{}, false
	}
	index, present := owner.schema.KeyIndex(key)
	if !present || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return engine.Ref[Coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[Coordinate, heap.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[Coordinate]{}, false
	}
	return implementation.Ref(Coordinate(index))
}

// SelectRoute emits one exact Heap route through this owner-native receipt.
func (owner *HotOwner) SelectRoute(context engine.SelectorContext, key heap.Key, tag heap.RawRouteTag) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

// SelectRouteTyped preserves an owner child's exact route-tag type.
func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, context engine.SelectorContext, key heap.Key, tag Tag) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

func (owner *HotOwner) admits(index Coordinate, value heap.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.schema.Admits(key, value)
}

func (owner *HotOwner) widenRank(index Coordinate, value heap.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	return owner.rank.At(key, value, component)
}

func (owner *HotOwner) keyAt(index Coordinate) (heap.Key, bool) {
	if owner == nil || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return heap.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
