package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Declaration-time rule slot capability machinery; the admission transaction
// that consumes these capabilities lives in runtime_rule_admit.go.

// RuleSlotCapability is the parent-issued identity of one rule slot.  The
// engine deliberately has no domain-role vocabulary: a capability is tied to
// the exact SchemaBinding authority and slot ordinal that issued it.  The
// artifact/analysis owner binds a sealed declaration Key to this opaque
// capability.
type RuleSlotCapability struct {
	state      *schemaBindingState
	authority  *schemaBindingAuthority
	ordinal    uint64
	kind       ruleCapabilityKind
	activation bool
}

type ruleCapabilityKind uint8

const (
	ruleCapabilityInvalid ruleCapabilityKind = iota
	ruleCapabilityMounted
	ruleCapabilityLink
	ruleCapabilityMountedPoint
)

func (capability RuleSlotCapability) available() bool {
	return capability.state != nil && capability.authority != nil && capability.kind != ruleCapabilityInvalid && capability.ordinal != ^uint64(0) && capability.state.authority == capability.authority
}

func (capability RuleSlotCapability) Available() bool { return capability.available() }

func (capability RuleSlotCapability) Mounted() bool { return capability.mounted() }

func (capability RuleSlotCapability) Link() bool { return capability.link() }

func (capability RuleSlotCapability) MountedPoint() bool { return capability.mountedPoint() }

func (capability RuleSlotCapability) Activation() bool {
	return capability.mounted() && capability.activation
}

func (capability RuleSlotCapability) mounted() bool {
	return capability.available() && capability.kind == ruleCapabilityMounted
}

func (capability RuleSlotCapability) link() bool {
	return capability.available() && capability.kind == ruleCapabilityLink
}

func (capability RuleSlotCapability) mountedPoint() bool {
	return capability.available() && capability.kind == ruleCapabilityMountedPoint
}

// mountedLane reports whether this capability is issued on one of the two
// artifact-addressed lanes: an ordinary mounted row, or a mounted-point row
// the engine expands over the sealed Point plane.
func (capability RuleSlotCapability) mountedLane() bool {
	return capability.mounted() || capability.mountedPoint()
}

// IssueMountedRuleCapability issues an opaque capability for an ordinary
// mounted Rule slot.  It is intentionally issued by the schema boundary, not
// from a domain enum or caller-supplied code.
func IssueMountedRuleCapability[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return issueRuleSlotCapability(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMounted, false)
}

// IssueMountedGeneratedRuleCapability issues the mounted lane capability for
// a Plan-generated Rule. Generated slots have a separate public type so they
// cannot be laundered into the legacy typed Rule binders.
func IssueMountedGeneratedRuleCapability(binding *SchemaBinding, slot *GeneratedRuleSlot) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil || slot.cell.generated == nil {
		return RuleSlotCapability{}, false
	}
	return issueRuleSlotCapability(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMounted, false)
}

// IssueActivationRuleCapability issues the structural mounted capability for
// an activation slot.  Activation is slot geometry, not an engine role.
func IssueActivationRuleCapability(binding *SchemaBinding, slot *SchemaActivationRuleSlot) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return issueRuleSlotCapability(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMounted, true)
}

// IssueLinkRuleCapability issues the mount-neutral capability for a Link
// bootstrap slot.  There is no separate Link role namespace in the engine.
func IssueLinkRuleCapability[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return issueRuleSlotCapability(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityLink, false)
}

// IssueMountedPointRuleCapability issues the artifact-independent capability
// whose member is instantiated once at every mounted Point.
func IssueMountedPointRuleCapability[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return issueRuleSlotCapability(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMountedPoint, false)
}

func issueRuleSlotCapability(binding *SchemaBinding, schema *Schema, ordinal uint64, kind ruleCapabilityKind, activation bool) (RuleSlotCapability, bool) {
	state := bindingState(binding)
	if state == nil || state.phase != schemaBindingOpen || state.authority == nil {
		return RuleSlotCapability{}, false
	}
	if schema == nil || schema != state.schema {
		return RuleSlotCapability{}, false
	}
	return RuleSlotCapability{state: state, authority: state.authority, ordinal: ordinal, kind: kind, activation: activation}, true
}

