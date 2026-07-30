package presence

import "testing"

func sample() []Value {
	return []Value{Bottom(), Present(), Absent(), Maybe()}
}

func TestEqualReflexive(t *testing.T) {
	for _, v := range sample() {
		if !Equal(v, v) {
			t.Fatalf("Equal not reflexive for %s", v)
		}
	}
}

func TestEqualSymmetric(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if Equal(a, b) != Equal(b, a) {
				t.Fatalf("Equal not symmetric: %s vs %s", a, b)
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
			if a.Covers(b) != (Join(a, b) == a) {
				t.Fatalf("Covers/Join inconsistent: %s, %s", a, b)
			}
		}
	}
}

// TestChainOrder pins the intended four-point order:
// Bottom < Present, Bottom < Absent, Present and Absent incomparable, both < Maybe.
func TestChainOrder(t *testing.T) {
	if !Present().Covers(Bottom()) || !Absent().Covers(Bottom()) {
		t.Fatal("Present/Absent must cover Bottom")
	}
	if !Maybe().Covers(Present()) || !Maybe().Covers(Absent()) {
		t.Fatal("Maybe must cover Present and Absent")
	}
	if Present().Covers(Absent()) || Absent().Covers(Present()) {
		t.Fatal("Present and Absent must be incomparable")
	}
	if Join(Present(), Absent()) != Maybe() {
		t.Fatalf("Present join Absent must be Maybe, got %s", Join(Present(), Absent()))
	}
}

func TestWidenEqualsJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if Widen(a, b) != Join(a, b) {
				t.Fatalf("Widen != Join for %s, %s", a, b)
			}
		}
	}
}
