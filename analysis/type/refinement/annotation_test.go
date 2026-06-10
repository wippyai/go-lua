package refinement

import (
	"testing"

	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestIsClosedUnionAnnotationRecognizesInstantiatedGenericUnion(t *testing.T) {
	tp := NewTypeParam("T", nil)
	result := NewGeneric("Result", []*TypeParam{tp}, NewUnion(
		NewRecord().Field("ok", LiteralBool(true)).Field("value", tp).Build(),
		NewRecord().Field("ok", LiteralBool(false)).Field("error", String).Build(),
	))

	if !IsClosedUnionAnnotation(Instantiate(result, NewRecord().Field("id", String).Build())) {
		t.Fatal("IsClosedUnionAnnotation(Result<User>) = false, want true")
	}
}

func TestIsRefinableAnnotation(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"nil", nil, false},
		{"any", Any, false},
		{"unknown", Unknown, false},
		{"optional any", NewOptional(Any), false},
		{"array any", NewArray(Any), true},
		{"map string any", NewMap(String, Any), true},
		{"map string any array", NewMap(String, NewArray(Any)), true},
		{"open table top", NewRecord().SetOpen(true).Build(), true},
		{"array or open table top", NewUnion(NewArray(Any), NewRecord().SetOpen(true).Build()), true},
		{"record map any", NewRecord().MapComponent(String, Any).Build(), true},
		{"record", NewRecord().Field("id", String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsRefinableAnnotation(tt.t); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneLessPreciseRefinableUnionMembersDropsDominatedSoftAlternative(t *testing.T) {
	soft := NewMap(String, NewArray(Any))
	precise := NewMap(String, NewArray(NewRecord().Field("id", String).Build()))

	got := PruneLessPreciseRefinableUnionMembers(NewUnion(soft, precise), func(candidate, baseline Type) bool {
		return TypeEquals(candidate, precise) && TypeEquals(baseline, soft)
	}, NewUnion)
	if !TypeEquals(got, precise) {
		t.Fatalf("pruned union = %v, want %v", got, precise)
	}
}
