package placementscalar

import (
	"testing"

	scalarfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/scalar"
)

func TestTargetPublicationEscapeScalarPublishesFact(t *testing.T) {
	fixture := scalarfixture.NewPublicationEscape(t)
	result, ok := fixture.Solve()
	if !ok {
		t.Fatal("publication escape solve")
	}
	rows, ok := fixture.Facts(result)
	if !ok {
		t.Fatal("publication escape facts")
	}
	assertScalarFact(t, fixture, result, rows)
}
