package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/factor"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

// HotOwner is Effect's Link-local Factor binder. The cold Factor identity and
// exact forms come only from SchemaFragment; the algebra supplies the
// Link-local lattice, admission, fingerprint, and rank laws.
//
// The engine implementation remains private to SchemaBinding. HotOwner keeps
// only immutable authorities and asks the binding for a fresh sealed proof
// when it must issue a coordinate reference.
type HotOwner struct {
	binding   *engine.SchemaBinding
	fragment  *SchemaFragment
	algebra   *factor.Algebra
	linkOwner link.OwnerCapability
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

// RuleImplementation is Effect's opaque pending Rule receipt. Child packages
// can resolve and attach only the exact Rule slot bound through this owner;
// Effect's private coordinate and Factor slot never escape.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[factor.Value, O]
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

// SelectedRuleBinding is Effect's no-carry selected-read transaction. The
// cold Rule owns its exact write and read order; this wrapper only preserves
// Effect's output coordinate authority across the one-shot engine assembly.
type SelectedRuleBinding[O any] struct {
	owner  *HotOwner
	tx     *engine.SelectedRouteRuleBindingTransaction[coordinate, factor.Value, O]
	issuer *RuleImplementation[O]
}

// BindHot attaches Effect's already-sealed Link-local algebra to its exact
// cold Factor slot. It never re-declares the Factor shape or retains a mutable
// FactorImplementation.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, algebra *factor.Algebra) (*HotOwner, bool) {
	if !effectBindingOpen(binding) || fragment == nil || algebra == nil || !algebra.Valid() || !validCoordinateCount(algebra.RootCount()) {
		return nil, false
	}
	lattice, ok := algebra.Lattice()
	if !ok {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, algebra: algebra, linkOwner: algebra.LinkOwner()}
	if !engine.BindFactor[coordinate](binding, fragment.slot, engine.HotFactorSpec[coordinate, factor.Value]{
		KeyEnd:      uint64(algebra.RootCount()),
		Lattice:     lattice,
		Default:     algebra.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, factor.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}) {
		return nil, false
	}
	return owner, true
}

// Algebra returns Effect's exact Link-local semantic authority.
func (owner *HotOwner) Algebra() *factor.Algebra {
	if owner == nil {
		return nil
	}
	return owner.algebra
}

// MatchesBinding proves exact hot transaction ownership without exposing the
// retained binding or Effect's private Factor coordinate.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

// BindExactWriteRule binds one zero-input Effect source through the exact
// owner Factor. It is intentionally disjoint from the selected-read
// transaction used by BodyCall.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[factor.Value, O], write engine.SchemaWriteSlot[factor.Value], spec engine.HotRuleSpec[factor.Value, O]) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[coordinate](owner.binding, slot, write, owner.fragment.slot, spec) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// FactorRef returns the exact cold Effect Factor surface without exposing its
// private dense coordinate or FactorSlot.
func (owner *HotOwner) FactorRef() engine.FactorRef[factor.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[factor.Value]{}
	}
	return owner.fragment.Ref()
}

// ExactRead returns Effect's exact body-root observation form.
func (owner *HotOwner) ExactRead() engine.SchemaReadForm[factor.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[factor.Value]{}
	}
	return owner.fragment.exactRead
}

// ExactWrite returns Effect's exact body-root output form.
func (owner *HotOwner) ExactWrite() engine.SchemaWriteForm[factor.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaWriteForm[factor.Value]{}
	}
	return owner.fragment.exactWrite
}

// BindExactQuery attaches Effect's catalog exact Query to this exact Factor
// owner. The hot projector is retained only in the shared SchemaBinding cell.
func BindExactQuery[R any](owner *HotOwner, query *engine.QuerySlot[R], spec engine.HotExactQuerySpec[factor.Value, R]) bool {
	if owner == nil || owner.binding == nil || owner.fragment == nil || query == nil {
		return false
	}
	return engine.BindExactQuery(owner.binding, query, owner.fragment.slot, spec)
}

// BindExactReadRule binds one heterogeneous exact predecessor and one exact
// Effect write. The predecessor Factor is supplied only as an opaque Ref; all
// output geometry remains fixed by this owner and the cold fragment.
func BindExactReadRule[O, RV any](owner *HotOwner, slot *engine.RuleSlot[factor.Value, O], readSlot engine.SchemaReadSlot[RV], readFactor engine.FactorRef[RV], write engine.SchemaWriteSlot[factor.Value], spec engine.HotRuleSpec[factor.Value, O]) (*RuleImplementation[O], engine.Read[engine.OrderedCells[RV]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[RV]]{}, false
	}
	read, ok := engine.BindRuleWithOpaqueExactRead[coordinate](owner.binding, slot, readSlot, readFactor, write, owner.fragment.Ref(), spec)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, read, true
}

