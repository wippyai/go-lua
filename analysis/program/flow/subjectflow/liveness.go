package subjectflow

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// subjectKey is the stable local key used by the body/subject indexes.  The
// ContentID is the owner-issued semantic subject path; a raw Term alone is
// deliberately not enough to identify a subject across replays.
type subjectKey struct {
	kind SubjectKind
	id   identity.ContentID
}

func makeSubjectKey(subject Subject) subjectKey {
	return subjectKey{kind: subject.Kind, id: subject.ID}
}

// graphRoute is the exact Causal route preimage needed by the neutral
// liveness query.  The endpoint Terms are used for reachability; point paths
// are retained only to authenticate the paired Boundary row.  No Source or
// SourceControl graph is reconstructed here.
type graphRoute struct {
	id       identity.ContentID
	from     keyspace.Term
	to       keyspace.Term
	arm      causal.BoundaryArmKind
	fromPath identity.ContentID
	toPath   identity.ContentID
}

// subjectGraph is a Term graph borrowed from the sealed Causal successor
// authority.  SCCs are only a query acceleration/termination structure: the
// graph never escapes this package and no second causal edge authority is
// published.
type subjectGraph struct {
	nodes     []keyspace.Term
	nodeIndex map[keyspace.Term]int
	edges     [][]int
	reverse   [][]int

	component    []int
	components   [][]int
	componentOut [][]int
	componentIn  [][]int
	routes       map[identity.ContentID]graphRoute
	termPaths    map[keyspace.Term]identity.ContentID
}

// newSubjectGraph indexes the existing Causal route union once.  The old
// liveness implementation ordered semantic paths by LocalWTO event index;
// that scalar order is not a control-flow relation in a branch or an SCC.
// This index keeps route/body queries O(1) while all reachability is answered
// against exact Causal endpoints.
func newSubjectGraph(result *causal.Result) (*subjectGraph, error) {
	if result == nil {
		return nil, ErrUnavailable
	}

	terms := make(map[keyspace.Term]struct{})
	routes := make([]graphRoute, 0, result.Successors().TotalCount())
	routeIndex := make(map[identity.ContentID]graphRoute, result.Successors().TotalCount())
	for index := 0; index < result.Successors().TotalCount(); index++ {
		successor, ok := result.Successors().TotalAt(index)
		if !ok || keyspace.TermFamily(successor.From) == keyspace.FamilyInvalid || keyspace.TermFamily(successor.To) == keyspace.FamilyInvalid {
			return nil, fmt.Errorf("%w: causal successor %d is unavailable", ErrMalformed, index)
		}
		id, idOK := successor.SemanticID()
		fromPoint, fromPointOK := successor.FromPoint()
		toPoint, toPointOK := successor.ToPoint()
		if !idOK || !id.Available() || !fromPointOK || !toPointOK || !fromPoint.PathID().Available() || !toPoint.PathID().Available() {
			return nil, fmt.Errorf("%w: causal successor %d has no owner-issued endpoint path", ErrMalformed, index)
		}
		route := graphRoute{id: id, from: successor.From, to: successor.To, arm: successor.Arm, fromPath: fromPoint.PathID(), toPath: toPoint.PathID()}
		if previous, duplicate := routeIndex[id]; duplicate {
			// A semantic route ID is the only lawful inverse key.  Refusing an
			// ambiguous inverse is safer than selecting whichever physical row
			// happened to be enumerated first.
			if previous != route {
				return nil, fmt.Errorf("%w: causal route %x is ambiguous", ErrMalformed, id[:4])
			}
			continue
		}
		routeIndex[id] = route
		routes = append(routes, route)
		terms[route.from] = struct{}{}
		terms[route.to] = struct{}{}
	}

	termPaths := make(map[keyspace.Term]identity.ContentID, result.Sites().Count())
	for index := 0; index < result.Sites().Count(); index++ {
		site, ok := result.Sites().At(index)
		if !ok {
			return nil, fmt.Errorf("%w: causal site %d is unavailable", ErrMalformed, index)
		}
		term, termOK := site.Term()
		path := site.PathID()
		if !termOK || keyspace.TermFamily(term) == keyspace.FamilyInvalid || !path.Available() {
			return nil, fmt.Errorf("%w: causal site %d is malformed", ErrMalformed, index)
		}
		if previous, duplicate := termPaths[term]; duplicate && previous != path {
			return nil, fmt.Errorf("%w: causal term %v has ambiguous paths", ErrMalformed, term)
		}
		termPaths[term] = path
		terms[term] = struct{}{}
	}

	nodes := make([]keyspace.Term, 0, len(terms))
	for term := range terms {
		nodes = append(nodes, term)
	}
	sort.Slice(nodes, func(left, right int) bool { return nodes[left] < nodes[right] })
	nodeIndex := make(map[keyspace.Term]int, len(nodes))
	for index, term := range nodes {
		nodeIndex[term] = index
	}
	edges := make([][]int, len(nodes))
	reverse := make([][]int, len(nodes))
	edgeSeen := make(map[[2]int]struct{}, len(routes))
	for _, route := range routes {
		from, fromOK := nodeIndex[route.from]
		to, toOK := nodeIndex[route.to]
		if !fromOK || !toOK {
			return nil, fmt.Errorf("%w: route %x has no graph endpoint", ErrMalformed, route.id[:4])
		}
		pair := [2]int{from, to}
		if _, duplicate := edgeSeen[pair]; duplicate {
			continue
		}
		edgeSeen[pair] = struct{}{}
		edges[from] = append(edges[from], to)
		reverse[to] = append(reverse[to], from)
	}
	for index := range edges {
		sort.Ints(edges[index])
		sort.Ints(reverse[index])
	}

	component, components := stronglyConnectedComponents(edges, reverse)
	componentOut := make([][]int, len(components))
	componentIn := make([][]int, len(components))
	componentEdgeSeen := make(map[[2]int]struct{}, len(routes))
	for from, successors := range edges {
		for _, to := range successors {
			left, right := component[from], component[to]
			if left == right {
				continue
			}
			pair := [2]int{left, right}
			if _, duplicate := componentEdgeSeen[pair]; duplicate {
				continue
			}
			componentEdgeSeen[pair] = struct{}{}
			componentOut[left] = append(componentOut[left], right)
			componentIn[right] = append(componentIn[right], left)
		}
	}
	for index := range componentOut {
		sort.Ints(componentOut[index])
		sort.Ints(componentIn[index])
	}

	return &subjectGraph{
		nodes: nodes, nodeIndex: nodeIndex, edges: edges, reverse: reverse,
		component: component, components: components,
		componentOut: componentOut, componentIn: componentIn,
		routes: routeIndex, termPaths: termPaths,
	}, nil
}

