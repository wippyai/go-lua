package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type exactRekeyKeys struct {
	from, to      *keyspace.KeySpace
	one, multiple keyspace.Key
	wantOne       keyspace.Key
	wantMultiple  keyspace.Key
}

func exactRekeyCollisionKeys(t *testing.T) exactRekeyKeys {
	t.Helper()
	from, to := keyspace.New(), keyspace.New()

	// These keys deliberately have the same legacy string spelling. Structural
	// import must retain the distinction between one punctuation-bearing field
	// and two ordinary fields.
	onePath := path.Path{Symbol: symbol.ID(901), Version: 3}.Field("left.right")
	multiplePath := path.Path{Symbol: symbol.ID(901), Version: 3}.Field("left").Field("right")
	one := from.FromPath(onePath)
	multiple := from.FromPath(multiplePath)
	if got, want := from.Format(one), from.Format(multiple); got != want {
		t.Fatalf("collision setup formats differ: %q != %q", got, want)
	}
	if one == multiple {
		t.Fatal("collision setup lost structural segment identity")
	}

	// Seed the destination in a different order from the source so correctness
	// cannot accidentally depend on dense segment ids or insertion order.
	wantMultiple := to.FromPath(multiplePath)
	_ = to.FromPath(path.Path{Symbol: symbol.ID(999), Version: 8}.Field("distractor"))
	wantOne := to.FromPath(onePath)
	if wantOne == wantMultiple {
		t.Fatal("destination collapsed structurally distinct keys")
	}
	return exactRekeyKeys{
		from: from, to: to, one: one, multiple: multiple,
		wantOne: wantOne, wantMultiple: wantMultiple,
	}
}

func TestLenFloorRekeyUsesExactStructuralImport(t *testing.T) {
	keys := exactRekeyCollisionKeys(t)
	lane, changed := (lenFloorLane{}).write(keys.one, 11)
	if !changed {
		t.Fatal("first length-floor write did not change the lane")
	}
	lane, changed = lane.write(keys.multiple, 22)
	if !changed {
		t.Fatal("second length-floor write collapsed with the first")
	}

	rekeyed, ok := lane.rekey(keys.from, keys.to)
	if !ok {
		t.Fatal("exact length-floor rekey failed")
	}
	assertLenFloor(t, rekeyed, keys.wantOne, 11)
	assertLenFloor(t, rekeyed, keys.wantMultiple, 22)
}

func TestNumBoundRekeyUsesExactStructuralImport(t *testing.T) {
	for _, test := range []struct {
		name      string
		direction numbound.Direction
		one       int64
		multiple  int64
	}{
		{name: "floor", direction: numbound.Lower, one: -11, multiple: 22},
		{name: "ceil", direction: numbound.Upper, one: 11, multiple: -22},
	} {
		t.Run(test.name, func(t *testing.T) {
			keys := exactRekeyCollisionKeys(t)
			lane, changed := (numBoundLane{}).Write(keys.one, test.one, test.direction)
			if !changed {
				t.Fatal("first numeric-bound write did not change the lane")
			}
			lane, changed = lane.Write(keys.multiple, test.multiple, test.direction)
			if !changed {
				t.Fatal("second numeric-bound write collapsed with the first")
			}

			rekeyed, ok := numBoundRekey(lane, keys.from, keys.to)
			if !ok {
				t.Fatal("exact numeric-bound rekey failed")
			}
			assertNumBound(t, rekeyed, keys.wantOne, test.one)
			assertNumBound(t, rekeyed, keys.wantMultiple, test.multiple)
		})
	}
}

func TestUserLatticeRekeyUsesExactStructuralImport(t *testing.T) {
	keys := exactRekeyCollisionKeys(t)
	const slot userlattice.AxisSlot = 7
	lane := userLatticeLane{values: map[userLatticeKey]userlattice.Element{
		{axis: slot, path: keys.one}:      3,
		{axis: slot, path: keys.multiple}: 5,
	}}

	rekeyed, ok := lane.rekey(keys.from, keys.to)
	if !ok {
		t.Fatal("exact user-lattice rekey failed")
	}
	if got := rekeyed.values[userLatticeKey{axis: slot, path: keys.wantOne}]; got != 3 {
		t.Fatalf("single-field user element = %d, want 3", got)
	}
	if got := rekeyed.values[userLatticeKey{axis: slot, path: keys.wantMultiple}]; got != 5 {
		t.Fatalf("multi-field user element = %d, want 5", got)
	}
}

