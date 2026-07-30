package summary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

// ReturnConditionParamRefinement records a parameter-relative value refinement
// selected by the boolean value of one return slot.
type ReturnConditionParamRefinement struct {
	ReturnIndex int
	ReturnValue bool
	Target      pathdom.Path
	Value       product.Value
}

type returnConditionParamRefinementKey struct {
	returnIndex int
	returnValue bool
	target      pathdom.PathKey
}

type returnConditionFactMap = factmap.Map[returnConditionParamRefinementKey, ReturnConditionParamRefinement, product.Value]

var returnConditionMaps registrycache.Cache[returnConditionFactMap]

// returnConditionMap is the canonical must (intersection) map lattice for
// return-condition refinements: refinements selected by the same return slot
// and target are narrowed by meet within a path and survive a join only when
// guaranteed on every path.
func returnConditionMap(reg *axis.Registry) returnConditionFactMap {
	return returnConditionMaps.GetFor(reg, newReturnConditionMap)
}

func newReturnConditionMap(reg *axis.Registry) returnConditionFactMap {
	return returnConditionFactMap{
		Key:   returnConditionKey,
		Value: func(r ReturnConditionParamRefinement) product.Value { return r.Value },
		WithValue: func(r ReturnConditionParamRefinement, v product.Value) ReturnConditionParamRefinement {
			r.Value = v
			return r
		},
		Less: returnConditionLess,
		Valid: func(r ReturnConditionParamRefinement) bool {
			return r.ReturnIndex >= 0 && r.Target.IsPlaceholder() && usefulReturnConditionValue(reg, r.Value)
		},
		CloneFact: func(r ReturnConditionParamRefinement) ReturnConditionParamRefinement {
			r.Target = r.Target.Clone()
			return r
		},
		Domain:    product.Domain(reg),
		Collide:   func(a, b product.Value) product.Value { return product.Meet(reg, a, b) },
		Intersect: true,
	}
}

func normalizeReturnConditionParamRefinements(reg *axis.Registry, in []ReturnConditionParamRefinement) []ReturnConditionParamRefinement {
	return returnConditionMap(reg).Normalize(in)
}

func cloneReturnConditionParamRefinements(in []ReturnConditionParamRefinement) []ReturnConditionParamRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnConditionParamRefinement, len(in))
	for i, refinement := range in {
		refinement.Target = refinement.Target.Clone()
		out[i] = refinement
	}
	return out
}

func returnConditionParamRefinementsEqual(reg *axis.Registry, a, b []ReturnConditionParamRefinement) bool {
	return returnConditionMap(reg).Equal(a, b)
}

func returnConditionParamRefinementsLessOrEq(reg *axis.Registry, a, b []ReturnConditionParamRefinement) bool {
	return returnConditionMap(reg).LessOrEq(a, b)
}

func joinReturnConditionParamRefinements(reg *axis.Registry, a, b []ReturnConditionParamRefinement) []ReturnConditionParamRefinement {
	return returnConditionMap(reg).Join(a, b)
}

func returnConditionKey(refinement ReturnConditionParamRefinement) returnConditionParamRefinementKey {
	return returnConditionParamRefinementKey{
		returnIndex: refinement.ReturnIndex,
		returnValue: refinement.ReturnValue,
		target:      refinement.Target.Key(),
	}
}

func returnConditionLess(a, b ReturnConditionParamRefinement) bool {
	left := returnConditionKey(a)
	right := returnConditionKey(b)
	if left.returnIndex != right.returnIndex {
		return left.returnIndex < right.returnIndex
	}
	if left.returnValue != right.returnValue {
		return !left.returnValue && right.returnValue
	}
	return left.target < right.target
}

func usefulReturnConditionValue(reg *axis.Registry, value product.Value) bool {
	return !product.Equal(reg, value, product.Bottom(reg)) && !product.Equal(reg, value, product.Top())
}
