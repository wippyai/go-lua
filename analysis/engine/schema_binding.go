// schema_binding.go owns the schema binding lifecycle: state, phase, seal and poison.

package engine

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/schema"
)

type schemaBindingPhase uint8

const (
	schemaBindingOpen schemaBindingPhase = iota + 1
	schemaBindingSealed
	schemaBindingPoisoned
)

// Keep this non-zero-sized. Go permits pointers to distinct zero-sized
// allocations to compare equal, which would collapse independent Binding
// receipt authorities.
type schemaBindingAuthority struct{ marker byte }

// SchemaBinding is a copyable handle to one shared lifecycle. It cannot
// publish until every Factor, Rule, Query, and activation-family cell is
// complete and fenced to this exact Schema.
type SchemaBinding struct{ state *schemaBindingState }

type schemaBindingState struct {
	mu         sync.Mutex
	schema     *Schema
	phase      schemaBindingPhase
	authority  *schemaBindingAuthority
	factors    []schemaFactorBinding
	rules      []schemaBindingCell
	queries    []schemaBindingCell
	activation []schemaBindingCell
	roleSlots  map[RuleSlotCapability]composition.Key
	// linkBootstrapTransports is the sole ordered transport authorization for
	// the Link-global bootstrap seam. The engine retains opaque capabilities,
	// never domain role names; the program owner registers the exact pair once
	// before this Binding seals.
	linkBootstrapTransports    [2]RuleSlotCapability
	linkBootstrapTransportPair bool
	// columns is the published columns this binding's publication is admitted
	// to write, by the column's authored key, and columnSlots is the same set
	// by the dense slot each occupies. Both are stated once, while the binding
	// is open, and are what a write capability is minted against afterwards.
	columns     map[schema.Key]*admittedColumn
	columnSlots map[uint32]schema.Key
}

type schemaBindingCell interface{ schemaBindingSchema() *Schema }

func NewSchemaBinding(schema *Schema) *SchemaBinding {
	if schema == nil || !schema.Available() {
		return nil
	}
	factors, rules, queries, activations, ok := schema.shapeCount()
	if !ok {
		return nil
	}
	return &SchemaBinding{state: &schemaBindingState{
		schema: schema, phase: schemaBindingOpen, authority: &schemaBindingAuthority{},
		factors:    make([]schemaFactorBinding, factors),
		rules:      make([]schemaBindingCell, rules),
		queries:    make([]schemaBindingCell, queries),
		activation: make([]schemaBindingCell, activations),
		roleSlots:  make(map[RuleSlotCapability]composition.Key),
	}}
}

func bindingState(binding *SchemaBinding) *schemaBindingState {
	if binding == nil {
		return nil
	}
	return binding.state
}

func (binding *SchemaBinding) Schema() *Schema {
	state := bindingState(binding)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil {
		return nil
	}
	return state.schema
}

func (binding *SchemaBinding) Sealed() bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == schemaBindingSealed && state.authority != nil
}

func (binding *SchemaBinding) Poisoned() bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == schemaBindingPoisoned
}

func (state *schemaBindingState) poisonLocked() {
	if state == nil || state.phase == schemaBindingSealed {
		return
	}
	state.phase = schemaBindingPoisoned
	state.factors = nil
	state.rules = nil
	state.queries = nil
	state.activation = nil
	state.columns = nil
	state.columnSlots = nil
	state.authority = nil
}

// Seal validates the Factor vertical, receipt-native Rule/activation lanes,
// and the currently supported exact-Factor Query lane. Activation families
// are inventoried separately from their Rule cells and must be complete before
// one Binding authority is published.
func (binding *SchemaBinding) Seal() bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.authority == nil {
		state.poisonLocked()
		return false
	}
	if state.schema == nil {
		return sealColumnBindingLocked(state)
	}
	if len(state.activation) != int(state.schema.activationCount()) {
		state.poisonLocked()
		return false
	}
	if len(state.factors) != schemaFactorCount(state.schema) {
		state.poisonLocked()
		return false
	}
	for ordinal, cell := range state.factors {
		if cell == nil || cell.schemaFactorSchema() != state.schema || cell.schemaFactorOrdinal() != uint64(ordinal) || !cell.schemaFactorComplete() {
			state.poisonLocked()
			return false
		}
	}
	for ordinal, cell := range state.rules {
		rule, ok := cell.(schemaRuleBindingCell)
		if !ok || rule == nil || rule.schemaBindingSchema() != state.schema || rule.schemaRuleOrdinal() != uint64(ordinal) || !rule.schemaRuleComplete() {
			state.poisonLocked()
			return false
		}
	}
	for ordinal, cell := range state.activation {
		family, ok := cell.(*schemaActivationFamilyBindingCell)
		if !ok || family == nil || cell.schemaBindingSchema() != state.schema || !family.activationComplete(state.schema, uint64(ordinal)) {
			state.poisonLocked()
			return false
		}
	}
	for ordinal, cell := range state.queries {
		query, ok := cell.(schemaQueryBindingCell)
		if !ok || query == nil || cell.schemaBindingSchema() != state.schema || query.schemaQueryState() != state || query.schemaQueryOrdinal() != uint64(ordinal) || !query.complete() {
			state.poisonLocked()
			return false
		}
	}
	if len(state.roleSlots) != 0 {
		if !completeCapabilityDirectory(state) {
			state.poisonLocked()
			return false
		}
	}
	if state.linkBootstrapTransportPair && !completeLinkBootstrapTransportPairLocked(state) {
		state.poisonLocked()
		return false
	}
	// The published columns this binding admits a writer for are stated at the
	// seal, so the pairs a capability may be minted for afterwards are exactly
	// the pairs the table declared and no publisher extends the set.
	if !completeAdmittedColumnsLocked(state) {
		state.poisonLocked()
		return false
	}
	state.phase = schemaBindingSealed
	return true
}

func completeLinkBootstrapTransportPairLocked(state *schemaBindingState) bool {
	if state == nil || state.schema == nil || state.authority == nil || !state.linkBootstrapTransportPair {
		return false
	}
	seenOutputs := make(map[composition.Key]struct{}, len(state.linkBootstrapTransports))
	for _, capability := range state.linkBootstrapTransports {
		semantic, registered := state.roleSlots[capability]
		shape, shapeOK := state.schema.ruleShapeAt(capability.ordinal)
		if !registered || !capability.link() || capability.state != state || capability.authority != state.authority || semantic != state.schema.ruleSemanticAt(capability.ordinal) || !shapeOK || shape.OutputKind != composition.FactorOutput || !shape.Output.Available() {
			return false
		}
		if _, duplicate := seenOutputs[shape.Output]; duplicate {
			return false
		}
		seenOutputs[shape.Output] = struct{}{}
	}
	return true
}
