package contract

import (
	"testing"

	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDeclaredErrorReturnConvention(t *testing.T) {
	fn := typ.Func().Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.String)).Build()
	declared := WithDeclaredErrorReturnConvention(fn)
	spec := ExtractSpec(declared)
	if spec == nil || spec.Effects.GetErrorReturn(0) == nil {
		t.Fatalf("declared optional value and trailing error need a return effect: %v", declared)
	}
	if ExtractSpec(fn) != nil {
		t.Fatal("declaration enrichment mutated the source function")
	}
	if WithDeclaredErrorReturnConvention(declared) != declared {
		t.Fatal("declaration enrichment must be idempotent")
	}
}

func TestDeclaredErrorReturnConvention_AmbiguousAndRequiredSlots(t *testing.T) {
	for _, fn := range []*typ.Function{
		typ.Func().Returns(typ.Boolean, typ.NewOptional(typ.String)).Build(),
		typ.Func().Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.String), typ.NewOptional(typ.String)).Build(),
		typ.Func().Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.Boolean)).Build(),
	} {
		if WithDeclaredErrorReturnConvention(fn) != fn {
			t.Fatalf("ambiguous or required slots must not gain a relation: %v", fn)
		}
	}
}

func TestDeclaredErrorReturnConvention_ExplicitEffectWins(t *testing.T) {
	spec := NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 1, ErrorIndex: 0})
	fn := typ.Func().Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).Spec(spec).Build()
	if WithDeclaredErrorReturnConvention(fn) != fn {
		t.Fatal("the declared effect must take precedence over the convention")
	}
}
