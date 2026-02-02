package keyscoll

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// KeysCollectorInfo tracks that a function returns keys of one of its parameters.
type KeysCollectorInfo struct {
	ParamIndex int // Which parameter the keys come from (0-based)
}

// DetectKeysCollector analyzes a function body to detect if it follows the
// "keys collector" pattern: creates a table, iterates with pairs over a param,
// inserts keys into the table, and returns it.
//
// Pattern:
//
//	local keys = {}
//	for k in pairs(param) do
//	    table.insert(keys, k)
//	end
//	return keys
func DetectKeysCollector(fn *ast.FunctionExpr) *KeysCollectorInfo {
	if fn == nil || fn.Stmts == nil || len(fn.Stmts) == 0 {
		return nil
	}

	graph := cfg.Build(fn)
	if graph == nil {
		return nil
	}

	// Use graph's own bindings since we build a fresh CFG.
	// Passed-in bindings may have different symbol IDs.
	bindings := graph.Bindings()

	// Track: which local symbol is the "keys" table
	// Track: which param symbol is being iterated with pairs
	var keysTableSym cfg.SymbolID
	var pairsParamSym cfg.SymbolID
	pairsParamIndex := -1
	var keyVarSym cfg.SymbolID
	insertedKeyIntoTable := false
	returnsKeysTable := false

	paramSymbols := graph.ParamSymbols()

	// Scan for local keys = {} pattern and generic for loop with pairs
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 {
			return
		}

		// Check for local keys = {} pattern
		if info.IsLocal && len(info.Sources) > 0 {
			target := info.Targets[0]
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				if tbl, ok := info.Sources[0].(*ast.TableExpr); ok && tbl != nil && len(tbl.Fields) == 0 {
					if keysTableSym == 0 {
						keysTableSym = target.Symbol
					}
				}
			}
		}

		// Check for generic for loop with pairs
		if len(info.IterExprs) > 0 {
			call, ok := info.IterExprs[0].(*ast.FuncCallExpr)
			if !ok || call == nil {
				return
			}
			// Check if it's pairs(something)
			if !isPairsCall(call) {
				return
			}
			if len(call.Args) == 0 {
				return
			}
			// Check if the argument is a parameter
			argIdent, ok := call.Args[0].(*ast.IdentExpr)
			if !ok {
				return
			}
			var argSym cfg.SymbolID
			if bindings != nil {
				argSym, _ = bindings.SymbolOf(argIdent)
			}
			if argSym == 0 {
				// Try graph bindings
				if gb := graph.Bindings(); gb != nil {
					argSym, _ = gb.SymbolOf(argIdent)
				}
			}
			// Check if argSym is a parameter
			for i, ps := range paramSymbols {
				if ps == argSym {
					pairsParamSym = argSym
					pairsParamIndex = i
					break
				}
			}
			// Track the key variable (first loop variable)
			if len(info.Targets) > 0 && info.Targets[0].Kind == cfg.TargetIdent {
				keyVarSym = info.Targets[0].Symbol
			}
		}
	})

	if keysTableSym == 0 || pairsParamSym == 0 || pairsParamIndex < 0 || keyVarSym == 0 {
		return nil
	}

	// Scan for table.insert(keys, k) pattern
	graph.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		if !isTableInsertCall(info) {
			return
		}
		if len(info.Args) < 2 {
			return
		}
		// Check first arg is the keys table
		argIdent, ok := info.Args[0].(*ast.IdentExpr)
		if !ok {
			return
		}
		var argSym cfg.SymbolID
		if bindings != nil {
			argSym, _ = bindings.SymbolOf(argIdent)
		}
		if argSym == 0 {
			if gb := graph.Bindings(); gb != nil {
				argSym, _ = gb.SymbolOf(argIdent)
			}
		}
		if argSym != keysTableSym {
			return
		}
		// Check second arg is the key variable
		valIdent, ok := info.Args[1].(*ast.IdentExpr)
		if !ok {
			return
		}
		var valSym cfg.SymbolID
		if bindings != nil {
			valSym, _ = bindings.SymbolOf(valIdent)
		}
		if valSym == 0 {
			if gb := graph.Bindings(); gb != nil {
				valSym, _ = gb.SymbolOf(valIdent)
			}
		}
		if valSym == keyVarSym {
			insertedKeyIntoTable = true
		}
	})

	if !insertedKeyIntoTable {
		return nil
	}

	// Scan for return keys pattern
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		retIdent, ok := info.Exprs[0].(*ast.IdentExpr)
		if !ok {
			return
		}
		var retSym cfg.SymbolID
		if bindings != nil {
			retSym, _ = bindings.SymbolOf(retIdent)
		}
		if retSym == 0 {
			if gb := graph.Bindings(); gb != nil {
				retSym, _ = gb.SymbolOf(retIdent)
			}
		}
		if retSym == keysTableSym {
			returnsKeysTable = true
		}
	})

	if !returnsKeysTable {
		return nil
	}

	return &KeysCollectorInfo{ParamIndex: pairsParamIndex}
}

