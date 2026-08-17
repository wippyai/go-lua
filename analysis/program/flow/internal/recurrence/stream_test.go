package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBoundaryHooksPreserveStableWorkOrder(t *testing.T) {
	counts := []uint32{1, 0, 1}
	work := []arcWork{
		{recurrent: true, first: 0, past: 2},
		{recurrent: true, first: 2, past: 2},
	}
	offsets, hooks, err := boundaryHooks(counts, work, true, len(work))
	if err != nil {
		t.Fatalf("boundaryHooks: %v", err)
	}
	if len(offsets) != len(counts)+1 || len(hooks) != len(work) || hooks[0] != 0 || hooks[1] != 1 {
		t.Fatalf("boundaryHooks = offsets %v hooks %v; want source-order hooks", offsets, hooks)
	}
	if headOwnedByComponent(keyspace.MakeTerm(keyspace.FamilyLoop, 1), 0, []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyLoop, 1)}, nil) == false {
		t.Fatal("primary head was not recognized as component-owned")
	}
	if headOwnedByComponent(keyspace.MakeTerm(keyspace.FamilyLoop, 2), 0, []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyLoop, 1)}, nil) {
		t.Fatal("foreign head was recognized as component-owned")
	}
}

func TestBoundaryHooksRejectDenominatorMismatch(t *testing.T) {
	if _, _, err := boundaryHooks([]uint32{1}, []arcWork{{recurrent: true, first: 0, past: 0}}, true, 0); err == nil {
		t.Fatal("boundaryHooks accepted a mismatched hook denominator")
	}
}
