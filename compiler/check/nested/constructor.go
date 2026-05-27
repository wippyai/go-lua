package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
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
	bindings *bind.BindingTable,
) (classSymbol, selfSymbol cfg.SymbolID) {
	pattern := DetectConstructorPatternInfo(nestedEvidence, parentEvidence, fn, funcDef, bindings)
	return pattern.ClassSymbol, pattern.SelfSymbol
}

// ConstructorPattern describes the instance/prototype relation discovered for a
// Lua setmetatable-backed constructor.
type ConstructorPattern struct {
	ClassSymbol             cfg.SymbolID
	PrototypeSymbol         cfg.SymbolID
	SelfSymbol              cfg.SymbolID
	InstanceLiteral         *ast.TableExpr
	InstancePoint           cfg.Point
	ReturnedViaSetmetatable bool
	// MetatableBindsReceiver is true when the setmetatable target structurally
	// references the class T (T directly or {__index = T}), proving the instance
	// is bound to that class rather than an unrelated prototype.
	MetatableBindsReceiver bool
}

// DetectConstructorPatternInfo detects constructors and identifies the prototype
// table that owns instance methods. In the common T.__index = T case the class
// and prototype are the same symbol; in split-prototype code such as
// `local mt = { __index = methods }`, the prototype is the method table.
func DetectConstructorPatternInfo(
	nestedEvidence api.FlowEvidence,
	parentEvidence api.FlowEvidence,
	fn *ast.FunctionExpr,
	funcDef *cfg.FuncDefInfo,
	bindings *bind.BindingTable,
) ConstructorPattern {
	if fn == nil {
		return ConstructorPattern{}
	}

	// Detect the function being assigned to a single field of a table T.
	// Constructors are recognized structurally (the body sets a metatable and
	// returns the instance); the field name is a confidence signal only.
	var receiverSymbol cfg.SymbolID
	var receiverName string
	var fieldName string
	if funcDef != nil && funcDef.TargetKind == cfg.FuncDefField {
		if funcDef.TargetPath.Symbol != 0 && len(funcDef.TargetPath.Segments) == 1 {
			seg := funcDef.TargetPath.Segments[0]
			if seg.Kind == constraint.SegmentField && seg.Name != "" {
				receiverSymbol = funcDef.TargetPath.Symbol
				receiverName = funcDef.ReceiverName
				fieldName = seg.Name
			}
		}
	}

	// Also check for T.field = function(...) pattern in the parent graph.
	if receiverSymbol == 0 {
		var found cfg.SymbolID
		var foundName string
		var foundField string
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
				if target.Kind == cfg.TargetField && target.BaseSymbol != 0 && len(target.FieldPath) == 1 && target.FieldPath[0] != "" {
					found = target.BaseSymbol
					foundName = target.BaseName
					foundField = target.FieldPath[0]
				}
			})
		}
		receiverSymbol = found
		receiverName = foundName
		fieldName = foundField
	}

	if receiverSymbol == 0 {
		return ConstructorPattern{}
	}

	pattern := findSetmetatableConstructorPattern(nestedEvidence, parentEvidence, receiverName, receiverSymbol, bindings)
	if pattern.SelfSymbol == 0 && pattern.InstanceLiteral == nil {
		return ConstructorPattern{}
	}

	// The literal field name "new" is the canonical constructor signal and is
	// accepted on the structural body match alone. Other field names must also
	// bind the instance to the class T (setmetatable target is T or {__index=T}),
	// so unrelated factory-shaped helpers do not get misread as constructors.
	if fieldName != "new" && !pattern.MetatableBindsReceiver {
		return ConstructorPattern{}
	}
	pattern.ClassSymbol = receiverSymbol
	if pattern.PrototypeSymbol == 0 {
		pattern.PrototypeSymbol = receiverSymbol
	}

	if pattern.SelfSymbol != 0 &&
		!pattern.ReturnedViaSetmetatable &&
		!isConstructorSymbolReturned(nestedEvidence.Returns, pattern.SelfSymbol) {
		return ConstructorPattern{}
	}

	return pattern
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