func isPairsCall(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || call.Receiver != nil {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		return false
	}
	return ident.Value == "pairs"
}

func isTableInsertCall(info *cfg.CallInfo) bool {
	if info == nil || info.Method != "" || info.Receiver != nil {
		return false
	}
	attr, ok := info.Callee.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	obj, ok := attr.Object.(*ast.IdentExpr)
	if !ok || obj.Value != "table" {
		return false
	}
	key, ok := attr.Key.(*ast.StringExpr)
	if !ok || key.Value != "insert" {
		return false
	}
	return true
}

// BuildKeysCollectorDetector returns a callback that detects if a call is to a
// keys collector function and returns the symbol of the table argument.
func BuildKeysCollectorDetector(graph *cfg.Graph) func(*cfg.CallInfo, cfg.Point) cfg.SymbolID {
	cache := make(map[cfg.SymbolID]*KeysCollectorInfo)
	bindings := graph.Bindings()

	return func(callInfo *cfg.CallInfo, p cfg.Point) cfg.SymbolID {
		if callInfo == nil || callInfo.Callee == nil {
			return 0
		}
		if callInfo.Method != "" || callInfo.Receiver != nil {
			return 0
		}

		calleeSym := callInfo.CalleeSymbol
		if calleeSym == 0 {
			return 0
		}

		// Check cache
		if info, ok := cache[calleeSym]; ok {
			if info == nil {
				return 0
			}
			return extractTableArgSymbol(callInfo, info.ParamIndex, bindings)
		}

		// Try to resolve callee to function literal
		fn := resolveSymToFuncLiteral(graph, calleeSym)
		if fn == nil {
			cache[calleeSym] = nil
			return 0
		}

		info := DetectKeysCollector(fn)
		cache[calleeSym] = info
		if info == nil {
			return 0
		}

		return extractTableArgSymbol(callInfo, info.ParamIndex, bindings)
	}
}

// extractTableArgSymbol extracts the symbol of the table argument at the given index.
func extractTableArgSymbol(callInfo *cfg.CallInfo, paramIndex int, bindings *bind.BindingTable) cfg.SymbolID {
	if paramIndex < 0 || paramIndex >= len(callInfo.Args) {
		return 0
	}
	argExpr := callInfo.Args[paramIndex]
	if argExpr == nil {
		return 0
	}
	ident, ok := argExpr.(*ast.IdentExpr)
	if !ok {
		return 0
	}
	if bindings == nil {
		return 0
	}
	sym, _ := bindings.SymbolOf(ident)
	return sym
}

// resolveSymToFuncLiteral resolves a symbol to a function literal defined in the graph.
func resolveSymToFuncLiteral(graph *cfg.Graph, sym cfg.SymbolID) *ast.FunctionExpr {
	if graph == nil || sym == 0 {
		return nil
	}

	var found *ast.FunctionExpr
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if found != nil || info == nil {
			return
		}
		if info.Symbol == sym {
			found = info.FuncExpr
		}
	})
	if found != nil {
		return found
	}
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if found != nil || info == nil {
			return
		}
		for i, target := range info.Targets {
			if target.Symbol == sym && i < len(info.Sources) {
				if fn, ok := info.Sources[i].(*ast.FunctionExpr); ok {
					found = fn
					return
				}
			}
		}
	})
	return found
}
