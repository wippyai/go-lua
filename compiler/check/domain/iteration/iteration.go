// Package iteration owns generic-for iterator semantics used by both canonical
// transfer and diagnostic-facing flows.
package iteration

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	flowfacts "github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// BuiltinName returns the trusted iterator builtin name for expr.
//
// Builtin fallback is admitted only after binding has normalized the source
// identifier to a recorded global symbol with the same name. A local/upvalue/
// parameter named pairs or ipairs is just a user function and must carry an
// Iterator contract if it wants iterator semantics.
func BuiltinName(expr ast.Expr, bindings *bind.BindingTable) string {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident == nil || bindings == nil {
		return ""
	}
	switch ident.Value {
	case "pairs", "ipairs":
	default:
		return ""
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return ""
	}
	kind, ok := bindings.Kind(sym)
	if !ok || kind != cfg.SymbolGlobal {
		return ""
	}
	if name := bindings.Name(sym); name != "" && name != ident.Value {
		return ""
	}
	return ident.Value
}

// Kind resolves an iterator's abstract iteration mode and source argument slot.
// It first uses the callee contract's Iterator effect; when no contract iterator
// is present, it recognizes a trusted built-in ipairs/pairs name produced by
// BuiltinName.
func Kind(fnType typ.Type, builtinName string, argCount int) (flowfacts.IteratorKind, int, bool) {
	if spec := contract.ExtractSpec(fnType); spec != nil {
		if it := spec.GetIterator(); it != nil {
			idx, ok := effect.ResolveParamIndex(it.Source, argCount)
			if !ok {
				return 0, 0, false
			}
			switch it.Kind {
			case effect.IterateIndexed:
				return flowfacts.IterateIndexed, idx, true
			case effect.IterateKeyed:
				return flowfacts.IterateKeyed, idx, true
			default:
				return 0, 0, false
			}
		}
	}
	switch builtinName {
	case "ipairs":
		return flowfacts.IterateIndexed, 0, true
	case "pairs":
		return flowfacts.IterateKeyed, 0, true
	default:
		return 0, 0, false
	}
}
