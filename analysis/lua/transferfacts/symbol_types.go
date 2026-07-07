package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/functiontype"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerSymbolTypes(
	bindings *bind.Result,
	graph cfg.Graph,
	meta cfgfacts.Metadata,
	result *semantics.Result,
	resolver *typeresolve.Resolver,
	moduleExports importlookup.Source,
	methodReceiverTypes map[symbol.ID]typ.Type,
) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil {
		return nil
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	out := make(map[symbol.ID]typ.Type)
	add := func(id symbol.ID, expr ast.TypeExpr) {
		if id == 0 || expr == nil {
			return
		}
		t, ok := resolver.Type(expr)
		if !ok {
			return
		}
		out[id] = t
	}
	for _, fn := range bindings.Functions() {
		for _, slot := range bindings.ParamSlots(fn) {
			add(slot.Symbol, slot.Type)
			if slot.ImplicitSelf && slot.Type == nil {
				if decl, ok := bindings.MethodReceiverType(fn); ok {
					if t, ok := resolver.Decl(decl); ok {
						out[slot.Symbol] = t
						continue
					}
				}
				if t, ok := metatableMethodSelfType(bindings, fn, methodReceiverTypes); ok {
					out[slot.Symbol] = t
					continue
				}
			}
		}
	}
	if result != nil {
		if fn := result.Function(); fn != nil {
			for _, capture := range bindings.DirectCaptures(fn) {
				if capture.Captured == 0 {
					continue
				}
				if _, present := out[capture.Captured]; present {
					continue
				}
				if modulePath, ok := moduleidentity.LocalRequireModulePath(bindings, capture.Captured); ok {
					if t, ok := moduleExports.LookupExport(modulePath); ok {
						out[capture.Captured] = t
					}
				}
			}
		}
	}
	for _, point := range graph.RPO() {
		fact, ok := functionDefinitionFactAt(meta, point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		if t, ok := functionExpressionType(fact.Func, bindings, resolver); ok {
			out[fact.TargetSymbol] = t
		}
	}
	for _, origin := range bindings.FunctionOrigins() {
		if !origin.HasTargetSymbol || origin.TargetSymbol == 0 || origin.Func == nil {
			continue
		}
		if _, present := out[origin.TargetSymbol]; present {
			continue
		}
		if t, ok := functionExpressionType(origin.Func, bindings, resolver); ok {
			out[origin.TargetSymbol] = t
		}
	}
	if result != nil {
		for _, point := range graph.RPO() {
			view, ok := result.LocalAssignmentView(point)
			if !ok {
				continue
			}
			fact, ok := view.Borrowed()
			if !ok || !fact.HasSymbol {
				continue
			}
			add(fact.Symbol, fact.Type)
		}
	}
	// A numeric-for control variable has no annotation, so record the strongest
	// type proven by the control operands. Lua uses an integer loop when init,
	// limit, and step are all integers; otherwise the variable is numeric.
	for _, point := range graph.RPO() {
		fact, ok := numericForFactAt(meta, point)
		if !ok || !fact.HasSymbol || fact.Symbol == 0 {
			continue
		}
		if _, present := out[fact.Symbol]; present {
			continue
		}
		out[fact.Symbol] = numericForSymbolType(out, bindings, fact.Init, fact.Limit, fact.Step)
	}
	if result == nil {
		if len(out) == 0 {
			return nil
		}
		return out
	}
	// Resolve un-annotated `local x = <access-chain>` locals whose initializer is
	// a static field/index chain rooted at an already-typed symbol. The chain's
	// element type is the local's checked type, used as the contextual record for
	// object literals later assigned to that local.
	for _, point := range graph.RPO() {
		view, ok := result.LocalAssignmentView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok || !fact.HasSymbol || fact.Symbol == 0 || fact.Type != nil || fact.Expr == nil {
			continue
		}
		if _, present := out[fact.Symbol]; present {
			continue
		}
		if modulePath, ok := moduleidentity.LocalRequireModulePath(bindings, fact.Symbol); ok {
			if t, ok := moduleExports.LookupExport(modulePath); ok {
				out[fact.Symbol] = t
				continue
			}
		}
		if fn, ok := fact.Expr.(*ast.FunctionExpr); ok {
			if t, ok := functionExpressionType(fn, bindings, resolver); ok {
				out[fact.Symbol] = t
				continue
			}
		}
		if t, ok := callFirstReturnType(out, bindings, fact.Expr); ok {
			out[fact.Symbol] = t
			continue
		}
		if t, ok := objectLiteralTypeFromSymbols(out, bindings, resolver, fact.Expr); ok {
			out[fact.Symbol] = t
			continue
		}
		if t, ok := accessChainType(out, bindings, fact.Expr); ok {
			out[fact.Symbol] = t
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metatableMethodSelfType(bindings *bind.Result, fn *ast.FunctionExpr, methodReceiverTypes map[symbol.ID]typ.Type) (typ.Type, bool) {
	if len(methodReceiverTypes) == 0 || bindings == nil || fn == nil {
		return nil, false
	}
	origin, ok := bindings.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginMethod {
		return nil, false
	}
	table, ok := bindings.MethodOriginReceiverSymbol(origin)
	if !ok || table == 0 {
		return nil, false
	}
	t := methodReceiverTypes[table]
	return t, t != nil
}

func lowerSymbolTypesFromWIR(body *wir.Body, bindings *bind.Result, moduleExports importlookup.Source) map[symbol.ID]typ.Type {
	if body == nil {
		return nil
	}
	out := make(map[symbol.ID]typ.Type)
	addWIRRequireExportSymbolTypes(out, body, bindings, moduleExports)
	tempDefs := wirTempDefinitions(body)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		switch inst.Op {
		case wir.OpClaim:
			if inst.Type == 0 || inst.Claim != wir.ClaimAnnotation {
				continue
			}
		case wir.OpMakeTable:
			if inst.Assign != wir.AssignLocalDeclaration {
				continue
			}
		case wir.OpAssign:
			if inst.Assign != wir.AssignLocalDeclaration {
				continue
			}
		case wir.OpCall:
			addWIRCallResultSymbolTypes(out, body, inst)
			continue
		default:
			continue
		}
		if inst.Dst.Kind != wir.OperandPath {
			continue
		}
		p := body.Path(wir.PathRef(inst.Dst.Ref))
		if p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
			continue
		}
		t := body.Type(inst.Type)
		if t == nil && inst.Op == wir.OpAssign && inst.Assign == wir.AssignLocalDeclaration {
			if source, ok := inst.AssignmentSourceOperand(); ok && source.Kind == wir.OperandPath {
				if inferred, ok := wirPathTypeFromSymbols(out, body.Path(wir.PathRef(source.Ref))); ok &&
					inferred != nil &&
					!typ.IsAny(inferred) &&
					!typ.IsUnknown(inferred) {
					t = inferred
				}
			}
		}
		if t == nil && inst.Op == wir.OpMakeTable {
			t, _ = wirObjectLiteralTypeFromSymbols(out, body, tempDefs, inst, nil)
		}
		if t == nil {
			continue
		}
		out[p.Symbol] = t
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addWIRRequireExportSymbolTypes(out map[symbol.ID]typ.Type, body *wir.Body, bindings *bind.Result, moduleExports importlookup.Source) {
	if out == nil || body == nil || bindings == nil {
		return
	}
	written := wirRootAssignmentSymbols(body)
	for id := range wirReferencedRootSymbols(body) {
		if _, exists := out[id]; exists || written[id] {
			continue
		}
		modulePath, ok := moduleidentity.LocalRequireModulePath(bindings, id)
		if !ok {
			continue
		}
		if t, ok := moduleExports.LookupExport(modulePath); ok {
			out[id] = t
		}
	}
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpCall {
			continue
		}
		for _, target := range body.CallResultTargets(inst.Point) {
			if target.Kind != wir.CallResultTargetLocalAssignment ||
				target.Path.Symbol == 0 ||
				len(target.Path.Segments) != 0 ||
				target.ResultIndex != 0 {
				continue
			}
			if _, exists := out[target.Path.Symbol]; exists {
				continue
			}
			modulePath, ok := moduleidentity.LocalRequireModulePath(bindings, target.Path.Symbol)
			if !ok {
				continue
			}
			if t, ok := moduleExports.LookupExport(modulePath); ok {
				out[target.Path.Symbol] = t
			}
		}
	}
}

func wirReferencedRootSymbols(body *wir.Body) map[symbol.ID]bool {
	if body == nil {
		return nil
	}
	out := make(map[symbol.ID]bool)
	addOperandRootSymbol := func(op wir.Operand) {
		if op.Kind != wir.OperandPath {
			return
		}
		p := body.Path(wir.PathRef(op.Ref))
		if p.Symbol != 0 {
			out[p.Symbol] = true
		}
	}
	addOperandRangeRootSymbols := func(r wir.OperandRange) {
		for _, op := range body.Operands(r) {
			addOperandRootSymbol(op)
		}
	}
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		addOperandRootSymbol(inst.Dst)
		addOperandRootSymbol(inst.A)
		addOperandRootSymbol(inst.B)
		addOperandRootSymbol(inst.Call.Callee)
		addOperandRootSymbol(inst.Call.Receiver)
		addOperandRangeRootSymbols(inst.List)
		addOperandRangeRootSymbols(inst.Results)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func wirRootAssignmentSymbols(body *wir.Body) map[symbol.ID]bool {
	if body == nil {
		return nil
	}
	out := make(map[symbol.ID]bool)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Assign == wir.AssignNone || inst.Dst.Kind != wir.OperandPath {
			continue
		}
		p := body.Path(wir.PathRef(inst.Dst.Ref))
		if p.Symbol != 0 && len(p.Segments) == 0 {
			out[p.Symbol] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func wirObjectLiteralTypeFromSymbols(
	symbolTypes map[symbol.ID]typ.Type,
	body *wir.Body,
	tempDefs map[uint32]wir.Instruction,
	inst wir.Instruction,
	seen map[uint32]bool,
) (typ.Type, bool) {
	if body == nil || inst.Op != wir.OpMakeTable {
		return nil, false
	}
	if inst.Type != 0 {
		t := body.Type(inst.Type)
		return t, t != nil
	}
	builder := typetable.NewConstructorBuilder()
	anySeen := false
	for _, entry := range body.TableEntries(inst.TableEntries) {
		keys, ok := wirConstructorKeysFromSuffix(entry.Suffix)
		if !ok {
			continue
		}
		valueType, ok := wirConstructorValueTypeFromSymbols(symbolTypes, body, tempDefs, entry.Value, seen)
		if !ok || valueType == nil {
			continue
		}
		if !builder.Add(keys, valueType) {
			return nil, false
		}
		anySeen = true
	}
	if !anySeen {
		return nil, false
	}
	return builder.Build()
}

func wirConstructorValueTypeFromSymbols(
	symbolTypes map[symbol.ID]typ.Type,
	body *wir.Body,
	tempDefs map[uint32]wir.Instruction,
	op wir.Operand,
	seen map[uint32]bool,
) (typ.Type, bool) {
	if body == nil {
		return nil, false
	}
	switch op.Kind {
	case wir.OperandPath:
		return wirPathTypeFromSymbols(symbolTypes, body.Path(wir.PathRef(op.Ref)))
	case wir.OperandTemp:
		if seen == nil {
			seen = make(map[uint32]bool)
		}
		if seen[op.Ref] {
			return nil, false
		}
		def, ok := tempDefs[op.Ref]
		if !ok {
			return nil, false
		}
		seen[op.Ref] = true
		defer delete(seen, op.Ref)
		if def.Type != 0 {
			if t := body.Type(def.Type); t != nil {
				return t, true
			}
		}
		switch def.Op {
		case wir.OpAssign:
			return wirConstructorValueTypeFromSymbols(symbolTypes, body, tempDefs, def.A, seen)
		case wir.OpMakeTable:
			return wirObjectLiteralTypeFromSymbols(symbolTypes, body, tempDefs, def, seen)
		}
	}
	return nil, false
}

func wirPathTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, p path.Path) (typ.Type, bool) {
	if p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	rootType, ok := symbolTypes[p.Symbol]
	if !ok || rootType == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	return typeprojection.ApplySegments(rootType, p.Segments)
}

func wirConstructorKeysFromSuffix(suffix path.Path) ([]typetable.ConstructorKey, bool) {
	if len(suffix.Segments) == 0 {
		return nil, false
	}
	keys := make([]typetable.ConstructorKey, 0, len(suffix.Segments))
	for _, seg := range suffix.Segments {
		switch seg.Kind {
		case segment.SegmentField:
			keys = append(keys, typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: seg.Name})
		case segment.SegmentIndexString:
			keys = append(keys, typetable.ConstructorKey{Kind: typetable.ConstructorStringIndex, Name: seg.Name})
		case segment.SegmentIndexInt:
			keys = append(keys, typetable.ConstructorKey{Kind: typetable.ConstructorIntIndex, Index: int64(seg.Index)})
		default:
			return nil, false
		}
	}
	return keys, true
}

func addWIRCallResultSymbolTypes(out map[symbol.ID]typ.Type, body *wir.Body, inst wir.Instruction) {
	if out == nil || body == nil || inst.Op != wir.OpCall {
		return
	}
	fn, ok := callableFromWIRCallType(body, inst)
	if !ok && inst.Type == 0 {
		fn, ok = callableFromWIRCallPathType(out, body, inst)
	}
	if !ok || fn == nil || len(fn.TypeParams) != 0 {
		return
	}
	for _, target := range body.CallResultTargets(inst.Point) {
		if target.Kind != wir.CallResultTargetLocalAssignment ||
			target.Path.Symbol == 0 ||
			len(target.Path.Segments) != 0 ||
			target.ResultIndex < 0 ||
			target.ResultIndex >= len(fn.Returns) ||
			fn.Returns[target.ResultIndex] == nil {
			continue
		}
		out[target.Path.Symbol] = fn.Returns[target.ResultIndex]
	}
}

func callableFromWIRCallPathType(symbolTypes map[symbol.ID]typ.Type, body *wir.Body, inst wir.Instruction) (*typ.Function, bool) {
	if body == nil || inst.Op != wir.OpCall || inst.Call.Callee.Kind != wir.OperandPath {
		return nil, false
	}
	p := body.Path(wir.PathRef(inst.Call.Callee.Ref))
	t, ok := wirPathTypeFromSymbols(symbolTypes, p)
	if !ok {
		return nil, false
	}
	fn, ok := t.(*typ.Function)
	return fn, ok && fn != nil
}

func callableFromWIRCallType(body *wir.Body, inst wir.Instruction) (*typ.Function, bool) {
	t := body.Type(inst.Type)
	if t == nil {
		return nil, false
	}
	if inst.Call.Method != 0 {
		method := body.Const(inst.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, false
		}
		fn, _, ok := typecall.MemberCallable(t, method.Str)
		return fn, ok
	}
	return typecall.Callable(t)
}

func functionDefinitionFactAt(meta cfgfacts.Metadata, point cfg.Point) (cfgfacts.FunctionDefinitionFact, bool) {
	return meta.FunctionDefinition(point)
}

func numericForFactAt(meta cfgfacts.Metadata, point cfg.Point) (cfgfacts.NumericForFact, bool) {
	return meta.NumericFor(point)
}

func numericForSymbolType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, init, limit, step ast.Expr) typ.Type {
	if numericForControlExprIsInteger(symbolTypes, bindings, init) &&
		numericForControlExprIsInteger(symbolTypes, bindings, limit) &&
		numericForControlExprIsInteger(symbolTypes, bindings, step) {
		return typ.Integer
	}
	return typ.Number
}

func numericForControlExprIsInteger(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) bool {
	if expr == nil {
		return true
	}
	t, ok := numericForControlExprType(symbolTypes, bindings, expr)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && subtype.IsSubtype(t, typ.Integer)
}

func numericForControlExprType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	switch e := inner.(type) {
	case *ast.NumberExpr, *ast.StringExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr:
		return valueexpr.LiteralType(e)
	case *ast.UnaryMinusOpExpr:
		operand, ok := numericForControlExprType(symbolTypes, bindings, e.Expr)
		if !ok {
			return nil, false
		}
		return typeoperator.UnaryOp("-", operand)
	case *ast.UnaryLenOpExpr:
		operand, ok := numericForControlExprType(symbolTypes, bindings, e.Expr)
		if !ok {
			return typ.Integer, true
		}
		return typeoperator.UnaryOp("#", operand)
	case *ast.ArithmeticOpExpr:
		left, ok := numericForControlExprType(symbolTypes, bindings, e.Lhs)
		if !ok {
			return nil, false
		}
		right, ok := numericForControlExprType(symbolTypes, bindings, e.Rhs)
		if !ok {
			return nil, false
		}
		return typeoperator.BinaryOp(left, e.Operator, right)
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return expressionTypeFromSymbols(symbolTypes, bindings, e)
	case *ast.FuncCallExpr:
		return callFirstReturnType(symbolTypes, bindings, e)
	default:
		return nil, false
	}
}

func functionExpressionType(fn *ast.FunctionExpr, bindings *bind.Result, resolver *typeresolve.Resolver) (typ.Type, bool) {
	return functiontype.Expression(fn, bindings, resolver)
}

func callFirstReturnType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return nil, false
	}
	if call.Method != "" && call.Receiver != nil {
		receiver, ok := expressionTypeFromSymbols(symbolTypes, bindings, call.Receiver)
		if !ok {
			return nil, false
		}
		fn, _, ok := typecall.MemberCallable(receiver, call.Method)
		if !ok {
			return nil, false
		}
		return nonGenericFunctionFirstReturn(fn)
	}
	callee, ok := expressionTypeFromSymbols(symbolTypes, bindings, call.Func)
	if !ok {
		return nil, false
	}
	fn, ok := typecall.Callable(callee)
	if !ok {
		return nil, false
	}
	return nonGenericFunctionFirstReturn(fn)
}

