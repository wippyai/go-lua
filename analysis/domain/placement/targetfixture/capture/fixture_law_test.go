package capture_test

import (
	"testing"

	capturefixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/capture"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestTargetRuntimeParity proves Capture's owner fold through the production
// target runtime and its canonical typed Fact query.
func TestTargetRuntimeParity(t *testing.T) {
	fixture := capturefixture.New(t)
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatal("capture target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("capture target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	rows, ok := fixture.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("capture target typed facts = available:%v rows:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.Presence().Is(model.Present) || !row.HasLineage() {
		t.Fatal("capture target typed fact metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("capture target typed fact = %#v, want %#v", fact, fixture.Expected())
	}
}