// stronglyConnectedComponents is iterative Kosaraju.  Causal graphs can
// contain long authored loops; using an explicit stack keeps sealing from
// depending on the Go call stack while preserving exact SCC membership.
func stronglyConnectedComponents(edges, reverse [][]int) ([]int, [][]int) {
	visited := make([]bool, len(edges))
	order := make([]int, 0, len(edges))
	type frame struct{ node, next int }
	for root := range edges {
		if visited[root] {
			continue
		}
		visited[root] = true
		stack := []frame{{node: root}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < len(edges[top.node]) {
				next := edges[top.node][top.next]
				top.next++
				if visited[next] {
					continue
				}
				visited[next] = true
				stack = append(stack, frame{node: next})
				continue
			}
			order = append(order, top.node)
			stack = stack[:len(stack)-1]
		}
	}

	component := make([]int, len(edges))
	for index := range component {
		component[index] = -1
	}
	components := make([][]int, 0)
	for orderIndex := len(order) - 1; orderIndex >= 0; orderIndex-- {
		root := order[orderIndex]
		if component[root] != -1 {
			continue
		}
		id := len(components)
		component[root] = id
		members := []int{root}
		stack := []int{root}
		for len(stack) != 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, next := range reverse[node] {
				if component[next] != -1 {
					continue
				}
				component[next] = id
				members = append(members, next)
				stack = append(stack, next)
			}
		}
		sort.Ints(members)
		components = append(components, members)
	}
	return component, components
}

