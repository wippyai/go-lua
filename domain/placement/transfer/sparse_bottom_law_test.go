package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestTransferSparseActualBottomIsTheFactorDefault states the Value Factor
// default law for the Target-transfer planner. The owner's exact sparse
// Bottom carries no allocation evidence, so it contributes no transfer demand
// and leaves a valid no-route plan. Sparse non-Bottom remains malformed.
func TestTransferSparseActualBottomIsTheFactorDefault(t *testing.T) {
	fixture := newTransferRouteLawFixture(t, true, "transfer-sparse-bottom")
	present, presentOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.actuals(t))
	if !presentOK || present.routeCount() != 1 {
		t.Fatalf("present transfer plan = %t/%d, want one exact route", presentOK, present.routeCount())
	}
	payload := -1
	for index, cell := range fixture.cells {
		if !fixture.values.Equal(cell.Value, fixture.values.Bottom()) {
			payload = index
			break
		}
	}
	if payload < 0 {
		t.Fatal("fixture has no rooted actual to sparsify")
	}
	sparse := append([]execution.MemberCell[valuedomain.Value](nil), fixture.cells...)
	sparse[payload] = execution.MemberCell[valuedomain.Value]{Value: fixture.values.Bottom()}
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, transferActuals(t, sparse))
	if !planOK || plan.routeCount() != 0 {
		t.Fatalf("owner-issued sparse Bottom transfer plan = %t/%d, want valid no-route", planOK, plan.routeCount())
	}
	forged := append([]execution.MemberCell[valuedomain.Value](nil), fixture.cells...)
	forged[payload].Present = false
	if _, forgedOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, transferActuals(t, forged)); forgedOK {
		t.Fatal("sparse non-Bottom Value cell admitted as the Value Factor default")
	}
}
