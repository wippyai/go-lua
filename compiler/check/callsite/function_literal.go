package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// FunctionLiteralForSymbol resolves a function symbol to its function literal.
//
// Resolution order:
//  1. Binding table reverse lookup for literal symbols.
func FunctionLiteralForSymbol(
	bindings *bind.BindingTable,
	evidence api.FlowEvidence,
	sym cfg.SymbolID,
) *ast.FunctionExpr {
	if sym == 0 {
		return nil
	}
	if bindings != nil {
		if fn, ok := bindings.FuncLitBySymbol(sym); ok && fn != nil {
			return fn
		}
	}
	if fn := functionLiteralFromDefinitions(evidence, sym, true); fn != nil {
		return fn
	}
	return nil
}

// FunctionLiteralForGraphSymbol resolves only graph-local stable function
// bindings for a symbol.
//
// Canonical boundary:
//   - include graph-local/global function definitions
//   - include local identifier assignments of function literals
//   - exclude mutable field-path symbols, whose current callable type must come
//     from value flow at the call site rather than binder symbol backtracking
func FunctionLiteralForGraphSymbol(evidence api.FlowEvidence, sym cfg.SymbolID) *ast.FunctionExpr {
	if sym == 0 {
		return nil
	}

	return functionLiteralFromDefinitions(evidence, sym, false)
}

func functionLiteralFromDefinitions(evidence api.FlowEvidence, sym cfg.SymbolID, includeMutableTargets bool) *ast.FunctionExpr {
	for _, def := range evidence.FunctionDefinitions {
		if def.Nested.Func == nil {
			continue
		}
		if includeMutableTargets && def.Nested.Symbol == sym {
			return def.Nested.Func
		}
		if def.Symbol == sym {
			if includeMutableTargets || stableGraphFunctionDefinition(def) {
				return def.Nested.Func
			}
		}
		if def.FuncDef != nil && def.FuncDef.Symbol == sym {
			if includeMutableTargets || def.FuncDef.TargetKind == cfg.FuncDefGlobal {
				return def.FuncDef.FuncExpr
			}
		}
	}
	return nil
}

func stableGraphFunctionDefinition(def api.FunctionDefinitionEvidence) bool {
	if def.FuncDef == nil {
		return def.IsLocal && def.Name != ""
	}
	return true
}

// AllowsDiscardedExtraArgs reports whether the source function has unannotated
// positional parameters, where Lua accepts and discards surplus call arguments.
// Explicit source varargs are represented by typ.Function.Variadic instead.
func AllowsDiscardedExtraArgs(fn *ast.FunctionExpr) bool {
	if fn == nil || fn.ParList == nil || fn.ParList.HasVargs {
		return false
	}
	for i := range fn.ParList.Names {
		if i >= len(fn.ParList.Types) || fn.ParList.Types[i] == nil {
			return true
		}
	}
	return false
}
