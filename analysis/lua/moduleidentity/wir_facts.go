package moduleidentity

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NewFromWIR builds a require/module identity projection directly from WIR.
// It is intentionally narrower than transferfacts: it only projects structural
// source identity needed by this package, not runtime value semantics.
func NewFromWIR(bindings *bind.Result, graph cfg.Graph, body *wir.Body, fn *ast.FunctionExpr) Projection {
	return NewFromFacts(bindings, graph, newWIRFlowFacts(bindings, body), fn)
}

type wirFlowFacts struct {
	body          *wir.Body
	tempDefs      map[uint32]wir.Instruction
	resultSources map[uint32]wirResultSource
}

type wirResultSource struct {
	point       cfg.Point
	resultIndex int
	exprID      wir.ExpressionID
}

func newWIRFlowFacts(bindings *bind.Result, body *wir.Body) wirFlowFacts {
	facts := wirFlowFacts{body: body}
	facts.tempDefs = facts.collectTempDefs()
	facts.resultSources = facts.collectResultSources()
	return facts
}

func (f wirFlowFacts) LocalAssignment(point cfg.Point) (Assignment, bool) {
	inst, ok := f.rootAssignment(point, wir.AssignLocalDeclaration)
	if !ok {
		return Assignment{}, false
	}
	return f.assignmentFromRootInstruction(point, inst)
}

func (f wirFlowFacts) OrdinaryAssignment(point cfg.Point) (Assignment, bool) {
	inst, ok := f.rootAssignment(point, wir.AssignOrdinaryRootWrite)
	if !ok {
		return Assignment{}, false
	}
	return f.assignmentFromRootInstruction(point, inst)
}

func (f wirFlowFacts) PathAssignment(point cfg.Point) (Assignment, bool) {
	if f.body == nil {
		return Assignment{}, false
	}
	for _, inst := range f.body.PointInstructions(point) {
		if inst.Op != wir.OpStaticMemberWrite {
			continue
		}
		target, ok := f.operandPath(inst.Dst)
		if !ok || len(target.Segments) == 0 {
			continue
		}
		source, ok := f.sourceFromAssignmentInstruction(point, inst)
		if !ok {
			source = Source{}
		}
		return Assignment{
			Target:       target.Clone(),
			TargetSymbol: target.Symbol,
			Source:       source,
		}, true
	}
	return Assignment{}, false
}

func (f wirFlowFacts) PathDescendantInvalidation(point cfg.Point) (pathdom.Path, bool) {
	if f.body == nil {
		return pathdom.Path{}, false
	}
	for _, inst := range f.body.PointInstructions(point) {
		if inst.Op != wir.OpDynamicIndexWrite {
			continue
		}
		target, ok := f.operandPath(inst.Dst)
		if ok {
			return target.Clone(), true
		}
	}
	return pathdom.Path{}, false
}

func (f wirFlowFacts) CallSite(point cfg.Point) (CallSite, bool) {
	inst, ok := f.callInstruction(point)
	if !ok {
		return CallSite{}, false
	}
	args := f.body.Operands(inst.List)
	outArgs := make([]Source, 0, len(args))
	for _, arg := range args {
		source, ok := f.sourceFromOperand(point, arg)
		if !ok {
			source = Source{}
		}
		outArgs = append(outArgs, source)
	}
	site := CallSite{
		Args:         outArgs,
		TypeArgCount: len(f.body.TypeRefs(inst.CallTypeArgs)),
	}
	if inst.Call.Method != 0 {
		method := f.body.Const(inst.Call.Method)
		if method.Kind == wir.ConstString {
			site.MethodName = method.Str
		}
		if receiver, ok := f.operandPath(inst.Call.Receiver); ok && site.MethodName != "" {
			site.Callee = receiver.Field(site.MethodName)
		}
		return site, true
	}
	if callee, ok := f.operandPath(inst.Call.Callee); ok {
		site.Callee = callee
	}
	return site, true
}

func (f wirFlowFacts) ObjectLiteral(expr SourceRef) ([]ObjectEntry, bool) {
	if f.body == nil {
		return nil, false
	}
	inst, ok := f.body.TableConstructorByExpressionID(wir.ExpressionID(expr))
	if !ok {
		return nil, false
	}
	entries := f.body.TableEntries(inst.TableEntries)
	if len(entries) == 0 {
		return nil, false
	}
	out := make([]ObjectEntry, 0, len(entries))
	for _, entry := range entries {
		source, ok := f.sourceFromOperand(inst.Point, entry.Value)
		if !ok {
			source = Source{}
		}
		out = append(out, ObjectEntry{
			Suffix: entry.Suffix.Clone(),
			Source: source,
		})
	}
	return out, true
}

func (f wirFlowFacts) ExpressionPath(expr SourceRef) (pathdom.Path, bool) {
	if expr == 0 || f.body == nil {
		return pathdom.Path{}, false
	}
	for i := 0; i < f.body.Len(); i++ {
		inst := f.body.Instr(i)
		if inst.ExprID != wir.ExpressionID(expr) {
			continue
		}
		if source, ok := f.sourceFromAssignmentInstruction(inst.Point, inst); ok {
			if source.Kind == SourcePath {
				return pathFromSourceKey(source.PathKey)
			}
		}
	}
	return pathdom.Path{}, false
}

