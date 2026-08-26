package delta

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	correlatedreplay "github.com/wippyai/go-lua/analysis/engine/relation/eval/replay"
	completeop "github.com/wippyai/go-lua/analysis/engine/relation/operator/complete"
	expandop "github.com/wippyai/go-lua/analysis/engine/relation/operator/expand"
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/join"
	mergeop "github.com/wippyai/go-lua/analysis/engine/relation/operator/merge"
	projectop "github.com/wippyai/go-lua/analysis/engine/relation/operator/project"
	selectop "github.com/wippyai/go-lua/analysis/engine/relation/operator/select"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	eventtuple "github.com/wippyai/go-lua/analysis/engine/relation/tuple/event"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// executePath redeems one changed Input occurrence and ascends its sealed
// parent frames in reverse.  The path is a zipper: no logical expression is
// walked and no intermediate node result is cached.
func (session Session) executePath(root arrangement.Node, path derivation.Path, all derivation.Plan) (value pathValue, active, valid bool) {
	if !session.Available() || !root.Available() || !path.Available() || !all.Available() {
		return pathValue{}, false, false
	}
	inputNode, ok := session.execution.LogicalNode(path.Node())
	if !ok || !inputNode.Available() || inputNode.Kind() != algebra.KindInput {
		return pathValue{}, false, false
	}
	inputBinding, ok := inputNode.Input()
	if !ok || !inputBinding.Available() {
		return pathValue{}, false, false
	}
	inputValues := inputBinding.Values()
	if !inputValues.Available() || inputValues.Digest() != path.Leaf().Physical() || inputBinding.Relation() != path.LeafRelation() || !sameColumnIDs(inputValues.Columns(), path.ReadColumns()) {
		return pathValue{}, false, false
	}
	layout, ok := session.layout(path.Leaf().Physical())
	if !ok || !layout.Available() || !layout.ValidFor(session.mounted.Fence()) {
		return pathValue{}, false, false
	}
	// The physical digest is the issued derivation identity.  The remaining
	// check is deliberately expressed in the owning APIs: only the logical
	// relation and ordered read vector cross the sealed derivation boundary.
	resolved := layout.Access()
	if !resolved.Available() || resolved.Relation() != path.LeafRelation() || resolved.Key().Available() || !sameColumnIDs(resolved.Columns(), path.ReadColumns()) {
		return pathValue{}, false, false
	}
	rangeAuthority, ok := inputNode.Range()
	if !ok || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindInput {
		return pathValue{}, false, false
	}
	if applyIndex, applyFrame, applyBinding, correlated, correlationOK := session.correlatedApplyFrame(path); !correlationOK {
		return pathValue{}, false, false
	} else if correlated {
		return session.executeCorrelatedReplayPath(root, path, all, applyIndex, applyFrame, applyBinding)
	}
	// Unary Input->Select/Project paths consume the canonical signed event
	// vector. Their Base and Next sides are transformed independently, so a
	// sparse removal or key replacement cannot be erased by the positive
	// After-only path used by compound operators.
	if applyIndex, signedApply := signedApplyPath(path); signedApply {
		return session.executeSignedApplyPath(root, inputNode.Digest(), path, layout, rangeAuthority, applyIndex)
	}
	if frame, unary := directUnaryFrame(path); unary {
		return session.executeSignedUnaryPath(root, inputNode.Digest(), frame, layout, rangeAuthority)
	}
	batches, active, ok := session.changedBatches(layout, rangeAuthority)
	if !ok {
		return pathValue{}, false, false
	}
	if !active {
		return pathValue{}, false, true
	}
	current, ok := relationValue(inputNode.Digest(), algebra.KindInput, batches)
	if !ok {
		return pathValue{}, false, false
	}
	for frameIndex := path.FrameCount() - 1; frameIndex >= 0; frameIndex-- {
		frame, frameOK := path.FrameAt(frameIndex)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, ok = session.ascend(root, current, frame)
		if !ok {
			return pathValue{}, false, false
		}
	}
	return current, true, current.available(session.mounted)
}

// correlatedApplyFrame locates the one sealed correlated Apply enclosing a
// leaf occurrence. The derivation path is the occurrence authority; this
// does not infer a child from its relation or expression shape.
func (session Session) correlatedApplyFrame(path derivation.Path) (int, derivation.Frame, arrangement.ApplyBinding, bool, bool) {
	if !session.Available() || !path.Available() {
		return -1, derivation.Frame{}, arrangement.ApplyBinding{}, false, false
	}
	index := -1
	var resultFrame derivation.Frame
	var resultBinding arrangement.ApplyBinding
	for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
		frame, frameOK := path.FrameAt(frameIndex)
		if !frameOK {
			return -1, derivation.Frame{}, arrangement.ApplyBinding{}, false, false
		}
		if frame.Kind() != algebra.KindApply {
			continue
		}
		node, nodeOK := session.execution.LogicalNode(frame.Node())
		if !nodeOK || !node.Available() || node.Kind() != algebra.KindApply {
			return -1, derivation.Frame{}, arrangement.ApplyBinding{}, false, false
		}
		binding, bindingOK := node.Apply()
		if !bindingOK || !binding.Available() || frame.Operation() != binding.Operation() {
			return -1, derivation.Frame{}, arrangement.ApplyBinding{}, false, false
		}
		if !binding.Correlation().Specified() {
			continue
		}
		if index >= 0 {
			return -1, derivation.Frame{}, arrangement.ApplyBinding{}, false, false
		}
		index, resultFrame, resultBinding = frameIndex, frame, binding
	}
	return index, resultFrame, resultBinding, index >= 0, true
}

// executeCorrelatedReplayPath elects the earliest sealed leaf occurrence
// under one correlated Apply as its delta pivot. If any exact leaf access of
// that Apply changed, the pivot redeems the mounted correlated operator once
// and then crosses only the outer zipper. Inner Select/Complete/Join nodes
// belong to CorrelatedSubtree and are not independently reinterpreted.
func (session Session) executeCorrelatedReplayPath(root arrangement.Node, path derivation.Path, all derivation.Plan, applyIndex int, applyFrame derivation.Frame, plan arrangement.ApplyBinding) (value pathValue, active, valid bool) {
	if !session.Available() || !root.Available() || !path.Available() || !all.Available() || applyIndex < 0 || !applyFrame.Available() || !plan.Available() || !plan.Correlation().Specified() {
		return pathValue{}, false, false
	}
	pivot := ^uint32(0)
	affected := false
	for index := 0; index < all.Len(); index++ {
		candidate, candidateOK := all.PathAt(index)
		if !candidateOK {
			return pathValue{}, false, false
		}
		candidateIndex, candidateFrame, candidatePlan, correlated, frameOK := session.correlatedApplyFrame(candidate)
		if !frameOK {
			return pathValue{}, false, false
		}
		if !correlated || candidateFrame.Node() != applyFrame.Node() || candidatePlan.Operation() != plan.Operation() {
			continue
		}
		if candidateIndex < 0 {
			return pathValue{}, false, false
		}
		if candidate.Occurrence() < pivot {
			pivot = candidate.Occurrence()
		}
		active, activeOK := session.pathLeafChanged(candidate)
		if !activeOK {
			return pathValue{}, false, false
		}
		affected = affected || active
	}
	if pivot == ^uint32(0) {
		return pathValue{}, false, false
	}
	if path.Occurrence() != pivot || !affected {
		return pathValue{}, false, true
	}
	values := make([]apply.Results, 0)
	completed, replayOK := correlatedreplay.Full(plan, session.mounted, session.delta.Next(), session.geometry, session.scratch, func(_ correlatedreplay.CoordinateEvidence, result apply.Results) bool {
		values = append(values, result)
		return true
	})
	if !replayOK || !completed {
		return pathValue{}, false, false
	}
	node, nodeOK := session.execution.LogicalNode(applyFrame.Node())
	if !nodeOK || !node.Available() || node.Kind() != algebra.KindApply {
		return pathValue{}, false, false
	}
	current, currentOK := applyValue(node.Digest(), values)
	if !currentOK {
		return pathValue{}, false, false
	}
	for frameIndex := applyIndex - 1; frameIndex >= 0; frameIndex-- {
		frame, frameOK := path.FrameAt(frameIndex)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, currentOK = session.ascend(root, current, frame)
		if !currentOK {
			return pathValue{}, false, false
		}
	}
	return current, true, current.available(session.mounted)
}