// fixedPoint computes the least graph-reachability fixed point from one
// endpoint.  The transfer relation is evaluated over the SCC condensation,
// so a loop/backedge is closed in one component and cannot manufacture a WTO
// scalar ordering.  reverse=true computes the exact predecessor slice.
func (graph *subjectGraph) fixedPoint(start int, reverse bool) []bool {
	if graph == nil || start < 0 || start >= len(graph.nodes) {
		return nil
	}
	adjacency := graph.componentOut
	if reverse {
		adjacency = graph.componentIn
	}
	startComponent := graph.component[start]
	markedComponents := make([]bool, len(graph.components))
	markedComponents[startComponent] = true
	queue := []int{startComponent}
	for head := 0; head < len(queue); head++ {
		component := queue[head]
		for _, next := range adjacency[component] {
			if markedComponents[next] {
				continue
			}
			markedComponents[next] = true
			queue = append(queue, next)
		}
	}
	markedNodes := make([]bool, len(graph.nodes))
	for component, marked := range markedComponents {
		if !marked {
			continue
		}
		for _, node := range graph.components[component] {
			markedNodes[node] = true
		}
	}
	return markedNodes
}

type eventAttachment struct {
	node     int
	attached bool
}

// livenessIndex owns all transient indexes for one subject-flow seal.  The
// maps are discarded after liveness rows are emitted; Result retains neither
// a graph nor a body index.
type livenessIndex struct {
	graph           *subjectGraph
	events          []eventAttachment
	eventsByOwner   map[keyspace.Term]map[subjectKey][]int
	subjectsByOwner map[keyspace.Term]map[subjectKey]Subject
	aliasesByOwner  map[keyspace.Term]map[subjectKey][]subjectKey
	forwardCache    map[int][]bool
	reverseCache    map[int][]bool
}

func (builder *sealBuilder) buildLivenessIndex() (*livenessIndex, error) {
	graph, err := newSubjectGraph(builder.causal)
	if err != nil {
		return nil, err
	}
	index := &livenessIndex{
		graph:           graph,
		events:          make([]eventAttachment, len(builder.events)),
		eventsByOwner:   make(map[keyspace.Term]map[subjectKey][]int),
		subjectsByOwner: make(map[keyspace.Term]map[subjectKey]Subject),
		aliasesByOwner:  make(map[keyspace.Term]map[subjectKey][]subjectKey),
		forwardCache:    make(map[int][]bool),
		reverseCache:    make(map[int][]bool),
	}
	for eventIndex, event := range builder.events {
		owner, ownerOK := builder.bodyForTerm(event.Term)
		if !ownerOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
			return nil, fmt.Errorf("%w: event %d has no body owner", ErrMalformed, eventIndex)
		}
		bySubject := index.eventsByOwner[owner]
		if bySubject == nil {
			bySubject = make(map[subjectKey][]int)
			index.eventsByOwner[owner] = bySubject
			index.subjectsByOwner[owner] = make(map[subjectKey]Subject)
			index.aliasesByOwner[owner] = make(map[subjectKey][]subjectKey)
		}
		addEventSubject := func(subject Subject) {
			if !subject.Kind.valid() || !subject.ID.Available() {
				return
			}
			key := makeSubjectKey(subject)
			index.subjectsByOwner[owner][key] = subject
			bySubject[key] = append(bySubject[key], eventIndex)
		}
		// roots() issues exactly one RoleRoot Define per body solely to
		// publish that body's owner directory (see subjectsForOwner); the
		// body's own root is not itself a tracked subject. Registering it
		// here would surface a spurious self-liveness row at every boundary
		// the body yields through, including bodies that are never captured
		// or returned as a value. A genuine reference to a body's root
		// (capture, alias, unknown pass-through) arrives through its own
		// event with a different Role and is indexed normally below.
		if event.Kind != EventDefine || event.Role != RoleRoot {
			addEventSubject(event.Subject)
			if !sameSubjectKey(event.Subject, event.Related) {
				addEventSubject(event.Related)
			}
		}
		if event.Kind == EventAlias && event.Subject.Kind.valid() && event.Subject.ID.Available() && event.Related.Kind.valid() && event.Related.ID.Available() {
			left, right := makeSubjectKey(event.Subject), makeSubjectKey(event.Related)
			index.aliasesByOwner[owner][left] = append(index.aliasesByOwner[owner][left], right)
			index.aliasesByOwner[owner][right] = append(index.aliasesByOwner[owner][right], left)
		}

		// Event.Term is an existing authored coordinate.  Causal's Site path
		// is the owner-issued attachment that authenticates it against the
		// final graph; a caller-supplied semantic path is never accepted as a
		// graph node by itself.
		path, pathOK := graph.termPaths[event.Term]
		node, nodeOK := graph.nodeIndex[event.Term]
		index.events[eventIndex] = eventAttachment{node: node, attached: pathOK && nodeOK && path == event.Path}
	}
	return index, nil
}

