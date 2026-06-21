package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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
	envs := cachedGuardEnvironments(result)
	truthyBranches := newTruthyDominatingBranchProofs(result)
	var out []diagnostic.Diagnostic
	seen := make(map[concatSeenKey]struct{})
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		check := concatOperandCheck{
			result: result,
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
	switch e := expr.(type) {
	case *ast.StringConcatOpExpr:
		p.walk(e.Lhs, check, seen, out, depth+1)
		p.walk(e.Rhs, check, seen, out, depth+1)
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
	case *ast.AttrGetExpr:
		p.walk(e.Object, check, seen, out, depth+1)
		if e.KeySyntax == ast.AttrKeyIndex {
			p.walk(e.Key, check, seen, out, depth+1)
		}
	case *ast.FuncCallExpr:
		p.walk(e.Func, check, seen, out, depth+1)
		p.walk(e.Receiver, check, seen, out, depth+1)
		for _, arg := range e.Args {
			p.walk(arg, check, seen, out, depth+1)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				p.walk(field.Key, check, seen, out, depth+1)
			}
			p.walk(field.Value, check, seen, out, depth+1)
		}
	case *ast.LogicalOpExpr:
		p.walk(e.Lhs, check, seen, out, depth+1)
		p.walk(e.Rhs, check, seen, out, depth+1)
	case *ast.RelationalOpExpr:
		p.walk(e.Lhs, check, seen, out, depth+1)
		p.walk(e.Rhs, check, seen, out, depth+1)
	case *ast.ArithmeticOpExpr:
		p.walk(e.Lhs, check, seen, out, depth+1)
		p.walk(e.Rhs, check, seen, out, depth+1)
	case *ast.UnaryMinusOpExpr:
		p.walk(e.Expr, check, seen, out, depth+1)
	case *ast.UnaryNotOpExpr:
		p.walk(e.Expr, check, seen, out, depth+1)
	case *ast.UnaryLenOpExpr:
		p.walk(e.Expr, check, seen, out, depth+1)
	case *ast.UnaryBNotOpExpr:
		p.walk(e.Expr, check, seen, out, depth+1)
	case *ast.CastExpr:
		p.walk(e.Expr, check, seen, out, depth+1)
	case *ast.NonNilAssertExpr:
		p.walk(e.Expr, check, seen, out, depth+1)
	}
}

type concatOperandCheck struct {
	result *body.Result
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

func (c concatOperandCheck) operandType(operand ast.Expr) (typ.Type, bool) {
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
	value, ok := c.result.ExpressionValueAtBoundary(c.point, operand)
	if !ok {
		return nil, false
	}
	return readmodel.New(c.result).ValueTypeWithPresence(value)
}

func (c concatOperandCheck) truthyDominatingBranchOperandType(operand ast.Expr, t typ.Type) (typ.Type, bool) {
	if c.result == nil || operand == nil || t == nil {
		return nil, false
	}
	accessPath, ok := c.result.ExpressionPath(operand)
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
	for _, branch := range graph.RPO() {
		fact, ok := result.BranchCondition(branch)
		if !ok || fact.Check.Kind != branchcond.CheckTruthy || fact.Check.Path.IsEmpty() {
			continue
		}
		for _, succ := range graph.Successors(branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if ok && cond {
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

func (c concatOperandCheck) rootIdentifierBoundaryType(operand ast.Expr) (typ.Type, bool) {
	if c.result == nil {
		return nil, false
	}
	if _, ok := operand.(*ast.IdentExpr); !ok {
		return nil, false
	}
	accessPath, ok := c.result.ExpressionPath(operand)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return nil, false
	}
	value, ok := c.result.ExpressionValueAtBoundary(c.point, operand)
	if !ok {
		return nil, false
	}
	return readmodel.New(c.result).ValueTypeWithPresence(value)
}

func (c concatOperandCheck) presentGuardOperandType(operand ast.Expr, t typ.Type) (typ.Type, bool) {
	if c.result == nil || operand == nil || t == nil {
		return nil, false
	}
	accessPath, ok := c.result.ExpressionPath(operand)
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
	accessPath, ok := c.result.ExpressionPath(operand)
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
	return readmodel.New(c.result).SourceType(point, fact.Source)
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
				Span:    operandSpan,
				Message: concatOperandTypeEvidence(side, operandName, got),
			},
		),
		Help: concatOperandHelp(operandName),
	})
}
