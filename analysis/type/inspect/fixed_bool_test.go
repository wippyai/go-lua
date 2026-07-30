package inspect

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestBoolFixedPointIsDepthIndependentAndCycleExact(t *testing.T) {
	equation := func(current typ.Type) BoolEquation {
		switch value := current.(type) {
		case *typ.Optional:
			return BoolEquation{Join: BoolAll, Inputs: []typ.Type{value.Inner}}
		case *typ.Recursive:
			return BoolEquation{Join: BoolAny, Inputs: []typ.Type{value.Body}}
		case *typ.Union:
			return BoolEquation{Join: BoolAny, Inputs: value.Members}
		default:
			return BoolEquation{Join: BoolConstant, Constant: current == typ.String}
		}
	}

	var deep typ.Type = typ.String
	for range 256 {
		deep = &typ.Optional{Inner: deep}
	}
	if !LeastBoolFixedPoint(deep, equation) || !GreatestBoolFixedPoint(deep, equation) {
		t.Fatal("a 256-wrapper path must have the same solution as its shallow leaf")
	}

	unproductive := typ.NewRecursivePlaceholder("Unproductive")
	unproductive.SetBody(unproductive)
	if LeastBoolFixedPoint(unproductive, equation) {
		t.Fatal("least fixed point of X = X must be false")
	}
	if !GreatestBoolFixedPoint(unproductive, equation) {
		t.Fatal("greatest fixed point of X = X must be true")
	}

	productive := typ.NewRecursivePlaceholder("Productive")
	productive.SetBody(&typ.Union{Members: []typ.Type{productive, typ.String}})
	if !LeastBoolFixedPoint(productive, equation) {
		t.Fatal("least fixed point must discover the productive String branch")
	}
}
