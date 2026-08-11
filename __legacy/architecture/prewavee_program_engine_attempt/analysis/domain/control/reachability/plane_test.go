package reachability

import (
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestPlaneConfigIsCompleteFiniteReachabilityContract(t *testing.T) {
	config := PlaneConfig()
	if config.Keys.End != 1 {
		t.Fatalf("reachability key universe = [0,%d), want [0,1)", config.Keys.End)
	}
	if config.Default != Unreachable {
		t.Fatalf("reachability default = %v, want Unreachable", config.Default)
	}
	if config.Semantic.ID.Available() == false || config.Semantic.Version == 0 {
		t.Fatal("reachability plane lacks a persistent semantic identity")
	}
	if config.Fingerprint == nil || config.WidenRank.Width != 1 || config.WidenRank.At == nil {
		t.Fatal("reachability plane lacks a complete typed-operation or termination contract")
	}
	if got := config.Fingerprint(Unreachable); got != uint64(Unreachable) {
		t.Fatalf("Unreachable fingerprint = %d, want %d", got, Unreachable)
	}
	if got := config.Fingerprint(Reachable); got != uint64(Reachable) {
		t.Fatalf("Reachable fingerprint = %d, want %d", got, Reachable)
	}
	if got := config.WidenRank.At(0, Unreachable, 0); got != 1 {
		t.Fatalf("Unreachable widen rank = %d, want 1", got)
	}
	if got := config.WidenRank.At(0, Reachable, 0); got != 0 {
		t.Fatalf("Reachable widen rank = %d, want 0", got)
	}
	latticelaws.LawSuite[Value]{
		Name:   "reachability typed plane",
		Domain: config.Lattice,
		Sample: []Value{Unreachable, Reachable},
	}.Run(t)
}
