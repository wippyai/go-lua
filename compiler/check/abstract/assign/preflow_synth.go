package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/cond"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/domain/conditionexpr"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	fbpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/synth/callarg"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type typeOpsNarrowResolver struct {
	ctx *db.QueryContext
	ops core.TypeOps
}

var _ narrow.Resolver = (*typeOpsNarrowResolver)(nil)

func (r typeOpsNarrowResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if r.ops == nil {
		return nil, false
	}
	return r.ops.Field(r.ctx, t, name)
}

func (r typeOpsNarrowResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if r.ops == nil {
		return nil, false
	}
	return r.ops.Index(r.ctx, t, key)
}

func (r typeOpsNarrowResolver) BinaryOp(left typ.Type, op string, right typ.Type) typ.Type {
	if r.ops == nil {
		return nil
	}
	return r.ops.BinaryOp(r.ctx, left, op, right)
}

func (r typeOpsNarrowResolver) UnaryOp(op string, operand typ.Type) typ.Type {
	if r.ops == nil {
		return nil
	}
	return r.ops.UnaryOp(r.ctx, op, operand)
}

type overlayTypeAt func(cfg.SymbolID, cfg.Point) (typ.Type, bool)

func mapOverlayTypeAt(overlay map[cfg.SymbolID]typ.Type) overlayTypeAt {
	return func(sym cfg.SymbolID, _ cfg.Point) (typ.Type, bool) {
		t, ok := overlay[sym]
		return t, ok
	}
}

// buildPreflowBranchSolution solves only branch/numeric edge facts that are
// already available before assignment extraction completes.
//
// This gives local inference access to canonical branch narrowing such as
// discriminant checks on parameters, without depending on later assignment-
// derived facts or full post-extraction solve.
func buildPreflowBranchSolution(fc *abstractcore.FlowContext, inputs *flow.Inputs) *flow.Solution {
	if fc == nil || inputs == nil || inputs.Graph == nil || fc.TypeOps == nil {
		return nil
	}

	temp := *inputs
	temp.EdgeConditions = nil
	temp.EdgeNumericConstraints = nil

	cond.ExtractEdgeConstraints(fc, &temp)
	cond.ExtractNumericConstraints(fc, &temp)

	return flow.SolveConditionView(&temp, typeOpsNarrowResolver{ctx: fc.CallCtx, ops: fc.TypeOps})
}

// synthWithOverlayAndPreflow wraps base synthesis with overlay lookup and a
// preflow branch-narrowing view for identifiers and attribute/index reads.
//
// This keeps assignment inference on the canonical synthesis path while letting
// recursive field/index expressions observe already-provable branch facts.
func synthWithOverlayAndPreflow(
	overlay overlayTypeAt,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *flow.Solution,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	return newPreflowOverlaySynthesizer(overlay, bindings, inputs, callCtx, typeOps, preflow, base).Synth
}

type overlayExprKey struct {
	expr  ast.Expr
	point cfg.Point
}

type preflowOverlaySynthesizer struct {
	overlay  overlayTypeAt
	bindings *bind.BindingTable
	inputs   *flow.Inputs
	ctx      *db.QueryContext
	typeOps  core.TypeOps
	preflow  *flow.Solution
	base     func(ast.Expr, cfg.Point) typ.Type
	query    *db.Query[overlayExprKey, typ.Type]

	localCondition *constraint.Condition
}

func newPreflowOverlaySynthesizer(
	overlay overlayTypeAt,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *flow.Solution,
	base func(ast.Expr, cfg.Point) typ.Type,
) *preflowOverlaySynthesizer {
	if callCtx == nil {
		callCtx = db.NewQueryContext(db.New())
	}
	s := &preflowOverlaySynthesizer{
		overlay:  overlay,
		bindings: bindings,
		inputs:   inputs,
		ctx:      callCtx,
		typeOps:  typeOps,
		preflow:  preflow,
		base:     base,
	}
	s.query = db.NewQueryWithSeedAndWiden(
		"check.assign.preflow-overlay-synth",
		func(_ *db.QueryContext, key overlayExprKey) typ.Type {
			return s.eval(key.expr, key.point)
		},
		typ.TypeEquals,
		func(*db.QueryContext, overlayExprKey) typ.Type {
			return typ.Unknown
		},
		value.MergeForConvergence,
	)
	return s
}

