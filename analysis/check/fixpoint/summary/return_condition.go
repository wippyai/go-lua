package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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

func normalizeReturnConditionParamRefinements(
	reg *axis.Registry,
	in []ReturnConditionParamRefinement,
) []ReturnConditionParamRefinement {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[returnConditionParamRefinementKey]ReturnConditionParamRefinement, len(in))
	for _, refinement := range in {
		if refinement.ReturnIndex < 0 || !refinement.Target.IsPlaceholder() ||
			!usefulReturnConditionValue(reg, refinement.Value) {
			continue
		}
		refinement.Target = refinement.Target.Clone()
		key := returnConditionKey(refinement)
		if existing, ok := merged[key]; ok {
			existing.Value = product.Meet(reg, existing.Value, refinement.Value)
			merged[key] = existing
			continue
		}
		merged[key] = refinement
	}
	if len(merged) == 0 {
		return nil
	}
	out := make([]ReturnConditionParamRefinement, 0, len(merged))
	for _, refinement := range merged {
		if usefulReturnConditionValue(reg, refinement.Value) {
			out = append(out, refinement)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := returnConditionKey(out[i])
		right := returnConditionKey(out[j])
		if left.returnIndex != right.returnIndex {
			return left.returnIndex < right.returnIndex
		}
		if left.returnValue != right.returnValue {
			return !left.returnValue && right.returnValue
		}
		return left.target < right.target
	})
	return out
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

func returnConditionParamRefinementsEqual(
	reg *axis.Registry,
	a []ReturnConditionParamRefinement,
	b []ReturnConditionParamRefinement,
) bool {
	a = normalizeReturnConditionParamRefinements(reg, a)
	b = normalizeReturnConditionParamRefinements(reg, b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if returnConditionKey(a[i]) != returnConditionKey(b[i]) ||
			!product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func returnConditionParamRefinementsLessOrEq(
	reg *axis.Registry,
	a []ReturnConditionParamRefinement,
	b []ReturnConditionParamRefinement,
) bool {
	a = normalizeReturnConditionParamRefinements(reg, a)
	b = normalizeReturnConditionParamRefinements(reg, b)
	bByKey := make(map[returnConditionParamRefinementKey]product.Value, len(b))
	for _, refinement := range b {
		bByKey[returnConditionKey(refinement)] = refinement.Value
	}
	for _, left := range a {
		right, ok := bByKey[returnConditionKey(left)]
		if !ok {
			continue
		}
		if !product.LessOrEq(reg, left.Value, right) {
			return false
		}
	}
	aByKey := make(map[returnConditionParamRefinementKey]struct{}, len(a))
	for _, refinement := range a {
		aByKey[returnConditionKey(refinement)] = struct{}{}
	}
	for _, right := range b {
		if _, ok := aByKey[returnConditionKey(right)]; !ok {
			return false
		}
	}
	return true
}

func joinReturnConditionParamRefinements(
	reg *axis.Registry,
	a []ReturnConditionParamRefinement,
	b []ReturnConditionParamRefinement,
) []ReturnConditionParamRefinement {
	a = normalizeReturnConditionParamRefinements(reg, a)
	b = normalizeReturnConditionParamRefinements(reg, b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	bByKey := make(map[returnConditionParamRefinementKey]product.Value, len(b))
	for _, refinement := range b {
		bByKey[returnConditionKey(refinement)] = refinement.Value
	}
	out := make([]ReturnConditionParamRefinement, 0, min(len(a), len(b)))
	for _, left := range a {
		right, ok := bByKey[returnConditionKey(left)]
		if !ok {
			continue
		}
		left.Value = product.Join(reg, left.Value, right)
		out = append(out, left)
	}
	return normalizeReturnConditionParamRefinements(reg, out)
}

func returnConditionKey(refinement ReturnConditionParamRefinement) returnConditionParamRefinementKey {
	return returnConditionParamRefinementKey{
		returnIndex: refinement.ReturnIndex,
		returnValue: refinement.ReturnValue,
		target:      refinement.Target.Key(),
	}
}

func usefulReturnConditionValue(reg *axis.Registry, value product.Value) bool {
	return !product.Equal(reg, value, product.Bottom(reg)) && !product.Equal(reg, value, product.Top())
}
