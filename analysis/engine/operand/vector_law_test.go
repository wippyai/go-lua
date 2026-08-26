// vector_law_test.go states the laws of the vector a many-valued read is
// delivered as. They live beside the type rather than beside any one cursor:
// a member set and a Factor partition are two backings of the same view, and
// a reader that could tell them apart would be reading a second vector type.

package operand

import "testing"

// TestAMemberVectorIsTheSameViewItsReaderIsDeclaredToReceive states why a
// nested member set is a Summary read and not a second kind of read.
//
// The owner's MemberCount and MemberAt are a closed denominator, so a read
// spanning that set is a whole-vector read. Where its cells come from is an
// execution detail - one exact coordinate at a time rather than one Factor
// cursor - and the reader must not be able to tell, because a many-valued
// input is ONE vector argument under the reducer call shape. A second vector
// type would split every fold that consumes one into two spellings of the same
// parameter.
func TestAMemberVectorIsTheSameViewItsReaderIsDeclaredToReceive(t *testing.T) {
	cells := []MemberCell[uint64]{{Value: 4, Present: true}, {Present: false}, {Value: 9, Present: true}}
	vector, ok := NewMemberVector(cells)
	if !ok || !vector.Valid() {
		t.Fatalf("member vector = %t/%t", ok, vector.Valid())
	}
	if vector.Count() != len(cells) {
		t.Fatalf("member vector width = %d, want %d", vector.Count(), len(cells))
	}
	for index, want := range cells {
		value, present, addressed := vector.At(index)
		if !addressed || present != want.Present || value != want.Value {
			t.Fatalf("cell %d = %d/%t/%t, want %d/%t", index, value, present, addressed, want.Value, want.Present)
		}
	}
	// Absence is a cell, not a gap: the position of a cell is the ordinal its
	// owner declared it at, so an absent member keeps its place.
	if _, present, addressed := vector.At(1); !addressed || present {
		t.Fatal("an absent member was compacted away instead of kept as an absent cell")
	}
	if _, _, addressed := vector.At(len(cells)); addressed {
		t.Fatal("an index past the declared width names a cell")
	}
	if _, _, addressed := vector.At(-1); addressed {
		t.Fatal("a negative index names a cell")
	}
}

// TestAnUnopenedMemberVectorIsNotAVector keeps the view fail-closed. A vector
// nothing opened carries no cells, and a caller cannot mint one from a nil
// slice and have it read as an empty declared denominator.
func TestAnUnopenedMemberVectorIsNotAVector(t *testing.T) {
	if vector, ok := NewMemberVector[uint64](nil); ok || vector.Valid() || vector.Count() != 0 {
		t.Fatal("a nil member set opened a vector")
	}
	var unopened SummaryVector[uint64]
	if unopened.Valid() || unopened.Count() != 0 {
		t.Fatal("the zero vector reports itself live")
	}
	if _, _, addressed := unopened.At(0); addressed {
		t.Fatal("the zero vector addressed a cell")
	}
}

// TestAMemberVectorOfNoMembersIsAnEmptyDenominator states the one case that
// must stay distinguishable from an unopened vector: an owner whose member set
// is genuinely empty published a denominator with no cells, and its reader
// receives an open vector of width zero.
func TestAMemberVectorOfNoMembersIsAnEmptyDenominator(t *testing.T) {
	vector, ok := NewMemberVector([]MemberCell[uint64]{})
	if !ok || !vector.Valid() || vector.Count() != 0 {
		t.Fatalf("empty member set = %t/%t/%d", ok, vector.Valid(), vector.Count())
	}
	if _, _, addressed := vector.At(0); addressed {
		t.Fatal("an empty denominator addressed a cell")
	}
}
