package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/domain/pack"
)

// HotOwner is Pack's opaque Link-local Factor implementation. Its only cold
// input is the exact SchemaFragment; the Pack Schema supplies the concrete
// admission, lattice, key universe, and widening law. No mutable coordinate
// snapshot or legacy Composition owner is retained.
type HotOwner struct {
	binding   *engine.SchemaBinding
	fragment  *SchemaFragment
	schema    *pack.Schema
	linkOwner link.OwnerCapability
}

// LinkOwner returns the exact detached Link witness captured at hot bind.
func (owner *HotOwner) LinkOwner() link.OwnerCapability {
	if owner == nil {
		return link.OwnerCapability{}
	}
	return owner.linkOwner
}

// MatchesBinding proves that this owner belongs to the exact shared hot
// transaction. Equal Schema contents from another Binding are not sufficient.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

func (owner *HotOwner) FactorRef() engine.FactorRef[pack.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[pack.Value]{}
	}
	return owner.fragment.Ref()
}

// RuleImplementation is a Pack-owned pending receipt issuer. It retains the
// exact child Rule slot without exposing Pack's private Factor coordinate.
type RuleImplementation[O any] struct {
	owner *HotOwner
	slot  *engine.RuleSlot[pack.Value, O]
}

func (issuer *RuleImplementation[O]) MountedCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.MountedCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

// BindExactWriteRule binds one Pack-output Rule through this owner's exact
// Factor slot. Child packages provide only their typed operand callbacks and
// transfer; they cannot choose another output Factor or coordinate universe.
func BindExactWriteRule[O any](owner *HotOwner, slot *engine.RuleSlot[pack.Value, O], write engine.SchemaWriteSlot[pack.Value], spec engine.HotRuleSpec[pack.Value, O], projectWrite func(O) (uint64, bool)) (*RuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil {
		return nil, false
	}
	if !engine.BindRule[coordinate](owner.binding, slot, write, owner.fragment.slot, spec, projectWrite) {
		return nil, false
	}
	return &RuleImplementation[O]{owner: owner, slot: slot}, true
}

// ResolveRuleImplementation issues the exact shared receipt only after the
// SchemaBinding seals. The private coordinate remains package-owned.
func ResolveRuleImplementation[O any](issuer *RuleImplementation[O]) (*engine.RuleImplementation[coordinate, pack.Value, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[coordinate, pack.Value, O](issuer.owner.binding, issuer.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

// BindHot admits Pack's concrete Factor algebra into the exact shared
// SchemaBinding. The fragment's private slot and exact forms are checked
// before binding; the resulting HotOwner exposes only typed capability and
// proof operations.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, schema *pack.Schema) (*HotOwner, bool) {
	if binding == nil || fragment == nil || !validPackSchema(schema) || !bindingOpen(binding) ||
		fragment.slot == nil || fragment.exactRead.Kind() != engine.SchemaFormReadExact ||
		fragment.exactWrite.Kind() != engine.SchemaFormWriteExact ||
		fragment.exactRead.Schema() == nil || fragment.exactRead.Schema() != fragment.slot.Schema() ||
		fragment.exactWrite.Schema() != fragment.slot.Schema() {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, schema: schema, linkOwner: schema.LinkOwner()}
	spec, specOK := owner.FactorSpec()
	if !specOK || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	return owner, true
}

// FactorSpec is Pack's exact Factor algebra for this binding: the same value
// BindHot hands to the engine. A declaration surface projects this record
// instead of restating the lattice, admission, or widening law, so the two
// cannot drift.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, pack.Value], bool) {
	if owner == nil || !validPackSchema(owner.schema) {
		return engine.HotFactorSpec[coordinate, pack.Value]{}, false
	}
	rootCount := owner.schema.RootCount()
	if rootCount < 0 || uint64(rootCount) > uint64(^uint32(0))+1 {
		return engine.HotFactorSpec[coordinate, pack.Value]{}, false
	}
	return engine.HotFactorSpec[coordinate, pack.Value]{
		KeyEnd:      uint64(rootCount),
		Lattice:     owner.schema.Lattice(),
		Default:     owner.schema.Bottom(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.schema.Fingerprint,
		WidenRank: engine.Measure[coordinate, pack.Value]{
			Width: 4,
			At:    owner.widenRank,
		},
	}, true
}

// implementation obtains a fresh sealed receipt each time. The receipt is
// immutable and authority-fenced by SchemaBinding; no cached mutable engine
// snapshot is retained by HotOwner.
func (owner *HotOwner) implementation() (*engine.FactorImplementation[coordinate, pack.Value], bool) {
	if owner == nil || !validPackSchema(owner.schema) || owner.binding == nil || owner.fragment == nil || owner.fragment.slot == nil {
		return nil, false
	}
	return engine.FactorImplementationAt[coordinate, pack.Value](owner.binding, owner.fragment.slot)
}

// Ref issues Pack's exact Factor capability for one Schema-local root.
func (owner *HotOwner) Ref(root pack.Root) (engine.Ref[coordinate], bool) {
	implementation, ok := owner.implementation()
	if !ok || owner.schema == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.schema.RootOrder(root)
	if !ok || index < 0 || uint64(index) >= uint64(owner.schema.RootCount()) {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

// SelectRoute emits one exact Pack payload route through the owner-issued
// capability and never exposes Pack's private carrier coordinate.
func (owner *HotOwner) SelectRoute(context engine.SelectorContext, root pack.Root, tag uint64) bool {
	ref, ok := owner.Ref(root)
	return ok && engine.SelectRoute(context, ref, tag)
}

// SelectRouteTyped preserves the exact Pack payload-tag type at the staged
// sink while retaining Pack's private coordinate authority.
func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *HotOwner, context engine.SelectorContext, root pack.Root, tag Tag) bool {
	ref, ok := owner.Ref(root)
	return ok && engine.SelectRoute(context, ref, tag)
}

// OwnsSchema authenticates a typed child binder's cold Pack schema without
// exposing the retained schema or any broad authority accessor.
func (owner *HotOwner) OwnsSchema(schema *pack.Schema) bool {
	return owner != nil && owner.schema == schema && validPackSchema(schema)
}

func (owner *HotOwner) admits(index coordinate, fact pack.Value) bool {
	if owner == nil || !validPackSchema(owner.schema) {
		return false
	}
	root, ok := owner.schema.RootAt(int(index))
	return ok && owner.schema.Admit(root, fact)
}

func (owner *HotOwner) widenRank(index coordinate, fact pack.Value, component int) uint64 {
	if owner == nil || !validPackSchema(owner.schema) {
		return 0
	}
	root, ok := owner.schema.RootAt(int(index))
	return func() uint64 {
		if !ok {
			return 0
		}
		return owner.schema.At(root, fact, component)
	}()
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}

// validPackSchema is the public-owner fence for Pack's sealed schema. Pack
// intentionally has no exported Valid method; the detached owner capability
// is the sealed witness available at this package layer. In particular, this
// rejects &pack.Schema{} before Lattice or any callback can dereference its
// private state.
func validPackSchema(schema *pack.Schema) bool {
	if schema == nil {
		return false
	}
	if !schema.LinkOwner().Available() {
		return false
	}
	count := schema.RootCount()
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
