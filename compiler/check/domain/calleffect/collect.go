package calleffect

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectTableInsertMutations reduces call evidence for table mutator calls on
// indexed expressions. For table.insert(t[k], v), it returns mutations grouped
// by the base symbol of t.
func CollectTableInsertMutations(
	calls []api.CallEvidence,
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
) map[cfg.SymbolID][]overlaymut.MapWriteInfo {
	result := make(map[cfg.SymbolID][]overlaymut.MapWriteInfo)
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

		targetAttr, ok := targetExpr.(*ast.AttrGetExpr)
		if !ok {
			continue
		}

		baseSym := callsite.SymbolOrCreateFieldFromExpr(targetAttr.Object, bindings)
		if baseSym == 0 {
			continue
		}

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

		keyType = narrow.ToTruthy(keyType)
		if keyType == nil || keyType.Kind().IsPlaceholder() {
			keyType = typ.String
		}

		var elemType typ.Type
		if synth != nil && valueExpr != nil {
			elemType = synth(valueExpr, p)
		}
		if elemType == nil {
			elemType = typ.Unknown
		}

		result[baseSym] = append(result[baseSym], overlaymut.MapWriteInfo{
			KeyType:   keyType,
			ValueType: typ.NewArray(elemType),
		})
	}

	return result
}

// CollectTableInsertOnDirect reduces call evidence for table mutator calls on
// direct variables. For table.insert(t, v), it returns element mutations grouped
// by the symbol of t.
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

		sym := callsite.SymbolOrCreateFieldFromExpr(targetExpr, bindings)
		if sym == 0 {
			continue
		}

		var elemType typ.Type
		if synth != nil && valueExpr != nil {
			elemType = synth(valueExpr, p)
		}
		if elemType == nil {
			elemType = typ.Unknown
		}

		if existing := result[sym]; existing != nil {
			result[sym] = overlaymut.JoinValueTypes(existing, elemType)
		} else {
			result[sym] = elemType
		}
	}

	return result
}
