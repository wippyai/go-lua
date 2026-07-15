package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r Relation) evaluateCallOutcomeBoundary(
	ctx context.Context,
	slot uint32,
	point cfg.Point,
	request EvaluatedRootRequest,
	evaluator *evaluatedTermEvaluator,
	guardWorlds map[Guard]evaluated.WorldSet,
) (evaluated.CallOutcomeBoundary, error) {
	site, ok := request.CallSurface.Site(point)
	if !ok {
		return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d is absent from sealed call surface", point)
	}
	targetBody, ok := site.Target.LexicalBody()
	if !ok {
		return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d is not an exact lexical call", point)
	}
	roles, err := sealedCallOutcomeRoles()
	if err != nil {
		return evaluated.CallOutcomeBoundary{}, err
	}
	out := evaluated.CallOutcomeBoundary{Slot: slot, Point: point, Owner: request.Identity.Body, Target: targetBody}
	matched := false
	for index, template := range r.annotations.calls {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.CallOutcomeBoundary{}, err
			}
		}
		if template.point != point {
			continue
		}
		matched = true
		if !template.valid(r.arena, r.shape) || template.owner != request.Identity.Body {
			return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d has malformed owner-local template", point)
		}
		if out.Occurrence.Valid() && out.Occurrence != template.occurrence {
			return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d has conflicting occurrences", point)
		}
		out.Occurrence = template.occurrence
		worlds, present := guardWorlds[template.guard]
		if !present {
			return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d has no shared guard world", point)
		}
		if worlds.Root == evaluated.DecisionFalse {
			continue
		}
		targetRelation, ok := request.Relations.Lookup(template.target.Cell)
		if !ok || targetRelation.shape != template.target.Shape || targetRelation.arena == nil || targetRelation.arena.reg != r.arena.reg || targetRelation.contextual != "" {
			return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d has no exact final target relation", point)
		}
		if err := targetRelation.validateCallbackFreeEvaluatedSummaryTerms(ctx); err != nil {
			return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d target is not neutral: %w", point, err)
		}
		values := make([]product.Value, len(template.values))
		paths := make([]pathdom.Path, len(template.paths))
		for valueIndex, term := range template.values {
			value, err := evaluator.value(term)
			if err != nil {
				return evaluated.CallOutcomeBoundary{}, err
			}
			values[valueIndex] = value
			if template.paths[valueIndex] == 0 {
				continue
			}
			path, ok := r.arena.specializeOptionalObserverPath(template.paths[valueIndex], evaluator.cursor)
			if !ok {
				return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d path %d cannot be specialized", point, valueIndex)
			}
			paths[valueIndex] = path.Clone()
		}
		cursor, err := NewBindingCursor(template.target.Shape, values, paths)
		if err != nil {
			return evaluated.CallOutcomeBoundary{}, err
		}
		targetEvaluator, err := newEvaluatedTermEvaluator(ctx, targetRelation.arena, cursor)
		if err != nil {
			return evaluated.CallOutcomeBoundary{}, err
		}
		specialized, err := targetRelation.evaluateOwnerSummary(ctx, targetEvaluator)
		if err != nil {
			return evaluated.CallOutcomeBoundary{}, err
		}
		fragment := evaluated.CallOutcomeFragment{
			Worlds: worlds, Summary: specialized,
			Results: make([]evaluated.IndexedValue, len(specialized.Returns)),
			Roles:   append([]evaluated.CallOutcomeRole(nil), roles...),
		}
		for resultIndex, value := range specialized.Returns {
			fragment.Results[resultIndex] = evaluated.IndexedValue{Index: uint32(resultIndex), Value: value}
		}
		out.Fragments = append(out.Fragments, fragment)
	}
	if !matched || !out.Occurrence.Valid() {
		return evaluated.CallOutcomeBoundary{}, fmt.Errorf("transformer: call-outcome point %d has no owner-local template", point)
	}
	return out, nil
}
