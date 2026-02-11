package guard

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/typ"
)

// TruthyPathKey uniquely identifies a field path for truthy guard tracking.
type TruthyPathKey struct {
	Symbol cfg.SymbolID
	Field  string
}

// CollectTruthyGuards scans the CFG for conditions that establish truthy guards
// and propagates them to dominated points. Used to narrow optional types.
func CollectTruthyGuards(graph *cfg.Graph, bindings *bind.BindingTable) map[cfg.Point]map[TruthyPathKey]bool {
	if graph == nil || bindings == nil {
		return nil
	}

	result := make(map[cfg.Point]map[TruthyPathKey]bool)

	graph.EachBranch(func(branchPoint cfg.Point, info *cfg.BranchInfo) {
		if info == nil || info.Condition == nil {
			return
		}

		succs := graph.Successors(branchPoint)
		var trueEdge cfg.Point
		for _, s := range succs {
			if cond, ok := graph.EdgeCond(branchPoint, s); ok && cond {
				trueEdge = s
				break
			}
		}
		if trueEdge == 0 {
			return
		}

		keys := ExtractTruthyPathKeys(info.Condition, bindings)
		if len(keys) == 0 {
			return
		}

		propagateTruthyGuards(graph, trueEdge, keys, result)
	})

	return result
}

// ExtractTruthyPathKeys extracts path keys from expressions in truthy position.
func ExtractTruthyPathKeys(expr ast.Expr, bindings *bind.BindingTable) []TruthyPathKey {
	if expr == nil || bindings == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := bindings.SymbolOf(e); ok && sym != 0 {
			return []TruthyPathKey{{Symbol: sym, Field: ""}}
		}
	case *ast.AttrGetExpr:
		if sym, fieldPath, ok := callsite.FieldPathWithBaseSymbol(bindings, e); ok && sym != 0 && fieldPath != "" {
			return []TruthyPathKey{{Symbol: sym, Field: fieldPath}}
		}
	case *ast.LogicalOpExpr:
		if e.Operator == "and" {
			var keys []TruthyPathKey
			keys = append(keys, ExtractTruthyPathKeys(e.Lhs, bindings)...)
			keys = append(keys, ExtractTruthyPathKeys(e.Rhs, bindings)...)
			return keys
		}
	}
	return nil
}

// propagateTruthyGuards propagates truthy guards from a starting point to all reachable points
// until a join point (multiple predecessors from outside the region) is reached.
func propagateTruthyGuards(graph *cfg.Graph, start cfg.Point, keys []TruthyPathKey, result map[cfg.Point]map[TruthyPathKey]bool) {
	visited := make(map[cfg.Point]bool)
	worklist := []cfg.Point{start}

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]

		if visited[p] {
			continue
		}
		visited[p] = true

		if result[p] == nil {
			result[p] = make(map[TruthyPathKey]bool)
		}
		for _, key := range keys {
			result[p][key] = true
		}

		succs := graph.Successors(p)
		for _, succ := range succs {
			if !visited[succ] {
				preds := graph.Predecessors(succ)
				hasUnvisitedPred := false
				for _, pred := range preds {
					if !visited[pred] && pred != p {
						hasUnvisitedPred = true
						break
					}
				}
				if !hasUnvisitedPred {
					worklist = append(worklist, succ)
				}
			}
		}
	}
}

// NarrowTableFieldsByGuard narrows optional record fields using truthy guards.
// When a table literal like {from = event.from} is inside a truthy guard for
// event.from, the field type should be narrowed from string? to string.
func NarrowTableFieldsByGuard(
	recType typ.Type,
	tbl *ast.TableExpr,
	p cfg.Point,
	bindings *bind.BindingTable,
	truthyGuards map[cfg.Point]map[TruthyPathKey]bool,
) typ.Type {
	rec, ok := recType.(*typ.Record)
	if !ok || rec == nil || len(rec.Fields) == 0 {
		return recType
	}
	guards := truthyGuards[p]
	if guards == nil {
		return recType
	}

	fieldSources := make(map[string]ast.Expr)
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		var name string
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			name = k.Value
		case *ast.IdentExpr:
			name = k.Value
		}
		if name != "" {
			fieldSources[name] = field.Value
		}
	}

	changed := false
	newFields := make([]typ.Field, len(rec.Fields))
	copy(newFields, rec.Fields)

	for i, f := range newFields {
		opt, isOpt := f.Type.(*typ.Optional)
		if !isOpt {
			continue
		}
		srcExpr := fieldSources[f.Name]
		if srcExpr == nil {
			continue
		}
		attr, isAttr := srcExpr.(*ast.AttrGetExpr)
		if !isAttr {
			continue
		}
		sym, fieldPath, ok := callsite.FieldPathWithBaseSymbol(bindings, attr)
		if !ok || sym == 0 || fieldPath == "" {
			continue
		}
		key := TruthyPathKey{Symbol: sym, Field: fieldPath}
		if guards[key] {
			newFields[i].Type = opt.Inner
			changed = true
		}
	}

	if !changed {
		return recType
	}

	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range newFields {
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	return builder.Build()
}
