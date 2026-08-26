package formal_test

import (
	"testing"

	formalfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/formal"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestTargetRuntimeParity proves one scalar Placement family through the
// target relation runtime, not through the legacy composite form.
func TestTargetRuntimeParity(t *testing.T) {
	fixture := formalfixture.New(t)
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatal("formal target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("formal target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	rows, ok := fixture.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("formal target typed facts = available:%v rows:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.HasLineage() || !row.Presence().Is(model.Present) {
		t.Fatal("formal target typed fact metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("formal target typed fact = %#v, want %#v", fact, fixture.Expected())
	}
}
