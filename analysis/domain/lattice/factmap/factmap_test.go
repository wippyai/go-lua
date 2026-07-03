package factmap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// tf is a test fact: Key indexes the map, Val is drawn from a small int-max
// value lattice (Bottom = 0, Join = max).
type tf struct {
	Key int
	Val int
}

func testMap() Map[int, tf, int] {
	maxInt := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}
	return Map[int, tf, int]{
		Key:       func(f tf) int { return f.Key },
		Value:     func(f tf) int { return f.Val },
		WithValue: func(f tf, v int) tf { f.Val = v; return f },
		Less:      func(a, b tf) bool { return a.Key < b.Key },
		Domain: lattice.Lattice[int]{
			Bottom:   func() int { return 0 },
			Equal:    func(a, b int) bool { return a == b },
			LessOrEq: func(a, b int) bool { return a <= b },
			Join:     maxInt,
			Widen:    maxInt,
		},
	}
}

func mapFacts(pairs ...int) []tf {
	out := make([]tf, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, tf{Key: pairs[i], Val: pairs[i+1]})
	}
	return out
}

func TestNormalizeDropsBottomAndMergesByKey(t *testing.T) {
	m := testMap()
	// key 1 collides (max → 3); key 2 value 0 dropped as bottom; ordered by key.
	got := m.Normalize(mapFacts(1, 2, 1, 3, 2, 0, 3, 5))
	if !m.Equal(got, mapFacts(1, 3, 3, 5)) {
		t.Fatalf("normalize = %v, want {1:3, 3:5}", got)
	}
	if !m.Equal(got, m.Normalize(got)) {
		t.Fatalf("normalize not idempotent")
	}
}

func TestNormalizeOwnedMayReuseFactPayloads(t *testing.T) {
	type taggedFact struct {
		Key int
		Val int
		Tag []int
	}
	m := Map[int, taggedFact, int]{
		Key:       func(f taggedFact) int { return f.Key },
		Value:     func(f taggedFact) int { return f.Val },
		WithValue: func(f taggedFact, v int) taggedFact { f.Val = v; return f },
		Less:      func(a, b taggedFact) bool { return a.Key < b.Key },
		CloneFact: func(f taggedFact) taggedFact {
			f.Tag = append([]int(nil), f.Tag...)
			return f
		},
		Domain: testMap().Domain,
	}

	defensiveInput := []taggedFact{{Key: 1, Val: 2, Tag: []int{7}}}
	defensive := m.Normalize(defensiveInput)
	defensiveInput[0].Tag[0] = 8
	if defensive[0].Tag[0] != 7 {
		t.Fatalf("Normalize reused caller payload: got tag %d, want 7", defensive[0].Tag[0])
	}

	ownedInput := []taggedFact{{Key: 1, Val: 2, Tag: []int{7}}}
	owned := m.NormalizeOwned(ownedInput)
	ownedInput[0].Tag[0] = 8
	if owned[0].Tag[0] != 8 {
		t.Fatalf("NormalizeOwned cloned caller-owned payload: got tag %d, want 8", owned[0].Tag[0])
	}
}

func TestPointwiseJoinLaws(t *testing.T) {
	m := testMap()
	a := mapFacts(1, 2, 2, 4)
	b := mapFacts(2, 1, 3, 5)
	c := mapFacts(1, 7, 3, 1)

	if !m.Equal(m.Join(a, b), mapFacts(1, 2, 2, 4, 3, 5)) {
		t.Fatalf("join = %v, want {1:2,2:4,3:5}", m.Join(a, b))
	}
	if !m.Equal(m.Join(a, b), m.Join(b, a)) {
		t.Fatalf("join not commutative")
	}
	if !m.Equal(m.Join(m.Join(a, b), c), m.Join(a, m.Join(b, c))) {
		t.Fatalf("join not associative")
	}
	if !m.Equal(m.Join(a, a), m.Normalize(a)) {
		t.Fatalf("join not idempotent")
	}
	if !m.LessOrEq(a, m.Join(a, b)) || !m.LessOrEq(b, m.Join(a, b)) {
		t.Fatalf("join is not an upper bound")
	}
	if !m.Equal(m.Widen(a, b), m.Join(a, b)) {
		t.Fatalf("widen must equal join for this finite value domain")
	}
}

func TestMustMapIntersectsKeysAndKeepsBottom(t *testing.T) {
	m := testMap()
	m.Intersect = true
	m.KeepBottom = true

	a := mapFacts(1, 2, 2, 4, 3, 0)
	b := mapFacts(2, 1, 3, 5)
	// Join keeps only shared keys (2,3); values merged by max; bottom retained.
	if !m.Equal(m.Join(a, b), mapFacts(2, 4, 3, 5)) {
		t.Fatalf("must join = %v, want {2:4,3:5}", m.Join(a, b))
	}
	// a ⊑ b iff a carries every key of b with a's value below b's.
	if !m.LessOrEq(mapFacts(1, 1, 2, 1), mapFacts(2, 5)) {
		t.Fatalf("superset-of-keys with smaller values must be below")
	}
	if m.LessOrEq(mapFacts(2, 5), mapFacts(1, 1, 2, 5)) {
		t.Fatalf("missing key 1 must fail must-lessOrEq")
	}
}

func TestPointwiseLessOrEqUsesValueBottomDefault(t *testing.T) {
	m := testMap()
	// {1:2} ⊑ {1:2, 2:5} because the missing key 2 defaults to bottom (0 ⊑ 5).
	if !m.LessOrEq(mapFacts(1, 2), mapFacts(1, 2, 2, 5)) {
		t.Fatalf("missing key should default to value bottom under lessOrEq")
	}
	// {1:5} ⋢ {1:2} since 5 ⋢ 2 at key 1.
	if m.LessOrEq(mapFacts(1, 5), mapFacts(1, 2)) {
		t.Fatalf("a greater value must not be below a smaller one")
	}
}

func TestEqualElementwiseSameSkipsNormalize(t *testing.T) {
	m := testMap()
	validCalls := 0
	m.Valid = func(tf) bool {
		validCalls++
		return false
	}
	facts := mapFacts(1, 2)
	if !m.Equal(facts, facts) {
		t.Fatalf("Equal should accept identical carriers")
	}
	if validCalls != 0 {
		t.Fatalf("Equal normalized identical carriers: validCalls=%d", validCalls)
	}
}
