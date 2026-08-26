package delta

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Result is the closed output of one Later-root schedule entry.  Relation
// roots carry changed tuple batches, Apply roots carry semantic applications,
// and Publish roots carry those child applications alongside ordered state
// settlements. These alternatives are kept explicit; a differential relation
// result is never hidden in a generic callback or a second row store.
// Relation batches remain the positive relation ABI. Apply applications stay
// beside the result until the publication door redeems them; that door is the
// sole positive contribution classifier. Signed unary Before/replacement
// remains a separate event path and is never projected into this result ABI.
type Result struct {
	dependency   model.DependencyID
	expression   model.ExpressionID
	node         identity.ContentID
	kind         algebra.Kind
	batches      []tuple.Batch
	applications []apply.Results
	settlements  []publish.Settlement
	inputDelta   database.Delta
	base         database.Version
	next         database.Version
	sealed       bool
}

func relationKind(kind algebra.Kind) bool {
	switch kind {
	case algebra.KindInput, algebra.KindSelect, algebra.KindProject, algebra.KindColumnProject, algebra.KindJoin, algebra.KindMerge, algebra.KindComplete, algebra.KindExpand:
		return true
	default:
		return false
	}
}

func composedKind(kind algebra.Kind) bool {
	return kind == algebra.KindMerge || kind == algebra.KindColumnProject
}

func valuesAvailable(batches []tuple.Batch) bool {
	if batches == nil {
		return false
	}
	for _, batch := range batches {
		if !batch.Available() {
			return false
		}
	}
	return true
}

func applicationsAvailable(values []apply.Results) bool {
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

func settlementsAvailable(values []publish.Settlement) bool {
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

func (result Result) valid() bool {
	if !result.sealed || !result.dependency.Available() || !result.expression.Available() || !result.node.Available() || !result.inputDelta.Available() || !result.base.Available() || !result.next.Available() || !result.base.Fence().Same(result.next.Fence()) {
		return false
	}
	if !valuesAvailable(result.batches) || !applicationsAvailable(result.applications) || !settlementsAvailable(result.settlements) {
		return false
	}
	switch result.kind {
	case algebra.KindApply:
		return len(result.batches) == 0 && len(result.settlements) == 0
	case algebra.KindPublish:
		return len(result.batches) == 0 && applicationsAvailable(result.applications)
	default:
		return relationKind(result.kind) && (len(result.applications) == 0 || composedKind(result.kind)) && len(result.settlements) == 0
	}
}

// Available authenticates the complete differential output.
func (result Result) Available() bool { return result.valid() }

func (result Result) Dependency() model.DependencyID {
	if !result.Available() {
		return model.DependencyID{}
	}
	return result.dependency
}

func (result Result) Expression() model.ExpressionID {
	if !result.Available() {
		return model.ExpressionID{}
	}
	return result.expression
}

func (result Result) Node() identity.ContentID {
	if !result.Available() {
		return identity.ContentID{}
	}
	return result.node
}

func (result Result) Kind() algebra.Kind {
	if !result.Available() {
		return algebra.KindInvalid
	}
	return result.kind
}

// Batches returns changed relation extents in canonical occurrence/path
// order. The returned slice is defensive; batches themselves are immutable.
func (result Result) Batches() []tuple.Batch {
	if !result.Available() || !relationKind(result.kind) {
		return nil
	}
	values := make([]tuple.Batch, len(result.batches))
	copy(values, result.batches)
	return values
}

// Applications returns Apply results, including applications carried through
// Merge or ColumnProject to a Publish boundary.
func (result Result) Applications() []apply.Results {
	if !result.Available() || (result.kind != algebra.KindApply && result.kind != algebra.KindPublish && !composedKind(result.kind)) {
		return nil
	}
	values := make([]apply.Results, len(result.applications))
	copy(values, result.applications)
	return values
}

// Settlements returns the ordered publication transitions. It is non-empty
// only when the root node is Publish.
func (result Result) Settlements() []publish.Settlement {
	if !result.Available() || result.kind != algebra.KindPublish {
		return nil
	}
	values := make([]publish.Settlement, len(result.settlements))
	copy(values, result.settlements)
	return values
}

// InputDelta returns the exact Later transition that selected this work.
func (result Result) InputDelta() database.Delta {
	if !result.Available() {
		return database.Delta{}
	}
	return result.inputDelta
}

// Base returns the predecessor root carried by the input Later delta.
func (result Result) Base() database.Version {
	if !result.Available() {
		return database.Version{}
	}
	return result.base
}

// Successor returns the exact root observed by this evaluation: the input
// delta's successor before any publication settlements. Next separately owns
// the final root after those settlements, so the two epochs cannot be
// conflated at the coordinator boundary.
func (result Result) Successor() database.Version {
	if !result.Available() {
		return database.Version{}
	}
	return result.inputDelta.Next()
}

// Next returns the final root after this result. Relation and Apply results
// retain the input successor; Publish results retain the last settlement's
// successor, including any newly committed output delta.
func (result Result) Next() database.Version {
	if !result.Available() {
		return database.Version{}
	}
	return result.next
}

// Changed reports whether at least one publication settlement committed a
// state ascent.
func (result Result) Changed() bool {
	if !result.Available() {
		return false
	}
	for _, settlement := range result.settlements {
		if settlement.Changed() {
			return true
		}
	}
	return false
}
