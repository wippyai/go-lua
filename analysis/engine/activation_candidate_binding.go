package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Solver-side mounted activation candidate issuer binding; the compile-side
// candidate admission lives in runtime_activation_candidate.go.

// MountedActivationCandidateIssuer is bound once to the sole activation slot
// and to the declared transport vector which makes its existing-body transport
// meaningful: the imported Factors carried into a mounted body and the
// exported Factors carried back out of it. A Factor named in both sets is one
// bidirectional transport, not two authorities. Its fields are private: no domain can
// mint selected factor edges, PointRefs, or a candidate from scalar transport
// coordinates.
type MountedActivationCandidateIssuer struct {
	state   *schemaBindingState
	rule    composition.Key
	family  composition.Key
	imports []composition.Key
	exports []composition.Key
}

type directActivationTransportSetKey struct {
	issuer *MountedActivationCandidateIssuer
	mount  identity.ContentID
	body   identity.ContentID
}

// BindMountedActivationCandidateIssuer joins the exact already-bound
// activation implementation slot with the caller's declared transport vector
// while Binding is open. The vector's arity is the caller's declaration: the
// engine admits any arity the Schema's own declared Factors cover, and each
// reference must name a distinct bound Factor of this Binding's Schema. The
// issuer becomes usable only after the activation row itself is admitted to a
// the private program row workspace.
func BindMountedActivationCandidateIssuer(binding *SchemaBinding, slot *SchemaActivationRuleSlot, imports, exports []AnyFactorRef) (*MountedActivationCandidateIssuer, bool) {
	state := bindingState(binding)
	if state == nil || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || len(imports) == 0 || len(exports) == 0 {
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
	importKeys, importsOK := activationTransportFactorSemantics(state, imports)
	exportKeys, exportsOK := activationTransportFactorSemantics(state, exports)
	if !importsOK || !exportsOK || !activationTransportSymmetric(importKeys, exportKeys) {
		return nil, false
	}
	ruleSemantic := state.schema.ruleSemanticAt(activationOrdinal)
	shape, shapeOK := state.schema.ruleShapeAt(activationOrdinal)
	if !ruleSemantic.Available() || !shapeOK || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		return nil, false
	}
	return &MountedActivationCandidateIssuer{state: state, rule: ruleSemantic, family: shape.ActivationFamily, imports: importKeys, exports: exportKeys}, true
}

func activationTransportFactorSemantics(state *schemaBindingState, refs []AnyFactorRef) ([]composition.Key, bool) {
	claimed := make(map[uint64]struct{}, len(refs))
	keys := make([]composition.Key, len(refs))
	for index, ref := range refs {
		key, ok := activationTransportFactorSemantic(state, ref, claimed)
		if !ok {
			return nil, false
		}
		keys[index] = key
	}
	return keys, true
}

// activationTransportFactorSemantic resolves one declared transport reference
// to its cold Factor semantic and claims the Factor's ordinal, so one vector
// cannot name the same bound Factor twice.
func activationTransportFactorSemantic(state *schemaBindingState, ref AnyFactorRef, claimed map[uint64]struct{}) (composition.Key, bool) {
	ordinal, ordinalOK := anyFactorRefOrdinal(ref, state.schema)
	if !ordinalOK || ordinal >= uint64(len(state.factors)) || state.factors[ordinal] == nil {
		return composition.Key{}, false
	}
	if _, duplicate := claimed[ordinal]; duplicate {
		return composition.Key{}, false
	}
	semantic := state.schema.factorSemanticAt(ordinal)
	if !semantic.Available() {
		return composition.Key{}, false
	}
	claimed[ordinal] = struct{}{}
	return semantic, true
}

// activationTransportSymmetric states the transport vector's symmetry law: an
// axis the export side names must be an axis the import side declares.
//
// The vector crosses one transition in both directions. The import side seeds
// a mounted body's entry Points from its trigger and the export side carries
// the body's exit Points back to that same trigger, so an exported value is a
// function of the in-state its own axis was seeded with. An export whose axis
// has no import therefore names a lane the body's entry never received, and
// the exit row it produces would publish that lane into the trigger from an
// unseeded start. The converse is not a defect: an axis carried in and never
// carried back is a lane the body only reads.
func activationTransportSymmetric(imports, exports []composition.Key) bool {
	imported := make(map[composition.Key]struct{}, len(imports))
	for _, factor := range imports {
		imported[factor] = struct{}{}
	}
	for _, factor := range exports {
		if _, declared := imported[factor]; !declared {
			return false
		}
	}
	return true
}
