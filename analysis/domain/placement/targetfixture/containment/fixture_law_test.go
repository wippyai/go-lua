package containment_test

import (
	"testing"

	containmentfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/containment"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestTargetRuntimeParity proves Containment's owner fold through the
// production target runtime and its canonical typed Fact query.
func TestTargetRuntimeParity(t *testing.T) {
	fixture := containmentfixture.New(t)
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatal("containment target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("containment target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	rows, ok := fixture.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("containment target typed facts = available:%v rows:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.Presence().Is(model.Present) || !row.HasLineage() {
		t.Fatal("containment target typed fact metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("containment target typed fact = %#v, want %#v", fact, fixture.Expected())
	}
}
