package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

func (issuer *MountedActivationCandidateIssuer) validFor(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt) bool {
	if issuer == nil || issuer.state == nil || assembly == nil || assembly.builder == nil || assembly.builder.inner == nil || occurrence.assembly != assembly || !occurrence.role.mounted() || !occurrence.role.activation || !issuer.rule.Available() || !issuer.family.Available() {
		return false
	}
	if len(issuer.imports) == 0 || !issuer.export.Available() {
		return false
	}
	for _, factor := range issuer.imports {
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
// point memberships from its own artifact snapshot and emits the transport
// roles its bound vector declares. It never accepts PointRefs, factor IDs, or
// edge slices.
func (issuer *MountedActivationCandidateIssuer) AddMountedActivationCandidate(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt, application, target, endpoint identity.SemanticKey, mount, body identity.ContentID) bool {
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
		set, setOK = equation.NewDirectActivationTransportSet(issuer.state.schema.cold, inner.batch, entries, exits, issuer.imports, issuer.export)
		if !setOK {
			inner.mu.Unlock()
			return false
		}
		inner.directTransportSets[setKey] = set
	}
	origin := equation.MaterializationOrigin{Family: shape.ActivationFamily, Application: compositionKeyOf(application), Target: compositionKeyOf(target), Endpoint: compositionKeyOf(endpoint), TriggerOrdinal: triggerIndex}
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
func (issuer *MountedActivationCandidateIssuer) CompleteMountedActivationCandidates(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt, application identity.SemanticKey, expected uint64) bool {
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
	valid := registered && !completed && actual == expected && (!hasApplication || knownApplication == compositionKeyOf(application)) && shapeOK && shape.ActivationCount == 1 && shape.ActivationFamily == issuer.family
	if !valid {
		inner.mu.Unlock()
		return false
	}
	inner.semantic.activationCandidates[trigger] = actual
	inner.semantic.activationExpected[trigger] = expected
	inner.semantic.activationApplication[trigger] = compositionKeyOf(application)
	inner.mu.Unlock()
	return true
}
