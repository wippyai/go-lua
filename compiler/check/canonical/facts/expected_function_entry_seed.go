package facts

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/signature"
	"github.com/wippyai/go-lua/types/typ"
)

// collectExpectedFunctionEntrySeeds projects source-declared expected callable
// types into entry values for returned module-local function literals. This is a
// forward contextual fact: it lets the nested body run under the API contract of
// the expression position without treating those contextual parameters as source
// annotations.
func collectExpectedFunctionEntrySeeds(p Program) []entrySeedRow {
	if p.RefForFuncSymbol == nil || p.DeclaredReturnTypes == nil {
		return nil
	}
	var out []entrySeedRow
	order := 0
	for _, owner := range p.Refs {
		g := graphOf(p, owner)
		if g == nil {
			continue
		}
		returns := p.DeclaredReturnTypes(owner)
		if len(returns) == 0 {
			continue
		}
		g.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
			if info == nil {
				return
			}
			for i, expr := range info.Exprs {
				if i >= len(returns) {
					continue
				}
				expected := returns[i]
				if expected == nil || typ.IsAbsentOrUnknown(expected) {
					continue
				}
				r, fn, ok := returnedFunctionLiteral(p, g, expr)
				if !ok {
					continue
				}
				expectedFn := signature.ExpectedFunctionLiteralSignature(fn, expected)
				if expectedFn == nil {
					continue
				}
				for slot, param := range expectedFn.Params {
					if param.Type == nil || typ.ContainsTypeParam(param.Type) {
						continue
					}
					out = append(out, entrySeedRow{
						FuncRef: r,
						Seed: FunctionEntrySeed{
							Slot: slot,
							Type: param.Type,
						},
						Order: order,
					})
					order++
				}
			}
		})
	}
	return out
}

func returnedFunctionLiteral(p Program, g *cfg.Graph, expr ast.Expr) (ref.FuncRef, *ast.FunctionExpr, bool) {
	if expr == nil || g == nil || p.RefForFuncSymbol == nil {
		return ref.FuncRef{}, nil, false
	}
	bindings := g.Bindings()
	raw := callsite.SymbolFromExpr(expr, bindings)
	sym := callsite.CanonicalSymbolFromExprWithAliases(
		expr,
		raw,
		g,
		bindings,
		bindings,
		func(candidate cfg.SymbolID) bool {
			_, ok := p.RefForFuncSymbol(candidate)
			return ok
		},
	)
	r, ok := p.RefForFuncSymbol(sym)
	if !ok {
		return ref.FuncRef{}, nil, false
	}
	if child := graphOf(p, r); child != nil && child.Func() != nil {
		return r, child.Func(), true
	}
	fn, ok := expr.(*ast.FunctionExpr)
	if !ok || fn == nil {
		return ref.FuncRef{}, nil, false
	}
	return r, fn, true
}
