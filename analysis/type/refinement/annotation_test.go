package refinement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestIsClosedUnionAnnotationRecognizesInstantiatedGenericUnion(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typ.NewUnion(
		typ.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", tp).Build(),
		typ.NewRecord().Field("ok", typ.LiteralBool(false)).Field("error", typ.String).Build(),
	))

	if !IsClosedUnionAnnotation(typ.Instantiate(result, typ.NewRecord().Field("id", typ.String).Build())) {
		t.Fatal("IsClosedUnionAnnotation(Result<User>) = false, want true")
	}
}

func TestIsRefinableAnnotation(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil", nil, false},
		{"any", typ.Any, false},
		{"unknown", typ.Unknown, false},
		{"optional any", typ.NewOptional(typ.Any), false},
		{"array any", typ.NewArray(typ.Any), true},
		{"map string any", typ.NewMap(typ.String, typ.Any), true},
		{"map string any array", typ.NewMap(typ.String, typ.NewArray(typ.Any)), true},
		{"open table top", typ.NewRecord().SetOpen(true).Build(), true},
		{"array or open table top", typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().SetOpen(true).Build()), true},
		{"record map any", typ.NewRecord().MapComponent(typ.String, typ.Any).Build(), true},
		{"record", typ.NewRecord().Field("id", typ.String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsRefinableAnnotation(tt.t); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneLessPreciseRefinableUnionMembersDropsDominatedSoftAlternative(t *testing.T) {
	soft := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	precise := typ.NewMap(typ.String, typ.NewArray(typ.NewRecord().Field("id", typ.String).Build()))

	got := PruneLessPreciseRefinableUnionMembers(typ.NewUnion(soft, precise), func(candidate, baseline typ.Type) bool {
		return typ.TypeEquals(candidate, precise) && typ.TypeEquals(baseline, soft)
	}, typ.NewUnion)
	if !typ.TypeEquals(got, precise) {
		t.Fatalf("pruned union = %v, want %v", got, precise)
	}
}
