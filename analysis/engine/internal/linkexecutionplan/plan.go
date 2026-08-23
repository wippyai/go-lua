// Package linkexecutionplan owns the first static Link execution projection.
//
// Equation Graphs remain singular: one Point/Group graph and one bytecode
// identity continue to describe the program. This package only lifts the
// graph's ordinary Point dependencies onto the compact contextfiber Layout
// and asks the sole schedule package to derive a fresh WTO over those state
// rows. It deliberately has no runtime arrays, activation overlays, or
// executor ownership.
package linkexecutionplan

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// StateEdge is one lifted dependency. A global endpoint has no
// context ordinal: its state row is singular and shared by every context.
// Mounted endpoints retain the exact context used to form the row.
type StateEdge struct {
	from, to                         contextfiber.StateOrdinal
	sourcePoint, targetPoint         contextfiber.PointOrdinal
	sourceContext, targetContext     contextfiber.ContextOrdinal
	sourceContextOK, targetContextOK bool
	transitionID, generationID       identity.ContentID
}

// ContextTransport is the Link-owned semantic transport for one
// authenticated module-call transition.  A StateEdge is enough for schedule
// reachability, but it does not tell the runtime how the source PointState is
// projected into the target Point's decision scope.  Keeping the exact
// (Context,Point) endpoints and equation reindex here prevents the executor
// from recovering a target context by module/name/path shape.
//
// The transport is deliberately still a singular graph relation: the same
// immutable Point/Program/Artifact is reused, while From/To identify the two
// mutable compact state rows that execute this occurrence.
type ContextTransport struct {
	from, to                     contextfiber.StateOrdinal
	sourcePoint, targetPoint     contextfiber.PointOrdinal
	sourceContext, targetContext contextfiber.ContextOrdinal
	transitionID, generationID   identity.ContentID
	reindex                      equation.Reindex
}

// From returns the exact source compact state row.
func (transport ContextTransport) From() contextfiber.StateOrdinal { return transport.from }

// To returns the exact target compact state row.
func (transport ContextTransport) To() contextfiber.StateOrdinal { return transport.to }

// SourcePoint returns the singular graph point at the source endpoint.
func (transport ContextTransport) SourcePoint() contextfiber.PointOrdinal {
	return transport.sourcePoint
}

// TargetPoint returns the singular graph point at the target endpoint.
func (transport ContextTransport) TargetPoint() contextfiber.PointOrdinal {
	return transport.targetPoint
}

// SourceContext returns the exact mounted source context.
func (transport ContextTransport) SourceContext() contextfiber.ContextOrdinal {
	return transport.sourceContext
}

// TargetContext returns the exact mounted target context.
func (transport ContextTransport) TargetContext() contextfiber.ContextOrdinal {
	return transport.targetContext
}

// TransitionID returns the authenticated ModuleCallTransition identity.
func (transport ContextTransport) TransitionID() identity.ContentID {
	return transport.transitionID
}

// GenerationID returns the matching target InitGeneration identity.
func (transport ContextTransport) GenerationID() identity.ContentID {
	return transport.generationID
}

// Reindex returns the exact source-to-target Point scope projection issued by
// the Link plan.  It contains only equation decisions; carrier lowering stays
// at the engine's one equation-to-carrier binding cut.
func (transport ContextTransport) Reindex() equation.Reindex { return transport.reindex }

func (transport ContextTransport) available(stateCount contextfiber.StateOrdinal, pointCount, contextCount int) bool {
	return uint64(transport.from) < uint64(stateCount) && uint64(transport.to) < uint64(stateCount) &&
		uint64(transport.sourcePoint) < uint64(pointCount) && uint64(transport.targetPoint) < uint64(pointCount) &&
		uint64(transport.sourceContext) < uint64(contextCount) && uint64(transport.targetContext) < uint64(contextCount) &&
		transport.transitionID.Available() && transport.generationID.Available() && transport.reindex.Available() &&
		transport.reindex.Source().Available() && transport.reindex.Target().Available()
}

