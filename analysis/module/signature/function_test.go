package signature

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFunctionEqualsComparesTypeAndEffectSeparately(t *testing.T) {
	fn := typ.Func().
		Param("value", typ.String).
		Returns(typ.Boolean).
		Build()
	io := Function{Type: fn, Effect: effect.Empty.With(control.IO{})}
	same := Function{Type: fn, Effect: effect.Empty.With(control.IO{})}
	pure := Function{Type: fn, Effect: effect.Empty}
	differentType := Function{
		Type: typ.Func().
			Param("value", typ.Number).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(control.IO{}),
	}

	if !io.Equals(same) {
		t.Fatalf("equal signatures were not equal")
	}
	if io.Equals(pure) {
		t.Fatalf("effect rows should be part of signature equality")
	}
	if io.Equals(differentType) {
		t.Fatalf("function type should be part of signature equality")
	}
}

func TestFunctionCloneCopiesEffectRowAndFunctionSlices(t *testing.T) {
	original := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Open("rho", control.IO{}),
	}

	clone := original.Clone()
	if !clone.Equals(original) {
		t.Fatalf("clone = %v, want %v", clone, original)
	}
	if clone.Type == original.Type {
		t.Fatalf("clone should rebuild the function carrier type")
	}

	clone.Type.Params[0].Type = typ.Number
	clone.Effect.Tail.Name = "sigma"

	if original.Type.Params[0].Type != typ.String {
		t.Fatalf("clone mutation changed original function params")
	}
	if original.Effect.Tail.Name != "rho" {
		t.Fatalf("clone mutation changed original effect tail")
	}
}

func TestFunctionCloneCopiesOperationalEffects(t *testing.T) {
	original := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Build(),
		OperationalEffects: &OperationalEffects{
			EscapeEvents: []EscapeEvent{{
				Target:    pathdom.NewPlaceholder(0).Field("child"),
				Kind:      EscapeSend,
				Recursive: true,
			}},
			PathStaticMembers: []PathStaticMemberFact{{
				Path: pathdom.NewPlaceholder(0).Field("member"),
				Type: typ.String,
			}},
		},
	}

	clone := original.Clone()
	if !clone.Equals(original) {
		t.Fatalf("clone = %#v, want %#v", clone, original)
	}
	if clone.OperationalEffects == original.OperationalEffects {
		t.Fatalf("clone should not share operational effects pointer")
	}

	clone.OperationalEffects.EscapeEvents[0].Target.Segments[0].Name = "other"
	clone.OperationalEffects.EscapeEvents[0].Kind = EscapeOpaque
	clone.OperationalEffects.PathStaticMembers[0].Path.Segments[0].Name = "changed"

	if got := original.OperationalEffects.EscapeEvents[0].Target.String(); got != "$0.child" {
		t.Fatalf("clone mutation changed original target: %s", got)
	}
	if got := original.OperationalEffects.EscapeEvents[0].Kind; got != EscapeSend {
		t.Fatalf("clone mutation changed original kind: %v", got)
	}
	if got := original.OperationalEffects.PathStaticMembers[0].Path.String(); got != "$0.member" {
		t.Fatalf("clone mutation changed original static member path: %s", got)
	}
}

func TestFunctionEqualsDistinguishesOperationalEffectsAuthority(t *testing.T) {
	fn := typ.Func().Param("value", typ.String).Build()
	nilOperational := Function{Type: fn}
	emptyOperational := Function{Type: fn, OperationalEffects: &OperationalEffects{}}
	withOperational := Function{Type: fn, OperationalEffects: &OperationalEffects{
		FrozenTables: []FrozenTable{{Target: pathdom.NewPlaceholder(0)}},
	}}
	withStaticMembers := Function{Type: fn, OperationalEffects: &OperationalEffects{
		PathStaticMembers: []PathStaticMemberFact{{Path: pathdom.NewPlaceholder(0), Type: typ.String}},
	}}

	if nilOperational.Equals(emptyOperational) {
		t.Fatalf("nil operational effects should differ from authoritative empty effects")
	}
	if emptyOperational.Equals(withOperational) {
		t.Fatalf("different operational effects should not compare equal")
	}
	if withOperational.Equals(withStaticMembers) {
		t.Fatalf("different operational effects lanes should not compare equal")
	}
}

func TestFunctionStringIncludesNonPureEffect(t *testing.T) {
	sig := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(control.IO{}),
	}

	if got, want := sig.String(), "fun(value: string) -> boolean ! {io}"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
