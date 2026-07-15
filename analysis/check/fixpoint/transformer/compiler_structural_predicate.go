package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// preparePredicateExpressions admits complete source-authored scalar predicate
// DAGs owned by either a return or a CFG branch. It builds one immutable
// expression allow-set before row solving. Leaf availability is certified
// structurally here and resolved against the current row only during transfer;
// local declarations do not exist in the entry environment yet.
func preparePredicateExpressions(ctx *planCompileContext) error {
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
		point := cfg.Point(raw)
		if ret, ok := ctx.facts.Return(point); ok {
			for _, source := range ret.Sources() {
				if source.HasExpr {
					if refinement, refined := ctx.facts.ExpressionRefinement(source.ExprRef); refined && refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
						return fmt.Errorf("expression refinement %d is outside a certified predicate", source.ExprRef)
					}
				}
				if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
					continue
				}
				op, operation := ctx.facts.ExpressionOperation(source.ExprRef)
				_, structural := ctx.plan.StructuralExpressionRegion(source.ExprRef)
				if operation && op.Kind() == factflow.ExpressionOperationBinary && (op.Op() == "and" || op.Op() == "or") && structural {
					roots = append(roots, source.ExprRef)
				}
			}
		}
		if !ctx.graph.IsBranch(point) {
			continue
		}
		condition, conditionOK := ctx.facts.BranchConditionSource(point)
		if conditionOK && condition.Kind == factflow.ValueSourceExpression && condition.HasExpr {
			if refinement, refined := ctx.facts.ExpressionRefinement(condition.ExprRef); refined && refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
				return fmt.Errorf("expression refinement %d is outside a certified predicate", condition.ExprRef)
			}
			if _, operation := ctx.facts.ExpressionOperation(condition.ExprRef); operation {
				roots = append(roots, condition.ExprRef)
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	ctx.predicateExpressions = make(map[factflow.ExprRef]struct{})
	ctx.predicateRefinements = make(map[factflow.ExprRef]struct{})
	ctx.structuralPredicates = make(map[factflow.ExprRef]factflow.StructuralExpressionRegion)
	active := make(map[factflow.ExprRef]bool)
	// Runtime casts are pure symbolic value producers in their own right. They
	// are not required to sit under a Boolean return expression: assignments,
	// call arguments, and later predicates may all consume the same exact cast.
	// Certify the finite factflow producer graph once, then let row execution
	// resolve its lexical leaves at their actual dominance point.
	for _, ref := range refinementRefs {
		refinement, _ := ctx.facts.ExpressionRefinement(ref)
		if refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
			continue
		}
		if err := validateRuntimeRefinement(ctx, ref, active); err != nil {
			// Preserve the public fail-closed classification used by existing
			// callers. The important distinction is semantic: exact standalone
			// runtime casts now enter the certified producer graph, while a
			// malformed/conflicting producer still rejects the whole relation.
			return fmt.Errorf("expression refinement %d is outside a certified predicate", ref)
		}
	}
	for _, root := range roots {
		if err := validatePredicateExpr(ctx, root, active); err != nil {
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
		op, _ := ctx.facts.ExpressionOperation(owner)
		wantRHSOnTrue := op.Op() == "and"
		if region.RHSOnTrue() != wantRHSOnTrue {
			return fmt.Errorf("expression %d logical %s region has wrong RHS polarity", owner, op.Op())
		}
		source, ok := ctx.facts.BranchConditionSource(region.Branch())
		if !ok {
			return fmt.Errorf("expression %d structural branch has no condition source", owner)
		}
		if !samePredicateSource(ctx.facts, op.Left(), source) {
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

func samePredicateSource(facts factflow.Facts, left, right factflow.ValueSource) bool {
	if left == right {
		return true
	}
	leftPath, leftPathOK := predicateSourcePath(facts, left)
	rightPath, rightPathOK := predicateSourcePath(facts, right)
	if leftPathOK || rightPathOK {
		return leftPathOK && rightPathOK && leftPath.Equal(rightPath)
	}
	if left.HasExpr || right.HasExpr {
		return left.HasExpr && right.HasExpr && left.ExprRef == right.ExprRef
	}
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case factflow.ValueSourceCall:
		return left.HasCallPoint && right.HasCallPoint && left.CallPoint == right.CallPoint && left.ResultIndex == right.ResultIndex
	case factflow.ValueSourceLiteral:
		return left.LiteralKind == right.LiteralKind && left.Bool == right.Bool && left.Int == right.Int && left.Float == right.Float && left.String == right.String
	case factflow.ValueSourceNil, factflow.ValueSourceUnknown:
		return true
	default:
		return false
	}
}

func predicateSourcePath(facts factflow.Facts, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourcePath {
		sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey)
		if !ok || sym == 0 {
			return pathdom.Path{}, false
		}
		segments, ok := segment.InternFormattedSegments(suffix)
		if !ok {
			return pathdom.Path{}, false
		}
		return pathdom.Path{Symbol: sym, Version: version, Segments: segments}, true
	}
	if source.HasExpr {
		path, ok := facts.ExpressionPathRef(source.ExprRef)
		return path, ok
	}
	return pathdom.Path{}, false
}

func validateRuntimeRefinement(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool) error {
	if _, done := ctx.predicateRefinements[ref]; done {
		return nil
	}
	if ref == 0 || active[ref] {
		return fmt.Errorf("cyclic expression DAG")
	}
	refinement, ok := ctx.facts.ExpressionRefinement(ref)
	if !ok || refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
		return fmt.Errorf("is not a runtime cast")
	}
	if _, conflict := ctx.facts.ExpressionOperation(ref); conflict {
		return fmt.Errorf("shares identity with a scalar operation")
	}
	if _, conflict := ctx.facts.ObjectLiteralView(ref); conflict {
		return fmt.Errorf("shares identity with an object literal")
	}
	if _, conflict := ctx.facts.ExpressionFunction(ref); conflict {
		return fmt.Errorf("shares identity with a function literal")
	}
	if path, shared := ctx.facts.ExpressionPathRef(ref); shared {
		innerPath, exact := predicateSourcePath(ctx.facts, refinement.Source())
		if !exact || !path.Equal(innerPath) {
			return fmt.Errorf("shares identity with an unrelated path")
		}
	}
	if _, conflict := ctx.facts.DynamicIndexExpression(ref); conflict {
		return fmt.Errorf("shares identity with a dynamic index")
	}
	active[ref] = true
	if err := validatePredicateSource(ctx, refinement.Source(), active); err != nil {
		delete(active, ref)
		return fmt.Errorf("source: %w", err)
	}
	delete(active, ref)
	ctx.predicateRefinements[ref] = struct{}{}
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

func validatePredicateExpr(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool) error {
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
		(op.Op() == "==" || op.Op() == "~=" || op.Op() == "and" || op.Op() == "or")
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
	if validBinary && (op.Op() == "and" || op.Op() == "or") {
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
		if err := validatePredicateSource(ctx, source, active); err != nil {
			delete(active, ref)
			return err
		}
	}
	delete(active, ref)
	ctx.predicateExpressions[ref] = struct{}{}
	return nil
}

func validatePredicateSource(ctx *planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) error {
	if !source.Valid() || source.Expanded || source.OpenTail {
		return fmt.Errorf("non-scalar operand %#v", source)
	}
	if source.HasExpr {
		if _, refined := ctx.facts.ExpressionRefinement(source.ExprRef); refined {
			return validateRuntimeRefinement(ctx, source.ExprRef, active)
		}
		if _, nested := ctx.facts.ExpressionOperation(source.ExprRef); nested {
			return validatePredicateExpr(ctx, source.ExprRef, active)
		}
		if _, conflict := ctx.facts.ObjectLiteralView(source.ExprRef); conflict {
			return fmt.Errorf("operand %d is an object literal", source.ExprRef)
		}
		if _, exact := exactSignatureExpressionTerm(*ctx, source); exact {
			return nil
		}
	}
	if _, err := exactCompilerSourceTermActive(*ctx, source, active); err != nil {
		if predicateSourceBoundDuringRow(*ctx, source) {
			return nil
		}
		return fmt.Errorf("operand %#v expressions=%v: %w", source, sortedExpressionTermRefs(ctx.expressions), err)
	}
	return nil
}

// predicateSourceBoundDuringRow recognizes a canonical lexical root whose
// producer is an operation-plan assignment. Preparation proves the producer
// exists; rootAssignmentPlanHandler and exactCompilerSourceTerm perform the
// dominance, uniqueness, and exact-value checks in each executing row. This
// keeps staging honest without manufacturing an entry value for a local.
func predicateSourceBoundDuringRow(ctx planCompileContext, source factflow.ValueSource) bool {
	var target symbol.ID
	switch source.Kind {
	case factflow.ValueSourcePath:
		sym, version, _, ok := pathaddr.ParseResolverPath(source.PathKey)
		if !ok || sym == 0 || version != 0 {
			return false
		}
		target = sym
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return false
		}
		path, ok := ctx.facts.ExpressionPathRef(source.ExprRef)
		if !ok || path.Symbol == 0 || path.Version != 0 {
			return false
		}
		target = path.Symbol
	default:
		return false
	}
	for raw := 0; raw < ctx.plan.PointCount(); raw++ {
		assignment, ok := ctx.facts.RootAssignment(cfg.Point(raw))
		if ok && assignment.TargetSymbol() == target {
			return true
		}
	}
	return false
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
