package sequence

import (
	"fmt"
	"math/bits"
	"testing"
)

// flatLabels deliberately has no subtype or union operation. It models the
// only label order Pack is allowed to use: exact equality plus one TypeTop.
type flatLabels struct{}

var (
	hNil   = NewHandle(1, 1)
	hNever = NewHandle(1, 2)
	hTop   = NewHandle(1, 3)
	hA     = NewHandle(1, 4)
	hB     = NewHandle(1, 5)
	hC     = NewHandle(1, 6)
)

func (flatLabels) Equal(left, right Handle) bool { return left == right }
func (flatLabels) Hash(value Handle) uint64      { return value.raw }
func (flatLabels) Nil() Handle                   { return hNil }
func (flatLabels) Never() Handle                 { return hNever }
func (flatLabels) TypeTop() Handle               { return hTop }

var labels flatLabels

func TestFiniteAlternativeJoinRetainsTheNoLUBCounterexample(t *testing.T) {
	// In the old single-form carrier these two patterns have incomparable
	// upper bounds and no least one-tail upper bound. The result must remain a
	// finite disjunction; falling back to an opaque or label-joined mode would
	// erase exact language facts before the solver reaches a Mu boundary.
	left := FromModes(labels, KnownMode(nil, hA, []Handle{hA}))
	right := FromModes(labels, KnownMode([]Handle{hA}, hB, []Handle{hA}))
	joined := Join(labels, left, right)
	if joined.ModeCount() != 2 {
		t.Fatalf("join modes=%d, want two alternatives", joined.ModeCount())
	}
	if !LessEqual(labels, left, joined) || !LessEqual(labels, right, joined) {
		t.Fatal("alternative union is not an upper bound")
	}
	if joined.IsTop() {
		t.Fatal("ordinary join widened to PackTop")
	}
}

func TestJoinPreservesTupleCorrelation(t *testing.T) {
	left := FromModes(labels, ClosedMode(hA, hB))
	right := FromModes(labels, ClosedMode(hB, hC))
	joined := Join(labels, left, right)
	if joined.ModeCount() != 2 {
		t.Fatalf("correlated alternatives collapsed: %d", joined.ModeCount())
	}
	if got := joined.Modes(); len(got) != 2 || got[0].Kind() != ModeClosed || got[1].Kind() != ModeClosed {
		t.Fatalf("modes=%#v", got)
	}
	// The impossible cross-pairs must not appear in the concretization.
	words := concrete(joined, 2)
	if words[wordKey([]Handle{hA, hC})] || words[wordKey([]Handle{hB, hB})] {
		t.Fatalf("join invented cross-product: %v", words)
	}
}

func TestNormalizationOnlyUsesProvedRewrites(t *testing.T) {
	plus := FromModes(labels, KnownMode(nil, hA, []Handle{hA, hA, hB}))
	modes := plus.Modes()
	if len(modes) != 1 {
		t.Fatalf("modes=%d", len(modes))
	}
	if got := modes[0].Prefix(); !sameHandles(got, []Handle{hA, hA}) {
		t.Fatalf("prefix=%v", got)
	}
	if got := modes[0].Suffix(); !sameHandles(got, []Handle{hB}) {
		t.Fatalf("suffix=%v", got)
	}
	if tail, ok := modes[0].Tail(); !ok || tail != hA {
		t.Fatalf("tail=%v,%v", tail, ok)
	}

	neverStar := FromModes(labels, KnownMode([]Handle{hA}, hNever, []Handle{hB}))
	if got := neverStar.Modes(); len(got) != 1 || got[0].Kind() != ModeClosed || !sameHandles(got[0].Prefix(), []Handle{hA, hB}) {
		t.Fatalf("Never*=%#v", got)
	}
	if got := FromModes(labels, ClosedMode(hA, hNever)); !got.IsBottom() {
		t.Fatal("fixed Never did not make its alternative empty")
	}
	if got := FromModes(labels, OpaqueMode(nil, nil)); !got.IsTop() {
		t.Fatal("unframed opaque mode did not canonicalize to PackTop")
	}
}

func TestFlatTopLabelAndSuffixContainment(t *testing.T) {
	exact := FromModes(labels, ClosedMode(hA))
	topLabel := FromModes(labels, ClosedMode(hTop))
	if !LessEqual(labels, exact, topLabel) || LessEqual(labels, topLabel, exact) {
		t.Fatal("flat TypeTop label order is wrong")
	}
	knownTop := FromModes(labels, KnownMode(nil, hTop, []Handle{hB}))
	if modes := knownTop.Modes(); len(modes) != 1 || modes[0].Kind() != ModeKnown {
		t.Fatalf("known TypeTop tail was split or erased: %#v", modes)
	}

	// Closed words are covered only when they are actually a feasible length
	// alignment of the open mode. This is the safe cross-skeleton inclusion
	// needed for suffix-bearing target results.
	word := FromModes(labels, ClosedMode(hA, hB, hC))
	open := FromModes(labels, KnownMode([]Handle{hA}, hB, []Handle{hC}))
	if !LessEqual(labels, word, open) {
		t.Fatal("closed word should be included by its suffix-bearing open mode")
	}
	if LessEqual(labels, FromModes(labels, ClosedMode(hA, hB)), open) {
		t.Fatal("non-feasible suffix alignment was accepted")
	}
}