// From returns the source compact state row.
func (edge StateEdge) From() contextfiber.StateOrdinal { return edge.from }

// To returns the target compact state row.
func (edge StateEdge) To() contextfiber.StateOrdinal { return edge.to }

// SourcePoint returns the singular graph point represented by the source row.
func (edge StateEdge) SourcePoint() contextfiber.PointOrdinal { return edge.sourcePoint }

// TargetPoint returns the singular graph point represented by the target row.
func (edge StateEdge) TargetPoint() contextfiber.PointOrdinal { return edge.targetPoint }

// SourceContext returns the mounted source context, or false for a global
// source row.
func (edge StateEdge) SourceContext() (contextfiber.ContextOrdinal, bool) {
	return edge.sourceContext, edge.sourceContextOK
}

// TargetContext returns the mounted target context, or false for a global
// target row.
func (edge StateEdge) TargetContext() (contextfiber.ContextOrdinal, bool) {
	return edge.targetContext, edge.targetContextOK
}

// TransitionID returns the authenticated module-call transition identity for
// a bound edge, or zero for an ordinary edge.
func (edge StateEdge) TransitionID() identity.ContentID {
	if !edge.transitionID.Available() {
		return identity.ContentID{}
	}
	return edge.transitionID
}

// GenerationID returns the matching initialization-generation identity for a
// bound edge, or zero for an ordinary edge.
func (edge StateEdge) GenerationID() identity.ContentID {
	if !edge.generationID.Available() {
		return identity.ContentID{}
	}
	return edge.generationID
}

func (edge StateEdge) available(stateCount contextfiber.StateOrdinal, pointCount int) bool {
	if uint64(edge.from) >= uint64(stateCount) || uint64(edge.to) >= uint64(stateCount) ||
		uint64(edge.sourcePoint) >= uint64(pointCount) || uint64(edge.targetPoint) >= uint64(pointCount) {
		return false
	}
	return edge.transitionID.Available() == edge.generationID.Available()
}

// LinkExecutionPlan is an immutable static projection over one singular
// equation Graph and one compact contextfiber Layout. State rows are globals
// followed by mounted rows in canonical context order; schedule is freshly
// recomputed over the lifted state edges.
//
// available is the projection verdict New reached over the lifted rows. The
// plan is immutable, so that verdict is decided exactly once and every accessor
// guard reads it instead of walking the rows again.
type LinkExecutionPlan struct {
	graph             *equation.Graph
	layout            contextfiber.Layout
	schedule          *schedule.Schedule
	edges             []schedule.Edge
	stateEdges        []StateEdge
	contextTransports []ContextTransport
	available         bool
}

