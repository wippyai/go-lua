package state

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

func TestUserLatticeStateSemantics(t *testing.T) {
	const taintAxis userlattice.AxisID = "state.test.taint"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(taintAxis, userlattice.CallBoundaryKeep))
	ks := keyspace.New()
	stateKey := pathaddr.StateKey("sym801@1")

	if got, ok := (State{}).ReadUserElement(reg, ks, taintAxis, stateKey); !ok || got != "Untainted" {
		t.Fatalf("bottom read = %q/%v, want Untainted/true", got, ok)
	}

	withTaint := State{}.WriteUserElement(reg, ks, taintAxis, stateKey, "Tainted")
	if got, ok := withTaint.ReadUserElement(reg, ks, taintAxis, stateKey); !ok || got != "Tainted" {
		t.Fatalf("written taint = %q/%v, want Tainted/true", got, ok)
	}

	sanitized := withTaint.ApplyUserClaim(reg, ks, taintAxis, stateKey, "sanitized")
	if got, _ := sanitized.ReadUserElement(reg, ks, taintAxis, stateKey); got != "Sanitized" {
		t.Fatalf("claim taint = %q, want Sanitized", got)
	}

	cleared := sanitized.ClearUserElement(reg, ks, taintAxis, stateKey)
	if got, ok := cleared.ReadUserElement(reg, ks, taintAxis, stateKey); !ok || got != "Untainted" {
		t.Fatalf("cleared taint = %q/%v, want Untainted/true", got, ok)
	}
	if snap := cleared.UserLatticesSnapshot(reg, ks); len(snap.Values) != 0 || snap.Top {
		t.Fatalf("cleared snapshot = %#v, want empty finite snapshot", snap)
	}
}

func TestUserLatticeCallBoundaryDrop(t *testing.T) {
	const taintAxis userlattice.AxisID = "state.test.call-drop"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(taintAxis, userlattice.CallBoundaryDrop))
	ks := keyspace.New()
	stateKey := pathaddr.StateKey("sym802@1")

	withTaint := State{}.WriteUserElement(reg, ks, taintAxis, stateKey, "Tainted")
	dropped := withTaint.ApplyUserCallBoundary(reg)
	if got, ok := dropped.ReadUserElement(reg, ks, taintAxis, stateKey); !ok || got != "Untainted" {
		t.Fatalf("call-boundary taint = %q/%v, want Untainted/true", got, ok)
	}
}

func TestUserLatticeJoinIdenticalAllocFree(t *testing.T) {
	const taintAxis userlattice.AxisID = "state.test.alloc"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(taintAxis, userlattice.CallBoundaryKeep))
	ks := keyspace.New()
	stateKey := pathaddr.StateKey("sym803@1")
	domain := DomainWithLanes(reg, []LaneID{LaneUserLattices})
	st := domain.Bottom().WriteUserElement(reg, ks, taintAxis, stateKey, "Tainted")

	allocs := testing.AllocsPerRun(1000, func() {
		_ = domain.Join(st, st)
	})
	if allocs != 0 {
		t.Fatalf("user-lattice identical join allocations/run = %.1f, want 0", allocs)
	}
}

func userLatticeTestRegistry(t *testing.T, spec userlattice.Spec) *axis.Registry {
	t.Helper()
	reg := axis.NewRegistry()
	if _, err := userlattice.Register(reg, spec); err != nil {
		t.Fatalf("register user lattice: %v", err)
	}
	return reg.Freeze()
}

func userLatticeTestSpec(id userlattice.AxisID, boundary userlattice.CallBoundaryMode) userlattice.Spec {
	return userlattice.Spec{
		ID:       id,
		Elements: []userlattice.ElementID{"Untainted", "Sanitized", "Tainted", "Unknown"},
		Bottom:   "Untainted",
		Top:      "Unknown",
		Order: []userlattice.OrderPair{
			{Lower: "Untainted", Upper: "Sanitized"},
			{Lower: "Untainted", Upper: "Tainted"},
			{Lower: "Sanitized", Upper: "Unknown"},
			{Lower: "Tainted", Upper: "Unknown"},
		},
		Hooks: userlattice.Hooks{
			OnAssign:       userlattice.AssignHook{Mode: userlattice.AssignPropagate},
			OnCallBoundary: userlattice.CallBoundaryHook{Mode: boundary},
			OnClaim: []userlattice.ClaimHook{
				{Claim: "tainted", Element: "Tainted"},
				{Claim: "sanitized", Element: "Sanitized"},
			},
		},
	}
}
