package axiscompose

import (
	"math/rand"
	"reflect"
	"testing"
)

type toySetup struct {
	catalog     Catalog
	may         Handle[uint8]
	must        Handle[uint8]
	unsupported Handle[uint8]
}

func newToySetup() *toySetup {
	s := &toySetup{}
	s.may = RegisterToyMay(&s.catalog, "may.tags")
	s.must = RegisterToyMust(&s.catalog, "must.init")
	s.unsupported = RegisterToyUnsupported(&s.catalog, "may.alias")
	return s
}

func mustSchema(t *testing.T, c *Catalog, ids ...AxisID) *Schema {
	t.Helper()
	s, err := c.Seal(ids...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSchemaAddRemoveAndCanonicalOrder(t *testing.T) {
	s := newToySetup()
	all := mustSchema(t, &s.catalog, s.unsupported.ID(), s.may.ID(), s.must.ID())
	if got, want := all.IDs(), []AxisID{s.may.ID(), s.must.ID(), s.unsupported.ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical IDs = %v, want %v", got, want)
	}
	selected := mustSchema(t, &s.catalog, s.may.ID(), s.must.ID())
	mayOnly := mustSchema(t, &s.catalog, s.may.ID())
	arena := &Arena{}
	value := Put(arena, Bottom(arena, selected), s.may, uint8(3))
	value = Put(arena, value, s.must, uint8(7))
	removed := Reconfigure(arena, value, mayOnly)
	if got, ok := Get(removed, s.may); !ok || got != 3 {
		t.Fatalf("preserved may = %d, %v", got, ok)
	}
	if _, ok := Get(removed, s.must); ok {
		t.Fatal("removed must axis remained readable")
	}
	readded := Reconfigure(arena, removed, selected)
	if got, _ := Get(readded, s.must); got != toyUniverse {
		t.Fatalf("re-added must = %#x, want bottom %#x", got, toyUniverse)
	}
	if _, err := s.catalog.Seal(s.may.ID(), s.may.ID()); err == nil {
		t.Fatal("duplicate selection accepted")
	}
}

func TestToyLaneAndProductLawsEveryLaneSet(t *testing.T) {
	s := newToySetup()
	ids := []AxisID{s.may.ID(), s.must.ID(), s.unsupported.ID()}
	for selection := 0; selection < 1<<len(ids); selection++ {
		var chosen []AxisID
		for i, id := range ids {
			if selection&(1<<i) != 0 {
				chosen = append(chosen, id)
			}
		}
		t.Run(string(rune('A'+selection)), func(t *testing.T) {
			schema := mustSchema(t, &s.catalog, chosen...)
			arena := &Arena{}
			bottom := Bottom(arena, schema)
			a := bottom
			b := bottom
			c := bottom
			if _, ok := schema.byID[s.may.ID()]; ok {
				a = Put(arena, a, s.may, uint8(1))
				b = Put(arena, b, s.may, uint8(2))
				c = Put(arena, c, s.may, uint8(4))
			}
			if _, ok := schema.byID[s.must.ID()]; ok {
				a = Put(arena, a, s.must, uint8(0b1110))
				b = Put(arena, b, s.must, uint8(0b1101))
				c = Put(arena, c, s.must, uint8(0b1011))
			}
			if _, ok := schema.byID[s.unsupported.ID()]; ok {
				a = Put(arena, a, s.unsupported, uint8(1))
				b = Put(arena, b, s.unsupported, uint8(2))
				c = Put(arena, c, s.unsupported, uint8(4))
			}
			if !Equal(Join(arena, a, a), a) {
				t.Fatal("join idempotence")
			}
			if !Equal(Join(arena, a, b), Join(arena, b, a)) {
				t.Fatal("join commutativity")
			}
			if !Equal(Join(arena, Join(arena, a, b), c), Join(arena, a, Join(arena, b, c))) {
				t.Fatal("join associativity")
			}
			joined := Join(arena, a, b)
			if !LessOrEq(a, joined) || !LessOrEq(b, joined) {
				t.Fatal("join is not an upper bound")
			}
			if !LessOrEq(bottom, a) {
				t.Fatal("bottom is not <= sample")
			}
		})
	}
}

func TestMustPolarityIsReverseInclusion(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.must.ID())
	arena := &Arena{}
	a := Put(arena, Bottom(arena, schema), s.must, uint8(0b1110))
	b := Put(arena, Bottom(arena, schema), s.must, uint8(0b1101))
	joined := Join(arena, a, b)
	if got, _ := Get(joined, s.must); got != 0b1100 {
		t.Fatalf("must join = %04b, want intersection", got)
	}
	if !LessOrEq(a, joined) || LessOrEq(joined, a) {
		t.Fatal("must reverse-inclusion order is wrong")
	}
}

func TestToyLaneLawsExhaustive(t *testing.T) {
	s := newToySetup()
	for _, id := range []AxisID{s.may.ID(), s.must.ID(), s.unsupported.ID()} {
		spec := s.catalog.specs[s.catalog.byID[id]]
		t.Run(string(id), func(t *testing.T) {
			bottom, top := spec.bottom(), spec.top()
			for ai := 0; ai < 16; ai++ {
				a := any(uint8(ai))
				if !spec.lessOrEq(bottom, a) || !spec.lessOrEq(a, top) || !spec.equal(spec.join(a, a), a) {
					t.Fatalf("identity/bounds failed for %d", ai)
				}
				for bi := 0; bi < 16; bi++ {
					b := any(uint8(bi))
					ab := spec.join(a, b)
					if !spec.equal(ab, spec.join(b, a)) || !spec.lessOrEq(a, ab) || !spec.lessOrEq(b, ab) {
						t.Fatalf("join laws failed for %d,%d", ai, bi)
					}
					for ci := 0; ci < 16; ci++ {
						c := any(uint8(ci))
						if !spec.equal(spec.join(spec.join(a, b), c), spec.join(a, spec.join(b, c))) {
							t.Fatalf("associativity failed for %d,%d,%d", ai, bi, ci)
						}
						if spec.lessOrEq(a, c) && spec.lessOrEq(b, c) && !spec.lessOrEq(ab, c) {
							t.Fatalf("least upper bound failed for %d,%d <= %d", ai, bi, ci)
						}
					}
				}
			}
		})
	}
}

func TestStampsAndPairRelativeMasks(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.must.ID())
	arena := &Arena{}
	base := Bottom(arena, schema)
	noOp := Put(arena, base, s.may, uint8(0))
	if !ChangeMask(base, noOp).Empty() {
		t.Fatal("semantic no-op changed stamp")
	}
	left := Put(arena, base, s.may, uint8(1))
	right := Put(arena, base, s.must, uint8(7))
	if got := ChangeMask(left, right).Count(); got != 2 {
		t.Fatalf("unrelated descendants change mask = %d, want 2", got)
	}
	if _, scans := LessOrEqScans(left, left); scans != 0 {
		t.Fatalf("same-content scans = %d, want 0", scans)
	}
	joined := Join(arena, base, left)
	if !ChangeMask(joined, left).Empty() {
		t.Fatal("join did not carry equal operand stamps")
	}
	// Equal semantics with distinct histories must not depend on stamps.
	a := Put(arena, base, s.may, uint8(3))
	b := Put(arena, Put(arena, base, s.may, uint8(1)), s.may, uint8(3))
	if ChangeMask(a, b).Empty() || !Equal(a, b) || SemanticDigest(a) != SemanticDigest(b) {
		t.Fatal("semantic equality/digest incorrectly depends on history stamps")
	}
}