// New constructs one static Link execution plan. The graph and bytecode stay
// singular; only ordinary graph dependencies are lifted.
//
// Every mounted-to-global dependency is refused because collapsing multiple
// context rows into one global row requires an owner-declared merge law. A
// global-to-mounted dependency fans only to contexts whose exact mounted
// owner matches the target module. Cross-module dependencies are admitted only
// through bound edges whose engine-issued ModuleCallTransition and concrete
// Point endpoints have already been authenticated; a raw transition row cannot
// prove arbitrary caller-supplied geometry.
func New(graph *equation.Graph, layout contextfiber.Layout, directory executioncontext.Directory, boundEdges []BoundEdge) (*LinkExecutionPlan, bool) {
	if !validInputs(graph, layout, directory) {
		return nil, false
	}
	pointEdges, ok := sealedPointEdges(graph)
	if !ok {
		return nil, false
	}
	pointCount := graph.PointCount()
	owners := make([]contextfiber.PointOwner, pointCount)
	for index := 0; index < pointCount; index++ {
		owner, ownerOK := layout.PointOwnerAt(contextfiber.PointOrdinal(index))
		if !ownerOK || !owner.Available() {
			return nil, false
		}
		if owner.LinkGlobal() && owner.LinkID() != directory.LinkID() {
			return nil, false
		}
		owners[index] = owner
	}

	contextModules, ok := contextShape(layout, directory)
	if !ok {
		return nil, false
	}

	stateEdges, ok := liftEdgesWithBoundEdges(graph, directory, pointEdges, owners, contextModules, layout, boundEdges)
	if !ok {
		return nil, false
	}
	contextTransports, ok := liftContextTransports(graph, layout, directory, boundEdges)
	if !ok {
		return nil, false
	}
	if uint64(layout.StateCount()) > uint64(maxInt()) {
		return nil, false
	}
	scheduleEdges := make([]schedule.Edge, len(stateEdges))
	for index, edge := range stateEdges {
		scheduleEdges[index] = schedule.Edge{From: schedule.Node(edge.from), To: schedule.Node(edge.to)}
	}
	prepared, err := schedule.Prepare(int(layout.StateCount()), scheduleEdges)
	if err != nil || prepared == nil || prepared.NodeCount() != int(layout.StateCount()) {
		return nil, false
	}
	plan := &LinkExecutionPlan{
		graph:             graph,
		layout:            layout,
		schedule:          prepared,
		edges:             append([]schedule.Edge(nil), scheduleEdges...),
		stateEdges:        append([]StateEdge(nil), stateEdges...),
		contextTransports: append([]ContextTransport(nil), contextTransports...),
	}
	plan.available = plan.completeProjection()
	return plan, plan.available
}

// Available reports whether the plan retains a complete immutable static
// projection. The verdict is sealed by New; it does not reopen graph or
// directory construction authority.
func (plan *LinkExecutionPlan) Available() bool { return plan != nil && plan.available }

func (plan *LinkExecutionPlan) completeProjection() bool {
	if plan == nil || plan.graph == nil || plan.layout.StateCount() == 0 || plan.schedule == nil ||
		plan.graph.PointCount() != plan.layout.PointCount() || plan.schedule.NodeCount() != int(plan.layout.StateCount()) ||
		len(plan.edges) != len(plan.stateEdges) {
		return false
	}
	for index, edge := range plan.stateEdges {
		if !edge.available(plan.layout.StateCount(), plan.layout.PointCount()) ||
			plan.edges[index] != (schedule.Edge{From: schedule.Node(edge.from), To: schedule.Node(edge.to)}) {
			return false
		}
	}
	for _, transport := range plan.contextTransports {
		if !transport.available(plan.layout.StateCount(), plan.layout.PointCount(), plan.layout.ContextCount()) {
			return false
		}
		source, sourceOK := plan.layout.StateAt(transport.from)
		target, targetOK := plan.layout.StateAt(transport.to)
		sourceContext, sourceContextOK := source.ContextOrdinal()
		targetContext, targetContextOK := target.ContextOrdinal()
		sourcePoint, sourcePointOK := source.PointOrdinal()
		targetPoint, targetPointOK := target.PointOrdinal()
		if !sourceOK || !targetOK || !sourceContextOK || !targetContextOK || !sourcePointOK || !targetPointOK ||
			sourceContext != transport.sourceContext || targetContext != transport.targetContext ||
			sourcePoint != transport.sourcePoint || targetPoint != transport.targetPoint {
			return false
		}
	}
	return true
}

// Graph returns the singular equation graph projected by the plan.
func (plan *LinkExecutionPlan) Graph() *equation.Graph {
	if plan == nil || !plan.Available() {
		return nil
	}
	return plan.graph
}

// Layout returns the exact compact state layout used by the plan.
func (plan *LinkExecutionPlan) Layout() contextfiber.Layout {
	if plan == nil || !plan.Available() {
		return contextfiber.Layout{}
	}
	return plan.layout
}

