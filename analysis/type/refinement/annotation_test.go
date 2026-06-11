package refinement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/identity"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestIsClosedUnionAnnotationRecognizesInstantiatedGenericUnion(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typ.NewUnion(
		typetable.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", tp).Build(),
		typetable.NewRecord().Field("ok", typ.LiteralBool(false)).Field("error", typ.String).Build(),
	))

	if !IsClosedUnionAnnotation(typ.Instantiate(result, typetable.NewRecord().Field("id", typ.String).Build())) {
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
		{"open table top", typetable.NewRecord().SetOpen(true).Build(), true},
		{"array or open table top", typ.NewUnion(typ.NewArray(typ.Any), typetable.NewRecord().SetOpen(true).Build()), true},
		{"record map any", typetable.NewRecord().MapComponent(typ.String, typ.Any).Build(), true},
		{"record", typetable.NewRecord().Field("id", typ.String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsRefinableAnnotation(tt.t); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneLessPreciseRefinableUnionMembersDropsDominatedSoftAlternative(t *testing.T) {
	soft := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	precise := typ.NewMap(typ.String, typ.NewArray(typetable.NewRecord().Field("id", typ.String).Build()))

	got := PruneLessPreciseRefinableUnionMembers(typ.NewUnion(soft, precise), func(candidate, baseline typ.Type) bool {
		return identity.TypeEquals(candidate, precise) && identity.TypeEquals(baseline, soft)
	}, typ.NewUnion)
	if !identity.TypeEquals(got, precise) {
		t.Fatalf("pruned union = %v, want %v", got, precise)
	}
}