func findSetmetatableConstructorPattern(
	nestedEvidence api.FlowEvidence,
	parentEvidence api.FlowEvidence,
	receiverName string,
	receiverSym cfg.SymbolID,
	bindings *bind.BindingTable,
) ConstructorPattern {
	for _, assign := range nestedEvidence.Assignments {
		info := assign.Info
		if info == nil || !info.IsLocal || len(info.Targets) == 0 {
			continue
		}
		call, ok := setmetatableCall(info.SourceAt(0), bindings)
		if !ok {
			continue
		}
		tableArg, ok := call.Args[0].(*ast.TableExpr)
		if !ok || tableArg == nil {
			continue
		}
		target, ok := info.FirstTarget()
		if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		return ConstructorPattern{
			PrototypeSymbol:        prototypeSymbolFromMetatableArg(call.Args[1], parentEvidence, receiverName, receiverSym),
			SelfSymbol:             target.Symbol,
			InstanceLiteral:        tableArg,
			InstancePoint:          assign.Point,
			MetatableBindsReceiver: metatableBindsReceiver(call.Args[1], parentEvidence, receiverName, receiverSym),
		}
	}

	for _, ret := range nestedEvidence.Returns {
		if ret.Info == nil || len(ret.Info.Exprs) == 0 {
			continue
		}
		call, ok := setmetatableCall(ret.Info.Exprs[0], bindings)
		if !ok {
			continue
		}
		selfSym, tableArg := constructorInstanceFromSetmetatableArg(call.Args[0], nestedEvidence.Assignments)
		if selfSym == 0 && tableArg == nil {
			continue
		}
		return ConstructorPattern{
			PrototypeSymbol:         prototypeSymbolFromMetatableArg(call.Args[1], parentEvidence, receiverName, receiverSym),
			SelfSymbol:              selfSym,
			InstanceLiteral:         tableArg,
			InstancePoint:           ret.Point,
			ReturnedViaSetmetatable: true,
			MetatableBindsReceiver:  metatableBindsReceiver(call.Args[1], parentEvidence, receiverName, receiverSym),
		}
	}

	return ConstructorPattern{}
}

