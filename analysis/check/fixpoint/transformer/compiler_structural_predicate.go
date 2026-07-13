package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// prepareReturnedStructuralPredicates admits only complete source-authored
// short-circuit predicates returned by the function. It builds one immutable
// expression allow-set before row solving; point-local lowering cannot grow
// the set opportunistically.
func prepareReturnedStructuralPredicates(ctx *planCompileContext) error {
	if ctx == nil || ctx.plan == nil {
		return fmt.Errorf("missing plan")
	}
	var roots []factflow.ExprRef
	for raw := 0; raw < ctx.plan.PointCount(); raw++ {
		ret, ok := ctx.facts.Return(cfg.Point(raw))
		if !ok {
			continue
		}
		for _, source := range ret.Sources() {
			if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
				continue
			}
			op, operation := ctx.facts.ExpressionOperation(source.ExprRef)
			_, structural := ctx.plan.StructuralExpressionRegion(source.ExprRef)
			if operation && op.Kind() == factflow.ExpressionOperationBinary && op.Op() == "and" && structural {
				roots = append(roots, source.ExprRef)
			}
		}
	}
	if len(roots) == 0 {
		return nil
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	ctx.predicateExpressions = make(map[factflow.ExprRef]struct{})
	ctx.structuralPredicates = make(map[factflow.ExprRef]factflow.StructuralExpressionRegion)
	active := make(map[factflow.ExprRef]bool)
	for _, root := range roots {
		if err := validateReturnedPredicateExpr(ctx, root, active); err != nil {
			return fmt.Errorf("expression %d: %w", root, err)
		}
	}
	// Graph/source ownership is checked after the whole DAG is known. Every
	// structural branch must be selected by an expression in this same DAG and
	// the conditional RHS producer must be inside the certified region.
	owners := make([]factflow.ExprRef, 0, len(ctx.structuralPredicates))
	for owner := range ctx.structuralPredicates {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i] < owners[j] })
	for _, owner := range owners {
		region := ctx.structuralPredicates[owner]
		source, ok := ctx.facts.BranchConditionSource(region.Branch())
		if !ok {
			return fmt.Errorf("expression %d structural branch has no condition source", owner)
		}
		if !source.HasExpr {
			return fmt.Errorf("expression %d structural branch condition has no expression identity", owner)
		}
		if _, owned := ctx.predicateExpressions[source.ExprRef]; !owned {
			return fmt.Errorf("expression %d structural branch condition %d is outside returned DAG", owner, source.ExprRef)
		}
		op, _ := ctx.facts.ExpressionOperation(owner)
		rhs := op.Right()
		if rhs.HasSourcePoint && !containsStructuralPoint(region.OwnedRHSPoints(), rhs.SourcePoint) {
			return fmt.Errorf("expression %d RHS producer is outside certified effect region", owner)
		}
		if !rhs.HasSourcePoint && !structuralRHSIsEffectFree(ctx.plan, region) {
			return fmt.Errorf("expression %d RHS has no producer point and certified region owns effects", owner)
		}
	}
	return nil
}

func structuralRHSIsEffectFree(plan *operationplan.Plan, region factflow.StructuralExpressionRegion) bool {
	if plan == nil {
		return false
	}
	catalog := DefaultEffectCatalog()
	for _, point := range region.OwnedRHSPoints() {
		var active []operationplan.Kind
		cursor := plan.Cursor(point)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			active = append(active, cell.Kind())
		}
		_, admitted, err := catalog.AdmitPoint(active)
		if err != nil || admitted {
			return false
		}
	}
	return true
}

func validateReturnedPredicateExpr(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool) error {
	if ref == 0 {
		return fmt.Errorf("zero expression identity")
	}
	if _, done := ctx.predicateExpressions[ref]; done {
		return nil
	}
	if active[ref] {
		return fmt.Errorf("cyclic expression DAG")
	}
	op, ok := ctx.facts.ExpressionOperation(ref)
	validUnary := ok && op.Kind() == factflow.ExpressionOperationUnary && op.Op() == "lua_type"
	validBinary := ok && op.Kind() == factflow.ExpressionOperationBinary &&
		(op.Op() == "==" || op.Op() == "~=" || op.Op() == "and")
	if !validUnary && !validBinary {
		return fmt.Errorf("unsupported predicate producer")
	}
	if _, conflict := ctx.facts.ObjectLiteralView(ref); conflict {
		return fmt.Errorf("object literal and scalar operation share an expression identity")
	}
	if _, conflict := ctx.facts.ExpressionFunction(ref); conflict {
		return fmt.Errorf("function literal and scalar operation share an expression identity")
	}
	if _, conflict := ctx.facts.ExpressionRefinement(ref); conflict {
		return fmt.Errorf("refinement and scalar operation share an expression identity")
	}
	if _, conflict := ctx.facts.ExpressionPathRef(ref); conflict {
		return fmt.Errorf("path and scalar operation share an expression identity")
	}
	if _, conflict := ctx.facts.DynamicIndexExpression(ref); conflict {
		return fmt.Errorf("dynamic index and scalar operation share an expression identity")
	}
	if validBinary && op.Op() == "and" {
		region, exact := ctx.plan.StructuralExpressionRegion(ref)
		if !exact {
			return fmt.Errorf("logical expression has no exact structural region")
		}
		ctx.structuralPredicates[ref] = region
	}
	active[ref] = true
	sources := []factflow.ValueSource{op.Left()}
	if validBinary {
		sources = append(sources, op.Right())
	}
	for _, source := range sources {
		if !source.Valid() || source.Expanded || source.OpenTail {
			delete(active, ref)
			return fmt.Errorf("non-scalar operand %#v", source)
		}
		if source.HasExpr {
			if _, nested := ctx.facts.ExpressionOperation(source.ExprRef); nested {
				if err := validateReturnedPredicateExpr(ctx, source.ExprRef, active); err != nil {
					delete(active, ref)
					return err
				}
				continue
			}
			if _, conflict := ctx.facts.ObjectLiteralView(source.ExprRef); conflict {
				delete(active, ref)
				return fmt.Errorf("operand %d is an object literal", source.ExprRef)
			}
			if _, exact := exactSignatureExpressionTerm(*ctx, source); exact {
				continue
			}
		}
		if _, err := exactCompilerSourceTermActive(*ctx, source, active); err != nil {
			delete(active, ref)
			return fmt.Errorf("operand %#v expressions=%v: %w", source, sortedExpressionTermRefs(ctx.expressions), err)
		}
	}
	delete(active, ref)
	ctx.predicateExpressions[ref] = struct{}{}
	return nil
}

func sortedExpressionTermRefs(expressions map[factflow.ExprRef][]ValueTerm) []factflow.ExprRef {
	out := make([]factflow.ExprRef, 0, len(expressions))
	for ref := range expressions {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func containsStructuralPoint(points []cfg.Point, target cfg.Point) bool {
	index := sort.Search(len(points), func(i int) bool { return points[i] >= target })
	return index < len(points) && points[index] == target
}
