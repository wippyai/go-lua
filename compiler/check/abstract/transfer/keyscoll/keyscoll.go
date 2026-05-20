package keyscoll

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/resolve"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
)

// KeysCollectorInfo tracks that a function returns keys of one of its parameters.
type KeysCollectorInfo struct {
	ParamIndex  int // Which parameter the keys come from (0-based)
	ReturnIndex int // Which return slot carries the keys table (0-based)
}

// GraphProvider resolves canonical CFGs for function literals.
type GraphProvider interface {
	GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph
}

// DetectKeysCollector analyzes a function graph to detect if it follows the
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
func DetectKeysCollector(graph *cfg.Graph, evidence api.FlowEvidence) *KeysCollectorInfo {
	if graph == nil {
		return nil
	}
	fn := graph.Func()
	if fn == nil || fn.Stmts == nil || len(fn.Stmts) == 0 {
		return nil
	}

	bindings := graph.Bindings()
	// Track: which local symbol is the "keys" table
	// Track: which param symbol is being iterated with pairs
	var keysTableSym cfg.SymbolID
	var pairsParamSym cfg.SymbolID
	pairsParamIndex := -1
	var keyVarSym cfg.SymbolID
	insertedKeyIntoTable := false
	keysReturnIndex := -1

	paramSlots := graph.ParamSlotsReadOnly()

	// Scan for local keys = {} pattern and generic for loop with pairs
	for _, assign := range evidence.Assignments {
		info := assign.Info
		if info == nil || len(info.Targets) == 0 {
			continue
		}

		// Check for local keys = {} pattern
		if info.IsLocal && len(info.Sources) > 0 {
			if target, ok := info.FirstTarget(); ok {
				if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
					if tbl, ok := info.SourceAt(0).(*ast.TableExpr); ok && tbl != nil && len(tbl.Fields) == 0 {
						if keysTableSym == 0 {
							keysTableSym = target.Symbol
						}
					}
				}
			}
		}

		// Check for generic for loop with pairs
		if len(info.IterExprs) > 0 {
			call, ok := info.IterExprs[0].(*ast.FuncCallExpr)
			if !ok || call == nil {
				continue
			}
			// Check if it's pairs(something)
			if !isPairsCall(call) {
				continue
			}
			if len(call.Args) == 0 {
				continue
			}
			// Check if the argument is a parameter
			argIdent, ok := call.Args[0].(*ast.IdentExpr)
			if !ok {
				continue
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
			// Check if argSym is a parameter. The slot index is the runtime
			// argument index, including implicit self when present.
			for i, slot := range paramSlots {
				if slot.Symbol == argSym {
					pairsParamSym = argSym
					pairsParamIndex = i
					break
				}
			}
			// Track the key variable (first loop variable)
			if target, ok := info.FirstTarget(); ok && target.Kind == cfg.TargetIdent {
				keyVarSym = target.Symbol
			}
		}
	}

	if keysTableSym == 0 || pairsParamSym == 0 || pairsParamIndex < 0 || keyVarSym == 0 {
		return nil
	}

	// Scan for table.insert(keys, k) pattern
	for _, call := range evidence.Calls {
		info := call.Info
		if info == nil {
			continue
		}
		if !isTableInsertCall(info) {
			continue
		}
		if len(info.Args) < 2 {
			continue
		}
		// Check first arg is the keys table
		argIdent, ok := info.Args[0].(*ast.IdentExpr)
		if !ok {
			continue
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
			continue
		}
		// Check second arg is the key variable
		valIdent, ok := info.Args[1].(*ast.IdentExpr)
		if !ok {
			continue
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
	}

	if !insertedKeyIntoTable {
		return nil
	}

	resolveIdentSym := func(expr ast.Expr) cfg.SymbolID {
		ident, ok := expr.(*ast.IdentExpr)
		if !ok {
			return 0
		}
		var sym cfg.SymbolID
		if bindings != nil {
			sym, _ = bindings.SymbolOf(ident)
		}
		if sym == 0 {
			if gb := graph.Bindings(); gb != nil {
				sym, _ = gb.SymbolOf(ident)
			}
		}
		return sym
	}

	// Scan for return keys pattern with a stable return slot index.
	for _, ret := range evidence.Returns {
		info := ret.Info
		if info == nil || len(info.Exprs) == 0 {
			continue
		}

		foundIdx := -1
		for i, expr := range info.Exprs {
			if resolveIdentSym(expr) != keysTableSym {
				continue
			}
			if foundIdx >= 0 {
				// Ambiguous: same return statement exposes keys at multiple slots.
				keysReturnIndex = -2
				break
			}
			foundIdx = i
		}
		if foundIdx < 0 {
			// Every return statement must carry the keys table for sound provenance.
			keysReturnIndex = -2
			continue
		}
		if keysReturnIndex == -1 {
			keysReturnIndex = foundIdx
			continue
		}
		if keysReturnIndex != foundIdx {
			// Ambiguous across return statements.
			keysReturnIndex = -2
		}
	}

	if keysReturnIndex < 0 {
		return nil
	}

	return &KeysCollectorInfo{ParamIndex: pairsParamIndex, ReturnIndex: keysReturnIndex}
}

func isPairsCall(call *ast.FuncCallExpr) bool {
	if call == nil || callsite.IsMethodLikeExpr(call) {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		return false
	}
	return ident.Value == "pairs"
}

func isTableInsertCall(info *cfg.CallInfo) bool {
	if info == nil || callsite.IsMethodLikeCallInfo(info) {
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

func functionGraph(fn *ast.FunctionExpr, owner *cfg.Graph, graphs GraphProvider) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if owner != nil && owner.Func() == fn {
		return owner
	}
	if graphs != nil {
		if graph := graphs.GetOrBuildCFG(fn); graph != nil {
			return graph
		}
	}
	if owner != nil && owner.Bindings() != nil {
		return cfg.BuildWithBindings(fn, owner.Bindings())
	}
	return cfg.Build(fn)
}

// BuildKeysCollectorDetector returns a callback that detects if a call is to a
// keys collector function and returns the symbol of the table argument.
func BuildKeysCollectorDetector(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	moduleBindings *bind.BindingTable,
	graphs GraphProvider,
) func(*cfg.CallInfo, cfg.Point, int) cfg.SymbolID {
	cache := make(map[cfg.SymbolID]*KeysCollectorInfo)
	bindings := graph.Bindings()

	return func(callInfo *cfg.CallInfo, _ cfg.Point, retIndex int) cfg.SymbolID {
		if callInfo == nil || callInfo.Callee == nil {
			return 0
		}
		candidates := callsite.CallableCalleeSymbolCandidates(callInfo, graph, bindings, moduleBindings)
		for _, calleeSym := range candidates {
			// Check cache
			if info, ok := cache[calleeSym]; ok {
				if info == nil {
					continue
				}
				if info.ReturnIndex != retIndex {
					continue
				}
				return callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(callInfo, info.ParamIndex), bindings)
			}

			// Resolve callee to a function literal through the graph evidence
			// and the available binding tables before classifying its body.
			fn := resolve.ResolveSymbolToFunctionLiteral(evidence, bindings, calleeSym)
			if fn == nil && moduleBindings != nil && moduleBindings != bindings {
				fn = resolve.ResolveSymbolToFunctionLiteral(evidence, moduleBindings, calleeSym)
			}
			if fn == nil {
				cache[calleeSym] = nil
				continue
			}

			fnGraph := functionGraph(fn, graph, graphs)
			fnEvidence := evidence
			if fnGraph != graph {
				fnEvidence = trace.GraphEvidence(fnGraph, graphBindings(fnGraph, moduleBindings))
			}
			info := DetectKeysCollector(fnGraph, fnEvidence)
			cache[calleeSym] = info
			if info == nil {
				continue
			}
			if info.ReturnIndex != retIndex {
				continue
			}
			return callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(callInfo, info.ParamIndex), bindings)
		}
		return 0
	}
}

func graphBindings(graph *cfg.Graph, fallback *bind.BindingTable) *bind.BindingTable {
	if graph != nil && graph.Bindings() != nil {
		return graph.Bindings()
	}
	return fallback
}
