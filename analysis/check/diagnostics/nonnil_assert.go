package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// nonNilAssertions reports `expr!` assertions whose operand is provably nil on
// every path reaching the assertion. A non-nil assertion is a runtime check that
// errors when the operand is nil; if the operand can only be nil, the check
// always fails, so the assertion is a guaranteed runtime error. Flagging it
// statically keeps a value the checker would otherwise treat as non-nil (the
// assertion's result projects nil away) from reaching the JIT.
type nonNilAssertions producerContext

func (p nonNilAssertions) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	seen := make(map[nonNilAssertSeenKey]struct{})
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		check := nonNilAssertCheck{
			result: result,
			typer:  newFlowExpressionTyper(result, p.resolver, point, envs[point]),
			point:  point,
		}
		emit := func(expr ast.Expr) {
			p.walk(expr, check, seen, &out, 0)
		}
		if fact, ok := result.LocalAssignment(point); ok {
			for _, expr := range fact.Exprs {
				emit(expr)
			}
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

func (p nonNilAssertions) walk(expr ast.Expr, check nonNilAssertCheck, seen map[nonNilAssertSeenKey]struct{}, out *[]diagnostic.Diagnostic, depth int) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	walkChild := func(child ast.Expr) {
		p.walk(child, check, seen, out, depth+1)
	}
	if e, ok := expr.(*ast.NonNilAssertExpr); ok {
		walkExprChildren(expr, walkChild)
		key := nonNilAssertSeenKey{expr: e, point: check.point}
		if _, done := seen[key]; done {
			return
		}
		seen[key] = struct{}{}
		if d, ok := check.diagnostic(e); ok {
			*out = append(*out, d)
		}
		return
	}
	walkExprChildren(expr, walkChild)
}

type nonNilAssertCheck struct {
	result *body.Result
	typer  expressionTyper
	point  cfg.Point
}

type nonNilAssertSeenKey struct {
	expr  *ast.NonNilAssertExpr
	point cfg.Point
}

func (c nonNilAssertCheck) diagnostic(assert *ast.NonNilAssertExpr) (diagnostic.Diagnostic, bool) {
	operand := assert.Expr
	if operand == nil {
		return diagnostic.Diagnostic{}, false
	}
	t, ok := c.operandType(operand)
	if !ok || !provablyNilOnly(t) {
		return diagnostic.Diagnostic{}, false
	}
	return nonNilAssertDiagnostic(operand, t), true
}

// operandType returns the most precise flow type of the assertion operand at the
// assertion point. The solved boundary value reflects narrowing the analysis has
// proven (e.g. the nil branch of `x == nil`), so it is consulted first; the
// declared/flow typer covers operands without a boundary value slot.
func (c nonNilAssertCheck) operandType(operand ast.Expr) (typ.Type, bool) {
	if c.result != nil {
		if value, ok := c.result.ExpressionValueAtBoundary(c.point, operand); ok {
			if t, ok := readmodel.New(c.result).ValueTypeWithPresence(value); ok && t != nil {
				return t, true
			}
		}
	}
	return c.typer.typeOf(operand)
}

// provablyNilOnly reports whether t admits no value other than nil. any and
// unknown carry non-nil inhabitants, and never is dead code rather than a nil
// proof, so all three are excluded.
func provablyNilOnly(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if !projectionHasNil(t) {
		return false
	}
	withoutNil := projectionWithoutNil(t)
	return withoutNil == nil || typ.IsNever(withoutNil)
}

func nonNilAssertDiagnostic(operand ast.Expr, got typ.Type) diagnostic.Diagnostic {
	span := ast.SpanOf(operand)
	name := exprEvidenceNameOK(operand)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeNonNilAssertAlwaysNil,
		Severity: diagnostic.SeverityError,
		Message:  nonNilAssertAlwaysNilMessage(name),
		Labels:   []diagnostic.Label{sourceLabel(span, labelValueAlwaysNil)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: nonNilAssertAlwaysNilEvidence(name),
			},
		),
		Help: nonNilAssertAlwaysNilHelp(name),
	})
}
