package transformer

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func formalPublicationFactorTupleForTest(
	t *testing.T,
	domain state.ProductDomain,
	input state.State,
) (state.ValueLaneFactor, []state.LaneFactor) {
	t.Helper()
	input = domain.Normalize(input)
	residual, values := state.DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	return values, factors
}

func TestFormalPublicationFactorAccumulatorEqualsStateJoin(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	root := pathaddr.StateKey("publication.factor.root")
	owner := identity.ID{Kind: "publication-factor", Site: "owner", Index: 1}
	slot := statekey.SymbolValue(symbol.ID(701))
	left := domain.Lattice().Bottom().
		WriteValue(reg, slot, typevalue.LiteralString(reg, "left")).
		WriteLenFloor(keys, root, 2).
		WritePlacement(owner, placement.Stack)
	right := domain.Lattice().Bottom().
		WriteValue(reg, slot, typevalue.LiteralInt(reg, 9)).
		WriteLenFloor(keys, root, 5).
		WritePlacement(owner, placement.OwnedHeap)

	accumulator := formalPublicationFactorAccumulator{domain: domain}
	for _, input := range []state.State{left, right} {
		values, factors := formalPublicationFactorTupleForTest(t, domain, input)
		if err := accumulator.join(values, factors); err != nil {
			t.Fatal(err)
		}
	}
	got, joined, err := accumulator.compose()
	if err != nil || !joined {
		t.Fatalf("factor accumulator = joined:%v err:%v", joined, err)
	}
	want := domain.Lattice().Join(left, right)
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("fused factor publication diverged from the corresponding State semantic join")
	}
}
