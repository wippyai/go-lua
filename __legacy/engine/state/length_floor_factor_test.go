package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func TestLengthFloorFactorMatchesCanonicalConcreteWrite(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	stateKey := pathaddr.StateKey("sym300@1.items")
	path, ok := keys.InternStateKey(stateKey)
	if !ok {
		t.Fatal("length path")
	}
	input := Reachable(State{}).WriteLenFloor(keys, stateKey, 2)
	domain := RegisteredProductDomain(reg)
	plan, err := domain.PrepareLengthFloorFactorPlan(keys, path, 5)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := domain.LengthFloorFactorCoordinateWrites(plan)
	if err != nil || len(writes) != 1 {
		t.Fatalf("length floor coordinate writes = %#v, err=%v", writes, err)
	}
	lane, err := domain.LengthFloorFactorLane(plan)
	if err != nil || writes[0].Family().Lane() != lane {
		t.Fatalf("length floor coordinate lane = %v, want %v (err=%v)", writes[0].Family().Lane(), lane, err)
	}
	got, err := domain.ApplyLengthFloor(plan, input)
	if err != nil {
		t.Fatal(err)
	}
	want := input.WriteLenFloor(keys, stateKey, 5)
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native length floor diverged from canonical concrete write")
	}
}
