package kind

// OutcomeKind is the closed authored control-outcome vocabulary. Its numeric
// ordinals are part of the canonical Flow representation; keep them explicit
// and append-only rather than deriving them from declaration order.
type OutcomeKind uint8

const (
	OutcomeNormal OutcomeKind = 1
	OutcomeReturn OutcomeKind = 2
	OutcomeThrow  OutcomeKind = 3
	OutcomeBreak  OutcomeKind = 4
	OutcomeGoto   OutcomeKind = 5
	OutcomeYield  OutcomeKind = 6
	OutcomeCancel OutcomeKind = 7
)
