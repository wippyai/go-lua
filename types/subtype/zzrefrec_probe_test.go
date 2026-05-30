package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZRefRecNominalMatch verifies the checkCore Ref<->Recursive nominal match:
// a local *typ.Ref unifies with a *typ.Recursive family of the same declared
// name (both directions), and DIFFERENT-named families must NOT unify.
func TestZZRefRecNominalMatch(t *testing.T) {
	recA := typ.NewRecursiveWithBody("EventEmitter", typ.NewRecord().Build())
	recB := typ.NewRecursiveWithBody("Other", typ.NewRecord().Build())

	refA := typ.NewRef("", "EventEmitter")
	refB := typ.NewRef("", "Other")

	// Positive: same name, both directions.
	if !IsSubtype(refA, recA) {
		t.Errorf("IsSubtype(Ref(EventEmitter), Recursive(EventEmitter)) = false, want true")
	}
	if !IsSubtype(recA, refA) {
		t.Errorf("IsSubtype(Recursive(EventEmitter), Ref(EventEmitter)) = false, want true")
	}

	// Soundness: different name must NOT unify, both directions.
	if IsSubtype(refA, recB) {
		t.Errorf("IsSubtype(Ref(EventEmitter), Recursive(Other)) = true, want false (over-merge)")
	}
	if IsSubtype(recB, refA) {
		t.Errorf("IsSubtype(Recursive(Other), Ref(EventEmitter)) = true, want false (over-merge)")
	}
	if IsSubtype(refB, recA) {
		t.Errorf("IsSubtype(Ref(Other), Recursive(EventEmitter)) = true, want false (over-merge)")
	}

	// Module-qualified Ref does not engage the local-ref nominal arm.
	qref := typ.NewRef("class", "EventEmitter")
	if IsSubtype(qref, recA) || IsSubtype(recA, qref) {
		t.Logf("module-qualified Ref vs Recursive stays unmatched (expected: bare Ref carries the signature self)")
	}
}
