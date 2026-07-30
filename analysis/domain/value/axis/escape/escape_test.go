package escape

import "testing"

func sample() []Value {
	return []Value{Bottom(), Fresh(), Escaped()}
}

func TestEqualReflexiveSymmetricTransitive(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		if !Equal(a, a) {
			t.Fatalf("Equal not reflexive for %s", a)
		}
		for _, b := range vs {
			if Equal(a, b) != Equal(b, a) {
				t.Fatalf("Equal not symmetric: %s vs %s", a, b)
			}
			for _, c := range vs {
				if Equal(a, b) && Equal(b, c) && !Equal(a, c) {
					t.Fatalf("Equal not transitive: %s, %s, %s", a, b, c)
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
				t.Fatalf("Equal values hash differently: %s vs %s", a, b)
			}
		}
	}
}

func TestJoinIdempotent(t *testing.T) {
	for _, v := range sample() {
		if !Equal(Join(v, v), v) {
			t.Fatalf("Join not idempotent for %s: got %s", v, Join(v, v))
		}
	}
}

func TestJoinCommutative(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if !Equal(Join(a, b), Join(b, a)) {
				t.Fatalf("Join not commutative: %s join %s", a, b)
			}
		}
	}
}

func TestJoinAssociative(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			for _, c := range vs {
				if !Equal(Join(Join(a, b), c), Join(a, Join(b, c))) {
					t.Fatalf("Join not associative: %s, %s, %s", a, b, c)
				}
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
				t.Fatalf("Join %s,%s = %s does not cover an operand", a, b, j)
			}
		}
	}
}

func TestJoinMonotone(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if !a.Covers(b) {
				continue
			}
			for _, c := range vs {
				if !Join(a, c).Covers(Join(b, c)) {
					t.Fatalf("Join not monotone via %s covers %s with %s", a, b, c)
				}
			}
		}
	}
}

func TestCoversConsistentWithJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if a.Covers(b) != Equal(Join(a, b), a) {
				t.Fatalf("Covers/Join inconsistent: %s, %s", a, b)
			}
		}
	}
}

func TestWidenEqualsJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if !Equal(Widen(a, b), Join(a, b)) {
				t.Fatalf("Widen != Join for %s, %s", a, b)
			}
		}
	}
}

// TestChainOrder pins the intended order: Bottom < Fresh < Escaped, with Escaped as
// Top (the conservative assumption) and Fresh strictly more precise.
func TestChainOrder(t *testing.T) {
	if !Fresh().Covers(Bottom()) || !Escaped().Covers(Fresh()) {
		t.Fatal("Bottom < Fresh < Escaped must hold")
	}
	if Fresh().Covers(Escaped()) {
		t.Fatal("Fresh must not cover Escaped")
	}
	if !Escaped().IsTop() || !Bottom().IsBottom() {
		t.Fatal("Escaped is Top and Bottom is Bottom")
	}
	if !Equal(Join(Fresh(), Escaped()), Escaped()) {
		t.Fatalf("Fresh join Escaped must be Escaped, got %s", Join(Fresh(), Escaped()))
	}
	if !Equal(Top(), Escaped()) {
		t.Fatal("Top must be Escaped")
	}
}
