package placementscalar

import (
	"testing"

	scalarfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/scalar"
)

func TestTargetReturnEscapeScalarPublishesFact(t *testing.T) {
	fixture := scalarfixture.NewReturnEscape(t)
	result, ok := fixture.Solve()
	if !ok {
		t.Fatal("return escape solve")
	}
	rows, ok := fixture.Facts(result)
	if !ok {
		t.Fatal("return escape facts")
	}
	assertScalarFact(t, fixture, result, rows)
}
