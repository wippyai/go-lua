package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func TestNumericOrdinaryLanesRegisterExactMeet(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	x := pathaddr.StateKey("local:x")
	leftOnly := pathaddr.StateKey("local:left")
	rightOnly := pathaddr.StateKey("local:right")

	tests := []struct {
		name  string
		lane  LaneID
		left  int64
		right int64
		want  int64
		write func(State, pathaddr.StateKey, int64) State
		read  func(State, pathaddr.StateKey) (int64, bool)
	}{
		{
			name: "length-floor", lane: LaneLenFloors, left: 2, right: 4, want: 4,
			write: func(s State, key pathaddr.StateKey, value int64) State { return s.WriteLenFloor(keys, key, value) },
			read:  func(s State, key pathaddr.StateKey) (int64, bool) { return s.ReadLenFloor(keys, key) },
		},
		{
			name: "numeric-floor", lane: LaneNumFloors, left: 2, right: 4, want: 4,
			write: func(s State, key pathaddr.StateKey, value int64) State { return s.WriteNumFloor(keys, key, value) },
			read:  func(s State, key pathaddr.StateKey) (int64, bool) { return s.ReadNumFloor(keys, key) },
		},
		{
			name: "numeric-ceiling", lane: LaneNumCeils, left: 8, right: 5, want: 5,
			write: func(s State, key pathaddr.StateKey, value int64) State { return s.WriteNumCeil(keys, key, value) },
			read:  func(s State, key pathaddr.StateKey) (int64, bool) { return s.ReadNumCeil(keys, key) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{test.lane})
			if err != nil {
				t.Fatal(err)
			}
			lane, ok := domain.ProductLane(test.lane)
			if !ok {
				t.Fatalf("lane %q is not registered", test.lane)
			}
			leftState := test.write(test.write(Reachable(State{}), x, test.left), leftOnly, test.left)
			rightState := test.write(test.write(Reachable(State{}), x, test.right), rightOnly, test.right)
			left := mustOnlyLaneFactor(t, domain, leftState)
			right := mustOnlyLaneFactor(t, domain, rightState)

			met, err := domain.LaneMeet(left, right)
			if err != nil {
				t.Fatalf("registered Meet: %v", err)
			}
			metState, err := domain.Compose([]LaneFactor{met})
			if err != nil {
				t.Fatal(err)
			}
			if got, present := test.read(metState, x); !present || got != test.want {
				t.Fatalf("shared coordinate = (%d,%t), want (%d,true)", got, present, test.want)
			}
			for _, key := range []pathaddr.StateKey{leftOnly, rightOnly} {
				if got, present := test.read(metState, key); !present || got == 0 {
					t.Fatalf("Meet dropped conjoined support %q: (%d,%t)", key, got, present)
				}
			}

			joined, err := domain.LaneJoin(left, right)
			if err != nil {
				t.Fatal(err)
			}
			meetAbsorption, err := domain.LaneMeet(left, joined)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorEqual(t, domain, meetAbsorption, left, "meet absorption")
			joinAbsorption, err := domain.LaneJoin(left, met)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorEqual(t, domain, joinAbsorption, left, "join absorption")

			bottom, err := domain.LaneBottom(lane)
			if err != nil {
				t.Fatal(err)
			}
			bottomMeet, err := domain.LaneMeet(bottom, left)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorEqual(t, domain, bottomMeet, bottom, "bottom absorption")
			top, err := domain.LaneTop(lane)
			if err != nil {
				t.Fatal(err)
			}
			topMeet, err := domain.LaneMeet(top, left)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorEqual(t, domain, topMeet, left, "top identity")
		})
	}
}