func nonGenericFunctionFirstReturn(fn *typ.Function) (typ.Type, bool) {
	if fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		return nil, false
	}
	return fn.Returns[0], true
}

func expressionTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	if t, ok := accessChainType(symbolTypes, bindings, expr); ok {
		return t, true
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr != nil && attr.KeySyntax == ast.AttrKeyIndex {
		container, ok := expressionTypeFromSymbols(symbolTypes, bindings, attr.Object)
		if !ok {
			return nil, false
		}
		key, ok := staticIndexKeyType(symbolTypes, bindings, attr)
		if !ok {
			return nil, false
		}
		return access.RuntimeIndex(container, key)
	}
	return nil, false
}

func objectLiteralTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, resolver *typeresolve.Resolver, expr ast.Expr) (typ.Type, bool) {
	table, ok := expr.(*ast.TableExpr)
	if !ok || table == nil || resolver == nil {
		return nil, false
	}
	builder := typetable.NewConstructorBuilder()
	seen := false
	for _, field := range table.Fields {
		key, ok := constructorKeyFromField(field)
		if !ok {
			continue
		}
		valueType, ok := constructorValueTypeFromSymbols(symbolTypes, bindings, resolver, field.Value)
		if !ok || valueType == nil {
			continue
		}
		if !builder.Add([]typetable.ConstructorKey{key}, valueType) {
			return nil, false
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build()
}

func constructorKeyFromField(field *ast.Field) (typetable.ConstructorKey, bool) {
	if field == nil || field.Key == nil {
		return typetable.ConstructorKey{}, false
	}
	switch key := field.Key.(type) {
	case *ast.IdentExpr:
		if field.KeySyntax != ast.AttrKeyDot {
			return typetable.ConstructorKey{}, false
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: key.Value}, true
	case *ast.StringExpr:
		if field.KeySyntax == ast.AttrKeyDot {
			return typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: key.Value}, true
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorStringIndex, Name: key.Value}, true
	case *ast.NumberExpr:
		t, ok := valueexpr.LiteralType(key)
		if !ok {
			return typetable.ConstructorKey{}, false
		}
		if lit, ok := t.(*typ.Literal); ok && lit.Base == kind.Integer {
			if n, ok := lit.Value.(int64); ok {
				return typetable.ConstructorKey{Kind: typetable.ConstructorIntIndex, Index: n}, true
			}
		}
		return typetable.ConstructorKey{}, false
	default:
		return typetable.ConstructorKey{}, false
	}
}

func constructorValueTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, resolver *typeresolve.Resolver, expr ast.Expr) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.CastExpr:
		return resolver.Type(e.Type)
	case *ast.NonNilAssertExpr:
		return constructorValueTypeFromSymbols(symbolTypes, bindings, resolver, e.Expr)
	case *ast.TableExpr:
		return objectLiteralTypeFromSymbols(symbolTypes, bindings, resolver, e)
	default:
		return expressionTypeFromSymbols(symbolTypes, bindings, expr)
	}
}

// accessChainType resolves the type of a static field/index access expression
// rooted at a symbol whose type is known.
func accessChainType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	resolved, ok := pathexpr.Resolve(expr, bindings)
	if !ok || resolved.Symbol == 0 {
		return nil, false
	}
	rootType, ok := symbolTypes[resolved.Symbol]
	if !ok || rootType == nil {
		return nil, false
	}
	if len(resolved.Segments) == 0 {
		return rootType, true
	}
	return typeprojection.ApplySegments(rootType, resolved.Segments)
}

func staticIndexKeyType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(key.Value), true
	case *ast.NumberExpr:
		return typ.Number, true
	case *ast.IdentExpr:
		id, ok := bindings.SymbolOf(key)
		if !ok || id == 0 {
			return nil, false
		}
		t, ok := symbolTypes[id]
		return t, ok && t != nil
	default:
		return nil, false
	}
}
