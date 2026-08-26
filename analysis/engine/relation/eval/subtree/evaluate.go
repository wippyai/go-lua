package subtree

import (
	"encoding/binary"

	completeop "github.com/wippyai/go-lua/analysis/engine/relation/operator/complete"
	inputop "github.com/wippyai/go-lua/analysis/engine/relation/operator/input"
	joinop "github.com/wippyai/go-lua/analysis/engine/relation/operator/join"
	selectop "github.com/wippyai/go-lua/analysis/engine/relation/operator/select"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Evaluate redeems one exact sealed correlated subtree for one owner-issued
// population denominator member and its authenticated cofiber scope. The
// replay binds the population authority and the child ordinal together, so a
// shared child cannot accept an arbitrary RowID or scope supplied from a
// different Apply invocation. The subtree root and every Input/Complete
// occurrence are selected by the mount's physical Node plus root-relative
// path. A relation name, logical digest, or runtime shape can never select a
// sibling occurrence.
func (session Session) Evaluate(replay arrangement.ApplyReplay, subtree arrangement.CorrelatedSubtree, row model.RowID, scope witness.Scope) (Result, bool) {
	population, ok := session.ForPopulation(replay, row, scope)
	if !ok {
		return Result{}, false
	}
	return population.Evaluate(subtree)
}

type occurrenceKey struct {
	node arrangement.Node
	path string
}

type evaluationState struct {
	session           Session
	subtree           arrangement.CorrelatedSubtree
	populationRef     model.DenominatorRef
	populationWitness binding.DenominatorWitness
	population        model.RowID
	populationScope   witness.Scope
	populationReader  read.Reader
	usedInputs        map[occurrenceKey]bool
	usedCompletes     map[occurrenceKey]bool
}

type nodeValue struct {
	batches []tuple.Batch
}

func (value nodeValue) available() bool {
	return batchesAvailable(value.batches)
}

func (session Session) evaluate(subtree arrangement.CorrelatedSubtree, populationRef model.DenominatorRef, populationWitness binding.DenominatorWitness, population model.RowID, populationScope witness.Scope, populationReader read.Reader) (Result, bool) {
	if !session.Available() || !subtree.Available() || !populationRef.Available() || !populationWitness.Available() || !populationWitness.ValidFor(session.mounted.RuntimeFence()) || !populationWitness.Matches(populationRef) || !populationWitness.Contains(population) || !population.Available() || !populationScope.ValidFor(session.mounted.RuntimeFence()) || !populationReader.Available() {
		return Result{}, false
	}
	if _, scopeOK := session.mounted.ScopeToken(populationScope); !scopeOK {
		return Result{}, false
	}
	if !populationReader.Layout().Available() || population.Relation() != populationRef.Relation() {
		return Result{}, false
	}
	root := subtree.Root()
	if !root.Available() || !session.ownsNode(root) {
		return Result{}, false
	}
	state := evaluationState{
		session: session, subtree: subtree, populationRef: populationRef, populationWitness: populationWitness, population: population, populationScope: populationScope, populationReader: populationReader,
		usedInputs: make(map[occurrenceKey]bool), usedCompletes: make(map[occurrenceKey]bool),
	}
	value, ok := state.executeNode(root, []uint32{})
	if !ok || !value.available() || !state.allExtentsUsed() {
		return Result{}, false
	}
	batches := make([]tuple.Batch, len(value.batches))
	copy(batches, value.batches)
	result := Result{subtree: subtree, root: root, populationRef: populationRef, population: population, populationScope: populationScope, populationFence: session.mounted.RuntimeFence(), batches: batches, sealed: true}
	return result, result.Available()
}

func (session Session) ownsNode(node arrangement.Node) bool {
	if !session.Available() || !node.Available() {
		return false
	}
	execution := session.root.Arrangement().Execution()
	if !execution.Available() {
		return false
	}
	canonical, ok := execution.Node(node.Digest())
	return ok && canonical.Available() && canonical == node
}

func (state *evaluationState) executeNode(node arrangement.Node, path []uint32) (nodeValue, bool) {
	if state == nil || !state.session.Available() || !state.subtree.Available() || !node.Available() || path == nil {
		return nodeValue{}, false
	}
	switch node.Kind() {
	case algebra.KindInput:
		return state.executeInput(node, path)
	case algebra.KindSelect:
		return state.executeSelect(node, path)
	case algebra.KindJoin:
		return state.executeJoin(node, path)
	case algebra.KindComplete:
		return state.executeComplete(node, path)
	default:
		// This is a closed evaluator vocabulary. Scan, Project, Merge, Group,
		// Apply, Publish, and any future kind need their own sealed extent
		// witness before they can cross this boundary.
		return nodeValue{}, false
	}
}

func (state *evaluationState) executeInput(node arrangement.Node, path []uint32) (nodeValue, bool) {
	if node.Kind() != algebra.KindInput {
		return nodeValue{}, false
	}
	extent, ok := state.inputExtent(node, path)
	if !ok {
		return nodeValue{}, false
	}
	bindingValue := extent.Binding()
	source := extent.Source()
	classification, ok := classifySource(source)
	if !ok || !bindingValue.Available() || classification.denominator.Relation() != bindingValue.Relation() {
		return nodeValue{}, false
	}
	if classification.driver {
		batches, driverOK := state.executeDriverInput(bindingValue, classification)
		if !driverOK {
			return nodeValue{}, false
		}
		return nodeValue{batches: batches}, true
	}
	emptyScope, emptyOK := state.emptyScope()
	if !emptyOK {
		return nodeValue{}, false
	}
	reader, readerOK := read.Bind(state.session.root, bindingValue.Values(), state.session.geometry, state.session.scratch)
	if !readerOK || !reader.Available() || !reader.Layout().Equal(bindingValue.Values()) {
		return nodeValue{}, false
	}
	var witnessValue binding.DenominatorWitness
	if classification.partition {
		witnessValue, ok = state.partitionWitness(classification)
		if !ok {
			return nodeValue{}, false
		}
	} else {
		witnessValue, ok = state.session.mounted.Denominator(classification.denominator)
		if !ok {
			return nodeValue{}, false
		}
	}
	if !witnessValue.Available() || !witnessValue.ValidFor(state.session.mounted.RuntimeFence()) || !witnessValue.Matches(classification.denominator) {
		return nodeValue{}, false
	}
	batches, executeOK := inputop.ExecuteExtentFromWitness(bindingValue, state.session.mounted, reader, witnessValue, emptyScope)
	if !executeOK || !batchesAvailable(batches) {
		return nodeValue{}, false
	}
	return nodeValue{batches: batches}, true
}

func (state *evaluationState) executeDriverInput(bindingValue arrangement.InputBinding, source extentSource) ([]tuple.Batch, bool) {
	if !source.driver || !source.driverLayout.Available() || !bindingValue.Available() || source.driverLayout.Access().Relation() != bindingValue.Relation() || source.slot.Child() != state.subtree.Ordinal() || (state.populationRef.Available() && state.populationRef != source.denominator) {
		return nil, false
	}
	columns := bindingValue.Values().Columns()
	if uint64(source.slot.Cell()) >= uint64(len(columns)) {
		return nil, false
	}
	populationWitness := state.populationWitness
	if !populationWitness.Available() || !populationWitness.ValidFor(state.session.mounted.RuntimeFence()) || !populationWitness.Matches(source.denominator) || populationWitness.Relation() != bindingValue.Relation() || !populationWitness.Contains(state.population) {
		return nil, false
	}
	// Population authentication uses the driver's complete coordinate layout,
	// while this Input may intentionally expose a narrower declared projection
	// of that same row. Bind the exact Input layout and retain the already-
	// authenticated RowID/scope; requiring the two layouts to be identical
	// would make scalar population children impossible unless they leaked every
	// driver column into the semantic frame.
	reader, readerOK := read.Bind(state.session.root, bindingValue.Values(), state.session.geometry, state.session.scratch)
	if !readerOK || !reader.Available() || !reader.Layout().Equal(bindingValue.Values()) {
		return nil, false
	}
	batch, batchOK := inputop.ExecuteRow(bindingValue, state.session.mounted, reader, populationWitness, state.population, state.populationScope)
	if !batchOK || !batch.Available() || !batch.ValidFor(state.session.mounted) || !batch.Scope().Same(state.populationScope) || batch.Len() != 1 {
		return nil, false
	}
	return []tuple.Batch{batch}, true
}

func (state *evaluationState) executeSelect(node arrangement.Node, path []uint32) (nodeValue, bool) {
	bindingValue, ok := node.Select()
	children := node.Children()
	if !ok || !bindingValue.Available() || len(children) != 1 {
		return nodeValue{}, false
	}
	child, childOK := state.executeNode(children[0], appendPath(path, 0))
	if !childOK || !child.available() {
		return nodeValue{}, false
	}
	result := make([]tuple.Batch, 0, len(child.batches))
	for _, source := range child.batches {
		values, executeOK := selectop.Execute(bindingValue, state.session.mounted, state.session.geometry, source)
		if !executeOK || values == nil || !batchesAvailable(values) {
			return nodeValue{}, false
		}
		result = append(result, values...)
	}
	return nodeValue{batches: result}, true
}

func (state *evaluationState) executeJoin(node arrangement.Node, path []uint32) (nodeValue, bool) {
	bindingValue, ok := node.Join()
	children := node.Children()
	if !ok || !bindingValue.Available() || len(children) != 2 {
		return nodeValue{}, false
	}
	left, leftOK := state.executeNode(children[0], appendPath(path, 0))
	right, rightOK := state.executeNode(children[1], appendPath(path, 1))
	if !leftOK || !rightOK || !left.available() || !right.available() {
		return nodeValue{}, false
	}
	result := make([]tuple.Batch, 0)
	for _, leftBatch := range left.batches {
		for _, rightBatch := range right.batches {
			value, executeOK := joinop.Join(bindingValue, state.session.mounted, state.session.geometry, leftBatch, rightBatch)
			if !executeOK || !value.Available() || !value.ValidFor(state.session.mounted) {
				return nodeValue{}, false
			}
			result = append(result, value)
		}
	}
	return nodeValue{batches: result}, true
}

func (state *evaluationState) executeComplete(node arrangement.Node, path []uint32) (nodeValue, bool) {
	bindingValue, ok := node.Complete()
	children := node.Children()
	if !ok || !bindingValue.Available() || len(children) != 1 {
		return nodeValue{}, false
	}
	extent, extentOK := state.completeExtent(node, path)
	if !extentOK || extent.Binding().Denominator() != bindingValue.Denominator() {
		return nodeValue{}, false
	}
	classification, sourceOK := classifySource(extent.Source())
	if !sourceOK || classification.driver {
		return nodeValue{}, false
	}
	var witnessValue binding.DenominatorWitness
	if classification.partition {
		witnessValue, sourceOK = state.partitionWitness(classification)
	} else {
		witnessValue, sourceOK = state.session.mounted.Denominator(classification.denominator)
	}
	if !sourceOK || !witnessValue.Available() || !witnessValue.ValidFor(state.session.mounted.RuntimeFence()) || !witnessValue.Matches(bindingValue.Denominator()) {
		return nodeValue{}, false
	}
	child, childOK := state.executeNode(children[0], appendPath(path, 0))
	if !childOK || !child.available() {
		return nodeValue{}, false
	}
	result := make([]tuple.Batch, 0, len(child.batches))
	for _, source := range child.batches {
		value, executeOK := completeop.Execute(bindingValue, state.session.mounted, source, witnessValue)
		if !executeOK || !value.Available() || !value.ValidFor(state.session.mounted) {
			return nodeValue{}, false
		}
		result = append(result, value)
	}
	return nodeValue{batches: result}, true
}

// partitionWitness redeems the exact q-local posting only after checking its
// directory's solve-local fence and population domain.  Lookup itself is the
// sole row selection operation; no directory enumeration or relation-level
// fallback is permitted. The replay-bound population authority must name the
// same directory domain as well.
func (state *evaluationState) partitionWitness(source extentSource) (binding.DenominatorWitness, bool) {
	if state == nil || !source.partition || !source.directory.Available() || !source.directory.ValidFor(state.session.mounted.RuntimeFence()) {
		return binding.DenominatorWitness{}, false
	}
	population := source.directory.Population()
	if !population.Available() || population.Relation() != state.population.Relation() {
		return binding.DenominatorWitness{}, false
	}
	if state.populationRef.Available() && population != state.populationRef {
		return binding.DenominatorWitness{}, false
	}
	value, ok := source.directory.Lookup(state.population)
	if !ok || !value.Available() || !value.ValidFor(state.session.mounted.RuntimeFence()) || !value.Matches(source.denominator) {
		return binding.DenominatorWitness{}, false
	}
	return value, true
}

func (state *evaluationState) inputExtent(node arrangement.Node, path []uint32) (arrangement.InputExtent, bool) {
	extent, ok := state.subtree.InputFor(node, path)
	if !ok || !extent.Available() || extent.Node() != node || !samePath(extent.Occurrence().Path(), path) {
		return arrangement.InputExtent{}, false
	}
	key := occurrenceKey{node: node, path: pathKey(path)}
	if state.usedInputs[key] {
		return arrangement.InputExtent{}, false
	}
	state.usedInputs[key] = true
	return extent, true
}

func (state *evaluationState) completeExtent(node arrangement.Node, path []uint32) (arrangement.DenominatorExtent, bool) {
	extent, ok := state.subtree.CompleteFor(node, path)
	if !ok || !extent.Available() || extent.Node() != node || !samePath(extent.Occurrence().Path(), path) {
		return arrangement.DenominatorExtent{}, false
	}
	key := occurrenceKey{node: node, path: pathKey(path)}
	if state.usedCompletes[key] {
		return arrangement.DenominatorExtent{}, false
	}
	state.usedCompletes[key] = true
	return extent, true
}

func (state *evaluationState) allExtentsUsed() bool {
	if state == nil || !state.subtree.Available() {
		return false
	}
	seenInputs := make(map[occurrenceKey]struct{}, state.subtree.InputCount())
	for index := 0; index < state.subtree.InputCount(); index++ {
		extent, ok := state.subtree.InputAt(index)
		if !ok || !extent.Available() {
			return false
		}
		occurrence := extent.Occurrence()
		key := occurrenceKey{node: occurrence.Node(), path: pathKey(occurrence.Path())}
		if _, duplicate := seenInputs[key]; duplicate {
			// A repeated authorized occurrence would let one redeemed extent
			// satisfy two physical leaves.  Exact node+path identity is the
			// admission key; duplicate anchors are never coalesced.
			return false
		}
		seenInputs[key] = struct{}{}
		if !state.usedInputs[key] {
			return false
		}
	}
	seenCompletes := make(map[occurrenceKey]struct{}, state.subtree.CompleteCount())
	for index := 0; index < state.subtree.CompleteCount(); index++ {
		extent, ok := state.subtree.CompleteAt(index)
		if !ok || !extent.Available() {
			return false
		}
		occurrence := extent.Occurrence()
		key := occurrenceKey{node: occurrence.Node(), path: pathKey(occurrence.Path())}
		if _, duplicate := seenCompletes[key]; duplicate {
			return false
		}
		seenCompletes[key] = struct{}{}
		if !state.usedCompletes[key] {
			return false
		}
	}
	return true
}

func (state *evaluationState) emptyScope() (witness.Scope, bool) {
	selectBinding, ok := state.subtree.EmptyScope()
	if !ok || !selectBinding.Available() || !selectBinding.ValidFor(state.session.mounted.Fence()) {
		return witness.Scope{}, false
	}
	scope, scopeOK := state.session.mounted.Scope(selectBinding.Scope())
	if !scopeOK || !scope.ValidFor(state.session.mounted.RuntimeFence()) {
		return witness.Scope{}, false
	}
	return scope, true
}

type extentSource struct {
	denominator  model.DenominatorRef
	driverLayout arrangement.Layout
	slot         algebra.SlotSource
	directory    binding.PartitionDirectory
	driver       bool
	partition    bool
}

// classifySource deliberately uses only the closed source accessors.  In
// particular, it does not inspect a public Kind discriminator or infer a
// source from relation names, layout keys, or runtime cardinality.
func classifySource(source arrangement.CorrelationExtentSource) (extentSource, bool) {
	if !source.Available() {
		return extentSource{}, false
	}
	denominator, denominatorOK := source.Denominator()
	if !denominatorOK || !denominator.Available() {
		return extentSource{}, false
	}
	driver, slot, driverOK := source.PopulationDriver()
	directory, partitionOK := source.Partition()
	if driverOK && partitionOK {
		return extentSource{}, false
	}
	if driverOK {
		if !driver.Available() || !slotCellUsable(slot) || directory.Available() {
			return extentSource{}, false
		}
		return extentSource{denominator: denominator, driverLayout: driver, slot: slot, driver: true}, true
	}
	if partitionOK {
		if !directory.Available() || directory.Child() != denominator {
			return extentSource{}, false
		}
		return extentSource{denominator: denominator, directory: directory, partition: true}, true
	}
	return extentSource{denominator: denominator}, true
}

func slotCellUsable(slot algebra.SlotSource) bool {
	// SlotSource intentionally has no Available predicate: its enclosing
	// certificate proves the child/cell address.  A zero slot is nevertheless a
	// valid address, so there is no value-level check beyond its use against the
	// sealed binding vector in executeDriverInput.
	return true
}

func appendPath(path []uint32, child uint32) []uint32 {
	result := make([]uint32, len(path)+1)
	copy(result, path)
	result[len(path)] = child
	return result
}

func samePath(left, right []uint32) bool {
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

func pathKey(path []uint32) string {
	if path == nil {
		return ""
	}
	encoded := make([]byte, 4*len(path)+4)
	binary.BigEndian.PutUint32(encoded, uint32(len(path)))
	for index, step := range path {
		binary.BigEndian.PutUint32(encoded[4+4*index:], step)
	}
	return string(encoded)
}
