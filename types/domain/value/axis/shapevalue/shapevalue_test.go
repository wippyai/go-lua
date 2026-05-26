package shapevalue

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// sample returns a representative set of Shape/Value axis values spanning Bottom,
// Top, primitives, containers, and a union, exercising the structural lattice
// without depending on any other axis. Nilability is the Presence axis and is
// deliberately excluded: the convergence join projects nil onto presence, so
// mixing nil here would conflate two orthogonal axes.
func sample() []Value {
	return []Value{
		Bottom(),
		Top(),
		Of(typ.Integer),
		Of(typ.Number),
		Of(typ.String),
		Of(typ.Boolean),
		Of(typ.NewArray(typ.Integer)),
		Of(typ.NewArray(typ.Number)),
		Of(typ.NewArray(typ.String)),
		Of(typ.NewUnion(typ.Integer, typ.String)),
		Of(typ.NewMap(typ.String, typ.Integer)),
	}
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
				t.Fatalf("Equal values hash differently: %s (%d) vs %s (%d)", a, a.Hash(), b, b.Hash())
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
				t.Fatalf("Join not commutative: %s join %s = %s, reversed = %s",
					a, b, Join(a, b), Join(b, a))
			}
		}
	}
}

// TestJoinUpperBound asserts the result of Join covers both operands; this is
// the defining property of a least upper bound that the monotonicity tests build
// on.
func TestJoinUpperBound(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			j := Join(a, b)
			if !j.Covers(a) {
				t.Fatalf("Join %s,%s = %s does not cover %s", a, b, j, a)
			}
			if !j.Covers(b) {
				t.Fatalf("Join %s,%s = %s does not cover %s", a, b, j, b)
			}
		}
	}
}

// TestJoinMonotone verifies monotonicity: if a covers b then joining each with a
// common c preserves the order.
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
					t.Fatalf("Join not monotone: %s covers %s but %s does not cover %s",
						a, b, ja, jb)
				}
			}
		}
	}
}

// TestCoversConsistentWithJoin checks the Covers/Join identity in both directions.
func TestCoversConsistentWithJoin(t *testing.T) {
	vs := sample()
	for _, a := range vs {
		for _, b := range vs {
			covers := a.Covers(b)
			joinIsA := Equal(Join(a, b), a)
			if covers != joinIsA {
				t.Fatalf("Covers/Join inconsistent: %s covers %s = %v, Join==a = %v",
					a, b, covers, joinIsA)
			}
		}
	}
}

func TestBottomIsLeastTopIsGreatest(t *testing.T) {
	for _, v := range sample() {
		if !v.Covers(Bottom()) {
			t.Fatalf("%s does not cover Bottom", v)
		}
		if !Top().Covers(v) {
			t.Fatalf("Top does not cover %s", v)
		}
		if !Equal(Join(Bottom(), v), v) {
			t.Fatalf("Bottom join %s != %s", v, Join(Bottom(), v))
		}
		if !Equal(Join(Top(), v), Top()) {
			t.Fatalf("Top join %s != Top", v)
		}
	}
}

// TestWidenIsUpperBoundOfJoin checks that Widen never sits below Join, so it is a
// sound accelerant of the ascending chain.
func TestWidenIsUpperBoundOfJoin(t *testing.T) {
	vs := sample()
	for _, prev := range vs {
		for _, next := range vs {
			w := Widen(prev, next)
			j := Join(prev, next)
			if !w.Covers(j) {
				t.Fatalf("Widen(%s,%s)=%s does not cover Join=%s", prev, next, w, j)
			}
		}
	}
}

func TestProjectRoundTrips(t *testing.T) {
	v := Of(typ.NewArray(typ.Integer))
	if v.Project() == nil {
		t.Fatal("Project returned nil for a concrete value")
	}
	if !Equal(Of(v.Project()), v) {
		t.Fatalf("Project/Of round-trip changed value: %s", v)
	}
}
