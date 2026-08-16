package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
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
func (owner *HotOwner) LinkID() keyspace.ContentID {
	if owner == nil {
		return keyspace.ContentID{}
	}
	return owner.linkOwner.ContentID()
}

// RuleImplementation is Heap's opaque pending rule-receipt issuer. It keeps
// the typed Rule slot private to the Heap Factor owner so child rules cannot
// restate Heap's output factor or its private dense coordinate type.
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

// SelectedRouteRuleBinding is Heap's owner-native handle for the one
// heterogeneous route transaction.  The engine coordinate type remains
// private to this package; callers can only add exact FactorRef/form
// capabilities and commit the sealed cell.
type SelectedRouteRuleBinding[O any] struct {
	owner  *HotOwner
	tx     *engine.SelectedRouteRuleBindingTransaction[coordinate, heap.Value, O]
	issuer *RuleImplementation[O]
}

// BeginSelectedRouteRuleBinding starts Heap's exact route/carry transaction.
// output must be the Ref issued by this owner; no caller-supplied Factor
// ordinal or alternate output authority is accepted.
func BeginSelectedRouteRuleBinding[O any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], carry engine.SchemaCarrySlot[heap.Value], write engine.SchemaWriteSlot[heap.Value], output engine.FactorRef[heap.Value], spec engine.HotRuleSpec[heap.Value, O], carrySpec engine.HotCarrySpec[heap.Value, O]) (*SelectedRouteRuleBinding[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil || output != owner.fragment.Ref() {
		return nil, false
	}
	tx, ok := engine.BeginSelectedRouteRuleBinding[coordinate](owner.binding, slot, carry, write, output, spec, carrySpec)
	if !ok || tx == nil {
		return nil, false
	}
	return &SelectedRouteRuleBinding[O]{owner: owner, tx: tx, issuer: &RuleImplementation[O]{owner: owner, slot: slot}}, true
}

// Implementation returns the pending issuer for this transaction. Resolution
// remains sealed-owner fenced and therefore fails until SchemaBinding.Seal.
func (tx *SelectedRouteRuleBinding[O]) Implementation() (*RuleImplementation[O], bool) {
	if tx == nil || tx.owner == nil || tx.issuer == nil {
		return nil, false
	}
	return tx.issuer, true
}

// AddExactRead appends one exact predecessor in canonical cold read order.
// The engine resolves the heterogeneous FactorRef through its sealed cell;
// no sibling owner coordinate is named here.
func AddExactRead[O any, RV any](tx *SelectedRouteRuleBinding[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV]) (engine.Read[engine.OrderedCells[RV]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.AddSelectedRouteExactRead(tx.tx, slot, factor)
}

// AddSelectedRead appends one selected predecessor in canonical cold read
// order. The callback can emit only exact owner-issued route capabilities.
func AddSelectedRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](tx *SelectedRouteRuleBinding[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil || locate == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.AddSelectedRouteRead[coordinate, heap.Value, O, RV, Tag](tx.tx, slot, factor, locate)
}

// AddOperandSelectedRead binds a selector whose route depends on the
// canonical typed Rule operand. The operand is obtained by the engine from
// the bound member at attach time.
func AddOperandSelectedRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](tx *SelectedRouteRuleBinding[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil || locate == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.AddSelectedRouteOperandRead[coordinate, heap.Value, O, RV, Tag](tx.tx, slot, factor, locate)
}

// Commit publishes the sole Rule cell after all exact and selected reads are
// present and the cold carry/write geometry has been revalidated.
func CommitSelectedRouteRuleBinding[O any](tx *SelectedRouteRuleBinding[O]) bool {
	return tx != nil && tx.owner != nil && tx.tx != nil && engine.CommitSelectedRouteRuleBinding(tx.tx)
}

// AbortSelectedRouteRuleBinding terminally rejects the shared Binding after
// an incomplete heterogeneous assembly; it never leaves a reusable pending
// ordinal behind.
func AbortSelectedRouteRuleBinding[O any](tx *SelectedRouteRuleBinding[O]) bool {
	return tx != nil && tx.owner != nil && tx.tx != nil && engine.AbortSelectedRouteRuleBinding(tx.tx)
}

// BindHot attaches Heap's already-sealed algebra to its one exact cold Factor
// fragment. It neither re-declares the Factor shape nor creates a legacy
// Composition owner.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, schema heap.Schema) (*HotOwner, bool) {
	if !bindingOpen(binding) || fragment == nil || !schema.Valid() || !schema.LinkOwner().Available() {
		return nil, false
	}
	keyEnd := schema.KeyCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return nil, false
	}
	rank, ok := heap.NewWidenRank(schema)
	if !ok {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, schema: schema, linkOwner: schema.LinkOwner(), rank: rank}
	if !engine.BindFactor[coordinate](binding, fragment.slot, engine.HotFactorSpec[coordinate, heap.Value]{
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
		WidenRank: engine.Measure[coordinate, heap.Value]{Width: rank.Width(), At: owner.widenRank},
	}) {
		return nil, false
	}
	return owner, true
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
// private carrier coordinate or engine FactorSlot.
func (owner *HotOwner) FactorRef() engine.FactorRef[heap.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[heap.Value]{}
	}
	return owner.fragment.Ref()
}

