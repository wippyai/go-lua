package facts

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/literal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// collectEntrySelfSeeds extracts declaration-context self values for table-owned
// function fields. These are forward entry values, not annotations: exact call
// contexts can still override them through the summary key.
func collectEntrySelfSeeds(p Program) []entrySelfSeedRow {
	if p.RefForFuncSymbol == nil {
		return nil
	}
	receiverTypes := make(map[cfg.SymbolID]typ.Type)
	var out []entrySelfSeedRow
	order := 0
	add := func(fn *ast.FunctionExpr, g *cfg.Graph, selfType typ.Type) {
		if fn == nil || g == nil || selfType == nil || typ.IsAbsentOrUnknown(selfType) {
			return
		}
		if !hasUnannotatedSelfParam(fn) {
			return
		}
		sym := funcLiteralSymbol(g, fn)
		if sym == 0 {
			return
		}
		r, ok := p.RefForFuncSymbol(sym)
		if !ok {
			return
		}
		out = append(out, entrySelfSeedRow{
			FuncRef: r,
			Seed: FunctionEntrySeed{
				Slot: 0,
				Type: selfType,
			},
			Order: order,
		})
		order++
	}

	for _, owner := range p.Refs {
		g := graphOf(p, owner)
		if g == nil {
			continue
		}
		assignments := assignmentsByPoint(g)
		for _, point := range g.RPO() {
			info := assignments[point]
			if info == nil {
				continue
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
				if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
					table, ok := src.(*ast.TableExpr)
					if !ok {
						return
					}
					selfType := tableLiteralSelfType(table)
					if selfType == nil {
						return
					}
					receiverTypes[target.Symbol] = selfType
					for _, field := range table.Fields {
						if field == nil || field.Key == nil {
							continue
						}
						fn, ok := field.Value.(*ast.FunctionExpr)
						if !ok {
							continue
						}
						add(fn, g, selfType)
					}
					return
				}
				if target.Kind != cfg.TargetField || target.BaseSymbol == 0 {
					return
				}
				fn, ok := src.(*ast.FunctionExpr)
				if !ok {
					return
				}
				add(fn, g, receiverTypes[target.BaseSymbol])
			})
		}
	}
	return out
}

func assignmentsByPoint(g *cfg.Graph) map[cfg.Point]*cfg.AssignInfo {
	assignments := make(map[cfg.Point]*cfg.AssignInfo)
	g.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		assignments[p] = info
	})
	return assignments
}

func hasUnannotatedSelfParam(fn *ast.FunctionExpr) bool {
	if fn == nil || fn.ParList == nil || len(fn.ParList.Names) == 0 || fn.ParList.Names[0] != "self" {
		return false
	}
	return len(fn.ParList.Types) == 0 || fn.ParList.Types[0] == nil
}

func tableLiteralSelfType(table *ast.TableExpr) typ.Type {
	if table == nil {
		return nil
	}
	builder := typ.NewRecord()
	fields := 0
	for _, field := range table.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		if _, ok := field.Value.(*ast.FunctionExpr); ok {
			continue
		}
		name, ok := fieldkey.RecordFieldNameFromTableField(field)
		if !ok {
			continue
		}
		builder.Field(name, receiverFieldType(field.Value))
		fields++
	}
	if fields == 0 {
		return nil
	}
	return builder.Build()
}

func receiverFieldType(expr ast.Expr) typ.Type {
	switch e := expr.(type) {
	case nil:
		return typ.Unknown
	case *ast.NilExpr:
		return typ.Unknown
	case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.FalseExpr:
		if lit, ok := literal.FromExpr(expr); ok {
			return widenReceiverLiteral(lit)
		}
		return typ.Unknown
	case *ast.TableExpr:
		if t := tableLiteralSelfType(e); t != nil {
			return t
		}
		return typ.NewRecord().Build()
	default:
		return typ.Unknown
	}
}

func widenReceiverLiteral(t typ.Type) typ.Type {
	lit, ok := t.(*typ.Literal)
	if !ok {
		return t
	}
	switch lit.Base {
	case kind.Boolean:
		return typ.Boolean
	case kind.String:
		return typ.String
	case kind.Integer:
		return typ.Integer
	case kind.Number:
		return typ.Number
	default:
		return t
	}
}
