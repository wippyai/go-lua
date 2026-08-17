package projection

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestProjectionString(t *testing.T) {
	tests := []struct {
		name string
		p    Projection
		want string
	}{
		{name: "empty", p: Projection{}, want: ""},
		{
			name: "chain",
			p: Projection{Steps: []Step{
				Field("make"),
				CallableReturn(),
				GenericArg(0),
			}},
			want: "make.return.arg[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStepConstructors(t *testing.T) {
	generic := typ.NewGeneric("Box", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.String)

	tests := []struct {
		name string
		got  Step
		want Step
	}{
		{name: "field", got: Field("name"), want: Step{Kind: StepField, Field: "name"}},
		{name: "callable return", got: CallableReturn(), want: Step{Kind: StepCallableReturn}},
		{name: "generic arg", got: GenericArg(1), want: Step{Kind: StepGenericArg, Index: 1}},
		{name: "instantiate generic", got: InstantiateGeneric(generic), want: Step{Kind: StepInstantiateGeneric, Type: generic}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Kind != tt.want.Kind || tt.got.Field != tt.want.Field ||
				tt.got.Index != tt.want.Index || !typ.TypeEquals(tt.got.Type, tt.want.Type) {
				t.Fatalf("constructor = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestStepString(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	generic := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	tests := []struct {
		name string
		step Step
		want string
	}{
		{name: "field", step: Field("name"), want: "name"},
		{name: "callable return", step: CallableReturn(), want: "return"},
		{name: "generic arg", step: GenericArg(2), want: "arg[2]"},
		{name: "instantiate generic", step: InstantiateGeneric(generic), want: "instantiate[Box<T>]"},
		{name: "unknown", step: Step{}, want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjectionEqual(t *testing.T) {
	a := Projection{Steps: []Step{Field("name"), GenericArg(0)}}
	b := Projection{Steps: []Step{Field("name"), GenericArg(0)}}
	c := Projection{Steps: []Step{Field("age"), GenericArg(0)}}

	if !Equal(a, b) {
		t.Fatal("matching projections should be equal")
	}
	if Equal(a, c) {
		t.Fatal("different projections should not be equal")
	}
}

func TestProjectionEqualInstantiateGeneric(t *testing.T) {
	p1 := typ.NewTypeParam("T", nil)
	p2 := typ.NewTypeParam("T", nil)
	box1 := typ.NewGeneric("Box", []*typ.TypeParam{p1}, p1)
	box2 := typ.NewGeneric("Box", []*typ.TypeParam{p2}, p2)
	other := typ.NewGeneric("Other", []*typ.TypeParam{p1}, p1)

	a := Projection{Steps: []Step{InstantiateGeneric(box1)}}
	b := Projection{Steps: []Step{InstantiateGeneric(box2)}}
	c := Projection{Steps: []Step{InstantiateGeneric(other)}}

	if !Equal(a, b) {
		t.Fatal("structurally equal generic instantiation steps should be equal")
	}
	if Equal(a, c) {
		t.Fatal("different generic instantiation steps should not be equal")
	}
}