// BeginSelectedRuleBinding starts Effect's exact-write/no-carry heterogeneous
// selected transaction. Carry-bearing Rules have a disjoint engine
// constructor and cannot enter this owner API.
func BeginSelectedRuleBinding[O any](owner *HotOwner, slot *engine.RuleSlot[factor.Value, O], write engine.SchemaWriteSlot[factor.Value], spec engine.HotRuleSpec[factor.Value, O]) (*SelectedRuleBinding[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	tx, ok := engine.BeginSelectedExactRuleBinding[coordinate](owner.binding, slot, write, owner.fragment.Ref(), spec)
	if !ok || tx == nil {
		return nil, false
	}
	return &SelectedRuleBinding[O]{owner: owner, tx: tx, issuer: &RuleImplementation[O]{owner: owner, slot: slot}}, true
}

func (tx *SelectedRuleBinding[O]) Implementation() (*RuleImplementation[O], bool) {
	if tx == nil || tx.owner == nil || tx.issuer == nil {
		return nil, false
	}
	return tx.issuer, true
}

// AddExactRead appends one exact predecessor in the cold Rule's canonical
// read order.
func AddExactRead[O, RV any](tx *SelectedRuleBinding[O], slot engine.SchemaReadSlot[RV], source engine.FactorRef[RV]) (engine.Read[engine.OrderedCells[RV]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.AddSelectedRouteExactRead(tx.tx, slot, source)
}

// AddOperandSelectedRead appends one selected predecessor whose exact routes
// are issued from the already-admitted operand receipt.
func AddOperandSelectedRead[O, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](tx *SelectedRuleBinding[O], slot engine.SchemaReadSlot[RV], source engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if tx == nil || tx.owner == nil || tx.tx == nil || locate == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.AddSelectedRouteOperandRead[coordinate, factor.Value, O, RV, Tag](tx.tx, slot, source, locate)
}

func CommitSelectedRuleBinding[O any](tx *SelectedRuleBinding[O]) bool {
	return tx != nil && tx.owner != nil && tx.tx != nil && engine.CommitSelectedRouteRuleBinding(tx.tx)
}

func AbortSelectedRuleBinding[O any](tx *SelectedRuleBinding[O]) bool {
	return tx != nil && tx.owner != nil && tx.tx != nil && engine.AbortSelectedRouteRuleBinding(tx.tx)
}

// ResolveRuleImplementation issues the engine receipt only after the exact
// shared SchemaBinding seals.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, factor.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.RuleImplementationAt[coordinate, factor.Value, O](issuer.owner.binding, issuer.slot)
}

func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, factor.Value, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

// Ref issues the exact Factor coordinate proof for one algebra-owned body
// root. Foreign roots, including roots from another same-Link algebra, fail
// closed at the algebra owner fence.
func (owner *HotOwner) Ref(root factor.Root) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || owner.algebra == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.algebra.RootIndex(root)
	if !ok || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, factor.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

// SelectRouteTyped emits one exact selected Effect root without exporting the
// private coordinate type.
func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, context engine.SelectorContext, root factor.Root, tag Tag) bool {
	ref, ok := owner.Ref(root)
	return ok && engine.SelectRoute(context, ref, tag)
}

// TargetMatches authenticates an exact staged Effect output.
func (owner *HotOwner) TargetMatches(target engine.RuleTarget, root factor.Root) bool {
	ref, ok := owner.Ref(root)
	return ok && engine.TargetMatchesRef(target, ref)
}

// ReadMatches authenticates one exact Effect read.
func ReadMatches[V, O, S any](owner *HotOwner, derivation engine.RuleDerivation[V, O], read engine.Read[S], root factor.Root) bool {
	ref, ok := owner.Ref(root)
	return ok && engine.DerivationReadMatchesRef(derivation, read, ref)
}

// SelectionMatches authenticates one selected Effect route and preserves its
// typed route tag.
func SelectionMatches[V, O, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, derivation engine.RuleDerivation[V, O], disposition engine.RuleDisposition[V], read engine.Read[engine.Selection[Tag, S]], ordinal int, root factor.Root) bool {
	ref, ok := owner.Ref(root)
	return ok && engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, read, ordinal, ref)
}

func (owner *HotOwner) admits(index coordinate, value factor.Value) bool {
	root, ok := owner.rootAt(index)
	return ok && owner.algebra.Admit(root, value)
}

func (owner *HotOwner) widenRank(index coordinate, value factor.Value, component int) uint64 {
	root, ok := owner.rootAt(index)
	if !ok {
		return 0
	}
	return owner.algebra.WidenRank(root, value, component)
}

func (owner *HotOwner) rootAt(index coordinate) (factor.Root, bool) {
	if owner == nil || owner.algebra == nil || uint64(index) >= uint64(owner.algebra.RootCount()) {
		return factor.Root{}, false
	}
	return owner.algebra.RootAt(int(index))
}

func effectBindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
