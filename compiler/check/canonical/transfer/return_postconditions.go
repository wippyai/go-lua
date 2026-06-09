package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// ApplyReturnPostconditions instantiates a callee's portable normal-return proof
// at a call continuation. ReturnPostconditions are the only cross-boundary
// carrier: transfer-local ParamNarrow effects must have been lowered before they
// reach this API.
func (t *Transfer) ApplyReturnPostconditions(out *flow.PointState, call *ast.FuncCallExpr, post paramevidence.ReturnPostconditions) (dead bool) {
	return t.ApplyReturnPostconditionsAtPoint(out, 0, call, post)
}

func (t *Transfer) ApplyReturnPostconditionsAtPoint(out *flow.PointState, point cfg.Point, call *ast.FuncCallExpr, post paramevidence.ReturnPostconditions) (dead bool) {
	if call == nil || !post.HasConstraints() {
		return false
	}
	if post.HasConstraints() {
		t.applyConditionEffect(out, ConditionEffect{Fact: post.Substitute(t.callArgumentPostconditionPaths(point, call))})
	}
	if out != nil && flow.PointStateDomain.Equal(*out, flow.PointStateDomain.Bottom()) {
		return true
	}
	for _, c := range post.Condition().MustConstraints() {
		if t.applyReturnPostconditionConstraint(out, point, call, c) {
			return true
		}
	}
	return false
}

func (t *Transfer) applyReturnPostconditionConstraint(out *flow.PointState, point cfg.Point, call *ast.FuncCallExpr, c constraint.Constraint) (dead bool) {
	if relation, ok := constraint.DirectPathRelation(c); ok {
		return t.applyReturnPostconditionRelation(out, point, call, relation)
	}
	predicate, ok := constraint.SinglePathPredicate(c)
	if !ok {
		return false
	}
	return t.applyReturnPostconditionPredicate(out, point, call, predicate)
}

func (t *Transfer) applyReturnPostconditionRelation(out *flow.PointState, point cfg.Point, call *ast.FuncCallExpr, relation constraint.PathRelation) (dead bool) {
	left, ok := placeholderArgument(relation.Left, call)
	if !ok || len(relation.Left.Segments) != 0 {
		return false
	}
	right, ok := placeholderArgument(relation.Right, call)
	if !ok || len(relation.Right.Segments) != 0 {
		return false
	}
	equality, ok := relation.IsEquality()
	if !ok {
		return false
	}
	if equality {
		t.applyArgumentEqualityProof(out, point, left, right)
		t.applyArgumentValueIntersection(out, left, right)
		return false
	}
	t.applyArgumentInequalityProof(out, point, left, right)
	return false
}

func (t *Transfer) applyReturnPostconditionPredicate(out *flow.PointState, point cfg.Point, call *ast.FuncCallExpr, predicate constraint.PathPredicate) (dead bool) {
	arg, ok := placeholderArgument(predicate.Path, call)
	if !ok {
		return false
	}
	if len(predicate.Path.Segments) == 0 {
		if check, ok := conditionArgumentCheckFromPathPredicate(predicate); ok {
			t.applyParamCondNarrowAtPoint(out, point, arg, check)
			if out != nil && flow.PointStateDomain.Equal(*out, flow.PointStateDomain.Bottom()) {
				return true
			}
		}
	}
	argSym, argSegs, ok := t.pathSymbol(arg)
	if !ok {
		return false
	}
	segs := append(append([]constraint.Segment{}, argSegs...), predicate.Path.Segments...)
	switch predicate.Kind {
	case constraint.PathPredicateHasType, constraint.PathPredicateNotHasType:
		check := cfg.CheckTypeEqual
		if predicate.Kind == constraint.PathPredicateNotHasType {
			check = cfg.CheckTypeNot
		}
		if t.narrowAssertTypePath(out, argSym, segs, check, predicate.Type) {
			return true
		}
	case constraint.PathPredicateIsNil, constraint.PathPredicateFalsy, constraint.PathPredicateNotNil, constraint.PathPredicateTruthy:
		check, ok := conditionCheckFromPathPredicate(predicate)
		if !ok {
			return false
		}
		if t.narrowAssertPath(out, argSym, segs, check) {
			return true
		}
		if len(segs) == 0 && (check == cfg.CheckNil || check == cfg.CheckFalsy) {
			t.applySiblingNilForErr(out, argSym)
		}
	}
	return false
}

func placeholderArgument(path constraint.Path, call *ast.FuncCallExpr) (ast.Expr, bool) {
	if call == nil {
		return nil, false
	}
	idx := path.PlaceholderIndex()
	if idx < 0 || idx >= len(call.Args) {
		return nil, false
	}
	return call.Args[idx], call.Args[idx] != nil
}

