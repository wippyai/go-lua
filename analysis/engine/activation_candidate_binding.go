package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Solver-side mounted activation candidate issuer binding; the compile-side
// candidate admission lives in activation_candidate_issuer.go.

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
