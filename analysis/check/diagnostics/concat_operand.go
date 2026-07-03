package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// concatOperands reports concat operands that may be nil. The value layer still
// materializes concat's present result type so downstream assignments do not
// disappear, but diagnostics must surface the runtime nil risk unless flow
// evidence proves the operand is present.
type concatOperands producerContext

func (p concatOperands) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := producerContext(p).guardEnvironments(result)
	truthyBranches := newTruthyDominatingBranchProofs(result)
	var out []diagnostic.Diagnostic
	seen := make(map[concatSeenKey]struct{})
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		check := concatOperandCheck{
			result: result,
			query:  producerContext(p).query(result),
			flow:   p.flow,
			typer:  newStructuralFlowExpressionTyper(result, p.resolver, point, envs[point]),
			envs:   envs,
			truthy: truthyBranches,
			point:  point,
		}
		emit := func(expr ast.Expr) {
			p.walk(expr, check, seen, &out, 0)
		}
		if fact, ok := result.LocalAssignment(point); ok {
			emit(fact.Expr)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			emit(fact.Value)
			emitAssignmentTargetReads(fact.Target, emit)
		}
		if fact, ok := result.Call(point); ok {
			emit(fact.Call)
		}
		if fact, ok := result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				emit(expr)
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			emit(fact.Condition)
		}
	}
	return out
}

func (p concatOperands) walk(expr ast.Expr, check concatOperandCheck, seen map[concatSeenKey]struct{}, out *[]diagnostic.Diagnostic, depth int) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	walkChild := func(child ast.Expr) {
		p.walk(child, check, seen, out, depth+1)
	}
	if e, ok := expr.(*ast.LogicalOpExpr); ok {
		p.walk(e.Lhs, check, seen, out, depth+1)
		switch e.Operator {
		case "and":
			next, reachable := check.withExpressionEdgeFacts(e.Lhs, true)
			if reachable {
				p.walk(e.Rhs, next, seen, out, depth+1)
			}
		case "or":
			next, reachable := check.withExpressionEdgeFacts(e.Lhs, false)
			if reachable {
				p.walk(e.Rhs, next, seen, out, depth+1)
			}
		default:
			p.walk(e.Rhs, check, seen, out, depth+1)
		}
		return
	}
	if e, ok := expr.(*ast.StringConcatOpExpr); ok {
		walkExprChildren(expr, walkChild)
		key := concatSeenKey{expr: e, point: check.point}
		if _, done := seen[key]; done {
			return
		}
		seen[key] = struct{}{}
		if d, ok := check.diagnostic(e, e.Lhs, "left"); ok {
			*out = append(*out, d)
			return
		}
		if d, ok := check.diagnostic(e, e.Rhs, "right"); ok {
			*out = append(*out, d)
		}
		return
	}
	walkExprChildren(expr, walkChild)
}

type concatOperandCheck struct {
	result *body.Result
	query  diagnosticQuery
	flow   *diagnosticFlowCache
	typer  expressionTyper
	envs   map[cfg.Point]guardEnv
	truthy truthyDominatingBranchProofs
	point  cfg.Point
}

type concatSeenKey struct {
	expr  *ast.StringConcatOpExpr
	point cfg.Point
}

func (c concatOperandCheck) diagnostic(expr *ast.StringConcatOpExpr, operand ast.Expr, side string) (diagnostic.Diagnostic, bool) {
	if c.operandProvenPresent(operand) {
		return diagnostic.Diagnostic{}, false
	}
	t, ok := c.operandType(operand)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return diagnostic.Diagnostic{}, false
	}
	if !projectionHasNil(t) {
		return diagnostic.Diagnostic{}, false
	}
	if attr, ok := operand.(*ast.AttrGetExpr); ok && indexReadProvenInRange(c.result, c.typer.resolver, c.point, attr) {
		if withoutNil := projectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return diagnostic.Diagnostic{}, false
		}
	}
	return concatOperandDiagnostic(operand, side, t), true
}

func (c concatOperandCheck) operandProvenPresent(operand ast.Expr) bool {
	if value, ok := c.query.ExpressionValueBeforeBoundary(c.point, operand); ok {
		switch p := product.PresenceOf(value); {
		case presence.Equal(p, presence.Present()):
			return true
		case presence.Equal(p, presence.Absent()):
			return false
		}
		if t, ok := c.query.ValueTypeWithPresence(value); ok && typ.Nil.Equals(t) {
			return false
		}
	}
	accessPath, ok := c.expressionPath(operand)
	if !ok {
		return false
	}
	if c.typer.env.hasNil(accessPath) || c.typer.env.hasFalsy(accessPath) {
		return false
	}
	if c.typer.env.hasPresent(accessPath) || c.typer.env.hasTruthy(accessPath) {
		return true
	}
	return c.truthy.provesPresentWithoutInvalidation(c.result, c.flow, c.point, accessPath)
}

