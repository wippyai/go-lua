package runtimekind

import "testing"

// The vocabulary's partitions are declared once, here. These laws state what a
// consumer is entitled to assume when it reads one instead of restating the
// member list or walking an ordinal range: each partition is exactly the union
// of the families it names, the partitions tile the closed vocabulary, and the
// enumeration over a Set visits every member once in vocabulary order.
//
// A family inserted into Kind and not placed in a partition is a rejected build
// here rather than a family a consumer silently stops classifying.

func union(kinds ...Kind) Set {
	var set Set
	for _, kind := range kinds {
		set |= Bit(kind)
	}
	return set
}

// TestPartitionsAreTheUnionOfTheFamiliesTheyName states each partition against
// the public singleton constructor, so a partition cannot drift into a bit
// pattern that names a different family than the one it documents.
func TestPartitionsAreTheUnionOfTheFamiliesTheyName(t *testing.T) {
	for _, law := range []struct {
		name      string
		partition Set
		families  []Kind
	}{
		{"Reference", Reference, []Kind{Table, Function, Thread, Userdata}},
		{"Scalar", Scalar, []Kind{Boolean, Number, String}},
		{"Opaque", Opaque, []Kind{Thread, Userdata}},
		{"NonNil", NonNil, []Kind{Boolean, Number, String, Table, Function, Thread, Userdata}},
	} {
		want := union(law.families...)
		if law.partition != want {
			t.Fatalf("partition %s = %#x, but the families it names union to %#x", law.name, law.partition, want)
		}
		if !law.partition.Valid() {
			t.Fatalf("partition %s carries an out-of-vocabulary bit", law.name)
		}
		for _, family := range law.families {
			if !law.partition.Contains(family) {
				t.Fatalf("partition %s does not contain family %d it names", law.name, family)
			}
		}
	}
}

// TestNonNilIsEveryFamilyThatIsNotNil derives the partition from the semantic
// predicate over the whole Kind space rather than from a member list, so a
// family added to the vocabulary joins it without an edit here.
func TestNonNilIsEveryFamilyThatIsNotNil(t *testing.T) {
	var derived Set
	for candidate := 0; candidate <= int(^uint8(0)); candidate++ {
		kind := Kind(candidate)
		if kind.Valid() && kind != Nil {
			derived |= Bit(kind)
		}
	}
	if NonNil != derived {
		t.Fatalf("NonNil = %#x, but scanning every family that is not nil yields %#x", NonNil, derived)
	}
	if NonNil.Contains(Nil) || NonNil|Bit(Nil) != All {
		t.Fatalf("NonNil = %#x is not the closed vocabulary %#x less nil", NonNil, All)
	}
}

// TestScalarAndReferenceTileTheNonNilVocabulary states the classification is
// total and exclusive: every family that is not nil is scalar or reference and
// never both, so a consumer that branches on one of them leaves no family
// unclassified.
func TestScalarAndReferenceTileTheNonNilVocabulary(t *testing.T) {
	if Scalar&Reference != 0 {
		t.Fatalf("scalar and reference families overlap at %#x", Scalar&Reference)
	}
	if Scalar|Reference != NonNil {
		t.Fatalf("scalar and reference families union to %#x, not the non-nil vocabulary %#x", Scalar|Reference, NonNil)
	}
	if Bit(Nil)|Scalar|Reference != All {
		t.Fatalf("nil, scalar and reference families do not tile the closed vocabulary %#x", All)
	}
	for kind := Invalid + 1; kind < Count; kind++ {
		classified := 0
		for _, partition := range []Set{Bit(Nil), Scalar, Reference} {
			if partition.Contains(kind) {
				classified++
			}
		}
		if classified != 1 {
			t.Fatalf("family %d is classified by %d partitions, want exactly one", kind, classified)
		}
	}
}

// TestOpaqueFamiliesAreReferencesWithoutStructure states the opaque partition
// is a proper part of the reference partition, so a consumer reading it never
// silently drops a reference family the analyzer does model.
func TestOpaqueFamiliesAreReferencesWithoutStructure(t *testing.T) {
	if Opaque&^Reference != 0 {
		t.Fatalf("opaque families %#x include a non-reference family", Opaque&^Reference)
	}
	if Reference&^Opaque != union(Table, Function) {
		t.Fatalf("reference families outside the opaque partition = %#x, want table and function", Reference&^Opaque)
	}
	if Opaque&Scalar != 0 || Opaque.Contains(Nil) {
		t.Fatalf("opaque partition %#x reaches outside the reference families", Opaque)
	}
}

// TestSetEnumerationIsTheDenseWalkOfItsMembers states the density law an
// exhaustive consumer iteration rests on: the walk yields every family the set
// contains, each once, in vocabulary order, and stops immediately after.
func TestSetEnumerationIsTheDenseWalkOfItsMembers(t *testing.T) {
	for _, set := range []Set{0, All, NonNil, Scalar, Reference, Opaque, Bit(Nil), Bit(Number) | Bit(Userdata)} {
		var walked []Kind
		for index := 0; ; index++ {
			kind, ok := set.MemberAt(index)
			if !ok {
				break
			}
			walked = append(walked, kind)
		}
		if len(walked) != set.Members() {
			t.Fatalf("set %#x walks %d families but reports %d members", set, len(walked), set.Members())
		}
		var expected []Kind
		for kind := Invalid + 1; kind < Count; kind++ {
			if set.Contains(kind) {
				expected = append(expected, kind)
			}
		}
		if len(walked) != len(expected) {
			t.Fatalf("set %#x walks %d families, but contains %d", set, len(walked), len(expected))
		}
		for position, kind := range walked {
			if kind != expected[position] {
				t.Fatalf("set %#x walk position %d is family %d, want %d", set, position, kind, expected[position])
			}
		}
	}
	if _, ok := All.MemberAt(-1); ok {
		t.Fatal("a negative position named a family")
	}
	if _, ok := Set(1 << 8).MemberAt(0); ok {
		t.Fatal("an out-of-vocabulary set enumerated a family")
	}
	if Set(1<<8).Members() != 0 {
		t.Fatal("an out-of-vocabulary set reported members")
	}
}

// TestSetEnumerationAllocatesNothing states that walking a partition costs no
// allocation, so a consumer visiting every family on a hot path reaches for the
// declared partition rather than a slice literal of its own.
func TestSetEnumerationAllocatesNothing(t *testing.T) {
	if allocations := testing.AllocsPerRun(100, func() {
		for index := 0; ; index++ {
			kind, ok := NonNil.MemberAt(index)
			if !ok {
				break
			}
			if !kind.Valid() {
				t.Fatal("enumeration yielded an inadmissible family")
			}
		}
	}); allocations != 0 {
		t.Fatalf("partition enumeration allocated %v times per run", allocations)
	}
}