// BindExactWriteRule binds a typed Heap-output rule through this exact owner's
// private Factor slot. Child packages supply only their cold Rule/write proofs
// and behavior; they cannot choose another output Factor or coordinate type.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], write engine.SchemaWriteSlot[heap.Value], spec engine.HotRuleSpec[heap.Value, O]) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[coordinate](owner.binding, slot, write, owner.fragment.slot, spec) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindExactReadAndCarryRule binds Heap's one exact-read/one transformed-carry
// output lane. The child receives the typed read receipt needed by its
// transfer and evidence checks, but never Heap's dense coordinate or Factor
// slot.
func BindExactReadAndCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], read engine.SchemaReadSlot[heap.Value], carry engine.SchemaCarrySlot[heap.Value], write engine.SchemaWriteSlot[heap.Value], spec engine.HotRuleSpec[heap.Value, O], carrySpec engine.HotCarrySpec[heap.Value, O]) (*RuleImplementation[O], engine.Read[engine.OrderedCells[heap.Value]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[heap.Value]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithExactReadAndCarry[coordinate, heap.Value, O, coordinate, heap.Value](owner.binding, slot, read, owner.fragment.slot, carry, write, owner.fragment.slot, spec, carrySpec)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[heap.Value]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, runtimeRead, true
}

// BindExactAndSummaryReadAndCarry binds the one heterogeneous Heap/Value
// receipt lane needed by closed allocation. FactorRefs keep both owner
// coordinate types private while the engine owns the shared binding cell.
func BindExactAndSummaryReadAndCarry[O, EV, SV, S any](owner *HotOwner, slot *engine.RuleSlot[heap.Value, O], exactSlot engine.SchemaReadSlot[EV], exactFactor engine.FactorRef[EV], summarySlot engine.SchemaReadSlot[SV], summaryFactor engine.FactorRef[SV], summaryForm engine.SchemaReadForm[SV], carry engine.SchemaCarrySlot[heap.Value], write engine.SchemaWriteSlot[heap.Value], spec engine.HotRuleSpec[heap.Value, O], carrySpec engine.HotCarrySpec[heap.Value, O]) (*RuleImplementation[O], engine.Read[engine.OrderedCells[EV]], engine.Read[S], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	tx, ok := engine.BeginSelectedRuleBinding[coordinate](owner.binding, slot, carry, write, owner.fragment.Ref(), spec, carrySpec)
	if !ok || tx == nil {
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	exactRead, exactOK := engine.AddSelectedRouteExactRead(tx, exactSlot, exactFactor)
	summaryRead, summaryOK := engine.AddSelectedRouteSummaryRead[coordinate, heap.Value, O, SV, S](tx, summarySlot, summaryFactor, summaryForm)
	if !exactOK || !summaryOK || !engine.CommitSelectedRouteRuleBinding(tx) {
		_ = engine.AbortSelectedRouteRuleBinding(tx)
		return nil, engine.Read[engine.OrderedCells[EV]]{}, engine.Read[S]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, exactRead, summaryRead, true
}

// BindExactQuery binds one typed query to this owner's exact Heap Factor
// surface while keeping the private carrier coordinate sealed here.
func BindExactQuery[R any](owner *HotOwner, query *engine.QuerySlot[R], spec engine.HotExactQuerySpec[heap.Value, R]) bool {
	if owner == nil || owner.binding == nil || owner.fragment == nil || query == nil {
		return false
	}
	return engine.BindExactQuery(owner.binding, query, owner.fragment.slot, spec)
}

// ResolveRuleImplementation issues the engine receipt only after the shared
// binding seals. Heap's coordinate remains private to this owner package.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, heap.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.RuleImplementationAt[coordinate, heap.Value, O](issuer.owner.binding, issuer.slot)
}

// ResolveRuleImplementationFor rejects a receipt issued by another equal
// Heap binding before resolving the private engine implementation.
func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, heap.Value, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

// Ref issues the exact Heap coordinate proof for an existing schema Key only
// after the shared binding publishes its immutable Factor implementation.
func (owner *HotOwner) Ref(key heap.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || !owner.schema.OwnsKey(key) {
		return engine.Ref[coordinate]{}, false
	}
	index, present := owner.schema.KeyIndex(key)
	if !present || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, heap.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
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

// TargetMatches proves a staged target belongs to this exact Heap owner.
func (owner *HotOwner) TargetMatches(target engine.RuleTarget, key heap.Key) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.TargetMatchesRef(target, ref)
}

// ReadMatches authenticates an exact Heap read through the sealed owner.
func ReadMatches[V, O, S any](owner *HotOwner, derivation engine.RuleDerivation[V, O], read engine.Read[S], key heap.Key) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.DerivationReadMatchesRef(derivation, read, ref)
}

// SelectionMatches authenticates a typed Heap route selection without
// exposing the private carrier coordinate.
func SelectionMatches[V, O, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, derivation engine.RuleDerivation[V, O], disposition engine.RuleDisposition[V], read engine.Read[engine.Selection[Tag, S]], ordinal int, key heap.Key) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, read, ordinal, ref)
}