// Schedule returns the freshly recomputed WTO over compact state rows.
func (plan *LinkExecutionPlan) Schedule() *schedule.Schedule {
	if plan == nil || !plan.Available() {
		return nil
	}
	return plan.schedule
}

// Generation returns the exact Layout revision fence.
func (plan *LinkExecutionPlan) Generation() identity.Generation {
	if plan == nil || !plan.Available() {
		return 0
	}
	return plan.layout.Generation()
}

// StateCount reports globals plus mounted context rows, never the Cartesian
// context-by-point product.
func (plan *LinkExecutionPlan) StateCount() contextfiber.StateOrdinal {
	if plan == nil || !plan.Available() {
		return 0
	}
	return plan.layout.StateCount()
}

// ContextCount reports the canonical Directory context cardinality retained
// by the compact layout.
func (plan *LinkExecutionPlan) ContextCount() int {
	if plan == nil || !plan.Available() {
		return 0
	}
	return plan.layout.ContextCount()
}

// PointCount reports the singular equation Point shape lifted by the plan.
func (plan *LinkExecutionPlan) PointCount() int {
	if plan == nil || !plan.Available() {
		return 0
	}
	return plan.layout.PointCount()
}

// StateAt performs the inverse compact mapping needed by later state-indexed
// adapters. Global cells intentionally return no fabricated context.
func (plan *LinkExecutionPlan) StateAt(state contextfiber.StateOrdinal) (contextfiber.StateCell, bool) {
	if plan == nil || !plan.Available() {
		return contextfiber.StateCell{}, false
	}
	cell, ok := plan.layout.StateAt(state)
	return cell, ok && cell.OwnedBy(plan.layout)
}

// Lookup maps an exact context/point coordinate to one compact state row.
func (plan *LinkExecutionPlan) Lookup(context contextfiber.ContextOrdinal, point contextfiber.PointOrdinal) (contextfiber.StateOrdinal, bool) {
	if plan == nil || !plan.Available() {
		return 0, false
	}
	return plan.layout.Lookup(context, point)
}

// PointState maps a graph-owned Point handle to its compact state row. It is
// the graph-aware adapter seam; callers need not retain Graph dense ordinals.
func (plan *LinkExecutionPlan) PointState(context contextfiber.ContextOrdinal, point equation.Point) (contextfiber.StateOrdinal, bool) {
	if plan == nil || !plan.Available() || !plan.graph.OwnsPoint(point) {
		return 0, false
	}
	index, ok := plan.graph.PointIndex(point)
	if !ok {
		return 0, false
	}
	return plan.layout.Lookup(context, contextfiber.PointOrdinal(index))
}

// EdgeCount reports the lifted schedule-edge count.
func (plan *LinkExecutionPlan) EdgeCount() int {
	if plan == nil || !plan.Available() {
		return 0
	}
	return len(plan.edges)
}

// EdgeAt returns one lifted schedule edge in deterministic source/target
// order.
func (plan *LinkExecutionPlan) EdgeAt(index int) (schedule.Edge, bool) {
	if plan == nil || !plan.Available() || index < 0 || index >= len(plan.edges) {
		return schedule.Edge{}, false
	}
	return plan.edges[index], true
}

// Edges returns a detached copy of all lifted schedule edges in deterministic
// source/target order.
func (plan *LinkExecutionPlan) Edges() []schedule.Edge {
	if plan == nil || !plan.Available() {
		return nil
	}
	return append([]schedule.Edge(nil), plan.edges...)
}

// StateEdgeCount reports the richer lifted-edge mapping count.
func (plan *LinkExecutionPlan) StateEdgeCount() int {
	if plan == nil || !plan.Available() {
		return 0
	}
	return len(plan.stateEdges)
}

// StateEdgeAt returns one lifted edge with graph-point and context mapping
// metadata for state-indexed adapters.
func (plan *LinkExecutionPlan) StateEdgeAt(index int) (StateEdge, bool) {
	if plan == nil || !plan.Available() || index < 0 || index >= len(plan.stateEdges) {
		return StateEdge{}, false
	}
	return plan.stateEdges[index], true
}

