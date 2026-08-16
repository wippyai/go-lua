package recurrence

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// hierarchyEvent is the private, graph-owner-issued Bourdoncle bracket
// stream. Vertices are sourcecontrol coordinates only while this seal is
// live; neither coordinates nor this proof can cross the recurrence binding.
type HierarchyEventKind uint8

const (
	HierarchyEnter HierarchyEventKind = iota + 1
	HierarchyPoint
	HierarchyExit
)

type HierarchyEvent struct {
	Kind HierarchyEventKind
	path identity.ContentID
}

type hierarchyEvent struct {
	Kind   HierarchyEventKind
	vertex uint32
	path   identity.ContentID
}

// hierarchyProof is produced once from the existing sourcecontrol graph. It
// is deliberately not an analyzer schedule: Causal must consume it together
// with the exact route plan and replace vertices with final Site/route refs.
type HierarchyProof struct{ events []HierarchyEvent }

type hierarchyDraft struct{ events []hierarchyEvent }

// hierarchyRegion is an exact baseline WTO bracket, identified by its Enter
// event and matching Exit event. It is deliberately not an SCC head: nested
// Bourdoncle regions may share an SCC while requiring different insertion
// sites for a carried Outcome subdivision.
type hierarchyRegions struct {
	nodeRegion []int
	nodeEvent  []int
	parent     []int
	enter      []int
	exitAt     []int
	children   [][]int
}

func hierarchyRegionsFor(events []hierarchyEvent, parts components) (hierarchyRegions, error) {
	regions := hierarchyRegions{
		nodeRegion: make([]int, len(parts.of)), nodeEvent: make([]int, len(parts.of)),
		parent: []int{-1}, enter: []int{-1}, exitAt: []int{-1}, children: make([][]int, 1),
	}
	for index := range regions.nodeEvent {
		regions.nodeEvent[index] = -1
	}
	active := 0 // virtual root: explicit root placement, never a WTO bracket.
	for index, event := range events {
		if int(event.vertex) >= len(regions.nodeRegion) {
			return hierarchyRegions{}, errors.New("program/flow/recurrence: hierarchy region vertex is outside components")
		}
		switch event.Kind {
		case HierarchyEnter:
			region := len(regions.parent)
			regions.parent = append(regions.parent, active)
			regions.enter = append(regions.enter, index)
			regions.exitAt = append(regions.exitAt, -1)
			regions.children = append(regions.children, nil)
			regions.children[active] = append(regions.children[active], region)
			active = region
			regions.nodeRegion[event.vertex], regions.nodeEvent[event.vertex] = active, index
		case HierarchyPoint:
			regions.nodeRegion[event.vertex], regions.nodeEvent[event.vertex] = active, index
		case HierarchyExit:
			if active == 0 {
				return hierarchyRegions{}, errors.New("program/flow/recurrence: hierarchy region brackets are unbalanced")
			}
			if events[regions.enter[active]].vertex != event.vertex {
				return hierarchyRegions{}, errors.New("program/flow/recurrence: hierarchy region exit differs from header")
			}
			regions.exitAt[active] = index
			active = regions.parent[active]
		default:
			return hierarchyRegions{}, errors.New("program/flow/recurrence: hierarchy region event is invalid")
		}
	}
	if active != 0 {
		return hierarchyRegions{}, errors.New("program/flow/recurrence: hierarchy region brackets remain open")
	}
	return regions, nil
}

func (regions hierarchyRegions) contains(region int, node uint32) bool {
	if region == 0 || int(node) >= len(regions.nodeRegion) || regions.nodeEvent[node] < 0 || region >= len(regions.enter) {
		return false
	}
	return regions.enter[region] <= regions.nodeEvent[node] && regions.nodeEvent[node] <= regions.exitAt[region]
}