// NewRefs begins one exact Heap reference vector owned by this bound Factor.
// It cannot be used before the binding seals or with a different hot owner.
func (owner *HotOwner) NewRefs() *engine.ClosedRefs[coordinate] {
	if owner == nil || owner.binding == nil || owner.fragment == nil {
		return nil
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, heap.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return nil
	}
	return implementation.NewClosedRefs()
}

// AppendKey adds one schema-owned Heap key to an owner-issued reference
// vector. The vector remains opaque to callers and is sealed by CloseRefs.
func (owner *HotOwner) AppendKey(refs *engine.ClosedRefs[coordinate], key heap.Key) bool {
	if !owner.ownsRefs(refs) {
		return false
	}
	ref, ok := owner.Ref(key)
	return ok && refs.Append(ref)
}

func (owner *HotOwner) CloseRefs(refs *engine.ClosedRefs[coordinate]) bool {
	return owner.ownsRefs(refs) && refs.Close()
}

func (owner *HotOwner) ownsRefs(refs *engine.ClosedRefs[coordinate]) bool {
	if owner == nil || refs == nil || owner.binding == nil || owner.fragment == nil {
		return false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, heap.Value](owner.binding, owner.fragment.slot)
	return ok && implementation.OwnsClosedRefs(refs)
}

func (owner *HotOwner) admits(index coordinate, value heap.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.schema.Admits(key, value)
}

func (owner *HotOwner) widenRank(index coordinate, value heap.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	return owner.rank.At(key, value, component)
}

func (owner *HotOwner) keyAt(index coordinate) (heap.Key, bool) {
	if owner == nil || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return heap.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

// KeyForAllocationReceipt is the owner-native post-seal allocation seam. The
// receipt is issued by Heap's mounted artifact catalog; no Program or
// TransformerInput is reopened on this path.
func (owner *HotOwner) KeyForAllocationReceipt(receipt heap.AllocationReceipt) (heap.Key, bool) {
	if owner == nil || owner.schema == (heap.Schema{}) {
		return heap.Key{}, false
	}
	return owner.schema.KeyForAllocationReceipt(receipt)
}

// IndexAccessForReceipt is the owner-native post-seal index occurrence seam.
func (owner *HotOwner) IndexAccessForReceipt(receipt heap.IndexAccessReceipt) (heap.IndexAccess, bool) {
	if owner == nil || owner.schema == (heap.Schema{}) {
		return heap.IndexAccess{}, false
	}
	return owner.schema.IndexAccessForReceipt(receipt)
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