func (s *preflowOverlaySynthesizer) Synth(expr ast.Expr, p cfg.Point) typ.Type {
	if expr == nil {
		return nil
	}
	if s.localCondition != nil {
		return s.eval(expr, p)
	}
	return s.query.Get(s.ctx, overlayExprKey{expr: expr, point: p})
}

func (s *preflowOverlaySynthesizer) withCondition(cond constraint.Condition) *preflowOverlaySynthesizer {
	if s == nil || (!cond.HasConstraints() && !cond.IsFalse()) {
		return s
	}
	if s.localCondition != nil {
		cond = constraint.And(*s.localCondition, cond)
	}
	next := *s
	next.localCondition = &cond
	return &next
}

func (s *preflowOverlaySynthesizer) eval(expr ast.Expr, p cfg.Point) typ.Type {
	if expr == nil {
		return nil
	}
	if t, ok := s.overlayIdent(expr, p); ok {
		return t
	}
	if t, ok := s.preflowPathFact(expr, p); ok {
		return t
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok {
		if t := s.attrRead(attr, p); !typ.IsAbsentOrUnknown(t) {
			return t
		}
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		return s.callFirstResult(call, expr, p)
	}
	if logical, ok := expr.(*ast.LogicalOpExpr); ok {
		return s.logicalResult(logical, expr, p)
	}
	// Operators recurse through the overlay so a gradual `any` operand resolved
	// from inference (and not visible to the base synth) carries its dynamic
	// contract into the operator result instead of degrading to `unknown`.
	if t, ok := s.operatorResult(expr, p); ok {
		return t
	}
	return s.baseResult(expr, p)
}

// operatorResult computes arithmetic/concat/relational/unary results from
// overlay-resolved operands. It returns ok=false for non-operator expressions
// and when the base synth must own the result (the overlay produced no more
// informative type than the base would).
func (s *preflowOverlaySynthesizer) operatorResult(expr ast.Expr, p cfg.Point) (typ.Type, bool) {
	if s.typeOps == nil {
		return nil, false
	}
	var op string
	var left, right ast.Expr
	switch ex := expr.(type) {
	case *ast.ArithmeticOpExpr:
		op, left, right = ex.Operator, ex.Lhs, ex.Rhs
	case *ast.StringConcatOpExpr:
		op, left, right = "..", ex.Lhs, ex.Rhs
	case *ast.UnaryMinusOpExpr:
		operand := s.Synth(ex.Expr, p)
		return s.overlayUnary("-", operand)
	case *ast.UnaryBNotOpExpr:
		operand := s.Synth(ex.Expr, p)
		return s.overlayUnary("~", operand)
	case *ast.UnaryLenOpExpr:
		operand := s.Synth(ex.Expr, p)
		return s.overlayUnary("#", operand)
	default:
		return nil, false
	}
	lt := s.Synth(left, p)
	rt := s.Synth(right, p)
	if typ.IsAbsentOrUnknown(lt) && typ.IsAbsentOrUnknown(rt) {
		return nil, false
	}
	res := s.typeOps.BinaryOp(s.ctx, lt, op, rt)
	if typ.IsAbsentOrUnknown(res) {
		return nil, false
	}
	return res, true
}

func (s *preflowOverlaySynthesizer) overlayUnary(op string, operand typ.Type) (typ.Type, bool) {
	if typ.IsAbsentOrUnknown(operand) {
		return nil, false
	}
	res := s.typeOps.UnaryOp(s.ctx, op, operand)
	if typ.IsAbsentOrUnknown(res) {
		return nil, false
	}
	return res, true
}

func (s *preflowOverlaySynthesizer) overlayIdent(expr ast.Expr, p cfg.Point) (typ.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || s.bindings == nil || s.overlay == nil {
		return nil, false
	}
	sym, ok := s.bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return nil, false
	}
	return s.overlay(sym, p)
}

