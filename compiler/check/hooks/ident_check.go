// ident_check.go implements undefined variable detection for the type checker.
//
// This pass validates that all identifier expressions refer to defined variables
// at their use point. It catches "undefined variable 'x'" errors.
//
// # RESOLUTION ORDER
//
// For each identifier, the checker attempts resolution in order:
//  1. Binding table lookup (local variables from CFG)
//  2. Type name lookup (type definitions in scope)
//
// If all lookups fail, the identifier is undefined.
//
// # SPECIAL CASES
//
// The underscore "_" identifier is always allowed (discard pattern).
// Type names are recognized via scope.LookupType to avoid false positives
// when types are used as values (e.g., metatables).
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckIdents validates that all identifier expressions are defined at their use point.
func CheckIdents(graph *cfg.Graph, evidence api.FlowEvidence, scopes map[cfg.Point]*scope.State, declared map[cfg.SymbolID]typ.Type, sourceName string) []diag.Diagnostic {
	if graph == nil {
		return nil
	}

	var diags []diag.Diagnostic
	checker := &identChecker{
		graph:      graph,
		scopes:     scopes,
		declared:   declared,
		sourceName: sourceName,
		diags:      &diags,
	}

	for _, use := range evidence.IdentifierUses {
		if use.Expr == nil {
			continue
		}
		checker.point = use.Point
		checker.scope = scopes[use.Point]
		checker.checkIdent(use.Expr)
	}

	return diags
}

type identChecker struct {
	graph      *cfg.Graph
	scopes     map[cfg.Point]*scope.State
	declared   map[cfg.SymbolID]typ.Type
	sourceName string
	point      cfg.Point
	scope      *scope.State
	diags      *[]diag.Diagnostic
}

func (c *identChecker) checkIdent(ident *ast.IdentExpr) {
	if ident == nil || ident.Value == "" {
		return
	}
	if ident.Value == "_" {
		return
	}

	if c.scope != nil {
		if _, isType := c.scope.LookupType(ident.Value); isType {
			return
		}
	}

	if bindings := c.graph.Bindings(); bindings != nil {
		if sym, ok := bindings.SymbolOf(ident); ok {
			if kind, found := bindings.Kind(sym); found && kind == cfg.SymbolGlobal {
				if c.declared[sym] != nil {
					return
				}
			} else if !bindings.IsImplicitGlobalUse(ident) {
				return
			}
		}
	}

	pos := diag.Position{
		File:   c.sourceName,
		Line:   ident.Line(),
		Column: ident.Column(),
	}
	*c.diags = append(*c.diags, diag.Diagnostic{
		Severity: diag.SeverityError,
		Code:     diag.ErrUndefined,
		Position: pos,
		Span:     ast.SpanOf(ident),
		Message:  "undefined variable '" + ident.Value + "'",
	})
}
