package engine

import "testing"

// The Rule boundary passes its capability by value. This law guards the hot
// dispatch shape itself: a synchronous callback receives no heap object per
// Product terminal. Expiration is covered separately through the public API.
func TestRuleAccessValueDispatchIsAllocationFree(t *testing.T) {
	identity := &ruleIdentity{}
	transaction := &transaction{executing: true}
	frame := &ruleExecution{transaction: transaction, epoch: 1, rule: identity}
	access := Access[uint64, uint8]{frame: frame, epoch: 1, identity: identity}
	run := func(Access[uint64, uint8]) bool { return true }
	if allocations := testing.AllocsPerRun(100, func() {
		if !run(access) {
			t.Fatal("value Rule callback rejected its capability")
		}
	}); allocations != 0 {
		t.Fatalf("value Rule Access dispatch allocations = %g, want 0", allocations)
	}
}