func TestDemandedPositionProjectionAndLuaFraming(t *testing.T) {
	open := FromModes(labels, KnownMode([]Handle{hA}, hB, []Handle{hC}))
	assertFixedAt := func(index int, want Value) {
		t.Helper()
		if got := FixedAt(labels, open, index); !Equal(labels, got, want) {
			t.Fatalf("FixedAt(%d)=%#v, want %#v", index, got.Modes(), want.Modes())
		}
	}
	assertFixedAt(0, FromModes(labels, ClosedMode(hA)))
	// At the first post-prefix position, the tail may be nonempty or the
	// suffix may begin immediately. Later positions also admit nil fill.
	assertFixedAt(1, FromModes(labels, ClosedMode(hB), ClosedMode(hC)))
	assertFixedAt(2, FromModes(labels, ClosedMode(hB), ClosedMode(hC), ClosedMode(hNil)))

	if got := Scalar(labels, FromModes(labels, ClosedMode())); !Equal(labels, got, FromModes(labels, ClosedMode(hNil))) {
		t.Fatalf("empty scalar=%#v", got.Modes())
	}
	if got := FixedAt(labels, FromModes(labels, ClosedMode(hA)), 2); !Equal(labels, got, FromModes(labels, ClosedMode(hNil))) {
		t.Fatalf("nil fill=%#v", got.Modes())
	}

	// Non-final terms scalarize; a final expression keeps its entire tail.
	final := FromModes(labels, KnownMode(nil, hB, []Handle{hC}))
	if got := Scalar(labels, final); !Equal(labels, got, FromModes(labels, ClosedMode(hB), ClosedMode(hC))) {
		t.Fatalf("non-final scalar=%#v", got.Modes())
	}
}

func TestAlternativeOrderDoesNotAffectValue(t *testing.T) {
	first := FromModes(labels, ClosedMode(hA, hB), KnownMode(nil, hC, []Handle{hA}))
	second := FromModes(labels, KnownMode(nil, hC, []Handle{hA}), ClosedMode(hA, hB))
	if !Equal(labels, first, second) || Hash(labels, first) != Hash(labels, second) {
		t.Fatalf("order leaked into value: %#v / %#v", first.Modes(), second.Modes())
	}
}

func TestIndexedDominanceMatchesPairwiseReference(t *testing.T) {
	// The indexed normalizer is an optimization of the same containment
	// relation, not a new approximation.  This corpus forces all three legal
	// dominance shapes: same-width closed words, same-frame open modes, and
	// closed words covered by differently framed open modes.
	corpus := []Mode{
		ClosedMode(),
		ClosedMode(hA),
		ClosedMode(hB),
		ClosedMode(hTop),
		ClosedMode(hA, hB),
		ClosedMode(hA, hTop),
		KnownMode(nil, hA, nil),
		KnownMode(nil, hTop, nil),
		KnownMode([]Handle{hA}, hB, nil),
		KnownMode([]Handle{hA}, hTop, nil),
		KnownMode(nil, hA, []Handle{hB}),
		OpaqueMode([]Handle{hA}, nil),
		OpaqueMode(nil, []Handle{hB}),
		OpaqueMode([]Handle{hA}, []Handle{hB}),
	}
	state := uint64(0x7a1d8c05)
	for trial := 0; trial < 1024; trial++ {
		// Vary both cardinality and insertion order.  Duplicates exercise the
		// exact hash-bucket equality path before dominance begins.
		count := 1 + int(nextTestRandom(&state)%17)
		input := make([]Mode, count)
		for index := range input {
			input[index] = corpus[int(nextTestRandom(&state)%uint64(len(corpus)))]
		}
		got := FromModes(labels, input...)
		want := normalizePairwiseReference(labels, input)
		if !Equal(labels, got, want) || Hash(labels, got) != Hash(labels, want) {
			t.Fatalf("trial=%d indexed=%#v pairwise=%#v", trial, got.Modes(), want.Modes())
		}
	}
}