func TestExactLaneRekeyFailsWithoutPublishingPartialLane(t *testing.T) {
	from, foreign, to := keyspace.New(), keyspace.New(), keyspace.New()
	owned := from.FromPath(path.Path{Symbol: symbol.ID(17), Version: 1}.Field("owned"))
	foreignKey := foreign.FromPath(path.Path{Symbol: symbol.ID(18), Version: 1}.Field("foreign"))

	lenLane, _ := (lenFloorLane{}).write(owned, 1)
	lenLane, _ = lenLane.write(foreignKey, 2)
	if got, ok := lenLane.rekey(from, to); ok || !sameLenFloorLane(got, lenLane) {
		t.Fatalf("length-floor failure published a partial lane: ok=%v", ok)
	}

	numLane, _ := (numBoundLane{}).Write(owned, 1, numbound.Lower)
	numLane, _ = numLane.Write(foreignKey, 2, numbound.Lower)
	if got, ok := numBoundRekey(numLane, from, to); ok || !sameNumBoundLane(got, numLane) {
		t.Fatalf("numeric-bound failure published a partial lane: ok=%v", ok)
	}

	userLane := userLatticeLane{values: map[userLatticeKey]userlattice.Element{
		{axis: 1, path: owned}:      1,
		{axis: 1, path: foreignKey}: 2,
	}}
	if got, ok := userLane.rekey(from, to); ok || !userLatticeEqual(got, userLane) {
		t.Fatalf("user-lattice failure published a partial lane: ok=%v", ok)
	}

	if _, ok := lenLane.rekey(nil, to); ok {
		t.Fatal("length-floor rekey accepted a nil source keyspace")
	}
	if _, ok := numBoundRekey(numLane, from, nil); ok {
		t.Fatal("numeric-bound rekey accepted a nil destination keyspace")
	}
	if _, ok := userLane.rekey(nil, nil); ok {
		t.Fatal("user-lattice rekey accepted nil keyspaces")
	}
}

func TestExactLaneRekeyOnlyRequiresAuthorityForConcreteKeys(t *testing.T) {
	original := keyspace.New()
	shallowValue := *original
	shallow := &shallowValue
	if shallow.Valid() {
		t.Fatal("shallow-copied keyspace unexpectedly retained authority")
	}

	if _, ok := (lenFloorLane{}).rekey(shallow, shallow); ok {
		t.Fatal("key-free length-floor lane accepted invalid nonnil authority")
	}
	if _, ok := numBoundRekey(numBoundLane{}, shallow, shallow); ok {
		t.Fatal("key-free numeric-bound lane accepted invalid nonnil authority")
	}
	if _, ok := (userLatticeLane{top: true}).rekey(shallow, shallow); ok {
		t.Fatal("key-free top user-lattice lane accepted invalid nonnil authority")
	}

	key := original.FromPath(path.Path{Symbol: symbol.ID(23), Version: 1}.Field("owned"))
	lenLane, _ := (lenFloorLane{}).write(key, 1)
	numLane, _ := (numBoundLane{}).Write(key, 1, numbound.Lower)
	userLane := userLatticeLane{values: map[userLatticeKey]userlattice.Element{{axis: 1, path: key}: 1}}
	if _, ok := lenLane.rekey(shallow, shallow); ok {
		t.Fatal("concrete length-floor key accepted a shallow-copied authority")
	}
	if _, ok := numBoundRekey(numLane, shallow, shallow); ok {
		t.Fatal("concrete numeric-bound key accepted a shallow-copied authority")
	}
	if _, ok := userLane.rekey(shallow, shallow); ok {
		t.Fatal("concrete user-lattice key accepted a shallow-copied authority")
	}
}

func assertLenFloor(t *testing.T, lane lenFloorLane, key keyspace.Key, want int64) {
	t.Helper()
	got, ok := lane.read(key)
	if !ok || got != want {
		t.Fatalf("length floor = %d/%v, want %d/true", got, ok, want)
	}
}

func assertNumBound(t *testing.T, lane numBoundLane, key keyspace.Key, want int64) {
	t.Helper()
	got, ok := lane.Read(key)
	if !ok || got != want {
		t.Fatalf("numeric bound = %d/%v, want %d/true", got, ok, want)
	}
}

func sameLenFloorLane(left, right lenFloorLane) bool {
	return lenFloorMapDomain().Equal(left, right)
}

func sameNumBoundLane(left, right numBoundLane) bool {
	return numBoundLaneDomain(numbound.Lower, nil).Equal(left, right)
}
