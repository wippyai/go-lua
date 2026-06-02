package callreturn

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyEffectTransforms_NoTransformReusesReturns(t *testing.T) {
	returns := []typ.Type{typ.String}
	got := ApplyEffectTransforms(EffectTransformInput{
		Callee:  typ.Func().Returns(typ.String).Build(),
		Returns: returns,
	})
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("ApplyEffectTransforms() = %#v, want string", got)
	}
	if &got[0] != &returns[0] {
		t.Fatal("ApplyEffectTransforms copied unchanged returns")
	}
}

func TestApplyEffectTransforms_NonFunctionReusesReturns(t *testing.T) {
	returns := []typ.Type{typ.String}
	got := ApplyEffectTransforms(EffectTransformInput{
		Callee:  typ.String,
		Returns: returns,
	})
	if &got[0] != &returns[0] {
		t.Fatal("non-function callee should reuse unchanged returns")
	}
}

func TestApplyEffectTransforms_ErrorReturnOptionalizesValueSlot(t *testing.T) {
	fn := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
		Build()

	got := ApplyEffectTransforms(EffectTransformInput{
		Callee:  fn,
		Returns: []typ.Type{typ.String, typ.NewOptional(typ.LuaError)},
	})
	want := typ.NewOptional(typ.String)
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("value return = %v, want %v", got[0], want)
	}
	if !typ.TypeEquals(got[1], typ.NewOptional(typ.LuaError)) {
		t.Fatalf("error return changed unexpectedly: %v", got[1])
	}
}

func TestApplyEffectTransforms_MethodReceiverParticipatesInRuntimeArgs(t *testing.T) {
	fn := typ.Func().
		Param("self", typ.String).
		Param("fallback", typ.Number).
		Returns(typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Return{
			ReturnIndex: 0,
			Transform:   effect.SameAs{Source: effect.ParamRef{Index: 0}},
		})).
		Build()

	got := ApplyEffectTransforms(EffectTransformInput{
		Callee:   fn,
		Args:     []typ.Type{typ.Number},
		Returns:  []typ.Type{typ.Any},
		Receiver: typ.String,
		IsMethod: true,
	})
	if !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("method return = %v, want receiver string", got[0])
	}
}

func TestApplyEffectTransforms_GenericFunctionBodyIsResolved(t *testing.T) {
	fn := typ.Func().
		Param("value", typ.String).
		Returns(typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Return{
			ReturnIndex: 0,
			Transform:   effect.SameAs{Source: effect.ParamRef{Index: 0}},
		})).
		Build()
	generic := &typ.Generic{Body: fn}

	got := ApplyEffectTransforms(EffectTransformInput{
		Callee:  generic,
		Args:    []typ.Type{typ.String},
		Returns: []typ.Type{typ.Any},
	})
	if !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("generic-body return = %v, want string", got[0])
	}
}
