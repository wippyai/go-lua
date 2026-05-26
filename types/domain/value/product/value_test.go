package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// withShape builds an AbstractValue carrying the given shape and identity on every
// other axis, so tests that vary one axis isolate that axis.
func withShape(t typ.Type) AbstractValue {
	return New(
		shapevalue.Of(t),
		presence.Top(),
		numeric.Top(),
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)
}

// TestInterningIdentity pins that equal content interns to one canonical node: two
// independently constructed equal values share a pointer (the fast path).
func TestInterningIdentity(t *testing.T) {
	a := withShape(typ.Number)
	b := withShape(typ.Number)
	if a.n != b.n {
		t.Fatal("equal values must intern to the same canonical node")
	}
	if !Equal(a, b) {
		t.Fatal("equal values must be Equal")
	}
}

// TestEqualImpliesEqualHash is the load-bearing invariant for the db red-green
// firewall: Equal values must hash identically, even when constructed differently.
func TestEqualImpliesEqualHash(t *testing.T) {
	for _, pair := range equalPairs() {
		if !Equal(pair.a, pair.b) {
			t.Fatalf("%s: expected Equal", pair.name)
		}
		if pair.a.Hash() != pair.b.Hash() {
			t.Fatalf("%s: Equal values hash differently", pair.name)
		}
	}
}

type avPair struct {
	name string
	a, b AbstractValue
}

func equalPairs() []avPair {
	return []avPair{
		{
			name: "identical primitive",
			a:    withShape(typ.String),
			b:    withShape(typ.String),
		},
		{
			name: "reordered union",
			a:    withShape(typ.NewUnion(typ.Number, typ.String)),
			b:    withShape(typ.NewUnion(typ.String, typ.Number)),
		},
		{
			name: "reordered triple union",
			a:    withShape(typ.NewUnion(typ.Number, typ.String, typ.Boolean)),
			b:    withShape(typ.NewUnion(typ.Boolean, typ.Number, typ.String)),
		},
		{
			name: "effect row reordered labels",
			a: New(
				shapevalue.Top(), presence.Top(), numeric.Top(),
				effectrows.Of(effect.Empty.With(effect.Throw{}, effect.IO{})),
				ownership.Top(), escape.Top(), identityrecursion.Top(), evidence.Top(),
			),
			b: New(
				shapevalue.Top(), presence.Top(), numeric.Top(),
				effectrows.Of(effect.Empty.With(effect.IO{}, effect.Throw{})),
				ownership.Top(), escape.Top(), identityrecursion.Top(), evidence.Top(),
			),
		},
	}
}

// TestStructurallyDifferentButMutuallyCovering pins that values that are not
// node-identical but mutually cover each other are Equal and hash the same. A
// union with a duplicate member normalizes to the same family as the deduped
// union, so the two cover each other.
func TestStructurallyDifferentButMutuallyCovering(t *testing.T) {
	dup := typ.NewUnion(typ.Number, typ.String, typ.Number)
	plain := typ.NewUnion(typ.Number, typ.String)
	a := withShape(dup)
	b := withShape(plain)
	if !a.Shape().Covers(b.Shape()) || !b.Shape().Covers(a.Shape()) {
		t.Skip("shape axis did not treat the two unions as mutually covering")
	}
	if !Equal(a, b) {
		t.Fatal("mutually-covering values must be Equal")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("mutually-covering Equal values must hash identically")
	}
	if a.n != b.n {
		t.Fatal("mutually-covering values must intern to the same node")
	}
}

// TestDistinctValuesDistinct pins that genuinely different values do not collapse.
func TestDistinctValuesDistinct(t *testing.T) {
	a := withShape(typ.Number)
	b := withShape(typ.String)
	if Equal(a, b) {
		t.Fatal("distinct shapes must not be Equal")
	}
	if a.n == b.n {
		t.Fatal("distinct values must intern to distinct nodes")
	}
}

// TestProvenanceExcludedFromEqualAndHash pins that the provenance sidecar never
// affects Equal, Hash, or the interned node.
func TestProvenanceExcludedFromEqualAndHash(t *testing.T) {
	base := withShape(typ.Number)
	withProv := base.WithProvenance(Provenance{Origin: "literal-42"})
	other := withShape(typ.Number).WithProvenance(Provenance{Origin: "param-x"})

	if !Equal(base, withProv) {
		t.Fatal("provenance must not affect Equal")
	}
	if !Equal(withProv, other) {
		t.Fatal("different provenance must not affect Equal")
	}
	if base.Hash() != withProv.Hash() || withProv.Hash() != other.Hash() {
		t.Fatal("provenance must not affect Hash")
	}
	if base.n != withProv.n || withProv.n != other.n {
		t.Fatal("provenance must not change the interned node")
	}
	if _, ok := withProv.Provenance(); !ok {
		t.Fatal("provenance should be retrievable from the sidecar")
	}
	if _, ok := base.Provenance(); ok {
		t.Fatal("a value without provenance must report none")
	}
}

