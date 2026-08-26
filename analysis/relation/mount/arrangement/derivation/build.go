package derivation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type rowShape struct {
	columns []model.ColumnID
}

func (shape rowShape) available() bool {
	if shape.columns == nil {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(shape.columns))
	for _, column := range shape.columns {
		if !column.Available() {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

type builder struct {
	root           model.ExpressionID
	bindings       []Binding
	inputs         map[identity.ContentID]InputBinding
	signatures     map[signature.Identity]signature.Signature
	paths          []Path
	stack          map[identity.ContentID]bool
	shapeMemo      map[identity.ContentID]rowShape
	shapeStack     map[identity.ContentID]bool
	expandEvidence expand.Catalog
}

// Build derives all Input-occurrence zippers for one already-checked root.
// The input and operation vectors are sealed mount evidence; no resolver is
// retained and no logical expression is stored in the result.
func Build(root model.ExpressionID, expression algebra.Expression, bindings []Binding, inputBindings []InputBinding, signatures []signature.Signature) (Plan, bool) {
	return BuildWithExpand(root, expression, bindings, inputBindings, signatures, expand.EmptyCatalog())
}

// BuildWithExpand derives the same canonical zipper while accepting the
// mount-sealed evidence digest for each Expand node. Only the digest crosses
// this child-package boundary; no owner source or runtime token is retained.
func BuildWithExpand(root model.ExpressionID, expression algebra.Expression, bindings []Binding, inputBindings []InputBinding, signatures []signature.Signature, expandEvidence expand.Catalog) (Plan, bool) {
	if !root.Available() || expression == nil || !expression.Digest().Available() {
		return Plan{}, false
	}
	value := &builder{
		root: root, bindings: append([]Binding(nil), bindings...),
		inputs:     make(map[identity.ContentID]InputBinding, len(inputBindings)),
		signatures: make(map[signature.Identity]signature.Signature, len(signatures)),
		paths:      make([]Path, 0), stack: make(map[identity.ContentID]bool),
		shapeMemo: make(map[identity.ContentID]rowShape), shapeStack: make(map[identity.ContentID]bool), expandEvidence: expandEvidence,
	}
	for index, binding := range value.bindings {
		if !binding.Available() {
			return Plan{}, false
		}
		for _, prior := range value.bindings[:index] {
			if prior.access.equal(binding.access) {
				return Plan{}, false
			}
		}
	}
	for _, input := range inputBindings {
		if !input.Available() {
			return Plan{}, false
		}
		if _, duplicate := value.inputs[input.Input()]; duplicate {
			return Plan{}, false
		}
		value.inputs[input.Input()] = input
	}
	for _, operation := range signatures {
		if !operation.Available() || !operation.Identity().Available() {
			return Plan{}, false
		}
		if _, duplicate := value.signatures[operation.Identity()]; duplicate {
			return Plan{}, false
		}
		value.signatures[operation.Identity()] = operation
	}
	// The live publication grammar excludes Project and Group beneath a Publish
	// root. Standalone algebra roots still receive their own complete frame
	// derivative, but a production publication is refused as soon as either
	// unsupported operator appears anywhere below that root. There is no
	// fallback path that reopens a declaration to reinterpret such a tree.
	if isPublishRoot(expression) && containsUnsupportedProductionNode(expression) {
		return Plan{}, false
	}
	if !value.walk(expression, nil) {
		return Plan{}, false
	}
	data := &planData{
		root: root, paths: value.paths, byPath: make(map[uint32]int, len(value.paths)), triggers: make([]ExpandReaderTrigger, 0), byTrigger: make(map[identity.ContentID]int), sealed: false,
	}
	for index, path := range data.paths {
		if !path.Available() || path.root != root || path.occurrence != uint32(index) {
			return Plan{}, false
		}
		if _, duplicate := data.byPath[path.occurrence]; duplicate {
			return Plan{}, false
		}
		data.byPath[path.occurrence] = index
	}
	triggers, triggerOK := value.expandReaderTriggers(data.paths)
	if !triggerOK {
		return Plan{}, false
	}
	data.triggers = triggers
	for index, trigger := range triggers {
		if _, duplicate := data.byTrigger[trigger.node]; duplicate {
			return Plan{}, false
		}
		data.byTrigger[trigger.node] = index
	}
	parts := make([][]byte, 0, len(data.paths)+1)
	parts = append(parts, nominalBytes(root.Owner().Content(), root.Content()))
	for _, path := range data.paths {
		parts = append(parts, contentBytes(path.digest))
	}
	for _, trigger := range data.triggers {
		triggerDigest, triggerDigestOK := trigger.digest()
		if !triggerDigestOK {
			return Plan{}, false
		}
		parts = append(parts, contentBytes(triggerDigest))
	}
	digest, ok := identity.DeriveContentID(pathDigestDomain, parts...)
	if !ok {
		return Plan{}, false
	}
	data.digest = digest
	data.sealed = true
	result := Plan{data: data}
	return result, result.Available()
}

// expandReaderTriggers seals the complete fixed-epoch replay program for
// every Expand node.  It groups every Input watcher below one Expand-left
// child, identifies exactly one order-driving C anchor, and retains the
// relation-cofiber access beside each authored row-vector access.  The only
// admitted child shapes are the production C-major forms Expand(Input(C)) and
// Expand(Join(C, Select(X))).  Anything else is a mount refusal: runtime must
// not guess a candidate, scan a relation, or partially replay a compound
// child.
func (value *builder) expandReaderTriggers(paths []Path) ([]ExpandReaderTrigger, bool) {
	if value == nil || paths == nil {
		return nil, false
	}
	type triggerState struct {
		node      identity.ContentID
		contract  model.ExpandContract
		reader    sealedAccess
		stop      uint32
		watchers  []ExpandWatcher
		candidate int
		shape     uint8
		joinNode  identity.ContentID
	}
	states := make([]triggerState, 0)
	for _, path := range paths {
		if !path.Available() {
			return nil, false
		}
		expandFrames := make([]int, 0, 1)
		for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
			frame, ok := path.FrameAt(frameIndex)
			if !ok {
				return nil, false
			}
			if frame.Kind() == algebra.KindExpand {
				expandFrames = append(expandFrames, frameIndex)
			}
		}
		// A path crossing two Expand frames is nested dependent replay. The
		// fixed-epoch program has one boundary and refuses nested composition
		// until its own sealed pivot exists.
		if len(expandFrames) > 1 {
			return nil, false
		}
		if len(expandFrames) == 0 {
			continue
		}
		stop := expandFrames[0]
		frame, ok := path.FrameAt(stop)
		if !ok || frame.Kind() != algebra.KindExpand || !frame.ExpandContract().Available() {
			return nil, false
		}
		node := frame.Node()
		contract := frame.ExpandContract()
		if !node.Available() || !contract.Available() {
			return nil, false
		}
		shape, joinNode, shapeOK := expandWatcherShape(path, stop, contract)
		if !shapeOK {
			return nil, false
		}
		reader, readerOK := frame.SiblingAt(1)
		if !readerOK || !reader.Available() {
			return nil, false
		}
		rangeAccess, rangeOK := value.rangeAccess(path.LeafRelation())
		if !rangeOK {
			return nil, false
		}
		for frameIndex := stop + 1; frameIndex < path.FrameCount(); frameIndex++ {
			childFrame, childOK := path.FrameAt(frameIndex)
			if !childOK || !replayFrameAllowed(childFrame) {
				return nil, false
			}
		}
		stateIndex := -1
		for index := range states {
			if states[index].node == node {
				stateIndex = index
				break
			}
		}
		if stateIndex < 0 {
			states = append(states, triggerState{node: node, contract: contract, reader: reader.value, stop: uint32(stop), candidate: -1, shape: shape, joinNode: joinNode})
			stateIndex = len(states) - 1
		} else {
			state := states[stateIndex]
			if state.contract != contract || !state.reader.equal(reader.value) || state.stop != uint32(stop) || state.shape != shape || state.joinNode != joinNode {
				return nil, false
			}
		}
		watcher, watcherOK := newExpandWatcher(path.Occurrence(), uint32(stop), node, path.leaf, rangeAccess)
		if !watcherOK {
			return nil, false
		}
		states[stateIndex].watchers = append(states[stateIndex].watchers, watcher)
		if path.LeafRelation() == contract.Candidate() {
			if states[stateIndex].candidate >= 0 {
				return nil, false
			}
			states[stateIndex].candidate = len(states[stateIndex].watchers) - 1
		}
	}
	triggers := make([]ExpandReaderTrigger, 0, len(states))
	for _, state := range states {
		if !validExpandReplayShape(state.contract, state.watchers, state.candidate, state.shape) {
			return nil, false
		}
		anchorWatcher := state.watchers[state.candidate]
		anchor, anchorOK := newExpandAnchor(anchorWatcher.PathOccurrence(), anchorWatcher.leaf, anchorWatcher.range_)
		if !anchorOK || state.candidate != 0 {
			// The first mounted watcher is the order-driving C path. A later C
			// leaf would make emission ambiguous and reorder cofibers.
			return nil, false
		}
		replay, replayOK := newExpandReplay(anchor.PathOccurrence(), anchor, state.watchers)
		if !replayOK {
			return nil, false
		}
		trigger, triggerOK := newExpandReaderTrigger(state.node, anchor.PathOccurrence(), state.stop, state.reader, replay)
		if !triggerOK {
			return nil, false
		}
		triggers = append(triggers, trigger)
	}
	return triggers, true
}

func (value *builder) rangeAccess(relation model.RelationID) (sealedAccess, bool) {
	if value == nil || !relation.Available() {
		return sealedAccess{}, false
	}
	access, ok := NewAccess(relation, model.KeyID{}, nil)
	if !ok {
		return sealedAccess{}, false
	}
	return value.lookup(access)
}

func replayFrameAllowed(frame Frame) bool {
	if !frame.Available() {
		return false
	}
	switch frame.Kind() {
	case algebra.KindSelect, algebra.KindJoin, algebra.KindComplete, algebra.KindColumnProject:
		return true
	default:
		return false
	}
}

// expandWatcherShape accepts only the two bounded child forms for which a
// fixed-epoch replay is currently sealed. Shape 1 is the direct C Input;
// shape 2 is the production C-major Join(C, Select(X)).
func expandWatcherShape(path Path, stop int, contract model.ExpandContract) (uint8, identity.ContentID, bool) {
	if !path.Available() || !contract.Available() || stop < 0 || stop >= path.FrameCount() {
		return 0, identity.ContentID{}, false
	}
	if path.LeafRelation() == contract.Candidate() {
		if stop+1 == path.FrameCount() {
			return 1, identity.ContentID{}, true
		}
		frame, ok := path.FrameAt(stop + 1)
		if !ok || frame.Kind() != algebra.KindJoin || frame.Orientation() != OrientationLeft {
			return 0, identity.ContentID{}, false
		}
		return 2, frame.Node(), true
	}
	if stop+3 != path.FrameCount() {
		return 0, identity.ContentID{}, false
	}
	joinFrame, joinOK := path.FrameAt(stop + 1)
	selectFrame, selectOK := path.FrameAt(stop + 2)
	if !joinOK || !selectOK || joinFrame.Kind() != algebra.KindJoin || joinFrame.Orientation() != OrientationRight || selectFrame.Kind() != algebra.KindSelect {
		return 0, identity.ContentID{}, false
	}
	return 2, joinFrame.Node(), true
}

func validExpandReplayShape(contract model.ExpandContract, watchers []ExpandWatcher, candidate int, shape uint8) bool {
	if !contract.Available() || len(watchers) == 0 || candidate < 0 || candidate >= len(watchers) {
		return false
	}
	anchor := watchers[candidate]
	if !anchor.Available() || anchor.Leaf().Access().Relation() != contract.Candidate() {
		return false
	}
	if shape == 1 {
		return len(watchers) == 1 && candidate == 0
	}
	// The only compound production shape admitted by the replay ABI is
	// Expand(Join(C, Select(X))). C is the left/order-driving branch, and the
	// right branch has exactly one bounded Select frame.
	if shape != 2 || len(watchers) != 2 || candidate != 0 {
		return false
	}
	return true
}

func isPublishRoot(expression algebra.Expression) bool {
	switch value := expression.(type) {
	case algebra.Publish:
		return true
	case *algebra.Publish:
		return value != nil
	default:
		return false
	}
}

func containsUnsupportedProductionNode(expression algebra.Expression) bool {
	if expression == nil {
		return true
	}
	switch value := expression.(type) {
	case algebra.Project, *algebra.Project, algebra.Group, *algebra.Group:
		return true
	case algebra.Input:
		return false
	case *algebra.Input:
		return value == nil
	case algebra.Select:
		return containsUnsupportedProductionNode(value.Child())
	case *algebra.Select:
		return value == nil || containsUnsupportedProductionNode(value.Child())
	case algebra.Complete:
		return containsUnsupportedProductionNode(value.Child())
	case *algebra.Complete:
		return value == nil || containsUnsupportedProductionNode(value.Child())
	case algebra.Join:
		return containsUnsupportedProductionNode(value.Left()) || containsUnsupportedProductionNode(value.Right())
	case *algebra.Join:
		return value == nil || containsUnsupportedProductionNode(value.Left()) || containsUnsupportedProductionNode(value.Right())
	case algebra.Merge:
		for _, child := range value.Inputs() {
			if containsUnsupportedProductionNode(child) {
				return true
			}
		}
		return false
	case *algebra.Merge:
		if value == nil {
			return true
		}
		return containsUnsupportedProductionNode(algebra.Merge(*value))
	case algebra.ColumnProject:
		return containsUnsupportedProductionNode(value.Child())
	case *algebra.ColumnProject:
		return value == nil || containsUnsupportedProductionNode(value.Child())
	case algebra.Expand:
		return containsUnsupportedProductionNode(value.Child())
	case *algebra.Expand:
		return value == nil || containsUnsupportedProductionNode(value.Child())
	case algebra.Apply:
		for _, child := range value.Inputs() {
			if containsUnsupportedProductionNode(child) {
				return true
			}
		}
		return false
	case *algebra.Apply:
		if value == nil {
			return true
		}
		return containsUnsupportedProductionNode(algebra.Apply(*value))
	case algebra.Publish:
		return containsUnsupportedProductionNode(value.Child())
	case *algebra.Publish:
		return value == nil || containsUnsupportedProductionNode(value.Child())
	default:
		return true
	}
}

func (value *builder) walk(expression algebra.Expression, frames []Frame) bool {
	if value == nil || expression == nil || !expression.Digest().Available() {
		return false
	}
	digest := expression.Digest()
	if value.stack[digest] {
		return false
	}
	value.stack[digest] = true
	defer delete(value.stack, digest)
	addFrame := func(child algebra.Expression, frame Frame) bool {
		frame.node = digest
		if !frame.Available() {
			return false
		}
		next := append(append([]Frame(nil), frames...), frame)
		return value.walk(child, next)
	}
	appendInput := func(expression algebra.Input) bool {
		input, ok := value.inputs[expression.Digest()]
		if !ok || !input.Available() {
			return false
		}
		if input.Relation() != expression.Relation() {
			return false
		}
		pathFrames := make([]Frame, len(frames))
		copy(pathFrames, frames)
		leafAccess, leafOK := NewAccess(expression.Relation(), model.KeyID{}, input.Columns())
		if !leafOK {
			return false
		}
		leaf := sealedAccess{access: leafAccess, physical: input.Physical()}
		if !leaf.available() {
			return false
		}
		// Bind the authored occurrence after the walk has assigned its stable
		// ordinal. CompleteReplay is otherwise sealed before the leaf is known;
		// updating the immutable copy here keeps the replay tied to this exact
		// path rather than to a guessed row or frame position.
		for frameIndex := range pathFrames {
			if pathFrames[frameIndex].kind != algebra.KindComplete || !pathFrames[frameIndex].completeReplay.specified() {
				continue
			}
			replay, replayOK := pathFrames[frameIndex].completeReplay.withOccurrence(uint32(len(value.paths)))
			if !replayOK || replay.ParentNode() != pathFrames[frameIndex].node {
				return false
			}
			pathFrames[frameIndex].completeReplay = replay
		}
		path := Path{root: value.root, occurrence: uint32(len(value.paths)), node: expression.Digest(), leafRelation: expression.Relation(), readColumns: input.Columns(), leaf: leaf, frames: pathFrames}
		path.digest, ok = digestPath(path)
		if !ok || !path.Available() {
			return false
		}
		value.paths = append(value.paths, path)
		return true
	}
	switch node := expression.(type) {
	case algebra.Input:
		return appendInput(node)
	case *algebra.Input:
		return node != nil && appendInput(*node)
	case algebra.Select:
		contract := node.Contract()
		if contract.Mode() != algebra.SelectByScope || !contract.Scope().Available() {
			return false
		}
		return addFrame(node.Child(), Frame{kind: algebra.KindSelect, orientation: OrientationNone, siblings: []sealedAccess{}, scope: contract.Scope()})
	case *algebra.Select:
		return node != nil && value.walk(algebra.Select(*node), frames)
	case algebra.Complete:
		return value.walkComplete(node.Child(), node.Denominator(), digest, frames)
	case *algebra.Complete:
		return node != nil && value.walkComplete(node.Child(), node.Denominator(), node.Digest(), frames)
	case algebra.Join:
		return value.walkJoin(node.Left(), node.Right(), node.Contract(), digest, frames)
	case *algebra.Join:
		return node != nil && value.walkJoin(node.Left(), node.Right(), node.Contract(), node.Digest(), frames)
	case algebra.Merge:
		return value.walkMerge(node.Inputs(), node.Contract().Key(), digest, frames)
	case *algebra.Merge:
		return node != nil && value.walkMerge(node.Inputs(), node.Contract().Key(), node.Digest(), frames)
	case algebra.Group:
		return value.walkGroup(node.Child(), node.Contract(), digest, frames)
	case *algebra.Group:
		return node != nil && value.walkGroup(node.Child(), node.Contract(), node.Digest(), frames)
	case algebra.ColumnProject:
		return value.walkColumnProject(node.Child(), node.Contract().Slots(), digest, frames)
	case *algebra.ColumnProject:
		return node != nil && value.walkColumnProject(node.Child(), node.Contract().Slots(), node.Digest(), frames)
	case algebra.Expand:
		return value.walkExpand(node, digest, frames)
	case *algebra.Expand:
		return node != nil && value.walkExpand(*node, node.Digest(), frames)
	case algebra.Project:
		return value.walkProject(node.Child(), node.Contract(), digest, frames)
	case *algebra.Project:
		return node != nil && value.walkProject(node.Child(), node.Contract(), node.Digest(), frames)
	case algebra.Apply:
		return value.walkApply(node.Inputs(), node.Contract(), digest, frames)
	case *algebra.Apply:
		return node != nil && value.walkApply(node.Inputs(), node.Contract(), node.Digest(), frames)
	case algebra.Publish:
		return value.walkPublish(node.Child(), node.Contract(), digest, frames)
	case *algebra.Publish:
		return node != nil && value.walkPublish(node.Child(), node.Contract(), node.Digest(), frames)
	default:
		return false
	}
}

// inputForExpression resolves the exact Input occurrence that contributes a
// structural relation inside an expression. It walks the expression tree
// rather than using RelationID as a binding key, so repeated occurrences with
// different projections remain distinct. Ambiguous duplicate matches refuse
// the derivation instead of selecting one by traversal order.
func (value *builder) inputForExpression(expression algebra.Expression, relation model.RelationID) (InputBinding, bool) {
	if value == nil || expression == nil || !relation.Available() {
		return InputBinding{}, false
	}
	var result InputBinding
	found := false
	visit := func(child algebra.Expression) bool {
		candidate, candidateOK := value.inputForExpression(child, relation)
		if !candidateOK {
			return true
		}
		if found {
			return candidate.Input() == result.Input()
		}
		result, found = candidate, true
		return true
	}
	switch node := expression.(type) {
	case algebra.Input:
		if node.Relation() != relation {
			return InputBinding{}, false
		}
		result, found = value.inputs[node.Digest()]
	case *algebra.Input:
		if node == nil {
			return InputBinding{}, false
		}
		return value.inputForExpression(algebra.Input(*node), relation)
	case algebra.Select:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.Select:
		if node != nil {
			return value.inputForExpression(algebra.Select(*node), relation)
		}
	case algebra.Complete:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.Complete:
		if node != nil {
			return value.inputForExpression(algebra.Complete(*node), relation)
		}
	case algebra.Group:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.Group:
		if node != nil {
			return value.inputForExpression(algebra.Group(*node), relation)
		}
	case algebra.ColumnProject:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.ColumnProject:
		if node != nil {
			return value.inputForExpression(algebra.ColumnProject(*node), relation)
		}
	case algebra.Project:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.Project:
		if node != nil {
			return value.inputForExpression(algebra.Project(*node), relation)
		}
	case algebra.Expand:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.Expand:
		if node != nil {
			return value.inputForExpression(algebra.Expand(*node), relation)
		}
	case algebra.Publish:
		return value.inputForExpression(node.Child(), relation)
	case *algebra.Publish:
		if node != nil {
			return value.inputForExpression(algebra.Publish(*node), relation)
		}
	case algebra.Join:
		if !visit(node.Left()) || !visit(node.Right()) {
			return InputBinding{}, false
		}
	case *algebra.Join:
		if node != nil {
			return value.inputForExpression(algebra.Join(*node), relation)
		}
	case algebra.Merge:
		for _, child := range node.Inputs() {
			if !visit(child) {
				return InputBinding{}, false
			}
		}
	case *algebra.Merge:
		if node != nil {
			return value.inputForExpression(algebra.Merge(*node), relation)
		}
	case algebra.Apply:
		for _, child := range node.Inputs() {
			if !visit(child) {
				return InputBinding{}, false
			}
		}
	case *algebra.Apply:
		if node != nil {
			return value.inputForExpression(algebra.Apply(*node), relation)
		}
	}
	return result, found && result.Available()
}

// rowInputForRelation resolves a structural row required by an operator whose
// contract names a relation but does not carry an Input expression (for
// example an Expand reader or a Project destination). It scans sealed logical
// Access bindings, chooses a unique maximal unkeyed vector, and reifies that
// vector as an exact Input identity. There is no relation-keyed binding table;
// an ambiguous set of projections is refused rather than aliased.
func (value *builder) rowInputForRelation(relation model.RelationID) (InputBinding, bool) {
	if value == nil || !relation.Available() {
		return InputBinding{}, false
	}
	var result Binding
	bestWidth := -1
	ambiguous := false
	for _, binding := range value.bindings {
		if !binding.Available() || binding.access.relation != relation || binding.access.key.Available() || len(binding.access.columns) == 0 {
			continue
		}
		width := len(binding.access.columns)
		if width < bestWidth {
			continue
		}
		if width > bestWidth {
			// A narrower vector cannot make the final structural row
			// ambiguous. Reset the candidate when a wider sealed vector is
			// encountered; only peers at the eventual maximum are compared.
			result, bestWidth = binding, width
			ambiguous = false
			continue
		}
		if !binding.access.equal(result.access) {
			ambiguous = true
		}
	}
	if bestWidth <= 0 || ambiguous {
		return InputBinding{}, false
	}
	input, inputOK := algebra.NewInputColumns(relation, result.access.columns)
	if !inputOK {
		return InputBinding{}, false
	}
	return NewInputBinding(input.Digest(), relation, result.access.columns, result.physical)
}

func (value *builder) walkExpand(node algebra.Expand, parent identity.ContentID, frames []Frame) bool {
	if value == nil || !node.Contract().Available() {
		return false
	}
	evidence, ok := value.expandEvidence.At(node.Digest())
	if !ok || !evidence.Available() || evidence.Contract() != node.Contract() || !parent.Available() {
		return false
	}
	contract := node.Contract()
	// The sibling vector is fixed at mount and redeemed by exact logical
	// Access. It is intentionally ordered C, R, R-key; P is represented by the
	// owner-frozen correspondence in Evidence and has no runtime row layout.
	candidateInput, candidateOK := value.inputForExpression(node.Child(), contract.Candidate())
	readerInput, readerOK := value.rowInputForRelation(contract.Reader())
	candidateColumns, readerColumns := candidateInput.Columns(), readerInput.Columns()
	if !candidateOK || !readerOK || len(candidateColumns) == 0 || len(readerColumns) == 0 {
		return false
	}
	candidateAccess, candidateAccessOK := NewAccess(contract.Candidate(), model.KeyID{}, candidateColumns)
	readerAccess, readerAccessOK := NewAccess(contract.Reader(), model.KeyID{}, readerColumns)
	keyAccess, keyAccessOK := value.lookupKeyColumn(contract.Key())
	if !candidateAccessOK || !readerAccessOK || !keyAccessOK {
		return false
	}
	candidate, candidateOK := value.lookup(candidateAccess)
	reader, readerOK := value.lookup(readerAccess)
	if !candidateOK || !readerOK {
		return false
	}
	frame := Frame{
		kind: algebra.KindExpand, orientation: OrientationNone, node: parent,
		siblings:       []sealedAccess{candidate, reader, keyAccess},
		expandContract: contract, expandEvidence: evidence.Digest(),
	}
	if !frame.Available() {
		return false
	}
	return value.walk(node.Child(), append(append([]Frame(nil), frames...), frame))
}

func (value *builder) walkComplete(child algebra.Expression, denominator model.DenominatorRef, parent identity.ContentID, frames []Frame) bool {
	if child == nil || !denominator.Available() {
		return false
	}
	access, ok := NewAccess(denominator.Relation(), denominator.Key(), nil)
	if !ok {
		return false
	}
	sibling, ok := value.lookup(access)
	if !ok {
		return false
	}
	// CompleteReplay is optional at the ordinary derivation boundary.  It is
	// sealed only for the exact Complete(Select(Input)) shape; unsupported
	// children remain valid ordinary Complete plans but the delta evaluator
	// must refuse them rather than recover an extent by scanning.
	replay, _ := value.completeReplay(child, denominator, parent, sibling)
	return value.walk(child, append(append([]Frame(nil), frames...), Frame{kind: algebra.KindComplete, orientation: OrientationNone, node: parent, siblings: []sealedAccess{sibling}, denominator: denominator, completeReplay: replay}))
}

// completeReplay recognizes the one closed replay shape currently supported
// by the differential evaluator.  It consumes only the mount-selected Input
// row vector and relation cofiber; no expression callback or runtime lookup is
// retained.  A missing scan/vector binding is an ordinary "not replayable"
// result, not a reason to weaken the Complete derivation itself.
func (value *builder) completeReplay(child algebra.Expression, denominator model.DenominatorRef, parent identity.ContentID, order sealedAccess) (CompleteReplay, bool) {
	if value == nil || child == nil || !denominator.Available() {
		return CompleteReplay{}, false
	}
	var selected algebra.Select
	switch node := child.(type) {
	case algebra.Select:
		selected = node
	case *algebra.Select:
		if node == nil {
			return CompleteReplay{}, false
		}
		selected = *node
	default:
		return CompleteReplay{}, false
	}
	if selected.Contract().Mode() != algebra.SelectByScope || !selected.Contract().Scope().Available() {
		return CompleteReplay{}, false
	}
	var input algebra.Input
	switch node := selected.Child().(type) {
	case algebra.Input:
		input = node
	case *algebra.Input:
		if node == nil {
			return CompleteReplay{}, false
		}
		input = *node
	default:
		return CompleteReplay{}, false
	}
	if input.Relation() != denominator.Relation() {
		return CompleteReplay{}, false
	}
	inputBinding, inputOK := value.inputs[input.Digest()]
	if !inputOK || !inputBinding.Available() {
		return CompleteReplay{}, false
	}
	valuesAccess, valuesOK := NewAccess(input.Relation(), model.KeyID{}, inputBinding.Columns())
	rangeAccess, rangeOK := NewAccess(input.Relation(), model.KeyID{}, nil)
	if !valuesOK || !rangeOK {
		return CompleteReplay{}, false
	}
	values, valuesFound := value.lookup(valuesAccess)
	range_, rangeFound := value.lookup(rangeAccess)
	if !valuesFound || !rangeFound {
		return CompleteReplay{}, false
	}
	return newCompleteReplay(parent, 0, input.Digest(), selected.Digest(), values, range_, order, denominator, selected.Contract().Scope())
}

func (value *builder) walkJoin(left, right algebra.Expression, contract algebra.JoinContract, parent identity.ContentID, frames []Frame) bool {
	leftColumns, rightColumns := contract.LeftColumns(), contract.RightColumns()
	if left == nil || right == nil || len(leftColumns) == 0 || len(leftColumns) != len(rightColumns) {
		return false
	}
	leftAccess, leftOK := NewAccess(leftColumns[0].Relation(), model.KeyID{}, leftColumns)
	rightAccess, rightOK := NewAccess(rightColumns[0].Relation(), model.KeyID{}, rightColumns)
	if !leftOK || !rightOK {
		return false
	}
	leftSibling, leftSiblingOK := value.lookup(rightAccess)
	rightSibling, rightSiblingOK := value.lookup(leftAccess)
	if !leftSiblingOK || !rightSiblingOK {
		return false
	}
	leftFrame := Frame{kind: algebra.KindJoin, orientation: OrientationLeft, node: parent, siblings: []sealedAccess{leftSibling}, columns: append([]model.ColumnID(nil), leftColumns...)}
	rightFrame := Frame{kind: algebra.KindJoin, orientation: OrientationRight, node: parent, siblings: []sealedAccess{rightSibling}, columns: append([]model.ColumnID(nil), rightColumns...)}
	return value.walk(left, append(append([]Frame(nil), frames...), leftFrame)) && value.walk(right, append(append([]Frame(nil), frames...), rightFrame))
}

func (value *builder) walkMerge(children []algebra.Expression, key model.KeyID, parent identity.ContentID, frames []Frame) bool {
	if len(children) == 0 || !key.Available() {
		return false
	}
	shapes := make([]rowShape, len(children))
	for index, child := range children {
		if child == nil {
			return false
		}
		shape, ok := value.shape(child)
		if !ok || !shape.available() || len(shape.columns) == 0 {
			return false
		}
		shapes[index] = shape
		if index != 0 && !sameColumns(shapes[0].columns, shape.columns) {
			return false
		}
	}
	// Resolve every authored child vector once, including the active child.
	// The ordinary sibling list below intentionally omits the active child,
	// but differential Merge redemption needs a complete sealed alternative
	// set and may not reopen the expression or scan a relation to recover it.
	childAccesses := make([]ChildWitness, len(shapes))
	for childIndex, shape := range shapes {
		access, accessOK := value.rowAccess(shape)
		if !accessOK {
			return false
		}
		sealed, sealedOK := value.lookup(access)
		if !sealedOK {
			return false
		}
		child, witnessOK := newChildWitness(uint32(childIndex), sealed, children[childIndex].Digest(), children[childIndex].Kind())
		if !witnessOK {
			return false
		}
		childAccesses[childIndex] = child
	}
	for index, child := range children {
		siblings := make([]sealedAccess, 0, len(children)-1)
		for siblingIndex := range shapes {
			if siblingIndex == index {
				continue
			}
			siblings = append(siblings, childAccesses[siblingIndex].value)
		}
		frame := Frame{kind: algebra.KindMerge, orientation: OrientationNone, node: parent, ordinal: uint32(index), siblings: siblings, children: append([]ChildWitness(nil), childAccesses...), key: key, columns: append([]model.ColumnID(nil), shapes[index].columns...)}
		if !value.walk(child, append(append([]Frame(nil), frames...), frame)) {
			return false
		}
	}
	return true
}

func (value *builder) walkGroup(child algebra.Expression, contract algebra.GroupContract, parent identity.ContentID, frames []Frame) bool {
	key := contract.Key()
	if child == nil || !key.Available() || !contract.Cardinality().Available() {
		return false
	}
	access, ok := NewAccess(key.Relation(), key, nil)
	if !ok {
		return false
	}
	sibling, ok := value.lookup(access)
	if !ok {
		return false
	}
	frame := Frame{kind: algebra.KindGroup, orientation: OrientationNone, node: parent, siblings: []sealedAccess{sibling}, key: key, cardinality: contract.Cardinality()}
	return value.walk(child, append(append([]Frame(nil), frames...), frame))
}

func (value *builder) walkColumnProject(child algebra.Expression, slots []algebra.ColumnSlot, parent identity.ContentID, frames []Frame) bool {
	if child == nil || len(slots) == 0 {
		return false
	}
	shape, ok := value.shape(child)
	if !ok || !shape.available() {
		return false
	}
	columns := make([]model.ColumnID, len(slots))
	seen := make(map[model.ColumnID]struct{}, len(slots))
	for index, slot := range slots {
		if int(slot.Cell()) >= len(shape.columns) || slot.Column() != shape.columns[slot.Cell()] {
			return false
		}
		if _, duplicate := seen[slot.Column()]; duplicate {
			return false
		}
		seen[slot.Column()] = struct{}{}
		columns[index] = slot.Column()
	}
	access, ok := NewAccess(columns[0].Relation(), model.KeyID{}, columns)
	if !ok {
		return false
	}
	sibling, ok := value.lookup(access)
	if !ok {
		return false
	}
	frame := Frame{kind: algebra.KindColumnProject, orientation: OrientationNone, node: parent, siblings: []sealedAccess{sibling}, columns: columns, slots: append([]algebra.ColumnSlot(nil), slots...)}
	return value.walk(child, append(append([]Frame(nil), frames...), frame))
}

func (value *builder) walkProject(child algebra.Expression, contract algebra.ProjectContract, parent identity.ContentID, frames []Frame) bool {
	if child == nil || !contract.Target().Available() || !contract.Key().Available() {
		return false
	}
	target, targetOK := value.rowInputForRelation(contract.Target())
	if !targetOK || !target.Available() {
		return false
	}
	targetAccess, ok := NewAccess(contract.Target(), model.KeyID{}, target.Columns())
	if !ok {
		return false
	}
	targetSibling, ok := value.lookup(targetAccess)
	if !ok {
		return false
	}
	keyAccess, ok := NewAccess(contract.Target(), contract.Key(), nil)
	if !ok {
		return false
	}
	keySibling, ok := value.lookup(keyAccess)
	if !ok {
		return false
	}
	siblings := []sealedAccess{targetSibling, keySibling}
	targetColumns := target.Columns()
	targetMembers := make(map[model.ColumnID]struct{}, len(targetColumns))
	for _, column := range targetColumns {
		targetMembers[column] = struct{}{}
	}
	seenTargets := make(map[model.ColumnID]struct{})
	groups := make([]rowShape, 0)
	for _, mapping := range contract.Mappings() {
		source := mapping.Source()
		targetColumn := mapping.Target()
		sourceInput, sourceFound := value.inputForExpression(child, source.Relation())
		if !source.Available() || !targetColumn.Available() || targetColumn.Relation() != contract.Target() || !sourceFound || !sourceInput.Available() {
			return false
		}
		sourceMember := false
		for _, column := range sourceInput.Columns() {
			if column == source {
				sourceMember = true
				break
			}
		}
		if !sourceMember {
			return false
		}
		if _, targetMember := targetMembers[targetColumn]; !targetMember {
			return false
		}
		if _, duplicate := seenTargets[targetColumn]; duplicate {
			return false
		}
		seenTargets[targetColumn] = struct{}{}
		groupIndex := -1
		for index := range groups {
			if len(groups[index].columns) != 0 && groups[index].columns[0].Relation() == source.Relation() {
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			groups = append(groups, rowShape{columns: []model.ColumnID{source}})
			groupIndex = len(groups) - 1
		} else {
			groups[groupIndex].columns = append(groups[groupIndex].columns, source)
		}
	}
	if len(seenTargets) == 0 {
		return false
	}
	for _, group := range groups {
		access, accessOK := NewAccess(group.columns[0].Relation(), model.KeyID{}, group.columns)
		if !accessOK {
			return false
		}
		sealed, sealedOK := value.lookup(access)
		if !sealedOK {
			return false
		}
		siblings = append(siblings, sealed)
	}
	mapTargets := make([]model.ColumnID, 0, len(seenTargets))
	for _, mapping := range contract.Mappings() {
		mapTargets = append(mapTargets, mapping.Target())
	}
	frame := Frame{kind: algebra.KindProject, orientation: OrientationNone, node: parent, siblings: siblings, destination: contract.Target(), key: contract.Key(), columns: target.Columns(), mapTargets: mapTargets}
	return value.walk(child, append(append([]Frame(nil), frames...), frame))
}

func (value *builder) walkApply(children []algebra.Expression, contract algebra.ApplyContract, parent identity.ContentID, frames []Frame) bool {
	operation := contract.Operation()
	signatureValue, ok := value.signatures[operation]
	if !ok || !signatureValue.Available() {
		return false
	}
	sources := contract.SlotSource()
	if len(sources) != signatureValue.InputLen() {
		return false
	}
	if len(children) == 0 {
		if signatureValue.InputLen() != 0 {
			return false
		}
		return true
	}
	if signatureValue.InputLen() == 0 {
		return false
	}
	childShapes := make([]rowShape, len(children))
	for index, child := range children {
		if child == nil {
			return false
		}
		shape, shapeOK := value.shape(child)
		if !shapeOK || !shape.available() {
			return false
		}
		childShapes[index] = shape
	}
	for _, source := range sources {
		if int(source.Child()) >= len(children) || int(source.Cell()) >= len(childShapes[source.Child()].columns) {
			return false
		}
	}
	siblings, siblingsOK := value.deliveryAccesses(signatureValue)
	if !siblingsOK {
		return false
	}
	for index, child := range children {
		frame := Frame{kind: algebra.KindApply, orientation: OrientationNone, node: parent, ordinal: uint32(index), siblings: siblings, operation: operation, sources: append([]algebra.SlotSource(nil), sources...)}
		if !value.walk(child, append(append([]Frame(nil), frames...), frame)) {
			return false
		}
	}
	return true
}

func (value *builder) walkPublish(child algebra.Expression, contract algebra.PublishContract, parent identity.ContentID, frames []Frame) bool {
	if child == nil || !contract.Destination().Available() || !contract.Key().Available() {
		return false
	}
	columns := contract.Columns()
	if len(columns) == 0 {
		input, ok := value.rowInputForRelation(contract.Destination())
		if !ok || !input.Available() {
			return false
		}
		columns = input.Columns()
	}
	if len(columns) == 0 {
		return false
	}
	destination, ok := NewAccess(contract.Destination(), model.KeyID{}, nil)
	if !ok {
		return false
	}
	destinationSibling, ok := value.lookup(destination)
	if !ok {
		return false
	}
	key, ok := NewAccess(contract.Destination(), contract.Key(), nil)
	if !ok {
		return false
	}
	keySibling, ok := value.lookup(key)
	if !ok {
		return false
	}
	vector, ok := NewAccess(columns[0].Relation(), model.KeyID{}, columns)
	if !ok || vector.relation != contract.Destination() {
		return false
	}
	vectorSibling, ok := value.lookup(vector)
	if !ok {
		return false
	}
	frame := Frame{kind: algebra.KindPublish, orientation: OrientationNone, node: parent, siblings: []sealedAccess{destinationSibling, keySibling, vectorSibling}, destination: contract.Destination(), key: contract.Key(), columns: append([]model.ColumnID(nil), columns...)}
	return value.walk(child, append(append([]Frame(nil), frames...), frame))
}

func (value *builder) deliveryAccesses(operation signature.Signature) ([]sealedAccess, bool) {
	result := make([]sealedAccess, 0, operation.InputLen()*2)
	for _, input := range operation.Inputs() {
		if !input.Available() {
			return nil, false
		}
		source, sourceOK := input.SourceDenominator()
		if !sourceOK || !source.Available() {
			return nil, false
		}
		access, ok := NewAccess(input.Relation, source.Key(), []model.ColumnID{input.Column})
		if !ok {
			return nil, false
		}
		sealed, ok := value.lookup(access)
		if !ok {
			return nil, false
		}
		result = append(result, sealed)
		if input.Delivery.IsSpan() {
			order, orderOK := NewAccess(input.Denominator.Relation(), input.Delivery.OrderKey(), nil)
			if !orderOK {
				return nil, false
			}
			orderSealed, orderOK := value.lookup(order)
			if !orderOK {
				return nil, false
			}
			result = append(result, orderSealed)
		}
	}
	return result, true
}

func (value *builder) lookup(wanted Access) (sealedAccess, bool) {
	if !wanted.Available() {
		return sealedAccess{}, false
	}
	for _, binding := range value.bindings {
		if binding.access.equal(wanted) {
			result := sealedAccess{access: binding.access, physical: binding.physical}
			return result, result.available()
		}
	}
	return sealedAccess{}, false
}

// lookupKeyColumn redeems the unique declared key access containing one Expand
// reader column. Key identity is selected during mount; a missing or
// ambiguous inverse is a hard derivation refusal, never a guessed ordinal.
func (value *builder) lookupKeyColumn(column model.ColumnID) (sealedAccess, bool) {
	if value == nil || !column.Available() {
		return sealedAccess{}, false
	}
	var result sealedAccess
	found := false
	for _, binding := range value.bindings {
		if !binding.Available() || !binding.access.key.Available() || binding.access.relation != column.Relation() || len(binding.access.columns) != 0 {
			continue
		}
		// The key's declared schema is not retained in derivation; the mount
		// binding's key identity is the exact inverse coordinate selected by
		// arrangement. Its relation/key match is sufficient here because the
		// checker has already proved the singleton column shape.
		if binding.access.key.Relation() != column.Relation() {
			continue
		}
		if found {
			return sealedAccess{}, false
		}
		result = sealedAccess{access: binding.access, physical: binding.physical}
		found = true
	}
	return result, found && result.available()
}

func (value *builder) rowAccess(shape rowShape) (Access, bool) {
	if !shape.available() || len(shape.columns) == 0 {
		return Access{}, false
	}
	return NewAccess(shape.columns[0].Relation(), model.KeyID{}, shape.columns)
}

func (value *builder) shape(expression algebra.Expression) (rowShape, bool) {
	if value == nil || expression == nil || !expression.Digest().Available() {
		return rowShape{}, false
	}
	digest := expression.Digest()
	if result, ok := value.shapeMemo[digest]; ok {
		return result, result.available()
	}
	if value.shapeStack[digest] {
		return rowShape{}, false
	}
	value.shapeStack[digest] = true
	defer delete(value.shapeStack, digest)
	var result rowShape
	var ok bool
	switch node := expression.(type) {
	case algebra.Input:
		input, found := value.inputs[node.Digest()]
		if found {
			result = rowShape{columns: input.Columns()}
			ok = result.available()
		}
	case *algebra.Input:
		if node != nil {
			return value.shape(algebra.Input(*node))
		}
	case algebra.Select:
		result, ok = value.shape(node.Child())
	case *algebra.Select:
		if node != nil {
			result, ok = value.shape(algebra.Select(*node))
		}
	case algebra.Complete:
		result, ok = value.shape(node.Child())
	case *algebra.Complete:
		if node != nil {
			result, ok = value.shape(algebra.Complete(*node))
		}
	case algebra.Join:
		left, leftOK := value.shape(node.Left())
		right, rightOK := value.shape(node.Right())
		if leftOK && rightOK {
			result = rowShape{columns: append(append([]model.ColumnID(nil), left.columns...), right.columns...)}
			ok = result.available()
		}
	case *algebra.Join:
		if node != nil {
			result, ok = value.shape(algebra.Join(*node))
		}
	case algebra.Merge:
		children := node.Inputs()
		if len(children) != 0 {
			result, ok = value.shape(children[0])
			for _, child := range children[1:] {
				other, otherOK := value.shape(child)
				if !otherOK || !ok || !sameColumns(result.columns, other.columns) {
					ok = false
					break
				}
			}
		}
	case *algebra.Merge:
		if node != nil {
			result, ok = value.shape(algebra.Merge(*node))
		}
	case algebra.Group:
		result, ok = value.shape(node.Child())
	case *algebra.Group:
		if node != nil {
			result, ok = value.shape(algebra.Group(*node))
		}
	case algebra.ColumnProject:
		child, childOK := value.shape(node.Child())
		if childOK {
			slots := node.Contract().Slots()
			columns := make([]model.ColumnID, len(slots))
			for index, slot := range slots {
				if int(slot.Cell()) >= len(child.columns) || child.columns[slot.Cell()] != slot.Column() {
					childOK = false
					break
				}
				columns[index] = slot.Column()
			}
			result, ok = rowShape{columns: columns}, childOK
		}
	case *algebra.ColumnProject:
		if node != nil {
			result, ok = value.shape(algebra.ColumnProject(*node))
		}
	case algebra.Expand:
		child, childOK := value.shape(node.Child())
		reader, readerOK := value.rowInputForRelation(node.Contract().Reader())
		if childOK && readerOK && reader.Available() {
			result = rowShape{columns: append(append([]model.ColumnID(nil), child.columns...), reader.Columns()...)}
			ok = result.available()
		}
	case *algebra.Expand:
		if node != nil {
			result, ok = value.shape(algebra.Expand(*node))
		}
	case algebra.Project:
		input, found := value.rowInputForRelation(node.Contract().Target())
		if found {
			result, ok = rowShape{columns: input.Columns()}, input.Available()
		}
	case *algebra.Project:
		if node != nil {
			result, ok = value.shape(algebra.Project(*node))
		}
	case algebra.Apply:
		operation, found := value.signatures[node.Contract().Operation()]
		if found {
			result, ok = operationShape(operation)
		}
	case *algebra.Apply:
		if node != nil {
			result, ok = value.shape(algebra.Apply(*node))
		}
	case algebra.Publish:
		columns := node.Contract().Columns()
		if len(columns) == 0 {
			if input, found := value.rowInputForRelation(node.Contract().Destination()); found {
				columns = input.Columns()
			}
		}
		if len(columns) != 0 {
			result, ok = rowShape{columns: columns}, validColumns(columns, node.Contract().Destination())
		}
	case *algebra.Publish:
		if node != nil {
			result, ok = value.shape(algebra.Publish(*node))
		}
	}
	if ok {
		value.shapeMemo[digest] = result
	}
	return result, ok && result.available()
}

func operationShape(operation signature.Signature) (rowShape, bool) {
	outputs := operation.Outputs()
	if !operation.Available() || len(outputs) == 0 {
		return rowShape{}, false
	}
	columns := make([]model.ColumnID, len(outputs))
	for index, output := range outputs {
		if !output.Available() || !output.Relation.Available() || !output.Column.Available() {
			return rowShape{}, false
		}
		if index != 0 && output.Relation != outputs[0].Relation {
			return rowShape{}, false
		}
		columns[index] = output.Column
	}
	result := rowShape{columns: columns}
	return result, result.available()
}

func validColumns(columns []model.ColumnID, relation model.RelationID) bool {
	for _, column := range columns {
		if !column.Available() || column.Relation() != relation {
			return false
		}
	}
	return len(columns) != 0
}

func sameColumns(left, right []model.ColumnID) bool {
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