func (session Session) pathLeafChanged(path derivation.Path) (bool, bool) {
	if !session.Available() || !path.Available() {
		return false, false
	}
	node, nodeOK := session.execution.LogicalNode(path.Node())
	if !nodeOK || !node.Available() || node.Kind() != algebra.KindInput {
		return false, false
	}
	binding, bindingOK := node.Input()
	layout, layoutOK := session.layout(path.Leaf().Physical())
	rangeAuthority, rangeOK := node.Range()
	if !bindingOK || !binding.Available() || !layoutOK || !layout.Available() || !layout.ValidFor(session.mounted.Fence()) || !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindInput || binding.Relation() != path.LeafRelation() || !sameColumnIDs(layout.Columns(), path.ReadColumns()) {
		return false, false
	}
	_, active, changedOK := session.changedBatches(layout, rangeAuthority)
	return active, changedOK
}

// executePathToExpand redeems one mount-sealed watcher only up to its Expand
// boundary. The changed Input frontier is therefore available to the
// evaluator as exact C source identities, while the child-side sibling frames
// are read from one fixed successor epoch. It never crosses Expand or scans a
// relation.
func (session Session) executePathToExpand(root arrangement.Node, path derivation.Path, all derivation.Plan, watcher derivation.ExpandWatcher) (pathValue, bool, bool) {
	if !session.Available() || !root.Available() || !path.Available() || !all.Available() || !watcher.Available() || watcher.PathOccurrence() != path.Occurrence() || !watcher.Leaf().Equal(path.Leaf()) {
		return pathValue{}, false, false
	}
	frameIndex := int(watcher.StopFrame())
	if frameIndex < 0 || frameIndex >= path.FrameCount() {
		return pathValue{}, false, false
	}
	expandFrame, frameOK := path.FrameAt(frameIndex)
	if !frameOK || expandFrame.Kind() != algebra.KindExpand || expandFrame.Node() != watcher.StopFrameDigest() {
		return pathValue{}, false, false
	}
	inputNode, inputOK := session.execution.LogicalNode(path.Node())
	if !inputOK || !inputNode.Available() || inputNode.Kind() != algebra.KindInput {
		return pathValue{}, false, false
	}
	inputBinding, inputOK := inputNode.Input()
	if !inputOK || !inputBinding.Available() || inputBinding.Relation() != path.LeafRelation() {
		return pathValue{}, false, false
	}
	inputValues := inputBinding.Values()
	layout, layoutOK := session.layout(path.Leaf().Physical())
	if !layoutOK || !layout.Available() || !layout.ValidFor(session.mounted.Fence()) || layout.KeyWidth() != 0 || layout.Access().Relation() != path.LeafRelation() || !sameColumnIDs(layout.Columns(), path.ReadColumns()) || !inputValues.Available() || inputValues.Digest() != path.Leaf().Physical() || !inputValues.Equal(layout) {
		return pathValue{}, false, false
	}
	rangeAuthority, rangeOK := inputNode.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindInput || rangeAuthority.Layout().KeyWidth() != 0 || len(rangeAuthority.Layout().Columns()) != 0 || rangeAuthority.Layout().Digest() != watcher.Range().Physical() || !sameArrangementDerivationAccess(rangeAuthority.Layout().Access(), watcher.Range().Access()) {
		return pathValue{}, false, false
	}
	batches, active, changedOK := session.changedBatches(layout, rangeAuthority)
	if !changedOK {
		return pathValue{}, false, false
	}
	if !active {
		return pathValue{}, false, true
	}
	current, currentOK := relationValue(inputNode.Digest(), algebra.KindInput, batches)
	if !currentOK {
		return pathValue{}, false, false
	}
	for index := path.FrameCount() - 1; index > frameIndex; index-- {
		frame, frameOK := path.FrameAt(index)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, currentOK = session.ascendEpoch(root, current, frame, session.delta.Next(), true)
		if !currentOK {
			return pathValue{}, false, false
		}
	}
	return current, true, current.available(session.mounted)
}

// executeExpandReplayPath redeems the complete affected C-left at the
// successor epoch named by an ExpandReplay seal. Candidate RowIDs are
// replayed through the state Reader's exact filtered inverse; every common
// cofiber is retained in encounter order, then the Expand is crossed once and
// the already-sealed outer zipper is ascended normally.
func (session Session) executeExpandReplayPath(root arrangement.Node, path derivation.Path, all derivation.Plan, delta expandReaderDelta) (pathValue, bool, bool) {
	if !session.Available() || !root.Available() || !path.Available() || !all.Available() || !delta.trigger.Available() || !delta.binding.Available() || len(delta.candidates) == 0 {
		return pathValue{}, false, false
	}
	replay := delta.trigger.Replay()
	if !replay.Available() || replay.EmitOccurrence() != replay.Anchor().PathOccurrence() || path.Occurrence() != replay.EmitOccurrence() {
		return pathValue{}, false, false
	}
	frameIndex := int(delta.trigger.FrameOrdinal())
	if frameIndex < 0 || frameIndex >= path.FrameCount() {
		return pathValue{}, false, false
	}
	expandFrame, frameOK := path.FrameAt(frameIndex)
	if !frameOK || expandFrame.Kind() != algebra.KindExpand || expandFrame.Node() != delta.trigger.Node() || expandFrame.ExpandContract() != delta.binding.Contract() {
		return pathValue{}, false, false
	}
	anchor := replay.Anchor()
	if !anchor.Available() || anchor.PathOccurrence() != path.Occurrence() || !anchor.Access().Equal(path.Leaf()) {
		return pathValue{}, false, false
	}
	candidateSibling, candidateSiblingOK := expandFrame.SiblingAt(0)
	readerSibling, readerSiblingOK := expandFrame.SiblingAt(1)
	if !candidateSiblingOK || !readerSiblingOK || !candidateSibling.Equal(anchor.Access()) || !readerSibling.Equal(delta.trigger.Reader()) {
		return pathValue{}, false, false
	}
	inputNode, inputOK := session.execution.LogicalNode(path.Node())
	if !inputOK || !inputNode.Available() || inputNode.Kind() != algebra.KindInput {
		return pathValue{}, false, false
	}
	inputBinding, inputOK := inputNode.Input()
	if !inputOK || !inputBinding.Available() || inputBinding.Relation() != delta.binding.Contract().Candidate() || inputBinding.Relation() != path.LeafRelation() {
		return pathValue{}, false, false
	}
	inputValues := inputBinding.Values()
	candidateLayout := delta.binding.Candidate()
	if !inputValues.Available() || !candidateLayout.Available() || !candidateLayout.Equal(anchorLayout(session, anchor.Access())) || !inputValues.Equal(candidateLayout) || inputValues.Digest() != path.Leaf().Physical() || !sameColumnIDs(inputValues.Columns(), path.ReadColumns()) || candidateLayout.KeyWidth() != 0 || candidateLayout.Access().Relation() != path.LeafRelation() || !sameColumnIDs(candidateLayout.Columns(), path.ReadColumns()) {
		return pathValue{}, false, false
	}
	rangeAuthority, rangeOK := inputNode.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindInput || rangeAuthority.Layout().KeyWidth() != 0 || len(rangeAuthority.Layout().Columns()) != 0 || rangeAuthority.Layout().Digest() != anchor.Range().Physical() || !sameArrangementDerivationAccess(rangeAuthority.Layout().Access(), anchor.Range().Access()) {
		return pathValue{}, false, false
	}
	candidateReader, readerOK := read.Bind(session.delta.Next(), candidateLayout, session.geometry, session.scratch)
	if !readerOK || !candidateReader.Available() || !candidateReader.Layout().Equal(candidateLayout) {
		return pathValue{}, false, false
	}
	type partition struct {
		scope  witness.Scope
		values []tuple.Tuple
	}
	partitions := make([]partition, 0)
	replayValid := true
	completed, valid := candidateReader.ReplayRowIDs(delta.candidates, func(row read.Row) bool {
		if row == nil || !row.Available() || row.ID().Relation() != delta.binding.Contract().Candidate() {
			replayValid = false
			return false
		}
		value, tupleOK := tuple.Input(session.mounted, candidateReader, row)
		if !tupleOK || !value.Available() || !value.Scope().Same(row.Scope()) {
			replayValid = false
			return false
		}
		if len(partitions) != 0 && partitions[len(partitions)-1].scope.Same(value.Scope()) {
			partitions[len(partitions)-1].values = append(partitions[len(partitions)-1].values, value)
		} else {
			partitions = append(partitions, partition{scope: value.Scope(), values: []tuple.Tuple{value}})
		}
		return true
	})
	if !completed || !valid || !replayValid {
		return pathValue{}, false, false
	}
	if len(partitions) == 0 {
		return pathValue{}, false, true
	}
	batches := make([]tuple.Batch, len(partitions))
	for index, partition := range partitions {
		batch, batchOK := tuple.NewRangeBatch(session.mounted, rangeAuthority, partition.scope, partition.values, bindingpkg.DenominatorWitness{})
		if !batchOK {
			return pathValue{}, false, false
		}
		batches[index] = batch
	}
	current, currentOK := relationValue(inputNode.Digest(), algebra.KindInput, batches)
	if !currentOK {
		return pathValue{}, false, false
	}
	for index := path.FrameCount() - 1; index > frameIndex; index-- {
		frame, frameOK := path.FrameAt(index)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, currentOK = session.ascendEpoch(root, current, frame, session.delta.Next(), true)
		if !currentOK {
			return pathValue{}, false, false
		}
	}
	current, currentOK = session.ascend(root, current, expandFrame)
	if !currentOK {
		return pathValue{}, false, false
	}
	for index := frameIndex - 1; index >= 0; index-- {
		frame, frameOK := path.FrameAt(index)
		if !frameOK {
			return pathValue{}, false, false
		}
		current, currentOK = session.ascend(root, current, frame)
		if !currentOK {
			return pathValue{}, false, false
		}
	}
	return current, true, current.available(session.mounted)
}

