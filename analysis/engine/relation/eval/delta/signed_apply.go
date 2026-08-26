package delta

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// signedApplyPath recognizes exactly the production delta vertical owned by
// this file: Input->[Select/Project]*->Apply, optionally carried through the
// Publish boundary. The scan is over the sealed zipper only; it never walks a
// logical expression or infers a missing sibling.
func signedApplyPath(path derivation.Path) (int, bool) {
	if !path.Available() || path.FrameCount() == 0 {
		return 0, false
	}
	applyIndex := -1
	for index := path.FrameCount() - 1; index >= 0; index-- {
		frame, ok := path.FrameAt(index)
		if !ok || !frame.Available() {
			return 0, false
		}
		switch frame.Kind() {
		case algebra.KindSelect, algebra.KindProject:
			if applyIndex >= 0 {
				// A unary operator outside Apply would make the semantic
				// application transport a relation again. It is not part of
				// this signed Apply vertical and must not be guessed.
				return 0, false
			}
		case algebra.KindApply:
			if applyIndex >= 0 {
				return 0, false
			}
			applyIndex = index
		case algebra.KindPublish:
			if applyIndex < 0 {
				// Publish is a root-side boundary. Seeing it below Apply
				// means the path shape is not an authored Apply child.
				return 0, false
			}
		default:
			return 0, false
		}
	}
	return applyIndex, applyIndex >= 0
}

// executeSignedApplyPath evaluates each side of the canonical Input event at
// its owning epoch, applies every sealed unary frame, and executes the one
// unary Apply child. The resulting Apply extents are paired by differential's
// exact InvocationAddress zipper and retained privately until Publish.
func (session Session) executeSignedApplyPath(root arrangement.Node, input identity.ContentID, path derivation.Path, layout arrangement.Layout, authority arrangement.RangeBinding, applyIndex int) (pathValue, bool, bool) {
	if !session.Available() || !root.Available() || !input.Available() || !path.Available() || !layout.Available() || !authority.Available() || applyIndex < 0 || applyIndex >= path.FrameCount() {
		return pathValue{}, false, false
	}
	changed, active, changedOK := session.changedEvents(layout, authority)
	if !changedOK {
		return pathValue{}, false, false
	}
	if !active {
		return pathValue{}, false, true
	}
	current, currentOK := signedInput(input, changed, session.mounted)
	if !currentOK {
		return pathValue{}, false, false
	}
	// Frames are sealed root-to-leaf. Redeem the unary child suffix first.
	for index := path.FrameCount() - 1; index > applyIndex; index-- {
		frame, frameOK := path.FrameAt(index)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, currentOK = session.ascendSigned(current, frame)
		if !currentOK {
			return pathValue{}, false, false
		}
	}
	applyFrame, frameOK := path.FrameAt(applyIndex)
	if !frameOK || applyFrame.Kind() != algebra.KindApply {
		return pathValue{}, false, false
	}
	current, currentOK = session.ascendSigned(current, applyFrame)
	if !currentOK {
		return pathValue{}, false, false
	}
	// Only the publication boundary may remain above Apply. The detector has
	// already sealed that shape; ascendSigned repeats the owner checks.
	for index := applyIndex - 1; index >= 0; index-- {
		frame, frameOK := path.FrameAt(index)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, currentOK = session.ascendSigned(current, frame)
		if !currentOK {
			return pathValue{}, false, false
		}
	}
	value, valueOK := signedPathValue(current)
	if !valueOK || !value.available(session.mounted) {
		return pathValue{}, false, false
	}
	return value, true, true
}

