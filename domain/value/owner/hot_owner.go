package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/value"
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

// BindSelectedRuleDirect installs Value's exact-write selected Rule cell at
// its declared ordinal. The returned issuer fills each cold read ordinal;
// no transaction handle or pending token crosses the Value boundary.
func BindSelectedRuleDirect[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], carry engine.SchemaCarrySlot[value.Value], write engine.SchemaWriteSlot[value.Value], output engine.FactorRef[value.Value], spec engine.HotRuleSpec[value.Value, O], carrySpec engine.HotCarrySpec[value.Value, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindSelectedRuleDirect[value.DenseCoordinate](owner.binding, slot, carry, write, output, spec, carrySpec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// AddSelectedRuleDirectExactRead fills the direct Rule's exact read at the
// SchemaReadSlot's packed ordinal. The slot itself, rather than append order,
// is the only read-position authority.
func AddSelectedRuleDirectExactRead[O any, RV any](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], project func(O) (uint64, bool)) (engine.Read[engine.OrderedCells[RV]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.BindSelectedRuleDirectExactRead[value.DenseCoordinate](issuer.owner.binding, issuer.slot, slot, factor, project)
}

// AddSelectedRuleDirectOperandRead installs an operand-dependent selector at
// the read's declared cold ordinal. The operand is supplied only when the
// sealed Rule executes; it is never captured in construction state.
func AddSelectedRuleDirectOperandRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](issuer *RuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.BindSelectedRuleDirectOperandRead[value.DenseCoordinate, value.Value, O, RV, Tag](issuer.owner.binding, issuer.slot, slot, factor, locate)
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
	if !specOK || !engine.BindFactor[value.DenseCoordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[value.DenseCoordinate, value.Value](binding, fragment.slot, fragment.summaryRead) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[value.DenseCoordinate, value.Value](binding, fragment.slot, fragment.foldRead) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Value's exact Factor algebra for this binding: the same value
// BindHot hands to the engine. A declaration surface projects this record
// instead of restating the lattice, admission, or widening law, so the two
// cannot drift.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[value.DenseCoordinate, value.Value], bool) {
	if owner == nil || owner.schema == nil {
		return engine.HotFactorSpec[value.DenseCoordinate, value.Value]{}, false
	}
	keyEnd := owner.schema.CoordinateCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[value.DenseCoordinate, value.Value]{}, false
	}
	return engine.HotFactorSpec[value.DenseCoordinate, value.Value]{
		KeyEnd:      uint64(keyEnd),
		Lattice:     owner.schema.Domain(),
		Default:     owner.schema.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.schema.Fingerprint,
		WidenRank: engine.Measure[value.DenseCoordinate, value.Value]{
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

func (owner *HotOwner) implementationAt() (*engine.FactorImplementation[value.DenseCoordinate, value.Value], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil {
		return nil, false
	}
	implementation, ok := engine.FactorImplementationAt[value.DenseCoordinate, value.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

// BindExactWriteRule binds one Value-output Rule through this owner's exact
// Factor slot. The child package supplies its own typed Rule slot, write
// slot, operand admission, and transfer implementation; it cannot supply a
// different output Factor or recover Value's coordinate type.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], write engine.SchemaWriteSlot[value.Value], spec engine.HotRuleSpec[value.Value, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[value.DenseCoordinate](owner.binding, slot, write, owner.fragment.slot, spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindSelectedRouteCarryRule binds Value's routed allocation issuance. The
// child selects only owner-issued Value coordinates; one fresh fact is then
// written atomically to the complete route image.
func BindSelectedRouteCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], carry engine.SchemaCarrySlot[value.Value], write engine.SchemaWriteSlot[value.Value], spec engine.HotRuleSpec[value.Value, O], carrySpec engine.HotCarrySpec[value.Value, O]) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindSelectedRouteRuleDirect[value.DenseCoordinate](owner.binding, slot, carry, write, owner.fragment.Ref(), spec, carrySpec, nil) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// BindExactReadAndCarryRule binds Value's one exact-read/one carry/one exact
// write lane through the same sealed Factor authority. The child receives the
// typed read capability required by its transfer and evidence checker, but it
// cannot recover Value's private coordinate or Factor slot.
func BindExactReadAndCarryRule[O any](owner *HotOwner, slot *engine.RuleSlot[value.Value, O], read engine.SchemaReadSlot[value.Value], carry engine.SchemaCarrySlot[value.Value], write engine.SchemaWriteSlot[value.Value], spec engine.HotRuleSpec[value.Value, O], carrySpec engine.HotCarrySpec[value.Value, O], projectRead func(O) (uint64, bool), projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], engine.Read[engine.OrderedCells[value.Value]], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, engine.Read[engine.OrderedCells[value.Value]]{}, false
	}
	runtimeRead, ok := engine.BindRuleWithExactReadAndCarry[value.DenseCoordinate, value.Value, O, value.DenseCoordinate, value.Value](owner.binding, slot, read, owner.fragment.slot, carry, write, owner.fragment.slot, spec, carrySpec, projectRead, projectWrite)
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

// ResolveRuleImplementation issues the owner-fenced sealed Rule row. The
// operand resolver was admitted into the cell before SchemaBinding.Seal; no
// post-seal mutation or lazy installation is possible.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[value.DenseCoordinate, value.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[value.DenseCoordinate, value.Value, O](issuer.owner.binding, issuer.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

// ResolveRuleImplementationFor is the explicit owner fence used by child
// binders that retain more than one equal-Schema HotOwner. A receipt issued
// by another binding is rejected before engine resolution.
func ResolveRuleImplementationFor[O any](owner *HotOwner, issuer *RuleImplementation[O]) (*engine.RuleImplementation[value.DenseCoordinate, value.Value, O], bool) {
	if owner == nil || issuer == nil || issuer.owner != owner {
		return nil, false
	}
	return ResolveRuleImplementation(issuer)
}

// Ref issues the exact Value key capability after binding publication. The
// caller supplies a Value schema coordinate, never a dense engine coordinate.
func (owner *HotOwner) Ref(location value.Coordinate) (engine.Ref[value.DenseCoordinate], bool) {
	implementation, ok := owner.implementationAt()
	if !ok || owner.schema == nil {
		return engine.Ref[value.DenseCoordinate]{}, false
	}
	index, ok := owner.schema.CoordinateIndex(location)
	if !ok || uint64(index) >= uint64(owner.schema.CoordinateCount()) {
		return engine.Ref[value.DenseCoordinate]{}, false
	}
	return implementation.Ref(value.DenseCoordinate(index))
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

func (owner *HotOwner) admits(index value.DenseCoordinate, fact value.Value) bool {
	if owner == nil || owner.schema == nil {
		return false
	}
	location, ok := owner.schema.CoordinateAt(int(index))
	return ok && owner.schema.AdmitsCoordinate(location, fact)
}

func (owner *HotOwner) widenRank(index value.DenseCoordinate, fact value.Value, component int) uint64 {
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
