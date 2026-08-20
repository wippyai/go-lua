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
)

func (capability RuleSlotCapability) available() bool {
	return capability.state != nil && capability.authority != nil && capability.kind != ruleCapabilityInvalid && capability.ordinal != ^uint64(0) && capability.state.authority == capability.authority
}

func (capability RuleSlotCapability) Available() bool { return capability.available() }

func (capability RuleSlotCapability) Mounted() bool { return capability.mounted() }

func (capability RuleSlotCapability) Link() bool { return capability.link() }

func (capability RuleSlotCapability) Activation() bool {
	return capability.mounted() && capability.activation
}

func (capability RuleSlotCapability) mounted() bool {
	return capability.available() && capability.kind == ruleCapabilityMounted
}

func (capability RuleSlotCapability) link() bool {
	return capability.available() && capability.kind == ruleCapabilityLink
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

func RegisterActivationRuleSlot(binding *SchemaBinding, slot *SchemaActivationRuleSlot, capability RuleSlotCapability) bool {
	if slot == nil || slot.cell == nil {
		return false
	}
	return registerRuleSlot(binding, slot.cell.schema, slot.cell.ordinal, capability)
}

// RegisterLinkBootstrapTransportPair is the one pre-seal authorization for
// the two factors allowed to leave the Link-global bootstrap point. Ordering
// is owner-significant and retained exactly; a same-Binding Link capability
// is not sufficient unless it occupies its registered position in this pair.
func RegisterLinkBootstrapTransportPair(binding *SchemaBinding, first, second RuleSlotCapability) bool {
	state := bindingState(binding)
	if state == nil || !first.link() || !second.link() || first == second || first.state != state || second.state != state || first.authority != state.authority || second.authority != state.authority {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.linkBootstrapTransportPair {
		return false
	}
	state.linkBootstrapTransports = [2]RuleSlotCapability{first, second}
	state.linkBootstrapTransportPair = true
	if !completeLinkBootstrapTransportPairLocked(state) {
		state.linkBootstrapTransports = [2]RuleSlotCapability{}
		state.linkBootstrapTransportPair = false
		return false
	}
	return true
}

func sealedLinkBootstrapTransportPair(state *schemaBindingState) ([2]RuleSlotCapability, bool) {
	if state == nil {
		return [2]RuleSlotCapability{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || !completeLinkBootstrapTransportPairLocked(state) {
		return [2]RuleSlotCapability{}, false
	}
	return state.linkBootstrapTransports, true
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
