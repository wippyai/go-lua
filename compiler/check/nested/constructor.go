package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// This file implements detection and analysis of the Lua OOP constructor pattern.
//
// Lua doesn't have built-in classes, but a common idiom uses metatables for OOP:
//
//	local T = {}
//	function T.new()
//	    local self = setmetatable({}, {__index = T})
//	    self.field = value
//	    return self
//	end
//
// The constructor pattern consists of:
//  1. A function named "new" on a table (the class)
//  2. Creating self via setmetatable with the class as metatable
//  3. Assigning instance fields to self
//  4. Returning self
//
// Detecting this pattern enables the type checker to:
//   - Track which fields are assigned in constructors
//   - Provide accurate self-type in instance methods
//   - Enable autocompletion for instance fields

// DetectConstructorPattern checks if a function is a constructor that:
// 1. Is named T.new (assigned to a field named "new" on a table)
// 2. Creates self via setmetatable({}, T) or setmetatable({}, {__index = T})
// 3. Returns the self variable
//
// The nested evidence belongs to the function being analyzed. The parent
// evidence belongs to the graph where it is defined, for `T.new = function()`.
func DetectConstructorPattern(
	nestedEvidence api.FlowEvidence,
	parentEvidence api.FlowEvidence,
	fn *ast.FunctionExpr,
	funcDef *cfg.FuncDefInfo,
) (classSymbol, selfSymbol cfg.SymbolID) {
	if fn == nil {
		return 0, 0
	}

	// Check if function is T.new pattern
	var receiverSymbol cfg.SymbolID
	var receiverName string
	if funcDef != nil && funcDef.TargetKind == cfg.FuncDefField {
		if funcDef.TargetPath.Symbol != 0 && len(funcDef.TargetPath.Segments) == 1 {
			seg := funcDef.TargetPath.Segments[0]
			if seg.Kind == constraint.SegmentField && seg.Name == "new" {
				receiverSymbol = funcDef.TargetPath.Symbol
				receiverName = funcDef.ReceiverName
			}
		}
	}

	// Also check for T.new = function(...) pattern in the parent graph
	if receiverSymbol == 0 {
		var found cfg.SymbolID
		var foundName string
		for _, assign := range parentEvidence.Assignments {
			if found != 0 {
				break
			}
			info := assign.Info
			if info == nil {
				continue
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
				fnExpr, ok := src.(*ast.FunctionExpr)
				if !ok || fnExpr != fn {
					return
				}
				if target.Kind == cfg.TargetField && target.BaseSymbol != 0 && len(target.FieldPath) == 1 && target.FieldPath[0] == "new" {
					found = target.BaseSymbol
					foundName = target.BaseName
				}
			})
		}
		receiverSymbol = found
		receiverName = foundName
	}

	if receiverSymbol == 0 {
		return 0, 0
	}

	// Find setmetatable call that creates self
	selfSym := findSetmetatablePatternByName(nestedEvidence.Assignments, receiverName)
	if selfSym == 0 {
		return 0, 0
	}

	// Check that self is returned
	if !isSymbolReturned(nestedEvidence.Returns, selfSym) {
		return 0, 0
	}

	return receiverSymbol, selfSym
}

func findSetmetatablePatternByName(assignments []api.AssignmentEvidence, expectedClassName string) cfg.SymbolID {
	if len(assignments) == 0 {
		return 0
	}

	var selfSym cfg.SymbolID

	for _, assign := range assignments {
		if selfSym != 0 {
			break
		}
		info := assign.Info
		if info == nil {
			continue
		}
		if !info.IsLocal || len(info.Targets) == 0 {
			continue
		}

		// Look for setmetatable call
		call, ok := info.SourceAt(0).(*ast.FuncCallExpr)
		if !ok {
			continue
		}

		// Check if it's a setmetatable call
		ident, ok := call.Func.(*ast.IdentExpr)
		if !ok || ident.Value != "setmetatable" {
			continue
		}

		if len(call.Args) < 2 {
			continue
		}

		// First arg should be an empty table literal or table with initial values
		if _, ok := call.Args[0].(*ast.TableExpr); !ok {
			continue
		}

		// Second arg is the metatable - check for T or {__index = T}
		var foundClassName string

		switch mt := call.Args[1].(type) {
		case *ast.IdentExpr:
			foundClassName = mt.Value
		case *ast.TableExpr:
			for _, field := range mt.Fields {
				if field.Key == nil {
					continue
				}
				keyStr, ok := field.Key.(*ast.StringExpr)
				if !ok || keyStr.Value != "__index" {
					continue
				}
				if valIdent, ok := field.Value.(*ast.IdentExpr); ok {
					foundClassName = valIdent.Value
				}
			}
		}

		// Validate class name if expected
		if expectedClassName != "" && foundClassName != expectedClassName {
			continue
		}

		if target, ok := info.FirstTarget(); ok {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				selfSym = target.Symbol
			}
		}
	}

	return selfSym
}

// isSymbolReturnedOnAllPaths checks if a symbol is returned on ALL return paths.
// Returns false if any return path returns something other than the symbol.
func isSymbolReturned(returns []api.ReturnEvidence, sym cfg.SymbolID) bool {
	if len(returns) == 0 || sym == 0 {
		return false
	}

	hasReturn := false
	allReturnSym := true
	for _, ret := range returns {
		if !allReturnSym {
			break
		}
		info := ret.Info
		if info == nil {
			continue
		}
		hasReturn = true

		// Empty return or return with no expressions doesn't return self.
		if len(info.Exprs) == 0 {
			allReturnSym = false
			break
		}

		// Check if first return expression is the symbol.
		if len(info.Symbols) == 0 || info.Symbols[0] != sym {
			allReturnSym = false
		}
	}

	return hasReturn && allReturnSym
}

// CollectConstructorFields collects field assignments to a self symbol in a constructor.
//
// This reduces transfer assignment evidence for statements like
// `self.field = value` and builds a map of field names to their types.
// These fields become part of the class's instance type, enabling the type
// checker to validate field access on instances created by this constructor.
func CollectConstructorFields(assignments []api.AssignmentEvidence, selfSym cfg.SymbolID, synth func(ast.Expr, cfg.Point) typ.Type) map[string]typ.Type {
	if len(assignments) == 0 || selfSym == 0 {
		return nil
	}

	filterSyms := map[cfg.SymbolID]bool{selfSym: true}
	fields := overlaymut.CollectFieldAssignments(assignments, synth, filterSyms)

	if selfFields, ok := fields[selfSym]; ok && len(selfFields) > 0 {
		filtered := make(map[string]typ.Type, len(selfFields))
		for name, t := range selfFields {
			if typ.IsAbsentOrUnknown(t) {
				continue
			}
			filtered[name] = t
		}
		if len(filtered) > 0 {
			return filtered
		}
	}
	return nil
}
