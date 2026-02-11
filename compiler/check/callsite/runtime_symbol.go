package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// SymbolOrCreateFieldFromExpr resolves a symbol from an expression.
//
// It first uses existing symbol bindings (identifiers, function literals, existing field
// symbols). If the expression is a static field path with no existing field symbol, it
// creates one via bindings.GetOrCreateFieldSymbol to provide canonical identity.
func SymbolOrCreateFieldFromExpr(expr ast.Expr, bindings *bind.BindingTable) cfg.SymbolID {
	if expr == nil || bindings == nil {
		return 0
	}
	if sym := SymbolFromExpr(expr, bindings); sym != 0 {
		return sym
	}
	baseSym, fieldPath, ok := FieldPathWithBaseSymbol(bindings, expr)
	if !ok || baseSym == 0 || fieldPath == "" {
		return 0
	}
	return bindings.GetOrCreateFieldSymbol(baseSym, fieldPath)
}

// RuntimeArgSymbolAt resolves the symbol for a runtime argument index
// (receiver-aware for method calls).
func RuntimeArgSymbolAt(info *cfg.CallInfo, paramIdx int, bindings *bind.BindingTable) cfg.SymbolID {
	return SymbolOrCreateFieldFromExpr(RuntimeArgAt(info, paramIdx), bindings)
}