func (f wirFlowFacts) rootAssignment(point cfg.Point, kind wir.AssignKind) (wir.Instruction, bool) {
	if f.body == nil {
		return wir.Instruction{}, false
	}
	for _, inst := range f.body.PointInstructions(point) {
		if inst.Assign != kind {
			continue
		}
		target, ok := f.operandPath(inst.Dst)
		if ok && len(target.Segments) == 0 {
			return inst, true
		}
	}
	return wir.Instruction{}, false
}

func (f wirFlowFacts) assignmentFromRootInstruction(point cfg.Point, inst wir.Instruction) (Assignment, bool) {
	target, ok := f.operandPath(inst.Dst)
	if !ok || target.Symbol == 0 || len(target.Segments) != 0 {
		return Assignment{}, false
	}
	source, ok := f.sourceFromAssignmentInstruction(point, inst)
	if !ok {
		source = Source{}
	}
	return Assignment{
		Target:       target.Clone(),
		TargetSymbol: target.Symbol,
		Source:       source,
	}, true
}

func (f wirFlowFacts) sourceFromAssignmentInstruction(point cfg.Point, inst wir.Instruction) (Source, bool) {
	if inst.Op == wir.OpMakeTable && inst.ExprID != 0 {
		return Source{Kind: SourceExpression, Expr: SourceRef(inst.ExprID), HasExpr: true}, true
	}
	if op, ok := inst.AssignmentSourceOperand(); ok {
		return f.sourceFromOperand(point, op)
	}
	switch inst.Op {
	case wir.OpSelect:
		return Source{Kind: SourceCall, CallPoint: point, ResultIndex: 0}, true
	default:
		return Source{}, false
	}
}

func (f wirFlowFacts) sourceFromOperand(point cfg.Point, op wir.Operand) (Source, bool) {
	if f.body == nil {
		return Source{}, false
	}
	return f.sourceFromOperandSeen(point, op, nil)
}

func (f wirFlowFacts) sourceFromOperandSeen(point cfg.Point, op wir.Operand, seen map[uint32]bool) (Source, bool) {
	if f.body == nil {
		return Source{}, false
	}
	switch op.Kind {
	case wir.OperandPath:
		p, ok := f.operandPath(op)
		if !ok {
			return Source{}, false
		}
		return Source{Kind: SourcePath, PathKey: p.Key()}, true
	case wir.OperandConst:
		c := f.body.Const(wir.ConstRef(op.Ref))
		if c.Kind == wir.ConstString {
			return Source{Kind: SourceStringLiteral, String: c.Str}, true
		}
		return Source{}, false
	case wir.OperandTemp:
		if result, ok := f.resultSources[op.Ref]; ok {
			return Source{
				Kind:        SourceCall,
				Expr:        SourceRef(result.exprID),
				HasExpr:     result.exprID != 0,
				CallPoint:   result.point,
				ResultIndex: result.resultIndex,
			}, true
		}
		if seen == nil {
			seen = make(map[uint32]bool)
		}
		if seen[op.Ref] {
			return Source{}, false
		}
		seen[op.Ref] = true
		defer delete(seen, op.Ref)
		def, ok := f.tempDefs[op.Ref]
		if !ok {
			return Source{}, false
		}
		if def.Op == wir.OpMakeTable && def.ExprID != 0 {
			return Source{Kind: SourceExpression, Expr: SourceRef(def.ExprID), HasExpr: true}, true
		}
		if src, ok := def.AssignmentSourceOperand(); ok {
			return f.sourceFromOperandSeen(def.Point, src, seen)
		}
	}
	return Source{}, false
}

func (f wirFlowFacts) callInstruction(point cfg.Point) (wir.Instruction, bool) {
	if f.body == nil {
		return wir.Instruction{}, false
	}
	for _, inst := range f.body.PointInstructions(point) {
		if inst.Op == wir.OpCall {
			return inst, true
		}
	}
	return wir.Instruction{}, false
}

func (f wirFlowFacts) operandPath(op wir.Operand) (pathdom.Path, bool) {
	if f.body == nil || op.Kind != wir.OperandPath {
		return pathdom.Path{}, false
	}
	p := f.body.Path(wir.PathRef(op.Ref))
	return p.Clone(), !p.IsEmpty() && p.Symbol != 0
}

func (f wirFlowFacts) collectTempDefs() map[uint32]wir.Instruction {
	out := make(map[uint32]wir.Instruction)
	if f.body == nil {
		return out
	}
	for i := 0; i < f.body.Len(); i++ {
		inst := f.body.Instr(i)
		if inst.Dst.Kind == wir.OperandTemp {
			out[inst.Dst.Ref] = inst
		}
	}
	return out
}

func (f wirFlowFacts) collectResultSources() map[uint32]wirResultSource {
	out := make(map[uint32]wirResultSource)
	if f.body == nil {
		return out
	}
	for i := 0; i < f.body.Len(); i++ {
		inst := f.body.Instr(i)
		switch inst.Op {
		case wir.OpCall:
			results := f.body.Operands(inst.Results)
			for resultIndex, result := range results {
				if result.Kind != wir.OperandTemp {
					continue
				}
				out[result.Ref] = wirResultSource{
					point:       inst.Point,
					resultIndex: resultIndex,
					exprID:      inst.ExprID,
				}
			}
		case wir.OpSelect:
			if inst.Dst.Kind == wir.OperandTemp {
				out[inst.Dst.Ref] = wirResultSource{
					point:       inst.Point,
					resultIndex: 0,
					exprID:      inst.ExprID,
				}
			}
		}
	}
	return out
}