func (regions hierarchyRegions) exit(region int) (int, bool) {
	if region <= 0 || region >= len(regions.exitAt) || regions.exitAt[region] < 0 {
		return 0, false
	}
	return regions.exitAt[region], true
}

type regionLCAQuery struct{ left, right int }

// lcas folds every region-tree LCA query onto the shared offline Tarjan pass.
// Region 0 is the single virtual root the bracket stream always opens with, so
// every region shares one root and a query naming it, or naming a region
// outside the tree, resolves to that explicit root placement.
func (regions hierarchyRegions) lcas(queries []regionLCAQuery) ([]int, error) {
	answers := make([]int, len(queries))
	if len(regions.parent) == 0 {
		return answers, nil
	}
	parents := make([]uint32, len(regions.parent))
	roots := make([]uint32, len(regions.parent))
	for index, parent := range regions.parent {
		if parent < 0 {
			parents[index] = NoNode
			continue
		}
		parents[index] = uint32(parent)
	}
	slots := make([]int, 0, len(queries))
	left := make([]uint32, 0, len(queries))
	right := make([]uint32, 0, len(queries))
	for index, query := range queries {
		if query.left <= 0 || query.right <= 0 || query.left >= len(parents) || query.right >= len(parents) {
			continue
		}
		slots = append(slots, index)
		left = append(left, uint32(query.left))
		right = append(right, uint32(query.right))
	}
	resolved := OfflineLCAs(parents, roots, left, right)
	if len(resolved) != len(slots) {
		return nil, errors.New("program/flow/recurrence: hierarchy region tree is not a rooted forest")
	}
	for position, slot := range slots {
		if resolved[position] != NoNode {
			answers[slot] = int(resolved[position])
		}
	}
	return answers, nil
}

