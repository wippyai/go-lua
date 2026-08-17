package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Solver-side mounted activation candidate issuer binding; the compile-side
// candidate admission lives in activation_candidate_issuer.go.

// MountedActivationCandidateIssuer is bound once to the sole activation slot
// and to the declared transport vector which makes its existing-body transport
// meaningful: the imported Factors carried into a mounted body and the one
// exported Factor carried back out of it. Its fields are private: no domain can
// mint selected factor edges, PointRefs, or a candidate from scalar transport
// coordinates.
type MountedActivationCandidateIssuer struct {
	state   *schemaBindingState
	rule    composition.Key
	family  composition.Key
	imports []composition.Key
	export  composition.Key
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
// ReceiptAssembly.
func BindMountedActivationCandidateIssuer(binding *SchemaBinding, slot *SchemaActivationRuleSlot, imports []AnyFactorRef, export AnyFactorRef) (*MountedActivationCandidateIssuer, bool) {
	state := bindingState(binding)
	if state == nil || slot == nil || slot.cell == nil || slot.cell.schema != state.schema || len(imports) == 0 {
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
	claimed := make(map[uint64]struct{}, len(imports)+1)
	keys := make([]composition.Key, len(imports))
	for index, ref := range imports {
		key, keyOK := activationTransportFactorSemantic(state, ref, claimed)
		if !keyOK {
			return nil, false
		}
		keys[index] = key
	}
	exportKey, exportOK := activationTransportFactorSemantic(state, export, claimed)
	if !exportOK {
		return nil, false
	}
	ruleSemantic := state.schema.ruleSemanticAt(activationOrdinal)
	shape, shapeOK := state.schema.ruleShapeAt(activationOrdinal)
	if !ruleSemantic.Available() || !shapeOK || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		return nil, false
	}
	return &MountedActivationCandidateIssuer{state: state, rule: ruleSemantic, family: shape.ActivationFamily, imports: keys, export: exportKey}, true
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
