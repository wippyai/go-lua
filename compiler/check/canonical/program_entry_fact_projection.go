package canonical

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

// EntryFacts owns the aggregate summary-level path proofs for one callee entry.
// Exact EntryFactsKey contexts remain exact; this fold supplies only facts that
// every known caller publishes for the callee, so default contexts never gain
// caller-specific precision by accident.
func (p *program) EntryFacts(ref summary.FuncRef, deps summary.EntryPublicationDependencies) flow.BoundaryFacts {
	if p == nil || deps == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return summary.AggregateEntryFacts(func(yield func(flow.BoundaryFacts)) {
		for _, dep := range p.callerRefs(ref) {
			yield(deps.CallEntryPublication(dep, ref).Facts)
		}
	})
}