func conditionArgumentCheckFromPathPredicate(predicate constraint.PathPredicate) (cfg.CondCheckKind, bool) {
	switch predicate.Kind {
	case constraint.PathPredicateTruthy:
		return cfg.CheckTruthy, true
	case constraint.PathPredicateFalsy:
		return cfg.CheckFalsy, true
	default:
		return cfg.CheckNone, false
	}
}

func conditionCheckFromPathPredicate(predicate constraint.PathPredicate) (cfg.CondCheckKind, bool) {
	switch predicate.Kind {
	case constraint.PathPredicateTruthy:
		return cfg.CheckTruthy, true
	case constraint.PathPredicateFalsy:
		return cfg.CheckFalsy, true
	case constraint.PathPredicateIsNil:
		return cfg.CheckNil, true
	case constraint.PathPredicateNotNil:
		return cfg.CheckNotNil, true
	default:
		return cfg.CheckNone, false
	}
}

// callArgumentPostconditionPaths is the call-site binding for placeholder-rooted
// callee postconditions. Untracked or dynamic arguments are left empty so
// constraint substitution drops only the facts that cannot be grounded in flow.
func (t *Transfer) callArgumentPostconditionPaths(point cfg.Point, call *ast.FuncCallExpr) []constraint.Path {
	args := make([]constraint.Path, len(call.Args))
	for i, arg := range call.Args {
		path, ok := t.versionedStaticPathOfExpr(point, arg)
		if !ok || path.Symbol == 0 {
			continue
		}
		args[i] = path
	}
	return args
}

func (t *Transfer) narrowAssertTypePath(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, check cfg.CondCheckKind, key narrow.TypeKey) (dead bool) {
	if key.Kind != narrow.TypeKeyBuiltin || key.Name == "" {
		return false
	}
	switch check {
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
	default:
		return false
	}
	return t.narrowAssertPathWithTypeName(out, sym, segments, check, key.Name, false)
}

// applySiblingNilForErr strips nil from the value siblings correlated with err
// symbol, when err has just been proven nil. It is the call-site (param-narrow)
// counterpart of narrowBySiblingNil's branch-edge narrowing.
func (t *Transfer) applySiblingNilForErr(out *flow.PointState, errSym cfg.SymbolID) {
	if out == nil || errSym == 0 {
		return
	}
	bind, ok := out.Rel.SiblingNil(errSym)
	if !ok {
		return
	}
	for _, vs := range bind.ValueSyms {
		if vs == 0 {
			continue
		}
		av, has := t.symbolValue(out, vs)
		if !has || av.IsZero() {
			continue
		}
		t.setNarrowedSymbol(out, vs, product.NarrowPresent(av))
	}
}

// applyParamCondNarrow narrows the value an argument CONDITION tests when the callee
// proves that argument's truthiness on every normal return. The proven check is the
// argument condition's truthiness (CheckFalsy for `maybeError(cond) if cond then
// error()`); applying it is equivalent to taking the branch edge on which `if arg`
// has that truthiness, so the argument's tested value is narrowed by the same per-edge
// machinery. An argument the narrowing cannot classify leaves out unchanged.
func (t *Transfer) applyParamCondNarrow(out *flow.PointState, arg ast.Expr, proven cfg.CondCheckKind) {
	t.applyParamCondNarrowAtPoint(out, 0, arg, proven)
}

func (t *Transfer) applyParamCondNarrowAtPoint(out *flow.PointState, point cfg.Point, arg ast.Expr, proven cfg.CondCheckKind) {
	wantTruthy := proven == cfg.CheckTruthy
	leaf := &cfg.BranchInfo{Condition: arg}
	condVar, check := extraction.ExtractCondition(arg)
	leaf.CondVar = condVar
	leaf.CondCheck = check
	leaf.CondSymbol = t.condRootSymbol(arg, condVar)
	narrowed := t.narrowEdgeInner(point, *out, leaf, wantTruthy, false)
	applyNarrowedEdgeState(out, narrowed)
}

// applyArgumentValueIntersection is the value-domain half of a proven argument
// equality. The condition axis records the relation; this opportunistically
// narrows a tracked left-hand argument to the overlap with the right-hand value.
func (t *Transfer) applyArgumentValueIntersection(out *flow.PointState, left, right ast.Expr) {
	targetSym, segs, ok := t.pathSymbol(left)
	if !ok || len(segs) != 0 {
		return
	}
	targetAV, has := t.narrowBaseFor(*out, targetSym, false)
	if !has {
		return
	}
	otherAV, ok := t.evalExpr(out, right, nil)
	if !ok || otherAV.IsZero() {
		return
	}
	targetType := targetAV.ProjectValue()
	otherType := otherAV.ProjectValue()
	if targetType == nil || otherType == nil {
		return
	}
	refined := narrow.Intersect(targetType, otherType)
	if refined == nil || refined == targetType {
		return
	}
	if refined.Kind().IsNever() {
		t.setNarrowedSymbol(out, targetSym, product.Bottom())
		return
	}
	t.setNarrowedSymbol(out, targetSym, product.FromType(refined))
}