// StateEdges returns a detached copy of the richer lifted-edge mapping.
func (plan *LinkExecutionPlan) StateEdges() []StateEdge {
	if plan == nil || !plan.Available() {
		return nil
	}
	return append([]StateEdge(nil), plan.stateEdges...)
}

// ContextTransportCount reports the number of authenticated cross-context
// semantic transports retained by this Link plan.
func (plan *LinkExecutionPlan) ContextTransportCount() int {
	if plan == nil || !plan.Available() {
		return 0
	}
	return len(plan.contextTransports)
}

// ContextTransportAt returns one exact Link-owned semantic transport.
func (plan *LinkExecutionPlan) ContextTransportAt(index int) (ContextTransport, bool) {
	if plan == nil || !plan.Available() || index < 0 || index >= len(plan.contextTransports) {
		return ContextTransport{}, false
	}
	return plan.contextTransports[index], true
}

// ContextTransports returns a detached copy of all contextual transports in
// deterministic source/target order.
func (plan *LinkExecutionPlan) ContextTransports() []ContextTransport {
	if plan == nil || !plan.Available() {
		return nil
	}
	return append([]ContextTransport(nil), plan.contextTransports...)
}

type pointPair struct {
	from, to contextfiber.PointOrdinal
}

func validInputs(graph *equation.Graph, layout contextfiber.Layout, directory executioncontext.Directory) bool {
	return graph != nil && graph.PointCount() > 0 && graph.Schedule() != nil &&
		graph.Schedule().NodeCount() == graph.PointCount() && layout.Available() && directory.Available() &&
		layout.Graph() == graph && layout.ContextCount() == directory.ContextCount() &&
		layout.PointCount() == graph.PointCount() && layout.Generation().Available()
}

func contextShape(layout contextfiber.Layout, directory executioncontext.Directory) ([]identity.ContentID, bool) {
	count := layout.ContextCount()
	modules := make([]identity.ContentID, count)
	for index := 0; index < count; index++ {
		ordinal := contextfiber.ContextOrdinal(index)
		id, idOK := layout.ContextID(ordinal)
		row, rowOK := directory.ContextAt(index)
		module, moduleOK := layout.ContextModuleKey(ordinal)
		if !idOK || !rowOK || !row.Available() || row.ID() != id || row.LinkID() != directory.LinkID() || !moduleOK || module != row.ModuleKey() {
			return nil, false
		}
		modules[index] = module
	}
	return modules, true
}

// sealedPointEdges reads the exact Point influence relation retained by the
// sealed equation Graph. Group and transport incidences are deliberately not
// re-derived here: local same-Point carrier closure is not a schedule edge,
// and replaying those rows would invent a contextual recurrence.
func sealedPointEdges(graph *equation.Graph) ([]pointPair, bool) {
	if graph == nil || graph.PointCount() == 0 || graph.Schedule() == nil {
		return nil, false
	}
	edges := make([]pointPair, graph.InfluenceEdgeCount())
	seen := make(map[pointPair]struct{}, len(edges))
	for index := range edges {
		edge, edgeOK := graph.InfluenceEdgeAt(index)
		if !edgeOK || edge.From < 0 || edge.To < 0 || int(edge.From) >= graph.PointCount() || int(edge.To) >= graph.PointCount() {
			return nil, false
		}
		pair := pointPair{from: contextfiber.PointOrdinal(edge.From), to: contextfiber.PointOrdinal(edge.To)}
		if _, duplicate := seen[pair]; duplicate {
			return nil, false
		}
		seen[pair] = struct{}{}
		edges[index] = pair
	}
	return edges, true
}

