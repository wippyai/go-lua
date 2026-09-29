package erreffect

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIntersectionForwardingRequiresTruthySuccessProof(t *testing.T) {
	plain := typ.Func().Returns(typ.NewOptional(typ.Boolean), typ.NewOptional(typ.String)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).Build()
	truthy := typ.Func().Returns(typ.NewOptional(typ.Boolean), typ.NewOptional(typ.String)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1, ValueTruthy: true})).Build()
	if allCallableAlternativesHaveTruthyErrorReturn(typ.NewIntersection(plain, truthy), 0, 1) {
		t.Fatal("a forwarded overload can return false on success")
	}
}

func TestStaleReturnRelationsAreRemovedBeforeReproof(t *testing.T) {
	spec := contract.NewSpec().WithEffects(
		effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		effect.CorrelatedReturn{Indices: []int{0, 1}},
		effect.GuardedReturnType{GuardIndex: 0, TargetIndex: 1, TargetType: typ.String},
		effect.Iterator{Source: effect.ParamRef{Index: 0}, Kind: effect.IterateIndexed},
	)
	fn := typ.Func().Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).Spec(spec).Build()
	clean := withoutReturnRelations(fn)
	got := contract.ExtractSpec(clean)
	if got == nil || got.Effects.GetErrorReturn(0) != nil || got.Effects.GetIterator() == nil {
		t.Fatalf("expected only unrelated iterator effect to survive: %v", got)
	}
	for _, label := range got.Effects.Labels {
		switch label.(type) {
		case effect.CorrelatedReturn, effect.GuardedReturnType:
			t.Fatalf("stale return relation survived: %v", label)
		}
	}
}