// operandType resolves the operand's type for the nil-risk check. The structural
// pass approximates the type from declarations and guard proofs; the flow-solved
// value state is authoritative for presence, so when the structural type still
// carries nil but the flow proves the operand present at this point (an inferred
// optional narrowed by a guard the structural pass cannot key on), the present
// type wins. This keeps the runtime-nil-risk surfaced unless flow evidence
// proves presence, for inferred optionals as well as declared ones.
func (c concatOperandCheck) operandType(operand ast.Expr) (typ.Type, bool) {
	t, ok := c.structuralOperandType(operand)
	if !ok {
		return nil, false
	}
	if projectionHasNil(t) {
		if present, ok := c.flowPresentOperandType(operand); ok {
			return present, true
		}
	}
	return t, true
}

// flowPresentOperandType returns the operand's flow-solved type when the solved
// value state proves it present (carries no nil) at this point. It reads the
// value before same-node materialization, the same boundary the assignment
// nil-check uses, so a guard that narrowed the operand is reflected regardless
// of whether the operand was a declared or an inferred optional.
func (c concatOperandCheck) flowPresentOperandType(operand ast.Expr) (typ.Type, bool) {
	if c.result == nil {
		return nil, false
	}
	if accessPath, ok := c.expressionPath(operand); ok && (c.typer.env.hasFalsy(accessPath) || c.typer.env.hasNil(accessPath)) {
		return nil, false
	}
	value, ok := c.query.ExpressionValueBeforeBoundary(c.point, operand)
	if !ok {
		return nil, false
	}
	t, ok := c.query.ValueTypeWithPresence(value)
	if !ok || t == nil || projectionHasNil(t) {
		return nil, false
	}
	return t, true
}

func (c concatOperandCheck) structuralOperandType(operand ast.Expr) (typ.Type, bool) {
	if t, ok := projectedOptionalIndexType(c.result, c.typer.resolver, c.point, operand); ok {
		if present, ok := c.presentGuardOperandType(operand, t); ok {
			return present, true
		}
		return t, true
	}
	if t, ok := c.typer.typeOf(operand); ok {
		if projectionHasNil(t) {
			if present, ok := c.presentGuardOperandType(operand, t); ok {
				return present, true
			}
			if present, ok := c.truthyDominatingBranchOperandType(operand, t); ok {
				return present, true
			}
			if boundary, ok := c.rootIdentifierBoundaryType(operand); ok && !projectionHasNil(boundary) {
				return boundary, true
			}
			if declared, ok := c.dominatingLocalDeclarationType(operand); ok {
				return declared, true
			}
		}
		return t, true
	}
	if t, ok := c.dominatingLocalDeclarationType(operand); ok {
		if projectionHasNil(t) {
			if present, ok := c.truthyDominatingBranchOperandType(operand, t); ok {
				return present, true
			}
		}
		return t, true
	}
	if c.result == nil {
		return nil, false
	}
	value, ok := c.query.ExpressionValueAtBoundary(c.point, operand)
	if !ok {
		return nil, false
	}
	return c.query.ValueTypeWithPresence(value)
}

func (c concatOperandCheck) withExpressionEdgeFacts(expr ast.Expr, cond bool) (concatOperandCheck, bool) {
	if c.result == nil || expr == nil {
		return c, true
	}
	env, reachable := applyExpressionEdgeGuards(c.result, c.point, expr, cond, c.typer.env)
	if !reachable {
		return c, false
	}
	c.typer.env = env
	return c, true
}

func (c concatOperandCheck) expressionPath(expr ast.Expr) (pathdom.Path, bool) {
	if c.result == nil || expr == nil {
		return pathdom.Path{}, false
	}
	p, ok := c.query.ExpressionPath(expr)
	if !ok || p.IsEmpty() {
		return pathdom.Path{}, false
	}
	return p, true
}

