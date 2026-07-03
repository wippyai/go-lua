package program

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func applyMetatableMethodReceiverEntryStates(
	keys *programKeys,
	bindings *bind.Result,
	reg *axis.Registry,
	external typeannotation.Resolver,
	moduleExports importlookup.Source,
	roots ...[]ast.Stmt,
) {
	if keys == nil || bindings == nil || reg == nil {
		return
	}
	context := collectMetatableMethodContext(bindings, external, moduleExports, roots...)
	methodReceivers := context.methodReceivers
	if len(methodReceivers) == 0 {
		return
	}
	applyMetatableMethodReceiverEntryStatesTo(keys.functions, methodReceivers, context.seedReceivers, context.proof, bindings, reg)
	keys.contexts.TransformEntries(func(fn keyedFunction) keyedFunction {
		functions := []keyedFunction{fn}
		applyMetatableMethodReceiverEntryStatesTo(functions, methodReceivers, context.seedReceivers, context.proof, bindings, reg)
		return functions[0]
	})
}

func applyMetatableMethodReceiverEntryStatesTo(
	functions []keyedFunction,
	methodReceivers map[symbol.ID]typ.Type,
	seedReceivers map[symbol.ID]typ.Type,
	proof metatableMethodProof,
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
		seedReceiver := seedReceivers[table]
		seeded := false
		if seedType := receiver; usableContextualTypeOnly(seedType) {
			if seed, ok := implicitSelfParamSeed(reg, bindings, fn, seedType); ok {
				functions[i].entryState = forceParamSeed(reg, functions[i].entryState, seed)
				seeded = true
			}
		}
		memberReceiver := receiver
		if usableContextualTypeOnly(seedReceiver) {
			memberReceiver = seedReceiver
		}
		members := proof.methodSurfaceMembers(reg, table, memberReceiver)
		var membersSeeded bool
		functions[i].entryState, functions[i].entryKeys, membersSeeded = seedImplicitSelfMethodStaticMembers(
			reg,
			functions[i].entryState,
			functions[i].entryKeys,
			bindings,
			fn,
			members,
		)
		if !seeded && !membersSeeded {
			continue
		}
		functions[i].hasEntryState = true
	}
}

func metatableMethodReceiverTypes(bindings *bind.Result, external typeannotation.Resolver, moduleExports importlookup.Source, roots ...[]ast.Stmt) map[symbol.ID]typ.Type {
	return collectMetatableMethodContext(bindings, external, moduleExports, roots...).methodReceivers
}

func collectMetatableMethodContext(bindings *bind.Result, external typeannotation.Resolver, moduleExports importlookup.Source, roots ...[]ast.Stmt) metatableMethodContext {
	if bindings == nil {
		return metatableMethodContext{}
	}
	resolver := typeresolve.NewWithExternal(bindings, external)
	collector := methodReceiverCollector{
		bindings:      bindings,
		resolver:      resolver,
		external:      external,
		moduleExports: moduleExports,
		localTypes:    make(map[symbol.ID]typ.Type),
		functionTypes: collectFunctionTypesByPath(bindings, external, roots...),
		metaIndexes:   make(map[symbol.ID]symbol.ID),
		receivers:     make(map[symbol.ID]typ.Type),
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
		collector.collectSetmetatableReceivers(stmts, nil)
	}
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func != nil {
			collector.collectSetmetatableReceivers(origin.Func.Stmts, collector.functionReturnTypes(origin.Func))
		}
	}
	collector.ensureSelfIndexPrototypeReceiverBases()
	proof := collector.proof()
	seedReceivers := collector.seedReceiverMap()
	collector.attachMethodSurfaces(proof)
	return metatableMethodContext{
		methodReceivers: collector.receivers,
		seedReceivers:   seedReceivers,
		proof:           proof,
	}
}

type metatableMethodContext struct {
	methodReceivers map[symbol.ID]typ.Type
	seedReceivers   map[symbol.ID]typ.Type
	proof           metatableMethodProof
}