func setmetatableCall(expr ast.Expr, bindings *bind.BindingTable) (*ast.FuncCallExpr, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil || len(call.Args) < 2 {
		return nil, false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	if bindings == nil {
		if ident.Value != "setmetatable" {
			return nil, false
		}
		return call, true
	}
	if !bindings.ResolvesToUnshadowedGlobal(ident, "setmetatable") {
		return nil, false
	}
	return call, true
}

func constructorInstanceFromSetmetatableArg(
	arg ast.Expr,
	assignments []api.AssignmentEvidence,
) (cfg.SymbolID, *ast.TableExpr) {
	switch inst := arg.(type) {
	case *ast.TableExpr:
		return 0, inst
	case *ast.IdentExpr:
		if inst.Value == "" {
			return 0, nil
		}
		for _, assign := range assignments {
			info := assign.Info
			if info == nil {
				continue
			}
			target, ok := info.FirstTarget()
			if !ok || target.Kind != cfg.TargetIdent || target.Name != inst.Value {
				continue
			}
			if tbl, ok := info.SourceAt(0).(*ast.TableExpr); ok {
				return target.Symbol, tbl
			}
		}
	}
	return 0, nil
}

func prototypeSymbolFromMetatableArg(arg ast.Expr, parentEvidence api.FlowEvidence, receiverName string, receiverSym cfg.SymbolID) cfg.SymbolID {
	switch mt := arg.(type) {
	case *ast.TableExpr:
		if name := indexPrototypeName(mt); name != "" {
			if sym := symbolForAssignedName(parentEvidence.Assignments, name); sym != 0 {
				return sym
			}
			if name == receiverName {
				return receiverSym
			}
		}
	case *ast.IdentExpr:
		if mt.Value == receiverName {
			return receiverSym
		}
		for _, assign := range parentEvidence.Assignments {
			info := assign.Info
			if info == nil || len(info.Targets) == 0 {
				continue
			}
			target, ok := info.FirstTarget()
			if !ok || target.Kind != cfg.TargetIdent || target.Name != mt.Value {
				continue
			}
			if tbl, ok := info.SourceAt(0).(*ast.TableExpr); ok {
				if name := indexPrototypeName(tbl); name != "" {
					if sym := symbolForAssignedName(parentEvidence.Assignments, name); sym != 0 {
						return sym
					}
					if name == receiverName {
						return receiverSym
					}
				}
			}
			if target.Symbol != 0 {
				return target.Symbol
			}
		}
	}
	return 0
}

// metatableBindsReceiver reports whether the setmetatable target structurally
// binds the instance to the class T: either setmetatable({...}, T) or
// setmetatable({...}, {__index = T}), where T is the receiver. This is the
// structural signal that distinguishes a constructor from a factory that wraps
// some other prototype.
func metatableBindsReceiver(arg ast.Expr, parentEvidence api.FlowEvidence, receiverName string, receiverSym cfg.SymbolID) bool {
	switch mt := arg.(type) {
	case *ast.TableExpr:
		name := indexPrototypeName(mt)
		if name == "" {
			return false
		}
		if name == receiverName {
			return true
		}
		return symbolForAssignedName(parentEvidence.Assignments, name) == receiverSym && receiverSym != 0
	case *ast.IdentExpr:
		if mt.Value == "" {
			return false
		}
		if mt.Value == receiverName {
			return true
		}
		// The metatable may be a local bound to {__index = T}.
		for _, assign := range parentEvidence.Assignments {
			info := assign.Info
			if info == nil || len(info.Targets) == 0 {
				continue
			}
			target, ok := info.FirstTarget()
			if !ok || target.Kind != cfg.TargetIdent || target.Name != mt.Value {
				continue
			}
			tbl, ok := info.SourceAt(0).(*ast.TableExpr)
			if !ok {
				continue
			}
			name := indexPrototypeName(tbl)
			if name == "" {
				return false
			}
			if name == receiverName {
				return true
			}
			return symbolForAssignedName(parentEvidence.Assignments, name) == receiverSym && receiverSym != 0
		}
	}
	return false
}

func indexPrototypeName(tbl *ast.TableExpr) string {
	if tbl == nil {
		return ""
	}
	for _, field := range tbl.Fields {
		key, ok := field.Key.(*ast.StringExpr)
		if !ok || key.Value != "__index" {
			continue
		}
		if ident, ok := field.Value.(*ast.IdentExpr); ok {
			return ident.Value
		}
	}
	return ""
}

func symbolForAssignedName(assignments []api.AssignmentEvidence, name string) cfg.SymbolID {
	if name == "" {
		return 0
	}
	for _, assign := range assignments {
		info := assign.Info
		if info == nil {
			continue
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Name == name && target.Symbol != 0 {
				return target.Symbol
			}
		}
	}
	return 0
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

func isConstructorSymbolReturned(returns []api.ReturnEvidence, sym cfg.SymbolID) bool {
	if len(returns) == 0 || sym == 0 {
		return false
	}
	for _, ret := range returns {
		info := ret.Info
		if info == nil || len(info.Symbols) == 0 {
			continue
		}
		if info.Symbols[0] == sym {
			return true
		}
	}
	return false
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

// CollectConstructorLiteralFields collects fields declared directly in the
// instance table passed to setmetatable.
func CollectConstructorLiteralFields(table *ast.TableExpr, point cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type) map[string]typ.Type {
	if table == nil {
		return nil
	}
	fields := make(map[string]typ.Type)
	for _, field := range table.Fields {
		key, ok := field.Key.(*ast.StringExpr)
		if !ok || key.Value == "" || field.Value == nil {
			continue
		}
		var fieldType typ.Type
		if synth != nil {
			fieldType = synth(field.Value, point)
		}
		if typ.IsAbsentOrUnknown(fieldType) {
			fieldType = typ.Any
		}
		fields[key.Value] = fieldType
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// MergeConstructorFieldMaps joins constructor field maps from assignment and
// literal sources.
func MergeConstructorFieldMaps(a, b map[string]typ.Type) map[string]typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make(map[string]typ.Type, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if prev := out[k]; prev != nil {
			out[k] = typ.JoinPreferNonSoft(prev, v)
		} else {
			out[k] = v
		}
	}
	return out
}