func (s *preflowOverlaySynthesizer) preflowPathFact(expr ast.Expr, p cfg.Point) (typ.Type, bool) {
	if s.preflow == nil || s.bindings == nil || s.inputs == nil {
		return nil, false
	}
	constResolver := predicate.BuildConstResolver(s.inputs, p)
	path := fbpath.FromExprWithBindings(expr, constResolver, s.bindings)
	if path.IsEmpty() {
		return nil, false
	}
	narrowed := s.conditionedPathFact(p, path)
	if typ.IsAbsentOrUnknown(narrowed) {
		return nil, false
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || s.typeOps == nil {
		return narrowed, true
	}
	refined, keep := s.refinePreflowAttrFact(attr, narrowed, p)
	return refined, keep
}

func (s *preflowOverlaySynthesizer) conditionedPathFact(p cfg.Point, path constraint.Path) typ.Type {
	if s.localCondition == nil {
		return s.preflow.ConditionTypeAt(p, path)
	}
	rootPath := constraint.Path{Root: path.Root, Symbol: path.Symbol}
	rootType := s.seedRootType(rootPath, p)
	if typ.IsAbsentOrUnknown(rootType) {
		return s.preflow.ConditionedTypeAt(p, path, *s.localCondition)
	}
	return s.preflow.ConditionedSeedTypeAt(p, rootPath, rootType, path, *s.localCondition)
}

func (s *preflowOverlaySynthesizer) seedRootType(rootPath constraint.Path, p cfg.Point) typ.Type {
	if s.overlay != nil && rootPath.Symbol != 0 {
		if t, ok := s.overlay(rootPath.Symbol, p); ok && !typ.IsAbsentOrUnknown(t) {
			return t
		}
	}
	if s.preflow != nil {
		return s.preflow.ConditionTypeAt(p, rootPath)
	}
	return nil
}

func (s *preflowOverlaySynthesizer) refinePreflowAttrFact(attr *ast.AttrGetExpr, narrowed typ.Type, p cfg.Point) (typ.Type, bool) {
	if objType := s.Synth(attr.Object, p); !typ.IsAbsentOrUnknown(objType) {
		if refined := refinePreflowLengthIndex(attr, objType, narrowed, p, s.bindings, s.inputs, s.preflow); refined != nil {
			return refined, true
		}
	}
	declared := declaredAttrReadType(attr, p, s.Synth, s.ctx, s.typeOps)
	if declared == nil {
		return narrowed, true
	}
	return value.ReconcilePathFactWithDeclaredRead(narrowed, declared)
}

func (s *preflowOverlaySynthesizer) attrRead(attr *ast.AttrGetExpr, p cfg.Point) typ.Type {
	if attr == nil || s.typeOps == nil {
		return nil
	}
	objType := s.Synth(attr.Object, p)
	if typ.IsAbsentOrUnknown(objType) {
		return nil
	}
	if seg, ok := pathseg.StaticAttrSegment(attr); ok {
		switch seg.Kind {
		case constraint.SegmentField:
			if ft, ok := s.staticAttrReadType(objType, seg); ok && !typ.IsAbsentOrUnknown(ft) {
				return s.reconcileFieldAgainstDeclaredObject(attr.Object, seg.Name, ft, p)
			}
		case constraint.SegmentIndexString, constraint.SegmentIndexInt:
			if it, ok := s.staticAttrReadType(objType, seg); ok && !typ.IsAbsentOrUnknown(it) {
				return it
			}
		}
		return nil
	}
	return s.dynamicAttrRead(attr, objType, p)
}

func (s *preflowOverlaySynthesizer) staticAttrReadType(objType typ.Type, seg constraint.Segment) (typ.Type, bool) {
	return staticAttrReadType(s.ctx, s.typeOps, objType, seg)
}

func staticAttrReadType(ctx *db.QueryContext, typeOps core.TypeOps, objType typ.Type, seg constraint.Segment) (typ.Type, bool) {
	if typeOps == nil || typ.IsAbsentOrUnknown(objType) {
		return nil, false
	}
	switch seg.Kind {
	case constraint.SegmentField:
		if seg.Name == "" {
			return nil, false
		}
		if ft, ok := typeOps.Field(ctx, objType, seg.Name); ok && !typ.IsAbsentOrUnknown(ft) {
			return ft, true
		}
		return typeOps.Index(ctx, objType, typ.LiteralString(seg.Name))
	case constraint.SegmentIndexString:
		return typeOps.Index(ctx, objType, typ.LiteralString(seg.Name))
	case constraint.SegmentIndexInt:
		if seg.Index < 1 {
			return nil, false
		}
		return typeOps.Index(ctx, objType, typ.LiteralInt(int64(seg.Index)))
	default:
		return nil, false
	}
}

// reconcileFieldAgainstDeclaredObject restores declared optionality on a
// value-precise field read by reconciling against the field's declared type
// resolved from the object's DECLARED type (walked from an annotated root). A
// literal constructed under an explicit annotation flows precise field shapes;
// the declared field's optionality must survive so the preflow seed matches the
// observation read.
func (s *preflowOverlaySynthesizer) reconcileFieldAgainstDeclaredObject(obj ast.Expr, key string, ft typ.Type, p cfg.Point) typ.Type {
	declaredObj := s.declaredExprType(obj, p)
	if declaredObj == nil {
		return ft
	}
	declaredField, ok := s.typeOps.Field(s.ctx, declaredObj, key)
	if !ok || declaredField == nil {
		return ft
	}
	if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(ft, declaredField); ok && reconciled != nil {
		return reconciled
	}
	return ft
}

// declaredExprType resolves the declared (annotation-derived) type of an
// identifier or static field-access path, walking to an annotated root symbol.
func (s *preflowOverlaySynthesizer) declaredExprType(expr ast.Expr, p cfg.Point) typ.Type {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if s.bindings == nil || s.inputs == nil {
			return nil
		}
		sym, ok := s.bindings.SymbolOf(e)
		if !ok || sym == 0 {
			return nil
		}
		if s.inputs.AnnotatedVars == nil || !s.inputs.AnnotatedVars[sym] {
			return nil
		}
		declared := s.inputs.DeclaredTypes[sym]
		if typ.IsAbsentOrUnknown(declared) || declared.Kind().IsPlaceholder() {
			return nil
		}
		return declared
	case *ast.AttrGetExpr:
		objDeclared := s.declaredExprType(e.Object, p)
		if objDeclared == nil {
			return nil
		}
		seg, ok := pathseg.StaticAttrSegment(e)
		if !ok {
			return nil
		}
		if t, ok := s.staticAttrReadType(objDeclared, seg); ok {
			return t
		}
	}
	return nil
}

