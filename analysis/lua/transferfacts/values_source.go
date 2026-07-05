package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

func (l *lowerer) valueSources(sources []sourceprovenance.ASTSource) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) returnValueSources(sources []sourceprovenance.ASTSource, result *semantics.Result) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, 0, len(sources))
	for _, source := range sources {
		for _, expanded := range l.expandTypeIsOpenTailReturnSource(source, result) {
			out = append(out, l.valueSource(expanded))
		}
	}
	return out
}

func (l *lowerer) returnValueSourcesFromWIR(point cfg.Point) ([]factflow.ValueSource, bool) {
	ret, ok := l.wirReturnInstruction(point)
	if !ok {
		return nil, false
	}
	ops := l.wir.Operands(ret.List)
	out := make([]factflow.ValueSource, len(ops))
	resultSources := l.resultValueSourcesByTempFromWIR()
	for i, op := range ops {
		final := i == len(ops)-1
		source, ok := l.returnValueSourceFromWIROperand(point, op, i, i, final, ret.ListSpread && final, ret.ListSpread && final, resultSources)
		if !ok {
			return nil, false
		}
		out[i] = source
	}
	return out, true
}

func (l *lowerer) wirReturnInstruction(point cfg.Point) (wir.Instruction, bool) {
	if l == nil || l.wir == nil {
		return wir.Instruction{}, false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op == wir.OpReturn {
			return inst, true
		}
	}
	return wir.Instruction{}, false
}

func (l *lowerer) returnValueSourceFromWIROperand(
	point cfg.Point,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
) (factflow.ValueSource, bool) {
	if source, ok := l.valueSourceFromWIRRootPathOperand(op, exprIndex, targetIndex, final, symbol.Local, symbol.Param); ok {
		return source, true
	}
	if source, ok := l.pathExpressionSourceFromWIR("return", point, op, exprIndex, targetIndex, final, expanded, openTail, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
		return source, true
	}
	return l.valueSourceFromWIROperand(op, exprIndex, targetIndex, final, expanded, openTail, resultSources)
}

type wirResultSource struct {
	point       cfg.Point
	resultIndex int
	exprID      wir.ExpressionID
}

type wirPathExprRefKey struct {
	kind        string
	point       cfg.Point
	path        path.PathKey
	exprIndex   int
	targetIndex int
	final       bool
	expanded    bool
	openTail    bool
}

func (l *lowerer) localRootPathExpressionSourceFromWIR(
	kind string,
	point cfg.Point,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	return l.rootPathExpressionSourceFromWIR(kind, point, op, exprIndex, targetIndex, final, expanded, openTail, symbol.Local)
}

func (l *lowerer) rootPathExpressionSourceFromWIR(
	kind string,
	point cfg.Point,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	allowedKinds ...symbol.Kind,
) (factflow.ValueSource, bool) {
	if op.Kind != wir.OperandPath || l == nil || l.wir == nil || l.bindings == nil {
		return factflow.ValueSource{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if len(p.Segments) != 0 || p.Symbol == 0 {
		return factflow.ValueSource{}, false
	}
	bindKind, ok := l.bindings.Kind(p.Symbol)
	if !ok || !symbolKindAllowed(bindKind, allowedKinds) {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirPathExprRefKey{
		kind:        kind,
		point:       point,
		path:        p.Key(),
		exprIndex:   exprIndex,
		targetIndex: targetIndex,
		final:       final,
		expanded:    expanded,
		openTail:    openTail,
	})
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]path.Path)
	}
	l.expressionPaths[exprRef] = p
	if t, ok := l.symbolTypes[p.Symbol]; ok {
		if l.expressionValues == nil {
			l.expressionValues = make(map[factflow.ExprRef]product.Value)
		}
		l.expressionValues[exprRef] = l.valueFromTypeWithWitness(t)
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) pathExpressionSourceFromWIR(
	kind string,
	point cfg.Point,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	allowedKinds ...symbol.Kind,
) (factflow.ValueSource, bool) {
	if op.Kind != wir.OperandPath || l == nil || l.wir == nil || l.bindings == nil {
		return factflow.ValueSource{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() || p.Symbol == 0 {
		return factflow.ValueSource{}, false
	}
	bindKind, ok := l.bindings.Kind(p.Symbol)
	if !ok || !symbolKindAllowed(bindKind, allowedKinds) {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirPathExprRefKey{
		kind:        kind,
		point:       point,
		path:        p.Key(),
		exprIndex:   exprIndex,
		targetIndex: targetIndex,
		final:       final,
		expanded:    expanded,
		openTail:    openTail,
	})
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]path.Path)
	}
	l.expressionPaths[exprRef] = p
	if len(p.Segments) == 0 {
		if t, ok := l.symbolTypes[p.Symbol]; ok {
			if l.expressionValues == nil {
				l.expressionValues = make(map[factflow.ExprRef]product.Value)
			}
			l.expressionValues[exprRef] = l.valueFromTypeWithWitness(t)
		}
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) valueSourceFromWIROperand(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
) (factflow.ValueSource, bool) {
	return l.valueSourceFromWIROperandSeen(op, exprIndex, targetIndex, final, expanded, openTail, resultSources, nil)
}