// RegisterRuleSlot is the pre-seal owner bridge.  The parent supplies the
// capability previously issued by this exact SchemaBinding; the engine never
// interprets a domain role or numeric role code.
func RegisterRuleSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O], capability RuleSlotCapability) bool {
	if slot == nil || slot.cell == nil {
		return false
	}
	return registerRuleSlot(binding, slot.cell.schema, slot.cell.ordinal, capability)
}

// RegisterGeneratedRuleSlot is the pre-seal handoff for a generated slot. It
// shares the opaque capability directory with legacy lanes but never accepts
// a typed RuleSlot or a RuleImplementation.
func RegisterGeneratedRuleSlot(binding *SchemaBinding, slot *GeneratedRuleSlot, capability RuleSlotCapability) bool {
	if slot == nil || slot.cell == nil || slot.cell.generated == nil {
		return false
	}
	return registerRuleSlot(binding, slot.cell.schema, slot.cell.ordinal, capability)
}

// RegisterMountedGeneratedSlot issues and registers one generated mounted
// lane in a single open-binding handoff.
func RegisterMountedGeneratedSlot(binding *SchemaBinding, slot *GeneratedRuleSlot) (RuleSlotCapability, bool) {
	capability, ok := IssueMountedGeneratedRuleCapability(binding, slot)
	if !ok || !RegisterGeneratedRuleSlot(binding, slot, capability) {
		return RuleSlotCapability{}, false
	}
	return capability, true
}

// RegisterMountedSlot issues the mounted capability and registers the slot
// against it in one pre-seal handoff.
func RegisterMountedSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	capability, ok := IssueMountedRuleCapability(binding, slot)
	if !ok || !RegisterRuleSlot(binding, slot, capability) {
		return RuleSlotCapability{}, false
	}
	return capability, true
}

// RegisterLinkSlot is the Link-lane handoff.
func RegisterLinkSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	capability, ok := IssueLinkRuleCapability(binding, slot)
	if !ok || !RegisterRuleSlot(binding, slot, capability) {
		return RuleSlotCapability{}, false
	}
	return capability, true
}

// RegisterMountedPointSlot is the mounted-point closure handoff.
func RegisterMountedPointSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	capability, ok := IssueMountedPointRuleCapability(binding, slot)
	if !ok || !RegisterRuleSlot(binding, slot, capability) {
		return RuleSlotCapability{}, false
	}
	return capability, true
}

func RegisterActivationRuleSlot(binding *SchemaBinding, slot *SchemaActivationRuleSlot, capability RuleSlotCapability) bool {
	if slot == nil || slot.cell == nil {
		return false
	}
	return registerRuleSlot(binding, slot.cell.schema, slot.cell.ordinal, capability)
}