func TestIndexedDominanceSkipsUnrelatedSkeletons(t *testing.T) {
	// This is the former pathological shape: no two alternatives can cover
	// one another, but the old normalizer still considered every pair.  The
	// result remains a finite exact union, now reached through length/frame
	// indexes rather than an all-pairs scan.
	const count = 1024
	input := make([]Mode, count)
	for index := range input {
		word := make([]Handle, index+1)
		for offset := range word {
			word[offset] = hA
		}
		input[index] = ClosedMode(word...)
	}
	got := FromModes(labels, input...)
	if got.ModeCount() != count {
		t.Fatalf("unrelated closed skeletons=%d, want %d", got.ModeCount(), count)
	}
	if want := normalizePairwiseReference(labels, input); !Equal(labels, got, want) || Hash(labels, got) != Hash(labels, want) {
		t.Fatal("indexed skeleton route changed normal form")
	}
}

func TestMuWidenGroupsEveryModeOfOneSkeleton(t *testing.T) {
	// The length-two alternative sorts between some length-one labels under a
	// plain lexical order. Grouping by skeleton must still see all three
	// length-one modes before it generalizes them.
	previous := FromModes(labels,
		ClosedMode(hA),
		ClosedMode(hA, hC),
		ClosedMode(hB),
	)
	next := FromModes(labels,
		ClosedMode(hA),
		ClosedMode(hA, hC),
		ClosedMode(hC),
	)
	widened := Widen(labels, previous, next)
	want := FromModes(labels, ClosedMode(hTop), ClosedMode(hA, hC))
	if !Equal(labels, widened, want) {
		t.Fatalf("widened=%#v, want %#v", widened.Modes(), want.Modes())
	}
	if !rankDescends(WidenRank(labels, previous), WidenRank(labels, widened)) {
		t.Fatalf("rank=%#v -> %#v", WidenRank(labels, previous), WidenRank(labels, widened))
	}
}

func TestExhaustiveFiniteConcreteSoundnessAndLaws(t *testing.T) {
	corpus := []Value{
		Bottom(),
		Top(),
		FromModes(labels, ClosedMode()),
		FromModes(labels, ClosedMode(hA)),
		FromModes(labels, ClosedMode(hB)),
		FromModes(labels, ClosedMode(hA, hB)),
		FromModes(labels, ClosedMode(hB, hA)),
		FromModes(labels, KnownMode(nil, hA, nil)),
		FromModes(labels, KnownMode(nil, hA, []Handle{hB})),
		FromModes(labels, KnownMode([]Handle{hA}, hB, nil)),
		FromModes(labels, OpaqueMode(nil, []Handle{hA})),
		FromModes(labels, OpaqueMode([]Handle{hB}, nil)),
		FromModes(labels, ClosedMode(hA, hB), ClosedMode(hB, hA)),
	}
	for leftIndex, left := range corpus {
		if !LessEqual(labels, left, left) {
			t.Fatalf("not reflexive %d", leftIndex)
		}
		if !Equal(labels, Join(labels, left, left), left) {
			t.Fatalf("not idempotent %d", leftIndex)
		}
		for rightIndex, right := range corpus {
			joined := Join(labels, left, right)
			if !LessEqual(labels, left, joined) || !LessEqual(labels, right, joined) {
				t.Fatalf("join is not upper bound %d,%d", leftIndex, rightIndex)
			}
			if !Equal(labels, joined, Join(labels, right, left)) {
				t.Fatalf("join is not commutative %d,%d", leftIndex, rightIndex)
			}
			if !concreteSubset(concrete(left, 4), concrete(joined, 4)) || !concreteSubset(concrete(right, 4), concrete(joined, 4)) {
				t.Fatalf("join concrete unsound %d,%d", leftIndex, rightIndex)
			}
			widened := Widen(labels, left, right)
			if !LessEqual(labels, left, widened) || !LessEqual(labels, right, widened) {
				t.Fatalf("widen is not upper bound %d,%d", leftIndex, rightIndex)
			}
			if !concreteSubset(concrete(left, 4), concrete(widened, 4)) || !concreteSubset(concrete(right, 4), concrete(widened, 4)) {
				t.Fatalf("widen concrete unsound %d,%d", leftIndex, rightIndex)
			}
			if !LessEqual(labels, right, left) && !Equal(labels, widened, left) && !rankDescends(WidenRank(labels, left), WidenRank(labels, widened)) {
				t.Fatalf("widen rank did not descend %d,%d: %#v -> %#v", leftIndex, rightIndex, WidenRank(labels, left), WidenRank(labels, widened))
			}
			for candidateIndex, candidate := range corpus {
				if LessEqual(labels, left, candidate) && LessEqual(labels, right, candidate) && !LessEqual(labels, joined, candidate) {
					t.Fatalf("join not least upper bound %d,%d <= %d", leftIndex, rightIndex, candidateIndex)
				}
				if LessEqual(labels, left, right) && LessEqual(labels, right, candidate) && !LessEqual(labels, left, candidate) {
					t.Fatalf("order not transitive %d,%d,%d", leftIndex, rightIndex, candidateIndex)
				}
				if LessEqual(labels, left, right) && LessEqual(labels, right, left) && !Equal(labels, left, right) {
					t.Fatalf("order not antisymmetric %d,%d", leftIndex, rightIndex)
				}
			}
			for thirdIndex, third := range corpus {
				if !Equal(labels, Join(labels, joined, third), Join(labels, left, Join(labels, right, third))) {
					t.Fatalf("join not associative %d,%d,%d", leftIndex, rightIndex, thirdIndex)
				}
			}
		}
	}
}

