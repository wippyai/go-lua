package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect/typeprojection"
	"github.com/wippyai/go-lua/types/typ"
)

// VariantCaseFieldProjectionValue is a proven field value for a selected variant
// origin case.
type VariantCaseFieldProjectionValue struct {
	Path  constraint.Path
	Value product.AbstractValue
}

// VariantCaseFieldProjectionValues selects field projections justified by fact.
func VariantCaseFieldProjectionValues(
	state *PointState,
	fact constraint.Condition,
	projections []VariantCaseFieldProjection,
) []VariantCaseFieldProjectionValue {
	if state == nil || len(projections) == 0 || fact.NumDisjuncts() == 0 {
		return nil
	}
	var joined map[constraint.PathKey]VariantCaseFieldProjectionValue
	for i := 0; i < fact.NumDisjuncts(); i++ {
		local := variantCaseFieldProjectionValuesForDisjunct(state, fact.DisjunctConstraints(i), projections)
		if i == 0 {
			joined = local
			continue
		}
		for key, current := range joined {
			next, ok := local[key]
			if !ok {
				delete(joined, key)
				continue
			}
			current.Value = product.Domain.Join(current.Value, next.Value)
			joined[key] = current
		}
	}
	if len(joined) == 0 {
		return nil
	}
	out := make([]VariantCaseFieldProjectionValue, 0, len(joined))
	for _, entry := range joined {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path.String() < out[j].Path.String()
	})
	return out
}

// ApplyVariantCaseFieldProjections materializes payload-field projections proven
// by the current variant-origin condition into the point-state static member
// domain. This keeps the selected-variant projection law inside flow, next to
// the projection reader and static-member writer it composes.
func ApplyVariantCaseFieldProjections(out *PointState, fact constraint.Condition, projections []VariantCaseFieldProjection) bool {
	if out == nil {
		return false
	}
	return ApplyStaticMemberReductions(out, VariantCaseFieldProjectionReductions(out, fact, projections))
}

// VariantCaseFieldProjectionReductions selects static-member reductions
// justified by variant-origin condition facts.
func VariantCaseFieldProjectionReductions(
	state *PointState,
	fact constraint.Condition,
	projections []VariantCaseFieldProjection,
) []StaticMemberReduction {
	values := VariantCaseFieldProjectionValues(state, fact, projections)
	if len(values) == 0 {
		return nil
	}
	out := make([]StaticMemberReduction, 0, len(values))
	for _, value := range values {
		out = append(out, StaticMemberReduction{Path: value.Path, Value: value.Value})
	}
	return out
}

func variantCaseFieldProjectionValuesForDisjunct(
	state *PointState,
	constraints []constraint.Constraint,
	projections []VariantCaseFieldProjection,
) map[constraint.PathKey]VariantCaseFieldProjectionValue {
	values := make(map[constraint.PathKey]VariantCaseFieldProjectionValue)
	for _, c := range constraints {
		eq, ok := c.(constraint.VariantCaseEquals)
		if !ok {
			continue
		}
		for _, projection := range projections {
			if projection.OriginFamily != eq.OriginFamily ||
				projection.CaseIndex != eq.CaseIndex ||
				!projection.Target.Equal(eq.Target) ||
				projection.Field == "" {
				continue
			}
			av, ok := variantCaseFieldProjectionValue(state, projection)
			if !ok {
				continue
			}
			path := projection.Target.Field(projection.Field)
			key := StablePathKey(path)
			if key == "" {
				continue
			}
			entry, exists := values[key]
			if exists {
				entry.Value = product.Domain.Join(entry.Value, av)
			} else {
				entry = VariantCaseFieldProjectionValue{Path: path, Value: av}
			}
			values[key] = entry
		}
	}
	return values
}

func variantCaseFieldProjectionValue(state *PointState, projection VariantCaseFieldProjection) (product.AbstractValue, bool) {
	if projection.Source.IsEmpty() {
		return product.AbstractValue{}, false
	}
	sourceType, ok := PointFactsOfBorrowed(state).PathType(projection.Source)
	if !ok || typ.IsAbsentOrUnknown(sourceType) || typ.ContainsAny(sourceType) {
		return product.AbstractValue{}, false
	}
	projected := typeprojection.ApplySteps(sourceType, projection.SourceSteps)
	if typ.IsAbsentOrUnknown(projected) || typ.ContainsAny(projected) {
		return product.AbstractValue{}, false
	}
	return product.FromType(projected), true
}
