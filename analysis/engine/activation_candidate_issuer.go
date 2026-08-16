package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// MountedActivationCandidateIssuer is bound once to the sole activation
// slot and the five factor capabilities which make its existing-body
// transport meaningful. Its fields are private: no domain can mint selected
// factor edges, PointRefs, or a candidate from scalar transport coordinates.
type MountedActivationCandidateIssuer struct {
	state   *schemaBindingState
	rule    composition.Key
	family  composition.Key
	factors [5]composition.Key
}

type directActivationTransportSetKey struct {
	issuer *MountedActivationCandidateIssuer
	mount  identity.ContentID
	body   identity.ContentID
}

// BindMountedActivationCandidateIssuer joins the exact already-bound
// activation implementation slot with the five factor refs while Binding is
// open. The issuer becomes usable only after the activation row itself is
// admitted to a ReceiptAssembly.
func BindMountedActivationCandidateIssuer[V, C, H, P, E any](binding *SchemaBinding, slot *SchemaActivationRuleSlot, value FactorRef[V], call FactorRef[C], heap FactorRef[H], pack FactorRef[P], effect FactorRef[E]) (*MountedActivationCandidateIssuer, bool) {
	state := bindingState(binding)
	if state == nil || slot == nil || slot.cell == nil || slot.cell.schema != state.schema {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen {
		return nil, false
	}
	activationOrdinal, activationOK := slot.Ordinal()
	if !activationOK || activationOrdinal >= uint64(len(state.rules)) {
		return nil, false
	}
	activationCell, activationBound := state.rules[activationOrdinal].(*schemaActivationRuleBindingCell)
	if !activationBound || !activationCell.schemaRuleComplete() {
		return nil, false
	}
	ordinals := []uint64{0, 0, 0, 0, 0}
	refsOK := true
	if ordinals[0], refsOK = factorRefOrdinal(value, state.schema); !refsOK {
		return nil, false
	}
	if ordinals[1], refsOK = factorRefOrdinal(call, state.schema); !refsOK {
		return nil, false
	}
	if ordinals[2], refsOK = factorRefOrdinal(heap, state.schema); !refsOK {
		return nil, false
	}
	if ordinals[3], refsOK = factorRefOrdinal(pack, state.schema); !refsOK {
		return nil, false
	}
	if ordinals[4], refsOK = factorRefOrdinal(effect, state.schema); !refsOK {
		return nil, false
	}
	keys := make([]composition.Key, len(ordinals))
	seen := make(map[uint64]struct{}, len(ordinals))
	for index, ordinal := range ordinals {
		if ordinal >= uint64(len(state.factors)) || state.factors[ordinal] == nil {
			return nil, false
		}
		if _, duplicate := seen[ordinal]; duplicate {
			return nil, false
		}
		seen[ordinal] = struct{}{}
		keys[index] = state.schema.factorSemanticAt(ordinal)
		if !keys[index].Available() {
			return nil, false
		}
	}
	ruleSemantic := state.schema.ruleSemanticAt(activationOrdinal)
	shape, shapeOK := state.schema.ruleShapeAt(activationOrdinal)
	if !ruleSemantic.Available() || !shapeOK || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		return nil, false
	}
	return &MountedActivationCandidateIssuer{state: state, rule: ruleSemantic, family: shape.ActivationFamily, factors: [5]composition.Key{keys[0], keys[1], keys[2], keys[3], keys[4]}}, true
}

func (issuer *MountedActivationCandidateIssuer) validFor(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt) bool {
	if issuer == nil || issuer.state == nil || assembly == nil || assembly.builder == nil || assembly.builder.inner == nil || occurrence.assembly != assembly || !occurrence.role.mounted() || !occurrence.role.activation || !issuer.rule.Available() || !issuer.family.Available() {
		return false
	}
	for _, factor := range issuer.factors {
		if !factor.Available() {
			return false
		}
	}
	inner := assembly.builder.inner
	return inner.state == issuer.state && inner.authority != nil && inner.authority == issuer.state.authority && issuer.state.phase == schemaBindingSealed && occurrenceRoleOwnsSchema(occurrence, issuer.state.schema, issuer.rule)
}