func (t *Transfer) applyArgumentInequalityProof(out *flow.PointState, point cfg.Point, left, right ast.Expr) {
	if out == nil || left == nil || right == nil {
		return
	}
	rel := &ast.RelationalOpExpr{
		Lhs:      left,
		Operator: "~=",
		Rhs:      right,
	}
	leaf := &cfg.BranchInfo{Condition: rel}
	narrowed := t.narrowEdgeInner(point, *out, leaf, true, false)
	applyNarrowedEdgeState(out, narrowed)
}

func (t *Transfer) applyArgumentEqualityProof(out *flow.PointState, point cfg.Point, left, right ast.Expr) {
	if out == nil || left == nil || right == nil {
		return
	}
	t.applyLengthEqualityProof(out, left, right)
	t.applyLiteralEqualityProof(out, point, left, right)
	t.applyLiteralEqualityProof(out, point, right, left)
}

func (t *Transfer) applyLengthEqualityProof(out *flow.PointState, left, right ast.Expr) {
	place, container, c, op, ok := t.orientLengthComparison(left, right, "==")
	if !ok {
		return
	}
	ops := flow.NumericLengthBoundContainerOps(container, op, c)
	if len(ops) == 0 {
		return
	}
	flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops})
	if floor, _, hasFloor, _ := flow.LengthBoundFromOp(op, c); hasFloor && floor > 0 {
		t.applyRefinementEffect(out, RefinementEffect{
			Place:     place,
			Kind:      RefinementLengthLowerBound,
			LengthMin: floor,
			PreferEnv: true,
		})
	}
}

func (t *Transfer) applyLiteralEqualityProof(out *flow.PointState, point cfg.Point, access ast.Expr, value ast.Expr) {
	lit, ok := literalValue(value)
	if !ok || lit == nil {
		return
	}
	t.seedPathIndexPresence(out, access)
	if t.narrowLiteralEqualityPath(out, access, lit) {
		return
	}
	if cond := t.literalEqualityCondition(point, access, lit); cond.HasConstraints() {
		t.applyConditionEffect(out, ConditionEffect{Fact: cond})
	}
}

func (t *Transfer) seedPathIndexPresence(out *flow.PointState, expr ast.Expr) {
	path, ok := t.staticPathOfExpr(expr)
	if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
		return
	}
	flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: flow.NumericLenGeConstIndexedPrefixOps(path)})
}

func (t *Transfer) narrowLiteralEqualityPath(out *flow.PointState, access ast.Expr, lit *typ.Literal) bool {
	attr, ok := access.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return false
	}
	field, ok := staticAttrFieldName(attr)
	if !ok || field == "" {
		return false
	}
	basePath, ok := t.staticPathOfExpr(attr.Object)
	if !ok || basePath.Symbol == 0 {
		return false
	}
	av, _ := t.symbolValue(out, basePath.Symbol)
	if av.IsZero() {
		return false
	}
	root := av.ProjectValue()
	if root == nil {
		return false
	}
	target, ok := readFieldPath(root, basePath.Segments)
	if !ok || target == nil {
		return false
	}
	refined, ok := narrowDiscriminantUnion(target, field, lit, true)
	if !ok {
		return false
	}
	rewritten := refineAtPath(root, basePath.Segments, func(typ.Type) typ.Type { return refined })
	if rewritten == nil || typ.TypeEquals(rewritten, root) {
		return false
	}
	t.setNarrowedSymbol(out, basePath.Symbol, product.FromType(rewritten))
	return true
}

func (t *Transfer) literalEqualityCondition(point cfg.Point, access ast.Expr, lit *typ.Literal) constraint.Condition {
	attr, ok := access.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return constraint.TrueCondition()
	}
	field, ok := staticAttrFieldName(attr)
	if !ok || field == "" {
		return constraint.TrueCondition()
	}
	target, ok := t.versionedStaticPathOfExpr(point, attr.Object)
	if !ok || target.Symbol == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromConstraints(constraint.FieldEquals{Target: target, Field: field, Value: lit})
}

func applyNarrowedEdgeState(out *flow.PointState, narrowed flow.PointState) {
	if out == nil {
		return
	}
	*out = narrowed
}
