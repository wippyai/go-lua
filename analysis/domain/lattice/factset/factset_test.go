package factset

import "testing"

// tf is a test fact: Key buckets facts, Rank acts like a per-key dominance
// level, Tag exercises the Clone hook's deep-copy isolation.
type tf struct {
	Key  int
	Rank int
	Tag  []int
}

func testSet() Set[int, tf] {
	return Set[int, tf]{
		Key:       func(f tf) int { return f.Key },
		EqualFact: func(a, b tf) bool { return a.Key == b.Key && a.Rank == b.Rank },
		Less: func(a, b tf) bool {
			if a.Key != b.Key {
				return a.Key < b.Key
			}
			return a.Rank < b.Rank
		},
		Valid:     func(f tf) bool { return f.Rank > 0 },
		CloneFact: func(f tf) tf { f.Tag = append([]int(nil), f.Tag...); return f },
		Prefer:    func(kept, incoming tf) bool { return incoming.Rank > kept.Rank },
		Dominates: func(super, sub tf) bool {
			return super.Key == sub.Key && super.Rank >= sub.Rank
		},
	}
}

func facts(pairs ...int) []tf {
	out := make([]tf, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, tf{Key: pairs[i], Rank: pairs[i+1]})
	}
	return out
}

func TestNormalizeFiltersDedupesAndOrders(t *testing.T) {
	s := testSet()
	// Rank 0 dropped by Valid; key 1 keeps the higher rank via Prefer; ordered by Less.
	got := s.Normalize(facts(2, 1, 1, 1, 1, 3, 3, 0))
	want := facts(1, 3, 2, 1)
	if !s.Equal(got, want) {
		t.Fatalf("normalize = %v, want %v", got, want)
	}
	if !s.Equal(got, s.Normalize(got)) {
		t.Fatalf("normalize not idempotent")
	}
}

func TestJoinSemilatticeLaws(t *testing.T) {
	s := testSet()
	a := facts(1, 2, 2, 1)
	b := facts(2, 3, 3, 1)
	c := facts(1, 1, 4, 2)

	if !s.Equal(s.Join(a, b), s.Join(b, a)) {
		t.Fatalf("join not commutative")
	}
	if !s.Equal(s.Join(s.Join(a, b), c), s.Join(a, s.Join(b, c))) {
		t.Fatalf("join not associative")
	}
	if !s.Equal(s.Join(a, a), s.Normalize(a)) {
		t.Fatalf("join not idempotent")
	}
	if !s.LessOrEq(a, s.Join(a, b)) || !s.LessOrEq(b, s.Join(a, b)) {
		t.Fatalf("join is not an upper bound")
	}
	if !s.Equal(s.Widen(a, b), s.Join(a, b)) {
		t.Fatalf("widen must equal join for a finite-height set")
	}
}

func TestBottomAndOrder(t *testing.T) {
	s := testSet()
	a := facts(1, 2, 5, 1)
	if !s.Equal(s.Join(nil, a), s.Normalize(a)) {
		t.Fatalf("bottom is not the join identity")
	}
	if !s.LessOrEq(nil, a) {
		t.Fatalf("bottom must be below every element")
	}
	if !s.LessOrEq(a, a) {
		t.Fatalf("lessOrEq not reflexive")
	}
	// Antisymmetry with Equal: a ⊑ b and b ⊑ a implies Equal.
	b := facts(1, 2, 5, 1)
	if s.LessOrEq(a, b) && s.LessOrEq(b, a) && !s.Equal(a, b) {
		t.Fatalf("antisymmetry violated")
	}
}

func TestDominationSubsumes(t *testing.T) {
	s := testSet()
	// Higher rank at key 1 dominates lower rank at key 1.
	if !s.LessOrEq(facts(1, 1), facts(1, 3)) {
		t.Fatalf("lower rank should be subsumed by higher rank under same key")
	}
	if s.LessOrEq(facts(1, 3), facts(1, 1)) {
		t.Fatalf("higher rank must not be subsumed by lower rank")
	}
}

func TestCloneIsolatesStoredFacts(t *testing.T) {
	s := testSet()
	in := []tf{{Key: 1, Rank: 1, Tag: []int{9}}}
	out := s.Normalize(in)
	in[0].Tag[0] = 100
	if out[0].Tag[0] != 9 {
		t.Fatalf("clone did not isolate stored fact from caller mutation")
	}
}