func TestProjectionConcreteSoundness(t *testing.T) {
	corpus := []Value{
		Top(),
		FromModes(labels, ClosedMode()),
		FromModes(labels, ClosedMode(hA, hB)),
		FromModes(labels, KnownMode(nil, hA, []Handle{hB})),
		FromModes(labels, KnownMode([]Handle{hA}, hB, []Handle{hC})),
		FromModes(labels, OpaqueMode(nil, []Handle{hA})),
	}
	for index, value := range corpus {
		for width := 1; width <= 4; width++ {
			for key := range concrete(value, 4) {
				word := parseWord(key)
				for position := 0; position < width; position++ {
					label := hNil
					if position < len(word) {
						label = word[position]
					}
					want := map[string]bool{wordKey([]Handle{label}): true}
					if !concreteSubset(want, concrete(FixedAt(labels, value, position), 1)) {
						t.Fatalf("FixedAt unsound value=%d width=%d position=%d", index, width, position)
					}
				}
			}
		}
		wantScalar := make(map[string]bool)
		for key := range concrete(value, 4) {
			word := parseWord(key)
			if len(word) == 0 {
				wantScalar[wordKey([]Handle{hNil})] = true
			} else {
				wantScalar[wordKey([]Handle{word[0]})] = true
			}
		}
		if !concreteSubset(wantScalar, concrete(Scalar(labels, value), 1)) {
			t.Fatalf("scalar projection unsound value=%d", index)
		}
	}
}

func TestAssembleBoundedConcreteSemanticsAndNormalForm(t *testing.T) {
	// This is an independent bounded concrete oracle for the only legal Values
	// shape: any number of scalar-adjusted inputs and one final forwarded Pack.
	// The corpus covers closed, suffix-bearing, opaque, and correlated inputs;
	// the finite alphabet lets equality (not merely containment) catch both a
	// dropped alignment and an invented cross-product word.
	closedEmpty := FromModes(labels, ClosedMode())
	left := FromModes(labels, ClosedMode(hA), ClosedMode(hB))
	correlated := FromModes(labels, ClosedMode(hA, hB), ClosedMode(hB, hC))
	corpus := []Value{
		Bottom(),
		closedEmpty,
		FromModes(labels, ClosedMode(hA)),
		FromModes(labels, ClosedMode(hA, hB)),
		FromModes(labels, KnownMode(nil, hA, []Handle{hB})),
		FromModes(labels, OpaqueMode([]Handle{hA}, []Handle{hB})),
		correlated,
	}
	patterns := [][]Value{{}, {left}, {left, FromModes(labels, ClosedMode(hB))}}
	for patternIndex, fixed := range patterns {
		for finalIndex, final := range corpus {
			got := Assemble(labels, fixed, final)
			want := concreteAssemble(fixed, final, 4)
			if actual := concrete(got, 4); !sameConcrete(actual, want) {
				t.Fatalf("concrete assembly differs fixed=%d final=%d: got=%v want=%v", patternIndex, finalIndex, actual, want)
			}
			// The direct construction intentionally bypasses normalize's
			// quadratic dominance scan, so re-normalization is the normal-form
			// witness rather than an implementation mechanism.
			if normalized := FromModes(labels, got.modes...); !Equal(labels, got, normalized) || Hash(labels, got) != Hash(labels, normalized) {
				t.Fatalf("assembly lost normal form fixed=%d final=%d", patternIndex, finalIndex)
			}
		}
	}
}

