package program

import (
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func applyMetatableMethodReceiverEntryStates(
	keys *programKeys,
	bindings *bind.Result,
	reg *axis.Registry,
	external typeannotation.Resolver,
	roots ...[]ast.Stmt,
) {
	if keys == nil || bindings == nil || reg == nil {
		return
	}
	methodReceivers := metatableMethodReceiverTypes(bindings, external, roots...)
	if len(methodReceivers) == 0 {
		return
	}
	applyMetatableMethodReceiverEntryStatesTo(keys.functions, methodReceivers, bindings, reg)
	applyMetatableMethodReceiverEntryStatesTo(keys.contexts, methodReceivers, bindings, reg)
}

func applyMetatableMethodReceiverEntryStatesTo(
	functions []keyedFunction,
	methodReceivers map[symbol.ID]typ.Type,
	bindings *bind.Result,
	reg *axis.Registry,
) {
	for i := range functions {
		fn := functions[i].funcExpr
		if fn == nil {
			continue
		}
		origin, ok := bindings.FunctionOrigin(fn)
		if !ok || origin.Kind != bind.FunctionOriginMethod {
			continue
		}
		table, ok := methodFunctionTableSymbol(bindings, origin)
		if !ok {
			continue
		}
		receiver, ok := methodReceivers[table]
		if !ok || !usableContextualTypeOnly(receiver) {
			continue
		}
		seed, ok := implicitSelfParamSeed(reg, bindings, fn, receiver)
		if !ok {
			continue
		}
		functions[i].entryState = applyParamSeeds(reg, functions[i].entryState, []paramSeed{seed})
		functions[i].hasEntryState = true
	}
}

func metatableMethodReceiverTypes(bindings *bind.Result, external typeannotation.Resolver, roots ...[]ast.Stmt) map[symbol.ID]typ.Type {
	if bindings == nil {
		return nil
	}
	resolver := typeresolve.NewWithExternal(bindings, external)
	collector := methodReceiverCollector{
		bindings:    bindings,
		resolver:    resolver,
		localTypes:  make(map[symbol.ID]typ.Type),
		metaIndexes: make(map[symbol.ID]symbol.ID),
		receivers:   make(map[symbol.ID]typ.Type),
	}
	for _, stmts := range roots {
		collector.collectTypesAndMetatables(stmts)
	}
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func != nil {
			collector.collectTypesAndMetatables(origin.Func.Stmts)
		}
	}
	for _, stmts := range roots {
		collector.collectSetmetatableReceivers(stmts)
	}
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func != nil {
			collector.collectSetmetatableReceivers(origin.Func.Stmts)
		}
	}
	if len(collector.receivers) == 0 {
		return nil
	}
	return collector.receivers
}

type methodReceiverCollector struct {
	bindings    *bind.Result
	resolver    *typeresolve.Resolver
	localTypes  map[symbol.ID]typ.Type
	metaIndexes map[symbol.ID]symbol.ID
	receivers   map[symbol.ID]typ.Type
}

func (c methodReceiverCollector) collectTypesAndMetatables(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.LocalAssignStmt:
			c.collectLocalTypes(s)
			c.collectLocalMetatableIndex(s)
		case *ast.AssignStmt:
			c.collectAssignedMetatableIndex(s)
		case *ast.FuncDefStmt:
			if s.Func != nil {
				c.collectTypesAndMetatables(s.Func.Stmts)
			}
		case *ast.DoBlockStmt:
			c.collectTypesAndMetatables(s.Stmts)
		case *ast.IfStmt:
			c.collectTypesAndMetatables(s.Then)
			c.collectTypesAndMetatables(s.Else)
		case *ast.WhileStmt:
			c.collectTypesAndMetatables(s.Stmts)
		case *ast.RepeatStmt:
			c.collectTypesAndMetatables(s.Stmts)
		case *ast.NumberForStmt:
			c.collectTypesAndMetatables(s.Stmts)
		case *ast.GenericForStmt:
			c.collectTypesAndMetatables(s.Stmts)
		}
	}
}