// liftEdgesWithBoundEdges lifts ordinary graph dependencies and joins
// cross-module dependencies to their already-authenticated module-call
// witnesses.  Bound witnesses are also added when no ordinary graph edge has
// the same point pair: the static schedule must retain context-only edges so
// a cross-context cycle cannot disappear during projection.
func liftEdgesWithBoundEdges(graph *equation.Graph, directory executioncontext.Directory, pointEdges []pointPair, owners []contextfiber.PointOwner, contextModules []identity.ContentID, layout contextfiber.Layout, boundEdges []BoundEdge) ([]StateEdge, bool) {
	if !validInputs(graph, layout, directory) || len(owners) != layout.PointCount() || len(contextModules) != layout.ContextCount() {
		return nil, false
	}
	boundByPair := make(map[pointPair][]StateEdge, len(boundEdges))
	if len(boundEdges) != 0 {
		for _, bound := range boundEdges {
			if !bound.boundEdgeBelongsTo(graph, layout, directory) {
				return nil, false
			}
			pair := bound.pointPair()
			state := bound.stateEdge()
			boundByPair[pair] = append(boundByPair[pair], state)
		}
	}

	stateAt := make(map[schedule.Edge]StateEdge)
	add := func(sourcePoint, targetPoint contextfiber.PointOrdinal, sourceContext, targetContext contextfiber.ContextOrdinal, sourceContextOK, targetContextOK bool) bool {
		from, fromOK := layout.Lookup(sourceContext, sourcePoint)
		to, toOK := layout.Lookup(targetContext, targetPoint)
		if !fromOK || !toOK {
			return false
		}
		edge := StateEdge{from: from, to: to, sourcePoint: sourcePoint, targetPoint: targetPoint, sourceContext: sourceContext, targetContext: targetContext, sourceContextOK: sourceContextOK, targetContextOK: targetContextOK}
		if !edge.available(layout.StateCount(), layout.PointCount()) {
			return false
		}
		key := schedule.Edge{From: schedule.Node(from), To: schedule.Node(to)}
		if existing, present := stateAt[key]; present {
			return existing.sourcePoint == edge.sourcePoint && existing.targetPoint == edge.targetPoint &&
				existing.sourceContext == edge.sourceContext && existing.targetContext == edge.targetContext &&
				existing.sourceContextOK == edge.sourceContextOK && existing.targetContextOK == edge.targetContextOK &&
				existing.transitionID == edge.transitionID && existing.generationID == edge.generationID
		}
		stateAt[key] = edge
		return true
	}
	for _, pointEdge := range pointEdges {
		if uint64(pointEdge.from) >= uint64(len(owners)) || uint64(pointEdge.to) >= uint64(len(owners)) {
			return nil, false
		}
		fromOwner, toOwner := owners[pointEdge.from], owners[pointEdge.to]
		switch {
		case fromOwner.LinkGlobal() && toOwner.LinkGlobal():
			if !add(pointEdge.from, pointEdge.to, 0, 0, false, false) {
				return nil, false
			}
		case fromOwner.LinkGlobal() && toOwner.Mounted():
			for contextIndex, module := range contextModules {
				if module != toOwner.ModuleKey() {
					continue
				}
				context := contextfiber.ContextOrdinal(contextIndex)
				if !add(pointEdge.from, pointEdge.to, context, context, false, true) {
					return nil, false
				}
			}
		case fromOwner.Mounted() && toOwner.LinkGlobal():
			// A mounted source exists once per context. Collapsing those rows
			// into one Link-global target is semantic aggregation, not a
			// schedule lift; no merge law is part of this static vertical.
			return nil, false
		case fromOwner.Mounted() && toOwner.Mounted() && fromOwner.ModuleKey() == toOwner.ModuleKey():
			for contextIndex, module := range contextModules {
				if module != fromOwner.ModuleKey() {
					continue
				}
				context := contextfiber.ContextOrdinal(contextIndex)
				if !add(pointEdge.from, pointEdge.to, context, context, true, true) {
					return nil, false
				}
			}
		case fromOwner.Mounted() && toOwner.Mounted():
			// A cross-module graph edge is admissible only when the exact
			// graph-point pair has an authenticated transition witness.  The
			// witness supplies the source and target context rows; no context
			// is inferred from module shape or ordinal position here.
			bound, present := boundByPair[pointEdge]
			if !present || len(bound) == 0 {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	for _, bound := range boundByPair {
		for _, edge := range bound {
			if !edge.available(layout.StateCount(), layout.PointCount()) {
				return nil, false
			}
			key := schedule.Edge{From: schedule.Node(edge.from), To: schedule.Node(edge.to)}
			if existing, present := stateAt[key]; present {
				if existing != edge {
					return nil, false
				}
				continue
			}
			stateAt[key] = edge
		}
	}
	result := make([]StateEdge, 0, len(stateAt))
	for _, edge := range stateAt {
		result = append(result, edge)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].from != result[right].from {
			return result[left].from < result[right].from
		}
		return result[left].to < result[right].to
	})
	return result, true
}

// liftContextTransports projects each already-authenticated BoundEdge into a
// semantic PointState transport.  The source and target scopes are read from
// the graph-owned Point sites; no module key, path, name, or context ordinal
// inference is permitted here.  The target decision with the same semantic
// identity is retained (or renamed when the target site issued a distinct
// equation decision), and every source decision absent at the target is
// explicitly forgotten.
func liftContextTransports(graph *equation.Graph, layout contextfiber.Layout, directory executioncontext.Directory, boundEdges []BoundEdge) ([]ContextTransport, bool) {
	if !validInputs(graph, layout, directory) {
		return nil, false
	}
	transports := make([]ContextTransport, 0, len(boundEdges))
	seen := make(map[[2]contextfiber.StateOrdinal]struct{}, len(boundEdges))
	for _, bound := range boundEdges {
		if !bound.boundEdgeBelongsTo(graph, layout, directory) {
			return nil, false
		}
		source, target := bound.SourcePoint(), bound.TargetPoint()
		// A bound pair owns one semantic PointState carrier.  Environment and
		// Factor rows already carry their own equation reindex and runtime
		// admission; accepting the same pair here would give two independent
		// authorities for one source/target transport.  Refuse the duplicate at
		// the immutable Link cut rather than letting the runtime fold it twice.
		if graphHasPointTransport(graph, source, target) {
			return nil, false
		}
		reindex, ok := contextualPointReindex(source, target)
		if !ok {
			return nil, false
		}
		key := [2]contextfiber.StateOrdinal{bound.From(), bound.To()}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		sourceContext, sourceContextOK := bound.SourceContext()
		targetContext, targetContextOK := bound.TargetContext()
		if !sourceContextOK || !targetContextOK {
			return nil, false
		}
		transports = append(transports, ContextTransport{
			from: bound.From(), to: bound.To(),
			sourcePoint:   contextfiber.PointOrdinal(mustGraphPointIndex(graph, source)),
			targetPoint:   contextfiber.PointOrdinal(mustGraphPointIndex(graph, target)),
			sourceContext: sourceContext, targetContext: targetContext,
			transitionID: bound.TransitionID(), generationID: bound.GenerationID(),
			reindex: reindex,
		})
	}
	sort.Slice(transports, func(left, right int) bool {
		if transports[left].from != transports[right].from {
			return transports[left].from < transports[right].from
		}
		if transports[left].to != transports[right].to {
			return transports[left].to < transports[right].to
		}
		if transports[left].sourcePoint != transports[right].sourcePoint {
			return transports[left].sourcePoint < transports[right].sourcePoint
		}
		return transports[left].targetPoint < transports[right].targetPoint
	})
	return transports, true
}

func graphHasPointTransport(graph *equation.Graph, source, target equation.Point) bool {
	if graph == nil || !graph.OwnsPoint(source) || !graph.OwnsPoint(target) {
		return false
	}
	sourceIndex, sourceOK := graph.PointIndex(source)
	targetIndex, targetOK := graph.PointIndex(target)
	if !sourceOK || !targetOK {
		return false
	}
	// A Group input is already a semantic PointState transport into the
	// Group's output. It is not represented by EnvironmentEdgeTotal or
	// FactorEdgeTotal, so omitting it here would let one exact pair be admitted
	// once as a Group producer and once as a Link ContextTransport. Include
	// both ordinary conjunctive inputs and the Group's separate environment
	// boundary: both carry an exact source Point and publish to the same output
	// Point.
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		if !groupOK {
			continue
		}
		outputIndex, outputOK := graph.PointIndex(group.Output())
		if !outputOK || outputIndex != targetIndex {
			continue
		}
		for inputIndex := 0; inputIndex < group.InputCount(); inputIndex++ {
			input, inputOK := group.InputAt(inputIndex)
			inputPoint, inputPointOK := graph.PointIndex(input.Point())
			if inputOK && inputPointOK && inputPoint == sourceIndex {
				return true
			}
		}
		if input, inputOK := group.EnvironmentInput(); inputOK {
			inputPoint, inputPointOK := graph.PointIndex(input.Point())
			if inputPointOK && inputPoint == sourceIndex {
				return true
			}
		}
	}
	for index := 0; index < graph.EnvironmentEdgeTotal(); index++ {
		edge, edgeOK := graph.EnvironmentEdgeAtIndex(index)
		inputIndex, inputOK := graph.PointIndex(edge.Input().Point())
		edgeTarget, targetOK := graph.PointIndex(edge.Target())
		if !edgeOK || !inputOK || !targetOK {
			continue
		}
		if inputIndex == sourceIndex && edgeTarget == targetIndex {
			return true
		}
	}
	for index := 0; index < graph.FactorEdgeTotal(); index++ {
		edge, edgeOK := graph.FactorEdgeAtIndex(index)
		inputIndex, inputOK := graph.PointIndex(edge.Input().Point())
		edgeTarget, targetOK := graph.PointIndex(edge.Target())
		if !edgeOK || !inputOK || !targetOK {
			continue
		}
		if inputIndex == sourceIndex && edgeTarget == targetIndex {
			return true
		}
	}
	return false
}

