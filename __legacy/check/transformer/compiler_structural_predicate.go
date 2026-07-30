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

// prepareCertifiedScalarExpressions admits complete source-authored scalar
// refinement and predicate DAGs. It builds one immutable expression allow-set
// before row solving. Leaf availability is certified structurally here and
// resolved against the current row only during transfer; local declarations
// do not exist in the entry environment yet.
func prepareCertifiedScalarExpressions(ctx *planCompileContext) error {
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
	// Unary and true-comparison scalar operations are value producers wherever
	// their result is consumed, not only when they happen to be a function
	// return or CFG branch condition: exactCompilerSourceTermActive in
	// compiler.go gates lowering on ctx.predicateExpressions for exactly these
	// two shapes (the lua_type/pure-unary branch, and the exactScalarComparison
	// branch for "==","~=","<","<=",">",">="), so certifying the complete
	// finite producer set here keeps certification aligned with what lowering
	// actually requires. and/or is deliberately excluded: ScalarBinaryValue
	// lowers "and"/"or" unconditionally regardless of certification, so their
	// only certification need is the return/branch-consumed case handled
	// below, which additionally proves the exact structural region a bare
	// and/or fact is not otherwise required to have.
	ctx.facts.ForEachExpressionOperation(func(ref factflow.ExprRef, operation factflow.ExpressionOperation) bool {
		identity, intrinsicOperation := operation.Intrinsic()
		supportedUnary := operation.Kind() == factflow.ExpressionOperationUnary &&
			(intrinsicOperation && identity == intrinsic.LuaType || !intrinsicOperation && isPureUnaryOperator(operation.Op()))
		supportedBinary := operation.Kind() == factflow.ExpressionOperationBinary &&
			isExactScalarPredicateOperator(operation.Op())
		if supportedUnary || supportedBinary {
			roots = append(roots, ref)
		}
		return true
	})
	for raw := 0; raw < ctx.plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		if ret, ok := ctx.facts.Return(point); ok {
			for _, source := range ret.Sources() {
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
			if _, operation := ctx.facts.ExpressionOperation(condition.ExprRef); operation {
				roots = append(roots, condition.ExprRef)
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	if len(roots) > 1 {
		unique := roots[:1]
		for _, ref := range roots[1:] {
			if ref != unique[len(unique)-1] {
				unique = append(unique, ref)
			}
		}
		roots = unique
	}
	ctx.predicateExpressions = make(map[factflow.ExprRef]struct{})
	ctx.expressionRefinements = make(map[factflow.ExprRef]struct{})
	ctx.structuralPredicates = make(map[factflow.ExprRef]factflow.StructuralExpressionRegion)
	active := make(map[factflow.ExprRef]bool)
	// Source-authored refinements are exact scalar producers in this dialect,
	// including assertions, declared claims, and runtime validations used by
	// assignments, call arguments, and returns. Certify each producer by its own
	// finite source graph; consumers still require the matching ExprRef, so an
	// unrelated refinement cannot lend authority to another expression.
	for _, ref := range refinementRefs {
		if err := validateExpressionRefinement(ctx, ref, active); err != nil {
			return fmt.Errorf("expression refinement %d is not a certified scalar expression", ref)
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
		condition, ok := ctx.facts.BranchCondition(region.Branch())
		if !ok {
			return fmt.Errorf("expression %d structural branch has no condition source", owner)
		}
		if !exactStructuralBranchCondition(ctx.facts, op.Left(), condition) {
			return fmt.Errorf("expression %d structural branch condition is not its exact left operand", owner)
		}
		rhs := op.Right()
		if rhs.HasSourcePoint && !containsStructuralPoint(region.OwnedRHSPoints(), rhs.SourcePoint) {
			return fmt.Errorf("expression %d RHS producer is outside certified effect region", owner)
		}
		// OwnedRHSPoints is execution ownership, not the scalar term: it is
		// deliberately complete enough to include conditional calls and writes.
		// validatePredicateExpr has already proved the frozen scalar DAG, while
		// predicateSourceBoundDuringRow proves a call result is a canonical leaf.
		// The operation-plan cell remains the sole executor of the call/effect;
		// specializing the scalar term only reads its resulting coordinate.
		//
		// Certifying an owner with an impure RHS (a write or a call) no longer
		// needs a standalone purity gate over the region's owned cell kinds. The
		// two consumers of a certified and/or's value are structurally incapable
		// of re-executing, duplicating, or reordering that RHS:
		//
		//  - The executable value is bound exactly once, unconditionally of
		//    certification, by bindStructuralExpressionTerms/
		//    structuralExpressionWrites (structural_freezer.go): a single phi
		//    write into the expression's ExpressionValue cell at region.Join(),
		//    sourced from op.Left() or op.Right() depending on which CFG edge
		//    control actually took. exactCompilerSourceTermActive's and/or case
		//    (compiler.go) reads that one bound cell by reference; it never
		//    reconstructs op.Right() from its operands.
		//  - exactPredicateProofTermActive is the only consumer that does
		//    reconstruct the RHS symbolically, and it exists solely to prove
		//    branch path evidence (validateRepresentedBranchEvidence); its own
		//    doc comment records that the result is never installed as a
		//    WorldProgram read, so building it has no runtime effect to
		//    duplicate.
		//
		// This is why a bare effectful RHS call remains certifiable as a
		// predicate leaf (TestPreparePredicateExpressions/"an effectful RHS call
		// remains CFG-owned while its result is a predicate leaf"): the call
		// cell it owns executes exactly once regardless of certification, which
		// that test proves directly by counting executable call cells.
		//
		// This was not always true. Before the phi-cell split, and/or's
		// executable value was itself reconstructed by exactCompilerSourceTermActive
		// from op.Left()/op.Right() (see git history at a254bab86), so an impure
		// RHS producer really could be re-read from an arbitrary consuming
		// context; structuralRHSIsEffectFree/structuralPredicatePointKindPurity
		// were the correct gate for that architecture. They are dead weight in
		// this one.
	}
	return nil
}

// exactStructuralBranchCondition recognizes the two lossless lowerings of a
// logical owner's left operand. Most guards retain the operand itself and use
// the true edge for truthiness. WIR normalizes `not x` to a falsy x check;
// that is equally exact only when the branch polarity records the inversion.
// No other unary form, source rewrite, or polarity mismatch is admitted.
func exactStructuralBranchCondition(facts factflow.Facts, left factflow.ValueSource, condition factflow.BranchCondition) bool {
	if condition.TruthyOnTrueEdge() && samePredicateSource(facts, left, condition.Source()) {
		return true
	}
	if condition.TruthyOnTrueEdge() || !left.HasExpr {
		return false
	}
	operation, ok := facts.ExpressionOperation(left.ExprRef)
	return ok && operation.Kind() == factflow.ExpressionOperationUnary && operation.Op() == "not" &&
		samePredicateSource(facts, operation.Left(), condition.Source())
}

func samePredicateSource(facts factflow.Facts, left, right factflow.ValueSource) bool {
	return samePredicateSourceActive(facts, left, right, make(map[predicateExpressionPair]bool))
}

type predicateExpressionPair struct {
	left  factflow.ExprRef
	right factflow.ExprRef
}

func samePredicateSourceActive(facts factflow.Facts, left, right factflow.ValueSource, active map[predicateExpressionPair]bool) bool {
	if left == right {
		return true
	}
	leftPath, leftPathOK := predicateSourcePath(facts, left)
	rightPath, rightPathOK := predicateSourcePath(facts, right)
	if leftPathOK || rightPathOK {
		return leftPathOK && rightPathOK && leftPath.Equal(rightPath)
	}
	if left.HasExpr || right.HasExpr {
		if !left.HasExpr || !right.HasExpr {
			return false
		}
		if left.ExprRef == right.ExprRef {
			return true
		}
		pair := predicateExpressionPair{left: left.ExprRef, right: right.ExprRef}
		if active[pair] {
			return false
		}
		leftDynamic, leftHasDynamic := facts.DynamicIndexExpression(left.ExprRef)
		rightDynamic, rightHasDynamic := facts.DynamicIndexExpression(right.ExprRef)
		if leftHasDynamic || rightHasDynamic {
			if !leftHasDynamic || !rightHasDynamic {
				return false
			}
			active[pair] = true
			equal := sameDynamicIndexSourceActive(facts, leftDynamic, rightDynamic, active)
			delete(active, pair)
			return equal
		}
		leftOperation, leftExact := facts.ExpressionOperation(left.ExprRef)
		rightOperation, rightExact := facts.ExpressionOperation(right.ExprRef)
		if !leftExact || !rightExact || leftOperation.Kind() != rightOperation.Kind() || leftOperation.Op() != rightOperation.Op() {
			return false
		}
		leftIntrinsic, leftHasIntrinsic := leftOperation.Intrinsic()
		rightIntrinsic, rightHasIntrinsic := rightOperation.Intrinsic()
		if leftHasIntrinsic != rightHasIntrinsic || leftHasIntrinsic && leftIntrinsic != rightIntrinsic {
			return false
		}
		active[pair] = true
		equal := samePredicateSourceActive(facts, leftOperation.Left(), rightOperation.Left(), active)
		if equal && leftOperation.Kind() == factflow.ExpressionOperationBinary {
			equal = samePredicateSourceActive(facts, leftOperation.Right(), rightOperation.Right(), active)
		}
		delete(active, pair)
		return equal
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

// sameDynamicIndexSourceActive recognizes two DynamicIndexExpression facts as
// the same table[key] read when they denote the same table and the same key,
// recursively. A static table path is the authoritative identity when either
// side carries one (per DynamicIndexExpression's own contract); pathdom.Path
// equality is symbol-and-version exact, so a reassignment of the table
// binding between the two facts already fails this comparison. When neither
// side has a static table path (an unnameable table producer, e.g. a cast
// result), the table producers themselves must recursively resolve to the
// same source through samePredicateSourceActive. This never certifies more
// than the existing per-fact identity already licenses: it is the same
// equivalence samePredicateSourceActive already grants to structurally
// identical ExpressionOperation trees, extended to the one fact kind
// (dynamic-index reads) that has neither a static ExpressionPath nor an
// ExpressionOperation of its own.
func sameDynamicIndexSourceActive(facts factflow.Facts, left, right factflow.DynamicIndexExpression, active map[predicateExpressionPair]bool) bool {
	leftPath, rightPath := left.TablePathRef(), right.TablePathRef()
	if !leftPath.IsEmpty() || !rightPath.IsEmpty() {
		if leftPath.IsEmpty() || rightPath.IsEmpty() || !leftPath.Equal(rightPath) {
			return false
		}
	} else {
		leftTableSource, leftHasTableSource := left.TableSource()
		rightTableSource, rightHasTableSource := right.TableSource()
		if !leftHasTableSource || !rightHasTableSource || !samePredicateSourceActive(facts, leftTableSource, rightTableSource, active) {
			return false
		}
	}
	return samePredicateSourceActive(facts, left.KeySource(), right.KeySource(), active)
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

func validateExpressionRefinement(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool) error {
	if _, done := ctx.expressionRefinements[ref]; done {
		return nil
	}
	if ref == 0 || active[ref] {
		return fmt.Errorf("cyclic expression DAG")
	}
	refinement, ok := ctx.facts.ExpressionRefinement(ref)
	if !ok {
		return fmt.Errorf("is not an expression refinement")
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
		resultPath, owned := refinement.ResultPathRef()
		if !owned || !path.Equal(resultPath) {
			return fmt.Errorf("shares identity with an unrelated path")
		}
	}
	if _, conflict := ctx.facts.DynamicIndexExpression(ref); conflict {
		return fmt.Errorf("shares identity with a dynamic index")
	}
	active[ref] = true
	if err := validatePredicateSource(ctx, refinement.Source(), active); err != nil &&
		(refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation || !runtimeValidationOwnsUnresolvedScalarSource(refinement.Source())) {
		delete(active, ref)
		return fmt.Errorf("source: %w", err)
	}
	delete(active, ref)
	ctx.expressionRefinements[ref] = struct{}{}
	return nil
}

// runtimeValidationOwnsUnresolvedScalarSource recognizes the one contextual
// producer for which a runtime validation is itself sufficient normal-path
// value authority. A closed indexed call-result coordinate denotes exactly
// one Lua value even when the surrounding value list expands or the call has
// no context-independent symbolic result. The
// validation may therefore use the sourcevalue package's established Bottom
// pre-check spelling and publish only its validated contract.
//
// This is deliberately not general predicate admission: an open tail is not
// a closed coordinate, and an adjusted call can select only slot zero. Paths,
// environments, resources, heaps, unknown sources, and arbitrary expression
// producers still require their ordinary exact term.
func runtimeValidationOwnsUnresolvedScalarSource(source factflow.ValueSource) bool {
	return source.Valid() && source.Kind == factflow.ValueSourceCall && source.HasCallPoint &&
		source.ResultIndex >= 0 && !source.OpenTail &&
		(!source.Adjusted || source.ResultIndex == 0)
}

// validatePredicateExpr certifies ref as a predicate ROOT: a return value or
// CFG branch condition, which must itself satisfy the root producer gate
// (comparison, and/or, unary, or the lua_type intrinsic).
func validatePredicateExpr(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool) error {
	return validatePredicateExprScope(ctx, ref, active, true)
}

// validatePredicateOperandExpr certifies ref as a predicate OPERAND: an
// expression reached only as the scalar operand of an already-certified
// root. Its gate is the operand-legality rule the downstream compiler can
// lower exactly for a nested operand position (isPureUnaryOperator /
// isPureBinaryOperator in exactCompilerSourceTermActive), which is wider
// than the root gate for binary operators: pure arithmetic such as "*" is a
// legal nested operand even though it can never itself be a predicate root.
// Comparison and and/or operators satisfy both gates identically, so an
// operand that is itself a comparison/logical root still goes through the
// same structural-region and registration bookkeeping as a root would.
func validatePredicateOperandExpr(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool) error {
	return validatePredicateExprScope(ctx, ref, active, false)
}

func validatePredicateExprScope(ctx *planCompileContext, ref factflow.ExprRef, active map[factflow.ExprRef]bool, root bool) error {
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
	validUnary := ok && op.Kind() == factflow.ExpressionOperationUnary &&
		(hasIdentity && identity == intrinsic.LuaType || !hasIdentity && isPureUnaryOperator(op.Op()))
	binaryGate := isSupportedPredicateBinaryOperator
	if !root {
		binaryGate = isPureBinaryOperator
	}
	validBinary := ok && op.Kind() == factflow.ExpressionOperationBinary && binaryGate(op.Op())
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
	// and/or certify only their controlling left operand. Their RHS is an
	// independently CFG-owned value producer selected by the structural
	// region; requiring it to be a predicate would reject ordinary composite
	// values (for example a guarded concat) despite never using that value as
	// branch evidence. Exact comparisons still require both scalar operands.
	if validBinary && op.Op() != "and" && op.Op() != "or" {
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

func isExactScalarPredicateOperator(operator string) bool {
	switch operator {
	case "==", "~=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func isSupportedPredicateBinaryOperator(operator string) bool {
	return isExactScalarPredicateOperator(operator) || operator == "and" || operator == "or"
}

func validatePredicateSource(ctx *planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) error {
	if !source.Valid() || source.Expanded || source.OpenTail {
		return fmt.Errorf("non-scalar operand %#v", source)
	}
	// Calls remain contextual operations; only their scalar result coordinate
	// participates in this predicate DAG. predicateSourceBoundDuringRow below
	// admits that coordinate only when the canonical call surface owns the exact
	// point/result slot. The call itself is still executed in row order, so its
	// effects, suspension, failure, and dispatch are never modeled as pure.
	if source.HasExpr {
		if _, refined := ctx.facts.ExpressionRefinement(source.ExprRef); refined {
			return validateExpressionRefinement(ctx, source.ExprRef, active)
		}
		if _, nested := ctx.facts.ExpressionOperation(source.ExprRef); nested {
			return validatePredicateOperandExpr(ctx, source.ExprRef, active)
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
	case factflow.ValueSourceCall:
		if !source.HasCallPoint || source.ResultIndex < 0 || source.Adjusted && source.ResultIndex != 0 {
			return false
		}
		site, exactSite := ctx.facts.CallSiteView(source.CallPoint)
		point, exactPoint := site.Point()
		if !exactSite || !exactPoint || point != source.CallPoint {
			return false
		}
		surface, exactSurface := ctx.plan.CallSurface()
		if !exactSurface {
			return false
		}
		var targetKind operationplan.CallSurfaceTargetKind
		for _, classified := range surface.Sites() {
			if classified.Point == source.CallPoint {
				targetKind = classified.Target.Kind()
				break
			}
		}
		if targetKind == 0 {
			return false
		}
		for index := 0; index < site.ResultTargetCount(); index++ {
			result, present := site.ResultTargetAt(index)
			if present && result.ResultIndex() == source.ResultIndex {
				if targetKind == operationplan.CallSurfaceTargetLexical {
					return true
				}
				_, frozen := ctx.builder.Arena().callResultValue(source.CallPoint, source.ResultIndex)
				return frozen
			}
		}
		return false
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
