package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

// ReturnConditionSlotRefinement records a return-slot value refinement selected
// by the Lua truthiness of another return slot.
type ReturnConditionSlotRefinement struct {
	ReturnIndex int
	ReturnValue bool
	TargetIndex int
	Value       product.Value
}

type returnConditionSlotRefinementKey struct {
	returnIndex int
	returnValue bool
	targetIndex int
}

type returnConditionSlotFactMap = factmap.Map[returnConditionSlotRefinementKey, ReturnConditionSlotRefinement, product.Value]

var returnConditionSlotMaps registrycache.Cache[returnConditionSlotFactMap]

func returnConditionSlotMap(reg *axis.Registry) returnConditionSlotFactMap {
	return returnConditionSlotMaps.GetFor(reg, newReturnConditionSlotMap)
}

func newReturnConditionSlotMap(reg *axis.Registry) returnConditionSlotFactMap {
	return returnConditionSlotFactMap{
		Key:   returnConditionSlotKey,
		Value: func(r ReturnConditionSlotRefinement) product.Value { return r.Value },
		WithValue: func(r ReturnConditionSlotRefinement, v product.Value) ReturnConditionSlotRefinement {
			r.Value = v
			return r
		},
		Less: returnConditionSlotLess,
		Valid: func(r ReturnConditionSlotRefinement) bool {
			return r.ReturnIndex >= 0 &&
				r.TargetIndex >= 0 &&
				r.ReturnIndex != r.TargetIndex &&
				usefulReturnConditionValue(reg, r.Value)
		},
		CloneFact: func(r ReturnConditionSlotRefinement) ReturnConditionSlotRefinement { return r },
		Domain:    product.Domain(reg),
		Collide:   func(a, b product.Value) product.Value { return product.Meet(reg, a, b) },
		Intersect: true,
	}
}

func normalizeReturnConditionSlotRefinements(reg *axis.Registry, in []ReturnConditionSlotRefinement) []ReturnConditionSlotRefinement {
	return returnConditionSlotMap(reg).Normalize(in)
}

func cloneReturnConditionSlotRefinements(in []ReturnConditionSlotRefinement) []ReturnConditionSlotRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnConditionSlotRefinement, len(in))
	copy(out, in)
	return out
}

func returnConditionSlotRefinementsEqual(reg *axis.Registry, a, b []ReturnConditionSlotRefinement) bool {
	return returnConditionSlotMap(reg).Equal(a, b)
}

func returnConditionSlotRefinementsLessOrEq(reg *axis.Registry, a, b []ReturnConditionSlotRefinement) bool {
	return returnConditionSlotMap(reg).LessOrEq(a, b)
}

func joinReturnConditionSlotRefinements(reg *axis.Registry, a, b []ReturnConditionSlotRefinement) []ReturnConditionSlotRefinement {
	return returnConditionSlotMap(reg).Join(a, b)
}

func returnConditionSlotKey(refinement ReturnConditionSlotRefinement) returnConditionSlotRefinementKey {
	return returnConditionSlotRefinementKey{
		returnIndex: refinement.ReturnIndex,
		returnValue: refinement.ReturnValue,
		targetIndex: refinement.TargetIndex,
	}
}

func returnConditionSlotLess(a, b ReturnConditionSlotRefinement) bool {
	left := returnConditionSlotKey(a)
	right := returnConditionSlotKey(b)
	if left.returnIndex != right.returnIndex {
		return left.returnIndex < right.returnIndex
	}
	if left.returnValue != right.returnValue {
		return !left.returnValue && right.returnValue
	}
	return left.targetIndex < right.targetIndex
}
