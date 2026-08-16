package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/call"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// HotOwner is Call's Link-local Factor implementation.  The cold
// SchemaFragment remains the only structural input; the Call Algebra supplies
// the typed lattice, key admission, coordinate census, and widening law.
// Binding and the fragment are retained privately so no mutable engine handle
// or structural slot can escape this package.
type HotOwner struct {
	binding  *engine.SchemaBinding
	fragment *SchemaFragment
	algebra  *call.Algebra
}

// LinkOwner returns the exact detached Link witness captured at hot bind.
func (owner *HotOwner) LinkOwner() link.OwnerCapability {
	if owner == nil || owner.algebra == nil {
		return link.OwnerCapability{}
	}
	return owner.algebra.LinkOwner()
}

// LinkID returns the scalar identity carried by the authoritative owner.
func (owner *HotOwner) LinkID() identity.ContentID {
	return owner.LinkOwner().ContentID()
}

// RuleImplementation is a Call-owned pending Factor-output Rule receipt. The
// private coordinate and Factor slot remain behind this owner boundary.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[call.Value, O]
}

func (issuer *RuleImplementation[O]) MountedCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.MountedCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

// HeterogeneousRuleImplementation is the Call-output issuer for a Rule
// whose exact predecessor factor is owned by another domain. It preserves
// Call's output coordinate fence while retaining the typed input V.
type HeterogeneousRuleImplementation[RV, O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[call.Value, O]
}

// MountedCapability returns the parent-issued capability for this exact
// heterogeneous Call rule.  Dispatch uses this opaque capability when it
// mounts a rule row; keeping the lookup on the owner-issued implementation
// prevents dispatch from reaching into Call's binding or slot directly.
func (issuer *HeterogeneousRuleImplementation[RV, O]) MountedCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.MountedCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

// ActivationRuleImplementation is Call owner's opaque issuer for one
// receipt-native structural activation Rule.  It deliberately retains neither
// an engine slot nor a graph capability outside this owner package.
type ActivationRuleImplementation struct {
	owner *HotOwner
	slot  *engine.SchemaActivationRuleSlot
}

func (issuer *ActivationRuleImplementation) MountedCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.ActivationCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

func (owner *HotOwner) FactorRef() engine.FactorRef[call.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[call.Value]{}
	}
	return owner.fragment.Ref()
}

// BindHot admits Call's concrete Link-local algebra into the exact Factor slot
// and forms issued by fragment.  It performs no semantic restatement: the
// FactorSlot is the cold identity and algebra is the sole executable authority.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, algebra *call.Algebra) (*HotOwner, bool) {
	if binding == nil || fragment == nil || algebra == nil || !algebra.Valid() || !hotBindingOpen(binding) || !validCoordinateCount(algebra.KeyCount()) {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, algebra: algebra}
	spec, specOK := owner.FactorSpec()
	if !specOK || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Call's exact Factor algebra for this binding: the same value
// BindHot hands to the engine. A declaration surface projects this record
// instead of restating the lattice, admission, or widening law, so the two
// cannot drift.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, call.Value], bool) {
	if owner == nil || owner.algebra == nil || !owner.algebra.Valid() || !validCoordinateCount(owner.algebra.KeyCount()) {
		return engine.HotFactorSpec[coordinate, call.Value]{}, false
	}
	lattice, ok := owner.algebra.Lattice()
	if !ok {
		return engine.HotFactorSpec[coordinate, call.Value]{}, false
	}
	return engine.HotFactorSpec[coordinate, call.Value]{
		KeyEnd:      uint64(owner.algebra.KeyCount()),
		Lattice:     lattice,
		Default:     owner.algebra.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, call.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, true
}

// Algebra returns Call's exact Link-local semantic authority.
func (owner *HotOwner) Algebra() *call.Algebra {
	if owner == nil {
		return nil
	}
	return owner.algebra
}

// MatchesBinding proves exact hot transaction ownership without exposing the
// retained binding or any private Factor coordinate.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

// BindExactWriteRule binds one Call-output Rule through this owner's exact
// write surface. Child packages cannot replace the output Factor or recover
// Call's private coordinate type.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[call.Value, O], write engine.SchemaWriteSlot[call.Value], spec engine.HotRuleSpec[call.Value, O]) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[coordinate](owner.binding, slot, write, owner.fragment.slot, spec) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindExactReadRule binds a heterogeneous exact-read Rule while retaining
// Call's pending typed implementation for mounted receipt attachment.
func BindExactReadRule[O any](owner *HotOwner, slot *engine.RuleSlot[call.Value, O], read engine.SchemaReadSlot[call.Value], readFactor engine.FactorRef[call.Value], write engine.SchemaWriteSlot[call.Value], writeFactor engine.FactorRef[call.Value], spec engine.HotRuleSpec[call.Value, O]) (*RuleImplementation[O], engine.Read[engine.OrderedCells[call.Value]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[call.Value]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithOpaqueExactRead[coordinate](owner.binding, slot, read, readFactor, write, writeFactor, spec)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[call.Value]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, runtimeRead, true
}

func BindHeterogeneousExactReadRule[RV, O any](owner *HotOwner, slot *engine.RuleSlot[call.Value, O], read engine.SchemaReadSlot[RV], readFactor engine.FactorRef[RV], write engine.SchemaWriteSlot[call.Value], spec engine.HotRuleSpec[call.Value, O]) (*HeterogeneousRuleImplementation[RV, O], engine.Read[engine.OrderedCells[RV]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[RV]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithOpaqueExactRead[coordinate](owner.binding, slot, read, readFactor, write, owner.fragment.Ref(), spec)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return &HeterogeneousRuleImplementation[RV, O]{owner: owner, slot: slot}, runtimeRead, true
}

func ResolveHeterogeneousRuleImplementation[RV, O any](issuer *HeterogeneousRuleImplementation[RV, O]) (*engine.RuleImplementation[coordinate, call.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.RuleImplementationAt[coordinate, call.Value, O](issuer.owner.binding, issuer.slot)
}

// ResolveRuleImplementation issues the exact receipt after the shared
// SchemaBinding seals.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, call.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.RuleImplementationAt[coordinate, call.Value, O](issuer.owner.binding, issuer.slot)
}

// ExactRead returns the Factor form needed by a future Call Rule binder.
func (owner *HotOwner) ExactRead() engine.SchemaReadForm[call.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[call.Value]{}
	}
	return owner.fragment.exactRead
}

// ExactWrite returns the Factor form needed by a future Call Rule binder.
func (owner *HotOwner) ExactWrite() engine.SchemaWriteForm[call.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaWriteForm[call.Value]{}
	}
	return owner.fragment.exactWrite
}