func (c concatOperandCheck) truthyDominatingBranchOperandType(operand ast.Expr, t typ.Type) (typ.Type, bool) {
	if c.result == nil || operand == nil || t == nil {
		return nil, false
	}
	accessPath, ok := c.query.ExpressionPath(operand)
	if !ok || accessPath.IsEmpty() {
		return nil, false
	}
	if !c.truthy.provesPresent(c.point, accessPath) {
		return nil, false
	}
	withoutNil := projectionWithoutNil(t)
	if withoutNil == nil || typ.IsNever(withoutNil) {
		return nil, false
	}
	return withoutNil, true
}

type truthyDominatingBranchProofs struct {
	dom      *dominance.ImmediateDominators
	branches []truthyDominatingBranch
}

type truthyDominatingBranch struct {
	path          pathdom.Path
	trueSuccessor cfg.Point
}

func newTruthyDominatingBranchProofs(result *body.Result) truthyDominatingBranchProofs {
	if result == nil {
		return truthyDominatingBranchProofs{}
	}
	graph := result.Graph()
	if graph == nil {
		return truthyDominatingBranchProofs{}
	}
	proofs := truthyDominatingBranchProofs{
		dom: dominance.ComputeImmediateDominatorInfo(graph),
	}
	for _, branch := range cfg.RPOReadOnly(graph) {
		fact, ok := result.BranchCondition(branch)
		if !ok || fact.Check.Kind != branchcond.CheckTruthy || fact.Check.Path.IsEmpty() {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if ok && cond {
				// A successor reached by both the true edge and another edge is a
				// join, not evidence that the true branch executed. The successor
				// may dominate a later point even when the condition was false.
				if len(cfg.PredecessorsReadOnly(graph, succ)) != 1 {
					continue
				}
				proofs.branches = append(proofs.branches, truthyDominatingBranch{
					path:          fact.Check.Path.Clone(),
					trueSuccessor: succ,
				})
			}
		}
	}
	return proofs
}

func (p truthyDominatingBranchProofs) provesPresent(point cfg.Point, accessPath pathdom.Path) bool {
	if p.dom == nil || point == 0 || accessPath.IsEmpty() {
		return false
	}
	for _, branch := range p.branches {
		if branch.path.Equal(accessPath) && p.dom.Dominates(branch.trueSuccessor, point) {
			return true
		}
	}
	return false
}

func (p truthyDominatingBranchProofs) provesPresentWithoutInvalidation(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, accessPath pathdom.Path) bool {
	if p.dom == nil || result == nil || point == 0 || accessPath.IsEmpty() {
		return false
	}
	for _, branch := range p.branches {
		if !branch.path.Equal(accessPath) || !p.dom.Dominates(branch.trueSuccessor, point) {
			continue
		}
		if !pathReassignedBetween(result, flow, branch.trueSuccessor, point, accessPath) {
			return true
		}
	}
	return false
}

func pathReassignedBetween(result *body.Result, flow *diagnosticFlowCache, from, to cfg.Point, target pathdom.Path) bool {
	if result == nil || target.IsEmpty() || from == 0 || to == 0 {
		return false
	}
	graph := result.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == from {
			continue
		}
		if !diagnosticCanReach(flow, graph, from, candidate) || !diagnosticCanReach(flow, graph, candidate, to) {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok {
			continue
		}
		if fact.HasSymbol && fact.Symbol != 0 && fact.Symbol == target.Symbol {
			return true
		}
		if fact.Target != nil {
			if targetPath, ok := result.ExpressionPath(fact.Target); ok && pathsOverlapIgnoringVersion(targetPath, target) {
				return true
			}
		}
		if !fact.HasPath {
			continue
		}
		if pathsOverlapIgnoringVersion(fact.Path, target) {
			return true
		}
	}
	return false
}

func pathsOverlapIgnoringVersion(left, right pathdom.Path) bool {
	if !samePathRootIgnoringVersion(left, right) {
		return false
	}
	return segmentsHavePrefixForConcat(left.Segments, right.Segments) ||
		segmentsHavePrefixForConcat(right.Segments, left.Segments)
}

func samePathRootIgnoringVersion(left, right pathdom.Path) bool {
	if left.Symbol != 0 || right.Symbol != 0 {
		return left.Symbol != 0 && left.Symbol == right.Symbol
	}
	return left.Root != "" && left.Root == right.Root
}

func segmentsHavePrefixForConcat(candidate, prefix []segment.Segment) bool {
	if len(prefix) > len(candidate) {
		return false
	}
	for i, seg := range prefix {
		if candidate[i] != seg {
			return false
		}
	}
	return true
}