func TestAssembleLuaShapesAlternativesAndMonotonicity(t *testing.T) {
	empty := FromModes(labels, ClosedMode())
	final := FromModes(labels, KnownMode(nil, hB, []Handle{hC}))

	// `return` and `return f()` are the identity cases. An absent Program tail
	// is represented by the closed-empty final Pack, never by a second API.
	if got := Assemble(labels, nil, empty); !Equal(labels, got, empty) {
		t.Fatalf("empty Values=%#v, want empty", got.Modes())
	}
	if got := Assemble(labels, nil, final); !Equal(labels, got, final) {
		t.Fatalf("final-only Values=%#v, want final", got.Modes())
	}

	// `return f(), g()` scalarizes f then forwards g. The suffix-bearing f
	// has two feasible first values; neither may be joined before framing g.
	first := FromModes(labels, KnownMode(nil, hA, []Handle{hB}))
	got := Assemble(labels, []Value{first}, final)
	want := FromModes(labels,
		KnownMode([]Handle{hA}, hB, []Handle{hC}),
		KnownMode([]Handle{hB}, hB, []Handle{hC}),
	)
	if !Equal(labels, got, want) {
		t.Fatalf("f(),g()=%#v, want %#v", got.Modes(), want.Modes())
	}

	// Independent scalar alternatives have an exact finite Cartesian result.
	// This is not a control-flow join: compatible guarded tuples are selected
	// by engine before this operation; Pack retains the remaining alternatives.
	left := FromModes(labels, ClosedMode(hA), ClosedMode(hB))
	right := FromModes(labels, ClosedMode(hB), ClosedMode(hC))
	got = Assemble(labels, []Value{left, right}, empty)
	want = FromModes(labels,
		ClosedMode(hA, hB), ClosedMode(hA, hC),
		ClosedMode(hB, hB), ClosedMode(hB, hC),
	)
	if !Equal(labels, got, want) {
		t.Fatalf("alternative Cartesian product=%#v, want %#v", got.Modes(), want.Modes())
	}

	// Assemble is monotone in both the scalar-adjusted fixed factors and final
	// pack. Flat TypeTop is the sole nontrivial label order in this carrier.
	lowerFixed := FromModes(labels, ClosedMode(hA))
	upperFixed := FromModes(labels, ClosedMode(hTop))
	lowerFinal := FromModes(labels, ClosedMode(hB))
	upperFinal := FromModes(labels, ClosedMode(hTop))
	lower := Assemble(labels, []Value{lowerFixed}, lowerFinal)
	upper := Assemble(labels, []Value{upperFixed}, upperFinal)
	if !LessEqual(labels, lower, upper) {
		t.Fatalf("assembly is not monotone: %#v <= %#v", lower.Modes(), upper.Modes())
	}
}

func TestAssembleLongClosedPrefixSharesPersistentHistory(t *testing.T) {
	const width = 10_000
	fixed := make([]Value, width)
	unit := FromModes(labels, ClosedMode(hA))
	for index := range fixed {
		fixed[index] = unit
	}
	value := Assemble(labels, fixed, FromModes(labels, ClosedMode()))
	if value.ModeCount() != 1 || value.modes[0].kind != ModeClosed {
		t.Fatalf("long assembly shape=%#v", value.Modes())
	}
	word := value.modes[0].closed
	if word.length != width {
		t.Fatalf("long assembly width=%d, want %d", word.length, width)
	}
	if height, limit := int(word.root.height), 2*bits.Len(uint(word.length)); height > limit {
		t.Fatalf("long assembly rope height=%d exceeds AVL bound=%d", height, limit)
	}
	for _, index := range []int{0, width / 2, width - 1} {
		if label, ok := word.At(index); !ok || label != hA {
			t.Fatalf("long assembly word[%d]=%v/%v, want %v", index, label, ok, hA)
		}
	}

	build := func(size int) float64 {
		return testing.AllocsPerRun(1, func() {
			inputs := make([]Value, size)
			for index := range inputs {
				inputs[index] = unit
			}
			if result := Assemble(labels, inputs, FromModes(labels, ClosedMode())); result.ModeCount() != 1 {
				t.Fatal("long assembly lost exact closed result")
			}
		})
	}
	short, wide := build(5_000), build(10_000)
	// AVL joins copy a logarithmic spine. Doubling work may pay that small
	// logarithmic factor, but cannot re-copy the accumulated prefix at every
	// extension.
	if wide > short*2.6+128 {
		t.Fatalf("long Assemble allocations grew too fast: 5k=%g 10k=%g", short, wide)
	}
}

func TestAssembleAlternativePrefixAvoidsHistoricalNormalization(t *testing.T) {
	const width = 10_000
	counting := &countingLabels{}
	inputs := make([]Value, width)
	inputs[0] = FromModes(counting, ClosedMode(hA), ClosedMode(hB))
	unit := FromModes(counting, ClosedMode(hC))
	for index := 1; index < len(inputs); index++ {
		inputs[index] = unit
	}
	counting.equalCalls = 0
	value := Assemble(counting, inputs, FromModes(counting, ClosedMode()))
	if value.ModeCount() != 2 {
		t.Fatalf("long alternative assembly modes=%d, want 2", value.ModeCount())
	}
	// The operation must not route a two-alternative result through
	// normalize/removeDominated on each growing prefix. Scalar construction,
	// rope joins, and direct canonical sorting need no label comparisons.
	if counting.equalCalls != 0 {
		t.Fatalf("long alternative assembly rescanned labels: Equal calls=%d", counting.equalCalls)
	}
}

