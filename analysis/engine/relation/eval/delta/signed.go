package delta

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	eventtuple "github.com/wippyai/go-lua/analysis/engine/relation/tuple/event"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// signedSide is the child-package representation of one side of a tuple
// transition.  present is row-level presence, not cell Presence: a present
// side may still contain an explicit ProvenAbsent cell, while an absent side
// is sparse state and has no synthetic tuple.  batches is deliberately
// allowed to be an authenticated empty partition so no-selection is kept
// distinct from refusal.
type signedSide struct {
	present bool
	batches []tuple.Batch
}

func (side signedSide) availableNoMount() bool {
	if !side.present {
		return side.batches == nil
	}
	if side.batches == nil {
		return false
	}
	for _, batch := range side.batches {
		if !batch.Available() {
			return false
		}
	}
	return true
}

func (side signedSide) validFor(mounted witness.Mounted) bool {
	if !side.availableNoMount() {
		return false
	}
	if !side.present {
		return true
	}
	for _, batch := range side.batches {
		if !batch.ValidFor(mounted) {
			return false
		}
	}
	return true
}

// signedTransition preserves one event's exact range authority and semantic
// components while unary operators independently transform each present
// side.  The two sides never share a tuple, Reader, or output batch.
type signedTransition struct {
	base      database.Version
	next      database.Version
	authority arrangement.RangeBinding
	before    signedSide
	after     signedSide
	semantic  bool
	lineage   bool
}

func (transition signedTransition) availableNoMount() bool {
	return transition.base.Available() && transition.next.Available() &&
		transition.next.SuccessorOf(transition.base) && transition.authority.Available() &&
		(transition.before.present || transition.after.present) &&
		(transition.before.availableNoMount()) && transition.after.availableNoMount() &&
		(transition.semantic || transition.lineage)
}

func (transition signedTransition) validFor(mounted witness.Mounted) bool {
	return transition.availableNoMount() && mounted.Available() &&
		transition.base.MountedDigest() == mounted.Digest() && transition.next.MountedDigest() == mounted.Digest() &&
		transition.base.Fence().Same(mounted.RuntimeFence()) && transition.next.Fence().Same(mounted.RuntimeFence()) &&
		transition.authority.ValidFor(mounted.Fence()) && transition.before.validFor(mounted) && transition.after.validFor(mounted)
}

// signedValue is intentionally private to eval/delta. Result remains the
// existing positive root ABI; this value exists while a unary vertical and
// its one-child Apply ascend so a deletion/replacement cannot be collapsed
// into an After-only tuple or application vector before the signed door sees
// it.
type signedValue struct {
	node          identity.ContentID
	kind          algebra.Kind
	transitions   []signedTransition
	differentials []applydifferential.Results
	semantic      bool
	lineage       bool
}

func (value signedValue) availableNoMount() bool {
	if !value.node.Available() || !signedKind(value.kind) || value.transitions == nil || value.differentials == nil || (!value.semantic && !value.lineage) {
		return false
	}
	for _, transition := range value.transitions {
		if !transition.availableNoMount() {
			return false
		}
	}
	for _, transport := range value.differentials {
		if !transport.Available() {
			return false
		}
	}
	return true
}

func signedKind(kind algebra.Kind) bool {
	return relationKind(kind) || kind == algebra.KindApply || kind == algebra.KindPublish
}

func (value signedValue) validFor(mounted witness.Mounted) bool {
	if !value.availableNoMount() || !mounted.Available() {
		return false
	}
	for _, transition := range value.transitions {
		if !transition.validFor(mounted) {
			return false
		}
	}
	return true
}