// placeOutcomePhasePoints extends the already-derived CSR hierarchy without
// changing any SCC partition. A phase reached through a proven intra-SCC
// carrier is inserted immediately before that component's existing Exit;
// carrierless and acyclic phases remain root singleton points. More than one
// carrier for the same phase must select the same component.
func (draft *hierarchyDraft) placeOutcomePhasePoints(
	proof sourcecontrol.OutcomePhases,
	plan *routeplan.Plan,
	graph *sourcecontrol.Result,
	parts components,
	heads []keyspace.Term,
) error {
	if draft == nil || plan == nil || graph == nil {
		return errors.New("program/flow/recurrence: Outcome phase placement is unavailable")
	}
	paths := make([]identity.ContentID, proof.Count())
	pathSet := make(map[identity.ContentID]struct{}, len(paths))
	parentByPath := make(map[identity.ContentID]identity.ContentID, len(paths))
	pathOrder := make(map[identity.ContentID]int, len(paths))
	for index := range paths {
		phase, ok := proof.At(index)
		path, pathOK := phase.VertexPath()
		if !ok || !pathOK {
			return errors.New("program/flow/recurrence: Outcome phase receipt is unavailable")
		}
		if _, duplicate := pathSet[path]; duplicate {
			return errors.New("program/flow/recurrence: Outcome phase receipt is duplicated")
		}
		paths[index], pathSet[path], pathOrder[path] = path, struct{}{}, index
		if parent, parentOK := phase.ParentPath(); parentOK {
			parentByPath[path] = parent
		}
	}
	for child, parent := range parentByPath {
		parentIndex, present := pathOrder[parent]
		if !present || parentIndex <= pathOrder[child] {
			return errors.New("program/flow/recurrence: Outcome receipt is not child-before-parent")
		}
	}
	regions, err := hierarchyRegionsFor(draft.events, parts)
	if err != nil {
		return err
	}
	type anchorRange struct {
		seen, root bool
		min, max   int
	}
	anchors := make(map[identity.ContentID]anchorRange)
	children := make(map[identity.ContentID]map[identity.ContentID]struct{})
	anchorPhase := func(path identity.ContentID, region int) error {
		if _, scheduled := pathSet[path]; !scheduled {
			return errors.New("program/flow/recurrence: route Outcome phase is outside receipt")
		}
		current := anchors[path]
		current.seen = true
		if region == 0 {
			current.root = true
			anchors[path] = current
			return nil
		}
		if current.min == 0 && current.max == 0 {
			current.min, current.max = region, region
		} else {
			if regions.enter[region] < regions.enter[current.min] {
				current.min = region
			}
			if regions.enter[region] > regions.enter[current.max] {
				current.max = region
			}
		}
		anchors[path] = current
		return nil
	}
	for ordinal := 0; ordinal < plan.Count(); ordinal++ {
		_, origin, ok := plan.At(ordinal)
		if !ok {
			return errors.New("program/flow/recurrence: Outcome placement route is malformed")
		}
		endpoint, endpointOK := origin.EndpointPhaseReceipt()
		if !endpointOK {
			return errors.New("program/flow/recurrence: Outcome placement endpoint is unavailable")
		}
		fromPhase, toPhase, endpointsOK := endpoint.Endpoints()
		if !endpointsOK {
			return errors.New("program/flow/recurrence: Outcome placement endpoints are malformed")
		}
		fromPath, fromPathOK := graph.ResolvePhaseRef(fromPhase)
		toPath, toPathOK := graph.ResolvePhaseRef(toPhase)
		if !fromPathOK || !toPathOK {
			return errors.New("program/flow/recurrence: Outcome placement paths are unavailable")
		}
		// SourceControl alone schedules Outcome propagation.  Plan rows only
		// attest the exact endpoint relations used for placement; record them
		// before carrier filtering so carrierless propagation cannot disappear.
		if fromPhase.OutcomePhase() && toPhase.OutcomePhase() && fromPath != toPath {
			if _, fromKnown := pathSet[fromPath]; !fromKnown {
				return errors.New("program/flow/recurrence: Outcome propagation source is outside receipt")
			}
			if _, toKnown := pathSet[toPath]; !toKnown {
				return errors.New("program/flow/recurrence: Outcome propagation target is outside receipt")
			}
			if parentByPath[fromPath] != toPath {
				return errors.New("program/flow/recurrence: Outcome propagation disagrees with parent receipt")
			}
		}
		if fromPhase.OutcomePhase() && !toPhase.OutcomePhase() {
			_, csr := graph.ResolveCSRPhaseNode(toPhase)
			if !csr {
				return errors.New("program/flow/recurrence: Outcome resume endpoint is not CSR")
			}
		}
		carrier, carrierOK := origin.RecurrenceCarrier()
		if !carrierOK {
			return errors.New("program/flow/recurrence: Outcome placement carrier is unavailable")
		}
		var from, to uint32
		if arc, arcOK := carrier.ArcRef(); arcOK {
			_, row, resolved := graph.ResolveArcRef(arc)
			if !resolved {
				return errors.New("program/flow/recurrence: Outcome placement Arc is foreign")
			}
			from, to = row.From, row.To
		} else if left, right, pairOK := carrier.NodePair(); pairOK {
			var leftOK, rightOK bool
			from, leftOK = graph.ResolveNodeRef(left)
			to, rightOK = graph.ResolveNodeRef(right)
			if !leftOK || !rightOK {
				return errors.New("program/flow/recurrence: Outcome placement NodePair is foreign")
			}
		} else {
			continue
		}
		row := bindingForNodes(parts, heads, from, to)
		if !row.member {
			continue
		}
		fromRegion, toRegion := regions.nodeRegion[from], regions.nodeRegion[to]
		if fromPhase.OutcomePhase() {
			if err := anchorPhase(fromPath, fromRegion); err != nil {
				return err
			}
			if err := anchorPhase(fromPath, toRegion); err != nil {
				return err
			}
		}
		if toPhase.OutcomePhase() {
			if err := anchorPhase(toPath, fromRegion); err != nil {
				return err
			}
			if err := anchorPhase(toPath, toRegion); err != nil {
				return err
			}
		}
	}
	// The parent-issued relation is the sole propagation authority. Plan rows
	// above only attest that any observed Outcome→Outcome edge agrees with it.
	for child, parent := range parentByPath {
		if _, present := pathSet[parent]; !present {
			return errors.New("program/flow/recurrence: Outcome parent is outside receipt")
		}
		if children[child] == nil {
			children[child] = make(map[identity.ContentID]struct{})
		}
		children[child][parent] = struct{}{}
	}
	// OutcomePhases is the parent-issued Kahn certificate, ordered child before
	// parent. Follow it verbatim: Plan can omit static/dead rows and therefore
	// has no authority to rebuild or reorder propagation.
	for _, path := range paths {
		current, anchored := anchors[path]
		if !anchored || !current.seen {
			continue
		}
		for parent := range children[path] {
			if current.root {
				if err := anchorPhase(parent, 0); err != nil {
					return err
				}
				continue
			}
			if err := anchorPhase(parent, current.min); err != nil {
				return err
			}
			if err := anchorPhase(parent, current.max); err != nil {
				return err
			}
		}
	}
	// Fold every direct/carried anchor set to its exact common hierarchy LCA in
	// one offline linear pass. A set spanning separate root regions becomes an
	// explicit root placement rather than a false conflict.
	queries := make([]regionLCAQuery, 0, len(anchors))
	queryPath := make([]identity.ContentID, 0, len(anchors))
	for _, path := range paths {
		current, anchored := anchors[path]
		if !anchored || !current.seen || current.root {
			continue
		}
		queries = append(queries, regionLCAQuery{left: current.min, right: current.max})
		queryPath = append(queryPath, path)
	}
	answers, lcaErr := regions.lcas(queries)
	if lcaErr != nil {
		return lcaErr
	}
	finalAnchors := make(map[identity.ContentID]int, len(anchors))
	for path, current := range anchors {
		if current.seen && current.root {
			finalAnchors[path] = 0
		}
	}
	for index, path := range queryPath {
		finalAnchors[path] = answers[index]
	}
	anchorExits := make(map[int][]identity.ContentID, len(anchors))
	for _, path := range paths {
		region, anchored := finalAnchors[path]
		if !anchored {
			continue
		}
		if region == 0 {
			continue
		}
		exit, exitOK := regions.exit(region)
		if !exitOK {
			return errors.New("program/flow/recurrence: anchored Outcome phase lacks hierarchy exit")
		}
		anchorExits[exit] = append(anchorExits[exit], path)
	}
	seen := make(map[identity.ContentID]struct{}, len(paths))
	events := make([]hierarchyEvent, 0, len(draft.events)+len(paths))
	for eventIndex, event := range draft.events {
		if event.Kind == HierarchyExit {
			for _, path := range anchorExits[eventIndex] {
				if _, duplicate := seen[path]; duplicate {
					return errors.New("program/flow/recurrence: Outcome phase inserted twice")
				}
				seen[path] = struct{}{}
				events = append(events, hierarchyEvent{Kind: HierarchyPoint, path: path})
			}
		}
		events = append(events, event)
	}
	for _, path := range paths {
		if _, inserted := seen[path]; inserted {
			continue
		}
		if region, anchored := finalAnchors[path]; anchored && region != 0 {
			return errors.New("program/flow/recurrence: anchored Outcome phase was not inserted into hierarchy")
		}
		seen[path] = struct{}{}
		events = append(events, hierarchyEvent{Kind: HierarchyPoint, path: path})
	}
	if len(seen) != len(paths) {
		return errors.New("program/flow/recurrence: Outcome phase coverage is incomplete")
	}
	draft.events = events
	return nil
}

