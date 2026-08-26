package subtree

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Result is the immutable output of evaluating one correlated subtree for
// one population RowID.  Batches retain their sealed range authorities and
// exact tuple scopes; Result does not introduce a second relation store.
type Result struct {
	subtree         arrangement.CorrelatedSubtree
	root            arrangement.Node
	populationRef   model.DenominatorRef
	population      model.RowID
	populationScope witness.Scope
	populationFence binding.Fence
	batches         []tuple.Batch
	sealed          bool
}

func batchesAvailable(values []tuple.Batch) bool {
	if values == nil {
		return false
	}
	for _, value := range values {
		if !value.Available() {
			return false
		}
	}
	return true
}

// Available authenticates the complete evaluation result.  Tuple batches
// are immutable, so the result only checks their owner availability here.
func (result Result) Available() bool {
	return result.sealed && result.subtree.Available() && result.root.Available() && result.root == result.subtree.Root() && result.populationRef.Available() && result.population.Available() && result.population.Relation() == result.populationRef.Relation() && result.populationScope.ValidFor(result.populationFence) && result.populationFence.Available() && batchesAvailable(result.batches)
}

// Batches returns the relation extents in evaluator transport order.
func (result Result) Batches() []tuple.Batch {
	if !result.Available() {
		return nil
	}
	values := make([]tuple.Batch, len(result.batches))
	copy(values, result.batches)
	return values
}

// Subtree returns the exact mount witness redeemed by this result.
func (result Result) Subtree() arrangement.CorrelatedSubtree {
	if !result.Available() {
		return arrangement.CorrelatedSubtree{}
	}
	return result.subtree
}

// Root returns the exact physical root of the evaluated subtree.
func (result Result) Root() arrangement.Node {
	if !result.Available() {
		return arrangement.Node{}
	}
	return result.root
}

// Node returns the physical root digest of the evaluated subtree.
func (result Result) Node() identity.ContentID {
	if !result.Available() {
		return identity.ContentID{}
	}
	return result.root.Digest()
}

// Population returns the owner-issued RowID used for this evaluation.
func (result Result) Population() model.RowID {
	if !result.Available() {
		return model.RowID{}
	}
	return result.population
}

// PopulationRef returns the exact owner-issued denominator used to
// authenticate Population.  It is retained even for shared-only children,
// whose source extents intentionally carry no Q-local directory.
func (result Result) PopulationRef() model.DenominatorRef {
	if !result.Available() {
		return model.DenominatorRef{}
	}
	return result.populationRef
}

// PopulationScope returns the exact authenticated cofiber supplied for the
// owner population row.  It is part of the result contract so a caller cannot
// silently widen a driver row to every reader cofiber.
func (result Result) PopulationScope() witness.Scope {
	if !result.Available() {
		return witness.Scope{}
	}
	return result.populationScope
}

// Scope is the concise alias used by tuple-consuming callers.
func (result Result) Scope() witness.Scope { return result.PopulationScope() }