func anchorLayout(session Session, access derivation.SiblingAccess) arrangement.Layout {
	if !access.Available() {
		return arrangement.Layout{}
	}
	layout, ok := session.layout(access.Physical())
	if !ok {
		return arrangement.Layout{}
	}
	return layout
}

func sameArrangementDerivationAccess(left arrangement.Access, right derivation.Access) bool {
	return left.Available() && right.Available() && left.Relation() == right.Relation() && left.Key() == right.Key() && sameColumnIDs(left.Columns(), right.Columns())
}

func (session Session) ascend(root arrangement.Node, current pathValue, frame derivation.Frame) (pathValue, bool) {
	return session.ascendEpoch(root, current, frame, database.Version{}, false)
}

func directUnaryFrame(path derivation.Path) (derivation.Frame, bool) {
	if !path.Available() || path.FrameCount() != 1 {
		return derivation.Frame{}, false
	}
	frame, ok := path.FrameAt(0)
	if !ok || !frame.Available() {
		return derivation.Frame{}, false
	}
	switch frame.Kind() {
	case algebra.KindSelect, algebra.KindProject:
		return frame, true
	default:
		return derivation.Frame{}, false
	}
}

// executeSignedUnaryPath performs the signed unary ascent for a direct
// Input->Select/Project occurrence. The event vector owns both roots; each
// present side is lowered and ascended against its own root before the value
// reaches the positive Result boundary, where unsupported removals refuse.
func (session Session) executeSignedUnaryPath(root arrangement.Node, input identity.ContentID, frame derivation.Frame, layout arrangement.Layout, authority arrangement.RangeBinding) (pathValue, bool, bool) {
	if !session.Available() || !root.Available() || !input.Available() || !frame.Available() || !layout.Available() || !authority.Available() || root.Digest() != frame.Node() {
		return pathValue{}, false, false
	}
	switch frame.Kind() {
	case algebra.KindSelect:
		// Select has no physical sibling; its child is the active leaf.
		if frame.SiblingCount() != 0 {
			return pathValue{}, false, false
		}
	case algebra.KindProject:
		// Project's first two siblings are the complete target and key
		// layouts; the remaining source vectors are mount metadata for the
		// authored correspondence. A direct child has exactly one source
		// relation, so reject extra groups instead of allowing a partial
		// mapping to masquerade as a unary input. The source sibling's
		// physical vector is intentionally not compared with the Input
		// values vector: mount may retain only mapped source columns there.
		if frame.SiblingCount() != 3 {
			return pathValue{}, false, false
		}
	default:
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
	current, currentOK = session.ascendSigned(current, frame)
	if !currentOK {
		return pathValue{}, false, false
	}
	value, valueOK := signedPathValue(current)
	if !valueOK || !value.available(session.mounted) {
		return pathValue{}, false, false
	}
	return value, true, true
}

func (session Session) ascendSigned(current signedValue, frame derivation.Frame) (signedValue, bool) {
	if !session.Available() || !current.validFor(session.mounted) || !frame.Available() {
		return signedValue{}, false
	}
	switch frame.Kind() {
	case algebra.KindSelect:
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || !node.Available() || node.Kind() != algebra.KindSelect {
			return signedValue{}, false
		}
		binding, ok := node.Select()
		if !ok || !binding.Available() || binding.Scope() != frame.Scope() {
			return signedValue{}, false
		}
		transitions := make([]signedTransition, len(current.transitions))
		for index, transition := range current.transitions {
			before, beforeOK := session.selectSignedSide(binding, transition.before)
			if !beforeOK {
				return signedValue{}, false
			}
			after, afterOK := session.selectSignedSide(binding, transition.after)
			if !afterOK {
				return signedValue{}, false
			}
			transition.before, transition.after = before, after
			transitions[index] = transition
		}
		value := signedValue{node: frame.Node(), kind: algebra.KindSelect, transitions: transitions, differentials: copyDifferentials(current.differentials), semantic: current.semantic, lineage: current.lineage}
		return value, value.validFor(session.mounted)

	case algebra.KindProject:
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || !node.Available() || node.Kind() != algebra.KindProject {
			return signedValue{}, false
		}
		binding, ok := node.Project()
		targetSibling, targetOK := frame.SiblingAt(0)
		keySibling, keyOK := frame.SiblingAt(1)
		if !ok || !binding.Available() || !targetOK || !keyOK || !targetSibling.Available() || !keySibling.Available() || targetSibling.Physical() != binding.Target().Digest() || keySibling.Physical() != binding.Key().Digest() {
			return signedValue{}, false
		}
		targetLayout := binding.Target()
		if !targetLayout.Available() || !targetLayout.ValidFor(session.mounted.Fence()) || targetLayout.Access().Key().Available() {
			return signedValue{}, false
		}
		baseTarget, baseOK := read.Bind(session.delta.Base(), targetLayout, session.geometry, session.scratch)
		nextTarget, nextOK := read.Bind(session.delta.Next(), targetLayout, session.geometry, session.scratch)
		if !baseOK || !nextOK || !baseTarget.Available() || !nextTarget.Available() || !baseTarget.Layout().Equal(targetLayout) || !nextTarget.Layout().Equal(targetLayout) {
			return signedValue{}, false
		}
		transitions := make([]signedTransition, len(current.transitions))
		for index, transition := range current.transitions {
			if !transition.base.Same(session.delta.Base()) || !transition.next.Same(session.delta.Next()) {
				return signedValue{}, false
			}
			before, beforeOK := session.projectSignedSide(binding, transition.before, baseTarget)
			if !beforeOK {
				return signedValue{}, false
			}
			after, afterOK := session.projectSignedSide(binding, transition.after, nextTarget)
			if !afterOK {
				return signedValue{}, false
			}
			transition.before, transition.after = before, after
			transitions[index] = transition
		}
		value := signedValue{node: frame.Node(), kind: algebra.KindProject, transitions: transitions, differentials: copyDifferentials(current.differentials), semantic: current.semantic, lineage: current.lineage}
		return value, value.validFor(session.mounted)
	case algebra.KindApply:
		return session.ascendSignedApply(current, frame)
	case algebra.KindPublish:
		// Differential Apply transport is opaque through Publish until the
		// publication phase owns its signed door. Do not turn it into a
		// positive Applications vector here.
		if current.kind != algebra.KindApply || len(current.differentials) == 0 || current.transitions == nil {
			return signedValue{}, false
		}
		node, nodeOK := session.execution.LogicalNode(frame.Node())
		binding, bindingOK := node.Publish()
		if !nodeOK || !node.Available() || node.Kind() != algebra.KindPublish || !bindingOK || !binding.Available() {
			return signedValue{}, false
		}
		value := signedValue{node: frame.Node(), kind: algebra.KindPublish, transitions: []signedTransition{}, differentials: copyDifferentials(current.differentials), semantic: current.semantic, lineage: current.lineage}
		return value, value.validFor(session.mounted)
	default:
		return signedValue{}, false
	}
}

func (session Session) selectSignedSide(binding arrangement.SelectBinding, side signedSide) (signedSide, bool) {
	if !side.availableNoMount() || !side.present {
		return side, side.availableNoMount()
	}
	outputs := make([]tuple.Batch, 0)
	for _, source := range side.batches {
		values, ok := selectop.Execute(binding, session.mounted, session.geometry, source)
		if !ok || values == nil {
			return signedSide{}, false
		}
		outputs = append(outputs, values...)
	}
	result := signedSide{present: true, batches: outputs}
	return result, result.availableNoMount()
}

func (session Session) projectSignedSide(binding arrangement.ProjectBinding, side signedSide, target read.Reader) (signedSide, bool) {
	if !side.availableNoMount() || !side.present || !target.Available() {
		return side, side.availableNoMount() && (!side.present || target.Available())
	}
	outputs := make([]tuple.Batch, 0)
	for _, source := range side.batches {
		values, ok := projectop.Execute(binding, session.mounted, source, target)
		if !ok || values == nil {
			return signedSide{}, false
		}
		outputs = append(outputs, values...)
	}
	result := signedSide{present: true, batches: outputs}
	return result, result.availableNoMount()
}

// ascendEpoch is the ordinary sealed zipper ascent with an optional fixed
// sibling epoch. Replay below Expand uses the fixed successor epoch for every
// child-side Join/Apply sibling; all other callers retain the canonical
// orientation-dependent Base/Next pivot.
func (session Session) ascendEpoch(root arrangement.Node, current pathValue, frame derivation.Frame, fixedEpoch database.Version, fixed bool) (pathValue, bool) {
	if !session.Available() || !current.available(session.mounted) || !frame.Available() || current.signed != nil {
		return pathValue{}, false
	}
	switch frame.Kind() {
	case algebra.KindSelect:
		if len(current.applications) != 0 {
			return pathValue{}, false
		}
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindSelect {
			return pathValue{}, false
		}
		binding, ok := node.Select()
		if !ok || !binding.Available() || binding.Scope() != frame.Scope() {
			return pathValue{}, false
		}
		outputs := make([]tuple.Batch, 0)
		for _, batch := range current.batches {
			values, executeOK := selectop.Execute(binding, session.mounted, session.geometry, batch)
			if !executeOK || values == nil {
				return pathValue{}, false
			}
			outputs = append(outputs, values...)
		}
		return relationValue(current.node, algebra.KindSelect, outputs)

	case algebra.KindJoin:
		if len(current.applications) != 0 || frame.SiblingCount() != 1 {
			return pathValue{}, false
		}
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindJoin {
			return pathValue{}, false
		}
		binding, ok := node.Join()
		if !ok || !binding.Available() {
			return pathValue{}, false
		}
		sibling, ok := frame.SiblingAt(0)
		if !ok {
			return pathValue{}, false
		}
		// The frame carries the sibling's logical access, while the Join
		// binding owns the exact physical correspondence coordinate.  A
		// derivation has one logical binding per access and may therefore
		// retain the neutral vector digest here; redeeming that digest would
		// silently turn this indexed join into an unkeyed scan/refusal.  Use
		// the owner-issued oriented layout and authenticate the frame only
		// against its logical side.
		var layout arrangement.Layout
		if frame.Orientation() == derivation.OrientationLeft {
			layout = binding.Right()
		} else if frame.Orientation() == derivation.OrientationRight {
			layout = binding.Left()
		} else {
			return pathValue{}, false
		}
		if !layout.Available() || !layout.ValidFor(session.mounted.Fence()) || !sameArrangementDerivationAccess(layout.Access(), sibling.Access()) {
			return pathValue{}, false
		}
		var epoch database.Version
		var epochOK bool
		if fixed {
			epoch = fixedEpoch
			epochOK = epoch.Available() && epoch.Same(session.delta.Next())
		} else {
			epoch, epochOK = session.stableEpoch(frame, 0)
		}
		if !epochOK {
			return pathValue{}, false
		}
		authority, ok := session.inputRange(root, layout.Access().Relation())
		if !ok {
			return pathValue{}, false
		}
		outputs := make([]tuple.Batch, 0)
		for _, source := range current.batches {
			for index := 0; index < source.Len(); index++ {
				currentTuple, tupleOK := source.At(index)
				if !tupleOK {
					return pathValue{}, false
				}
				stable, stableOK := session.lookupJoinBatches(epoch, layout, authority, currentTuple, frame.Columns(), sibling.Access().Columns())
				if !stableOK {
					return pathValue{}, false
				}
				one, oneOK := tuple.PreserveRange(session.mounted, source, currentTuple.Scope(), []tuple.Tuple{currentTuple})
				if !oneOK {
					return pathValue{}, false
				}
				if frame.Orientation() == derivation.OrientationLeft {
					for _, right := range stable {
						value, executeOK := join.Join(binding, session.mounted, session.geometry, one, right)
						if !executeOK || !value.Available() {
							return pathValue{}, false
						}
						outputs = append(outputs, value)
					}
				} else if frame.Orientation() == derivation.OrientationRight {
					for _, left := range stable {
						value, executeOK := join.Join(binding, session.mounted, session.geometry, left, one)
						if !executeOK || !value.Available() {
							return pathValue{}, false
						}
						outputs = append(outputs, value)
					}
				} else {
					return pathValue{}, false
				}
			}
		}
		return relationValue(current.node, algebra.KindJoin, outputs)

	case algebra.KindComplete:
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindComplete {
			return pathValue{}, false
		}
		binding, ok := node.Complete()
		if !ok || !binding.Available() || len(current.applications) != 0 {
			return pathValue{}, false
		}
		replay := frame.CompleteReplay()
		if !replay.Available() {
			// A Later input carries only the changed frontier. Complete is
			// non-distributive, so an ordinary frame cannot be redeemed without
			// treating unchanged denominator members as ProvenAbsent. Refuse
			// until mount seals the exact Complete(Select(Input)) witness.
			return pathValue{}, false
		}
		value, replayOK := session.replayComplete(root, frame.Node(), current, binding, replay, fixedEpoch, fixed)
		if !replayOK {
			return pathValue{}, false
		}
		return value, true

	case algebra.KindApply:
		if len(current.applications) != 0 {
			return pathValue{}, false
		}
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindApply {
			return pathValue{}, false
		}
		binding, ok := node.Apply()
		// The Later signed Apply path admits only one independent child. A
		// multi-child or correlated binding requires a sealed product/replay
		// witness that this evaluator does not own; never guess a stable
		// sibling or fall back to changedBatches for it.
		if !ok || binding.ChildCount() != 1 || binding.Correlation().Specified() || int(frame.Ordinal()) != 0 || frame.Operation() != binding.Operation() {
			return pathValue{}, false
		}
		children := make([][]tuple.Batch, binding.ChildCount())
		children[frame.Ordinal()] = append([]tuple.Batch(nil), current.batches...)
		deliveries := binding.Deliveries()
		sources := binding.SlotSource()
		if len(deliveries) != len(sources) {
			return pathValue{}, false
		}
		for deliveryIndex, delivery := range deliveries {
			childIndex := int(sources[deliveryIndex].Child())
			if childIndex == int(frame.Ordinal()) {
				continue
			}
			sibling, siblingOK := siblingForDelivery(frame, delivery)
			if !siblingOK || !sibling.Available() {
				return pathValue{}, false
			}
			layout, layoutOK := session.layout(sibling.Physical())
			if !layoutOK || !layout.Available() || !layout.ValidFor(session.mounted.Fence()) {
				return pathValue{}, false
			}
			authority, authorityOK := session.inputRange(root, layout.Access().Relation())
			if !authorityOK {
				return pathValue{}, false
			}
			epoch := session.delta.Next()
			var epochOK bool
			if fixed {
				epoch = fixedEpoch
				epochOK = epoch.Available() && epoch.Same(session.delta.Next())
			} else {
				epoch, epochOK = stableChildEpoch(session.delta.Base(), session.delta.Next(), int(frame.Ordinal()), childIndex)
			}
			if !epochOK {
				return pathValue{}, false
			}
			value, valueOK := session.stableBatches(epoch, layout, authority)
			if !valueOK {
				return pathValue{}, false
			}
			children[childIndex] = value
		}
		witnesses := make([]bindingpkg.DenominatorWitness, len(deliveries))
		for deliveryIndex, delivery := range deliveries {
			input := delivery.Requirement().Input()
			witnessValue, witnessOK := session.mounted.Denominator(input.Denominator)
			if !witnessOK || !witnessValue.ValidFor(session.mounted.RuntimeFence()) || !witnessValue.Matches(input.Denominator) {
				return pathValue{}, false
			}
			witnesses[deliveryIndex] = witnessValue
		}
		result, executeOK := apply.Execute(binding, session.mounted, children, session.geometry, witness.Scope{}, witnesses)
		if !executeOK || !result.Available() {
			return pathValue{}, false
		}
		return applyValue(current.node, []apply.Results{result})

	case algebra.KindMerge:
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindMerge {
			return pathValue{}, false
		}
		binding, ok := node.Merge()
		if !ok || frame.ChildCount() <= 0 || int(frame.Ordinal()) >= frame.ChildCount() {
			return pathValue{}, false
		}
		activeChild, activeOK := frame.ChildAt(int(frame.Ordinal()))
		if !activeOK || !activeChild.Available() || activeChild.Kind() != current.kind {
			return pathValue{}, false
		}
		childWitnesses := make([]derivation.ChildWitness, frame.ChildCount())
		for childIndex := 0; childIndex < frame.ChildCount(); childIndex++ {
			child, childOK := frame.ChildAt(childIndex)
			if !childOK || !child.Available() {
				return pathValue{}, false
			}
			childWitnesses[childIndex] = child
			switch child.Kind() {
			case algebra.KindApply, algebra.KindInput, algebra.KindSelect, algebra.KindJoin, algebra.KindMerge, algebra.KindComplete, algebra.KindColumnProject, algebra.KindExpand:
			default:
				return pathValue{}, false
			}
		}
		if len(binding.ProposalOperations()) != 0 {
			return proposalMergeValue(session.mounted, binding, current)
		}
		if len(current.applications) != 0 || !relationKind(current.kind) {
			return pathValue{}, false
		}
		return session.recomputeMergeAffected(root, binding, frame, current, childWitnesses)

	case algebra.KindColumnProject:
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindColumnProject {
			return pathValue{}, false
		}
		binding, ok := node.ColumnProject()
		sibling, siblingOK := frame.SiblingAt(0)
		if !ok || !binding.Available() || !siblingOK || binding.Values().Digest() != sibling.Physical() || binding.SlotCount() != len(frame.Slots()) {
			return pathValue{}, false
		}
		for index, slot := range frame.Slots() {
			candidate, candidateOK := binding.SlotAt(index)
			if !candidateOK || candidate != slot {
				return pathValue{}, false
			}
		}
		projected, projectOK := projectBatches(session.mounted, current.batches, binding)
		if !projectOK {
			return pathValue{}, false
		}
		if len(current.applications) != 0 {
			return carriedValue(current.node, algebra.KindColumnProject, projected, current.applications)
		}
		return relationValue(current.node, algebra.KindColumnProject, projected)

	case algebra.KindExpand:
		if len(current.applications) != 0 || frame.SiblingCount() != 3 {
			return pathValue{}, false
		}
		node, ok := session.execution.LogicalNode(frame.Node())
		if !ok || node.Kind() != algebra.KindExpand {
			return pathValue{}, false
		}
		binding, ok := node.Expand()
		if !ok || !binding.Available() || binding.Contract() != frame.ExpandContract() || binding.Evidence().Digest() != frame.ExpandEvidence() {
			return pathValue{}, false
		}
		reader, readerOK := read.Bind(session.delta.Next(), binding.Reader(), session.geometry, session.scratch)
		if !readerOK || !reader.Available() {
			return pathValue{}, false
		}
		outputs := make([]tuple.Batch, 0, len(current.batches))
		for _, source := range current.batches {
			values, executeOK := expandop.Execute(binding.Evidence(), session.mounted, session.geometry, source, reader)
			if !executeOK || values == nil {
				return pathValue{}, false
			}
			outputs = append(outputs, values...)
		}
		return relationValue(frame.Node(), algebra.KindExpand, outputs)

	case algebra.KindPublish:
		if len(current.batches) != 0 && len(current.applications) == 0 {
			return pathValue{}, false
		}
		applications := make([]apply.Results, len(current.applications))
		copy(applications, current.applications)
		return pathValue{node: current.node, kind: algebra.KindPublish, batches: []tuple.Batch{}, applications: applications, differentials: []applydifferential.Results{}, settlements: []publish.Settlement{}}, true

	case algebra.KindGroup:
		// Group remains unavailable on an ordinary positive path; its
		// non-distributive extent requires a sealed key replay.
		return pathValue{}, false
	default:
		return pathValue{}, false
	}
}

// proposalMergeValue is the differential redemption of the same sealed
// proposal authority used by the full evaluator. A carried relation child is
// an authenticated no-op; an Apply-derived child preserves its Results even
// when that extent is empty.
func proposalMergeValue(mounted witness.Mounted, binding arrangement.MergeBinding, current pathValue) (pathValue, bool) {
	if !mounted.Available() || !binding.Available() || len(binding.ProposalOperations()) == 0 || !current.available(mounted) {
		return pathValue{}, false
	}
	for _, results := range current.applications {
		if !results.Available() || !binding.AcceptsProposal(current.node, results.Operation()) {
			return pathValue{}, false
		}
	}
	return carriedValue(current.node, algebra.KindMerge, []tuple.Batch{}, current.applications)
}

// replayComplete reconstructs the full successor child extent for one
// affected scope from the mount-issued Complete(Select(Input)) witness.  The
// denominator RowIDs are already ordered by the mounted owner; ReplayRowIDs
// therefore performs the bounded exact inverse without a relation scan.  The
// sealed Select is reapplied before Complete executes exactly once per scope.
func (session Session) replayComplete(root arrangement.Node, node identity.ContentID, current pathValue, complete arrangement.CompleteBinding, replay derivation.CompleteReplay, fixedEpoch database.Version, fixed bool) (pathValue, bool) {
	if !session.Available() || !root.Available() || !node.Available() || !current.available(session.mounted) || !complete.Available() || !replay.Available() || len(current.applications) != 0 {
		return pathValue{}, false
	}
	if current.kind != algebra.KindSelect || complete.Denominator() == (model.DenominatorRef{}) {
		return pathValue{}, false
	}
	inputNode, inputOK := session.execution.LogicalNode(replay.InputNode())
	selectNode, selectOK := session.execution.LogicalNode(replay.SelectNode())
	if !inputOK || !selectOK || !inputNode.Available() || !selectNode.Available() || inputNode.Kind() != algebra.KindInput || selectNode.Kind() != algebra.KindSelect || len(selectNode.Children()) != 1 {
		return pathValue{}, false
	}
	child := selectNode.Children()[0]
	if !child.Available() || child.Digest() != inputNode.Digest() {
		return pathValue{}, false
	}
	inputBinding, inputOK := inputNode.Input()
	selectBinding, selectOK := selectNode.Select()
	inputRange, rangeOK := inputNode.Range()
	order := replay.Order()
	completeKey := complete.Key()
	if !inputOK || !selectOK || !rangeOK || !inputBinding.Available() || !selectBinding.Available() || !inputRange.Available() || !order.Available() || !completeKey.Available() || replay.Denominator() != complete.Denominator() || order.Physical() != completeKey.Digest() || order.Access().Relation() != complete.Denominator().Relation() || order.Access().Key() != complete.Denominator().Key() || inputRange.Kind() != algebra.KindInput || selectBinding.Scope() != replay.Scope() || inputBinding.Values().Digest() != replay.Values().Physical() || inputBinding.Scan().Digest() != replay.Range().Physical() || inputRange.Layout().Digest() != replay.Range().Physical() || inputBinding.Relation() != complete.Denominator().Relation() {
		return pathValue{}, false
	}
	valuesLayout := inputBinding.Values()
	if !valuesLayout.Available() || !valuesLayout.ValidFor(session.mounted.Fence()) || valuesLayout.KeyWidth() != 0 || valuesLayout.Access().Relation() != inputBinding.Relation() {
		return pathValue{}, false
	}
	epoch := session.delta.Next()
	if fixed {
		if !fixedEpoch.Available() || !fixedEpoch.Same(epoch) {
			return pathValue{}, false
		}
		epoch = fixedEpoch
	}
	reader, readerOK := read.Bind(epoch, valuesLayout, session.geometry, session.scratch)
	if !readerOK || !reader.Available() || !reader.Layout().Equal(valuesLayout) {
		return pathValue{}, false
	}
	denominator, denominatorOK := session.mounted.Denominator(complete.Denominator())
	if !denominatorOK || !denominator.Available() || !denominator.ValidFor(session.mounted.RuntimeFence()) || !denominator.Matches(complete.Denominator()) {
		return pathValue{}, false
	}
	rowIDs := make([]model.RowID, denominator.Len())
	for index := range rowIDs {
		row, rowOK := denominator.At(index)
		if !rowOK || !row.Available() {
			return pathValue{}, false
		}
		rowIDs[index] = row
	}
	type partition struct {
		scope  witness.Scope
		values []tuple.Tuple
	}
	partitions := make([]partition, 0, len(current.batches))
	for _, batch := range current.batches {
		if !batch.ValidFor(session.mounted) {
			return pathValue{}, false
		}
		scope := batch.Scope()
		if !scope.ValidFor(session.mounted.RuntimeFence()) {
			return pathValue{}, false
		}
		found := false
		for _, prior := range partitions {
			if prior.scope.Same(scope) {
				found = true
				break
			}
		}
		if !found {
			partitions = append(partitions, partition{scope: scope})
		}
	}
	if len(partitions) == 0 {
		return relationValue(node, algebra.KindComplete, []tuple.Batch{})
	}
	frontierValid := true
	completed, valid := reader.ReplayRowIDs(rowIDs, func(row read.Row) bool {
		if row == nil || !row.Available() || row.ID().Relation() != inputBinding.Relation() {
			frontierValid = false
			return false
		}
		value, tupleOK := tuple.Input(session.mounted, reader, row)
		if !tupleOK || !value.ValidFor(session.mounted) || !value.Scope().Same(row.Scope()) {
			frontierValid = false
			return false
		}
		for index := range partitions {
			if partitions[index].scope.Same(value.Scope()) {
				partitions[index].values = append(partitions[index].values, value)
				break
			}
		}
		return true
	})
	if !completed || !valid || !frontierValid {
		return pathValue{}, false
	}
	outputs := make([]tuple.Batch, 0, len(partitions))
	denominatorWitness, witnessOK := session.mounted.Denominator(complete.Denominator())
	if !witnessOK || !denominatorWitness.ValidFor(session.mounted.RuntimeFence()) || !denominatorWitness.Matches(complete.Denominator()) {
		return pathValue{}, false
	}
	for _, partition := range partitions {
		inputBatch, inputBatchOK := tuple.NewRangeBatch(session.mounted, inputRange, partition.scope, partition.values, bindingpkg.DenominatorWitness{})
		if !inputBatchOK {
			return pathValue{}, false
		}
		selected, selectedOK := selectop.Execute(selectBinding, session.mounted, session.geometry, inputBatch)
		if !selectedOK || len(selected) != 1 {
			return pathValue{}, false
		}
		completedBatch, completedOK := completeop.Execute(complete, session.mounted, selected[0], denominatorWitness)
		if !completedOK || !completedBatch.ValidFor(session.mounted) || !completedBatch.Scope().Same(partition.scope) {
			return pathValue{}, false
		}
		outputs = append(outputs, completedBatch)
	}
	return relationValue(node, algebra.KindComplete, outputs)
}

func siblingForDelivery(frame derivation.Frame, delivery arrangement.DeliveryBinding) (derivation.SiblingAccess, bool) {
	if !frame.Available() || !delivery.Available() {
		return derivation.SiblingAccess{}, false
	}
	var result derivation.SiblingAccess
	position := -1
	for index := 0; index < frame.SiblingCount(); index++ {
		candidate, candidateOK := frame.SiblingAt(index)
		if !candidateOK || candidate.Physical() != delivery.Layout().Digest() {
			continue
		}
		if position >= 0 {
			return derivation.SiblingAccess{}, false
		}
		result, position = candidate, index
	}
	return result, position >= 0
}

func sameColumnIDs(left, right []model.ColumnID) bool {
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

func projectBatches(mounted witness.Mounted, source []tuple.Batch, binding arrangement.ColumnProjectBinding) ([]tuple.Batch, bool) {
	if !mounted.Available() || !binding.Available() || source == nil {
		return nil, false
	}
	result := make([]tuple.Batch, 0, len(source))
	for _, batch := range source {
		if !batch.ValidFor(mounted) {
			return nil, false
		}
		values := make([]tuple.Tuple, batch.Len())
		for index := 0; index < batch.Len(); index++ {
			value, ok := batch.At(index)
			if !ok {
				return nil, false
			}
			projected, projectedOK := tuple.ProjectColumns(mounted, value, binding)
			if !projectedOK {
				return nil, false
			}
			values[index] = projected
		}
		output, outputOK := tuple.PreserveRange(mounted, batch, batch.Scope(), values)
		if !outputOK {
			return nil, false
		}
		result = append(result, output)
	}
	return result, true
}

// changedEvents binds the changed frontier to the canonical immutable
// before/after tuple-event vector. No side is synthesized and no relation
// scan is available at this boundary.
func (session Session) changedEvents(layout arrangement.Layout, authority arrangement.RangeBinding) (eventtuple.Batch, bool, bool) {
	if !session.Available() || !layout.Available() || !authority.Available() {
		return eventtuple.Batch{}, false, false
	}
	changed, ok := eventtuple.Bind(session.mounted, session.delta, layout, authority, session.geometry, session.scratch)
	if !ok || !changed.Available() || !changed.ValidFor(session.mounted) || !changed.Base().Same(session.delta.Base()) || !changed.Next().Same(session.delta.Next()) || !changed.Layout().Equal(layout) {
		return eventtuple.Batch{}, false, false
	}
	return changed, changed.Len() != 0, true
}

// changedBatches is the legacy positive projection for compound paths. The
// source is still the canonical event batch; signed unary paths never call
// this adapter. A missing After is therefore a sparse removal, not a
// fabricated tuple or a refusal of the underlying event transition.
func (session Session) changedBatches(layout arrangement.Layout, authority arrangement.RangeBinding) ([]tuple.Batch, bool, bool) {
	changes, active, ok := session.changedEvents(layout, authority)
	if !ok {
		return nil, false, false
	}
	if !active {
		return []tuple.Batch{}, false, true
	}
	partitions := make([]struct {
		scope  witness.Scope
		values []tuple.Tuple
	}, 0)
	for index := 0; index < changes.Len(); index++ {
		transition, transitionOK := changes.At(index)
		if !transitionOK || !transition.Available() || !transition.ValidFor(session.mounted) {
			return nil, false, false
		}
		value, valueOK := transition.After()
		if !valueOK {
			// Keep the event active for callers that need removal wake-up, but
			// do not invent a successor tuple in this positive adapter.
			continue
		}
		found := -1
		for index := range partitions {
			if partitions[index].scope.Same(value.Scope()) {
				found = index
				break
			}
		}
		if found < 0 {
			partitions = append(partitions, struct {
				scope  witness.Scope
				values []tuple.Tuple
			}{scope: value.Scope(), values: []tuple.Tuple{value}})
		} else {
			partitions[found].values = append(partitions[found].values, value)
		}
	}
	batches := make([]tuple.Batch, len(partitions))
	for index, partition := range partitions {
		batch, batchOK := tuple.NewRangeBatch(session.mounted, authority, partition.scope, partition.values, bindingpkg.DenominatorWitness{})
		if !batchOK {
			return nil, false, false
		}
		batches[index] = batch
	}
	return batches, true, true
}

// stableBatches deliberately has no full-reader fallback. Apply's non-pivot
// child has no changed tuple key from which the sealed delivery vector could
// issue a Lookup; enumerating that child would require Reader.Scan/index.Scan,
// which is outside the Later ABI. Refuse the path until the derivation carries
// an exact correlated lookup witness.
func (session Session) stableBatches(root database.Version, layout arrangement.Layout, authority arrangement.RangeBinding) ([]tuple.Batch, bool) {
	if !session.Available() || !root.Available() || !layout.Available() || !authority.Available() || !layout.ValidFor(session.mounted.Fence()) {
		return nil, false
	}
	return nil, false
}

// recomputeMergeAffected redeems one differential Merge occurrence through
// the sealed parent key coordinate and child witnesses. The changed path
// supplies the affected (scope,key) frontier; every authored alternative is
// then looked up at the successor root for exactly those keys before the merge
// owner folds them.
// There is no relation-wide scan and no post-hoc output deduplication.
func (session Session) recomputeMergeAffected(root arrangement.Node, binding arrangement.MergeBinding, frame derivation.Frame, current pathValue, children []derivation.ChildWitness) (pathValue, bool) {
	if !session.Available() || !root.Available() || !binding.Available() || !frame.Available() || len(children) == 0 || len(children) != frame.ChildCount() || current.kind == algebra.KindApply || len(current.applications) != 0 || current.batches == nil {
		return pathValue{}, false
	}
	mergeRange, rangeOK := binding.Range()
	keyLayout := binding.Key()
	if !rangeOK || !mergeRange.Available() || mergeRange.Kind() != algebra.KindMerge || !keyLayout.Available() || !keyLayout.ValidFor(session.mounted.Fence()) {
		return pathValue{}, false
	}
	keyColumns := keyLayout.KeyColumns()
	if len(keyColumns) == 0 {
		return pathValue{}, false
	}
	pivot, pivotOK := session.mergePivot(root, children)
	if !pivotOK {
		return pathValue{}, false
	}
	if pivot != int(frame.Ordinal()) {
		// Multiple authored alternatives may observe the same physical delta.
		// The earliest changed child is the sealed occurrence pivot; later
		// occurrences are authenticated no-ops so the affected fold is issued
		// exactly once without output deduplication.
		return relationValue(current.node, algebra.KindMerge, []tuple.Batch{})
	}
	affected, affectedOK := mergeAffectedKeys(session.mounted, current.batches, keyColumns)
	if !affectedOK {
		return pathValue{}, false
	}
	alternatives := make([][]tuple.Batch, len(children))
	for _, representative := range affected {
		for childIndex, child := range children {
			layout, layoutOK := session.layout(child.Physical())
			if !layoutOK || !layout.Available() || !layout.ValidFor(session.mounted.Fence()) || !sameColumnIDs(layout.Columns(), child.Access().Columns()) {
				return pathValue{}, false
			}
			childAuthority, authorityOK := session.inputRange(root, layout.Access().Relation())
			if !authorityOK {
				return pathValue{}, false
			}
			batches, batchesOK := session.lookupMergeBatches(session.delta.Next(), layout, childAuthority, mergeRange, representative, keyColumns)
			if !batchesOK {
				return pathValue{}, false
			}
			alternatives[childIndex] = append(alternatives[childIndex], batches...)
		}
	}
	outputs, executeOK := mergeop.RecomputeAffected(binding, session.mounted, current.batches, alternatives)
	if !executeOK || outputs == nil {
		return pathValue{}, false
	}
	return relationValue(current.node, algebra.KindMerge, outputs)
}

func (session Session) mergePivot(root arrangement.Node, children []derivation.ChildWitness) (int, bool) {
	if !session.Available() || !root.Available() || len(children) == 0 {
		return -1, false
	}
	pivot := -1
	for childIndex, child := range children {
		if !child.Available() || child.Kind() == algebra.KindApply {
			continue
		}
		layout, layoutOK := session.layout(child.Physical())
		if !layoutOK {
			return -1, false
		}
		authority, authorityOK := session.inputRange(root, layout.Access().Relation())
		if !authorityOK {
			return -1, false
		}
		_, active, changedOK := session.changedBatches(layout, authority)
		if !changedOK {
			return -1, false
		}
		if active && pivot < 0 {
			pivot = childIndex
		}
	}
	return pivot, pivot >= 0
}

func mergeAffectedKeys(mounted witness.Mounted, batches []tuple.Batch, columns []model.ColumnID) ([]tuple.Tuple, bool) {
	if !mounted.Available() || batches == nil || len(columns) == 0 {
		return nil, false
	}
	result := make([]tuple.Tuple, 0)
	for _, batch := range batches {
		if !batch.ValidFor(mounted) {
			return nil, false
		}
		for index := 0; index < batch.Len(); index++ {
			value, ok := batch.At(index)
			if !ok || !value.ValidFor(mounted) {
				return nil, false
			}
			found := false
			for _, prior := range result {
				if prior.Scope().Same(value.Scope()) && tuple.SameKey(mounted, prior, value, columns) {
					found = true
					break
				}
			}
			if !found {
				result = append(result, value)
			}
		}
	}
	return result, true
}

func (session Session) lookupMergeBatches(root database.Version, layout arrangement.Layout, childAuthority, mergeRange arrangement.RangeBinding, representative tuple.Tuple, keyColumns []model.ColumnID) ([]tuple.Batch, bool) {
	// Merge children carry their complete unkeyed row vector. The parent
	// Merge range owns the lookup-only key layout; redeem that index first to
	// obtain owner-issued RowIDs, then replay each ID through the exact child
	// vector. Requiring the child vector itself to be keyed would reject the
	// mounted Input shape before any source row can be redeemed.
	if !session.Available() || !root.Available() || !layout.Available() || !childAuthority.Available() || !mergeRange.Available() || representative.ValidFor(session.mounted) == false || len(keyColumns) == 0 || layout.Access().Key().Available() || len(layout.Columns()) == 0 {
		return nil, false
	}
	lookupLayout := mergeRange.Layout()
	if !lookupLayout.Available() || !lookupLayout.ValidFor(session.mounted.Fence()) || lookupLayout.Access().Relation() != layout.Access().Relation() || !sameColumnIDs(lookupLayout.KeyColumns(), keyColumns) || len(lookupLayout.Columns()) != 0 {
		return nil, false
	}
	lookupReader, ok := read.Bind(root, lookupLayout, session.geometry, session.scratch)
	if !ok || !lookupReader.Available() {
		return nil, false
	}
	reader, ok := read.Bind(root, layout, session.geometry, session.scratch)
	if !ok || !reader.Available() {
		return nil, false
	}
	keys, ok := tupleKeyValues(representative, keyColumns)
	if !ok {
		return nil, false
	}
	query, ok := lookupReader.TupleFrom(keys)
	if !ok || !query.Available() {
		return nil, false
	}
	raw, ok := lowerRows(session.mounted, childAuthority, reader, func(visit func(read.Row) bool) (bool, bool) {
		completed, valid := lookupReader.Lookup(query, func(keyRow read.Row) bool {
			if keyRow == nil || !keyRow.Available() || keyRow.ID().Relation() != layout.Access().Relation() {
				return false
			}
			rowCompleted, rowValid := reader.LookupRowID(keyRow.ID(), visit)
			return rowCompleted && rowValid
		})
		return completed, valid
	})
	if !ok {
		return nil, false
	}
	result := make([]tuple.Batch, 0, len(raw))
	for _, batch := range raw {
		if batch.Len() == 0 {
			return nil, false
		}
		values := make([]tuple.Tuple, batch.Len())
		for index := range values {
			value, valueOK := batch.At(index)
			if !valueOK {
				return nil, false
			}
			values[index] = value
		}
		keyValues, keyOK := tupleKeyValues(values[0], keyColumns)
		if !keyOK {
			return nil, false
		}
		keyed, keyedOK := tuple.NewKeyRangeBatch(session.mounted, mergeRange, batch.Scope(), keyValues, values)
		if !keyedOK {
			return nil, false
		}
		result = append(result, keyed)
	}
	return result, true
}

func tupleKeyValues(value tuple.Tuple, columns []model.ColumnID) ([]bindingpkg.ValueToken, bool) {
	if !value.Available() || len(columns) == 0 {
		return nil, false
	}
	result := make([]bindingpkg.ValueToken, len(columns))
	for index, column := range columns {
		cell, ok := value.CellFor(column)
		if !ok || !cell.Value().Available() {
			return nil, false
		}
		result[index] = cell.Value()
	}
	return result, true
}

// lookupJoinBatches redeems only the sibling rows whose sealed join key equals
// one changed tuple. The physical layout must therefore be a keyed vector
// over exactly the sibling correspondence columns; an unkeyed correspondence
// vector has no public indexed redemption and is refused rather than scanned.
func (session Session) lookupJoinBatches(root database.Version, layout arrangement.Layout, authority arrangement.RangeBinding, current tuple.Tuple, currentColumns, siblingColumns []model.ColumnID) ([]tuple.Batch, bool) {
	if !session.Available() || !root.Available() || !layout.Available() || !authority.Available() || !layout.ValidFor(session.mounted.Fence()) || !current.ValidFor(session.mounted) || layout.KeyWidth() == 0 || !sameColumnIDs(layout.Columns(), siblingColumns) || !sameColumnIDs(layout.KeyColumns(), siblingColumns) || len(currentColumns) != len(siblingColumns) {
		return nil, false
	}
	reader, ok := read.Bind(root, layout, session.geometry, session.scratch)
	if !ok || !reader.Available() {
		return nil, false
	}
	keys := make([]bindingpkg.ValueToken, len(currentColumns))
	for index, column := range currentColumns {
		cell, cellOK := current.CellFor(column)
		if !cellOK || !cell.Column().Available() || !cell.Value().Available() {
			return nil, false
		}
		keys[index] = cell.Value()
	}
	query, ok := reader.TupleFrom(keys)
	if !ok || !query.Available() {
		return nil, false
	}
	return lowerRows(session.mounted, authority, reader, func(visit func(read.Row) bool) (bool, bool) {
		return reader.Lookup(query, visit)
	})
}

type scanRows func(func(read.Row) bool) (bool, bool)

func lowerRows(mounted witness.Mounted, authority arrangement.RangeBinding, reader read.Reader, scan scanRows) ([]tuple.Batch, bool) {
	if !mounted.Available() || !authority.Available() || !reader.Available() || scan == nil {
		return nil, false
	}
	type partition struct {
		scope  witness.Scope
		values []tuple.Tuple
	}
	partitions := make([]partition, 0)
	completed, valid := scan(func(row read.Row) bool {
		if row == nil || !row.Available() {
			return false
		}
		value, ok := tuple.Input(mounted, reader, row)
		if !ok || !value.ValidFor(mounted) || !value.Scope().Same(row.Scope()) {
			return false
		}
		found := -1
		for index := range partitions {
			if partitions[index].scope.Same(row.Scope()) {
				found = index
				break
			}
		}
		if found < 0 {
			partitions = append(partitions, partition{scope: row.Scope(), values: []tuple.Tuple{value}})
		} else {
			partitions[found].values = append(partitions[found].values, value)
		}
		return true
	})
	if !completed || !valid {
		return nil, false
	}
	result := make([]tuple.Batch, len(partitions))
	for index, partition := range partitions {
		batch, ok := tuple.NewRangeBatch(mounted, authority, partition.scope, partition.values, bindingpkg.DenominatorWitness{})
		if !ok {
			return nil, false
		}
		result[index] = batch
	}
	return result, true
}

// stableEpoch implements the canonical oriented-Join pivot. The derivation's
// frame orientation is the sealed occurrence-order witness: a changed left
// occurrence expands against the predecessor right side, while a changed
// right occurrence expands against the successor left side.
func (session Session) stableEpoch(frame derivation.Frame, siblingIndex int) (database.Version, bool) {
	if !session.Available() || !frame.Available() || siblingIndex < 0 || siblingIndex >= frame.SiblingCount() {
		return database.Version{}, false
	}
	sibling, ok := frame.SiblingAt(siblingIndex)
	if !ok || !sibling.Access().Available() {
		return database.Version{}, false
	}
	if frame.Kind() != algebra.KindJoin {
		return database.Version{}, false
	}
	switch frame.Orientation() {
	case derivation.OrientationLeft:
		return session.delta.Base(), true
	case derivation.OrientationRight:
		return session.delta.Next(), true
	default:
		return database.Version{}, false
	}
}

// stableChildEpoch is the sealed child-ordinal pivot for multi-child Apply.
// Children before the changed ordinal are read from the predecessor; children
// after it are read from the successor. The two expansions therefore have no
// shared all-old/all-new cross-term.
func stableChildEpoch(base, next database.Version, pivot, sibling int) (database.Version, bool) {
	if !base.Available() || !next.Available() || !next.SuccessorOf(base) || pivot < 0 || sibling < 0 || sibling == pivot {
		return database.Version{}, false
	}
	if sibling < pivot {
		return base, true
	}
	return next, true
}

func (session Session) layout(physical identity.ContentID) (arrangement.Layout, bool) {
	if !session.Available() || !physical.Available() {
		return arrangement.Layout{}, false
	}
	for _, layout := range session.mounted.Arrangement().Layouts() {
		if layout.Available() && layout.Digest() == physical && layout.ValidFor(session.mounted.Fence()) {
			return layout, true
		}
	}
	return arrangement.Layout{}, false
}

func (session Session) inputRange(root arrangement.Node, relation model.RelationID) (arrangement.RangeBinding, bool) {
	if !session.Available() || !root.Available() || !relation.Available() {
		return arrangement.RangeBinding{}, false
	}
	var found arrangement.RangeBinding
	var visit func(arrangement.Node) bool
	visit = func(node arrangement.Node) bool {
		if !node.Available() {
			return true
		}
		if node.Kind() == algebra.KindInput {
			binding, ok := node.Input()
			if ok && binding.Available() && binding.Relation() == relation {
				candidate, rangeOK := node.Range()
				if rangeOK && candidate.Available() {
					found = candidate
					return false
				}
			}
		}
		for _, child := range node.Children() {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	visit(root)
	return found, found.Available()
}
