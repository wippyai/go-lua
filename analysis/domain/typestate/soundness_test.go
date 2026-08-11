package typestate

import "testing"

// TestTypestateWidenRankIsExactAndStrictForJoin proves the measure covers all
// default-holder state × duty coordinates and all three count alternatives.
func TestTypestateWidenRankIsExactAndStrictForJoin(t *testing.T) {
	algebra, schema, keys := typestateSoundnessSetup(t)
	key := keys[0]
	protocol, ok := schema.protocol(key)
	if !ok {
		t.Fatal("key protocol")
	}
	contract, ok := schema.universe.source.Boundary().Target()
	if !ok {
		t.Fatal("target contract")
	}
	holders := schema.holderCount(key)
	if holders != 1 {
		t.Fatalf("default-holder support count = %d, want 1", holders)
	}
	wantCapacity := uint64(contract.StateCount(protocol)) * uint64(DutyUnknown) * uint64(holders) * 3
	if got := algebra.WidenRank(key, algebra.Default(), 0); got != wantCapacity {
		t.Fatalf("default widen capacity = %d, want %d", got, wantCapacity)
	}
	coordinate := firstCoordinateForKey(t, schema, key)
	one, ok := algebra.Of(key, Entry{State: coordinate.State, Duty: DutyLocal, Holder: coordinate.Holder, Count: CountOne})
	if !ok {
		t.Fatal("one alternative")
	}
	zero, ok := algebra.Of(key, Entry{State: coordinate.State, Duty: DutyLocal, Holder: coordinate.Holder, Count: CountZero})
	if !ok {
		t.Fatal("zero alternative")
	}
	joined := algebra.Join(one, zero)
	before := algebra.WidenRank(key, one, 0)
	after := algebra.WidenRank(key, joined, 0)
	if before != wantCapacity-1 || after != wantCapacity-2 || after >= before {
		t.Fatalf("join rank = %d -> %d, want %d -> %d", before, after, wantCapacity-1, wantCapacity-2)
	}
}

// TestTypestateRejectsCrossKeyFactValues proves a family-wide carrier value
// cannot be supplied at a different resource key.
func TestTypestateRejectsCrossKeyFactValues(t *testing.T) {
	algebra, schema, keys := typestateSoundnessSetup(t)
	if len(keys) < 2 {
		t.Fatal("fixture has fewer than two keys")
	}
	foreignKey := keys[1]
	coordinate := firstCoordinateForKey(t, schema, foreignKey)
	foreign, ok := algebra.Of(foreignKey, Entry{State: coordinate.State, Duty: DutyLocal, Holder: coordinate.Holder, Count: CountOne})
	if !ok {
		t.Fatal("foreign value")
	}
	if algebra.AdmitsAt(keys[0], foreign) {
		t.Fatal("cross-key relation passed per-key admission")
	}
	if _, ok := algebra.Substitute(Fact{Key: keys[0], Value: foreign}, Substitution{}); ok {
		t.Fatal("substitution accepted a source fact with mismatched key and value resource")
	}
}

func typestateSoundnessSetup(t testing.TB) (Algebra, Schema, []Key) {
	t.Helper()
	source := typestateProtocolSource(t)
	schema, ok := NewSchema(source)
	if !ok {
		t.Fatal("schema")
	}
	algebra, ok := NewAlgebra(schema)
	if !ok {
		t.Fatal("algebra")
	}
	keys := make([]Key, schema.KeyCount())
	for index := range keys {
		key, ok := schema.KeyAt(index)
		if !ok {
			t.Fatal("key")
		}
		keys[index] = key
	}
	return algebra, schema, keys
}

func firstCoordinateForKey(t testing.TB, schema Schema, key Key) Coordinate {
	t.Helper()
	for index := 0; index < schema.CoordinateCount(); index++ {
		candidateKey, coordinate, ok := schema.CoordinateAt(index)
		if !ok {
			t.Fatal("coordinate")
		}
		if candidateKey == key {
			return coordinate
		}
	}
	t.Fatal("key coordinate")
	return Coordinate{}
}
