package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// HotOwner binds Static's sealed Runtime rows to Value's existing coordinate
// denominator. It neither retains Program data nor creates another geometry.
type HotOwner struct {
	binding  *engine.SchemaBinding
	fragment *SchemaFragment
	static   *staticdomain.Authority
	values   *valuedomain.Schema
	classes  *staticdomain.ClassSet
}

// RuleImplementation is Static's owner-fenced pending Rule issuer.  The
// Static carrier coordinate remains private to this package; a rule receives
// only the typed issuer and the read capabilities it declared.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[staticdomain.TypeFact, O]
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

// BindSeedRuleDirect installs a Static exact-write selected Rule at the
// cold slot's declared ordinal. The child supplies only its operand and Fold;
// Static's Factor cell and private carrier coordinate stay owner-authenticated.
func BindSeedRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[staticdomain.TypeFact, O], write engine.SchemaWriteSlot[staticdomain.TypeFact], spec engine.HotRuleSpec[staticdomain.TypeFact, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindSelectedExactRuleDirect[coordinate](owner.binding, slot, write, owner.fragment.Ref(), spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// AddSelectedRuleDirectExactRead binds one heterogeneous exact predecessor to
// the cold read ordinal declared by a selected Static Rule. It is the Static
// analogue of the other Factor owners' direct-read seam: the child never
// receives Static's private dense coordinate type.
func AddSelectedRuleDirectExactRead[O any, RV any](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], project func(O) (uint64, bool)) (engine.Read[engine.OrderedCells[RV]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.BindSelectedRuleDirectExactRead[coordinate](issuer.owner.binding, issuer.slot, slot, factor, project)
}

// BindCarryRuleDirect installs a Static-output exact-write Rule through
// this owner's Factor slot. The child rule supplies only its operand and
// transfer behavior; Static retains sole authority over the output factor and
// its dense coordinate carrier.
func BindCarryRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[staticdomain.TypeFact, O], carry engine.SchemaCarrySlot[staticdomain.TypeFact], write engine.SchemaWriteSlot[staticdomain.TypeFact], output engine.FactorRef[staticdomain.TypeFact], spec engine.HotRuleSpec[staticdomain.TypeFact, O], carrySpec engine.HotCarrySpec[staticdomain.TypeFact, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil || output != owner.fragment.Ref() {
		return nil, false
	}
	if !engine.BindSelectedRuleDirect[coordinate](owner.binding, slot, carry, write, output, spec, carrySpec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindExactReadAndCarryRule binds one Static exact-read/ordinary-carry/exact-
// write lane through this owner's Factor. The child contributes only its
// typed operand behavior and endpoint projections; it cannot select another
// output Factor or recover Static's private coordinate type.
func BindExactReadAndCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[staticdomain.TypeFact, O], read engine.SchemaReadSlot[staticdomain.TypeFact], carry engine.SchemaCarrySlot[staticdomain.TypeFact], write engine.SchemaWriteSlot[staticdomain.TypeFact], spec engine.HotRuleSpec[staticdomain.TypeFact, O], carrySpec engine.HotCarrySpec[staticdomain.TypeFact, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], engine.Read[engine.OrderedCells[staticdomain.TypeFact]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[staticdomain.TypeFact]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithExactReadAndCarry[coordinate, staticdomain.TypeFact, O, coordinate, staticdomain.TypeFact](owner.binding, slot, read, owner.fragment.slot, carry, write, owner.fragment.slot, spec, carrySpec, projectRead, projectWrite)
	if !ok {
		return nil, engine.Read[engine.OrderedCells[staticdomain.TypeFact]]{}, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, runtimeRead, true
}

// ResolveRuleImplementation issues Static's sealed engine receipt only after
// the shared binding seals. No rule package can recover Static's coordinate
// or factor slot from this accessor.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, staticdomain.TypeFact, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[coordinate, staticdomain.TypeFact, O](issuer.owner.binding, issuer.slot)
	return implementation, ok
}

// ResolveRuleImplementationFor is the explicit owner fence used when a rule
// retains more than one equal-looking Static owner. An issuer from another
// binding must never cross into this one.
func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, staticdomain.TypeFact, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, statics *staticdomain.Authority, values *valuedomain.Schema) (*HotOwner, bool) {
	if !bindingOpen(binding) || fragment == nil || statics == nil || values == nil || !statics.LinkID().Available() || statics.LinkID() != values.LinkOwner().ContentID() || values.CoordinateCount() == 0 {
		return nil, false
	}
	classes := statics.Classes()
	if classes == nil {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, static: statics, values: values, classes: classes}
	spec, ok := owner.FactorSpec()
	if !ok || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	return owner, true
}

func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, staticdomain.TypeFact], bool) {
	if owner == nil || owner.classes == nil || owner.values == nil {
		return engine.HotFactorSpec[coordinate, staticdomain.TypeFact]{}, false
	}
	count := owner.values.CoordinateCount()
	if count <= 0 || uint64(count) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[coordinate, staticdomain.TypeFact]{}, false
	}
	return engine.HotFactorSpec[coordinate, staticdomain.TypeFact]{
		KeyEnd:      uint64(count),
		Lattice:     owner.classes.TypeFactLattice(),
		Default:     owner.classes.TypeBottom(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.classes.TypeFactFingerprint,
		WidenRank:   engine.Measure[coordinate, staticdomain.TypeFact]{Width: 1, At: owner.widenRank},
	}, true
}

func (owner *HotOwner) Classes() *staticdomain.ClassSet {
	if owner == nil {
		return nil
	}
	return owner.classes
}

func (owner *HotOwner) StaticAuthority() *staticdomain.Authority {
	if owner == nil {
		return nil
	}
	return owner.static
}

func (owner *HotOwner) ValueSchema() *valuedomain.Schema {
	if owner == nil {
		return nil
	}
	return owner.values
}

func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding == binding
}

func (owner *HotOwner) LinkID() identity.ContentID {
	if owner == nil || owner.static == nil {
		return identity.ContentID{}
	}
	return owner.static.LinkID()
}

func (owner *HotOwner) FactorRef() engine.FactorRef[staticdomain.TypeFact] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[staticdomain.TypeFact]{}
	}
	return owner.fragment.Ref()
}

func (owner *HotOwner) ExactRead() engine.SchemaReadForm[staticdomain.TypeFact] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[staticdomain.TypeFact]{}
	}
	return owner.fragment.ExactRead()
}

func (owner *HotOwner) ExactWrite() engine.SchemaWriteForm[staticdomain.TypeFact] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaWriteForm[staticdomain.TypeFact]{}
	}
	return owner.fragment.ExactWrite()
}

func (owner *HotOwner) Ref(location valuedomain.Coordinate) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || owner.values == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.values.CoordinateIndex(location)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, staticdomain.TypeFact](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

func (owner *HotOwner) admits(at coordinate, fact staticdomain.TypeFact) bool {
	return owner != nil && owner.classes != nil && owner.values != nil && uint64(at) < uint64(owner.values.CoordinateCount()) && owner.classes.OwnsTypeFact(fact)
}

func (owner *HotOwner) widenRank(at coordinate, fact staticdomain.TypeFact, component int) uint64 {
	if component != 0 || !owner.admits(at, fact) {
		return 0
	}
	return owner.classes.TypeFactWidenRank(fact)
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