func (draft hierarchyDraft) transfer() (HierarchyProof, error) {
	events := make([]HierarchyEvent, len(draft.events))
	for index, event := range draft.events {
		if !event.path.Available() {
			return HierarchyProof{}, errors.New("program/flow/recurrence: hierarchy path receipt is unavailable")
		}
		events[index] = HierarchyEvent{Kind: event.Kind, path: event.path}
	}
	return HierarchyProof{events: events}, nil
}

func (proof HierarchyProof) Count() int { return len(proof.events) }
func (proof HierarchyProof) At(index int) (HierarchyEvent, bool) {
	if index < 0 || index >= len(proof.events) {
		return HierarchyEvent{}, false
	}
	return proof.events[index], true
}

// VertexPath is the semantic receipt for the event's vertex. It is copied
// from SourceControl during recurrence sealing so downstream consumers do not
// need to retain or re-query the graph coordinate.
func (event HierarchyEvent) VertexPath() (identity.ContentID, bool) {
	if !event.path.Available() {
		return identity.ContentID{}, false
	}
	return event.path, true
}

// deriveHierarchy is the iterative Bourdoncle partitioner. It reads each
// published sourcecontrol successor row once and never re-runs Tarjan or
// rebuilds adjacency from authored terms. The graph already emits unique,
// ascending successors, so no secondary sort/table is needed here.
func deriveHierarchy(graph *sourcecontrol.Result, live []bool) (hierarchyDraft, error) {
	var empty hierarchyDraft
	if graph == nil || graph.NodeCount() == 0 || !graph.VertexCatalogAvailable() {
		return empty, errors.New("program/flow/recurrence: hierarchy graph is unavailable")
	}
	count := int(graph.NodeCount())
	if len(live) != count {
		return empty, errors.New("program/flow/recurrence: hierarchy live denominator is unavailable")
	}
	dfn := make([]int, count)
	vertexStack := make([]uint32, 0, count)
	const done = int(^uint(0) >> 1)
	next := 0
	partitions := make([][]hierarchyEvent, 1)
	type frame struct {
		vertex, partition, body, nextSuccessor, head int
		loop, bodyPhase, awaitMinimum                bool
	}
	frames := make([]frame, 0, count)
	push := func(vertex, partition int) {
		next++
		dfn[vertex] = next
		vertexStack = append(vertexStack, uint32(vertex))
		frames = append(frames, frame{vertex: vertex, partition: partition, body: -1, head: next})
	}
	absorb := func(current *frame, minimum int) {
		if minimum <= current.head {
			current.head = minimum
			current.loop = true
		}
	}
	finish := func(minimum int) {
		frames = frames[:len(frames)-1]
		if len(frames) != 0 {
			parent := &frames[len(frames)-1]
			if parent.awaitMinimum {
				parent.awaitMinimum = false
				absorb(parent, minimum)
			}
		}
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		rootNode, canonical := graph.CanonicalNodeAt(ordinal)
		if !canonical {
			return empty, errors.New("program/flow/recurrence: canonical vertex permutation is unavailable")
		}
		root := int(rootNode)
		if !live[root] || dfn[root] != 0 {
			continue
		}
		push(root, 0)
		for len(frames) != 0 {
			current := &frames[len(frames)-1]
			if !current.bodyPhase && current.nextSuccessor < graph.SuccessorCount(uint32(current.vertex)) {
				successor, ok := graph.SuccessorAt(uint32(current.vertex), current.nextSuccessor)
				current.nextSuccessor++
				if !ok || successor >= uint32(count) {
					return empty, errors.New("program/flow/recurrence: hierarchy successor disappeared")
				}
				if !live[successor] {
					continue
				}
				minimum := dfn[successor]
				if minimum == 0 {
					current.awaitMinimum = true
					push(int(successor), current.partition)
					continue
				}
				absorb(current, minimum)
				continue
			}
			if !current.bodyPhase {
				if current.head != dfn[current.vertex] {
					finish(current.head)
					continue
				}
				dfn[current.vertex] = done
				if !current.loop {
					vertexStack = vertexStack[:len(vertexStack)-1]
					path, pathOK := graph.VertexPathAt(uint32(current.vertex))
					if !pathOK {
						return empty, errors.New("program/flow/recurrence: hierarchy vertex path disappeared")
					}
					partitions[current.partition] = append(partitions[current.partition], hierarchyEvent{Kind: HierarchyPoint, vertex: uint32(current.vertex), path: path})
					finish(current.head)
					continue
				}
				for {
					last := len(vertexStack) - 1
					vertex := vertexStack[last]
					vertexStack = vertexStack[:last]
					dfn[vertex] = 0
					if vertex == uint32(current.vertex) {
						break
					}
				}
				dfn[current.vertex] = done
				current.body = len(partitions)
				partitions = append(partitions, make([]hierarchyEvent, 0))
				current.bodyPhase = true
				current.nextSuccessor = 0
				continue
			}
			if current.nextSuccessor < graph.SuccessorCount(uint32(current.vertex)) {
				successor, ok := graph.SuccessorAt(uint32(current.vertex), current.nextSuccessor)
				current.nextSuccessor++
				if !ok || successor >= uint32(count) {
					return empty, errors.New("program/flow/recurrence: hierarchy body successor disappeared")
				}
				if live[successor] && dfn[successor] == 0 {
					push(int(successor), current.body)
				}
				continue
			}
			// partitions contain flattened bracket subtrees. Reversing that raw
			// stream would turn Enter/Exit pairs inside-out; DFS successor order
			// is already canonical in sourcecontrol, so preserve it verbatim.
			body := partitions[current.body]
			path, pathOK := graph.VertexPathAt(uint32(current.vertex))
			if !pathOK {
				return empty, errors.New("program/flow/recurrence: hierarchy vertex path disappeared")
			}
			partitions[current.partition] = append(partitions[current.partition], hierarchyEvent{Kind: HierarchyEnter, vertex: uint32(current.vertex), path: path})
			partitions[current.partition] = append(partitions[current.partition], body...)
			partitions[current.partition] = append(partitions[current.partition], hierarchyEvent{Kind: HierarchyExit, vertex: uint32(current.vertex), path: path})
			finish(current.head)
		}
	}
	root := partitions[0]
	return hierarchyDraft{events: root}, validateHierarchy(root, live)
}