func (c methodReceiverCollector) collectSetmetatableReceivers(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.LocalAssignStmt:
			c.collectLocalSetmetatableReceiver(s)
		case *ast.AssignStmt:
			c.collectAssignmentSetmetatableReceiver(s)
		case *ast.ReturnStmt:
			for _, expr := range s.Exprs {
				c.collectSetmetatableReceiver(expr, 0, nil)
			}
		case *ast.FuncDefStmt:
			if s.Func != nil {
				c.collectSetmetatableReceivers(s.Func.Stmts)
			}
		case *ast.DoBlockStmt:
			c.collectSetmetatableReceivers(s.Stmts)
		case *ast.IfStmt:
			c.collectSetmetatableReceivers(s.Then)
			c.collectSetmetatableReceivers(s.Else)
		case *ast.WhileStmt:
			c.collectSetmetatableReceivers(s.Stmts)
		case *ast.RepeatStmt:
			c.collectSetmetatableReceivers(s.Stmts)
		case *ast.NumberForStmt:
			c.collectSetmetatableReceivers(s.Stmts)
		case *ast.GenericForStmt:
			c.collectSetmetatableReceivers(s.Stmts)
		}
	}
}

func (c methodReceiverCollector) collectLocalTypes(stmt *ast.LocalAssignStmt) {
	if stmt == nil {
		return
	}
	symbols := c.bindings.LocalSymbols(stmt)
	for i, sym := range symbols {
		if sym == 0 || i >= len(stmt.Types) || stmt.Types[i] == nil {
			continue
		}
		t, ok := c.resolver.Type(stmt.Types[i])
		if !ok || !usableMetatableReceiverType(t) {
			continue
		}
		c.localTypes[sym] = t
	}
}

func (c methodReceiverCollector) collectLocalMetatableIndex(stmt *ast.LocalAssignStmt) {
	if stmt == nil {
		return
	}
	symbols := c.bindings.LocalSymbols(stmt)
	for i, expr := range stmt.Exprs {
		if i >= len(symbols) || symbols[i] == 0 {
			continue
		}
		table, ok := metatableIndexTable(c.bindings, expr)
		if ok {
			c.metaIndexes[symbols[i]] = table
		}
	}
}

func (c methodReceiverCollector) collectAssignedMetatableIndex(stmt *ast.AssignStmt) {
	if stmt == nil || len(stmt.Lhs) == 0 || len(stmt.Rhs) == 0 {
		return
	}
	for i, lhs := range stmt.Lhs {
		if i >= len(stmt.Rhs) {
			break
		}
		meta, ok := assignedMetatableIndex(c.bindings, lhs, stmt.Rhs[i])
		if !ok {
			continue
		}
		c.metaIndexes[meta.metatable] = meta.methods
	}
}

func (c methodReceiverCollector) collectLocalSetmetatableReceiver(stmt *ast.LocalAssignStmt) {
	if stmt == nil {
		return
	}
	symbols := c.bindings.LocalSymbols(stmt)
	for i, expr := range stmt.Exprs {
		var receiver typ.Type
		hasReceiver := false
		if i < len(symbols) && symbols[i] != 0 {
			receiver, hasReceiver = c.localTypes[symbols[i]]
		}
		c.collectSetmetatableReceiver(expr, symbolsAt(symbols, i), optionalType(receiver, hasReceiver))
	}
}

func (c methodReceiverCollector) collectAssignmentSetmetatableReceiver(stmt *ast.AssignStmt) {
	if stmt == nil {
		return
	}
	for i, expr := range stmt.Rhs {
		if i >= len(stmt.Lhs) {
			break
		}
		target, _ := pathexpr.Resolve(stmt.Lhs[i], c.bindings)
		c.collectSetmetatableReceiver(expr, target.Symbol, optionalType(nil, false))
	}
}

