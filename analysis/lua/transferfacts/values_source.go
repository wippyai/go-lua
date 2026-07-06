package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
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
			source = factflow.NewUnknownValueSource(i)
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
	p, ok := l.wirPathOperand(op, true, allowedKinds...)
	if !ok {
		return factflow.ValueSource{}, false
	}
	var witness typ.Type
	if t, ok := l.symbolTypes[p.Symbol]; ok {
		witness = t
	}
	return l.wirPathExpressionSource(kind, point, p, witness, exprIndex, targetIndex, final, expanded, openTail)
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
	p, ok := l.wirPathOperand(op, false, allowedKinds...)
	if !ok {
		return factflow.ValueSource{}, false
	}
	witness, _ := l.aliasPathType(p)
	return l.wirPathExpressionSource(kind, point, p, witness, exprIndex, targetIndex, final, expanded, openTail)
}

func (l *lowerer) wirPathExpressionSource(
	kind string,
	point cfg.Point,
	p path.Path,
	witness typ.Type,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
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
	if witness != nil {
		if l.expressionValues == nil {
			l.expressionValues = make(map[factflow.ExprRef]product.Value)
		}
		l.expressionValues[exprRef] = l.valueFromTypeWithWitness(witness)
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirPathOperand(op wir.Operand, rootOnly bool, allowedKinds ...symbol.Kind) (path.Path, bool) {
	if op.Kind != wir.OperandPath || l == nil || l.wir == nil || l.bindings == nil {
		return path.Path{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() || p.Symbol == 0 {
		return path.Path{}, false
	}
	if rootOnly && len(p.Segments) != 0 {
		return path.Path{}, false
	}
	bindKind, ok := l.bindings.Kind(p.Symbol)
	if !ok || !symbolKindAllowed(bindKind, allowedKinds) {
		return path.Path{}, false
	}
	return p, true
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
	if source, ok := l.wirMultiDefTempExpressionValueSource(op.Ref, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen); ok {
		return source, true
	}
	inst, ok := l.wirTempDefs()[op.Ref]
	if !ok {
		return factflow.ValueSource{}, false
	}
	switch inst.Op {
	case wir.OpAssign:
		return l.valueSourceFromWIROperandSeen(inst.A, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpDynamicIndexRead:
		return l.wirDynamicIndexReadTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpConcat:
		return l.wirConcatTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	case wir.OpBinOp, wir.OpLogical:
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

func (l *lowerer) wirMultiDefTempExpressionValueSource(
	temp uint32,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	defs := l.wirTempDefSets()[temp]
	if len(defs) != 2 || l.graph == nil {
		return factflow.ValueSource{}, false
	}
	leftDef, rhsDef, op, ok := l.wirLogicalTempDefs(defs)
	if !ok {
		return factflow.ValueSource{}, false
	}
	left, ok := l.wirInstructionExpressionOperandValueSource(leftDef, leftDef.A, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	right, ok := l.wirDefinitionValueSource(temp, rhsDef, exprIndex, targetIndex, resultSources, seen)
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
	if !l.addWIRLogicalExpressionOperationValue(exprRef, operation, left, right) {
		l.addWIRExpressionOperationValue(exprRef, operation, left, right)
	}
	l.addWIRLogicalExpressionCondition(exprRef, op, left, right)
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirLogicalTempDefs(defs []wir.Instruction) (wir.Instruction, wir.Instruction, string, bool) {
	if leftDef, rhsDef, op, ok := l.wirLogicalTempDefsOrdered(defs[0], defs[1]); ok {
		return leftDef, rhsDef, op, true
	}
	return l.wirLogicalTempDefsOrdered(defs[1], defs[0])
}

func (l *lowerer) wirLogicalTempDefsOrdered(leftDef, rhsDef wir.Instruction) (wir.Instruction, wir.Instruction, string, bool) {
	if leftDef.Op != wir.OpAssign || !wirInstructionDefinesTemp(rhsDef, leftDef.Dst.Ref) {
		return wir.Instruction{}, wir.Instruction{}, "", false
	}
	branch, ok := l.wirLogicalBranchForLeftDef(leftDef)
	if !ok {
		return wir.Instruction{}, wir.Instruction{}, "", false
	}
	edge, ok := l.wirEdgeFromBranchToPoint(branch.Point, rhsDef.Point)
	if !ok {
		return wir.Instruction{}, wir.Instruction{}, "", false
	}
	if edge {
		return leftDef, rhsDef, "and", true
	}
	return leftDef, rhsDef, "or", true
}

func (l *lowerer) wirDefinitionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	switch inst.Op {
	case wir.OpAssign:
		return l.wirInstructionExpressionOperandValueSource(inst, inst.A, exprIndex, targetIndex, resultSources, seen)
	case wir.OpDynamicIndexRead:
		return l.wirDynamicIndexReadTempExpressionValueSource(temp, inst, exprIndex, targetIndex, true, false, false, resultSources, seen)
	case wir.OpConcat:
		return l.wirConcatTempExpressionValueSource(temp, inst, exprIndex, targetIndex, true, false, false, resultSources, seen)
	case wir.OpBinOp, wir.OpLogical:
		return l.wirBinaryTempExpressionValueSource(temp, inst, exprIndex, targetIndex, true, false, false, resultSources, seen)
	case wir.OpUnOp:
		return l.wirUnaryTempExpressionValueSource(temp, inst, exprIndex, targetIndex, true, false, false, resultSources, seen)
	case wir.OpClaim:
		return l.wirClaimTempExpressionValueSource(temp, inst, exprIndex, targetIndex, true, false, false, resultSources, seen)
	case wir.OpClosure:
		return l.wirClosureTempExpressionValueSource(temp, inst, exprIndex, targetIndex, true, false, false)
	case wir.OpMakeTable:
		return l.wirTableExpressionValueSource(inst, exprIndex, targetIndex, true, false, false)
	default:
		return factflow.ValueSource{}, false
	}
}

func (l *lowerer) wirLogicalBranchForLeftDef(def wir.Instruction) (wir.Instruction, bool) {
	for _, inst := range l.wir.PointInstructions(def.Point) {
		if inst.Op != wir.OpBranch {
			continue
		}
		if l.wirBranchReadsOperand(inst, def.A) {
			return inst, true
		}
	}
	return wir.Instruction{}, false
}

func (l *lowerer) wirBranchReadsOperand(branch wir.Instruction, op wir.Operand) bool {
	if branch.A.Kind != wir.OperandNone {
		if wirOperandEqual(branch.A, op) {
			return true
		}
		if branch.A.Kind == wir.OperandTemp && op.Kind == wir.OperandTemp && l.wirTempsHaveEquivalentAssignDefs(branch.A.Ref, op.Ref) {
			return true
		}
		return wirOperandEqual(branch.A, op)
	}
	if op.Kind == wir.OperandTemp && branch.Check != 0 {
		def, ok := l.wirTempDefs()[op.Ref]
		if ok && def.Check != 0 {
			return wirChecksEquivalent(l.wir.Check(branch.Check), l.wir.Check(def.Check))
		}
	}
	if op.Kind != wir.OperandPath || l == nil || l.wir == nil {
		return false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() {
		return false
	}
	check := l.wir.Check(branch.Check)
	switch check.Kind {
	case wir.CheckTruthy, wir.CheckFalsy, wir.CheckNil, wir.CheckNotNil, wir.CheckTypeEqual, wir.CheckTypeNot, wir.CheckLiteralEqual, wir.CheckLiteralNot:
		return check.Path.Equal(p)
	default:
		return false
	}
}

func (l *lowerer) wirTempsHaveEquivalentAssignDefs(left, right uint32) bool {
	if l == nil || left == right {
		return left == right
	}
	leftDefs := l.wirTempDefSets()[left]
	rightDefs := l.wirTempDefSets()[right]
	if len(leftDefs) == 0 || len(leftDefs) != len(rightDefs) {
		return false
	}
	used := make([]bool, len(rightDefs))
	for _, leftDef := range leftDefs {
		if leftDef.Op != wir.OpAssign {
			return false
		}
		found := false
		for i, rightDef := range rightDefs {
			if used[i] || rightDef.Op != wir.OpAssign || leftDef.Point != rightDef.Point || !wirOperandEqual(leftDef.A, rightDef.A) {
				continue
			}
			used[i] = true
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func (l *lowerer) wirEdgeFromBranchToPoint(branch, target cfg.Point) (bool, bool) {
	if l.graph == nil {
		return false, false
	}
	var out bool
	var have bool
	if l.wirReachability == nil {
		l.wirReachability = cfg.NewReachability(l.graph)
	}
	for _, succ := range cfg.SuccessorsReadOnly(l.graph, branch) {
		edge, ok := l.graph.EdgeCond(branch, succ)
		if !ok {
			continue
		}
		if succ != target && !l.wirReachability.CanReach(succ, target) {
			continue
		}
		if have {
			return false, false
		}
		out = edge
		have = true
	}
	return out, have
}

func wirOperandEqual(a, b wir.Operand) bool {
	return a.Kind == b.Kind && a.Ref == b.Ref
}

func wirChecksEquivalent(a, b wir.Check) bool {
	if a.Kind != b.Kind ||
		a.TypeName != b.TypeName ||
		a.LiteralString != b.LiteralString ||
		a.LenFloor != b.LenFloor ||
		a.NumFloor != b.NumFloor ||
		a.Negated != b.Negated ||
		!a.Path.Equal(b.Path) ||
		!a.OtherPath.Equal(b.OtherPath) {
		return false
	}
	if a.Literal == nil || b.Literal == nil {
		return a.Literal == nil && b.Literal == nil
	}
	return typ.TypeEquals(a.Literal, b.Literal)
}

func wirInstructionDefinesTemp(inst wir.Instruction, temp uint32) bool {
	if inst.Dst.Kind == wir.OperandTemp && inst.Dst.Ref == temp {
		return true
	}
	return false
}

type wirConcatFoldExprRefKey struct {
	temp  uint32
	index int
}

func (l *lowerer) wirConcatTempExpressionValueSource(
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
	ops := wirConcatOperands(l.wir, inst)
	if len(ops) == 2 {
		return l.wirBinaryTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
	}
	if len(ops) < 2 {
		return factflow.ValueSource{}, false
	}
	sources := make([]factflow.ValueSource, len(ops))
	for i, operand := range ops {
		source, ok := l.wirInstructionExpressionOperandValueSource(inst, operand, exprIndex, targetIndex, resultSources, seen)
		if !ok {
			return factflow.ValueSource{}, false
		}
		sources[i] = source
	}
	finalRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	foldSource := sources[0]
	for i := 1; i < len(sources); i++ {
		ref := finalRef
		if i != len(sources)-1 {
			ref, ok = l.exprRef(wirConcatFoldExprRefKey{temp: temp, index: i})
			if !ok {
				return factflow.ValueSource{}, false
			}
		}
		operation, ok := factflow.NewBinaryExpressionOperation("..", foldSource, sources[i])
		if !ok {
			return factflow.ValueSource{}, false
		}
		if l.expressionOperations == nil {
			l.expressionOperations = make(map[factflow.ExprRef]factflow.ExpressionOperation)
		}
		l.expressionOperations[ref] = operation
		l.addWIRExpressionOperationValue(ref, operation, foldSource, sources[i])
		foldShape, ok := factflow.NewValueSourceShape(true, false, false, false)
		if !ok {
			return factflow.ValueSource{}, false
		}
		foldSource, ok = factflow.NewExpressionValueSource(ref, sourceprovenance.NoSourceIndex, sourceprovenance.NoSourceIndex, 0, foldShape)
		if !ok {
			return factflow.ValueSource{}, false
		}
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(finalRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirDynamicIndexReadTempExpressionValueSource(
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
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirDynamicIndexReadExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
}

type wirDynamicIndexReadExprRefKey struct {
	id wir.ExpressionID
}

func (l *lowerer) wirDynamicIndexReadExpressionValueSource(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if inst.ExprID == 0 {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirDynamicIndexReadExprRefKey{id: inst.ExprID})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirDynamicIndexReadExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, resultSources, seen)
}

func (l *lowerer) wirDynamicIndexReadExpressionValueSourceWithRef(
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	tableSource, ok := l.wirDynamicIndexReadOperandSource(inst, inst.A, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	keySource, ok := l.wirDynamicIndexReadOperandSource(inst, inst.B, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	dynamicExpr, ok := l.wirDynamicIndexReadExpression(inst, tableSource, keySource)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.dynamicIndexExpressions == nil {
		l.dynamicIndexExpressions = make(map[factflow.ExprRef]factflow.DynamicIndexExpression)
	}
	l.dynamicIndexExpressions[exprRef] = dynamicExpr
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirDynamicIndexReadOperandSource(
	inst wir.Instruction,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	resultSources map[uint32]wirResultSource,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if source, ok := l.pathExpressionSourceFromWIR(
		"dynamic-index-read",
		inst.Point,
		op,
		exprIndex,
		targetIndex,
		true,
		false,
		false,
		symbol.Local,
		symbol.Param,
		symbol.Global,
		symbol.Upvalue,
	); ok {
		return source, true
	}
	return l.wirInstructionExpressionOperandValueSource(inst, op, exprIndex, targetIndex, resultSources, seen)
}

func (l *lowerer) wirDynamicIndexReadExpression(
	inst wir.Instruction,
	tableSource factflow.ValueSource,
	keySource factflow.ValueSource,
) (factflow.DynamicIndexExpression, bool) {
	if inst.A.Kind == wir.OperandPath && l != nil && l.wir != nil {
		tablePath := l.wir.Path(wir.PathRef(inst.A.Ref))
		if !tablePath.IsEmpty() {
			expr, ok := factflow.NewDynamicIndexExpression(tablePath, keySource)
			if !ok {
				return factflow.DynamicIndexExpression{}, false
			}
			if tableSource.Valid() {
				expr = expr.WithTableSource(tableSource)
			}
			return expr, true
		}
	}
	return factflow.NewDynamicIndexExpressionFromSource(tableSource, keySource)
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
	if _, exists := l.expressionValues[exprRef]; !exists {
		l.expressionValues[exprRef] = value
	}
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
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirClosureExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail)
}

type wirClosureExprRefKey struct {
	id wir.ExpressionID
}

func (l *lowerer) wirClosureExpressionValueSource(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	if inst.ExprID == 0 {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirClosureExprRefKey{id: inst.ExprID})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirClosureExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail)
}

func (l *lowerer) wirClosureExpressionValueSourceWithRef(
	inst wir.Instruction,
	exprRef factflow.ExprRef,
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
		return l.wirCheckTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail)
	}
	leftOp, rightOp, ok := wirBinaryExpressionOperands(l.wir, inst)
	if !ok {
		return l.wirCheckTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail)
	}
	left, ok := l.wirBinaryExpressionOperandValueSource(inst, leftOp, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return l.wirCheckTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail)
	}
	right, ok := l.wirBinaryExpressionOperandValueSource(inst, rightOp, exprIndex, targetIndex, resultSources, seen)
	if !ok {
		return l.wirCheckTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail)
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
	if !l.addWIRLogicalExpressionOperationValue(exprRef, operation, left, right) {
		l.addWIRExpressionOperationValue(exprRef, operation, left, right)
	}
	l.addWIRExpressionCondition(exprRef, inst)
	l.addWIRLogicalExpressionCondition(exprRef, op, left, right)
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirCheckTempExpressionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	if inst.Check == 0 {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	l.addWIRExpressionCondition(exprRef, inst)
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[exprRef] = l.valueFromTypeWithWitness(typ.Boolean)
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) addWIRExpressionOperationValue(exprRef factflow.ExprRef, op factflow.ExpressionOperation, left, right factflow.ValueSource) {
	if exprRef == 0 || l == nil {
		return
	}
	leftValue, ok := l.staticValueSourceValue(left)
	if !ok {
		return
	}
	var rightValue product.Value
	if op.Kind() == factflow.ExpressionOperationBinary {
		rightValue, ok = l.staticValueSourceValue(right)
		if !ok {
			return
		}
	}
	value, ok := luasourcevalue.ExpressionOperationValue(l.registry, l.typeValues, op, leftValue, rightValue)
	if !ok {
		return
	}
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[exprRef] = value
}

func (l *lowerer) addWIRLogicalExpressionOperationValue(exprRef factflow.ExprRef, op factflow.ExpressionOperation, left, right factflow.ValueSource) bool {
	if exprRef == 0 || l == nil || op.Kind() != factflow.ExpressionOperationBinary {
		return false
	}
	var selected factflow.ExpressionConditionFacts
	switch op.Op() {
	case "and":
		condition, ok := l.sourceTruthinessCondition(left)
		if !ok {
			return false
		}
		selected = condition.FactsForValue(true)
	case "or":
		condition, ok := l.sourceTruthinessCondition(left)
		if !ok {
			return false
		}
		selected = condition.FactsForValue(false)
	default:
		return false
	}
	leftValue, ok := l.staticValueSourceValue(left)
	if !ok {
		return false
	}
	rightValue, ok := l.staticValueSourceValueWithRefinements(right, selected.Refinements())
	if !ok {
		return false
	}
	value, ok := luasourcevalue.ExpressionOperationValue(l.registry, l.typeValues, op, leftValue, rightValue)
	if !ok {
		return false
	}
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[exprRef] = value
	return true
}

func (l *lowerer) staticValueSourceValueWithRefinements(source factflow.ValueSource, refinements []factflow.PostconditionRefinement) (product.Value, bool) {
	return l.staticValueSourceValueWithRefinementsSeen(source, refinements, nil)
}

func (l *lowerer) staticValueSourceValueWithRefinementsSeen(source factflow.ValueSource, refinements []factflow.PostconditionRefinement, seen map[factflow.ExprRef]bool) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourcePath:
		for _, refinement := range refinements {
			if !pathKeyMatchesPath(source.PathKey, refinement.TargetPathRef()) {
				continue
			}
			value, ok := refinement.Value().Constraint()
			if ok {
				return value, true
			}
		}
	case factflow.ValueSourceExpression:
		if source.HasExpr {
			if seen[source.ExprRef] {
				break
			}
			op, ok := l.expressionOperations[source.ExprRef]
			if ok {
				nextSeen := seen
				if nextSeen == nil {
					nextSeen = make(map[factflow.ExprRef]bool, 1)
				}
				nextSeen[source.ExprRef] = true
				leftValue, leftOK := l.staticValueSourceValueWithRefinementsSeen(op.Left(), refinements, nextSeen)
				if !leftOK {
					break
				}
				var rightValue product.Value
				if op.Kind() == factflow.ExpressionOperationBinary {
					var rightOK bool
					rightValue, rightOK = l.staticValueSourceValueWithRefinementsSeen(op.Right(), refinements, nextSeen)
					if !rightOK {
						break
					}
				}
				value, ok := luasourcevalue.ExpressionOperationValue(l.registry, l.typeValues, op, leftValue, rightValue)
				if ok {
					return value, true
				}
			}
		}
	}
	return l.staticValueSourceValue(source)
}

func pathKeyMatchesPath(key path.PathKey, p path.Path) bool {
	sym, segments, ok := rootSymbolPathKey(key)
	if !ok || sym != p.Symbol || len(segments) != len(p.Segments) {
		return false
	}
	for i := range segments {
		if segments[i] != p.Segments[i] {
			return false
		}
	}
	return true
}

func pathFromRootSymbolKey(key path.PathKey) (path.Path, bool) {
	sym, segments, ok := rootSymbolPathKey(key)
	if !ok || sym == 0 {
		return path.Path{}, false
	}
	return path.Path{Symbol: sym, Segments: append([]segment.Segment(nil), segments...)}, true
}

func (l *lowerer) staticValueSourceValue(source factflow.ValueSource) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		value, ok := l.expressionValues[source.ExprRef]
		return value, ok
	case factflow.ValueSourceLiteral:
		return l.literalSourceValue(source)
	case factflow.ValueSourceNil:
		return l.valueFromTypeWithWitness(typ.Nil), true
	case factflow.ValueSourcePath:
		sym, segments, ok := rootSymbolPathKey(source.PathKey)
		if !ok || len(segments) != 0 {
			return product.Value{}, false
		}
		t, ok := l.staticSymbolType(sym)
		if !ok || t == nil {
			return product.Value{}, false
		}
		return l.valueFromTypeWithWitness(t), true
	default:
		return product.Value{}, false
	}
}

func (l *lowerer) staticSymbolType(sym symbol.ID) (typ.Type, bool) {
	if sym == 0 || l == nil {
		return nil, false
	}
	if t, ok := l.symbolTypes[sym]; ok && t != nil {
		return t, true
	}
	if l.bindings == nil {
		return nil, false
	}
	expr, ok := l.bindings.SymbolTypeAnnotation(sym)
	if !ok {
		return nil, false
	}
	return l.resolveType(expr)
}

func rootSymbolPathKey(key path.PathKey) (symbol.ID, []segment.Segment, bool) {
	if sym, _, suffix, ok := pathaddr.ParseResolverPath(key); ok {
		segments, segOK := segment.ParseFormattedSegments(suffix)
		return sym, segments, segOK
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(key); ok {
		return sym, segments, true
	}
	return 0, nil, false
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
	l.addWIRExpressionOperationValue(exprRef, operation, operand, factflow.ValueSource{})
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

func (l *lowerer) addWIRLogicalExpressionCondition(exprRef factflow.ExprRef, op string, left, right factflow.ValueSource) {
	if exprRef == 0 || l == nil {
		return
	}
	leftCondition, leftOK := l.sourceTruthinessCondition(left)
	rightCondition, rightOK := l.sourceTruthinessCondition(right)
	if !leftOK && !rightOK {
		return
	}
	var trueRefinements []factflow.PostconditionRefinement
	var falseRefinements []factflow.PostconditionRefinement
	var trueRelations []factflow.PostconditionPathRelation
	var falseRelations []factflow.PostconditionPathRelation
	switch op {
	case "and":
		if leftOK {
			facts := leftCondition.FactsForValue(true)
			trueRefinements = append(trueRefinements, facts.Refinements()...)
			trueRelations = append(trueRelations, facts.PathRelations()...)
		}
		if rightOK {
			facts := rightCondition.FactsForValue(true)
			trueRefinements = append(trueRefinements, facts.Refinements()...)
			trueRelations = append(trueRelations, facts.PathRelations()...)
		}
	case "or":
		if leftOK {
			facts := leftCondition.FactsForValue(false)
			falseRefinements = append(falseRefinements, facts.Refinements()...)
			falseRelations = append(falseRelations, facts.PathRelations()...)
		}
		if rightOK {
			facts := rightCondition.FactsForValue(false)
			falseRefinements = append(falseRefinements, facts.Refinements()...)
			falseRelations = append(falseRelations, facts.PathRelations()...)
		}
	default:
		return
	}
	condition := factflow.NewExpressionCondition(trueRefinements, falseRefinements, trueRelations, falseRelations)
	if condition.IsEmpty() {
		return
	}
	if l.expressionConditions == nil {
		l.expressionConditions = make(map[factflow.ExprRef]factflow.ExpressionCondition)
	}
	l.expressionConditions[exprRef] = condition
}

func (l *lowerer) sourceExpressionCondition(source factflow.ValueSource) (factflow.ExpressionCondition, bool) {
	if l == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ExpressionCondition{}, false
	}
	condition, ok := l.expressionConditions[source.ExprRef]
	return condition, ok
}

func (l *lowerer) sourceTruthinessCondition(source factflow.ValueSource) (factflow.ExpressionCondition, bool) {
	if condition, ok := l.sourceExpressionCondition(source); ok {
		return condition, true
	}
	if l == nil || source.Kind != factflow.ValueSourcePath {
		return factflow.ExpressionCondition{}, false
	}
	p, ok := pathFromRootSymbolKey(source.PathKey)
	if !ok || p.IsEmpty() {
		return factflow.ExpressionCondition{}, false
	}
	check := branchcond.Check{Kind: branchcond.CheckTruthy, Path: p}
	condition := factflow.NewExpressionCondition(
		postconditionRefinementsFromBranchEdge(l.branchEdgeRefinements(check, true), true),
		postconditionRefinementsFromBranchEdge(l.branchEdgeRefinements(check, false), false),
		nil,
		nil,
	)
	if condition.IsEmpty() {
		return factflow.ExpressionCondition{}, false
	}
	return condition, true
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
	if inst.Check != 0 {
		check := branchCheckFromWIR(l.wir.Check(inst.Check))
		return check, check.Kind != branchcond.CheckNone
	}
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
	if op.Kind == wir.OperandPath {
		if source, ok := l.valueSourceFromWIRRootPathOperand(op, exprIndex, targetIndex, true, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
			return source, true
		}
		if source, ok := l.pathExpressionSourceFromWIR("expr-op", inst.Point, op, sourceprovenance.NoSourceIndex, sourceprovenance.NoSourceIndex, true, false, false, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
			return source, true
		}
	}
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
		ops := wirConcatOperands(body, inst)
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

func wirConcatOperands(body *wir.Body, inst wir.Instruction) []wir.Operand {
	if inst.Op != wir.OpConcat {
		return nil
	}
	if inst.List.Len != 0 && body != nil {
		return body.Operands(inst.List)
	}
	if inst.A.Kind != wir.OperandNone && inst.B.Kind != wir.OperandNone {
		return []wir.Operand{inst.A, inst.B}
	}
	return nil
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