// AddMountedActivationCandidate is the only direct-candidate ingress. The
// caller supplies an exact admitted activation occurrence plus its sealed
// application/target tuple and mounted body identity; the engine resolves all
// point memberships from its own artifact snapshot and emits the five fixed
// transport roles. It never accepts PointRefs, factor IDs, or edge slices.
func (issuer *MountedActivationCandidateIssuer) AddMountedActivationCandidate(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt, application, target, endpoint SemanticKey, mount, body identity.ContentID) bool {
	if !issuer.validFor(assembly, occurrence) || !application.Available() || !target.Available() || !endpoint.Available() || target == endpoint || !mount.Available() || !body.Available() {
		return false
	}
	inner, locked := assembly.builder.lockTopologyOpen()
	if !locked {
		return false
	}
	artifact := inner.artifact
	trigger, registered := inner.semantic.activations[occurrence.activation]
	transport, transportOK := artifact.mountedBody(mount, body)
	_, triggerPoint, triggerPointOK := artifact.mountedPoint(occurrence.mount, occurrence.reusable)
	shape, shapeOK := bindingActivationRuleShape(inner, trigger)
	triggerIndex := -1
	if trigger != 0 {
		triggerIndex = int(uint64(trigger) - 1)
	}
	if !registered || triggerIndex < 0 || triggerIndex >= len(inner.spec.Rules) || !transportOK || !triggerPointOK || !shapeOK || shape.ActivationCount != 1 || shape.ActivationFamily != issuer.family || inner.spec.Rules[triggerIndex].Schema != issuer.rule {
		inner.mu.Unlock()
		return false
	}
	setKey := directActivationTransportSetKey{issuer: issuer, mount: mount, body: body}
	set, knownSet := inner.directTransportSets[setKey]
	if !knownSet {
		entries := make([]equation.PointRef, 0, len(transport.entry))
		for _, reusable := range transport.entry {
			_, entry, entryOK := artifact.mountedPoint(mount, reusable)
			if !entryOK {
				inner.mu.Unlock()
				return false
			}
			entries = append(entries, entry)
		}
		exits := make([]equation.PointRef, 0, len(transport.exits))
		for _, reusable := range transport.exits {
			_, exit, exitOK := artifact.mountedPoint(mount, reusable)
			if !exitOK {
				inner.mu.Unlock()
				return false
			}
			exits = append(exits, exit)
		}
		var setOK bool
		set, setOK = equation.NewDirectActivationTransportSet(issuer.state.schema.cold, inner.batch, entries, exits, issuer.factors[:4], issuer.factors[4])
		if !setOK {
			inner.mu.Unlock()
			return false
		}
		inner.directTransportSets[setKey] = set
	}
	origin := equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: application.compositionKey(), Target: target.compositionKey(), Endpoint: endpoint.compositionKey(), TriggerOrdinal: triggerIndex}
	base, source := inner.batch, issuer.state.schema.cold
	inner.mu.Unlock()
	candidate, candidateOK := equation.NewDirectActivationCandidate(source, base, origin, triggerPoint, set)
	if !candidateOK {
		return false
	}
	receipt, issued := assembly.builder.issueDirectActivationCandidate(candidate)
	return issued && assembly.builder.addDirectActivationCandidate(receipt)
}

// CompleteMountedActivationCandidates closes one activation occurrence's
// exact candidate denominator, including the lawful zero-candidate case. The
// expected count is checked against candidates already admitted through this
// same issuer; it cannot turn an omitted or rejected candidate into success.
func (issuer *MountedActivationCandidateIssuer) CompleteMountedActivationCandidates(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt, application SemanticKey, expected uint64) bool {
	if !issuer.validFor(assembly, occurrence) || !application.Available() {
		return false
	}
	inner, locked := assembly.builder.lockTopologyOpen()
	if !locked {
		return false
	}
	trigger, registered := inner.semantic.activations[occurrence.activation]
	shape, shapeOK := bindingActivationRuleShape(inner, trigger)
	actual := inner.semantic.activationCandidates[trigger]
	knownApplication, hasApplication := inner.semantic.activationApplication[trigger]
	_, completed := inner.semantic.activationExpected[trigger]
	valid := registered && !completed && actual == expected && (!hasApplication || knownApplication == application.compositionKey()) && shapeOK && shape.ActivationCount == 1 && shape.ActivationFamily == issuer.family
	if !valid {
		inner.mu.Unlock()
		return false
	}
	inner.semantic.activationCandidates[trigger] = actual
	inner.semantic.activationExpected[trigger] = expected
	inner.semantic.activationApplication[trigger] = application.compositionKey()
	inner.mu.Unlock()
	return true
}
