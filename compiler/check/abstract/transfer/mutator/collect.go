package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexerInfo holds key and value types for dynamic index assignments.
type IndexerInfo struct {
	KeyType typ.Type
	ValType typ.Type
}

// CollectTableInsertMutations reduces transfer call evidence for table mutator calls on indexed expressions.
// For table.insert(t[k], v), returns mutations grouped by the base symbol of t.
// Uses the effect-based detection via TableMutatorFromCall.
func CollectTableInsertMutations(
	calls []api.CallEvidence,
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
) map[cfg.SymbolID][]IndexerInfo {
	result := make(map[cfg.SymbolID][]IndexerInfo)
	if len(calls) == 0 || graph == nil {
		return result
	}

	for _, call := range calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}

		tm := TableMutatorFromCall(info, p, synth, nil, graph, bindings, nil)
		if tm == nil {
			continue
		}

		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			continue
		}

		// Check if target is an indexed expression: t[k]
		targetAttr, ok := targetExpr.(*ast.AttrGetExpr)
		if !ok {
			continue
		}

		baseSym := callsite.SymbolOrCreateFieldFromExpr(targetAttr.Object, bindings)
		if baseSym == 0 {
			continue
		}

		// Get key type from the index key
		var keyType typ.Type
		switch k := targetAttr.Key.(type) {
		case *ast.IdentExpr:
			if synth != nil {
				keyType = synth(k, p)
			}
		case *ast.StringExpr:
			keyType = typ.LiteralString(k.Value)
		case *ast.NumberExpr:
			keyType = typ.Integer
		default:
			if synth != nil && targetAttr.Key != nil {
				keyType = synth(targetAttr.Key, p)
			}
		}
		if keyType == nil {
			keyType = typ.String
		}

		// Strip falsy types from key types
		keyType = narrow.ToTruthy(keyType)
		if keyType == nil {
			keyType = typ.String
		}

		// Get value type from the inserted element
		var elemType typ.Type
		if synth != nil && valueExpr != nil {
			elemType = synth(valueExpr, p)
		}
		if elemType == nil {
			elemType = typ.Unknown
		}

		// The value type is an array of the element type
		valType := typ.NewArray(elemType)

		result[baseSym] = append(result[baseSym], IndexerInfo{
			KeyType: keyType,
			ValType: valType,
		})
	}

	return result
}

// CollectTableInsertOnDirect reduces transfer call evidence for table mutator calls on direct variables.
// For table.insert(t, v), returns mutations grouped by the symbol of t.
// Uses the effect-based detection via TableMutatorFromCall.
func CollectTableInsertOnDirect(
	calls []api.CallEvidence,
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
) map[cfg.SymbolID]typ.Type {
	result := make(map[cfg.SymbolID]typ.Type)
	if len(calls) == 0 || graph == nil {
		return result
	}

	for _, call := range calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}

		tm := TableMutatorFromCall(info, p, synth, nil, graph, bindings, nil)
		if tm == nil {
			continue
		}

		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			continue
		}

		// Check if target resolves to a direct symbol (identifier or static field path).
		sym := callsite.SymbolOrCreateFieldFromExpr(targetExpr, bindings)
		if sym == 0 {
			continue
		}

		// Get value type from the inserted element
		var elemType typ.Type
		if synth != nil && valueExpr != nil {
			elemType = synth(valueExpr, p)
		}
		if elemType == nil {
			elemType = typ.Unknown
		}

		// Join with existing element type
		if existing := result[sym]; existing != nil {
			result[sym] = typ.JoinPreferNonSoft(existing, elemType)
		} else {
			result[sym] = elemType
		}
	}

	return result
}

// MergeIndexerMutations merges table mutator mutations into indexer assignments.
func MergeIndexerMutations(
	indexers map[cfg.SymbolID][]IndexerInfo,
	mutations map[cfg.SymbolID][]IndexerInfo,
) {
	for sym, infos := range mutations {
		indexers[sym] = append(indexers[sym], infos...)
	}
}
