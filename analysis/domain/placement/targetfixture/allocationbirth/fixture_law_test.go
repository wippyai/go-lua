package allocationbirth_test

import (
	"testing"

	fixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/allocationbirth"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestTargetRuntimeParity(t *testing.T) {
	world := fixture.New(t)
	result, ok := world.Solve()
	if !ok || !result.Available() {
		t.Fatal("allocationbirth target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("allocationbirth target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	rows, ok := world.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("allocationbirth target typed rows = available:%v rows:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.Presence().Is(model.Present) || !row.HasLineage() {
		t.Fatal("allocationbirth target row metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, world.Expected()) {
		t.Fatalf("allocationbirth target fact = %#v, want %#v", fact, world.Expected())
	}
}
