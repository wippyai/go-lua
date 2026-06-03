package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/typ"
)

// muNextRoundTrip builds the canonical recursive record family mu X.{next: X?} used
// by the round-trip lossless test. It mirrors muNext in recursive_test.go but is
// named distinctly so the two test files stay independent.
func muNextRoundTrip() typ.Type {
	return typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", typ.NewOptional(self)).Build()
	})
}

// TestFromTypeProjectRoundTripLossless is the P3.1 representational-adequacy gate:
// lifting a structural type into the Shape/Value axis and projecting it back must
// preserve the value-domain convergence distinction. SameConvergedFact is the flow
// engine's no-op relation, so a value seeded `unknown` must not round-trip to `any`.
//
// The check is on the Shape/Value axis, the axis P3.1 corrects. Nilability is
// factored onto the Presence axis by design, so the shape of a nil-admitting type
// carries only its non-nil structural content; the projection round-trips that
// content, and a separate presence assertion guards the nilability that lives off
// the shape.
func TestFromTypeProjectRoundTripLossless(t *testing.T) {
	cases := []struct {
		name string
		t    typ.Type
		pres presence.Value
	}{
		{"any", typ.Any, presence.Top()},
		{"unknown", typ.Unknown, presence.Top()},
		{"nil", typ.Nil, presence.Absent()},
		{"number", typ.Number, presence.Present()},
		{"string", typ.String, presence.Present()},
		{"record", typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build(), presence.Present()},
		{"union", typ.NewUnion(typ.Number, typ.String), presence.Present()},
		{"recursive", muNextRoundTrip(), presence.Present()},
	}

	for _, c := range cases {
		av := FromType(c.t)
		// Re-lifting the shape projection must yield the same Shape/Value axis point:
		// the round-trip preserves the converged fact without loss.
		reShape := shapevalue.Of(av.Project())
		if !shapevalue.Equal(av.Shape(), reShape) {
			t.Fatalf("%s: shape round-trip changed the value: shape=%s -> reLifted=%s", c.name, av.Shape(), reShape)
		}
		// Presence carries the nilability the shape factors off, so the full value is
		// lossless once both axes are considered.
		if !presence.Equal(av.Presence(), c.pres) {
			t.Fatalf("%s: presence mismatch: got %s want %s", c.name, av.Presence(), c.pres)
		}
	}
}

// TestProjectValueRoundTripLossless is the STEP 0 egress gate: ProjectValue is the
// lossless inverse of FromType at the value-domain boundary, so the full structural
// type (including nilability, which FromType factors onto presence) is recovered up
// to the flow engine's convergence relation SameConvergedFact.
func TestProjectValueRoundTripLossless(t *testing.T) {
	cases := []struct {
		name string
		t    typ.Type
	}{
		{"any", typ.Any},
		{"unknown", typ.Unknown},
		{"nil", typ.Nil},
		{"never", typ.Never},
		{"number", typ.Number},
		{"string", typ.String},
		{"optional-string", typ.NewOptional(typ.String)},
		{"optional-number", typ.NewOptional(typ.Number)},
		{"record", typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()},
		{"record-opt-field", typ.NewRecord().Field("x", typ.Number).OptField("y", typ.String).Build()},
		{"union", typ.NewUnion(typ.Number, typ.String)},
		{"nilable-union", typ.NewUnion(typ.Number, typ.String, typ.Nil)},
		{"array", typ.NewArray(typ.Number)},
		{"map", typ.NewMap(typ.String, typ.Number)},
		{"recursive", muNextRoundTrip()},
	}

	for _, c := range cases {
		got := FromType(c.t).ProjectValue()
		if !value.SameConvergedFact(c.t, got) {
			t.Fatalf("%s: ProjectValue not lossless: FromType(%s).ProjectValue() = %s", c.name, c.t, got)
		}
	}
}

func TestProjectValueOrUnknownZero(t *testing.T) {
	got := ProjectValueOrUnknown(AbstractValue{})
	if !typ.IsUnknown(got) {
		t.Fatalf("ProjectValueOrUnknown(zero) = %v, want unknown", got)
	}
}

func TestVectorAdmissionAndProjectionBoundary(t *testing.T) {
	values := FromTypes([]typ.Type{typ.String, nil, typ.Unknown})
	if len(values) != 3 {
		t.Fatalf("FromTypes length = %d, want 3", len(values))
	}
	if values[0].IsZero() || !typ.TypeEquals(values[0].ProjectValue(), typ.String) {
		t.Fatalf("FromTypes[0] = %v, want string value", values[0].ProjectValue())
	}
	if !values[1].IsZero() || !values[2].IsZero() {
		t.Fatalf("nil/unknown slots must remain zero product values: %v", values)
	}
	projected := ProjectValuesOrUnknown(values)
	if len(projected) != 3 || !typ.TypeEquals(projected[0], typ.String) || !typ.IsUnknown(projected[1]) || !typ.IsUnknown(projected[2]) {
		t.Fatalf("ProjectValuesOrUnknown = %v, want [string unknown unknown]", projected)
	}
}