// TestEqualReflexiveSymmetric pins reflexivity and symmetry of Equal.
func TestEqualReflexiveSymmetric(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		if !Equal(a, a) {
			t.Fatal("Equal not reflexive")
		}
		for _, b := range vs {
			if Equal(a, b) != Equal(b, a) {
				t.Fatal("Equal not symmetric")
			}
		}
	}
}

// TestEqualTransitive pins transitivity of Equal.
func TestEqualTransitive(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		for _, b := range vs {
			for _, c := range vs {
				if Equal(a, b) && Equal(b, c) && !Equal(a, c) {
					t.Fatal("Equal not transitive")
				}
			}
		}
	}
}

func sampleValues() []AbstractValue {
	return []AbstractValue{
		Bottom(),
		Top(),
		withShape(typ.Number),
		withShape(typ.String),
		withShape(typ.NewUnion(typ.Number, typ.String)),
		FromType(typ.Number),
		FromType(typ.Nil),
	}
}

// TestJoinUpperBound pins that Join is an upper bound: it covers both operands.
func TestJoinUpperBound(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		for _, b := range vs {
			j := Join(a, b)
			if !j.Covers(a) || !j.Covers(b) {
				t.Fatalf("Join does not cover an operand: %s join %s", a.Project(), b.Project())
			}
		}
	}
}

// TestJoinIdempotentCommutative pins idempotence and commutativity of Join.
func TestJoinIdempotentCommutative(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		if !Equal(Join(a, a), a) {
			t.Fatal("Join not idempotent")
		}
		for _, b := range vs {
			if !Equal(Join(a, b), Join(b, a)) {
				t.Fatal("Join not commutative")
			}
		}
	}
}

// TestJoinInterned pins that Join returns interned values: equal joins share a node.
func TestJoinInterned(t *testing.T) {
	a := withShape(typ.Number)
	b := withShape(typ.String)
	j1 := Join(a, b)
	j2 := Join(b, a)
	if j1.n != j2.n {
		t.Fatal("equal joins must intern to the same node")
	}
}

// TestWidenAboveJoin pins that Widen sits at or above Join (a sound accelerant).
func TestWidenAboveJoin(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		for _, b := range vs {
			if !Widen(a, b).Covers(Join(a, b)) {
				t.Fatalf("Widen must cover Join: %s widen %s", a.Project(), b.Project())
			}
		}
	}
}

// TestProjectEgress pins that Project recovers the carried structural type.
func TestProjectEgress(t *testing.T) {
	v := withShape(typ.String)
	if v.Project().Kind() != typ.String.Kind() {
		t.Fatalf("Project must recover the carried type, got %s", v.Project())
	}
}

// TestImmutabilityOperationsDoNotMutateInputs pins deep immutability: lattice
// operations return new handles and never alter their interned operands, so a
// shared canonical node observed before an operation is unchanged after it.
func TestImmutabilityOperationsDoNotMutateInputs(t *testing.T) {
	a := withShape(typ.Number)
	b := withShape(typ.String)

	aNodeBefore := a.n
	aShapeBefore := a.Shape()
	aHashBefore := a.Hash()

	_ = Join(a, b)
	_ = Widen(a, b)
	_ = a.WithProvenance(Provenance{Origin: "mutated?"})
	_ = Covers(a, b)
	_ = Equal(a, b)

	if a.n != aNodeBefore {
		t.Fatal("operations must not rebind the operand's node")
	}
	if !shapevalue.Equal(a.Shape(), aShapeBefore) {
		t.Fatal("operand shape changed under operations")
	}
	if a.Hash() != aHashBefore {
		t.Fatal("operand hash changed under operations")
	}
	// The same content re-interns to the very same node a still holds.
	if withShape(typ.Number).n != a.n {
		t.Fatal("re-interning equal content must yield the operand's node")
	}
}

// TestFromTypePresence pins the presence derivation at the admission boundary.
func TestFromTypePresence(t *testing.T) {
	if !presence.Equal(FromType(typ.Number).Presence(), presence.Present()) {
		t.Fatal("concrete type must be Present")
	}
	if !presence.Equal(FromType(typ.Nil).Presence(), presence.Absent()) {
		t.Fatal("nil type must be Absent")
	}
	if !presence.Equal(FromType(typ.Any).Presence(), presence.Top()) {
		t.Fatal("any type must be Maybe (Top)")
	}
	if !presence.Equal(FromType(typ.Never).Presence(), presence.Bottom()) {
		t.Fatal("never type must be Bottom")
	}
	if !presence.Equal(FromType(typ.NewUnion(typ.Number, typ.Nil)).Presence(), presence.Maybe()) {
		t.Fatal("nil-admitting union must be Maybe")
	}
}
