package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// HasLocalCallSites checks whether the graph contains call sites to local functions.
//
// This is an optimization check. If a function's CFG contains no calls to other
// local functions, it has no mutual recursion dependencies and can be analyzed
// independently without SCC iteration. This allows skipping the more expensive
// fixpoint computation for simple functions.
func HasLocalCallSites(graph *cfg.Graph, localFuncs map[cfg.SymbolID]*LocalFuncInfo) bool {
	if graph == nil || len(localFuncs) == 0 {
		return false
	}
	matches := func(info *cfg.CallInfo) bool {
		if info == nil || info.CalleeSymbol == 0 {
			return false
		}
		_, ok := localFuncs[info.CalleeSymbol]
		return ok
	}
	found := false
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if found {
			return
		}
		if matches(info) {
			found = true
		}
	})
	return found
}

// CollectCalledNestedFieldAssignments collects field assignments recorded for
// called nested functions that target symbols from the parent graph (captured variables).
//
// When a nested function assigns fields to a captured variable, those assignments
// affect the type of the variable in the parent scope. This function scans nested
// functions that are called from the parent graph and collects field assignments
// that target parent-scope variables.
//
// Example:
//
//	local t = {}
//	local function setup()
//	    t.name = "hello"  -- This assignment is collected
//	end
//	setup()  -- Because setup() is called, t gains field "name"
//
// The result maps symbols to their assigned field types, which can be merged
// into the symbol's type in the parent scope.
func CollectCalledNestedFieldAssignments(
	parent *cfg.Graph,
	bindings *bind.BindingTable,
	capturedByCallee map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
	if parent == nil || len(capturedByCallee) == 0 {
		return result
	}

	// Gather all symbols known in the parent graph (avoid per-point merges).
	parentSymbols := parent.AllSymbolIDs()

	// Find which local functions are called in the parent graph.
	calledSyms := make(map[cfg.SymbolID]bool)
	parent.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		if info.CalleeSymbol != 0 {
			calledSyms[info.CalleeSymbol] = true
			return
		}
		if ident, ok := info.Callee.(*ast.IdentExpr); ok && bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				calledSyms[sym] = true
			}
		}
	})

	// Collect field assignments from called nested functions and merge into result.
	if len(calledSyms) == 0 {
		return result
	}
	syms := make([]cfg.SymbolID, 0, len(calledSyms))
	for sym := range calledSyms {
		syms = append(syms, sym)
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i] < syms[j] })
	for _, sym := range syms {
		nestedFields := capturedByCallee[sym]
		if len(nestedFields) == 0 {
			continue
		}
		for baseSym, fields := range nestedFields {
			if !parentSymbols[baseSym] {
				continue
			}
			if result[baseSym] == nil {
				result[baseSym] = make(map[string]typ.Type)
			}
			for fieldName, fieldType := range fields {
				if existing := result[baseSym][fieldName]; existing != nil {
					result[baseSym][fieldName] = typ.JoinPreferNonSoft(existing, fieldType)
				} else {
					result[baseSym][fieldName] = fieldType
				}
			}
		}
	}

	return result
}
