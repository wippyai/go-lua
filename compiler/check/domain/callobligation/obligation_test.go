package callobligation

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestInformativeTypeConcreteBoundary(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typ.NewRecord().ReadonlyField("value", tp).Build())

	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "concrete primitive", in: typ.String, want: true},
		{name: "any is gradual top", in: typ.Any, want: false},
		{name: "unknown is absent", in: typ.Unknown, want: false},
		{name: "union with any is gradual top", in: typ.NewUnion(typ.String, typ.Any), want: false},
		{name: "free type param", in: tp, want: false},
		{name: "record with any field is structural", in: typ.NewRecord().ReadonlyField("run", typ.Any).Build(), want: true},
		{name: "record with free type param field", in: typ.NewRecord().ReadonlyField("value", tp).Build(), want: false},
		{name: "function with free type param", in: typ.Func().Param("x", tp).Returns(tp).Build(), want: false},
		{name: "self placeholder", in: typ.Self, want: false},
		{
			name: "interface binds self in methods",
			in: typ.NewInterface("time.Time", []typ.Method{{
				Name: "sub",
				Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(typ.Number).Build(),
			}}),
			want: true,
		},
		{name: "instantiated with free type param", in: typ.Instantiate(box, tp), want: false},
		{name: "instantiated with concrete arg", in: typ.Instantiate(box, typ.String), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InformativeType(tt.in); got != tt.want {
				t.Fatalf("InformativeType(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasFreeVariableUsesRecursiveFamilySeen(t *testing.T) {
	closed := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			ReadonlyField("next", typ.NewOptional(self)).
			ReadonlyField("get", typ.Func().OptParam("self", typ.Any).Returns(self).Build()).
			Build()
	})
	if HasFreeVariable(closed) {
		t.Fatalf("closed recursive obligation reported free variable: %v", closed)
	}
	if !InformativeType(closed) {
		t.Fatalf("closed recursive obligation should be informative: %v", closed)
	}

	open := typ.NewRecursive("OpenNode", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			ReadonlyField("next", typ.NewOptional(self)).
			ReadonlyField("get", typ.Func().OptParam("self", typ.Any).Returns(typ.NewRef("", "T")).Build()).
			Build()
	})
	if !HasFreeVariable(open) {
		t.Fatalf("recursive obligation with open return did not report free variable: %v", open)
	}
	if InformativeType(open) {
		t.Fatalf("recursive obligation with open return should not be informative: %v", open)
	}
}