func contextualPointReindex(source, target equation.Point) (equation.Reindex, bool) {
	if !source.Available() || !target.Available() || !source.Scope().Available() || !target.Scope().Available() {
		return equation.Reindex{}, false
	}
	targetBySemantic := make(map[composition.Key]equation.Decision, target.Scope().Count())
	for index := 0; index < target.Scope().Count(); index++ {
		decision, ok := target.Scope().At(index)
		if !ok || !decision.Available() {
			return equation.Reindex{}, false
		}
		if _, duplicate := targetBySemantic[decision.Key()]; duplicate {
			return equation.Reindex{}, false
		}
		targetBySemantic[decision.Key()] = decision
	}
	maps := make([]equation.DecisionMap, source.Scope().Count())
	for index := 0; index < source.Scope().Count(); index++ {
		decision, ok := source.Scope().At(index)
		if !ok || !decision.Available() {
			return equation.Reindex{}, false
		}
		targetDecision, retained := targetBySemantic[decision.Key()]
		switch {
		case !retained:
			maps[index] = equation.Forget(decision)
		case targetDecision == decision:
			maps[index] = equation.Identity(decision)
		default:
			maps[index] = equation.Rename(decision, targetDecision)
		}
	}
	return equation.NewReindex(source.Scope(), target.Scope(), maps)
}

func mustGraphPointIndex(graph *equation.Graph, point equation.Point) int {
	if graph == nil {
		return -1
	}
	index, ok := graph.PointIndex(point)
	if !ok {
		return -1
	}
	return index
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
