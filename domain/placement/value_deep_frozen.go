package placement

import (
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// DeepFrozenValue reduces one complete Value relation against Placement's
// allocation evidence. It is the consumer-side proof needed at publication
// boundaries: scalar alternatives are vacuously immutable, every exact
// allocation alternative must carry a transitive proof, and any opaque or
// non-allocation reference keeps the verdict Unknown.
//
// The Value and Placement authorities must project the exact same Heap
// schema. This prevents an equal-content foreign Value relation or detached
// summary from authorizing immutable sharing.
func DeepFrozenValue(schema Schema, values *valuedomain.Schema, summary SummaryResult, fact valuedomain.Value) EvidenceState {
	if !schema.Valid() || values == nil || !values.OwnsHeapSchema(schema.Heap()) ||
		!summary.Available() || summary.SchemaID() != schema.ContentID() {
		return EvidenceUnknown
	}
	state := EvidenceProven
	visited := values.VisitSupport(fact, func(atom valuedomain.Atom) {
		if state == EvidenceRefuted {
			return
		}
		classification, classificationOK := ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			state = mergeDeepFrozenVerdict(state, EvidenceUnknown)
			return
		}
		switch classification.Class {
		case AtomClassScalar:
			// Scalar and nil atoms carry no mutable graph.
			return
		case AtomClassBoot, AtomClassRoot, AtomClassOpaque:
			// Non-local and opaque references are not allocation rows in the
			// public Placement summary. Do not fabricate a negative proof from
			// their absence.
			state = mergeDeepFrozenVerdict(state, EvidenceUnknown)
			return
		case AtomClassAllocation:
			if classification.Role != materialization.Recent && classification.Role != materialization.Summary {
				// Exact is an unmaterialized structural occurrence, not proof of
				// either runtime allocation version.
				state = mergeDeepFrozenVerdict(state, EvidenceUnknown)
				return
			}
			keyID, keyIDOK := schema.Heap().KeyID(classification.Key)
			frozen, frozenOK := summary.DeepFrozenFor(keyID)
			if !keyIDOK || !frozenOK {
				state = mergeDeepFrozenVerdict(state, EvidenceUnknown)
				return
			}
			state = mergeDeepFrozenVerdict(state, frozen)
		}
	})
	if !visited {
		return EvidenceUnknown
	}
	return state
}

func mergeDeepFrozenVerdict(left, right EvidenceState) EvidenceState {
	if left == EvidenceRefuted || right == EvidenceRefuted {
		return EvidenceRefuted
	}
	if left == EvidenceUnknown || right == EvidenceUnknown {
		return EvidenceUnknown
	}
	if left == EvidenceProven && right == EvidenceProven {
		return EvidenceProven
	}
	return EvidenceUnknown
}