func (s *preflowOverlaySynthesizer) dynamicAttrRead(attr *ast.AttrGetExpr, objType typ.Type, p cfg.Point) typ.Type {
	keyType := s.Synth(attr.Key, p)
	if typ.IsAbsentOrUnknown(keyType) {
		return nil
	}
	it, ok := s.typeOps.Index(s.ctx, objType, keyType)
	if !ok || typ.IsAbsentOrUnknown(it) {
		return nil
	}
	if refined := refinePreflowLengthIndex(attr, objType, it, p, s.bindings, s.inputs, s.preflow); refined != nil {
		return refined
	}
	return it
}

func (s *preflowOverlaySynthesizer) callFirstResult(call *ast.FuncCallExpr, expr ast.Expr, p cfg.Point) typ.Type {
	if s.typeOps != nil {
		if result := evalOverlayCallFirstResult(call, p, s.Synth, s.ctx, s.typeOps, s.bindings); !typ.IsAbsentOrUnknown(result) {
			return result
		}
	}
	if direct := s.baseResult(expr, p); !typ.IsAbsentOrUnknown(direct) {
		return direct
	}
	return typ.Unknown
}

func (s *preflowOverlaySynthesizer) logicalResult(logical *ast.LogicalOpExpr, expr ast.Expr, p cfg.Point) typ.Type {
	left := s.Synth(logical.Lhs, p)
	var result typ.Type
	switch logical.Operator {
	case "and":
		if ops.IsFalsy(left) {
			return left
		}
		right := s.withCondition(s.branchCondition(logical.Lhs, p, true)).Synth(logical.Rhs, p)
		result = ops.LogicalAndTyped(left, right)
	case "or":
		if ops.IsTruthy(left) {
			return left
		}
		right := s.withCondition(s.branchCondition(logical.Lhs, p, false)).Synth(logical.Rhs, p)
		result = ops.LogicalOrTyped(left, right)
	default:
		result = typ.Unknown
	}
	if typ.IsAbsentOrUnknown(result) || typ.IsAny(result) {
		if direct := s.baseResult(expr, p); !typ.IsAbsentOrUnknown(direct) && !typ.IsAny(direct) {
			return direct
		}
	}
	return result
}