func (l *lowerer) valueSourceFromWIROperandSeen(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	switch op.Kind {
	case wir.OperandPath:
		return l.valueSourceFromWIRRootPathOperand(op, exprIndex, targetIndex, final, symbol.Param)
	case wir.OperandConst:
		c := l.wir.Const(wir.ConstRef(op.Ref))
		if c.Kind == wir.ConstNil {
			return factflow.NewNilValueSource(targetIndex), true
		}
		shape, ok := factflow.NewValueSourceShape(final, false, false, false)
		if !ok {
			return factflow.ValueSource{}, false
		}
		switch c.Kind {
		case wir.ConstBool:
			return mustValueSource(factflow.NewBoolLiteralValueSource(c.Bool, exprIndex, targetIndex, 0, shape)), true
		case wir.ConstNumber:
			if i, ok := numparse.ParseIntegerLiteral(c.Number); ok {
				return mustValueSource(factflow.NewIntegerLiteralValueSource(i, exprIndex, targetIndex, 0, shape)), true
			}
			if f, ok := numparse.ParseFloatLiteral(c.Number); ok {
				return mustValueSource(factflow.NewNumberLiteralValueSource(f, exprIndex, targetIndex, 0, shape)), true
			}
			return factflow.ValueSource{}, false
		case wir.ConstString:
			return mustValueSource(factflow.NewStringLiteralValueSource(c.Str, exprIndex, targetIndex, 0, shape)), true
		}
	case wir.OperandVararg:
		shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
		if !ok {
			return factflow.ValueSource{}, false
		}
		return mustValueSource(factflow.NewVarargValueSource(0, exprIndex, targetIndex, 0, shape)), true
	case wir.OperandTemp:
		if source, ok := resultValueSourceFromWIR(op, exprIndex, targetIndex, final, expanded, openTail, resultSources); ok {
			return source, true
		}
		if source, ok := l.wirTempExpressionValueSource(op, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen); ok {
			return source, true
		}
	}
	return factflow.ValueSource{}, false
}

type wirTempExprRefKey struct {
	temp uint32
}

func (l *lowerer) wirTempExpressionValueSource(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if op.Kind != wir.OperandTemp || l == nil || l.wir == nil {
		return factflow.ValueSource{}, false
	}
	if seen == nil {
		seen = make(map[uint32]bool)
	}
	if seen[op.Ref] {
		return factflow.ValueSource{}, false
	}
	seen[op.Ref] = true
	defer delete(seen, op.Ref)
	inst, ok := l.wirTempDefs()[op.Ref]
	if !ok {
		return factflow.ValueSource{}, false
	}
	switch inst.Op {
	case wir.OpAssign:
		return l.valueSourceFromWIROperandSeen(inst.A, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpBinOp, wir.OpLogical, wir.OpConcat:
		return l.wirBinaryTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpUnOp:
		return l.wirUnaryTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpClaim:
		return l.wirClaimTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpClosure:
		return l.wirClosureTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail)
	case wir.OpMakeTable:
		return l.wirTableExpressionValueSource(inst, exprIndex, targetIndex, final, expanded, openTail)
	default:
		return factflow.ValueSource{}, false
	}
}

