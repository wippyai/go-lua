package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// HotOwner is Placement's Link-local Factor owner. Its dense coordinate is
// exactly Heap's coordinate at the same ordinal; no second index is retained.
type HotOwner struct {
	binding          *engine.SchemaBinding
	fragment         *SchemaFragment
	schema           placement.Schema
	containmentCache *placement.StaticContainmentCache
}

// BindHot attaches Placement's owner algebra to its exact cold Factor slot.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, schema placement.Schema) (*HotOwner, bool) {
	if !bindingOpen(binding) || fragment == nil || !schema.Valid() {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, schema: schema, containmentCache: placement.NewStaticContainmentCache(schema)}
	if owner.containmentCache == nil {
		return nil, false
	}
	spec, ok := owner.FactorSpec()
	if !ok || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[coordinate, placement.Placement](binding, fragment.slot, fragment.foldRead) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Placement's exact Link-local factor algebra.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, placement.Placement], bool) {
	if owner == nil || !owner.schema.Valid() {
		return engine.HotFactorSpec[coordinate, placement.Placement]{}, false
	}
	keyEnd := owner.schema.KeyCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[coordinate, placement.Placement]{}, false
	}
	return engine.HotFactorSpec[coordinate, placement.Placement]{
		KeyEnd:      uint64(keyEnd),
		Lattice:     placement.Lattice(),
		Default:     placement.Bottom,
		AdmitAt:     owner.admits,
		Fingerprint: func(value placement.Placement) uint64 { return value.Hash() },
		WidenRank: engine.Measure[coordinate, placement.Placement]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, true
}

// Schema returns Placement's exact cold authority.
func (owner *HotOwner) Schema() placement.Schema {
	if owner == nil {
		return placement.Schema{}
	}
	return owner.schema
}

// StaticContainmentCache returns the exact owner-lifetime memo used by the
// heterogeneous Placement+Heap query. The cache is fenced to this owner's
// Schema and binding at construction; callers cannot replace its authority.
func (owner *HotOwner) StaticContainmentCache() *placement.StaticContainmentCache {
	if owner == nil {
		return nil
	}
	return owner.containmentCache
}

// MatchesBinding proves that owner and binding share the exact hot
// transaction; equal schema content from another binding is insufficient.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

// FactorRef returns Placement's sealed Factor surface without exposing its
// private carrier coordinate or FactorSlot.
func (owner *HotOwner) FactorRef() engine.FactorRef[placement.Placement] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[placement.Placement]{}
	}
	return owner.fragment.Ref()
}

// ExactRead returns Placement's exact factor read form.
func (owner *HotOwner) ExactRead() engine.SchemaReadForm[placement.Placement] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[placement.Placement]{}
	}
	return owner.fragment.ExactRead()
}

// FoldSummaryRead returns Placement's coordinatewise summary read form. The
// query vertical consumes this public owner surface; the base owner does not
// carry any producer-owned evidence or query state.
func (owner *HotOwner) FoldSummaryRead() engine.SchemaReadForm[placement.Placement] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[placement.Placement]{}
	}
	return owner.fragment.FoldSummaryRead()
}

// ExactWrite returns Placement's exact factor write form.
func (owner *HotOwner) ExactWrite() engine.SchemaWriteForm[placement.Placement] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaWriteForm[placement.Placement]{}
	}
	return owner.fragment.ExactWrite()
}