func collectFunctionTypesByPath(bindings *bind.Result, external typeannotation.Resolver, roots ...[]ast.Stmt) map[pathdom.PathKey]*typ.Function {
	if bindings == nil {
		return nil
	}
	targets := collectFunctionPathTargets(bindings, roots...)
	if len(targets) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]*typ.Function, len(targets))
	for fn, target := range targets {
		if fn == nil || target.IsEmpty() {
			continue
		}
		fnType, ok := lowerFunctionExprType(fn, bindings, external)
		if !ok || fnType == nil {
			continue
		}
		out[target.Key()] = fnType
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type methodReceiverCollector struct {
	bindings      *bind.Result
	resolver      *typeresolve.Resolver
	external      typeannotation.Resolver
	moduleExports importlookup.Source
	localTypes    map[symbol.ID]typ.Type
	functionTypes map[pathdom.PathKey]*typ.Function
	metaIndexes   map[symbol.ID]symbol.ID
	receivers     map[symbol.ID]typ.Type
	surfaceOnly   map[symbol.ID]struct{}
}

type metatableMethodProof struct {
	bindings        *bind.Result
	resolver        *typeresolve.Resolver
	metaIndexes     map[symbol.ID]symbol.ID
	receiverHints   map[symbol.ID]typ.Type
	methodReceivers map[symbol.ID]typ.Type
}

func (c methodReceiverCollector) proof() metatableMethodProof {
	return metatableMethodProof{
		bindings:        c.bindings,
		resolver:        c.resolver,
		metaIndexes:     copyMetatableIndexMap(c.metaIndexes),
		receiverHints:   c.receiverHintsByMetatable(),
		methodReceivers: copyReceiverMap(c.receivers),
	}
}

func (c methodReceiverCollector) receiverHintsByMetatable() map[symbol.ID]typ.Type {
	if len(c.metaIndexes) == 0 || len(c.receivers) == 0 {
		return nil
	}
	out := make(map[symbol.ID]typ.Type)
	for metatable, methods := range c.metaIndexes {
		receiver := c.receivers[methods]
		if receiver == nil {
			continue
		}
		out[metatable] = receiver
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p metatableMethodProof) empty() bool {
	return p.bindings == nil || p.resolver == nil || len(p.metaIndexes) == 0
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
				c.collectFunctionParamTypes(s.Func)
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

func (c methodReceiverCollector) collectFunctionParamTypes(fn *ast.FunctionExpr) {
	if fn == nil || c.resolver == nil {
		return
	}
	for _, slot := range c.bindings.ParamSlots(fn) {
		if slot.Symbol == 0 || slot.Type == nil {
			continue
		}
		t, ok := c.resolver.Type(slot.Type)
		if !ok || !usableMetatableReceiverType(t) {
			continue
		}
		c.localTypes[slot.Symbol] = t
	}
}

func (c methodReceiverCollector) collectSetmetatableReceivers(stmts []ast.Stmt, returnTypes []typ.Type) {
	for stmtIndex, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.LocalAssignStmt:
			c.collectLocalSetmetatableReceiver(s)
			c.collectLocalConstructorReceiver(s, stmts[stmtIndex+1:])
		case *ast.AssignStmt:
			c.collectAssignmentSetmetatableReceiver(s)
		case *ast.ReturnStmt:
			for i, expr := range s.Exprs {
				c.collectReturnSetmetatableReceiver(expr, 0, typeAt(returnTypes, i), stmts[:stmtIndex])
			}
		case *ast.FuncCallStmt:
			c.collectSetmetatableReceiver(s.Expr, 0, nil)
		case *ast.FuncDefStmt:
			if s.Func != nil {
				c.collectSetmetatableReceivers(s.Func.Stmts, c.functionReturnTypes(s.Func))
			}
		case *ast.DoBlockStmt:
			c.collectSetmetatableReceivers(s.Stmts, returnTypes)
		case *ast.IfStmt:
			c.collectSetmetatableReceivers(s.Then, returnTypes)
			c.collectSetmetatableReceivers(s.Else, returnTypes)
		case *ast.WhileStmt:
			c.collectSetmetatableReceivers(s.Stmts, returnTypes)
		case *ast.RepeatStmt:
			c.collectSetmetatableReceivers(s.Stmts, returnTypes)
		case *ast.NumberForStmt:
			c.collectSetmetatableReceivers(s.Stmts, returnTypes)
		case *ast.GenericForStmt:
			c.collectSetmetatableReceivers(s.Stmts, returnTypes)
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

func (c methodReceiverCollector) collectLocalConstructorReceiver(stmt *ast.LocalAssignStmt, tail []ast.Stmt) {
	if stmt == nil {
		return
	}
	symbols := c.bindings.LocalSymbols(stmt)
	for i, expr := range stmt.Exprs {
		self := symbolsAt(symbols, i)
		if self == 0 {
			continue
		}
		methods, ok := c.setmetatableMethods(expr)
		if !ok || methods == 0 {
			continue
		}
		base := c.localTypes[self]
		receiver, ok := c.constructorReceiverType(self, base, tail)
		if !ok || !usableMetatableReceiverType(receiver) {
			continue
		}
		c.receivers[methods] = receiver
		if c.surfaceOnly != nil {
			delete(c.surfaceOnly, methods)
		}
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
	c.collectSetmetatableReceiverWithDerived(expr, target, targetType, nil)
}

func (c methodReceiverCollector) collectReturnSetmetatableReceiver(expr ast.Expr, target symbol.ID, targetType typ.Type, prefix []ast.Stmt) {
	c.collectSetmetatableReceiverWithDerived(expr, target, targetType, func(object symbol.ID) (typ.Type, bool) {
		return c.returnedLocalConstructorReceiver(object, prefix)
	})
}

func (c methodReceiverCollector) collectSetmetatableReceiverWithDerived(expr ast.Expr, target symbol.ID, targetType typ.Type, derive func(symbol.ID) (typ.Type, bool)) {
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
	if receiver == nil && derive != nil {
		if derived, ok := derive(object.Symbol); ok {
			receiver = derived
		}
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

func (c methodReceiverCollector) returnedLocalConstructorReceiver(self symbol.ID, prefix []ast.Stmt) (typ.Type, bool) {
	if self == 0 {
		return nil, false
	}
	var (
		seen   bool
		base   typ.Type
		fields map[string]typ.Type
	)
	for _, stmt := range prefix {
		if !seen {
			local, ok := stmt.(*ast.LocalAssignStmt)
			if !ok {
				continue
			}
			symbols := c.bindings.LocalSymbols(local)
			for i, sym := range symbols {
				if sym != self {
					continue
				}
				seen = true
				base = c.localTypes[self]
				if base == nil {
					if t, ok := c.staticExpressionType(exprAt(local.Exprs, i)); ok && usableMetatableReceiverType(t) {
						base = t
					}
				}
				fields = make(map[string]typ.Type)
				break
			}
			continue
		}
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if !c.collectConstructorAssignments(self, fields, s) {
				return nil, false
			}
		case *ast.LocalAssignStmt:
			if c.localAssignMentionsSymbol(s, self) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if !seen {
		return nil, false
	}
	return c.buildConstructorReceiver(base, fields)
}

func (c methodReceiverCollector) setmetatableMethods(expr ast.Expr) (symbol.ID, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) < 2 {
		return 0, false
	}
	if !calleeIsGlobal(c.bindings, call.Func, "setmetatable") {
		return 0, false
	}
	meta, ok := pathexpr.Resolve(call.Args[1], c.bindings)
	if !ok || meta.Symbol == 0 {
		return 0, false
	}
	methods, ok := c.metaIndexes[meta.Symbol]
	if !ok || methods == 0 {
		return 0, false
	}
	return methods, true
}

func (c methodReceiverCollector) constructorReceiverType(self symbol.ID, base typ.Type, tail []ast.Stmt) (typ.Type, bool) {
	if self == 0 {
		return nil, false
	}
	fields := make(map[string]typ.Type)
	for _, stmt := range tail {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if !c.collectConstructorAssignments(self, fields, s) {
				return nil, false
			}
		case *ast.LocalAssignStmt:
			if c.localAssignMentionsSymbol(s, self) {
				return nil, false
			}
		case *ast.ReturnStmt:
			if !returnStartsWithSymbol(c.bindings, s, self) {
				return nil, false
			}
			return c.buildConstructorReceiver(base, fields)
		default:
			return nil, false
		}
	}
	return nil, false
}

func (c methodReceiverCollector) collectConstructorAssignments(self symbol.ID, fields map[string]typ.Type, stmt *ast.AssignStmt) bool {
	if stmt == nil {
		return true
	}
	for i, lhs := range stmt.Lhs {
		target, ok := pathexpr.Resolve(lhs, c.bindings)
		if ok && target.Symbol == self {
			name, direct := directConstructorField(target)
			if !direct {
				return false
			}
			t, typeOK := c.staticExpressionType(exprAt(stmt.Rhs, i))
			if !typeOK || !usableConstructorFieldType(t) {
				delete(fields, name)
				continue
			}
			fields[name] = widenConstructorFieldType(t)
			continue
		}
		if container, ok := pathexpr.ResolveMutationContainer(lhs, c.bindings); ok && container.Symbol == self {
			return false
		}
	}
	return true
}

func (c methodReceiverCollector) localAssignMentionsSymbol(stmt *ast.LocalAssignStmt, self symbol.ID) bool {
	if stmt == nil || self == 0 {
		return false
	}
	for _, expr := range stmt.Exprs {
		if exprMentionsSymbol(c.bindings, expr, self) {
			return true
		}
	}
	return false
}

func (c methodReceiverCollector) buildConstructorReceiver(base typ.Type, fields map[string]typ.Type) (typ.Type, bool) {
	if len(fields) == 0 {
		return base, usableMetatableReceiverType(base)
	}
	overlay := typetable.NewRecord()
	for name, t := range fields {
		if name == "" || !usableConstructorFieldType(t) {
			continue
		}
		overlay.Field(name, t)
	}
	witness := overlay.Build()
	if base == nil {
		return witness, true
	}
	if overlaid, ok := typetable.OverlayRecordMembers(base, witness); ok {
		return overlaid, true
	}
	return base, usableMetatableReceiverType(base)
}

func (c methodReceiverCollector) staticExpressionType(expr ast.Expr) (typ.Type, bool) {
	switch e := expr.(type) {
	case nil:
		return nil, false
	case *ast.CastExpr:
		if c.resolver != nil && e.Type != nil {
			if t, ok := c.resolver.Type(e.Type); ok && usableConstructorFieldType(t) {
				return t, true
			}
		}
		return c.staticExpressionType(e.Expr)
	case *ast.NonNilAssertExpr:
		return c.staticExpressionType(e.Expr)
	case *ast.FunctionExpr:
		return lowerFunctionExprType(e, c.bindings, c.external)
	case *ast.FuncCallExpr:
		if t, ok := c.callFirstReturnType(e); ok {
			return t, true
		}
	case *ast.LogicalOpExpr:
		return c.staticBinaryExpressionType(e.Lhs, e.Operator, e.Rhs)
	case *ast.RelationalOpExpr:
		return c.staticBinaryExpressionType(e.Lhs, e.Operator, e.Rhs)
	case *ast.StringConcatOpExpr:
		return c.staticBinaryExpressionType(e.Lhs, "..", e.Rhs)
	case *ast.ArithmeticOpExpr:
		return c.staticBinaryExpressionType(e.Lhs, e.Operator, e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return c.staticUnaryExpressionType("-", e.Expr)
	case *ast.UnaryNotOpExpr:
		return c.staticUnaryExpressionType("not", e.Expr)
	case *ast.UnaryLenOpExpr:
		return c.staticUnaryExpressionType("#", e.Expr)
	case *ast.UnaryBNotOpExpr:
		return c.staticUnaryExpressionType("~", e.Expr)
	case *ast.TableExpr:
		return c.staticTableExpressionType(e), true
	}
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	p, ok := pathexpr.Resolve(expr, c.bindings)
	if !ok || p.IsEmpty() {
		return nil, false
	}
	return c.staticPathType(p)
}

func (c methodReceiverCollector) staticBinaryExpressionType(left ast.Expr, op string, right ast.Expr) (typ.Type, bool) {
	leftType, ok := c.staticExpressionType(left)
	if !ok {
		return nil, false
	}
	rightType, ok := c.staticExpressionType(right)
	if !ok {
		return nil, false
	}
	return typeoperator.BinaryOp(leftType, op, rightType)
}

func (c methodReceiverCollector) staticUnaryExpressionType(op string, expr ast.Expr) (typ.Type, bool) {
	operand, ok := c.staticExpressionType(expr)
	if !ok {
		return nil, false
	}
	return typeoperator.UnaryOp(op, operand)
}

func (c methodReceiverCollector) staticTableExpressionType(table *ast.TableExpr) typ.Type {
	builder := typetable.NewRecord()
	if table == nil {
		return builder.Build()
	}
	nextIndex := int64(1)
	for _, field := range table.Fields {
		if field == nil {
			continue
		}
		t, ok := c.staticExpressionType(field.Value)
		if !ok || !usableConstructorFieldType(t) {
			if field.Key == nil {
				nextIndex++
			}
			continue
		}
		t = widenConstructorFieldType(t)
		if field.Key == nil {
			builder.StaticIntIndex(nextIndex, t)
			nextIndex++
			continue
		}
		name := ast.KeyName(field.Key)
		if name == "" {
			continue
		}
		if field.KeySyntax == ast.AttrKeyIndex {
			builder.StaticStringIndex(name, t)
			continue
		}
		builder.Field(name, t)
	}
	return builder.Build()
}

func (c methodReceiverCollector) callFirstReturnType(call *ast.FuncCallExpr) (typ.Type, bool) {
	if call == nil || call.Receiver != nil || call.Method != "" {
		return nil, false
	}
	callee, ok := pathexpr.Resolve(call.Func, c.bindings)
	if !ok || callee.IsEmpty() {
		return nil, false
	}
	if fnType := c.functionTypes[callee.Key()]; fnType != nil && len(fnType.Returns) != 0 {
		return fnType.Returns[0], true
	}
	calleeType, ok := c.staticPathType(callee)
	if !ok {
		return nil, false
	}
	return typecall.CallableReturn(calleeType)
}

func (c methodReceiverCollector) staticPathType(p pathdom.Path) (typ.Type, bool) {
	if p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	if fnType := c.functionTypes[p.Key()]; fnType != nil {
		return fnType, true
	}
	rootType := c.localTypes[p.Symbol]
	if rootType == nil {
		root := p.DisplayRoot(c.bindings.Name)
		if export, ok := c.moduleExports.LookupExport(root); ok {
			rootType = export
		}
	}
	if rootType == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	return typeprojection.ApplySegments(rootType, p.Segments)
}

func (c methodReceiverCollector) functionReturnTypes(fn *ast.FunctionExpr) []typ.Type {
	if fn == nil || c.resolver == nil || len(fn.ReturnTypes) == 0 {
		return nil
	}
	exprs := functionReturnTypeExprs(fn.ReturnTypes)
	out := make([]typ.Type, 0, len(exprs))
	for _, expr := range exprs {
		t, ok := c.resolver.Type(expr)
		if !ok {
			return nil
		}
		out = append(out, t)
	}
	return out
}

func (c methodReceiverCollector) attachMethodSurfaces(proof metatableMethodProof) {
	for methods, receiver := range c.receivers {
		surfaced, ok := proof.receiverWithMethodSurface(methods, receiver, receiver)
		if !ok {
			continue
		}
		c.receivers[methods] = surfaced
	}
}

func (c methodReceiverCollector) ensureSelfIndexPrototypeReceiverBases() {
	for metatable, methods := range c.metaIndexes {
		if metatable == 0 || methods == 0 || metatable != methods {
			continue
		}
		if _, ok := c.receivers[methods]; ok {
			continue
		}
		if receiver, ok := c.declaredMethodReceiver(methods); ok {
			c.receivers[methods] = receiver
			continue
		}
		c.receivers[methods] = typetable.NewRecord().Build()
		if c.surfaceOnly == nil {
			c.surfaceOnly = make(map[symbol.ID]struct{})
		}
		c.surfaceOnly[methods] = struct{}{}
	}
}

func (c methodReceiverCollector) declaredMethodReceiver(methods symbol.ID) (typ.Type, bool) {
	if methods == 0 || c.bindings == nil || c.resolver == nil {
		return nil, false
	}
	for _, origin := range c.bindings.FunctionOrigins() {
		if origin.Func == nil || origin.Kind != bind.FunctionOriginMethod {
			continue
		}
		table, ok := methodFunctionTableSymbol(c.bindings, origin)
		if !ok || table != methods {
			continue
		}
		decl, ok := c.bindings.MethodReceiverType(origin.Func)
		if !ok {
			continue
		}
		t, ok := c.resolver.Decl(decl)
		if ok && usableMetatableReceiverType(t) {
			return t, true
		}
	}
	return nil, false
}

func (c methodReceiverCollector) seedReceiverMap() map[symbol.ID]typ.Type {
	if len(c.receivers) == 0 {
		return nil
	}
	out := make(map[symbol.ID]typ.Type, len(c.receivers))
	for methods, receiver := range c.receivers {
		if _, surfaceOnly := c.surfaceOnly[methods]; surfaceOnly {
			continue
		}
		out[methods] = receiver
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p metatableMethodProof) receiverWithMethodSurfaceForMetatable(metatable symbol.ID, receiver typ.Type) (typ.Type, bool) {
	if p.empty() || metatable == 0 {
		return nil, false
	}
	methods, ok := p.metaIndexes[metatable]
	if !ok || methods == 0 {
		return nil, false
	}
	selfReceiver := receiver
	if hint := p.receiverHints[metatable]; hint != nil {
		selfReceiver = hint
	}
	return p.receiverWithMethodSurface(methods, receiver, selfReceiver)
}

func (p metatableMethodProof) receiverWithMethodSurface(methods symbol.ID, receiver, selfReceiver typ.Type) (typ.Type, bool) {
	receiver = concreteMetatableReceiverType(receiver)
	selfReceiver = concreteMetatableReceiverType(selfReceiver)
	if !usableMetatableReceiverType(receiver) {
		return nil, false
	}
	if !usableMetatableReceiverType(selfReceiver) {
		selfReceiver = receiver
	}
	surface, ok := p.methodSurface(methods, selfReceiver)
	if !ok {
		return receiver, true
	}
	if overlaid, ok := typetable.OverlayRecordMembers(receiver, surface); ok {
		return overlaid, true
	}
	return receiver, true
}

func (p metatableMethodProof) methodSurface(methods symbol.ID, selfReceiver typ.Type) (typ.Type, bool) {
	entries := p.methodSurfaceEntries(methods, selfReceiver)
	surface := typetable.NewRecord()
	for _, entry := range entries {
		surface.StaticStringIndex(entry.name, entry.fnType)
	}
	if len(entries) == 0 {
		return nil, false
	}
	return surface.Build(), true
}

type methodSurfaceEntry struct {
	name           string
	fnType         *typ.Function
	functionSymbol symbol.ID
}

type methodStaticMemberSeed struct {
	name  string
	value product.Value
}

func (p metatableMethodProof) methodSurfaceMembers(reg *axis.Registry, methods symbol.ID, receiver typ.Type) []methodStaticMemberSeed {
	if reg == nil {
		return nil
	}
	entries := p.methodSurfaceEntries(methods, receiver)
	out := make([]methodStaticMemberSeed, 0, len(entries))
	for _, entry := range entries {
		memberType := receiverStaticMemberType(receiver, entry.name)
		if memberType == nil {
			memberType = entry.fnType
		}
		value := typevalue.FromType(reg, memberType)
		if entry.functionSymbol != 0 {
			value = product.Set(reg, value, identity.Key, identity.Singleton(identity.LuaFunction(uint64(entry.functionSymbol))))
		}
		out = append(out, methodStaticMemberSeed{
			name:  entry.name,
			value: typevalue.WithWitness(reg, value, memberType),
		})
	}
	return out
}

func receiverStaticMemberType(receiver typ.Type, name string) typ.Type {
	if receiver == nil || name == "" {
		return nil
	}
	record, ok := unwrap.Alias(receiver).(*typ.Record)
	if !ok || record == nil {
		return nil
	}
	member := record.GetStaticStringIndex(name)
	if member == nil {
		return nil
	}
	return member.Type
}

func (p metatableMethodProof) methodSurfaceEntries(methods symbol.ID, selfReceiver typ.Type) []methodSurfaceEntry {
	if methods == 0 || p.bindings == nil {
		return nil
	}
	var out []methodSurfaceEntry
	for _, origin := range p.bindings.FunctionOrigins() {
		if origin.Func == nil || origin.Kind != bind.FunctionOriginMethod || origin.Method == "" {
			continue
		}
		table, ok := methodFunctionTableSymbol(p.bindings, origin)
		if !ok || table != methods {
			continue
		}
		fnType, ok := p.methodFunctionType(origin, selfReceiver)
		if !ok {
			continue
		}
		fnSymbol, _ := p.bindings.FunctionSymbol(origin.Func)
		out = append(out, methodSurfaceEntry{name: origin.Method, fnType: fnType, functionSymbol: fnSymbol})
	}
	return out
}

func (p metatableMethodProof) methodFunctionType(origin bind.FunctionOrigin, receiver typ.Type) (*typ.Function, bool) {
	fn := origin.Func
	if fn == nil || p.bindings == nil || p.resolver == nil {
		return nil, false
	}
	builder := typ.Func()
	for _, decl := range p.bindings.FunctionTypeParams(fn) {
		t, ok := p.resolver.Decl(decl)
		param, paramOK := t.(*typ.TypeParam)
		if !ok || !paramOK || param == nil {
			return nil, false
		}
		builder.TypeParamRef(param)
	}
	slots := p.bindings.ParamSlots(fn)
	builder.ReserveParams(len(slots) + 1)
	if origin.Kind == bind.FunctionOriginMethod && !paramSlotsHaveImplicitSelf(slots) {
		builder.Param("self", receiver)
	}
	for _, slot := range slots {
		t := typ.Any
		if slot.ImplicitSelf && slot.Type == nil {
			t = receiver
		} else if slot.Type != nil {
			resolved, ok := p.resolver.Type(slot.Type)
			if !ok {
				return nil, false
			}
			t = resolved
		}
		if slot.Vararg {
			builder.Variadic(t)
			continue
		}
		builder.Param(slot.Name, t)
	}
	returns := make([]typ.Type, 0, len(fn.ReturnTypes))
	for _, ret := range functionReturnTypeExprs(fn.ReturnTypes) {
		t, ok := p.resolver.Type(ret)
		if !ok {
			return nil, false
		}
		returns = append(returns, t)
	}
	if len(returns) != 0 {
		builder.Returns(returns...)
	}
	return builder.Build(), true
}

func copyMetatableIndexMap(in map[symbol.ID]symbol.ID) map[symbol.ID]symbol.ID {
	if len(in) == 0 {
		return nil
	}
	out := make(map[symbol.ID]symbol.ID, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyReceiverMap(in map[symbol.ID]typ.Type) map[symbol.ID]typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[symbol.ID]typ.Type, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func paramSlotsHaveImplicitSelf(slots []bind.ParamSlot) bool {
	for _, slot := range slots {
		if slot.ImplicitSelf {
			return true
		}
	}
	return false
}

func concreteMetatableReceiverType(t typ.Type) typ.Type {
	if inner := unwrap.Optional(t); inner != nil {
		return inner
	}
	return t
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
		if valueSlot == 0 {
			return paramSeed{}, false
		}
		return paramSeed{
			slot:  valueSlot,
			value: typevalue.WithWitness(reg, typevalue.FromType(reg, t), t),
		}, true
	}
	return paramSeed{}, false
}

func seedImplicitSelfMethodStaticMembers(
	reg *axis.Registry,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	members []methodStaticMemberSeed,
) (state.State, *keyspace.KeySpace, bool) {
	if reg == nil || bindings == nil || fn == nil || len(members) == 0 {
		return entry, entryKeys, false
	}
	self, ok := implicitSelfSymbol(bindings, fn)
	if !ok {
		return entry, entryKeys, false
	}
	if entryKeys == nil {
		entryKeys = keyspace.New()
	}
	out := entry
	edit := out.EditPathEvidence(reg)
	seeded := false
	for _, member := range members {
		if member.name == "" {
			continue
		}
		segments := []segment.Segment{{
			Kind: segment.SegmentField,
			Name: member.name,
		}}
		local, ok := pathaddr.LocalKeyForVersion(self, 1, segments)
		if !ok {
			continue
		}
		edit.WritePathStaticMember(entryKeys, local.PathKey(), member.value)
		if stable, ok := entryKeys.FromStableSymbol(self, segments); ok {
			edit.WriteLocalPathStaticMember(stable, member.value)
		}
		seeded = true
	}
	return edit.DoneOn(out), entryKeys, seeded
}

func forceParamSeed(reg *axis.Registry, entry state.State, seed paramSeed) state.State {
	if reg == nil || seed.slot == 0 {
		return entry
	}
	return entry.Snapshot().WriteValue(reg, seed.slot, seed.value)
}

func implicitSelfSymbol(bindings *bind.Result, fn *ast.FunctionExpr) (symbol.ID, bool) {
	for _, slot := range bindings.ParamSlots(fn) {
		if slot.Symbol != 0 && slot.Name == "self" && slot.ImplicitSelf {
			return slot.Symbol, true
		}
	}
	return 0, false
}

func directConstructorField(p pathdom.Path) (string, bool) {
	if len(p.Segments) != 1 {
		return "", false
	}
	seg := p.Segments[0]
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func returnStartsWithSymbol(bindings *bind.Result, stmt *ast.ReturnStmt, sym symbol.ID) bool {
	if stmt == nil || len(stmt.Exprs) == 0 || sym == 0 {
		return false
	}
	p, ok := pathexpr.Resolve(stmt.Exprs[0], bindings)
	return ok && p.Symbol == sym && len(p.Segments) == 0
}

func exprAt(exprs []ast.Expr, index int) ast.Expr {
	if index < 0 || index >= len(exprs) {
		return nil
	}
	return exprs[index]
}

func exprMentionsSymbol(bindings *bind.Result, expr ast.Expr, sym symbol.ID) bool {
	if expr == nil || sym == 0 {
		return false
	}
	if p, ok := pathexpr.Resolve(expr, bindings); ok && p.Symbol == sym {
		return true
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		return exprMentionsSymbol(bindings, e.Object, sym) || exprMentionsSymbol(bindings, e.Key, sym)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if exprMentionsSymbol(bindings, field.Key, sym) || exprMentionsSymbol(bindings, field.Value, sym) {
				return true
			}
		}
	case *ast.FuncCallExpr:
		if exprMentionsSymbol(bindings, e.Func, sym) || exprMentionsSymbol(bindings, e.Receiver, sym) {
			return true
		}
		for _, arg := range e.Args {
			if exprMentionsSymbol(bindings, arg, sym) {
				return true
			}
		}
	case *ast.LogicalOpExpr:
		return exprMentionsSymbol(bindings, e.Lhs, sym) || exprMentionsSymbol(bindings, e.Rhs, sym)
	case *ast.RelationalOpExpr:
		return exprMentionsSymbol(bindings, e.Lhs, sym) || exprMentionsSymbol(bindings, e.Rhs, sym)
	case *ast.StringConcatOpExpr:
		return exprMentionsSymbol(bindings, e.Lhs, sym) || exprMentionsSymbol(bindings, e.Rhs, sym)
	case *ast.ArithmeticOpExpr:
		return exprMentionsSymbol(bindings, e.Lhs, sym) || exprMentionsSymbol(bindings, e.Rhs, sym)
	case *ast.UnaryMinusOpExpr:
		return exprMentionsSymbol(bindings, e.Expr, sym)
	case *ast.UnaryNotOpExpr:
		return exprMentionsSymbol(bindings, e.Expr, sym)
	case *ast.UnaryLenOpExpr:
		return exprMentionsSymbol(bindings, e.Expr, sym)
	case *ast.UnaryBNotOpExpr:
		return exprMentionsSymbol(bindings, e.Expr, sym)
	case *ast.CastExpr:
		return exprMentionsSymbol(bindings, e.Expr, sym)
	case *ast.NonNilAssertExpr:
		return exprMentionsSymbol(bindings, e.Expr, sym)
	}
	return false
}

func usableConstructorFieldType(t typ.Type) bool {
	return t != nil &&
		t != typ.Nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!typ.IsNever(t) &&
		!refinement.ContainsFreeTypeParam(t)
}

func widenConstructorFieldType(t typ.Type) typ.Type {
	lit, ok := unwrap.Alias(t).(*typ.Literal)
	if !ok || lit == nil {
		return t
	}
	switch lit.Base {
	case kind.Boolean:
		return typ.Boolean
	case kind.String:
		return typ.String
	case kind.Integer:
		return typ.Integer
	case kind.Number:
		return typ.Number
	default:
		return t
	}
}

func usableMetatableReceiverType(t typ.Type) bool {
	t = concreteMetatableReceiverType(t)
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

func typeAt(types []typ.Type, index int) typ.Type {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
