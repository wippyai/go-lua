package canonical

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/typ"
)

// Predicate inputs are implementation evidence for body refinement. They must
// not leak into the public function surface published to callers.
func (p *program) publicPredicateContracts(ref summary.FuncRef, contracts paramevidence.Contracts) paramevidence.Contracts {
	slots := p.predicateInputSlots(ref)
	if len(slots) == 0 || len(contracts) == 0 {
		return contracts
	}
	out := make(paramevidence.Contracts, len(contracts))
	for slot, demand := range contracts {
		out[slot] = demand
	}
	for _, slot := range slots {
		delete(out, slot)
	}
	return out
}

func (p *program) publicPredicateParamVector(ref summary.FuncRef, params []typ.Type) []typ.Type {
	slots := p.predicateInputSlots(ref)
	if len(slots) == 0 || len(params) == 0 {
		return params
	}
	out := append([]typ.Type(nil), params...)
	for _, slot := range slots {
		if slot >= 0 && slot < len(out) {
			out[slot] = nil
		}
	}
	return out
}

func (p *program) predicateInputSlots(ref summary.FuncRef) []int {
	if p == nil {
		return nil
	}
	preds := p.facts.PredicateFacts()
	if len(preds) == 0 {
		return nil
	}
	var out []int
	g := p.Graph(ref)
	fn := p.funcExpr(ref)
	for _, pred := range preds {
		predRef, ok := p.refBySymbol(pred.FuncSym)
		if !ok || predRef != ref {
			continue
		}
		_, slot, ok := paramevidence.ParamSlotForSourceParam(g, fn, pred.ParamIndex)
		if !ok || slot < 0 {
			continue
		}
		out = append(out, slot)
	}
	return out
}