func TestHotIdentityPathsAllocateNothing(t *testing.T) {
	value := FromModes(labels, ClosedMode(hA, hB), KnownMode(nil, hC, []Handle{hA}))
	if allocations := testing.AllocsPerRun(100, func() {
		if !Equal(labels, value, value) || !LessEqual(labels, value, value) {
			t.Fatal("identity relation")
		}
		if !Equal(labels, Join(labels, value, value), value) {
			t.Fatal("identity join")
		}
		if !Equal(labels, Widen(labels, value, value), value) {
			t.Fatal("identity widen")
		}
		_ = Hash(labels, value)
	}); allocations != 0 {
		t.Fatalf("hot identity path allocations=%g", allocations)
	}
}

func TestClosedWordsAreSegmentationIndependentAndHotIterable(t *testing.T) {
	flat := FromModes(labels, ClosedMode(hA, hB, hC, hC))
	segmented := FromModes(labels, Mode{kind: ModeClosed, closed: concatClosedWords(
		closedWordFromFlat([]Handle{hA}),
		closedWordFromLeaf(closedRepeatLeaf(hB, 1)),
		closedWordFromFlat([]Handle{hC}),
		closedWordFromLeaf(closedRepeatLeaf(hC, 1)),
	)})
	if !Equal(labels, flat, segmented) || Hash(labels, flat) != Hash(labels, segmented) {
		t.Fatalf("segmentation changed semantic value: flat=%#v segmented=%#v", flat, segmented)
	}
	mode := segmented.Modes()[0]
	if got, ok := mode.ClosedAt(2); !ok || got != hC {
		t.Fatalf("ClosedAt(2) = %v/%v, want %v", got, ok, hC)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		iterator := mode.ClosedIterator()
		for range 4 {
			if _, ok := iterator.Next(); !ok {
				t.Fatal("iterator ended early")
			}
		}
		if _, ok := iterator.Next(); ok {
			t.Fatal("iterator exceeded exact length")
		}
	}); allocations != 0 {
		t.Fatalf("segmented closed iteration allocated %g times", allocations)
	}
}

func TestClosedHashCollisionDoesNotImplyEquality(t *testing.T) {
	collision := collisionLabels{}
	left := FromModes(collision, ClosedMode(hA))
	right := FromModes(collision, ClosedMode(hB))
	if Hash(collision, left) != Hash(collision, right) {
		t.Fatal("test setup did not collide")
	}
	if Equal(collision, left, right) {
		t.Fatal("hash collision became semantic equality")
	}
}

func TestFixedAtTopAndBottom(t *testing.T) {
	if got := FixedAt(labels, Top(), 0); !Equal(labels, got, FromModes(labels, ClosedMode(hTop))) {
		t.Fatalf("PackTop scalar=%#v", got.Modes())
	}
	if got := FixedAt(labels, Bottom(), 0); !got.IsBottom() {
		t.Fatalf("Bottom scalar=%#v", got.Modes())
	}
}

func TestFixedAtTypeTopAndCollisionSemantics(t *testing.T) {
	topPrefix := FromModes(labels, KnownMode([]Handle{hTop}, hA, []Handle{hB}))
	topTail := FromModes(labels, KnownMode(nil, hTop, []Handle{hB}))
	topSuffix := FromModes(labels, KnownMode(nil, hA, []Handle{hTop}))
	for name, value := range map[string]Value{"prefix": topPrefix, "tail": topTail, "suffix": topSuffix} {
		if got := FixedAt(labels, value, 0); !Equal(labels, got, FromModes(labels, ClosedMode(hTop))) {
			t.Fatalf("TypeTop %s scalar=%#v", name, got.Modes())
		}
	}
	collision := collisionLabels{}
	input := FromModes(collision, KnownMode(nil, hA, []Handle{hB}))
	got := FixedAt(collision, input, 1)
	want := FromModes(collision, ClosedMode(hA), ClosedMode(hB), ClosedMode(hNil))
	if !Equal(collision, got, want) || Hash(collision, got) != Hash(collision, want) {
		t.Fatalf("collision lost exact scalar alternatives: got=%#v want=%#v", got.Modes(), want.Modes())
	}
}

func TestFixedAtHighSuffixScalesWithScalarOutput(t *testing.T) {
	measure := func(width int) (int, int) {
		labels := &countingLabels{}
		suffix := make([]Handle, width)
		for index := range suffix {
			suffix[index] = NewHandle(1, uint32(100+index))
		}
		value := FromModes(labels, KnownMode(nil, hA, suffix))
		labels.equalCalls = 0
		projected := FixedAt(labels, value, width-1)
		return labels.equalCalls, projected.ModeCount()
	}
	shortCalls, shortModes := measure(1024)
	wideCalls, wideModes := measure(2048)
	if shortModes != 1025 || wideModes != 2049 {
		t.Fatalf("high-suffix scalar alternatives = %d/%d, want 1025/2049", shortModes, wideModes)
	}
	// Exact scalar output itself is linear in the observable suffix choices.
	// The normalizer must therefore use no pairwise dominance work.
	if shortCalls != 0 || wideCalls != 0 {
		t.Fatalf("scalar dedup consulted Labels.Equal: 1024=%d 2048=%d", shortCalls, wideCalls)
	}
}

