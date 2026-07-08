package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

func (l *lowerer) returnValueSourcesFromWIR(point cfg.Point) ([]factflow.ValueSource, bool) {
	ret, ok := l.wirReturnInstruction(point)
	if !ok {
		return nil, false
	}
	ops := l.wir.Operands(ret.List)
	out := make([]factflow.ValueSource, len(ops))
	for i, op := range ops {
		final := i == len(ops)-1
		source, ok := l.returnValueSourceFromWIROperand(point, op, i, i, final, ret.ListSpread && final, ret.ListSpread && final)
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
) (factflow.ValueSource, bool) {
	if source, ok := l.valueSourceFromWIRRootPathOperand(op, exprIndex, targetIndex, final, symbol.Local, symbol.Param); ok {
		return source, true
	}
	if source, ok := l.pathExpressionSourceFromWIR("return", point, op, exprIndex, targetIndex, final, expanded, openTail, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
		return source, true
	}
	return l.valueSourceFromWIROperand(op, exprIndex, targetIndex, final, expanded, openTail)
}

type wirResultSource struct {
	point       cfg.Point
	resultIndex int
	targetIndex int
	final       bool
	expanded    bool
	adjusted    bool
	openTail    bool
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
	return l.wirPathExpressionSourceWithShape(kind, point, p, witness, exprIndex, targetIndex, final, expanded, !expanded, openTail)
}

func (l *lowerer) wirPathExpressionSourceWithShape(
	kind string,
	point cfg.Point,
	p path.Path,
	witness typ.Type,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	adjusted bool,
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
	shape, ok := factflow.NewValueSourceShape(final, expanded, adjusted, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	source, ok := factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return source.WithSourcePoint(point), true
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
) (factflow.ValueSource, bool) {
	return l.valueSourceFromWIROperandSeen(op, exprIndex, targetIndex, final, expanded, openTail, nil)
}

func (l *lowerer) valueSourceFromWIROperandSeen(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
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
		if source, ok := resultValueSourceFromWIR(op, exprIndex, targetIndex, final, expanded, openTail, l.resultValueSourcesByTempFromWIR()); ok {
			return source, true
		}
		if source, ok := l.wirTempExpressionValueSource(op, exprIndex, targetIndex, final, expanded, openTail, seen); ok {
			return source, true
		}
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) addWIRCallResultExpressionValue(source factflow.ValueSource) {
	if l == nil || l.registry == nil || source.Kind != factflow.ValueSourceCall || !source.HasExpr || source.ExprRef == 0 || !source.HasCallPoint {
		return
	}
	site, ok := l.callSiteFromWIR(source.CallPoint)
	if !ok {
		return
	}
	t, ok := l.callSiteReturnTypeAt(source.CallPoint, site, source.ResultIndex)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return
	}
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[source.ExprRef] = l.valueFromTypeWithWitness(t)
}

type wirTempExprRefKey struct {
	temp uint32
}

type wirTempDefinitionExprRefKey struct {
	temp  uint32
	point cfg.Point
}

func (l *lowerer) wirTempExpressionValueSource(
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
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
	if source, ok := l.wirMultiDefTempExpressionValueSource(op.Ref, exprIndex, targetIndex, final, expanded, openTail, seen); ok {
		return source, true
	}
	inst, ok := l.wirTempDefs()[op.Ref]
	if !ok {
		return factflow.ValueSource{}, false
	}
	switch inst.Op {
	case wir.OpAssign:
		return l.valueSourceFromWIROperandSeen(inst.A, exprIndex, targetIndex, final, expanded, openTail, seen)
	case wir.OpDynamicIndexRead:
		return l.wirDynamicIndexReadTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, seen)
	case wir.OpConcat:
		return l.wirConcatTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, seen)
	case wir.OpBinOp, wir.OpLogical:
		return l.wirBinaryTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, seen)
	case wir.OpUnOp:
		return l.wirUnaryTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, seen)
	case wir.OpClaim:
		return l.wirClaimTempExpressionValueSource(op.Ref, inst, exprIndex, targetIndex, final, expanded, openTail, seen)
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
	left, ok := l.wirInstructionExpressionOperandValueSource(leftDef, leftDef.A, exprIndex, targetIndex, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	rightRef, ok := l.exprRef(wirTempDefinitionExprRefKey{temp: temp, point: rhsDef.Point})
	if !ok {
		return factflow.ValueSource{}, false
	}
	right, ok := l.wirDefinitionValueSourceWithRef(temp, rhsDef, rightRef, exprIndex, targetIndex, seen)
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

func (l *lowerer) wirDefinitionValueSourceWithRef(
	temp uint32,
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	switch inst.Op {
	case wir.OpAssign:
		return l.wirInstructionExpressionOperandValueSource(inst, inst.A, exprIndex, targetIndex, seen)
	case wir.OpDynamicIndexRead:
		return l.wirDynamicIndexReadExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, true, false, false, seen)
	case wir.OpConcat:
		return l.wirConcatTempExpressionValueSourceWithRef(temp, inst, exprRef, exprIndex, targetIndex, true, false, false, seen)
	case wir.OpBinOp, wir.OpLogical:
		return l.wirBinaryTempExpressionValueSourceWithRef(temp, inst, exprRef, exprIndex, targetIndex, true, false, false, seen)
	case wir.OpUnOp:
		return l.wirUnaryTempExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, true, false, false, seen)
	case wir.OpClaim:
		return l.wirClaimTempExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, true, false, false, seen)
	case wir.OpClosure:
		return l.wirClosureExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, true, false, false)
	case wir.OpMakeTable:
		return l.wirTableExpressionValueSourceWithShape(inst, exprIndex, targetIndex, true, false, false, false)
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
	return l.wirTempsHaveEquivalentDefs(left, right, nil)
}

