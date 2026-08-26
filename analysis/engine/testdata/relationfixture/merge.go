package testfixture

import "github.com/wippyai/go-lua/analysis/relation/mount/arrangement"

// ProposalMergeNode returns the sealed Merge whose first alternative is an
// Apply proposal and whose second is the prior destination row projected to
// the same writable payload. It is the generic carry specimen used by full
// and differential evaluator laws.
func (fixture Fixture) ProposalMergeNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.mergeApplyExpression)
}
