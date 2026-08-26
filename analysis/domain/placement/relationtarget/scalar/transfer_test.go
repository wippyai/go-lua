package placementscalar

import (
	"testing"

	scalarfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/scalar"
)

func TestTargetTransferScalarPublishesFact(t *testing.T) {
	fixture := scalarfixture.NewTransfer(t)
	result, ok := fixture.Solve()
	if !ok {
		t.Fatal("transfer solve")
	}
	rows, ok := fixture.Facts(result)
	if !ok {
		t.Fatal("transfer facts")
	}
	assertScalarFact(t, fixture, result, rows)
}