type wirTempEquivalencePair struct {
	left  uint32
	right uint32
}

func (l *lowerer) wirTempsHaveEquivalentDefs(left, right uint32, seen map[wirTempEquivalencePair]bool) bool {
	if l == nil || left == right {
		return left == right
	}
	pair := wirTempEquivalencePair{left: left, right: right}
	if seen[pair] {
		return true
	}
	if seen == nil {
		seen = make(map[wirTempEquivalencePair]bool, 1)
	}
	seen[pair] = true
	defer delete(seen, pair)
	leftDefs := l.wirTempDefSets()[left]
	rightDefs := l.wirTempDefSets()[right]
	if len(leftDefs) == 0 || len(leftDefs) != len(rightDefs) {
		return false
	}
	used := make([]bool, len(rightDefs))
	for _, leftDef := range leftDefs {
		found := false
		for i, rightDef := range rightDefs {
			if used[i] || !l.wirInstructionsHaveEquivalentValue(leftDef, rightDef, seen) {
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

func (l *lowerer) wirInstructionsHaveEquivalentValue(left, right wir.Instruction, seen map[wirTempEquivalencePair]bool) bool {
	if left.Point != right.Point || left.Op != right.Op {
		return false
	}
	switch left.Op {
	case wir.OpAssign:
		return l.wirOperandsHaveEquivalentValue(left.A, right.A, seen)
	case wir.OpBinOp, wir.OpLogical:
		return left.Operator == right.Operator &&
			l.wirOperandsHaveEquivalentValue(left.A, right.A, seen) &&
			l.wirOperandsHaveEquivalentValue(left.B, right.B, seen)
	case wir.OpUnOp:
		return left.Operator == right.Operator &&
			l.wirOperandsHaveEquivalentValue(left.A, right.A, seen)
	default:
		return false
	}
}

func (l *lowerer) wirOperandsHaveEquivalentValue(left, right wir.Operand, seen map[wirTempEquivalencePair]bool) bool {
	if wirOperandEqual(left, right) {
		return true
	}
	if left.Kind != wir.OperandTemp || right.Kind != wir.OperandTemp {
		return false
	}
	return l.wirTempsHaveEquivalentDefs(left.Ref, right.Ref, seen)
}

func (l *lowerer) wirEdgeFromBranchToPoint(branch, target cfg.Point) (bool, bool) {
	if l.graph == nil {
		return false, false
	}
	var out bool
	var have bool
	for _, succ := range cfg.SuccessorsReadOnly(l.graph, branch) {
		if succ != target {
			continue
		}
		edge, ok := l.graph.EdgeCond(branch, succ)
		if !ok {
			continue
		}
		if have {
			return false, false
		}
		out = edge
		have = true
	}
	if have {
		return out, true
	}
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
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirConcatTempExpressionValueSourceWithRef(temp, inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
}

func (l *lowerer) wirConcatTempExpressionValueSourceWithRef(
	temp uint32,
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	ops := wirConcatOperands(l.wir, inst)
	if len(ops) == 2 {
		return l.wirBinaryTempExpressionValueSourceWithRef(temp, inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
	}
	if len(ops) < 2 {
		return factflow.ValueSource{}, false
	}
	sources := make([]factflow.ValueSource, len(ops))
	for i, operand := range ops {
		source, ok := l.wirInstructionExpressionOperandValueSource(inst, operand, exprIndex, targetIndex, seen)
		if !ok {
			return factflow.ValueSource{}, false
		}
		sources[i] = source
	}
	foldSource := sources[0]
	for i := 1; i < len(sources); i++ {
		ref := exprRef
		if i != len(sources)-1 {
			var ok bool
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
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) wirDynamicIndexReadTempExpressionValueSource(
	temp uint32,
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirDynamicIndexReadExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
}

type wirDynamicIndexReadExprRefKey struct {
	id wir.ExpressionID
}

type wirAssignmentExprRefKey struct {
	id wir.ExpressionID
}

func (l *lowerer) wirAssignmentExpressionValueSource(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if l == nil || inst.ExprID == 0 || inst.Op == wir.OpAssign {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirAssignmentExprRefKey{id: inst.ExprID})
	if !ok {
		return factflow.ValueSource{}, false
	}
	if source, ok := l.wirAssignmentExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen); ok {
		return source, true
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) wirAssignmentExpressionValueSourceWithRef(
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if exprRef == 0 {
		return factflow.ValueSource{}, false
	}
	switch inst.Op {
	case wir.OpDynamicIndexRead:
		return l.wirDynamicIndexReadExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
	case wir.OpClaim:
		return l.wirClaimTempExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
	}
	if inst.A.Kind == wir.OperandTemp {
		def, ok := l.wirTempDefs()[inst.A.Ref]
		if !ok {
			return factflow.ValueSource{}, false
		}
		return l.wirDefinitionValueSourceWithRef(inst.A.Ref, def, exprRef, exprIndex, targetIndex, seen)
	}
	if inst.A.Kind == wir.OperandConst {
		if value, ok := l.wirConstValue(inst.A); ok {
			if l.expressionValues == nil {
				l.expressionValues = make(map[factflow.ExprRef]product.Value)
			}
			l.expressionValues[exprRef] = value
		}
		shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
		if !ok {
			return factflow.ValueSource{}, false
		}
		return mustValueSource(factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)).WithSourcePoint(inst.Point), true
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) wirConstValue(op wir.Operand) (product.Value, bool) {
	if l == nil || l.wir == nil || op.Kind != wir.OperandConst {
		return product.Value{}, false
	}
	c := l.wir.Const(wir.ConstRef(op.Ref))
	if c.Kind == wir.ConstNil {
		return l.valueFromTypeWithWitness(typ.Nil), true
	}
	t, ok := wirLiteralType(c)
	if !ok {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(t), true
}

func (l *lowerer) wirDynamicIndexReadExpressionValueSource(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if inst.ExprID == 0 {
		return factflow.ValueSource{}, false
	}
	exprRef, ok := l.exprRef(wirDynamicIndexReadExprRefKey{id: inst.ExprID})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirDynamicIndexReadExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
}

func (l *lowerer) wirDynamicIndexReadExpressionValueSourceWithRef(
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	tableSource, ok := l.wirDynamicIndexReadOperandSource(inst, inst.A, exprIndex, targetIndex, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	keySource, ok := l.wirDynamicIndexReadOperandSource(inst, inst.B, exprIndex, targetIndex, seen)
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
	l.addWIRDynamicIndexReadExpressionValue(exprRef, inst, tableSource)
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) addWIRDynamicIndexReadExpressionValue(exprRef factflow.ExprRef, inst wir.Instruction, tableSource factflow.ValueSource) {
	if l == nil || exprRef == 0 {
		return
	}
	container, ok := l.dynamicIndexReadContainerType(inst, tableSource)
	if !ok {
		return
	}
	valueType, ok := l.dynamicIndexReadValueType(inst, container)
	if !ok {
		return
	}
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[exprRef] = l.valueFromTypeWithWitness(valueType)
}

func (l *lowerer) dynamicIndexReadContainerType(inst wir.Instruction, tableSource factflow.ValueSource) (typ.Type, bool) {
	if tableSource.HasExpr && l.expressionValues != nil {
		if value, ok := l.expressionValues[tableSource.ExprRef]; ok {
			if t, ok := typevalue.TypeOf(l.registry, value); ok {
				return t, true
			}
		}
	}
	if inst.A.Kind != wir.OperandPath {
		return nil, false
	}
	tablePath, ok := l.wirAssignmentPath(inst.A)
	if !ok {
		return nil, false
	}
	return l.aliasPathType(tablePath)
}

func (l *lowerer) dynamicIndexReadValueType(inst wir.Instruction, container typ.Type) (typ.Type, bool) {
	if seg, ok := l.dynamicIndexReadKeySegment(inst.B); ok {
		if projected, ok := luatypeprojection.ApplySegments(container, []segment.Segment{seg}); ok {
			return projected, true
		}
	}
	valueType, ok := dynamicIndexMapValueType(container, 0)
	if !ok {
		return nil, false
	}
	return typeexpr.Optional(valueType), true
}

func (l *lowerer) dynamicIndexReadKeySegment(op wir.Operand) (segment.Segment, bool) {
	if l == nil || l.wir == nil || op.Kind != wir.OperandConst {
		return segment.Segment{}, false
	}
	c := l.wir.Const(wir.ConstRef(op.Ref))
	switch c.Kind {
	case wir.ConstString:
		return segment.Segment{Kind: segment.SegmentIndexString, Name: c.Str}, true
	case wir.ConstNumber:
		if i, ok := numparse.ParseIntegerLiteral(c.Number); ok {
			return segment.Segment{Kind: segment.SegmentIndexInt, Index: int(i)}, true
		}
	}
	return segment.Segment{}, false
}

func (l *lowerer) wirDynamicIndexReadOperandSource(
	inst wir.Instruction,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
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
	source, ok := l.wirInstructionExpressionOperandValueSource(inst, op, exprIndex, targetIndex, seen)
	if !ok {
		return factflow.ValueSource{}, false
	}
	l.addWIRCallResultExprRefFromID(&source, op)
	l.addWIRCallResultExpressionValue(source)
	return source, true
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
	return l.wirTableExpressionValueSourceWithShape(inst, exprIndex, targetIndex, final, expanded, !expanded, openTail)
}

func (l *lowerer) wirTableExpressionValueSourceWithShape(
	inst wir.Instruction,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	adjusted bool,
	openTail bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.tableConstructorExprRefFromWIR(inst)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.tableExpressionValueSourceWithShape(exprRef, exprIndex, targetIndex, final, expanded, adjusted, openTail)
}

func (l *lowerer) tableExpressionValueSourceWithShape(
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	adjusted bool,
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
	shape, ok := factflow.NewValueSourceShape(final, expanded, adjusted, openTail)
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
	l.expressionFunctions[exprRef] = symbol.ID(proto.Symbol)
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
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirClaimTempExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
}

func (l *lowerer) wirClaimTempExpressionValueSourceWithRef(
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	inner, ok := l.wirClaimInnerValueSource(inst, exprIndex, targetIndex, final, expanded, openTail, seen)
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
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	if source, ok := l.valueSourceFromWIRRootPathOperand(inst.A, exprIndex, targetIndex, final, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
		return source, true
	}
	if source, ok := l.valueSourceFromWIROperandSeen(inst.A, exprIndex, targetIndex, final, expanded, openTail, seen); ok {
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
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirBinaryTempExpressionValueSourceWithRef(temp, inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
}

func (l *lowerer) wirBinaryTempExpressionValueSourceWithRef(
	temp uint32,
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
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
	left, ok := l.wirBinaryExpressionOperandValueSource(inst, leftOp, exprIndex, targetIndex, seen)
	if !ok {
		return l.wirCheckTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail)
	}
	right, ok := l.wirBinaryExpressionOperandValueSource(inst, rightOp, exprIndex, targetIndex, seen)
	if !ok {
		return l.wirCheckTempExpressionValueSource(temp, inst, exprIndex, targetIndex, final, expanded, openTail)
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
					if cached, cachedOK := l.expressionValues[source.ExprRef]; cachedOK && expressionValueHasTypeWitness(l.registry, cached) && !expressionValueHasTypeWitness(l.registry, value) {
						return cached, true
					}
					return value, true
				}
			}
		}
	}
	return l.staticValueSourceValue(source)
}

func expressionValueHasTypeWitness(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() || witness.IsBottom() {
		return false
	}
	_, ok := witness.Type()
	return ok
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
		p, ok := pathFromRootSymbolKey(source.PathKey)
		if !ok {
			return product.Value{}, false
		}
		t, ok := l.aliasPathType(p)
		if !ok || t == nil {
			return product.Value{}, false
		}
		return l.valueFromTypeWithWitness(t), true
	default:
		return product.Value{}, false
	}
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
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	exprRef, ok := l.exprRef(wirTempExprRefKey{temp: temp})
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.wirUnaryTempExpressionValueSourceWithRef(inst, exprRef, exprIndex, targetIndex, final, expanded, openTail, seen)
}

func (l *lowerer) wirUnaryTempExpressionValueSourceWithRef(
	inst wir.Instruction,
	exprRef factflow.ExprRef,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	openTail bool,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	op, ok := wirExpressionOperator(inst)
	if !ok {
		return factflow.ValueSource{}, false
	}
	operand, ok := l.wirInstructionExpressionOperandValueSource(inst, inst.A, exprIndex, targetIndex, seen)
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
	l.addWIRUnaryExpressionCondition(exprRef, op, operand)
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, exprIndex, targetIndex, 0, shape)
}

func (l *lowerer) addWIRUnaryExpressionCondition(exprRef factflow.ExprRef, op string, operand factflow.ValueSource) {
	if exprRef == 0 || op != "not" || l == nil {
		return
	}
	condition, ok := l.sourceTruthinessCondition(operand)
	if !ok || condition.IsEmpty() {
		return
	}
	trueFacts := condition.FactsForValue(false)
	falseFacts := condition.FactsForValue(true)
	inverted := factflow.NewExpressionCondition(
		trueFacts.Refinements(),
		falseFacts.Refinements(),
		trueFacts.PathRelations(),
		falseFacts.PathRelations(),
	)
	if inverted.IsEmpty() {
		return
	}
	if l.expressionConditions == nil {
		l.expressionConditions = make(map[factflow.ExprRef]factflow.ExpressionCondition)
	}
	l.expressionConditions[exprRef] = inverted
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
		postconditionPathRelationsFromCheck(check, true),
		postconditionPathRelationsFromCheck(check, false),
	)
	if condition.IsEmpty() {
		return
	}
	if l.expressionConditions == nil {
		l.expressionConditions = make(map[factflow.ExprRef]factflow.ExpressionCondition)
	}
	l.expressionConditions[exprRef] = condition
}

func postconditionPathRelationsFromCheck(check branchcond.Check, trueValue bool) []factflow.PostconditionPathRelation {
	var out []factflow.PostconditionPathRelation
	for _, relation := range checkPathRelations(check, true, true) {
		if relation.Kind() != factflow.BranchPathRelationEqual || !relation.ActiveOnEdge(trueValue) {
			continue
		}
		out = append(out, factflow.NewPostconditionPathEquality(relation.LeftPath(), relation.RightPath()))
	}
	return out
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

func (l *lowerer) wirExpressionOperandValueSource(op wir.Operand, seen map[uint32]bool) (factflow.ValueSource, bool) {
	return l.valueSourceFromWIROperandSeen(op,
		sourceprovenance.NoSourceIndex,
		sourceprovenance.NoSourceIndex,
		true,
		false,
		false,
		seen,
	)
}

func (l *lowerer) wirInstructionExpressionOperandValueSource(
	inst wir.Instruction,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
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
	return l.wirExpressionOperandValueSource(op, seen)
}

func (l *lowerer) wirBinaryExpressionOperandValueSource(
	inst wir.Instruction,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	seen map[uint32]bool,
) (factflow.ValueSource, bool) {
	return l.wirInstructionExpressionOperandValueSource(inst, op, exprIndex, targetIndex, seen)
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
				out[result.Ref] = wirResultSource{
					point:       inst.Point,
					resultIndex: resultIndex,
					targetIndex: wirCallResultTargetIndex(l.wir, inst.Point, resultIndex),
					final:       inst.CallFinal,
					expanded:    inst.CallExpanded,
					adjusted:    inst.CallAdjusted,
					openTail:    inst.CallOpenTail,
					exprID:      inst.ExprID,
				}
			}
		case wir.OpSelect:
			if inst.Dst.Kind == wir.OperandTemp {
				out[inst.Dst.Ref] = wirResultSource{
					point:       inst.Point,
					resultIndex: 0,
					targetIndex: wirCallResultTargetIndex(l.wir, inst.Point, 0),
					final:       true,
					expanded:    false,
					adjusted:    false,
					openTail:    false,
					exprID:      inst.ExprID,
				}
			}
		}
	}
	l.wirResultSources = out
	return out
}

func wirCallResultTargetIndex(body *wir.Body, point cfg.Point, resultIndex int) int {
	if body == nil {
		return factflow.NoValueSourceIndex
	}
	for _, target := range body.CallResultTargets(point) {
		if target.ResultIndex == resultIndex {
			return target.Index
		}
	}
	return factflow.NoValueSourceIndex
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
	if exprIndex == factflow.NoValueSourceIndex {
		final = result.final
		expanded = result.expanded
		openTail = result.openTail
		if result.targetIndex != factflow.NoValueSourceIndex {
			targetIndex = result.targetIndex
		}
	}
	adjusted := !expanded
	if exprIndex == factflow.NoValueSourceIndex {
		adjusted = result.adjusted
	}
	shape, ok := factflow.NewValueSourceShape(final, expanded, adjusted, openTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if exprIndex == factflow.NoValueSourceIndex && result.targetIndex != factflow.NoValueSourceIndex {
		exprIndex = result.targetIndex
	}
	if targetIndex == factflow.NoValueSourceIndex && result.targetIndex != factflow.NoValueSourceIndex {
		targetIndex = result.targetIndex
	}
	return mustValueSource(factflow.NewCallValueSource(0, exprIndex, targetIndex, result.resultIndex, result.point, shape)), true
}

func mustValueSource(source factflow.ValueSource, ok bool) factflow.ValueSource {
	if !ok {
		panic("transferfacts: invalid value source")
	}
	return source
}