// signedInput lowers the canonical event batch to tuple batches while
// retaining both optional sides.  event.Bind has already redeemed the exact
// Base and Next Readers; this function only installs the producer range proof
// needed by the tuple operator and never reopens a row or scans a root.
func signedInput(node identity.ContentID, batch eventtuple.Batch, mounted witness.Mounted) (signedValue, bool) {
	if !node.Available() || !batch.Available() || !mounted.Available() || !batch.ValidFor(mounted) {
		return signedValue{}, false
	}
	transitions := make([]signedTransition, batch.Len())
	semantic, lineage := batch.SemanticChanged(), batch.LineageChanged()
	for index := 0; index < batch.Len(); index++ {
		event, ok := batch.At(index)
		if !ok || !event.Available() || !event.ValidFor(mounted) || !event.Base().Available() || !event.Next().Available() || !event.Next().SuccessorOf(event.Base()) || !event.Base().Same(batch.Base()) || !event.Next().Same(batch.Next()) || !event.Layout().Equal(batch.Layout()) || event.Range().Producer() != batch.Range().Producer() || event.Range().Kind() != batch.Range().Kind() || !event.Range().Layout().Equal(batch.Range().Layout()) {
			return signedValue{}, false
		}
		transition := signedTransition{
			base: event.Base(), next: event.Next(), authority: event.Range(),
			before: signedSide{}, after: signedSide{},
			semantic: event.SemanticChanged(), lineage: event.LineageChanged(),
		}
		semantic = semantic || transition.semantic
		lineage = lineage || transition.lineage
		before, beforeOK := event.Before()
		if beforeOK {
			beforeBatch, batchOK := tuple.NewRangeBatch(mounted, transition.authority, before.Scope(), []tuple.Tuple{before}, binding.DenominatorWitness{})
			if !batchOK || !beforeBatch.ValidFor(mounted) {
				return signedValue{}, false
			}
			transition.before = signedSide{present: true, batches: []tuple.Batch{beforeBatch}}
		}
		after, afterOK := event.After()
		if afterOK {
			afterBatch, batchOK := tuple.NewRangeBatch(mounted, transition.authority, after.Scope(), []tuple.Tuple{after}, binding.DenominatorWitness{})
			if !batchOK || !afterBatch.ValidFor(mounted) {
				return signedValue{}, false
			}
			transition.after = signedSide{present: true, batches: []tuple.Batch{afterBatch}}
		}
		if !transition.availableNoMount() || !transition.validFor(mounted) {
			return signedValue{}, false
		}
		transitions[index] = transition
	}
	value := signedValue{node: node, kind: algebra.KindInput, transitions: transitions, differentials: []applydifferential.Results{}, semantic: semantic, lineage: lineage}
	return value, value.validFor(mounted)
}

// afterBatches adapts a signed unary value to the existing positive Result
// ABI only after ascent and only when the caller has established that no
// non-empty transformed Before output needs a removal destination. It never
// participates in operator execution; explicit empty operator results remain
// authenticated empty tuple batches.
func (value signedValue) afterBatches(mounted witness.Mounted) ([]tuple.Batch, bool) {
	if !value.validFor(mounted) {
		return nil, false
	}
	result := make([]tuple.Batch, 0)
	for _, transition := range value.transitions {
		if !transition.after.present {
			continue
		}
		result = append(result, transition.after.batches...)
	}
	return result, true
}

// hasBeforeOutput reports whether the transformed predecessor contributes a
// row that would need an owner-authenticated removal. A present predecessor
// whose Select/Project output is an authenticated empty batch is a valid
// no-selection and needs no negative publication.
func (value signedValue) hasBeforeOutput() bool {
	for _, transition := range value.transitions {
		if !transition.before.present {
			continue
		}
		for _, batch := range transition.before.batches {
			if batch.Len() != 0 {
				return true
			}
		}
	}
	return false
}

func signedPathValue(value signedValue) (pathValue, bool) {
	if !value.availableNoMount() {
		return pathValue{}, false
	}
	copyOf := value
	copyOf.transitions = make([]signedTransition, len(value.transitions))
	copy(copyOf.transitions, value.transitions)
	copyOf.differentials = copyDifferentials(value.differentials)
	differentials := copyDifferentials(value.differentials)
	result := pathValue{
		node: value.node, kind: value.kind, batches: []tuple.Batch{},
		applications: []apply.Results{}, differentials: differentials, settlements: []publish.Settlement{}, signed: &copyOf,
	}
	return result, result.availableNoMount()
}

func copyDifferentials(values []applydifferential.Results) []applydifferential.Results {
	if values == nil {
		return nil
	}
	result := make([]applydifferential.Results, len(values))
	copy(result, values)
	return result
}