// BindExactWriteRule binds one zero-input Placement write through this
// owner's Factor. It is the minimal rule lane shared by allocation-root seed
// rules.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[placement.Placement, O], write engine.SchemaWriteSlot[placement.Placement], spec engine.HotRuleSpec[placement.Placement, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[coordinate](owner.binding, slot, write, owner.fragment.slot, spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindSelectedRuleDirect installs a selected direct-write Placement rule.
func BindSelectedRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[placement.Placement, O], write engine.SchemaWriteSlot[placement.Placement], spec engine.HotRuleSpec[placement.Placement, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindSelectedExactRuleDirect[coordinate](owner.binding, slot, write, owner.fragment.Ref(), spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindSelectedRouteRuleDirect installs a selected-read/routed-write
// Placement Rule. The route read and write geometry remain in the cold
// fragment; this wrapper only admits the exact owner-fenced hot cell.
func BindSelectedRouteRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[placement.Placement, O], carry engine.SchemaCarrySlot[placement.Placement], write engine.SchemaWriteSlot[placement.Placement], output engine.FactorRef[placement.Placement], spec engine.HotRuleSpec[placement.Placement, O], carrySpec engine.HotCarrySpec[placement.Placement, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil || output != owner.fragment.Ref() {
		return nil, false
	}
	if !engine.BindSelectedRouteRuleDirect[coordinate](owner.binding, slot, carry, write, output, spec, carrySpec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// AddSelectedRuleDirectExactRead installs a heterogeneous exact predecessor
// at its declared cold ordinal on a selected Placement Rule.
func AddSelectedRuleDirectExactRead[O any, RV any](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], project func(O) (uint64, bool)) (engine.Read[engine.OrderedCells[RV]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.BindSelectedRuleDirectExactRead[coordinate](issuer.owner.binding, issuer.slot, slot, factor, project)
}

// AddSelectedRuleDirectOperandRead installs an operand-dependent selected
// predecessor at its declared cold ordinal. It is used by containment to
// discover exact child roots from the current Heap operand, not from a
// precomputed parent/child pair catalog.
func AddSelectedRuleDirectOperandRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.BindSelectedRuleDirectOperandRead[coordinate, placement.Placement, O, RV, Tag](issuer.owner.binding, issuer.slot, slot, factor, locate)
}

// RuleImplementation is Placement's opaque pending rule receipt.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[placement.Placement, O]
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

// ResolveRuleImplementation issues the engine rule receipt after sealing.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, placement.Placement, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[coordinate, placement.Placement, O](issuer.owner.binding, issuer.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

// ResolveRuleImplementationFor rejects a receipt issued by another equal
// Placement binding before resolving the private engine implementation.
func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, placement.Placement, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

// Ref issues the exact Placement coordinate proof for an existing Heap key.
// Allocation identity and dense ordinal remain owned by Heap; Placement only
// projects that already-authenticated coordinate into its factor.
func (owner *HotOwner) Ref(key heap.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || !owner.schema.Valid() || !owner.schema.Heap().OwnsKey(key) {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.schema.Heap().KeyIndex(key)
	if !ok || index < 0 || uint64(index) > uint64(^uint32(0)) || index >= owner.schema.KeyCount() {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, placement.Placement](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

// SelectRoute emits one exact Placement route through this owner-native
// receipt. The numeric tag is transport evidence only; the target remains the
// owner-issued Heap-aligned coordinate.
func (owner *HotOwner) SelectRoute(context engine.SelectorContext, key heap.Key, tag uint64) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

// SelectRouteTyped preserves a child rule's exact route-tag type at the
// owner boundary.
func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, context engine.SelectorContext, key heap.Key, tag Tag) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

func (owner *HotOwner) admits(index coordinate, fact placement.Placement) bool {
	key, ok := owner.keyAt(index)
	if !ok {
		return false
	}
	if key.Kind() == heap.RootBoot {
		return fact == placement.Bottom
	}
	_, valid := placementRank(fact)
	return key.Kind() == heap.RootAllocation && valid
}

func (owner *HotOwner) widenRank(index coordinate, fact placement.Placement, component int) uint64 {
	if owner == nil || component != 0 || !owner.admits(index, fact) {
		return 0
	}
	rank, ok := placementRank(fact)
	if !ok {
		return 0
	}
	return uint64(rank)
}

func placementRank(fact placement.Placement) (int, bool) {
	switch fact {
	case placement.Bottom:
		// The Placement lattice ascends toward Unknown while the engine's
		// widening witness must descend on every strict ascent.  Keep the
		// most precise point at the largest finite rank and Top at zero.
		return 4, true
	case placement.Stack:
		return 3, true
	case placement.OwnedHeap:
		return 2, true
	case placement.SharedHeap:
		return 1, true
	case placement.Unknown:
		return 0, true
	default:
		return 0, false
	}
}

func (owner *HotOwner) keyAt(index coordinate) (heap.Key, bool) {
	if owner == nil || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return heap.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