// ascendSignedApply executes the exact one-child Apply binding for both
// signed sides. Missing sides remain sparse zero Results; present empty
// batches are still executed as authenticated no-selection extents.
func (session Session) ascendSignedApply(current signedValue, frame derivation.Frame) (signedValue, bool) {
	if !session.Available() || !current.validFor(session.mounted) || !frame.Available() || frame.Kind() != algebra.KindApply || current.kind == algebra.KindApply || current.kind == algebra.KindPublish {
		return signedValue{}, false
	}
	node, nodeOK := session.execution.LogicalNode(frame.Node())
	if !nodeOK || !node.Available() || node.Kind() != algebra.KindApply {
		return signedValue{}, false
	}
	bindingValue, bindingOK := node.Apply()
	if !bindingOK || !bindingValue.Available() || bindingValue.ChildCount() != 1 || bindingValue.Correlation().Specified() || frame.Ordinal() != 0 || frame.Operation() != bindingValue.Operation() {
		return signedValue{}, false
	}
	if !sameApplySources(frame.SlotSources(), bindingValue.SlotSource()) {
		return signedValue{}, false
	}
	deliveries := bindingValue.Deliveries()
	if len(deliveries) == 0 || frame.SiblingCount() != len(deliveries) {
		return signedValue{}, false
	}
	for _, delivery := range deliveries {
		if !delivery.Available() || delivery.Requirement().Operation() != bindingValue.Operation() || !delivery.Requirement().Input().Available() {
			return signedValue{}, false
		}
		if delivery.Requirement().Input().Delivery.IsSpan() && !delivery.Requirement().Input().Delivery.IsComplete() {
			// A bounded span would require a replay witness and is
			// deliberately refused here. CompleteSpan remains one sealed
			// child extent and can be redeemed without a state scan.
			return signedValue{}, false
		}
		if _, siblingOK := siblingForDelivery(frame, delivery); !siblingOK {
			return signedValue{}, false
		}
	}
	witnesses, witnessesOK := signedApplyWitnesses(session.mounted, deliveries)
	if !witnessesOK {
		return signedValue{}, false
	}
	beforeBatches, beforePresent, beforeOK := signedApplySide(current, true)
	if !beforeOK {
		return signedValue{}, false
	}
	afterBatches, afterPresent, afterOK := signedApplySide(current, false)
	if !afterOK || (!beforePresent && !afterPresent) {
		return signedValue{}, false
	}
	before, beforeOK := session.executeSignedApplyExtent(bindingValue, beforeBatches, beforePresent, witnesses)
	if !beforeOK {
		return signedValue{}, false
	}
	after, afterOK := session.executeSignedApplyExtent(bindingValue, afterBatches, afterPresent, witnesses)
	if !afterOK {
		return signedValue{}, false
	}
	paired, pairOK := applydifferential.Pair(before, after)
	if !pairOK || !paired.Available() {
		return signedValue{}, false
	}
	result := signedValue{
		node:          frame.Node(),
		kind:          algebra.KindApply,
		transitions:   []signedTransition{},
		differentials: []applydifferential.Results{paired},
		semantic:      current.semantic,
		lineage:       current.lineage,
	}
	return result, result.validFor(session.mounted)
}

func sameApplySources(left, right []algebra.SlotSource) bool {
	if left == nil || right == nil || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func signedApplyWitnesses(mounted witness.Mounted, deliveries []arrangement.DeliveryBinding) ([]binding.DenominatorWitness, bool) {
	if !mounted.Available() || deliveries == nil || len(deliveries) == 0 {
		return nil, false
	}
	result := make([]binding.DenominatorWitness, len(deliveries))
	for index, delivery := range deliveries {
		if !delivery.Available() {
			return nil, false
		}
		input := delivery.Requirement().Input()
		value, ok := mounted.Denominator(input.Denominator)
		if !ok || !value.Available() || !value.ValidFor(mounted.RuntimeFence()) || !value.Matches(input.Denominator) {
			return nil, false
		}
		result[index] = value
	}
	return result, true
}

func signedApplySide(current signedValue, before bool) ([]tuple.Batch, bool, bool) {
	if !current.availableNoMount() {
		return nil, false, false
	}
	result := make([]tuple.Batch, 0)
	present := false
	for _, transition := range current.transitions {
		side := transition.after
		if before {
			side = transition.before
		}
		if !side.availableNoMount() {
			return nil, false, false
		}
		if !side.present {
			continue
		}
		present = true
		result = append(result, side.batches...)
	}
	return result, present, true
}

func (session Session) executeSignedApplyExtent(plan arrangement.ApplyBinding, batches []tuple.Batch, present bool, witnesses []binding.DenominatorWitness) (apply.Results, bool) {
	if !session.Available() || !plan.Available() || batches == nil || witnesses == nil {
		return apply.Results{}, false
	}
	if !present {
		return apply.Results{}, true
	}
	children := [][]tuple.Batch{append([]tuple.Batch{}, batches...)}
	result, ok := apply.Execute(plan, session.mounted, children, session.geometry, witness.Scope{}, witnesses)
	if !ok || !result.Available() || result.Operation() != plan.Operation() {
		return apply.Results{}, false
	}
	return result, true
}
