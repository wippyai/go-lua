package sequence

import "testing"

type groupedFixture struct {
	groups []Value
	fixed  []uint32
	tail   uint32
	has    bool
	empty  Value
}

func (fixture groupedFixture) GroupCount() int               { return len(fixture.groups) }
func (fixture groupedFixture) GroupAt(index int) Value       { return fixture.groups[index] }
func (fixture groupedFixture) FixedCount() int               { return len(fixture.fixed) }
func (fixture groupedFixture) FixedGroupAt(index int) uint32 { return fixture.fixed[index] }
func (fixture groupedFixture) TailGroup() (uint32, bool)     { return fixture.tail, fixture.has }
func (fixture groupedFixture) Empty() Value                  { return fixture.empty }

func TestAssembleGroupedRetainsRepeatedSourceDiagonal(t *testing.T) {
	pair := FromModes(labels, ClosedMode(hA), ClosedMode(hB))
	got := AssembleGrouped(labels, groupedFixture{
		groups: []Value{pair}, fixed: []uint32{0, 0}, empty: FromModes(labels, ClosedMode()),
	})
	words := concrete(got, 2)
	if !words[wordKey([]Handle{hA, hA})] || !words[wordKey([]Handle{hB, hB})] {
		t.Fatalf("diagonal results missing: %v", words)
	}
	if words[wordKey([]Handle{hA, hB})] || words[wordKey([]Handle{hB, hA})] {
		t.Fatalf("repeated source invented a cross-product: %v", words)
	}
}

func TestAssembleGroupedMatchesBoundedCorrelatedConcreteLaw(t *testing.T) {
	empty := FromModes(labels, ClosedMode())
	for _, fixture := range []groupedFixture{
		{groups: []Value{FromModes(labels, ClosedMode(hA), ClosedMode(hB))}, fixed: []uint32{0, 0}, empty: empty},
		{groups: []Value{FromModes(labels, KnownMode(nil, hA, []Handle{hB}))}, fixed: []uint32{0}, tail: 0, has: true, empty: empty},
		{
			groups: []Value{
				FromModes(labels, ClosedMode(hA), ClosedMode(hB)),
				FromModes(labels, KnownMode(nil, hC, []Handle{hA})),
			},
			fixed: []uint32{0, 1, 0}, tail: 1, has: true, empty: empty,
		},
	} {
		got := AssembleGrouped(labels, fixture)
		want := concreteGrouped(fixture, 4)
		if actual := concrete(got, 4); !sameConcrete(actual, want) {
			t.Fatalf("grouped concrete law mismatch: got=%v want=%v", actual, want)
		}
	}
}

func TestAssembleGroupedRetainsNonAdjacentRepeatedSource(t *testing.T) {
	left := FromModes(labels, ClosedMode(hA), ClosedMode(hB))
	right := FromModes(labels, ClosedMode(hC))
	got := AssembleGrouped(labels, groupedFixture{
		groups: []Value{left, right}, fixed: []uint32{0, 1, 0}, empty: FromModes(labels, ClosedMode()),
	})
	words := concrete(got, 3)
	if !words[wordKey([]Handle{hA, hC, hA})] || !words[wordKey([]Handle{hB, hC, hB})] {
		t.Fatalf("non-adjacent diagonal results missing: %v", words)
	}
	if words[wordKey([]Handle{hA, hC, hB})] || words[wordKey([]Handle{hB, hC, hA})] {
		t.Fatalf("non-adjacent repeated source crossed: %v", words)
	}
}

func TestAssembleGroupedKeepsDistinctEqualPacksIndependent(t *testing.T) {
	choices := FromModes(labels, ClosedMode(hA), ClosedMode(hB))
	got := AssembleGrouped(labels, groupedFixture{
		groups: []Value{choices, choices}, fixed: []uint32{0, 1}, empty: FromModes(labels, ClosedMode()),
	})
	words := concrete(got, 2)
	for _, word := range [][]Handle{{hA, hA}, {hA, hB}, {hB, hA}, {hB, hB}} {
		if !words[wordKey(word)] {
			t.Fatalf("independent group result %v missing: %v", word, words)
		}
	}
}

func TestAssembleGroupedConditionsFixedTailOnSameStarRealization(t *testing.T) {
	pack := FromModes(labels, KnownMode(nil, hA, []Handle{hB}))
	got := AssembleGrouped(labels, groupedFixture{
		groups: []Value{pack}, fixed: []uint32{0}, tail: 0, has: true, empty: FromModes(labels, ClosedMode()),
	})
	words := concrete(got, 6)
	if !words[wordKey([]Handle{hB, hB})] || !words[wordKey([]Handle{hA, hA, hB})] || !words[wordKey([]Handle{hA, hA, hA, hB})] {
		t.Fatalf("correlated zero/positive tail cases missing: %v", words)
	}
	if words[wordKey([]Handle{hA, hB})] {
		t.Fatalf("fixed+tail admitted impossible zero/positive mix: %v", words)
	}
}