// RegisterLinkBootstrapTransports is the one pre-seal authorization for the
// complete factor set allowed to leave the Link-global bootstrap point.
// Ordering is owner-significant and retained exactly; a same-Binding Link
// capability is not sufficient unless it occupies its registered position.
func RegisterLinkBootstrapTransports(binding *SchemaBinding, capabilities ...RuleSlotCapability) bool {
	state := bindingState(binding)
	if state == nil || len(capabilities) == 0 {
		return false
	}
	for _, capability := range capabilities {
		if !capability.link() || capability.state != state || capability.authority != state.authority {
			return false
		}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.linkBootstrapTransportSet {
		return false
	}
	state.linkBootstrapTransports = append([]RuleSlotCapability(nil), capabilities...)
	state.linkBootstrapTransportSet = true
	if !completeLinkBootstrapTransportsLocked(state) {
		state.linkBootstrapTransports = nil
		state.linkBootstrapTransportSet = false
		return false
	}
	return true
}

func sealedLinkBootstrapTransports(state *schemaBindingState) ([]RuleSlotCapability, bool) {
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || !completeLinkBootstrapTransportsLocked(state) {
		return nil, false
	}
	return append([]RuleSlotCapability(nil), state.linkBootstrapTransports...), true
}

// BindingRuleSlot resolves one runtime Rule semantic through the exact sealed
// role directory. It exists only for closed diagnostics: callers receive a
// role enum, never a Schema ordinal, callback, or mutable binding row.
func BindingRuleSlot(binding *SchemaBinding, semantic identity.SemanticKey) (RuleSlotCapability, bool) {
	state := bindingState(binding)
	if state == nil || !semantic.Available() {
		return RuleSlotCapability{}, false
	}
	key := compositionKeyOf(semantic)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed {
		return RuleSlotCapability{}, false
	}
	ordinal, found := state.schema.ruleOrdinalOf(key)
	if !found {
		return RuleSlotCapability{}, false
	}
	if role, candidate, found := lookupRuleSlotCapabilityLocked(state, ordinal, ruleCapabilityMounted, false); found && candidate == key {
		return role, true
	}
	if role, candidate, found := lookupRuleSlotCapabilityLocked(state, ordinal, ruleCapabilityMounted, true); found && candidate == key {
		return role, true
	}
	if role, candidate, found := lookupRuleSlotCapabilityLocked(state, ordinal, ruleCapabilityLink, false); found && candidate == key {
		return role, true
	}
	if role, candidate, found := lookupRuleSlotCapabilityLocked(state, ordinal, ruleCapabilityMountedPoint, false); found && candidate == key {
		return role, true
	}
	return RuleSlotCapability{}, false
}

// MountedCapabilityForSlot returns the parent-issued capability registered
// for this exact ordinary Rule slot.  It is the post-seal owner lookup used by
// domain adapters; no semantic role name crosses this boundary.
func MountedCapabilityForSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return capabilityForSlot(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMounted, false)
}

// MountedGeneratedCapabilityForSlot recovers the registered generated
// mounted capability after the Binding seals.
func MountedGeneratedCapabilityForSlot(binding *SchemaBinding, slot *GeneratedRuleSlot) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil || slot.cell.generated == nil {
		return RuleSlotCapability{}, false
	}
	return capabilityForSlot(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMounted, false)
}

// ActivationCapabilityForSlot is the structural counterpart for an
// activation slot.
func ActivationCapabilityForSlot(binding *SchemaBinding, slot *SchemaActivationRuleSlot) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return capabilityForSlot(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMounted, true)
}

func LinkCapabilityForSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return capabilityForSlot(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityLink, false)
}

func MountedPointCapabilityForSlot[V, O any](binding *SchemaBinding, slot *RuleSlot[V, O]) (RuleSlotCapability, bool) {
	if slot == nil || slot.cell == nil {
		return RuleSlotCapability{}, false
	}
	return capabilityForSlot(binding, slot.cell.schema, slot.cell.ordinal, ruleCapabilityMountedPoint, false)
}