func (c methodReceiverCollector) collectSetmetatableReceiver(expr ast.Expr, target symbol.ID, targetType typ.Type) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) < 2 {
		return
	}
	if !calleeIsGlobal(c.bindings, call.Func, "setmetatable") {
		return
	}
	object, ok := pathexpr.Resolve(call.Args[0], c.bindings)
	if !ok || object.Symbol == 0 {
		return
	}
	receiver := targetType
	if receiver == nil {
		receiver = c.localTypes[object.Symbol]
	}
	if receiver == nil && target != 0 {
		receiver = c.localTypes[target]
	}
	if !usableMetatableReceiverType(receiver) {
		return
	}
	meta, ok := pathexpr.Resolve(call.Args[1], c.bindings)
	if !ok || meta.Symbol == 0 {
		return
	}
	methods, ok := c.metaIndexes[meta.Symbol]
	if !ok || methods == 0 {
		return
	}
	c.receivers[methods] = receiver
}

type metatableIndexAssignment struct {
	metatable symbol.ID
	methods   symbol.ID
}

func assignedMetatableIndex(bindings *bind.Result, lhs, rhs ast.Expr) (metatableIndexAssignment, bool) {
	p, ok := pathexpr.Resolve(lhs, bindings)
	if !ok || p.Symbol == 0 || len(p.Segments) != 1 {
		return metatableIndexAssignment{}, false
	}
	if p.Segments[0].Name != "__index" {
		return metatableIndexAssignment{}, false
	}
	methods, ok := pathexpr.Resolve(rhs, bindings)
	if !ok || methods.Symbol == 0 || len(methods.Segments) != 0 {
		return metatableIndexAssignment{}, false
	}
	return metatableIndexAssignment{metatable: p.Symbol, methods: methods.Symbol}, true
}

func metatableIndexTable(bindings *bind.Result, expr ast.Expr) (symbol.ID, bool) {
	table, ok := expr.(*ast.TableExpr)
	if !ok || table == nil {
		return 0, false
	}
	arrayIndex := 0
	for _, field := range table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok || len(suffix.Path.Segments) != 1 || suffix.Path.Segments[0].Name != "__index" {
			continue
		}
		p, ok := pathexpr.Resolve(field.Value, bindings)
		if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
			continue
		}
		return p.Symbol, true
	}
	return 0, false
}

func methodFunctionTableSymbol(bindings *bind.Result, origin bind.FunctionOrigin) (symbol.ID, bool) {
	stmt, ok := origin.Stmt.(*ast.FuncDefStmt)
	if !ok || stmt == nil || stmt.Name == nil || stmt.Name.Method == "" {
		return 0, false
	}
	p, ok := pathexpr.ResolveFuncName(stmt.Name, bindings)
	if !ok || p.Symbol == 0 || len(p.Segments) == 0 {
		return 0, false
	}
	return p.Symbol, true
}

func implicitSelfParamSeed(reg *axis.Registry, bindings *bind.Result, fn *ast.FunctionExpr, t typ.Type) (paramSeed, bool) {
	for _, slot := range bindings.ParamSlots(fn) {
		if slot.Symbol == 0 || slot.Name != "self" || !slot.ImplicitSelf {
			continue
		}
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == "" {
			return paramSeed{}, false
		}
		return paramSeed{
			slot:  valueSlot,
			value: typevalue.WithWitness(reg, typevalue.FromType(reg, t), t),
		}, true
	}
	return paramSeed{}, false
}

func usableMetatableReceiverType(t typ.Type) bool {
	return t != nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!typ.IsNever(t) &&
		!refinement.ContainsFreeTypeParam(t)
}

func calleeIsGlobal(bindings *bind.Result, expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return false
	}
	id, ok := pathexpr.Resolve(ident, bindings)
	return ok && id.Symbol != 0 && len(id.Segments) == 0 && bindings.ResolvesToGlobal(ident, name)
}

func symbolsAt(symbols []symbol.ID, index int) symbol.ID {
	if index < 0 || index >= len(symbols) {
		return 0
	}
	return symbols[index]
}

func optionalType(t typ.Type, ok bool) typ.Type {
	if !ok {
		return nil
	}
	return t
}