func (l *lowerer) wirTableExpressionValueSource(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.tableConstructorExprRefFromWIR(inst)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.tableExpressionValueSource(exprRef, exprIndex, targetIndex, final, expanded, openTail)
}

func (l *lowerer) tableConstructorExpressionValueSource(
	expr ast.Expr,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	if _, ok := l.tableConstructorExpressionID(expr); !ok {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.tableConstructorExprRef(expr)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.tableExpressionValueSource(exprRef, exprIndex, targetIndex, final, expanded, openTail)
}

func (l *lowerer) tableExpressionValueSource(
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	value := product.NewWithPresence(l.registry, product.ShapeTop, presence.Present())
	value = product.Set(l.registry, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(l.registry, value, identity.Key, identity.Singleton(identity.LuaTableLiteral(l.graphID, uint64(exprRef))))
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[exprRef] = value
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirClosureTempExpressionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	if l == nil || l.wir == nil || inst.Func == 0 {
		return factflow.ValueSource{}, false
	}
	proto := l.wir.Proto(inst.Func)
	if proto.Symbol == 0 {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.expressionFunctions == nil {
		l.expressionFunctions = make(map[factflow.ExprRef]symbol.ID)
	}
	l.expressionFunctions[exprRef] = proto.Symbol
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	value := product.NewWithPresence(l.registry, product.ShapeTop, presence.Present())
	value = product.Set(l.registry, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	if proto.Type != nil {
		value = l.valueFromTypeWithWitness(proto.Type)
	}
	value = product.Set(l.registry, value, identity.Key, identity.Singleton(identity.LuaFunction(uint64(proto.Symbol))))
	l.expressionValues[exprRef] = value
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirClaimTempExpressionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	inner, ok := l.wirClaimInnerValueSource(inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	source, ok := factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
	if !ok {
		return factflow.ValueSource{}, false
	}
	l.addWIRClaimExpressionPath(exprRef, inst)
	if !l.recordExpressionRefinementFromWIRClaim(source, inner, inst) {
		return factflow.ValueSource{}, false
	}
	l.addWIRClaimExpressionValue(exprRef, inst)
	return source, true
}

func (l *lowerer) wirClaimInnerValueSource(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if source, ok := l.valueSourceFromWIRRootPathOperand(inst.A, exprIndex, targetIndex, final, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
		return source, true
	}
	if source, ok := l.valueSourceFromWIROperandSeen(inst.A, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen); ok {
		return source, true
	}
	return l.pathExpressionSourceFromWIR("claim-inner", inst.Point, inst.A, exprIndex, targetIndex, final, expanded, openTail, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue)
}

func (l *lowerer) addWIRClaimExpressionPath(exprRef factflow.ExprRef, inst wir.Instruction) {
	if exprRef == 0 || inst.A.Kind != wir.OperandPath || l == nil || l.wir == nil {
		return
	}
	p := l.wir.Path(wir.PathRef(inst.A.Ref))
	if p.IsEmpty() {
		return
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]path.Path)
	}
	l.expressionPaths[exprRef] = p
}

func (l *lowerer) addWIRClaimExpressionValue(exprRef factflow.ExprRef, inst wir.Instruction) {
	if exprRef == 0 || inst.Claim != wir.ClaimCast {
		return
	}
	t := l.wir.Type(inst.Type)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return
	}
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[exprRef] = l.valueFromTypeWithWitness(t)
}

func (l *lowerer) wirBinaryTempExpressionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	op, ok := wirExpressionOperator(inst)
	if !ok {
		return factflow.ValueSource{}, false
	}
	leftOp, rightOp, ok := wirBinaryExpressionOperands(l.wir, inst)
	if !ok {
		return factflow.ValueSource{}, false
	}
	left, ok := l.wirBinaryExpressionOperandValueSource(inst, leftOp, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	right, ok := l.wirBinaryExpressionOperandValueSource(inst, rightOp, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	operation, ok := factflow.NewBinaryExpressionOperation(op, left, right)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.expressionOperations == nil {
		l.expressionOperations = make(map[factflow.ExprRef]factflow.ExpressionOperation)
	}
	l.expressionOperations[exprRef] = operation
	l.addWIRExpressionCondition(exprRef, inst)
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirUnaryTempExpressionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	op, ok := wirExpressionOperator(inst)
	if !ok {
		return factflow.ValueSource{}, false
	}
	operand, ok := l.wirInstructionExpressionOperandValueSource(inst, inst.A, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	operation, ok := factflow.NewUnaryExpressionOperation(op, operand)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.expressionOperations == nil {
		l.expressionOperations = make(map[factflow.ExprRef]factflow.ExpressionOperation)
	}
	l.expressionOperations[exprRef] = operation
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) addWIRExpressionCondition(exprRef factflow.ExprRef, inst wir.Instruction) {
	if exprRef == 0 {
		return
	}
	check, ok := l.wirExpressionConditionCheck(inst)
	if !ok {
		return
	}
	condition := factflow.NewExpressionCondition(
		postconditionRefinementsFromBranchEdge(l.branchEdgeRefinements(check, true), true),
		postconditionRefinementsFromBranchEdge(l.branchEdgeRefinements(check, false), false),
		nil,
		nil,
	)
	if condition.IsEmpty() {
		return
	}
	if l.expressionConditions == nil {
		l.expressionConditions = make(map[factflow.ExprRef]factflow.ExpressionCondition)
	}
	l.expressionConditions[exprRef] = condition
}

func postconditionRefinementsFromBranchEdge(refinements []factflow.BranchRefinement, edge bool) []factflow.PostconditionRefinement {
	if len(refinements) == 0 {
		return nil
	}
	out := make([]factflow.PostconditionRefinement, 0, len(refinements))
	for _, refinement := range refinements {
		value, ok := refinement.ValueForEdge(edge)
		if !ok {
			continue
		}
		out = append(out, factflow.NewPostconditionRefinement(refinement.TargetPath(), value))
	}
	return out
}

func (l *lowerer) wirExpressionConditionCheck(inst wir.Instruction) (branchcond.Check, bool) {
	if inst.Op != wir.OpBinOp || (inst.Operator != wir.BinEq && inst.Operator != wir.BinNe) {
		return branchcond.Check{}, false
	}
	if check, ok := l.wirEqualityConditionCheck(inst.A, inst.B, inst.Operator); ok {
		return check, true
	}
	return l.wirEqualityConditionCheck(inst.B, inst.A, inst.Operator)
}

func (l *lowerer) wirEqualityConditionCheck(pathOp wir.Operand, literalOp wir.Operand, op wir.Operator) (branchcond.Check, bool) {
	if pathOp.Kind != wir.OperandPath || literalOp.Kind != wir.OperandConst || l == nil || l.wir == nil {
		return branchcond.Check{}, false
	}
	p := l.wir.Path(wir.PathRef(pathOp.Ref))
	if p.IsEmpty() {
		return branchcond.Check{}, false
	}
	c := l.wir.Const(wir.ConstRef(literalOp.Ref))
	if c.Kind == wir.ConstNil {
		if op == wir.BinEq {
			return branchcond.Check{Kind: branchcond.CheckNil, Path: p}, true
		}
		return branchcond.Check{Kind: branchcond.CheckNotNil, Path: p}, true
	}
	lit, ok := wirLiteralType(c)
	if !ok {
		return branchcond.Check{}, false
	}
	kind := branchcond.CheckLiteralEqual
	if op == wir.BinNe {
		kind = branchcond.CheckLiteralNot
	}
	return branchcond.Check{Kind: kind, Path: p, Literal: lit}, true
}

func wirLiteralType(c wir.Const) (typ.Type, bool) {
	switch c.Kind {
	case wir.ConstBool:
		return typ.LiteralBool(c.Bool), true
	case wir.ConstNumber:
		if i, ok := numparse.ParseIntegerLiteral(c.Number); ok {
			return typ.LiteralInt(i), true
		}
		if f, ok := numparse.ParseFloatLiteral(c.Number); ok {
			return typ.LiteralNumber(f), true
		}
	case wir.ConstString:
		return typ.LiteralString(c.Str), true
	}
	return nil, false
}

func (l *lowerer) wirExpressionOperandValueSource(
	op wir.Operand,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	return l.valueSourceFromWIROperandSeen(op,
		sourceprovenance.NoSourceIndex,
		sourceprovenance.NoSourceIndex,
		true,
		false,
		false,
		resultSources,
		seen,
	)
}

func (l *lowerer) wirInstructionExpressionOperandValueSource(
	inst wir.Instruction,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if inst.Op == wir.OpConcat || (exprIndex == sourceprovenance.NoSourceIndex && targetIndex == sourceprovenance.NoSourceIndex) {
		if source, ok := l.pathExpressionSourceFromWIR("expr-op", inst.Point, op, sourceprovenance.NoSourceIndex, sourceprovenance.NoSourceIndex, true, false, false, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
			return source, true
		}
	}
	return l.wirExpressionOperandValueSource(op, resultSources, seen)
}

func (l *lowerer) wirBinaryExpressionOperandValueSource(
	inst wir.Instruction,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	return l.wirInstructionExpressionOperandValueSource(inst, op, exprIndex, targetIndex, resultSources, seen)
}

func wirBinaryExpressionOperands(body *wir.Body, inst wir.Instruction) (wir.Operand, wir.Operand, bool) {
	if inst.Op == wir.OpConcat && inst.List.Len != 0 {
		ops := body.Operands(inst.List)
		if len(ops) != 2 {
			return wir.Operand{}, wir.Operand{}, false
		}
		return ops[0], ops[1], true
	}
	if inst.A.Kind == wir.OperandNone || inst.B.Kind == wir.OperandNone {
		return wir.Operand{}, wir.Operand{}, false
	}
	return inst.A, inst.B, true
}

func wirExpressionOperator(inst wir.Instruction) (string, bool) {
	if inst.Op == wir.OpConcat {
		return "..", true
	}
	switch inst.Operator {
	case wir.BinAdd:
		return "+", true
	case wir.BinSub:
		return "-", true
	case wir.BinMul:
		return "*", true
	case wir.BinDiv:
		return "/", true
	case wir.BinIDiv:
		return "//", true
	case wir.BinMod:
		return "%", true
	case wir.BinPow:
		return "^", true
	case wir.BinBAnd:
		return "&", true
	case wir.BinBOr:
		return "|", true
	case wir.BinBXor:
		return "~", true
	case wir.BinShl:
		return "<<", true
	case wir.BinShr:
		return ">>", true
	case wir.BinEq:
		return "==", true
	case wir.BinNe:
		return "~=", true
	case wir.BinLt:
		return "<", true
	case wir.BinLe:
		return "<=", true
	case wir.BinGt:
		return ">", true
	case wir.BinGe:
		return ">=", true
	case wir.UnNeg:
		return "-", true
	case wir.UnNot:
		return "not", true
	case wir.UnLen:
		return "#", true
	case wir.UnBNot:
		return "~", true
	case wir.LogAnd:
		return "and", true
	case wir.LogOr:
		return "or", true
	default:
		return "", false
	}
}

func (l *lowerer) valueSourceFromWIRRootPathOperand(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	allowedKinds ...symbol.Kind,
) (factflow.ValueSource, bool) {
	if op.Kind != wir.OperandPath || l == nil || l.wir == nil || l.bindings == nil {
		return factflow.ValueSource{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if len(p.Segments) != 0 {
		return factflow.ValueSource{}, false
	}
	if l.bindings.IsImplicitGlobalSymbol(p.Symbol) {
		return factflow.ValueSource{}, false
	}
	kind, ok := l.bindings.Kind(p.Symbol)
	if !ok || !symbolKindAllowed(kind, allowedKinds) {
		return factflow.ValueSource{}, false
	}
	shape, ok := factflow.NewValueSourceShape(final, false, false, false)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return mustValueSource(factflow.NewPathValueSource(p.Key(), exprIndex, targetIndex, 0, shape)), true
}

func symbolKindAllowed(kind symbol.Kind, allowed []symbol.Kind) bool {
	for _, candidate := range allowed {
		if kind == candidate {
			return true
		}
	}
	return false
}

func (l *lowerer) resultValueSourcesByTempFromWIR() map[uint32]wirResultSource {
	if l.wirResultSources != nil {
		return l.wirResultSources
	}
	out := make(map[uint32]wirResultSource)
	if l == nil || l.wir == nil {
		return out
	}
	for i := 0; i < l.wir.Len(); i++ {
		inst := l.wir.Instr(i)
		switch inst.Op {
		case wir.OpCall:
			results := l.wir.Operands(inst.Results)
			for resultIndex, result := range results {
				if result.Kind != wir.OperandTemp {
					continue
				}
				out[result.Ref] = wirResultSource{point: inst.Point, resultIndex: resultIndex, exprID: inst.ExprID}
			}
		case wir.OpSelect:
			if inst.Dst.Kind == wir.OperandTemp {
				out[inst.Dst.Ref] = wirResultSource{point: inst.Point, resultIndex: 0, exprID: inst.ExprID}
			}
		}
	}
	l.wirResultSources = out
	return out
}

type wirCallExprRefKey struct {
	id wir.ExpressionID
}

func resultValueSourceFromWIR(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
) (factflow.ValueSource, bool) {
	if op.Kind != wir.OperandTemp {
		return factflow.ValueSource{}, false
	}
	result, ok := resultSources[op.Ref]
	if !ok {
		return factflow.ValueSource{}, false
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return mustValueSource(factflow.NewCallValueSource(0, exprIndex, targetIndex, result.resultIndex, result.point, shape)), true
}

func (l *lowerer) expandTypeIsOpenTailReturnSource(source sourceprovenance.ASTSource, result *semantics.Result) []sourceprovenance.ASTSource {
	if source.Kind != sourceprovenance.SourceCall || !source.OpenTail || !source.Expanded ||
		!source.HasCallPoint || result == nil {
		return []sourceprovenance.ASTSource{source}
	}
	view, ok := result.CallView(source.CallPoint)
	if !ok {
		return []sourceprovenance.ASTSource{source}
	}
	fact, _ := view.Borrowed()
	if _, _, ok := l.typeIsCall(fact); !ok {
		return []sourceprovenance.ASTSource{source}
	}
	value := source
	value.OpenTail = false
	errorSource := source
	errorSource.TargetIndex = source.TargetIndex + 1
	errorSource.ResultIndex = source.ResultIndex + 1
	errorSource.OpenTail = false
	return []sourceprovenance.ASTSource{value, errorSource}
}

func (l *lowerer) valueSource(source sourceprovenance.ASTSource) factflow.ValueSource {
	if source.Kind == sourceprovenance.SourceExpression {
		if tableSource, ok := l.tableConstructorExpressionValueSource(
			source.Expr,
			source.ExprIndex,
			source.TargetIndex,
			source.Final,
			source.Expanded,
			source.OpenTail,
		); ok {
			return tableSource
		}
	}
	exprRef, hasExpr := l.valueSourceExprRef(source)
	if hasExpr {
		l.addExpressionPath(exprRef, source.Expr)
		l.addExpressionCondition(exprRef, source.Expr)
		if source.Kind == sourceprovenance.SourceExpression {
			l.addExpressionValue(exprRef, source.Expr)
		}
	}
	shape, ok := factflow.NewValueSourceShape(source.Final, source.Expanded, source.Adjusted, source.OpenTail)
	if !ok {
		panic("transferfacts: invalid value source shape")
	}
	switch source.Kind {
	case sourceprovenance.SourceExpression:
		return mustValueSource(factflow.NewExpressionValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape))
	case sourceprovenance.SourceCall:
		if !source.HasCallPoint {
			return factflow.NewUnknownValueSource(source.TargetIndex)
		}
		return mustValueSource(factflow.NewCallValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, source.CallPoint, shape))
	case sourceprovenance.SourceVararg:
		return mustValueSource(factflow.NewVarargValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape))
	case sourceprovenance.SourceNil:
		return factflow.NewNilValueSource(source.TargetIndex)
	case sourceprovenance.SourceUnknown:
		return factflow.NewUnknownValueSource(source.TargetIndex)
	default:
		panic("transferfacts: unknown value source kind")
	}
}

func mustValueSource(source factflow.ValueSource, ok bool) factflow.ValueSource {
	if !ok {
		panic("transferfacts: invalid value source")
	}
	return source
}
