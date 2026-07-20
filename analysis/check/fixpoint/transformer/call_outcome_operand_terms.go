package transformer

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