func TestAssembleGroupedAllUniqueAgreesWithAssembleAndGroupOrder(t *testing.T) {
	left := FromModes(labels, ClosedMode(hA), ClosedMode(hB))
	right := FromModes(labels, ClosedMode(hC))
	tail := FromModes(labels, KnownMode(nil, hB, []Handle{hC}))
	unique := AssembleGrouped(labels, groupedFixture{
		groups: []Value{left, right, tail}, fixed: []uint32{0, 1}, tail: 2, has: true, empty: FromModes(labels, ClosedMode()),
	})
	want := Assemble(labels, []Value{left, right}, tail)
	if !Equal(labels, unique, want) {
		t.Fatalf("unique grouped assembly=%#v, ordinary assembly=%#v", unique.Modes(), want.Modes())
	}
	permuted := AssembleGrouped(labels, groupedFixture{
		groups: []Value{tail, left, right}, fixed: []uint32{1, 2}, tail: 0, has: true, empty: FromModes(labels, ClosedMode()),
	})
	if !Equal(labels, unique, permuted) || Hash(labels, unique) != Hash(labels, permuted) {
		t.Fatalf("group order changed result: %#v / %#v", unique.Modes(), permuted.Modes())
	}
}

func TestAssembleGroupedEmptyAndOpaqueContainment(t *testing.T) {
	empty := FromModes(labels, ClosedMode())
	if got := AssembleGrouped(labels, groupedFixture{empty: empty}); !Equal(labels, got, empty) {
		t.Fatalf("empty grouped assembly=%#v", got.Modes())
	}
	opaque := FromModes(labels, OpaqueMode(nil, []Handle{hB}))
	got := AssembleGrouped(labels, groupedFixture{
		groups: []Value{opaque}, fixed: []uint32{0}, tail: 0, has: true, empty: empty,
	})
	words := concrete(got, 5)
	if !words[wordKey([]Handle{hB, hB})] || !words[wordKey([]Handle{hA, hA, hB})] {
		t.Fatalf("opaque correlated containment missing: %v", words)
	}
	if words[wordKey([]Handle{hA, hB})] {
		t.Fatalf("opaque correlated tail admitted impossible short mix: %v", words)
	}
	top := AssembleGrouped(labels, groupedFixture{
		groups: []Value{Top()}, fixed: []uint32{0}, tail: 0, has: true, empty: empty,
	})
	topWords := concrete(top, 4)
	if !topWords[wordKey([]Handle{hA, hA})] || topWords[wordKey([]Handle{hA})] {
		t.Fatalf("top correlated containment=%v", topWords)
	}
}

func TestAssembleGroupedRejectsMalformedLayoutAndPropagatesBottom(t *testing.T) {
	empty := FromModes(labels, ClosedMode())
	if got := AssembleGrouped(labels, groupedFixture{groups: []Value{FromModes(labels, ClosedMode(hA))}, fixed: []uint32{1}, empty: empty}); !got.IsBottom() {
		t.Fatal("out-of-range group accepted")
	}
	if got := AssembleGrouped(labels, groupedFixture{groups: []Value{Bottom()}, fixed: []uint32{0}, empty: empty}); !got.IsBottom() {
		t.Fatal("bottom group lost")
	}
}

// concreteGrouped is an independent bounded interpreter for the grouped
// source law. Each group chooses exactly one concrete source word, after
// which every repeated fixed slot and the optional tail reuse that word.
func concreteGrouped(fixture groupedFixture, limit int) map[string]bool {
	words := make([][][]Handle, len(fixture.groups))
	for index, group := range fixture.groups {
		for key := range concrete(group, limit) {
			words[index] = append(words[index], parseWord(key))
		}
	}
	selected := make([][]Handle, len(words))
	result := make(map[string]bool)
	var choose func(int)
	choose = func(index int) {
		if index != len(words) {
			for _, word := range words[index] {
				selected[index] = word
				choose(index + 1)
			}
			return
		}
		output := make([]Handle, 0, len(fixture.fixed)+limit)
		for _, raw := range fixture.fixed {
			word := selected[int(raw)]
			label := hNil
			if len(word) != 0 {
				label = word[0]
			}
			output = append(output, label)
		}
		if fixture.has {
			output = append(output, selected[int(fixture.tail)]...)
		}
		if len(output) <= limit {
			result[wordKey(output)] = true
		}
	}
	choose(0)
	return result
}