// componentEvents returns every event attached to one exact-alias component.
// Alias is an equality proof, so a use of either endpoint is a use of the
// represented subject. The closure is seal-local and publishes no second
// alias representation; liveness rows retain the original owner subjects.
func (index *livenessIndex) componentEvents(owner keyspace.Term, subject subjectKey) []int {
	if index == nil {
		return nil
	}
	bySubject := index.eventsByOwner[owner]
	if len(bySubject) == 0 {
		return nil
	}
	aliases := index.aliasesByOwner[owner]
	queue := []subjectKey{subject}
	seenSubjects := make(map[subjectKey]struct{})
	seenEvents := make(map[int]struct{})
	result := make([]int, 0)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := seenSubjects[current]; seen {
			continue
		}
		seenSubjects[current] = struct{}{}
		for _, event := range bySubject[current] {
			if _, seen := seenEvents[event]; seen {
				continue
			}
			seenEvents[event] = struct{}{}
			result = append(result, event)
		}
		queue = append(queue, aliases[current]...)
	}
	sort.Ints(result)
	return result
}

func sameSubjectKey(left, right Subject) bool {
	return left.Kind.valid() && right.Kind.valid() && left.Kind == right.Kind && left.ID.Available() && left.ID == right.ID
}

func (index *livenessIndex) reach(node int, reverse bool) []bool {
	if reverse {
		if cached, ok := index.reverseCache[node]; ok {
			return cached
		}
		cached := index.graph.fixedPoint(node, true)
		index.reverseCache[node] = cached
		return cached
	}
	if cached, ok := index.forwardCache[node]; ok {
		return cached
	}
	cached := index.graph.fixedPoint(node, false)
	index.forwardCache[node] = cached
	return cached
}

func subjectsForOwner(index *livenessIndex, owner keyspace.Term) ([]Subject, bool) {
	if index == nil || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
		return nil, false
	}
	seen, published := index.subjectsByOwner[owner]
	// roots() issues one event for every body before this index is built, so
	// buildLivenessIndex always publishes an owner directory for a real body;
	// an absent directory means the source never reached this owner and is
	// malformed evidence, not proof of an empty subject set. A published but
	// empty directory is the true, common case: this body tracks no local
	// subject beyond its own root bookkeeping, and it must be admitted with
	// zero members rather than refused as if it were missing.
	if !published {
		return nil, false
	}
	subjects := make([]Subject, 0, len(seen))
	for _, subject := range seen {
		subjects = append(subjects, subject)
	}
	sort.Slice(subjects, func(left, right int) bool {
		if subjects[left].Kind != subjects[right].Kind {
			return subjects[left].Kind < subjects[right].Kind
		}
		return bytes.Compare(subjects[left].ID[:], subjects[right].ID[:]) < 0
	})
	return subjects, true
}

func (builder *sealBuilder) boundarySubjects(boundary Boundary, index *livenessIndex) ([]Subject, bool) {
	return subjectsForOwner(index, boundaryOwner(builder, boundary))
}

func boundaryOwner(builder *sealBuilder, boundary Boundary) keyspace.Term {
	owner, _, _, _, ownerOK := builder.authored.Calls().Get(boundary.Call)
	if !ownerOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
		return 0
	}
	return owner
}

