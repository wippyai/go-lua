package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectFieldAssignments scans the graph for field assignments and groups them by base symbol.
// Returns a map: symbolID -> map[fieldName]typ.Type representing fields assigned to each symbol.
// The synth function is used to synthesize field value types.
// If filterSyms is non-nil, only symbols in the filter are collected.
func CollectFieldAssignments(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
	if graph == nil {
		return result
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		sources := info.Sources
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(sources) {
				source = sources[i]
			}
			var sym cfg.SymbolID
			var fieldName string

			switch target.Kind {
			case cfg.TargetField:
				if target.BaseSymbol != 0 && len(target.FieldPath) == 1 {
					sym = target.BaseSymbol
					fieldName = target.FieldPath[0]
				}
			case cfg.TargetIndex:
				if target.BaseSymbol != 0 && target.Key != nil {
					if strKey, ok := target.Key.(*ast.StringExpr); ok && strKey.Value != "" {
						sym = target.BaseSymbol
						fieldName = strKey.Value
					}
				}
			}

			if sym == 0 || fieldName == "" {
				continue
			}
			if filterSyms != nil && !filterSyms[sym] {
				continue
			}

			var fieldType typ.Type
			if source != nil && synth != nil {
				fieldType = synth(source, p)
			}
			if fieldType == nil {
				fieldType = typ.Unknown
			}

			if result[sym] == nil {
				result[sym] = make(map[string]typ.Type)
			}
			if existing := result[sym][fieldName]; existing != nil {
				result[sym][fieldName] = typ.NewUnion(existing, fieldType)
			} else {
				result[sym][fieldName] = fieldType
			}
		}
	})

	return result
}

// CollectIndexerAssignments scans the graph for dynamic index assignments (t[k] = v where k is non-const).
// Returns a map: symbolID -> []IndexerInfo representing index assignments to each symbol.
func CollectIndexerAssignments(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID][]mutator.IndexerInfo {
	result := make(map[cfg.SymbolID][]mutator.IndexerInfo)
	if graph == nil {
		return result
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		sources := info.Sources
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(sources) {
				source = sources[i]
			}
			if target.Kind != cfg.TargetIndex {
				continue
			}
			sym := target.BaseSymbol
			if sym == 0 {
				continue
			}
			if filterSyms != nil && !filterSyms[sym] {
				continue
			}

			// Skip string literal keys (handled by field assignments)
			if _, ok := target.Key.(*ast.StringExpr); ok {
				continue
			}

			result[sym] = append(result[sym], mutator.IndexerInfo{
				KeyType: dynamicKeyType(target.Key, p, synth),
				ValType: assignedValueType(source, p, synth),
			})
		}
	})

	return result
}

// CollectNestedFieldWrites scans the graph for writes into tables that
// targets reach by static fields: t.a.x = v, t.a["x"] = v and t.a[k] = v.
// Writes to the target table itself are collected by CollectFieldAssignments
// and CollectIndexerAssignments. Writes by dynamic keys are recorded under
// flow.IndexerWriteField as the map {[K]: V} they add.
func CollectNestedFieldWrites(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	targets map[cfg.SymbolID]bool,
) map[cfg.SymbolID]api.FieldWriteSet {
	result := make(map[cfg.SymbolID]api.FieldWriteSet)
	if graph == nil || len(targets) == 0 {
		return result
	}
	add := func(sym cfg.SymbolID, key api.FieldWriteKey, t typ.Type) {
		set := result[sym]
		if set == nil {
			set = make(api.FieldWriteSet)
			result[sym] = set
		}
		set[key] = api.JoinFieldWrite(key, set[key], t)
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(info.Sources) {
				source = info.Sources[i]
			}
			switch target.Kind {
			case cfg.TargetField:
				n := len(target.FieldPath)
				if n < 2 || !targets[target.BaseSymbol] {
					continue
				}
				segments := make([]constraint.Segment, n-1)
				for j, name := range target.FieldPath[:n-1] {
					segments[j] = constraint.Segment{Kind: constraint.SegmentField, Name: name}
				}
				add(target.BaseSymbol, api.NewFieldWriteKey(segments, target.FieldPath[n-1]), assignedValueType(source, p, synth))
			case cfg.TargetIndex:
				if target.Base == nil || target.Key == nil {
					continue
				}
				base := flowpath.FromExprWithBindings(target.Base, nil, bindings)
				if len(base.Segments) == 0 || !targets[base.Symbol] {
					continue
				}
				if strKey, ok := target.Key.(*ast.StringExpr); ok {
					if strKey.Value != "" {
						add(base.Symbol, api.NewFieldWriteKey(base.Segments, strKey.Value), assignedValueType(source, p, synth))
					}
					continue
				}
				written := typ.NewMap(dynamicKeyType(target.Key, p, synth), assignedValueType(source, p, synth))
				add(base.Symbol, api.NewFieldWriteKey(base.Segments, flow.IndexerWriteField), written)
			}
		}
	})

	return result
}

// dynamicKeyType returns the type of the dynamic key of an index write at p.
func dynamicKeyType(key ast.Expr, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	var keyType typ.Type
	switch k := key.(type) {
	case *ast.NumberExpr:
		keyType = typ.Integer
	default:
		if synth != nil && k != nil {
			keyType = synth(k, p)
		}
	}
	if keyType == nil || keyType.Kind().IsPlaceholder() {
		return typ.String
	}
	return keyType
}

// assignedValueType returns the type of the value source assigns at p.
func assignedValueType(source ast.Expr, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	if source != nil && synth != nil {
		if t := synth(source, p); t != nil {
			return t
		}
	}
	return typ.Unknown
}