func validateHierarchy(events []hierarchyEvent, live []bool) error {
	seen := make([]bool, len(live))
	stack := make([]uint32, 0)
	for _, event := range events {
		if int(event.vertex) >= len(live) || !live[event.vertex] {
			return errors.New("program/flow/recurrence: hierarchy vertex is invalid")
		}
		switch event.Kind {
		case HierarchyEnter:
			// A cyclic header is its schedule point. Claim it at Enter so a
			// second Enter or a later Point cannot silently duplicate a vertex.
			if seen[event.vertex] {
				return errors.New("program/flow/recurrence: hierarchy duplicated a header vertex")
			}
			seen[event.vertex] = true
			stack = append(stack, event.vertex)
		case HierarchyPoint:
			if seen[event.vertex] {
				return errors.New("program/flow/recurrence: hierarchy duplicated a vertex")
			}
			seen[event.vertex] = true
		case HierarchyExit:
			if len(stack) == 0 || stack[len(stack)-1] != event.vertex {
				return errors.New("program/flow/recurrence: hierarchy brackets are unbalanced")
			}
			stack = stack[:len(stack)-1]
		default:
			return errors.New("program/flow/recurrence: hierarchy event is invalid")
		}
	}
	if len(stack) != 0 {
		return errors.New("program/flow/recurrence: hierarchy did not close")
	}
	for index, present := range seen {
		if live[index] && !present {
			return errors.New("program/flow/recurrence: hierarchy omitted a graph vertex")
		}
	}
	return nil
}