func (builder *sealBuilder) classifyLiveness(boundary Boundary, subject Subject, index *livenessIndex) LivenessState {
	if boundary.State != BoundaryPaired || index == nil || index.graph == nil {
		return LivenessUnknown
	}
	owner := boundaryOwner(builder, boundary)
	if owner == 0 {
		return LivenessUnknown
	}
	yield, yieldOK := index.graph.routes[boundary.YieldRoute]
	reentry, reentryOK := index.graph.routes[boundary.ReentryRoute]
	if !yieldOK || !reentryOK || yield.arm != causal.BoundaryYield || !isReentryArm(reentry.arm) ||
		yield.fromPath != boundary.YieldFromPath || yield.toPath != boundary.YieldToPath ||
		reentry.fromPath != boundary.ReentryFromPath || reentry.toPath != boundary.ReentryToPath {
		return LivenessUnknown
	}
	yieldFromNode, yieldFromOK := index.graph.nodeIndex[yield.from]
	reentryToNode, reentryToOK := index.graph.nodeIndex[reentry.to]
	if !yieldFromOK || !reentryToOK {
		return LivenessUnknown
	}
	priorReach := index.reach(yieldFromNode, true)
	postReach := index.reach(reentryToNode, false)
	events := index.componentEvents(owner, makeSubjectKey(subject))
	if len(events) == 0 {
		return LivenessUnknown
	}

	prior, post, unknown := false, false, false
	// observedBefore and definedAfter carry the positive proof of the negative
	// answer: nothing in the pre-yield slice touches this subject, and its
	// definition lies in the post-reentry slice. A boundary cannot observe a
	// subject whose definition it precedes.
	observedBefore, definedAfter := false, false
	for _, eventIndex := range events {
		if eventIndex < 0 || eventIndex >= len(builder.events) || eventIndex >= len(index.events) {
			return LivenessUnknown
		}
		event := builder.events[eventIndex]
		attachment := index.events[eventIndex]
		if !attachment.attached {
			// An event that cannot be attached to the owner-issued graph cannot
			// prove either side of the suspension cut.  This is deliberately
			// Unknown even when its kind is exact.
			return LivenessUnknown
		}
		inPrior := priorReach[attachment.node]
		inPost := postReach[attachment.node]
		if inPrior {
			observedBefore = true
		}
		if inPost && event.Kind == EventDefine {
			definedAfter = true
		}
		if !inPrior && !inPost {
			continue
		}
		if event.Kind == EventUnknown {
			// Unknown is absorbing over every relevant predecessor/successor
			// slice; a later exact use cannot turn an opaque effect into proof.
			unknown = true
		}
		if inPrior && event.Kind != EventUnknown {
			prior = true
		}
		if inPost && (event.Kind == EventUse || event.Kind == EventAlias) {
			post = true
		}
	}
	if !prior {
		// A subject the pre-yield slice never touches, whose definition the
		// post-reentry slice carries, does not exist at the cut. That is a
		// proof of the negative answer, not an absence of evidence, so it is
		// stated as DiesBefore rather than widened to Unknown.
		if !observedBefore && definedAfter {
			return LivenessDiesBefore
		}
		return LivenessUnknown
	}
	// An exact post-reentry use or alias is sufficient positive evidence that
	// this subject is live on this arm. An opaque event on the same reachable
	// slice may hide additional uses, but it cannot invalidate the witnessed
	// use. Unknown remains absorbing only for the negative DiesBefore proof,
	// where every relevant event must be accounted for.
	if post {
		return LivenessLive
	}
	if unknown {
		return LivenessUnknown
	}
	return LivenessDiesBefore
}

type livenessAccumulator struct {
	row    Liveness
	states []LivenessState
}

type livenessKey struct {
	route   identity.ContentID
	subject subjectKey
}

