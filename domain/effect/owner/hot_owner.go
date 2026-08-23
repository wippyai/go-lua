package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/domain/effect/factor"
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
func (owner *HotOwner) LinkID() identity.ContentID {
	if owner == nil {
		return identity.ContentID{}
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

// BindHot attaches Effect's already-sealed Link-local algebra to its exact
// cold Factor slot. It never re-declares the Factor shape or retains a mutable
// FactorImplementation.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, algebra *factor.Algebra) (*HotOwner, bool) {
	if !effectBindingOpen(binding) || fragment == nil || algebra == nil || !algebra.Valid() || !validCoordinateCount(algebra.RootCount()) {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, algebra: algebra, linkOwner: algebra.LinkOwner()}
	spec, specOK := owner.FactorSpec()
	if !specOK || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Effect's exact Factor algebra for this binding: the same value
// BindHot hands to the engine. A declaration surface projects this record
// instead of restating the lattice, admission, or widening law, so the two
// cannot drift.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, factor.Value], bool) {
	if owner == nil || owner.algebra == nil || !owner.algebra.Valid() || !validCoordinateCount(owner.algebra.RootCount()) {
		return engine.HotFactorSpec[coordinate, factor.Value]{}, false
	}
	lattice, ok := owner.algebra.Lattice()
	if !ok {
		return engine.HotFactorSpec[coordinate, factor.Value]{}, false
	}
	return engine.HotFactorSpec[coordinate, factor.Value]{
		KeyEnd:      uint64(owner.algebra.RootCount()),
		Lattice:     lattice,
		Default:     owner.algebra.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, factor.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, true
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
func BindExactReadRule[O, RV any](owner *HotOwner, slot *engine.RuleSlot[factor.Value, O], readSlot engine.SchemaReadSlot[RV], readFactor engine.FactorRef[RV], write engine.SchemaWriteSlot[factor.Value], spec engine.HotRuleSpec[factor.Value, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], engine.Read[engine.OrderedCells[RV]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[RV]]{}, false
	}
	read, ok := engine.BindRuleWithOpaqueExactRead[coordinate](owner.binding, slot, readSlot, readFactor, write, owner.fragment.Ref(), spec, projectRead, projectWrite)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, read, true
}

// BindSelectedRuleDirect installs Effect's exact-write/no-carry selected Rule
// at its declared schema ordinal. The returned issuer fills each read at its
// own packed ordinal; no transaction, callback, or pending token crosses the
// Effect boundary.
func BindSelectedRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[factor.Value, O], write engine.SchemaWriteSlot[factor.Value], spec engine.HotRuleSpec[factor.Value, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindSelectedExactRuleDirect[coordinate](owner.binding, slot, write, owner.fragment.Ref(), spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// AddSelectedRuleDirectExactRead fills one exact predecessor at the
// SchemaReadSlot's packed ordinal. The slot, not call order, owns read
// position.
func AddSelectedRuleDirectExactRead[O, RV any](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], project func(O) (uint64, bool)) (engine.Read[engine.OrderedCells[RV]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.BindSelectedRuleDirectExactRead[coordinate](issuer.owner.binding, issuer.slot, slot, factor, project)
}

// AddSelectedRuleDirectOperandRead fills one operand-dependent selected
// predecessor at the SchemaReadSlot's packed ordinal. The locator remains in
// the sealed engine cell and receives the bound operand only during solving.
func AddSelectedRuleDirectOperandRead[O, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], source engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil || locate == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.BindSelectedRuleDirectOperandRead[coordinate, factor.Value, O, RV, Tag](issuer.owner.binding, issuer.slot, slot, source, locate)
}

// ResolveRuleImplementation issues the engine receipt only after the exact
// shared SchemaBinding seals.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, factor.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[coordinate, factor.Value, O](issuer.owner.binding, issuer.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
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