func (s *preflowOverlaySynthesizer) branchCondition(expr ast.Expr, p cfg.Point, truthy bool) constraint.Condition {
	if expr == nil {
		return constraint.TrueCondition()
	}
	return (conditionexpr.Extractor{
		P:             p,
		Inputs:        s.inputs,
		Bindings:      s.bindings,
		ConstResolver: predicate.BuildConstResolver(s.inputs, p),
	}).ConditionForTruth(expr, truthy)
}

func (s *preflowOverlaySynthesizer) baseResult(expr ast.Expr, p cfg.Point) typ.Type {
	if s.base == nil {
		return nil
	}
	return s.base(expr, p)
}

func refinePreflowLengthIndex(attr *ast.AttrGetExpr, objType, indexResult typ.Type, p cfg.Point, bindings *bind.BindingTable, inputs *flow.Inputs, preflow *flow.Solution) typ.Type {
	if attr == nil || bindings == nil || inputs == nil || preflow == nil {
		return nil
	}
	constResolver := predicate.BuildConstResolver(inputs, p)
	tablePath := fbpath.FromExprWithBindings(attr.Object, constResolver, bindings)
	if tablePath.IsEmpty() {
		return nil
	}
	lenPath, offset, ok := lengthIndexPathFromExpr(attr.Key, constResolver, bindings)
	if !ok || !lenPath.Equal(tablePath) {
		return nil
	}
	lower, _, ok := preflow.LengthBoundsAt(p, tablePath)
	if !ok {
		return nil
	}
	return narrow.RefineLengthIndex(objType, indexResult, lower, offset)
}

// evalOverlayCallFirstResult evaluates a call expression inside assignment
// transfer using the shared call domain. It is a local value evaluator only:
// facts and diagnostics are still published by the canonical call/evidence
// consumers after the abstract state is solved.
func evalOverlayCallFirstResult(
	call *ast.FuncCallExpr,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	bindings *bind.BindingTable,
) typ.Type {
	if call == nil || synth == nil || typeOps == nil {
		return nil
	}
	if metatable.IsSetMetatableCall(call, bindings) {
		return nil
	}
	args := make([]typ.Type, len(call.Args))
	for i, arg := range call.Args {
		args[i] = synth(arg, p)
	}
	def := ops.CallDef{
		Args:  args,
		Query: typeOps,
	}
	if call.Method != "" {
		def.IsMethod = true
		def.MethodName = call.Method
		def.Receiver = synth(call.Receiver, p)
	} else {
		def.Callee = synth(call.Func, p)
	}
	result := ops.NewCallPipeline(callCtx, def, len(call.Args)).
		WithReSynth(assignmentCallArgReSynth(call.Args, synth, p)).
		Run()
	if len(result.Returns) > 0 {
		return result.Returns[0]
	}
	return ops.ExtractFirstValue(result.Type)
}

func assignmentCallArgReSynth(args []ast.Expr, synth func(ast.Expr, cfg.Point) typ.Type, p cfg.Point) ops.ArgReSynth {
	if synth == nil {
		return nil
	}
	return callarg.ForArgs(args, callarg.Full(
		func(arg ast.Expr, _ cfg.Point, _ typ.Type) typ.Type {
			return synth(arg, p)
		},
		nil,
		p,
	))
}

func declaredAttrReadType(
	attr *ast.AttrGetExpr,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
) typ.Type {
	if attr == nil || synth == nil || typeOps == nil {
		return nil
	}
	objType := synth(attr.Object, p)
	if typ.IsAbsentOrUnknown(objType) {
		return nil
	}
	if seg, ok := pathseg.StaticAttrSegment(attr); ok {
		if t, ok := staticAttrReadType(callCtx, typeOps, objType, seg); ok {
			return t
		}
	} else {
		keyType := synth(attr.Key, p)
		if !typ.IsAbsentOrUnknown(keyType) {
			if it, ok := typeOps.Index(callCtx, objType, keyType); ok {
				return it
			}
		}
	}
	return nil
}
