package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func factorTupleJoinTestDecompose(t *testing.T, domain ProductDomain, input State) (ValueLaneFactor, []LaneFactor) {
	t.Helper()
	input = domain.Normalize(input)
	residual, values := DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	return values, factors
}

func TestJoinFactorTuplesEqualsWholeProductJoinAcrossAllRegisteredLanes(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	left, right := domain.Lattice().Bottom(), domain.Lattice().Bottom()
	samples := stateLawLaneSamples(reg, keys)
	if len(samples) != len(DefaultLanes()) {
		t.Fatalf("state-law inventory = %d, enabled lanes = %d", len(samples), len(DefaultLanes()))
	}
	for index, sample := range samples {
		if index%2 == 0 {
			left = domain.Lattice().Join(left, sample.state)
		} else {
			right = domain.Lattice().Join(right, sample.state)
		}
	}
	leftValues, leftFactors := factorTupleJoinTestDecompose(t, domain, left)
	rightValues, rightFactors := factorTupleJoinTestDecompose(t, domain, right)
	values, factors, err := domain.JoinFactorTuples(leftValues, leftFactors, rightValues, rightFactors)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ComposeFactorTuple(values, factors)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Lattice().Join(left, right)
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("componentwise factor join diverged from the registered whole-product join")
	}
	if got.numericConsistency != numericConsistencyCertified {
		t.Fatalf("composed factor join numeric consistency = %v, want certified", got.numericConsistency)
	}
}