func TestFixedAtScalarDedupIgnoresLabelHashCollisions(t *testing.T) {
	const width = 2048
	labels := &countingCollisionLabels{}
	suffix := make([]Handle, width)
	for index := range suffix {
		suffix[index] = NewHandle(1, uint32(100+index))
	}
	value := FromModes(labels, KnownMode(nil, hA, suffix))
	labels.equalCalls = 0
	projected := FixedAt(labels, value, width-1)
	if projected.ModeCount() != width+1 {
		t.Fatalf("colliding hash scalar alternatives=%d, want %d", projected.ModeCount(), width+1)
	}
	if labels.equalCalls != 0 {
		t.Fatalf("scalar dedup compared %d colliding labels", labels.equalCalls)
	}

	duplicates := FromModes(labels, KnownMode(nil, hA, []Handle{hB, hB, hB}))
	labels.equalCalls = 0
	got := FixedAt(labels, duplicates, 3)
	if got.ModeCount() != 3 {
		t.Fatalf("duplicate scalar labels not deduplicated: %#v", got.Modes())
	}
	for index, want := range []Handle{hA, hB, hNil} {
		label, ok := got.Modes()[index].ClosedAt(0)
		if !ok || label != want {
			t.Fatalf("duplicate scalar alternative %d=%v/%v, want %v", index, label, ok, want)
		}
	}
	if labels.equalCalls != 0 {
		t.Fatalf("duplicate scalar dedup compared %d labels", labels.equalCalls)
	}
}

func BenchmarkFixedAtLowIndex(b *testing.B) {
	input := FromModes(labels, KnownMode([]Handle{hA}, hB, []Handle{hC}))
	b.ReportAllocs()
	for range b.N {
		_ = FixedAt(labels, input, 0)
	}
}

func BenchmarkFixedAtHighSuffix(b *testing.B) {
	const width = 2048
	suffix := make([]Handle, width)
	for index := range suffix {
		suffix[index] = NewHandle(1, uint32(100+index))
	}
	input := FromModes(labels, KnownMode(nil, hA, suffix))
	b.ReportAllocs()
	for range b.N {
		_ = FixedAt(labels, input, width-1)
	}
}

func BenchmarkAssembleClosed10000(b *testing.B) {
	const width = 10_000
	fixed := make([]Value, width)
	unit := FromModes(labels, ClosedMode(hA))
	for index := range fixed {
		fixed[index] = unit
	}
	final := FromModes(labels, ClosedMode())
	b.ReportAllocs()
	for range b.N {
		_ = Assemble(labels, fixed, final)
	}
}

func BenchmarkNormalizeUnrelatedClosedSkeletons(b *testing.B) {
	const count = 2048
	input := make([]Mode, count)
	for index := range input {
		word := make([]Handle, index+1)
		for offset := range word {
			word[offset] = hA
		}
		input[index] = ClosedMode(word...)
	}
	b.ReportAllocs()
	for range b.N {
		if value := FromModes(labels, input...); value.ModeCount() != count {
			b.Fatal("normalization lost alternatives")
		}
	}
}

type collisionLabels struct{ flatLabels }

func (collisionLabels) Hash(Handle) uint64 { return 1 }

type countingLabels struct {
	flatLabels
	equalCalls int
}

func (labels *countingLabels) Equal(left, right Handle) bool {
	labels.equalCalls++
	return left == right
}

type countingCollisionLabels struct {
	flatLabels
	equalCalls int
}

func (labels *countingCollisionLabels) Equal(left, right Handle) bool {
	labels.equalCalls++
	return left == right
}

func (countingCollisionLabels) Hash(Handle) uint64 { return 1 }

func rankDescends(before, after Rank) bool {
	if after.ShapeClass != before.ShapeClass {
		return after.ShapeClass < before.ShapeClass
	}
	return after.ExactLabels < before.ExactLabels
}

func nextTestRandom(state *uint64) uint64 {
	*state = *state*6364136223846793005 + 1442695040888963407
	return *state
}

