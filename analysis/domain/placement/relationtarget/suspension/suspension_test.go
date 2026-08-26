package suspension_test

import (
	"testing"

	suspensionfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/suspension"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestTargetRuntimeParity proves Suspension's declared binding (D/B), target
// solve (R), and canonical typed Placement query (Q) through one
// targetfixture Build → Solve → Snapshot path. The domain fixture supplies
// only authentic private-constructor payloads, never target schema or runtime
// authority.
func TestTargetRuntimeParity(t *testing.T) {
	fixture := suspensionfixture.New(t)
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatal("suspension target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("suspension target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	rows, ok := fixture.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("suspension target typed facts = available:%v rows:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.HasLineage() || !row.Presence().Is(model.Present) {
		t.Fatal("suspension target typed fact metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("suspension target typed fact = %#v, want %#v", fact, fixture.Expected())
	}
}
