// Package iteration owns generic-for iterator semantics used by both canonical
// transfer and diagnostic-facing flows.
package iteration

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// VarProjection is the semantic result of projecting generic-for variables from
// an iterator source. Empty distinguishes a recognized iterator over a source
// with no present entries from an unrecognized iterator; transfer can use that
// to make the loop body unreachable through the CFG's first-variable not-nil
// check instead of falling back to imprecise unknown variables.
type VarProjection struct {
	Types []typ.Type
	Empty bool
}

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
func Kind(fnType typ.Type, builtinName string, argCount int) (effect.IteratorKind, int, bool) {
	if spec := contract.ExtractSpec(fnType); spec != nil {
		if it := spec.GetIterator(); it != nil {
			idx, ok := effect.ResolveParamIndex(it.Source, argCount)
			if !ok {
				return 0, 0, false
			}
			switch it.Kind {
			case effect.IterateIndexed, effect.IterateKeyed:
				return it.Kind, idx, true
			default:
				return 0, 0, false
			}
		}
	}
	switch builtinName {
	case "ipairs":
		return effect.IterateIndexed, 0, true
	case "pairs":
		return effect.IterateKeyed, 0, true
	default:
		return 0, 0, false
	}
}

// VarTypes projects generic-for loop variable types from the iterator kind and
// source container. It returns false when typing the loop variables would require
// assuming a relation the source type does not prove.
func VarTypes(kind effect.IteratorKind, count int, source typ.Type) ([]typ.Type, bool) {
	proj, ok := ProjectVarTypes(kind, count, source)
	if !ok || proj.Empty {
		return nil, false
	}
	return proj.Types, true
}

// ProjectVarTypes projects generic-for loop variable types and preserves the
// recognized-empty case. It is the canonical iterator-domain operation used by
// the canonical transfer; VarTypes is the legacy two-state wrapper.
func ProjectVarTypes(kind effect.IteratorKind, count int, source typ.Type) (VarProjection, bool) {
	if count <= 0 {
		return VarProjection{}, false
	}
	if typ.IsAny(source) && kind == effect.IterateKeyed {
		out := make([]typ.Type, count)
		for i := range out {
			out[i] = typ.Any
		}
		return VarProjection{Types: out}, true
	}
	if typ.IsNever(source) {
		return VarProjection{Empty: true}, true
	}
	if source == nil || typ.IsAbsentOrUnknown(source) {
		return VarProjection{}, false
	}
	out := make([]typ.Type, count)
	switch kind {
	case effect.IterateIndexed:
		out[0] = typ.Integer
		if count > 1 {
			out[1] = core.ElementType(source)
			if out[1] == nil && isPlaceholder(unwrap.Underlying(source)) {
				out[1] = typ.Any
			}
		}
	case effect.IterateKeyed:
		out[0] = core.EntryKeyType(source)
		if out[0] == nil {
			if IsUniformKeyedContainer(source) {
				return VarProjection{Empty: true}, true
			}
			return VarProjection{}, false
		}
		if count > 1 {
			out[1] = core.EntryValueType(source)
			if out[1] == nil {
				return VarProjection{Empty: true}, true
			}
		}
	default:
		return VarProjection{}, false
	}
	return VarProjection{Types: out}, true
}

// IsUniformKeyedContainer reports whether pairs-style iteration may soundly
// project yielded entries through EntryKeyType/EntryValueType. Closed records are
// accepted: they have a finite present-entry key domain. Closed empty records are
// recognized as iterable but empty, so ProjectVarTypes reports Empty.
func IsUniformKeyedContainer(t typ.Type) bool {
	switch v := unwrap.Underlying(t).(type) {
	case *typ.Map, *typ.ReadonlyMap:
		return true
	case *typ.Array, *typ.Tuple:
		return true
	case *typ.Record:
		return true
	case *typ.Optional:
		return IsUniformKeyedContainer(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !IsUniformKeyedContainer(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isPlaceholder(t typ.Type) bool {
	return t != nil && t.Kind().IsPlaceholder()
}
