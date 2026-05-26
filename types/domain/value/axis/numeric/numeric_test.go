package numeric

import "testing"

// sample returns a representative set of Numeric/Interval axis values spanning
// Bottom, Top, bounded ranges, exact points, half-bounded ranges, and modular
// refinements.
func sample() []Value {
	return []Value{
		Bottom(),
		Top(),
		Exact(0),
		Exact(5),
		Range(0, 10),
		Range(-5, 5),
		Range(0, 100),
		Range(3, 3),
		Range(0, 1<<40),
		Exact(6).WithModulus(2, 0),
		Range(0, 10).WithModulus(2, 0),
		Range(0, 10).WithModulus(3, 1),
		Range(0, 9).WithModulus(2, 1),
	}
}

func TestEqualReflexive(t *testing.T) {
	for _, v := range sample() {
		if !Equal(v, v) {
			t.Fatalf("Equal not reflexive for %+v", v)
		}
	}
}

func TestEqualSymmetric(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if Equal(a, b) != Equal(b, a) {
				t.Fatalf("Equal not symmetric: %+v vs %+v", a, b)
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
					t.Fatalf("Equal not transitive: %+v, %+v, %+v", a, b, c)
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
				t.Fatalf("Equal values hash differently: %+v vs %+v", a, b)
			}
		}
	}
}

func TestJoinIdempotent(t *testing.T) {
	for _, v := range sample() {
		if !Equal(Join(v, v), v) {
			t.Fatalf("Join not idempotent for %+v: got %+v", v, Join(v, v))
		}
	}
}

func TestJoinCommutative(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			if !Equal(Join(a, b), Join(b, a)) {
				t.Fatalf("Join not commutative: %+v join %+v", a, b)
			}
		}
	}
}

func TestJoinAssociative(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			for _, c := range vs {
				left := Join(Join(a, b), c)
				right := Join(a, Join(b, c))
				if !Equal(left, right) {
					t.Fatalf("Join not associative: (%+v,%+v,%+v) -> %+v vs %+v", a, b, c, left, right)
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
			if !j.Covers(a) {
				t.Fatalf("Join %+v,%+v = %+v does not cover %+v", a, b, j, a)
			}
			if !j.Covers(b) {
				t.Fatalf("Join %+v,%+v = %+v does not cover %+v", a, b, j, b)
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
				ja := Join(a, c)
				jb := Join(b, c)
				if !ja.Covers(jb) {
					t.Fatalf("Join not monotone: %+v covers %+v but %+v does not cover %+v",
						a, b, ja, jb)
				}
			}
		}
	}
}

func TestCoversConsistentWithJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			covers := a.Covers(b)
			joinIsA := Equal(Join(a, b), a)
			if covers != joinIsA {
				t.Fatalf("Covers/Join inconsistent: %+v covers %+v = %v, Join==a = %v",
					a, b, covers, joinIsA)
			}
		}
	}
}

func TestBottomIsLeastTopIsGreatest(t *testing.T) {
	for _, v := range sample() {
		if !v.Covers(Bottom()) {
			t.Fatalf("%+v does not cover Bottom", v)
		}
		if !Top().Covers(v) {
			t.Fatalf("Top does not cover %+v", v)
		}
		if !Equal(Join(Bottom(), v), v) {
			t.Fatalf("Bottom join %+v changed it", v)
		}
		if !Equal(Join(Top(), v), Top()) {
			t.Fatalf("Top join %+v != Top", v)
		}
	}
}

func TestWidenIsUpperBoundOfJoin(t *testing.T) {
	vs := sample()
	for _, prev := range vs {
		for _, next := range vs {
			w := Widen(prev, next)
			j := Join(prev, next)
			if !w.Covers(j) {
				t.Fatalf("Widen(%+v,%+v)=%+v does not cover Join=%+v", prev, next, w, j)
			}
		}
	}
}

// TestWidenTerminates checks the defining property of widening: an ascending
// chain that keeps growing reaches a stable fixed point in a bounded number of
// steps (the upper bound is released to infinity).
func TestWidenTerminates(t *testing.T) {
	acc := Exact(0)
	prev := acc
	for i := int64(1); i <= 1000; i++ {
		acc = Widen(prev, Join(prev, Exact(i)))
		if Equal(acc, prev) {
			_, upper := acc.Interval()
			if upper != maxInt {
				t.Fatalf("stabilized at non-infinite upper bound %d", upper)
			}
			return
		}
		prev = acc
	}
	t.Fatal("widening did not stabilize within 1000 steps")
}

// TestEmptyRangeNormalizesToBottom checks that an inverted interval and an
// unsatisfiable residue both collapse to the canonical Bottom.
func TestEmptyRangeNormalizesToBottom(t *testing.T) {
	if !Range(5, 1).IsBottom() {
		t.Fatal("inverted interval did not normalize to Bottom")
	}
	// [0,1] cannot contain x % 4 == 3.
	if got := Range(0, 1).WithModulus(4, 3); !got.IsBottom() {
		t.Fatalf("unsatisfiable residue did not normalize to Bottom: %+v", got)
	}
}

// TestModularJoinDropsDisagreeingResidue checks Join keeps a residue only when
// both operands carry the identical one.
func TestModularJoinDropsDisagreeingResidue(t *testing.T) {
	even := Range(0, 10).WithModulus(2, 0)
	odd := Range(0, 9).WithModulus(2, 1)
	j := Join(even, odd)
	if _, _, ok := j.Modulus(); ok {
		t.Fatalf("Join of even and odd kept a residue: %+v", j)
	}
	sameEven := Range(2, 8).WithModulus(2, 0)
	je := Join(even, sameEven)
	if m, r, ok := je.Modulus(); !ok || m != 2 || r != 0 {
		t.Fatalf("Join of two even values dropped the shared residue: %+v", je)
	}
}

const maxInt = int64(^uint64(0) >> 1)
