// return_check.go implements return statement validation for the type checker.
//
// This pass validates that return statements match their function's declared
// return type annotations. Functions without return type annotations are not
// validated (their return types are inferred instead).
//
// # VALIDATION
//
// For each return statement in the function body, the pass:
//  1. Resolves the function's declared return types from annotations
//  2. Synthesizes types for each return expression (with contextual typing)
//  3. Checks subtyping: actual <: declared for each position
//
// Example:
//
//	function foo(): number
//	    return "hello"  -- ERROR: cannot return string, expected number
//	end
//
// # MULTIPLE RETURNS
//
// Lua supports multiple return values. Each position is validated independently:
//
//	function bar(): number, string
//	    return 1, 2  -- ERROR at position 2: cannot return number, expected string
//	end
//
// # CONTEXTUAL TYPING
//
// The expected return type is used for contextual typing of return expressions,
// enabling table literals to be checked against expected record types.
package hooks

import (
	"strconv"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

type synthForReturn interface {
	TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type
}

// CheckReturns validates return statements against declared return types.
func CheckReturns(
	fn *ast.FunctionExpr,
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	baseScope *scope.State,
	declared api.Synth,
	narrowView api.BaseSynth,
	sourceName string,
) []diag.Diagnostic {
	if fn == nil || graph == nil {
		return nil
	}

	if len(fn.ReturnTypes) == 0 {
		return nil
	}

	declaredReturns := resolveDeclaredReturns(fn.ReturnTypes, baseScope, declared)
	if len(declaredReturns) == 0 {
		return nil
	}

	var synthToUse synthForReturn
	if narrowView != nil {
		synthToUse = narrowView
	} else if declared != nil {
		synthToUse = declared
	} else {
		return nil
	}

	var diags []diag.Diagnostic

	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		if len(info.Exprs) == 0 {
			var posNode ast.PositionHolder
			if fn != nil {
				posNode = fn
			}
			if info.Stmt != nil {
				posNode = info.Stmt
			}
			if posNode != nil {
				pos := diag.Position{File: sourceName, Line: posNode.Line(), Column: posNode.Column()}
				span := ast.SpanOf(posNode)
				msg := "missing return"
				_, help := diag.ContextualHelp(diag.ErrMissingReturn, msg, "")
				diags = append(diags, diag.Diagnostic{
					Severity: diag.SeverityError,
					Code:     diag.ErrMissingReturn,
					Position: pos,
					Span:     span,
					Message:  msg,
					Help:     help,
				})
			}
			return
		}

		for i, expr := range info.Exprs {
			if i >= len(declaredReturns) {
				break
			}

			declaredType := declaredReturns[i]
			if declaredType == nil {
				continue
			}

			actual := synthToUse.TypeOfWithExpected(expr, p, declaredType)
			if actual == nil {
				actual = typ.Unknown
			}

			if !subtype.IsSubtype(actual, declaredType) {
				pos := diag.Position{File: sourceName, Line: expr.Line(), Column: expr.Column()}
				span := ast.SpanOf(expr)
				msg := formatReturnMismatch(actual, declaredType, i)
				_, help := diag.ContextualHelp(diag.ErrTypeMismatch, msg, "")
				diags = append(diags, diag.Diagnostic{
					Severity: diag.SeverityError,
					Code:     diag.ErrTypeMismatch,
					Position: pos,
					Span:     span,
					Message:  msg,
					Help:     help,
				})
			}
		}
	})

	return diags
}

func resolveDeclaredReturns(returnTypes []ast.TypeExpr, baseScope *scope.State, s api.Synth) []typ.Type {
	if s == nil {
		return nil
	}
	result := make([]typ.Type, 0, len(returnTypes))
	for _, rt := range returnTypes {
		if rt == nil {
			result = append(result, nil)
			continue
		}
		resolved := s.ResolveType(rt, baseScope)
		result = append(result, resolved)
	}
	return result
}

func formatReturnMismatch(actual, declared typ.Type, index int) string {
	if index == 0 {
		return "cannot return " + typ.FormatShort(actual) + ", expected " + typ.FormatShort(declared)
	}
	return "cannot return " + typ.FormatShort(actual) + " at position " + strconv.Itoa(index+1) + ", expected " + typ.FormatShort(declared)
}
