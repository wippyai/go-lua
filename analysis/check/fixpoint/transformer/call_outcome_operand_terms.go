package transformer

import "github.com/wippyai/go-lua/analysis/engine/callpayload"

// callOutcomeOperandTerms is the ordered provider-facing root tuple of the
// same canonical ValueTerm DAG whose closure is stored in externalAccess. It
// is metadata, not a second expression language.
type callOutcomeOperandTerms struct {
	callee      ValueTerm
	hasCallee   bool
	receiver    ValueTerm
	hasReceiver bool
	arguments   []ValueTerm
}

func (o callOutcomeOperandTerms) clone() callOutcomeOperandTerms {
	o.arguments = append([]ValueTerm(nil), o.arguments...)
	return o
}

func (o callOutcomeOperandTerms) each(fn func(ValueTerm) bool) bool {
	if o.hasCallee && !fn(o.callee) || o.hasReceiver && !fn(o.receiver) {
		return false
	}
	for _, term := range o.arguments {
		if !fn(term) {
			return false
		}
	}
	return true
}

// selectObservation is the physical counterpart of a provider's sealed input
// certificate.  Omitted roles stay absent in CallOutcomeInput, so an evaluator
// cannot accidentally acquire a source term through the site-wide tuple.
func (o callOutcomeOperandTerms) selectObservation(observation callpayload.CallOutcomeInputObservation) callOutcomeOperandTerms {
	out := callOutcomeOperandTerms{}
	if o.hasCallee && observation.ObservesCallee() {
		out.callee, out.hasCallee = o.callee, true
	}
	if o.hasReceiver && observation.ObservesReceiver() {
		out.receiver, out.hasReceiver = o.receiver, true
	}
	for index, term := range o.arguments {
		if observation.ObservesArgument(index) {
			out.arguments = append(out.arguments, term)
		}
	}
	return out
}

func (o callOutcomeOperandTerms) valid(arena *Arena, shape Shape, owned map[callFrameTerm]struct{}) bool {
	if o.hasCallee != (o.callee != 0) || o.hasReceiver != (o.receiver != 0) {
		return false
	}
	return o.each(func(term ValueTerm) bool {
		return term != 0 && arena.validValue(term, shape, make(map[ValueTerm]bool)) && arena.valueFramesOwned(term, owned, make(map[ValueTerm]bool))
	})
}

func (o callOutcomeOperandTerms) validBits(arena *Arena, shape Shape, owned []uint64) bool {
	if o.hasCallee != (o.callee != 0) || o.hasReceiver != (o.receiver != 0) {
		return false
	}
	return o.each(func(term ValueTerm) bool {
		return term != 0 && arena.validValue(term, shape, make(map[ValueTerm]bool)) && valueFramesOwnedBits(arena, term, owned)
	})
}
