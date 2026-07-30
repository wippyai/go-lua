package factflow

// BranchCondition is the source-owned description of one CFG branch test.
// TruthyOnTrueEdge records whether the tested value is truthy on the graph's
// true edge. Keeping polarity beside the source prevents consumers from
// guessing it from syntax or from treating normalized `not x` checks as `x`.
type BranchCondition struct {
	source           ValueSource
	truthyOnTrueEdge bool
}

// NewBranchCondition creates a branch condition over a valid scalar source.
func NewBranchCondition(source ValueSource, truthyOnTrueEdge bool) (BranchCondition, bool) {
	if !source.Valid() || source.Expanded || source.OpenTail {
		return BranchCondition{}, false
	}
	return BranchCondition{source: source, truthyOnTrueEdge: truthyOnTrueEdge}, true
}

func (c BranchCondition) Source() ValueSource    { return c.source }
func (c BranchCondition) TruthyOnTrueEdge() bool { return c.truthyOnTrueEdge }
func (c BranchCondition) TruthyOnEdge(edge bool) bool {
	return edge == c.truthyOnTrueEdge
}
