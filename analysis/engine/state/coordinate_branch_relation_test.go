package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCoordinateBranchMutationsEqualCanonicalStateWrites(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	path := keys.FromPath(pathdom.NewPath(symbol.ID(77001), "x"))
	stateKey, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(path))
	if !ok {
		t.Fatal("path has no StateKey")
	}
	base := Reachable(State{})

	assertMutation := func(name string, mutation CoordinateBranchMutation, want State) {
		t.Helper()
		lane := mutation.Slot().Family().Lane()
		factors, err := domain.DecomposeLanes(base, []ProductLane{lane})
		if err != nil || len(factors) != 1 {
			t.Fatalf("%s factor: %v", name, err)
		}
		skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], mutation.Slot().Family(), keys)
		if err != nil {
			t.Fatalf("%s decompose: %v", name, err)
		}
		current, err := domain.CoordinateDefault(skeleton, mutation.Slot())
		if err != nil {
			t.Fatalf("%s default: %v", name, err)
		}
		nextSkeleton, nextScalar, err := domain.ApplyCoordinateBranchMutation(mutation, skeleton, current)
		if err != nil {
			t.Fatalf("%s apply: %v", name, err)
		}
		gotFactor, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{nextSkeleton}, [][]CoordinateScalarFactor{{nextScalar}})
		if err != nil {
			t.Fatalf("%s compose: %v (prior=%d)", name, err, len(scalars))
		}
		wantFactor, err := domain.DecomposeLanes(want, []ProductLane{lane})
		if err != nil || len(wantFactor) != 1 {
			t.Fatalf("%s canonical factor: %v", name, err)
		}
		equal, err := domain.LaneEqual(gotFactor, wantFactor[0])
		if err != nil || !equal {
			t.Fatalf("%s differs from canonical State write: equal=%t err=%v", name, equal, err)
		}
	}

	length, err := domain.PrepareCoordinateBranchBound(CoordinateBoundLength, CoordinateBoundLower, keys, path, 7)
	if err != nil {
		t.Fatal(err)
	}
	assertMutation("length-floor", length, base.WriteLenFloor(keys, stateKey, 7))

	floor, err := domain.PrepareCoordinateBranchBound(CoordinateBoundValue, CoordinateBoundLower, keys, path, -3)
	if err != nil {
		t.Fatal(err)
	}
	assertMutation("numeric-floor", floor, base.WriteNumFloor(keys, stateKey, -3))

	ceil, err := domain.PrepareCoordinateBranchBound(CoordinateBoundValue, CoordinateBoundUpper, keys, path, 11)
	if err != nil {
		t.Fatal(err)
	}
	assertMutation("numeric-ceiling", ceil, base.WriteNumCeil(keys, stateKey, 11))

	other := keys.FromPath(pathdom.NewPath(symbol.ID(77002), "y"))
	otherState, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(other))
	if !ok {
		t.Fatal("other path has no StateKey")
	}
	constraint := RelConstraint{CoA: 1, A: RelValueOperand(stateKey), C: RelValueOperand(otherState), K: 5}
	relation, err := domain.PrepareCoordinateBranchConstraint(keys, constraint)
	if err != nil {
		t.Fatal(err)
	}
	assertMutation("difference", relation, base.WriteScaledConstraint(1, constraint.A, 0, RelOperand{}, constraint.C, constraint.K))
}

func TestCoordinateBranchMutationsAcceptRegisteredExtremeSkeletons(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	path := keys.FromPath(pathdom.NewPath(symbol.ID(77011), "xs"))
	other := keys.FromPath(pathdom.NewPath(symbol.ID(77012), "ys"))
	left, leftOK := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(path))
	right, rightOK := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(other))
	if !leftOK || !rightOK {
		t.Fatal("paths have no StateKey")
	}
	relation := RelConstraint{CoA: 1, A: RelValueOperand(left), C: RelValueOperand(right), K: 5}

	mutations := []CoordinateBranchMutation{}
	for _, prepare := range []func() (CoordinateBranchMutation, error){
		func() (CoordinateBranchMutation, error) {
			return domain.PrepareCoordinateBranchBound(CoordinateBoundLength, CoordinateBoundLower, keys, path, 7)
		},
		func() (CoordinateBranchMutation, error) {
			return domain.PrepareCoordinateBranchBound(CoordinateBoundValue, CoordinateBoundLower, keys, path, -3)
		},
		func() (CoordinateBranchMutation, error) {
			return domain.PrepareCoordinateBranchConstraint(keys, relation)
		},
	} {
		mutation, err := prepare()
		if err != nil {
			t.Fatal(err)
		}
		mutations = append(mutations, mutation)
	}

	for _, mutation := range mutations {
		for _, extreme := range []struct {
			name string
			make func(CoordinateFamily, *keyspace.KeySpace) (CoordinateFamilySkeleton, error)
		}{
			{name: "bottom", make: domain.CoordinateSkeletonBottom},
			{name: "top", make: domain.CoordinateSkeletonTop},
			{name: "widened", make: func(family CoordinateFamily, keys *keyspace.KeySpace) (CoordinateFamilySkeleton, error) {
				bottom, err := domain.CoordinateSkeletonBottom(family, keys)
				if err != nil {
					return CoordinateFamilySkeleton{}, err
				}
				top, err := domain.CoordinateSkeletonTop(family, keys)
				if err != nil {
					return CoordinateFamilySkeleton{}, err
				}
				return domain.CoordinateSkeletonWiden(bottom, top)
			}},
		} {
			t.Run(string(mutation.Slot().Family().ID())+"/"+extreme.name, func(t *testing.T) {
				skeleton, err := extreme.make(mutation.Slot().Family(), keys)
				if err != nil {
					t.Fatal(err)
				}
				current, err := domain.CoordinateDefault(skeleton, mutation.Slot())
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := domain.ApplyCoordinateBranchMutation(mutation, skeleton, current); err != nil {
					t.Fatalf("registered extreme rejected by its branch law: %v", err)
				}
			})
		}
	}
}