func TestTotalVectorAdmissionProducesCarrierValues(t *testing.T) {
	values := FromTypesTotal([]typ.Type{typ.String, nil, typ.Unknown})
	if len(values) != 3 {
		t.Fatalf("FromTypesTotal length = %d, want 3", len(values))
	}
	for i, v := range values {
		if v.IsZero() {
			t.Fatalf("FromTypesTotal[%d] is zero; total admission must produce product carrier values", i)
		}
	}
	projected := ProjectValuesOrUnknown(values)
	if len(projected) != 3 || !typ.TypeEquals(projected[0], typ.String) || !typ.IsUnknown(projected[1]) || !typ.IsUnknown(projected[2]) {
		t.Fatalf("ProjectValuesOrUnknown(FromTypesTotal) = %v, want [string unknown unknown]", projected)
	}
	joined := Domain.Join(values[1], FromType(typ.Number))
	if joined.IsZero() {
		t.Fatal("joining total-admitted unknown with number produced zero")
	}
}

// TestUnknownIsNotAny is the core P3.1 invariant: the gradual placeholder `unknown`
// and the dynamic top `any` are distinct converged facts, so they must not be Equal
// and must intern to distinct nodes. The old mutual-Covers equality conflated them
// because each covers the other under gradual subtyping.
func TestUnknownIsNotAny(t *testing.T) {
	anyV := FromType(typ.Any)
	unknownV := FromType(typ.Unknown)

	if Equal(anyV, unknownV) {
		t.Fatal("FromType(any) and FromType(unknown) must not be Equal: unknown is a refinable placeholder, any is dynamic top")
	}
	if anyV.n == unknownV.n {
		t.Fatal("FromType(any) and FromType(unknown) must intern to distinct nodes")
	}
	// The projection must recover the original placeholder, not collapse to any.
	if !typ.IsUnknown(unknownV.Project()) {
		t.Fatalf("FromType(unknown).Project() must stay unknown, got %s", unknownV.Project())
	}
	if !typ.IsAny(anyV.Project()) {
		t.Fatalf("FromType(any).Project() must stay any, got %s", anyV.Project())
	}
	// SameConvergedFact, the relation Equal is built on, already separates them.
	if value.SameConvergedFact(typ.Any, typ.Unknown) {
		t.Fatal("SameConvergedFact must distinguish any from unknown")
	}
}

// TestReorderedUnionStaysEqual pins that the finer Equal still folds reordered
// unions: admission through typ.NewUnion gives a canonical member order, so {A|B}
// and {B|A} are the same converged fact and intern to one node.
func TestReorderedUnionStaysEqual(t *testing.T) {
	ab := FromType(typ.NewUnion(typ.Number, typ.String))
	ba := FromType(typ.NewUnion(typ.String, typ.Number))

	if !Equal(ab, ba) {
		t.Fatal("reordered unions must stay Equal under SameConvergedFact (canonical member order)")
	}
	if ab.n != ba.n {
		t.Fatal("reordered unions must intern to one canonical node")
	}
	if ab.Hash() != ba.Hash() {
		t.Fatal("reordered Equal unions must hash identically")
	}

	abc := FromType(typ.NewUnion(typ.Number, typ.String, typ.Boolean))
	cab := FromType(typ.NewUnion(typ.Boolean, typ.Number, typ.String))
	if !Equal(abc, cab) || abc.n != cab.n {
		t.Fatal("reordered triple unions must stay Equal and share a node")
	}
}

// TestRecursiveFamilyStaysEqual pins that two observations of the same recursive
// product family remain Equal under the finer Equal, since the flow engine
// converges them under SameConvergedFact.
func TestRecursiveFamilyStaysEqual(t *testing.T) {
	a := FromType(muNextRoundTrip())
	b := FromType(muNextRoundTrip())

	if !Equal(a, b) {
		t.Fatal("same recursive family must stay Equal under SameConvergedFact")
	}
	if a.n != b.n {
		t.Fatal("same recursive family must intern to one canonical node")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("Equal recursive values must hash identically")
	}
}

// TestJoinCommutativeAcrossUnionOrder pins that the finer Equal keeps Join
// commutative when the convergence merge composes unions in operand order: the
// axis canonicalizes the merge output, so joining in either order is the same
// converged fact.
func TestJoinCommutativeAcrossUnionOrder(t *testing.T) {
	a := withShape(typ.Number)
	b := withShape(typ.NewUnion(typ.Integer, typ.String))

	ab := Join(a, b)
	ba := Join(b, a)
	if !Equal(ab, ba) {
		t.Fatalf("Join must be commutative under the finer Equal: %s vs %s", ab.Project(), ba.Project())
	}
	if ab.n != ba.n {
		t.Fatal("commutative joins must intern to one node")
	}
}
