//go:build typprobe

package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"

// ZZProbeCellPair re-exports the solver-ladder join-distinctness counters
// (KB go-lua-solver-mathematics-post-unwind step 0) so a corpus lane outside
// the internal tree can read them. Compiled only with -tags typprobe.
func ZZProbeCellPair() (total, distinct, internedLeft uint64) {
	return semantic.ZZProbeCellPairCounters()
}

// ZZProbeCellPairDomains re-exports the same counters split by concrete
// Factor payload type. Compiled only with -tags typprobe.
func ZZProbeCellPairDomains() map[string][3]uint64 {
	return semantic.ZZProbeCellPairDomains()
}
