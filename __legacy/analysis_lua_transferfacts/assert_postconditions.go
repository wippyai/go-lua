package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func (l *lowerer) assertPostconditionRefinementFromWIR(point cfg.Point) (factflow.PostconditionRefinement, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.CallContext != wir.CallContextStatement || inst.Check == 0 || !l.isDirectGlobalAssertWIRCall(inst) {
		return factflow.PostconditionRefinement{}, false
	}
	check := branchCheckFromWIR(l.wir.Check(inst.Check))
	if !l.branchCheckAuthorized(check) {
		return factflow.PostconditionRefinement{}, false
	}
	branchRefinement, ok := l.branchValueRefinementForCheck(check)
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	value, ok := branchRefinement.TrueValue()
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(branchRefinement.TargetPath(), value), true
}

func (l *lowerer) isDirectGlobalAssertWIRCall(inst wir.Instruction) bool {
	if l == nil || l.wir == nil || inst.Call.Method != 0 || inst.Call.Callee.Kind != wir.OperandPath {
		return false
	}
	calleePath := l.wir.Path(wir.PathRef(inst.Call.Callee.Ref))
	return l.wir.SymbolResolvesToGlobal(calleePath.Symbol, "assert")
}
