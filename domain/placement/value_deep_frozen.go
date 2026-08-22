package placement

import (
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// DeepFrozenValue reduces one complete Value relation against Placement's
// allocation evidence. It is the consumer-side proof needed at publication
// boundaries: scalar alternatives are vacuously immutable, every exact
// allocation alternative must carry a transitive proof, and any opaque or
// non-allocation reference keeps the verdict Unknown. An allocation row whose
// DeepFrozen column no producer decided keeps the verdict absent, which is a
// pending reduction rather than an authenticated undecidable one.
//
// The Value and Placement authorities must project the exact same Heap
// schema. This prevents an equal-content foreign Value relation or detached
// summary from authorizing immutable sharing.
func DeepFrozenValue(schema Schema, values *valuedomain.Schema, summary SummaryResult, fact valuedomain.Value) EvidenceState {
	if !schema.Valid() || values == nil || !values.OwnsHeapSchema(schema.Heap()) ||
		!summary.Available() || summary.SchemaID() != schema.ContentID() {
		return invalidEvidenceState
	}
	state := EvidenceProven
	visited := values.VisitSupport(fact, func(atom valuedomain.Atom) {
		if !state.Valid() || state == EvidenceRefuted {
			return
		}
		classification, classificationOK := ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			state = invalidEvidenceState
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
				// either runtime allocation version. This is a valid, authenticated
				// alternative, but it carries no concrete object proof.
				state = mergeDeepFrozenVerdict(state, EvidenceUnknown)
				return
			}
			keyID, keyIDOK := schema.Heap().KeyID(classification.Key)
			frozen, frozenOK := summary.DeepFrozenFor(keyID)
			if !keyIDOK || !frozenOK {
				// A missing allocation row is a malformed/incomplete summary,
				// not an absent proof that can be widened to Unknown.
				state = invalidEvidenceState
				return
			}
			state = mergeDeepFrozenVerdict(state, frozen)
		}
	})
	if !visited {
		return invalidEvidenceState
	}
	return state
}

// mergeDeepFrozenVerdict is the conjunctive consumer reduction: a value is
// deeply frozen only when every alternative it can denote is. The dominance
// order is therefore Refuted, then Absent, then Unknown, then Proven. Refuted
// is final because one mutable alternative settles the whole value negatively
// no matter what the remaining rows say. Absence outranks Unknown because the
// reduction is not finished while a contributing column is still unwritten:
// reporting Unknown there would settle a verdict the producers never reached
// and would stop the consumer from refiring once the column arrives.
func mergeDeepFrozenVerdict(left, right EvidenceState) EvidenceState {
	if !left.Valid() || !right.Valid() {
		return invalidEvidenceState
	}
	if left == EvidenceRefuted || right == EvidenceRefuted {
		return EvidenceRefuted
	}
	if left == EvidenceAbsent || right == EvidenceAbsent {
		return EvidenceAbsent
	}
	if left == EvidenceUnknown || right == EvidenceUnknown {
		return EvidenceUnknown
	}
	if left == EvidenceProven && right == EvidenceProven {
		return EvidenceProven
	}
	return invalidEvidenceState
}