func capabilityForSlot(binding *SchemaBinding, schema *Schema, ordinal uint64, kind ruleCapabilityKind, activation bool) (RuleSlotCapability, bool) {
	state := bindingState(binding)
	if state == nil || schema == nil || state.schema != schema || state.phase != schemaBindingSealed {
		return RuleSlotCapability{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	capability, _, found := lookupRuleSlotCapabilityLocked(state, ordinal, kind, activation)
	return capability, found
}

// lookupRuleSlotCapabilityLocked probes the exact opaque capability key. The
// caller must hold state.mu; capability kind is a small closed set, so callers
// needing a semantic reverse lookup can probe it without walking the directory.
func lookupRuleSlotCapabilityLocked(state *schemaBindingState, ordinal uint64, kind ruleCapabilityKind, activation bool) (RuleSlotCapability, composition.Key, bool) {
	if state == nil || state.authority == nil || state.roleSlots == nil || kind == ruleCapabilityInvalid || (activation && kind != ruleCapabilityMounted) {
		return RuleSlotCapability{}, composition.Key{}, false
	}
	capability := RuleSlotCapability{state: state, authority: state.authority, ordinal: ordinal, kind: kind, activation: activation}
	semantic, found := state.roleSlots[capability]
	return capability, semantic, found
}

func registerRuleSlot(binding *SchemaBinding, schema *Schema, ordinal uint64, capability RuleSlotCapability) bool {
	if binding == nil || binding.state == nil || schema == nil || binding.state.schema != schema || !capability.available() || capability.state != binding.state || capability.authority != binding.state.authority {
		return false
	}
	semantic := schema.ruleSemanticAt(ordinal)
	if !semantic.Available() {
		return false
	}
	state := binding.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.roleSlots == nil {
		return false
	}
	if capability.ordinal != ordinal || capability.kind == ruleCapabilityInvalid || (capability.activation && capability.kind != ruleCapabilityMounted) {
		return false
	}
	if _, exists := state.roleSlots[capability]; exists {
		return false
	}
	state.roleSlots[capability] = semantic
	return true
}

func completeCapabilityDirectory(state *schemaBindingState) bool {
	return state != nil && len(state.roleSlots) != 0
}

// resolveSealedRuleCell authenticates a parent-issued capability and returns
// the exact canonical sealed schema cell at its ordinal. The capability's
// state, authority, role-directory entry, and sealed row all have to agree;
// callers never reconstruct a RuleImplementation or carry a callback-bearing
// wrapper across the construction boundary.
func resolveSealedRuleCell(capability RuleSlotCapability) (sealedRuleCell, bool) {
	state := capability.state
	if state == nil || capability.authority == nil || !capability.available() {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority != capability.authority || state.roleSlots == nil || state.rules == nil {
		return nil, false
	}
	if _, registered := state.roleSlots[capability]; !registered || capability.ordinal >= uint64(len(state.rules)) {
		return nil, false
	}
	raw := state.rules[capability.ordinal]
	if raw == nil {
		return nil, false
	}
	if capability.activation {
		cell, ok := raw.(*schemaActivationRuleBindingCell)
		if !ok || cell == nil || !cell.schemaRuleComplete() {
			return nil, false
		}
		return cell, true
	}
	if _, activation := raw.(*schemaActivationRuleBindingCell); activation {
		return nil, false
	}
	cell, ok := raw.(sealedRuleCell)
	if !ok || cell == nil || !cell.schemaRuleComplete() {
		return nil, false
	}
	return cell, true
}

// resolveGeneratedRuleCell authenticates the distinct generated sealed arm.
// It deliberately does not widen resolveSealedRuleCell: generated rows must
// never enter the legacy operand/provider binding path.
func resolveGeneratedRuleCell(capability RuleSlotCapability) (*generatedRuleBindingCell, bool) {
	state := capability.state
	if state == nil || capability.authority == nil || !capability.available() {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority != capability.authority || state.roleSlots == nil || state.rules == nil || capability.activation {
		return nil, false
	}
	if _, registered := state.roleSlots[capability]; !registered || capability.ordinal >= uint64(len(state.rules)) {
		return nil, false
	}
	cell, ok := state.rules[capability.ordinal].(*generatedRuleBindingCell)
	if !ok || cell == nil || !cell.schemaRuleComplete() {
		return nil, false
	}
	return cell, true
}

func resolveOrdinaryRuleCell(capability RuleSlotCapability) (ordinarySealedRuleCell, bool) {
	if capability.activation {
		return nil, false
	}
	cell, ok := resolveSealedRuleCell(capability)
	if !ok {
		return nil, false
	}
	ordinary, ok := cell.(ordinarySealedRuleCell)
	return ordinary, ok
}

func resolveActivationRuleCell(capability RuleSlotCapability) (*schemaActivationRuleBindingCell, bool) {
	if !capability.activation {
		return nil, false
	}
	cell, ok := resolveSealedRuleCell(capability)
	if !ok {
		return nil, false
	}
	activation, ok := cell.(*schemaActivationRuleBindingCell)
	return activation, ok
}
