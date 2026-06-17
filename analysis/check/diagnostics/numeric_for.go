package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// numericForOperands reports numeric-for operands that are statically known not
// to be numbers. Unknown, any, and partly numeric unions are left to runtime.
type numericForOperands producerContext

func (p numericForOperands) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	typer := newExpressionTyper(result, p.resolver)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.NumericFor(point)
		if !ok || fact.Role != cfgfacts.NumericForRoleInit {
			continue
		}
		if d, ok := numericForOperandDiagnostic(typer, fact.Init, "initial value"); ok {
			out = append(out, d)
		}
		if d, ok := numericForOperandDiagnostic(typer, fact.Limit, "limit"); ok {
			out = append(out, d)
		}
		if fact.Step != nil {
			if d, ok := numericForOperandDiagnostic(typer, fact.Step, "step"); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

func numericForOperandDiagnostic(typer expressionTyper, expr ast.Expr, role string) (diagnostic.Diagnostic, bool) {
	got, ok := typer.typeOf(expr)
	if !ok || !definitelyNotNumber(got) {
		return diagnostic.Diagnostic{}, false
	}
	span := ast.SpanOf(expr)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: fmt.Sprintf("%s is %s", role, formatType(got)),
		},
	}
	if _, ok := explicitTopLikeCastType(typer.resolver, expr); ok {
		evidence = append(evidence, explicitTopLikeCastEvidence(span, typ.Number, expr)...)
	}
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:        span,
		Code:        CodeNumericForOperand,
		Severity:    diagnostic.SeverityError,
		Message:     fmt.Sprintf("numeric for %s must be number, got %s", role, formatType(got)),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels: []diagnostic.Label{
			{Span: span, Message: "numeric for " + role},
		},
	}, true
}

func definitelyNotNumber(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if subtype.IsSubtype(t, typ.Number) {
		return false
	}
	return !mayContainNumber(t, 0)
}

func mayContainNumber(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	if typ.IsNever(t) {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	if subtype.IsSubtype(t, typ.Number) {
		return true
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return mayContainNumber(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return mayContainNumber(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if mayContainNumber(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded == nil || expanded == t || mayContainNumber(expanded, depth+1)
	default:
		switch v.Kind() {
		case kind.Nil, kind.Boolean, kind.String, kind.Function, kind.Array, kind.Map, kind.Record, kind.Tuple, kind.ReadonlyMap:
			return false
		case kind.Literal:
			return subtype.IsSubtype(v, typ.Number)
		default:
			return true
		}
	}
}
