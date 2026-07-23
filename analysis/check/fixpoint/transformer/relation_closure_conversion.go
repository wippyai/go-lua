package transformer

import (
	"bytes"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// PublishedRelationClosure is the production publication envelope for one
// body after the producer has materialized its result channels. Unlike a
// summary projection, it carries all four comparison channels required by the
// equation VM: values, outcomes, diagnostic candidates, and allocation rekeys.
// Fact bytes remain owned by the production publisher and are copied here.
type PublishedRelationClosure struct {
	Values               []equation.Fact
	Outcomes             []equation.Fact
	DiagnosticCandidates []equation.Fact
	AllocationRekeys     []equation.AllocationRekey
}

// ToOutputClosure is the lossless production-to-VM conversion. It does no
// filtering, reclassification, or summary projection; OutputClosure.Equal is
// responsible only for canonical comparison after this boundary.
func (p PublishedRelationClosure) ToOutputClosure() equation.OutputClosure {
	return equation.OutputClosure{
		Values:           cloneClosureFacts(p.Values),
		Outcomes:         cloneClosureFacts(p.Outcomes),
		Diagnostics:      cloneClosureFacts(p.DiagnosticCandidates),
		AllocationRekeys: append([]equation.AllocationRekey(nil), p.AllocationRekeys...),
	}
}

// PublishedRelationClosureFromOutputClosure reverses ToOutputClosure exactly.
// This is intentionally a structural conversion rather than a production
// re-evaluation, so round trips cannot hide a dropped publication channel.
func PublishedRelationClosureFromOutputClosure(closure equation.OutputClosure) PublishedRelationClosure {
	return PublishedRelationClosure{
		Values:               cloneClosureFacts(closure.Values),
		Outcomes:             cloneClosureFacts(closure.Outcomes),
		DiagnosticCandidates: cloneClosureFacts(closure.Diagnostics),
		AllocationRekeys:     append([]equation.AllocationRekey(nil), closure.AllocationRekeys...),
	}
}

// Equal compares the raw, ordered production envelope. It is intended for the
// conversion round-trip boundary; VM comparisons should use OutputClosure.Equal.
func (p PublishedRelationClosure) Equal(other PublishedRelationClosure) bool {
	return equalClosureFacts(p.Values, other.Values) &&
		equalClosureFacts(p.Outcomes, other.Outcomes) &&
		equalClosureFacts(p.DiagnosticCandidates, other.DiagnosticCandidates) &&
		equalAllocationRekeys(p.AllocationRekeys, other.AllocationRekeys)
}

// RequireComplete checks that the production publisher named every required
// channel. A nil channel is distinct from an explicitly empty publication,
// preventing an absent observation from being compared as an empty result.
func (p PublishedRelationClosure) RequireComplete() error {
	if p.Values == nil || p.Outcomes == nil || p.DiagnosticCandidates == nil || p.AllocationRekeys == nil {
		return fmt.Errorf("transformer: production relation closure omitted a publication channel")
	}
	return nil
}

func cloneClosureFacts(in []equation.Fact) []equation.Fact {
	out := make([]equation.Fact, len(in))
	for index, fact := range in {
		out[index] = equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)}
	}
	return out
}

func equalClosureFacts(left, right []equation.Fact) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Key != right[index].Key || !bytes.Equal(left[index].Value, right[index].Value) {
			return false
		}
	}
	return true
}

func equalAllocationRekeys(left, right []equation.AllocationRekey) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