// BindExactActivationRule attaches one Call-read activation callback to an
// exact cold activation Rule.  Child packages supply only their own cold
// rule/read proofs and callback; Call keeps the binding and private Factor
// cell required to bind the typed read.
func BindExactActivationRule(owner *HotOwner, slot *engine.SchemaActivationRuleSlot, read engine.SchemaReadSlot[call.Value], spec engine.HotActivationSpec) (*ActivationRuleImplementation, engine.Read[engine.OrderedCells[call.Value]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || owner.algebra == nil || !owner.algebra.Valid() || slot == nil {
		return nil, engine.Read[engine.OrderedCells[call.Value]]{}, false
	}
	runtimeRead, ok := engine.BindActivationRuleWithExactRead[coordinate, call.Value](owner.binding, slot, read, owner.fragment.slot, spec)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[call.Value]]{}, false
	}
	return &ActivationRuleImplementation{owner: owner, slot: slot}, runtimeRead, true
}

// BindMountedActivationCandidateIssuer binds the exact CallActivation slot
// to the five typed factor capabilities needed by its receipt-native body
// transport. The engine keeps the resulting issuer opaque; child packages
// cannot submit point/factor edge rows themselves.
func BindMountedActivationCandidateIssuer[V, C, H, P, E any](issuer *ActivationRuleImplementation, value engine.FactorRef[V], calls engine.FactorRef[C], heap engine.FactorRef[H], pack engine.FactorRef[P], effect engine.FactorRef[E]) (*engine.MountedActivationCandidateIssuer, bool) {
	if issuer == nil || issuer.owner == nil || issuer.owner.binding == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.BindMountedActivationCandidateIssuer(issuer.owner.binding, issuer.slot, value, calls, heap, pack, effect)
}

// ResolveActivationRuleImplementation issues the engine receipt only after
// the exact shared Binding seals.  Equal schema content from another Binding
// cannot cross the owner pointer fence.
func ResolveActivationRuleImplementation(issuer *ActivationRuleImplementation) (*engine.ActivationRuleImplementation, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	return engine.ActivationRuleImplementationAt(issuer.owner.binding, issuer.slot)
}

// ResolveActivationRuleImplementationFor additionally requires the exact
// Call HotOwner that issued the child receipt.
func ResolveActivationRuleImplementationFor(owner *HotOwner, issuer *ActivationRuleImplementation) (*engine.ActivationRuleImplementation, bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveActivationRuleImplementation(issuer)
}

// Ref issues one opaque Call source-key capability.  Implementation lookup
// is fresh and sealed on every call; HotOwner never retains a mutable or
// race-prone implementation snapshot.
func (owner *HotOwner) Ref(key call.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || owner.algebra == nil || !key.Valid() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.algebra.KeyIndex(key)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, call.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

// SelectRoute emits one exact Call route through this owner's sealed Factor
// receipt. It keeps Call's private carrier coordinate out of child binders.
func (owner *HotOwner) SelectRoute(context engine.SelectorContext, key call.Key, tag uint64) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, context engine.SelectorContext, key call.Key, tag Tag) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

// ReadMatches authenticates one exact Call predecessor against this owner's
// sealed key capability without exporting Call's private coordinate.
func ReadMatches[V, O, S any](owner *HotOwner, derivation engine.RuleDerivation[V, O], read engine.Read[S], key call.Key) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.DerivationReadMatchesRef(derivation, read, ref)
}

func (owner *HotOwner) admits(index coordinate, value call.Value) bool {
	if owner == nil || owner.algebra == nil || uint64(index) >= uint64(owner.algebra.KeyCount()) {
		return false
	}
	key, ok := owner.algebra.KeyAt(int(index))
	return ok && owner.algebra.Admits(key, value)
}

func (owner *HotOwner) widenRank(index coordinate, value call.Value, component int) uint64 {
	if owner == nil || owner.algebra == nil {
		return 0
	}
	key, ok := owner.algebra.KeyAt(int(index))
	if !ok || component != 0 {
		return 0
	}
	rank, ok := owner.algebra.WidenRank(key)
	if !ok {
		return 0
	}
	measure, ok := rank.At(value, component)
	if !ok {
		return 0
	}
	return measure
}

func hotBindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
