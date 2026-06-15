package refinement

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/type/subst"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestIsClosedUnionAnnotationRecognizesInstantiatedGenericUnion(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(
		typetable.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", tp).Build(),
		typetable.NewRecord().Field("ok", typ.LiteralBool(false)).Field("error", typ.String).Build(),
	))

	if !IsClosedUnionAnnotation(typ.Instantiate(result, typetable.NewRecord().Field("id", typ.String).Build())) {
		t.Fatal("IsClosedUnionAnnotation(Result<User>) = false, want true")
	}
}

func TestIsClosedUnionAnnotationRejectsNestedInstantiatedAnyOrUnknown(t *testing.T) {
	t.Run("any", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		inner := typ.NewGeneric("MaybeAny", []*typ.TypeParam{tp}, typ.Any)
		outer := typ.NewGeneric("Wrapper", []*typ.TypeParam{tp}, typeexpr.Union(
			typ.Instantiate(inner, tp),
			typetable.NewRecord().Field("value", tp).Build(),
		))

		if IsClosedUnionAnnotation(typ.Instantiate(outer, typ.String)) {
			t.Fatal("instantiated member expanding to any should not be closed")
		}
	})

	t.Run("unknown", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		inner := typ.NewGeneric("MaybeUnknown", []*typ.TypeParam{tp}, typ.Unknown)
		outer := typ.NewGeneric("Wrapper", []*typ.TypeParam{tp}, typeexpr.Union(
			typ.Instantiate(inner, tp),
			typetable.NewRecord().Field("value", tp).Build(),
		))

		if IsClosedUnionAnnotation(typ.Instantiate(outer, typ.String)) {
			t.Fatal("instantiated member expanding to unknown should not be closed")
		}
	})
}

func TestExpandInstantiatedPreservesSameNameFunctionBinder(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	inner := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(inner).
		Param("value", inner).
		Returns(inner).
		Build()
	generic := typ.NewGeneric("Wrapper", []*typ.TypeParam{outer}, typeexpr.Union(
		typetable.NewRecord().Field("value", outer).Build(),
		fn,
	))

	expanded := subst.ExpandInstantiated(typ.Instantiate(generic, typ.String))
	union, ok := expanded.(*typ.Union)
	if !ok {
		t.Fatalf("expanded = %T, want union", expanded)
	}

	var gotFn *typ.Function
	for _, member := range union.Members {
		candidate, ok := member.(*typ.Function)
		if ok {
			gotFn = candidate
			break
		}
	}
	if gotFn == nil {
		t.Fatalf("expanded union did not contain function member: %v", union)
	}
	if len(gotFn.TypeParams) != 1 || gotFn.TypeParams[0] != inner {
		t.Fatalf("function binder was captured or rewritten: %v", gotFn.TypeParams)
	}
	if len(gotFn.Params) != 1 || gotFn.Params[0].Type != inner {
		t.Fatalf("function parameter type = %v, want nested binder", gotFn.Params[0].Type)
	}
	if len(gotFn.Returns) != 1 || gotFn.Returns[0] != inner {
		t.Fatalf("function return type = %v, want nested binder", gotFn.Returns[0])
	}
}

func TestIsClosedUnionAnnotationHandlesSelfInstantiatingGenericUnionWithoutHang(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	generic := typ.NewGeneric("Loop", []*typ.TypeParam{tp}, nil)
	generic.SetBody(typeexpr.Union(
		typetable.NewRecord().Field("value", tp).Build(),
		typ.Instantiate(generic, tp),
	))

	done := make(chan bool, 1)
	go func() {
		done <- IsClosedUnionAnnotation(typ.Instantiate(generic, typ.String))
	}()

	select {
	case got := <-done:
		if !got {
			t.Fatal("self-instantiating generic union should remain closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("IsClosedUnionAnnotation(self-instantiating generic) hung")
	}
}

func TestIsClosedUnionAnnotationRejectsForwardGenericWithNilBody(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	generic := typ.NewGeneric("Forward", []*typ.TypeParam{tp}, nil)

	if IsClosedUnionAnnotation(typ.Instantiate(generic, typ.String)) {
		t.Fatal("forward generic with nil body should not be closed")
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
		{"optional any", typeexpr.Optional(typ.Any), false},
		{"array any", typ.NewArray(typ.Any), true},
		{"map string any", typ.NewMap(typ.String, typ.Any), true},
		{"map string any array", typ.NewMap(typ.String, typ.NewArray(typ.Any)), true},
		{"open table top", typetable.NewRecord().SetOpen(true).Build(), true},
		{"array or open table top", typeexpr.Union(typ.NewArray(typ.Any), typetable.NewRecord().SetOpen(true).Build()), true},
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

	got := PruneLessPreciseRefinableUnionMembers(typeexpr.Union(soft, precise), func(candidate, baseline typ.Type) bool {
		return typ.TypeEquals(candidate, precise) && typ.TypeEquals(baseline, soft)
	}, typeexpr.Union)
	if !typ.TypeEquals(got, precise) {
		t.Fatalf("pruned union = %v, want %v", got, precise)
	}
}
