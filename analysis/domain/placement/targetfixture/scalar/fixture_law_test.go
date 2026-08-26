package scalar_test

import (
	"testing"

	scalarfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/scalar"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestTargetRuntimeParity runs every scalar route through the family
// declaration, target compile/check/mount/bootstrap, solve, and canonical
// snapshot query. The fixture's Expected value is the owner-issued route
// consequence; this test does not restate the fold in a second implementation.
func TestTargetRuntimeParity(t *testing.T) {
	cases := []struct {
		name  string
		build func(scalarfixture.Probe) scalarfixture.Fixture
	}{
		{name: "publication escape", build: scalarfixture.NewPublicationEscape},
		{name: "return escape", build: scalarfixture.NewReturnEscape},
		{name: "transfer", build: scalarfixture.NewTransfer},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := test.build(t)
			result, ok := fixture.Solve()
			if !ok || !result.Available() {
				t.Fatal("scalar target solve")
			}
			if result.Evaluations() != 1 || result.Publications() != 1 {
				t.Fatalf("scalar target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
			}
			rows, ok := fixture.Facts(result)
			if !ok || !rows.Available() || rows.Len() != 1 {
				t.Fatalf("scalar target snapshot rows = available:%v rows:%d, want 1", rows.Available(), rows.Len())
			}
			row, ok := rows.At(0)
			if !ok || !row.Available() || !row.HasLineage() || !row.Presence().Is(model.Present) {
				t.Fatal("scalar target snapshot row metadata")
			}
			fact, ok := row.Fact()
			if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
				t.Fatalf("scalar target snapshot fact = %#v, want %#v", fact, fixture.Expected())
			}
		})
	}
}
