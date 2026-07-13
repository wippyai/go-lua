package transformer

import (
	"fmt"
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
)

// prepareReturnedStructuralPredicates admits only complete source-authored
// short-circuit predicates returned by the function. It builds one immutable
// expression allow-set before row solving; point-local lowering cannot grow
// the set opportunistically.
func prepareReturnedStructuralPredicates(ctx *planCompileContext) error {
	if ctx == nil || ctx.plan == nil {
		return fmt.Errorf("missing plan")
	}
	var refinementRefs []factflow.ExprRef
	ctx.facts.ForEachExpressionRefinement(func(ref factflow.ExprRef, _ factflow.ExpressionRefinement) bool {
		refinementRefs = append(refinementRefs, ref)
		return true
	})
	sort.Slice(refinementRefs, func(i, j int) bool { return refinementRefs[i] < refinementRefs[j] })
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
		if len(refinementRefs) != 0 {
			return fmt.Errorf("expression refinement %d is outside a returned structural predicate", refinementRefs[0])
		}
		return nil
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	ctx.predicateExpressions = make(map[factflow.ExprRef]struct{})
	ctx.predicateRefinements = make(map[factflow.ExprRef]struct{})
	ctx.structuralPredicates = make(map[factflow.ExprRef]factflow.StructuralExpressionRegion)
	active := make(map[factflow.ExprRef]bool)
	for _, root := range roots {
		if err := validateReturnedPredicateExpr(ctx, root, active); err != nil {
			return fmt.Errorf("expression %d: %w", root, err)
		}
	}
	for _, ref := range refinementRefs {
		if _, certified := ctx.predicateRefinements[ref]; !certified {
			return fmt.Errorf("expression refinement %d is outside returned DAG", ref)
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
		if !region.RHSOnTrue() {
			return fmt.Errorf("expression %d logical and region has wrong RHS polarity", owner)
		}
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
		if op.Left() != source {
			return fmt.Errorf("expression %d structural branch condition is not its exact left operand", owner)
		}
		rhs := op.Right()
		if rhs.HasSourcePoint && !containsStructuralPoint(region.OwnedRHSPoints(), rhs.SourcePoint) {
			return fmt.Errorf("expression %d RHS producer is outside certified effect region", owner)
		}
		if !structuralRHSIsEffectFree(ctx.plan, region) {
			return fmt.Errorf("expression %d certified RHS region owns impure operations", owner)
		}
	}
	return nil
}

func structuralRHSIsEffectFree(plan *operationplan.Plan, region factflow.StructuralExpressionRegion) bool {
	if plan == nil {
		return false
	}
	for _, point := range region.OwnedRHSPoints() {
		cursor := plan.Cursor(point)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			pure, known := structuralPredicatePointKindPurity(cell.Kind())
			if !known || !pure {
				return false
			}
		}
	}
	return true
}

func structuralPredicatePointKindPurity(kind operationplan.Kind) (pure, known bool) {
	switch kind {
	case operationplan.BranchEdgeReachability, operationplan.BranchConditionSource,
		operationplan.BranchRefinement, operationplan.BranchPresenceRelation,
		operationplan.BranchPathRelation, operationplan.BranchPathEvidence,
		operationplan.BranchSufficientLiteralCase:
		return true, true
	case operationplan.RootAssignment, operationplan.PathAssignment, operationplan.PathStaticMemberWrite,
		operationplan.DynamicIndexWrite, operationplan.PathDescendantInvalidation, operationplan.CovariantExposure,
		operationplan.NoNormalReturn, operationplan.PathValuePresenceImplication, operationplan.ChannelSelect,
		operationplan.PostconditionRefinement, operationplan.PostconditionPathRelation, operationplan.CallResultValue,
		operationplan.ReturnPresenceRelation, operationplan.Return, operationplan.CallSite, operationplan.ObjectLiteral,
		operationplan.ExpressionValue, operationplan.ExpressionOperation, operationplan.ExpressionFunction,
		operationplan.ExpressionRefinement, operationplan.ExpressionPath, operationplan.DynamicIndexExpression,
		operationplan.ExpressionCondition:
		return false, true
	default:
		return false, false
	}
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
	identity, hasIdentity := op.Intrinsic()
	validUnary := ok && op.Kind() == factflow.ExpressionOperationUnary && hasIdentity && identity == intrinsic.LuaType
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
		if err := validateReturnedPredicateSource(ctx, source, active); err != nil {
			delete(active, ref)
			return err
		}
	}
	delete(active, ref)
	ctx.predicateExpressions[ref] = struct{}{}
	return nil
}

func validateReturnedPredicateSource(ctx *planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) error {
	if !source.Valid() || source.Expanded || source.OpenTail {
		return fmt.Errorf("non-scalar operand %#v", source)
	}
	if source.HasExpr {
		if refinement, refined := ctx.facts.ExpressionRefinement(source.ExprRef); refined {
			if refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
				return fmt.Errorf("operand %d refinement is not a runtime cast", source.ExprRef)
			}
			if active[source.ExprRef] {
				return fmt.Errorf("cyclic expression DAG")
			}
			if _, conflict := ctx.facts.ExpressionOperation(source.ExprRef); conflict {
				return fmt.Errorf("refinement and scalar operation share expression %d", source.ExprRef)
			}
			if _, conflict := ctx.facts.ObjectLiteralView(source.ExprRef); conflict {
				return fmt.Errorf("refinement and object literal share expression %d", source.ExprRef)
			}
			if _, conflict := ctx.facts.ExpressionFunction(source.ExprRef); conflict {
				return fmt.Errorf("refinement and function literal share expression %d", source.ExprRef)
			}
			if path, shared := ctx.facts.ExpressionPathRef(source.ExprRef); shared {
				inner := refinement.Source()
				sym, version, suffix, exact := pathaddr.ParseResolverPath(inner.PathKey)
				if inner.Kind != factflow.ValueSourcePath || !exact || sym == 0 || version != 0 || suffix != "" ||
					path.Symbol != sym || path.Version != version || len(path.Segments) != 0 {
					return fmt.Errorf("refinement and unrelated path share expression %d", source.ExprRef)
				}
			}
			if _, conflict := ctx.facts.DynamicIndexExpression(source.ExprRef); conflict {
				return fmt.Errorf("refinement and dynamic index share expression %d", source.ExprRef)
			}
			active[source.ExprRef] = true
			if err := validateReturnedPredicateSource(ctx, refinement.Source(), active); err != nil {
				delete(active, source.ExprRef)
				return fmt.Errorf("runtime cast %d source: %w", source.ExprRef, err)
			}
			delete(active, source.ExprRef)
			ctx.predicateRefinements[source.ExprRef] = struct{}{}
			return nil
		}
		if _, nested := ctx.facts.ExpressionOperation(source.ExprRef); nested {
			return validateReturnedPredicateExpr(ctx, source.ExprRef, active)
		}
		if _, conflict := ctx.facts.ObjectLiteralView(source.ExprRef); conflict {
			return fmt.Errorf("operand %d is an object literal", source.ExprRef)
		}
		if _, exact := exactSignatureExpressionTerm(*ctx, source); exact {
			return nil
		}
	}
	if _, err := exactCompilerSourceTermActive(*ctx, source, active); err != nil {
		return fmt.Errorf("operand %#v expressions=%v: %w", source, sortedExpressionTermRefs(ctx.expressions), err)
	}
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
