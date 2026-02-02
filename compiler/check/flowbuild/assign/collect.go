package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
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
		for i, target := range info.Targets {
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
			if i < len(info.Sources) && info.Sources[i] != nil && synth != nil {
				fieldType = synth(info.Sources[i], p)
			}
			if fieldType == nil {
				fieldType = typ.Unknown
			}

			if result[sym] == nil {
				result[sym] = make(map[string]typ.Type)
			}
			if existing := result[sym][fieldName]; existing != nil {
				result[sym][fieldName] = join.Two(existing, fieldType)
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
		for i, target := range info.Targets {
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

			// Determine key type
			var keyType typ.Type
			switch k := target.Key.(type) {
			case *ast.IdentExpr:
				if synth != nil {
					keyType = synth(k, p)
				}
				if keyType == nil && bindings != nil {
					if keySym, found := bindings.SymbolOf(k); found && keySym != 0 {
						keyType = typ.String
					}
				}
			case *ast.NumberExpr:
				keyType = typ.Integer
			default:
				if synth != nil && target.Key != nil {
					keyType = synth(target.Key, p)
				}
			}
			if keyType == nil {
				keyType = typ.String
			}

			// Strip falsy types (nil, false) from key types since they can't be valid map keys.
			keyType = narrow.ToTruthy(keyType)
			if keyType == nil {
				keyType = typ.String
			}

			// Determine value type
			var valType typ.Type
			if i < len(info.Sources) && info.Sources[i] != nil && synth != nil {
				valType = synth(info.Sources[i], p)
			}
			if valType == nil {
				valType = typ.Unknown
			}

			result[sym] = append(result[sym], mutator.IndexerInfo{
				KeyType: keyType,
				ValType: valType,
			})
		}
	})

	return result
}