func (builder *sealBuilder) liveness() error {
	if len(builder.boundariesRows) == 0 {
		return nil
	}
	index, err := builder.buildLivenessIndex()
	if err != nil {
		return err
	}
	rows := make(map[livenessKey]*livenessAccumulator)
	for _, boundary := range builder.boundariesRows {
		subjects, subjectsOK := builder.boundarySubjects(boundary, index)
		if !subjectsOK {
			return fmt.Errorf("%w: boundary has no owner-issued subject directory", ErrMalformed)
		}
		for _, subject := range subjects {
			key := livenessKey{route: boundary.YieldRoute, subject: subjectKey{kind: subject.Kind, id: subject.ID}}
			entry := rows[key]
			if entry == nil {
				entry = &livenessAccumulator{row: Liveness{
					Call:          boundary.Call,
					YieldRoute:    boundary.YieldRoute,
					YieldFromPath: boundary.YieldFromPath,
					YieldToPath:   boundary.YieldToPath,
					Subject:       subject,
				}}
				rows[key] = entry
			}
			entry.states = append(entry.states, builder.classifyLiveness(boundary, subject, index))
		}
	}

	if err := builder.orderYieldBoundaries(); err != nil {
		return err
	}

	// The accumulators are ordered directly. The comparison key is the
	// accumulated row's own route and subject, so ordering the accumulators
	// reads each one through its pointer; ordering their map keys instead
	// would resolve the map twice per comparison and copy a row per read.
	ordered := make([]*livenessAccumulator, 0, len(rows))
	for _, entry := range rows {
		ordered = append(ordered, entry)
	}
	sort.Slice(ordered, func(left, right int) bool {
		leftRow, rightRow := &ordered[left].row, &ordered[right].row
		if leftRow.YieldRoute != rightRow.YieldRoute {
			return bytes.Compare(leftRow.YieldRoute[:], rightRow.YieldRoute[:]) < 0
		}
		if leftRow.Subject.Kind != rightRow.Subject.Kind {
			return leftRow.Subject.Kind < rightRow.Subject.Kind
		}
		return bytes.Compare(leftRow.Subject.ID[:], rightRow.Subject.ID[:]) < 0
	})
	for _, entry := range ordered {
		entry.row.State = AggregateLiveness(entry.states)
		entry.row.ID = livenessID(entry.row.YieldRoute, entry.row.YieldFromPath, entry.row.YieldToPath, entry.row.Subject)
		if !entry.row.ID.Available() {
			return fmt.Errorf("%w: liveness identity is unavailable", ErrMalformed)
		}
		builder.livenessRows = append(builder.livenessRows, entry.row)
	}
	return nil
}

// orderYieldBoundaries numbers the distinct yield routes in program order.
// The Program plane publishes liveness as ranges over this ordinal, so the
// order has to follow control flow for a run to mean anything: a body's
// routes are numbered by the ordinal of the call that yields them, and the
// bodies are laid out one after another so every body owns a contiguous
// block and a run inside a body is a run inside the sequence.
func (builder *sealBuilder) orderYieldBoundaries() error {
	type routeRow struct {
		owner keyspace.Term
		call  keyspace.Term
		row   YieldOrdinal
	}
	seen := make(map[identity.ContentID]int, len(builder.boundariesRows))
	ordered := make([]routeRow, 0, len(builder.boundariesRows))
	for _, boundary := range builder.boundariesRows {
		owner := boundaryOwner(builder, boundary)
		if owner == 0 {
			return fmt.Errorf("%w: yield boundary has no body owner", ErrMalformed)
		}
		if index, duplicate := seen[boundary.YieldRoute]; duplicate {
			// One route can pair with several re-entry arms. They are one
			// boundary in the sequence, and they must agree on where it is.
			if ordered[index].owner != owner || ordered[index].call != boundary.Call {
				return fmt.Errorf("%w: yield route %x is owned by two bodies", ErrMalformed, boundary.YieldRoute[:4])
			}
			continue
		}
		seen[boundary.YieldRoute] = len(ordered)
		ordered = append(ordered, routeRow{owner: owner, call: boundary.Call, row: YieldOrdinal{
			Call: boundary.Call, YieldRoute: boundary.YieldRoute,
			YieldFromPath: boundary.YieldFromPath, YieldToPath: boundary.YieldToPath,
		}})
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].owner != ordered[right].owner {
			return ordered[left].owner < ordered[right].owner
		}
		if ordered[left].call != ordered[right].call {
			return ordered[left].call < ordered[right].call
		}
		return bytes.Compare(ordered[left].row.YieldRoute[:], ordered[right].row.YieldRoute[:]) < 0
	})
	builder.yieldOrder = make([]YieldOrdinal, len(ordered))
	for index, entry := range ordered {
		entry.row.Ordinal = uint32(index)
		builder.yieldOrder[index] = entry.row
	}
	return nil
}
