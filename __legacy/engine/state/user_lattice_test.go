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
	dropped, err := RegisteredProductDomain(reg).ApplyCallBoundary(withTaint)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := dropped.ReadUserElement(reg, ks, taintAxis, stateKey); !ok || got != "Untainted" {
		t.Fatalf("call-boundary taint = %q/%v, want Untainted/true", got, ok)
	}
}

func TestCallBoundaryFactorMatchesConcreteUserLatticeLaw(t *testing.T) {
	const taintAxis userlattice.AxisID = "state.test.call-factor"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(taintAxis, userlattice.CallBoundaryDrop))
	ks := keyspace.New()
	stateKey := pathaddr.StateKey("sym812@1")
	domain := RegisteredProductDomain(reg)
	input := State{}.WriteUserElement(reg, ks, taintAxis, stateKey, "Tainted")

	got, err := domain.ApplyCallBoundary(input)
	if err != nil {
		t.Fatal(err)
	}
	lanes := domain.CallBoundaryFactorLanes()
	if len(lanes) != 1 || lanes[0].ID() != LaneUserLattices {
		t.Fatalf("call-boundary participants = %#v, want only user-lattices", lanes)
	}
	factors, err := domain.DecomposeLanes(input, lanes)
	if err != nil {
		t.Fatal(err)
	}
	factors[0], err = domain.ApplyCallBoundaryFactor(factors[0])
	if err != nil {
		t.Fatal(err)
	}
	factored, err := domain.PatchLaneFactors(input, factors)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Lattice().Equal(factored, got) {
		t.Fatal("factor-native call boundary differs from concrete law")
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

func TestUserLatticeMeetIsPointwiseExact(t *testing.T) {
	const taintAxis userlattice.AxisID = "state.test.meet"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(taintAxis, userlattice.CallBoundaryKeep))
	ks := keyspace.New()
	first := pathaddr.StateKey("sym804@1")
	second := pathaddr.StateKey("sym805@1")
	productDomain, err := DefaultLaneCatalog().TryProductDomainWithLaneSet(reg, NewLaneSet(LaneUserLattices))
	if err != nil {
		t.Fatalf("product domain: %v", err)
	}
	domain := productDomain.Lattice()
	left := domain.Bottom().
		WriteUserElement(reg, ks, taintAxis, first, "Sanitized").
		WriteUserElement(reg, ks, taintAxis, second, "Tainted")
	right := domain.Bottom().
		WriteUserElement(reg, ks, taintAxis, first, "Tainted")
	lane, _ := productDomain.ProductLane(LaneUserLattices)
	leftFactor, err := productDomain.DecomposeLanes(left, []ProductLane{lane})
	if err != nil {
		t.Fatalf("decompose left: %v", err)
	}
	rightFactor, err := productDomain.DecomposeLanes(right, []ProductLane{lane})
	if err != nil {
		t.Fatalf("decompose right: %v", err)
	}
	metFactor, err := productDomain.LaneMeet(leftFactor[0], rightFactor[0])
	if err != nil {
		t.Fatalf("lane meet: %v", err)
	}
	met, err := productDomain.Compose([]LaneFactor{metFactor})
	if err != nil {
		t.Fatalf("compose meet: %v", err)
	}
	if got, _ := met.ReadUserElement(reg, ks, taintAxis, first); got != "Untainted" {
		t.Fatalf("first meet = %q, want Untainted", got)
	}
	if got, _ := met.ReadUserElement(reg, ks, taintAxis, second); got != "Untainted" {
		t.Fatalf("second meet = %q, want Untainted", got)
	}
	topFactor, _ := productDomain.LaneTop(lane)
	topMeet, err := productDomain.LaneMeet(topFactor, leftFactor[0])
	if err != nil {
		t.Fatalf("Meet(Top,left): %v", err)
	}
	if equal, _ := productDomain.LaneEqual(topMeet, leftFactor[0]); !equal {
		t.Fatalf("Meet(Top,left) != left factor")
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
