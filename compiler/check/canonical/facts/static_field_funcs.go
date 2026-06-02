package facts

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
)

// collectFieldFunctions extracts statically named function values stored in table
// fields. It covers both field definitions (`function M.f()`) and table literals
// (`local M = { f = function() end }`). Dynamic keys and non-function fields are
// runtime flow facts, so they are intentionally ignored here.
func collectFieldFunctions(p Program) []topology.FieldFunction {
	if p.RefForFuncSymbol == nil {
		return nil
	}
	var out []topology.FieldFunction
	order := 0
	add := func(container cfg.SymbolID, field fieldkey.Key, sym cfg.SymbolID) {
		if container == 0 || field == (fieldkey.Key{}) || sym == 0 {
			return
		}
		r, ok := p.RefForFuncSymbol(sym)
		if !ok {
			return
		}
		out = append(out, topology.FieldFunction{
			ContainerSym: container,
			Field:        field,
			FuncRef:      r,
			Order:        order,
		})
		order++
	}
	for _, owner := range p.Refs {
		g := graphOf(p, owner)
		if g == nil {
			continue
		}
		g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil {
				return
			}
			if base, key, ok := fieldFuncDefinition(info); ok {
				add(base, key, info.Symbol)
			}
		})
		g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
			if info == nil {
				return
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					return
				}
				table, ok := src.(*ast.TableExpr)
				if !ok {
					return
				}
				for _, field := range table.Fields {
					if field == nil || field.Key == nil {
						continue
					}
					key, ok := tableLiteralFieldKey(field)
					if !ok {
						continue
					}
					fn, ok := field.Value.(*ast.FunctionExpr)
					if !ok {
						continue
					}
					add(target.Symbol, key, funcLiteralSymbol(g, fn))
				}
			})
		})
	}
	return out
}

func funcLiteralSymbol(g *cfg.Graph, fn *ast.FunctionExpr) cfg.SymbolID {
	if g == nil || fn == nil || g.Bindings() == nil {
		return 0
	}
	sym, ok := g.Bindings().FuncLitSymbol(fn)
	if !ok {
		return 0
	}
	return sym
}

func fieldFuncDefinition(info *cfg.FuncDefInfo) (cfg.SymbolID, fieldkey.Key, bool) {
	if info == nil {
		return 0, fieldkey.Key{}, false
	}
	base := info.TargetPath.Symbol
	if base == 0 || len(info.TargetPath.Segments) != 1 {
		return 0, fieldkey.Key{}, false
	}
	seg := info.TargetPath.Segments[0]
	key, ok := fieldkey.FromSegment(seg)
	if !ok {
		return 0, fieldkey.Key{}, false
	}
	return base, key, true
}

// tableLiteralFieldKey resolves a static table-literal field key to a structural
// field key, or false for dynamic/positional keys.
func tableLiteralFieldKey(field *ast.Field) (fieldkey.Key, bool) {
	return fieldkey.FromTableField(field)
}
