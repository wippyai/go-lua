package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
)

// attachTestComposition keeps ordinary test setup at carrier's only
// publication cut. Tests that exercise either phase use carrier's canonical
// PrepareComposition and Attach operations directly.
func attachTestComposition(t testing.TB, operations []carrier.FactorOperation) (*carrier.Composition, bool) {
	t.Helper()
	prepared, ok := carrier.PrepareComposition(operations)
	if !ok {
		return nil, false
	}
	return prepared.Attach()
}