func TestMaskedLessOrEqDifferentialRandomized(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.must.ID(), s.unsupported.ID())
	arena := &Arena{}
	rng := rand.New(rand.NewSource(7))
	states := []State{Bottom(arena, schema)}
	for i := 0; i < 20_000; i++ {
		base := states[rng.Intn(len(states))]
		var next State
		switch rng.Intn(4) {
		case 0:
			next = Put(arena, base, s.may, uint8(rng.Intn(16)))
		case 1:
			next = Put(arena, base, s.must, uint8(rng.Intn(16)))
		case 2:
			next = Put(arena, base, s.unsupported, uint8(rng.Intn(16)))
		default:
			next = Join(arena, base, states[rng.Intn(len(states))])
		}
		states = append(states, next)
		if len(states) > 256 {
			states = states[len(states)-256:]
		}
		a := states[rng.Intn(len(states))]
		b := states[rng.Intn(len(states))]
		if got, want := LessOrEq(a, b), LessOrEqBaseline(a, b); got != want {
			t.Fatalf("iteration %d: masked=%v baseline=%v", i, got, want)
		}
		for lane := range a.slots {
			if a.slots[lane].stamp == b.slots[lane].stamp && !a.schema.specs[lane].equal(a.slots[lane].value, b.slots[lane].value) {
				t.Fatalf("iteration %d lane %d: equal stamp for unequal values", i, lane)
			}
		}
	}
}

func TestOneChangedLaneScansOnlyOneOfSeventeen(t *testing.T) {
	fixture := newComparisonFixture(17)
	if ok, scans := LessOrEqScans(fixture.base, fixture.changed); !ok || scans != 1 {
		t.Fatalf("masked result=%v scans=%d, want true/1", ok, scans)
	}
	if ok, scans := lessOrEq(fixture.base, fixture.changed, false); !ok || scans != 17 {
		t.Fatalf("baseline result=%v scans=%d, want true/17", ok, scans)
	}
}
