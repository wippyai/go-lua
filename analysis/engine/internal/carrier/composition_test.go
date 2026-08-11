package carrier

import "testing"

// attachTestComposition keeps ordinary test setup at the sole production cut.
// Publication-law tests call PrepareComposition and Attach directly when they
// need to observe either phase independently.
func attachTestComposition(t testing.TB, operations []FactorOperation) (*Composition, bool) {
	t.Helper()
	prepared, ok := PrepareComposition(operations)
	if !ok {
		return nil, false
	}
	return prepared.Attach()
}
