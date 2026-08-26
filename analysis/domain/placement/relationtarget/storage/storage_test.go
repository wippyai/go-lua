package storage_test

import (
	"testing"

	storagefixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/storage"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestTargetRuntimeParity proves Storage's declared binding (D/B), target
// solve (R), and canonical typed Placement query (Q) as one target fixture.
// The source fixture provides only owner-authenticated Value/Program payloads;
// this test's target schema and runtime path are the sole targetfixture Build
// → Solve → Snapshot path.
func TestTargetRuntimeParity(t *testing.T) {
	fixture := storagefixture.New(t)
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatal("storage target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("storage target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	rows, ok := fixture.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("storage target typed facts = available:%v rows:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.HasLineage() || !row.Presence().Is(model.Present) {
		t.Fatal("storage target typed fact metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("storage target typed fact = %#v, want %#v", fact, fixture.Expected())
	}
}