// normalizePairwiseReference is the direct mathematical definition used by
// the previous implementation.  It deliberately does no indexing, so the
// equality above catches an omitted candidate class in removeDominated.
func normalizePairwiseReference(labels Labels, input []Mode) Value {
	modes := make([]Mode, 0, len(input))
	for _, source := range input {
		mode, keep, top := normalizeMode(labels, source)
		if top {
			return Top()
		}
		if keep {
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 {
		return Bottom()
	}
	modes = deduplicate(labels, modes)
	out := make([]Mode, 0, len(modes))
	for index, mode := range modes {
		dominated := false
		for otherIndex, other := range modes {
			if index == otherIndex || modeEqual(labels, mode, other) {
				continue
			}
			if modeLessEqual(labels, mode, other) {
				dominated = true
				break
			}
		}
		if !dominated {
			out = append(out, mode)
		}
	}
	if len(out) == 0 {
		return Bottom()
	}
	sortModes(out)
	return Value{state: stateModes, modes: out}
}

func sameHandles(left, right []Handle) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// The finite concrete interpreter below is deliberately tiny and independent
// of the abstract operations. It expands TypeTop to the three ordinary labels
// and bounded star lengths, sufficient to falsify an unsound transform.
var concreteAlphabet = []Handle{hNil, hA, hB, hC}

func concrete(value Value, limit int) map[string]bool {
	out := make(map[string]bool)
	switch value.state {
	case stateBottom:
		return out
	case stateTop:
		for length := 0; length <= limit; length++ {
			for _, word := range allWords(length) {
				out[wordKey(word)] = true
			}
		}
		return out
	case stateModes:
		for _, mode := range value.modes {
			for _, word := range concreteMode(mode, limit) {
				out[wordKey(word)] = true
			}
		}
		return out
	default:
		return out
	}
}

// concreteAssemble is deliberately independent of Assemble. It enumerates a
// bounded concrete language for the source-law definition: fixed inputs
// contribute exactly one first value and final contributes its whole word.
func concreteAssemble(fixed []Value, final Value, limit int) map[string]bool {
	prefixes := [][]Handle{{}}
	for _, input := range fixed {
		next := make([][]Handle, 0)
		for key := range concrete(input, limit) {
			word := parseWord(key)
			label := hNil
			if len(word) != 0 {
				label = word[0]
			}
			for _, prefix := range prefixes {
				if len(prefix) < limit {
					next = append(next, append(append([]Handle(nil), prefix...), label))
				}
			}
		}
		prefixes = next
	}
	out := make(map[string]bool)
	for _, prefix := range prefixes {
		for key := range concrete(final, limit) {
			word := parseWord(key)
			if len(prefix)+len(word) > limit {
				continue
			}
			joined := append(append([]Handle(nil), prefix...), word...)
			out[wordKey(joined)] = true
		}
	}
	return out
}

func sameConcrete(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func concreteMode(mode Mode, limit int) [][]Handle {
	if mode.kind == ModeClosed {
		return expandLabels(mode.Prefix(), limit)
	}
	maxRepeat := limit - len(mode.prefix) - len(mode.suffix)
	if maxRepeat < 0 {
		return nil
	}
	var out [][]Handle
	for repetitions := 0; repetitions <= maxRepeat; repetitions++ {
		word := append([]Handle(nil), mode.prefix...)
		if mode.kind == ModeKnown {
			for range repetitions {
				word = append(word, mode.tail)
			}
			out = append(out, expandLabels(append(word, mode.suffix...), limit)...)
			continue
		}
		for _, middle := range allWords(repetitions) {
			candidate := append(append([]Handle(nil), word...), middle...)
			candidate = append(candidate, mode.suffix...)
			out = append(out, expandLabels(candidate, limit)...)
		}
	}
	return out
}

func expandLabels(labels []Handle, limit int) [][]Handle {
	if len(labels) > limit {
		return nil
	}
	out := [][]Handle{{}}
	for _, label := range labels {
		choices := []Handle{label}
		if label == hTop {
			choices = concreteAlphabet
		}
		next := make([][]Handle, 0, len(out)*len(choices))
		for _, prefix := range out {
			for _, choice := range choices {
				next = append(next, append(append([]Handle(nil), prefix...), choice))
			}
		}
		out = next
	}
	return out
}

func allWords(length int) [][]Handle {
	if length == 0 {
		return [][]Handle{{}}
	}
	previous := allWords(length - 1)
	out := make([][]Handle, 0, len(previous)*len(concreteAlphabet))
	for _, prefix := range previous {
		for _, label := range concreteAlphabet {
			out = append(out, append(append([]Handle(nil), prefix...), label))
		}
	}
	return out
}

func concreteSubset(left, right map[string]bool) bool {
	for word := range left {
		if !right[word] {
			return false
		}
	}
	return true
}

func wordKey(word []Handle) string { return fmt.Sprint(word) }

func parseWord(key string) []Handle {
	// wordKey is used only by this test oracle and contains dense ordinals in
	// Go's default struct formatting. Avoid coupling the concrete oracle to
	// the abstract carrier by using its own fixed dictionary.
	for length := 0; length <= 4; length++ {
		for _, word := range allWords(length) {
			if wordKey(word) == key {
				return word
			}
		}
	}
	panic("unknown concrete word " + key)
}