func (c concatOperandCheck) rootIdentifierBoundaryType(operand ast.Expr) (typ.Type, bool) {
	if c.result == nil {
		return nil, false
	}
	if _, ok := operand.(*ast.IdentExpr); !ok {
		return nil, false
	}
	accessPath, ok := c.query.ExpressionPath(operand)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return nil, false
	}
	value, ok := c.query.ExpressionValueAtBoundary(c.point, operand)
	if !ok {
		return nil, false
	}
	return c.query.ValueTypeWithPresence(value)
}

func (c concatOperandCheck) presentGuardOperandType(operand ast.Expr, t typ.Type) (typ.Type, bool) {
	if c.result == nil || operand == nil || t == nil {
		return nil, false
	}
	accessPath, ok := c.query.ExpressionPath(operand)
	if !ok || accessPath.IsEmpty() || !c.typer.env.hasPresent(accessPath) {
		return nil, false
	}
	withoutNil := projectionWithoutNil(t)
	if withoutNil == nil || typ.IsNever(withoutNil) {
		return nil, false
	}
	return withoutNil, true
}

func (c concatOperandCheck) dominatingLocalDeclarationType(operand ast.Expr) (typ.Type, bool) {
	if c.result == nil || operand == nil {
		return nil, false
	}
	accessPath, ok := c.query.ExpressionPath(operand)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return nil, false
	}
	fact, declarationPoint, ok := dominatingRootLocalAssignment(c.result, c.flow, c.point, accessPath.Symbol)
	if !ok {
		return nil, false
	}
	return c.localDeclarationType(declarationPoint, fact)
}

func (c concatOperandCheck) localDeclarationType(point cfg.Point, fact semantics.LocalAssignmentFact) (typ.Type, bool) {
	if fact.Type != nil {
		if lowered, ok := lowerType(fact.Type, c.typer.resolver); ok {
			return transparentComparableType(c.result, lowered), true
		}
	}
	if fact.Expr != nil {
		if got, ok := newExpressionTyper(c.result, c.typer.resolver).typeOf(fact.Expr); ok {
			if projectionHasNil(got) {
				if tightened, ok := c.logicalOrPresentFallbackType(fact.Expr); ok {
					return tightened, true
				}
			}
			return got, true
		}
		if got, ok := newFlowExpressionTyper(c.result, c.typer.resolver, point, c.envs[point]).typeOf(fact.Expr); ok {
			if projectionHasNil(got) {
				if tightened, ok := c.logicalOrPresentFallbackType(fact.Expr); ok {
					return tightened, true
				}
				if tightened, ok := newFlowExpressionTyper(c.result, c.typer.resolver, c.point, c.typer.env).typeOf(fact.Expr); ok && !projectionHasNil(tightened) {
					return tightened, true
				}
			}
			return got, true
		}
	}
	return c.query.SourceType(point, fact.Source)
}

func (c concatOperandCheck) logicalOrPresentFallbackType(expr ast.Expr) (typ.Type, bool) {
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok || logical.Operator != "or" {
		return nil, false
	}
	typer := newFlowExpressionTyper(c.result, c.typer.resolver, c.point, c.typer.env)
	right, ok := typer.typeOf(logical.Rhs)
	if !ok || projectionHasNil(right) {
		return nil, false
	}
	left, ok := typer.typeOf(logical.Lhs)
	if !ok {
		return nil, false
	}
	got, ok := typeoperator.BinaryOp(left, "or", right)
	if !ok || projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func concatOperandDiagnostic(operand ast.Expr, side string, got typ.Type) diagnostic.Diagnostic {
	operandSpan := ast.SpanOf(operand)
	operandName := exprEvidenceNameOK(operand)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     operandSpan,
		Code:     CodeConcatOperand,
		Severity: diagnostic.SeverityWarning,
		Message:  concatOperandMessage(side),
		Labels:   []diagnostic.Label{sourceLabel(operandSpan, labelValueMayBeNil)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Reason:  concatOperandEvidenceReason(got),
				Span:    operandSpan,
				Message: concatOperandTypeEvidence(side, operandName, got),
			},
		),
		Help: concatOperandHelp(operandName),
	})
}

func concatOperandEvidenceReason(got typ.Type) diagnostic.EvidenceReason {
	if got != nil && typ.Nil.Equals(got) {
		return diagnostic.EvidenceReasonExactType
	}
	if projectionHasNil(got) {
		return diagnostic.EvidenceReasonUnionType
	}
	return diagnostic.EvidenceReasonUnspecified
}
