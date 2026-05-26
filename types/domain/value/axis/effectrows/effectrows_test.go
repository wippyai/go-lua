package effectrows

import (
	"testing"

	"github.com/wippyai/go-lua/types/effect"
)

func sample() []Value {
	throw := effect.Empty.With(effect.Throw{})
	io := effect.Empty.With(effect.IO{})
	throwIO := effect.Empty.With(effect.Throw{}, effect.IO{})
	return []Value{Bottom(), Of(throw), Of(io), Of(throwIO), Top()}
}

func TestEqualReflexive(t *testing.T) {
	for _, v := range sample() {
		if !Equal(v, v) {
			t.Fatalf("Equal not reflexive for %s", v.Row())
		}
	}
}

func TestEqualSymmetric(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if Equal(a, b) != Equal(b, a) {
				t.Fatalf("Equal not symmetric: %s vs %s", a.Row(), b.Row())
			}
		}
	}
}

func TestEqualTransitive(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			for _, c := range vs {
				if Equal(a, b) && Equal(b, c) && !Equal(a, c) {
					t.Fatalf("Equal not transitive: %s, %s, %s", a.Row(), b.Row(), c.Row())
				}
			}
		}
	}
}

func TestEqualImpliesEqualHash(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if Equal(a, b) && a.Hash() != b.Hash() {
				t.Fatalf("Equal values hash differently: %s vs %s", a.Row(), b.Row())
			}
		}
	}
}

// TestHashOrderIndependent pins that two rows equal up to label order hash the
// same, matching the label-set semantics of Row equality.
func TestHashOrderIndependent(t *testing.T) {
	ab := Of(effect.Empty.With(effect.Throw{}, effect.IO{}))
	ba := Of(effect.Empty.With(effect.IO{}, effect.Throw{}))
	if !Equal(ab, ba) {
		t.Fatal("rows differing only in label order must be Equal")
	}
	if ab.Hash() != ba.Hash() {
		t.Fatal("Equal rows must hash identically regardless of label order")
	}
}

func TestJoinIdempotent(t *testing.T) {
	for _, v := range sample() {
		if !Equal(Join(v, v), v) {
			t.Fatalf("Join not idempotent for %s", v.Row())
		}
	}
}

func TestJoinCommutative(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if !Equal(Join(a, b), Join(b, a)) {
				t.Fatalf("Join not commutative: %s join %s", a.Row(), b.Row())
			}
		}
	}
}

func TestJoinUpperBound(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			j := Join(a, b)
			if !j.Covers(a) || !j.Covers(b) {
				t.Fatalf("Join %s,%s does not cover an operand", a.Row(), b.Row())
			}
		}
	}
}

func TestCoversConsistentWithJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if a.Covers(b) != Equal(Join(a, b), a) {
				t.Fatalf("Covers/Join inconsistent: %s, %s", a.Row(), b.Row())
			}
		}
	}
}

func TestBottomIsLeast(t *testing.T) {
	for _, v := range sample() {
		if !v.Covers(Bottom()) {
			t.Fatalf("every value must cover Bottom, %s does not", v.Row())
		}
	}
}

func TestTopIsGreatest(t *testing.T) {
	for _, v := range sample() {
		if !Top().Covers(v) {
			t.Fatalf("Top must cover every value, fails for %s", v.Row())
		}
	}
}

func TestWidenEqualsJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if !Equal(Widen(a, b), Join(a, b)) {
				t.Fatalf("Widen != Join for %s, %s", a.Row(), b.Row())
			}
		}
	}
}
