package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// bindRuntimeContextTransports lowers only the immutable ContextTransport
// rows issued by the LinkExecutionPlan. Their source/target StateOrdinal pair
// is retained verbatim and indexed directly; there is deliberately no lookup
// by graph Point, module key, actor name, or global fallback.
func bindRuntimeContextTransports(graph *equation.Graph, plan *linkexecutionplan.LinkExecutionPlan, runtime *carrier.Composition, reindexes runtimeReindexes) ([]runtimeContextTransport, [][]int, [][]int, map[[2]int]int, bool) {
	if graph == nil || plan == nil || !plan.Available() || runtime == nil || runtime.Guards() == nil || reindexes.scopes == nil || reindexes.decisions == nil {
		return nil, nil, nil, nil, false
	}
	stateCount := int(plan.StateCount())
	if stateCount <= 0 {
		return nil, nil, nil, nil, false
	}
	// ContextTransport carries only the authenticated point relation and its
	// equation reindex; BoundEdge has no separate activation premise or
	// postcondition.  The immutable edge law is therefore unconditional True
	// on both sides of the carrier transport.  This is not a replay-wide mask:
	// the exact StateOrdinal row remains the sole admission address.
	pre, preOK := support.True(runtime.Guards())
	if !preOK {
		return nil, nil, nil, nil, false
	}
	post := pre
	transports := make([]runtimeContextTransport, 0, plan.ContextTransportCount())
	incoming := make([][]int, stateCount)
	outgoing := make([][]int, stateCount)
	sourceAt := make(map[[2]int]int, plan.ContextTransportCount())
	seen := make(map[[2]int]struct{}, plan.ContextTransportCount())
	for index := 0; index < plan.ContextTransportCount(); index++ {
		bound, boundOK := plan.ContextTransportAt(index)
		if !boundOK {
			return nil, nil, nil, nil, false
		}
		from, to := int(bound.From()), int(bound.To())
		sourcePoint, targetPoint := int(bound.SourcePoint()), int(bound.TargetPoint())
		if from < 0 || to < 0 || from >= stateCount || to >= stateCount || sourcePoint < 0 || targetPoint < 0 ||
			sourcePoint >= graph.PointCount() || targetPoint >= graph.PointCount() {
			return nil, nil, nil, nil, false
		}
		pair := [2]int{from, to}
		if _, duplicate := seen[pair]; duplicate {
			return nil, nil, nil, nil, false
		}
		seen[pair] = struct{}{}
		source, sourceOK := graph.PointAt(schedule.Node(sourcePoint))
		target, targetOK := graph.PointAt(schedule.Node(targetPoint))
		if !sourceOK || !targetOK || !graph.OwnsPoint(source) || !graph.OwnsPoint(target) {
			return nil, nil, nil, nil, false
		}
		sourceContext := bound.SourceContext()
		targetContext := bound.TargetContext()
		sourceCell, sourceCellOK := plan.StateAt(bound.From())
		targetCell, targetCellOK := plan.StateAt(bound.To())
		sourceCellContext, sourceCellContextOK := sourceCell.ContextOrdinal()
		targetCellContext, targetCellContextOK := targetCell.ContextOrdinal()
		sourceCellPoint, sourceCellPointOK := sourceCell.PointOrdinal()
		targetCellPoint, targetCellPointOK := targetCell.PointOrdinal()
		if !sourceCellOK || !targetCellOK || !sourceCellContextOK || !targetCellContextOK ||
			!sourceCellPointOK || !targetCellPointOK || sourceCellContext != sourceContext || targetCellContext != targetContext ||
			int(sourceCellPoint) != sourcePoint || int(targetCellPoint) != targetPoint {
			return nil, nil, nil, nil, false
		}
		inverseKey := [2]int{to, sourcePoint}
		if prior, duplicate := sourceAt[inverseKey]; duplicate && prior != from {
			return nil, nil, nil, nil, false
		}
		if _, duplicate := sourceAt[inverseKey]; duplicate {
			// The StateOrdinal pair was checked above, but retain one inverse row
			// only: two transport authorities for the same target/source key are
			// ambiguous even when their metadata happens to repeat.
			return nil, nil, nil, nil, false
		}
		// ContextTransport's equation relation is already authenticated by the
		// Link plan. Lower it exactly once at this cut against the graph-issued
		// carrier scopes; a cached Group/Environment/Factor plan is a different
		// structural authority and must not become a fallback for this edge.
		reindex, reindexOK := lowerRuntimeReindex(runtime, bound.Reindex(), reindexes.scopes, reindexes.decisions)
		if !reindexOK || !reindex.Valid() || reindex.CoordinateCount() != source.Scope().Count() {
			return nil, nil, nil, nil, false
		}
		transport := runtimeContextTransport{from: from, to: to, sourcePoint: sourcePoint, targetPoint: targetPoint, sourceContext: sourceContext, targetContext: targetContext, plan: reindex, pre: pre, post: post}
		if !transport.validFor(runtime, stateCount, graph.PointCount()) {
			return nil, nil, nil, nil, false
		}
		transportIndex := len(transports)
		transports = append(transports, transport)
		incoming[to] = append(incoming[to], transportIndex)
		outgoing[from] = append(outgoing[from], transportIndex)
		sourceAt[inverseKey] = from
	}
	return transports, incoming, outgoing, sourceAt, true
}

// contextTransportSourceState performs the one exact inverse lookup used by
// source selection. A missing row is refusal; it never falls back to the
// current target context or a graph-global row.
func (runtime *solverRuntime) contextTransportSourceState(targetState, sourcePoint int) (int, bool) {
	if runtime == nil || !runtime.artifactBacked || targetState < 0 || sourcePoint < 0 || runtime.contextTransportSource == nil {
		return 0, false
	}
	source, ok := runtime.contextTransportSource[[2]int{targetState, sourcePoint}]
	if !ok || source < 0 || source >= runtime.stateCount() {
		return 0, false
	}
	return source, true
}

// buildStatePointRows is the immutable inverse of LinkExecutionPlan.StateAt.
// It records only admitted StateOrdinal occurrences for each singular graph
// Point, so a mounted point can be resolved in every owning context without
// allocating a dense Context×Point table.
func buildStatePointRows(graph *equation.Graph, plan interface {
	StateCount() contextfiber.StateOrdinal
	StateAt(contextfiber.StateOrdinal) (contextfiber.StateCell, bool)
}, artifact bool) ([][]int, bool) {
	if graph == nil || graph.PointCount() == 0 {
		return nil, false
	}
	rows := make([][]int, graph.PointCount())
	if !artifact {
		for point := 0; point < graph.PointCount(); point++ {
			rows[point] = []int{point}
		}
		return rows, true
	}
	if plan == nil {
		return nil, false
	}
	total := uint64(plan.StateCount())
	if total == 0 {
		return nil, false
	}
	for state := contextfiber.StateOrdinal(0); uint64(state) < total; state++ {
		cell, ok := plan.StateAt(state)
		point, pointOK := cell.PointOrdinal()
		if !ok || !pointOK || uint64(point) >= uint64(graph.PointCount()) {
			return nil, false
		}
		rows[int(point)] = append(rows[int(point)], int(state))
	}
	for point := range rows {
		if len(rows[point]) == 0 {
			return nil, false
		}
		sort.Ints(rows[point])
	}
	return rows, true
}

// buildArtifactStateDemand lifts the ordinary query/observation demand roots
// onto the immutable Link plan.  A graph Point is not a demand root in the
// mounted runtime: its query/observation row carries the exact StateOrdinal
// that names the actor context.  The reverse closure follows the plan's
// contextual dependency rows, and WTO regions are expanded on the contextual
// schedule before the closure is repeated.  This keeps two equal graph points
// in different contexts from sharing one active bit while retaining one row
// for Link-global points.
func buildArtifactStateDemand(graph *equation.Graph, program *runtimeProgram, plan interface {
	StateCount() contextfiber.StateOrdinal
	StateAt(contextfiber.StateOrdinal) (contextfiber.StateCell, bool)
	Edges() []schedule.Edge
	Schedule() *schedule.Schedule
}, graphActive []bool) ([]bool, []bool, []int, bool) {
	if graph == nil || program == nil || !program.valid() || len(graphActive) != graph.PointCount() || plan == nil || plan.StateCount() == 0 {
		return nil, nil, nil, false
	}
	execution := plan.Schedule()
	if execution == nil || execution.NodeCount() != int(plan.StateCount()) || execution.EventCount() == 0 {
		return nil, nil, nil, false
	}
	stateCount := int(plan.StateCount())
	active := make([]bool, stateCount)
	roots := make([]int, 0, len(program.queryTable)+len(program.observationTable))
	addRoot := func(state contextfiber.StateOrdinal) bool {
		if uint64(state) >= uint64(stateCount) {
			return false
		}
		cell, ok := plan.StateAt(state)
		point, pointOK := cell.PointOrdinal()
		if !ok || !pointOK || uint64(point) >= uint64(len(graphActive)) || !graphActive[point] {
			return false
		}
		if !active[state] {
			active[state] = true
			roots = append(roots, int(state))
		}
		return true
	}
	for _, row := range program.queryTable {
		if !addRoot(row.state) {
			return nil, nil, nil, false
		}
	}
	for _, row := range program.observationTable {
		if !addRoot(row.state) {
			return nil, nil, nil, false
		}
	}
	if len(roots) == 0 {
		return nil, nil, nil, false
	}
	reverse := make([][]int, stateCount)
	for _, edge := range plan.Edges() {
		from, to := int(edge.From), int(edge.To)
		if from < 0 || from >= stateCount || to < 0 || to >= stateCount {
			return nil, nil, nil, false
		}
		reverse[to] = append(reverse[to], from)
	}
	for index := range reverse {
		sort.Ints(reverse[index])
	}
	// Add a state and its contextual predecessors. The returned bool makes
	// malformed plan rows fail at assembly rather than becoming an empty solve.
	closePredecessors := func() bool {
		stack := append([]int(nil), roots...)
		for len(stack) != 0 {
			last := len(stack) - 1
			state := stack[last]
			stack = stack[:last]
			for _, predecessor := range reverse[state] {
				if !active[predecessor] {
					active[predecessor] = true
					stack = append(stack, predecessor)
				}
			}
		}
		return true
	}
	// A reached contextual WTO region owns its complete state interval. Any
	// newly admitted interval may expose further predecessors, so iterate until
	// both the region expansion and reverse closure reach a fixed point.
	for {
		before := 0
		for _, admitted := range active {
			if admitted {
				before++
			}
		}
		for regionIndex := 0; regionIndex < execution.RegionCount(); regionIndex++ {
			region, ok := execution.RegionAt(regionIndex)
			if !ok || region.Enter < 0 || region.Exit < region.Enter || region.Exit >= execution.EventCount() {
				return nil, nil, nil, false
			}
			reached := false
			for eventIndex := region.Enter; eventIndex <= region.Exit; eventIndex++ {
				event, eventOK := execution.EventAt(eventIndex)
				if !eventOK {
					return nil, nil, nil, false
				}
				if event.Kind == schedule.EventNode && int(event.Node) >= 0 && int(event.Node) < stateCount && active[event.Node] {
					reached = true
				}
			}
			if !reached {
				continue
			}
			for eventIndex := region.Enter; eventIndex <= region.Exit; eventIndex++ {
				event, eventOK := execution.EventAt(eventIndex)
				if !eventOK {
					return nil, nil, nil, false
				}
				if event.Kind == schedule.EventNode && int(event.Node) >= 0 && int(event.Node) < stateCount {
					active[event.Node] = true
				}
			}
		}
		roots = roots[:0]
		for state, admitted := range active {
			if admitted {
				roots = append(roots, state)
			}
		}
		if !closePredecessors() {
			return nil, nil, nil, false
		}
		after := 0
		for _, admitted := range active {
			if admitted {
				after++
			}
		}
		if after == before {
			break
		}
	}
	selected := make([]int, 0)
	activePoints := make([]bool, len(graphActive))
	for state, admitted := range active {
		if !admitted {
			continue
		}
		cell, ok := plan.StateAt(contextfiber.StateOrdinal(state))
		point, pointOK := cell.PointOrdinal()
		if !ok || !pointOK || uint64(point) >= uint64(len(activePoints)) {
			return nil, nil, nil, false
		}
		activePoints[point] = true
		selected = append(selected, state)
	}
	if len(selected) == 0 {
		return nil, nil, nil, false
	}
	return active, activePoints, selected, true
}

// buildStateFactorIndex lifts singular graph factor rows onto the exact
// compact state rows. Mounted-to-global transport is deliberately refused:
// without an owner-issued merge law it would collapse distinct contexts into
// one mutable target. Global-to-mounted rows fan to every eligible target
// context, and mounted-to-mounted rows stay in their one exact context.
func buildStateFactorIndex(graph *equation.Graph, plan interface {
	StateCount() contextfiber.StateOrdinal
	StateAt(contextfiber.StateOrdinal) (contextfiber.StateCell, bool)
	Lookup(contextfiber.ContextOrdinal, contextfiber.PointOrdinal) (contextfiber.StateOrdinal, bool)
}, factors []runtimeFactorEdge, artifact bool) ([][]int, [][]int, []runtimeStateFactorRow, [][]int, bool) {
	if graph == nil || len(factors) < graph.FactorEdgeTotal() {
		return nil, nil, nil, nil, false
	}
	if !artifact {
		return nil, nil, nil, nil, true
	}
	stateRows, ok := buildStatePointRows(graph, plan, true)
	if !ok || plan == nil {
		return nil, nil, nil, nil, false
	}
	stateCount := int(plan.StateCount())
	if stateCount == 0 {
		return nil, nil, nil, nil, false
	}
	incoming := make([][]int, stateCount)
	outgoing := make([][]int, stateCount)
	rows := make([]runtimeStateFactorRow, 0)
	add := func(edgeIndex, source, target int) bool {
		if source < 0 || source >= stateCount || target < 0 || target >= stateCount {
			return false
		}
		row := len(rows)
		rows = append(rows, runtimeStateFactorRow{edge: edgeIndex, source: source, target: target})
		outgoing[source] = append(outgoing[source], row)
		incoming[target] = append(incoming[target], row)
		return true
	}
	for edgeIndex, edge := range factors {
		if edge.source < 0 || edge.target < 0 || edge.source >= len(stateRows) || edge.target >= len(stateRows) || !edge.key.Available() {
			return nil, nil, nil, nil, false
		}
		if edge.context.Available() {
			sourceState, sourceStateOK := plan.Lookup(edge.fromContext, contextfiber.PointOrdinal(edge.source))
			targetState, targetStateOK := plan.Lookup(edge.toContext, contextfiber.PointOrdinal(edge.target))
			if !sourceStateOK || !targetStateOK || !add(edgeIndex, int(sourceState), int(targetState)) {
				return nil, nil, nil, nil, false
			}
			continue
		}
		if !edge.context.WellFormed() {
			return nil, nil, nil, nil, false
		}
		fromOwner, fromOK := stateRowOwner(plan, stateRows[edge.source])
		toOwner, toOK := stateRowOwner(plan, stateRows[edge.target])
		if !fromOK || !toOK {
			return nil, nil, nil, nil, false
		}
		switch {
		case fromOwner.LinkGlobal() && toOwner.LinkGlobal():
			if len(stateRows[edge.source]) != 1 || len(stateRows[edge.target]) != 1 || !add(edgeIndex, stateRows[edge.source][0], stateRows[edge.target][0]) {
				return nil, nil, nil, nil, false
			}
		case fromOwner.LinkGlobal() && toOwner.Mounted():
			for _, targetState := range stateRows[edge.target] {
				if !add(edgeIndex, stateRows[edge.source][0], targetState) {
					return nil, nil, nil, nil, false
				}
			}
		case fromOwner.Mounted() && toOwner.Mounted():
			for _, sourceState := range stateRows[edge.source] {
				_, _, sourceContext, sourceOK := stateCellContext(plan, sourceState)
				if !sourceOK {
					return nil, nil, nil, nil, false
				}
				targetState, targetOK := planLookup(plan, sourceContext, contextfiber.PointOrdinal(edge.target))
				if !targetOK || !add(edgeIndex, sourceState, targetState) {
					return nil, nil, nil, nil, false
				}
			}
		case fromOwner.Mounted() && toOwner.LinkGlobal():
			return nil, nil, nil, nil, false
		default:
			return nil, nil, nil, nil, false
		}
	}
	return incoming, outgoing, rows, stateRows, true
}

// The small plan adapters below keep the state-index builders independent of
// the concrete LinkExecutionPlan type while retaining exact Layout ownership.

// stateRowOwner reads the authenticated point owner out of a point's admitted
// compact state rows. buildStatePointRows sorts the occurrences of one graph
// point, so the first row is the lowest state ordinal carrying that point and
// its cell publishes exactly the layout's point owner. A point with no admitted
// row has no owner and is refused.
func stateRowOwner(plan interface {
	StateAt(contextfiber.StateOrdinal) (contextfiber.StateCell, bool)
}, rows []int) (contextfiber.PointOwner, bool) {
	if len(rows) == 0 || rows[0] < 0 {
		return contextfiber.PointOwner{}, false
	}
	cell, cellOK := plan.StateAt(contextfiber.StateOrdinal(rows[0]))
	if !cellOK {
		return contextfiber.PointOwner{}, false
	}
	if _, pointOK := cell.PointOrdinal(); !pointOK {
		return contextfiber.PointOwner{}, false
	}
	return cell.Owner(), true
}

func stateCellContext(plan interface {
	StateAt(contextfiber.StateOrdinal) (contextfiber.StateCell, bool)
}, state int) (contextfiber.StateCell, bool, contextfiber.ContextOrdinal, bool) {
	cell, ok := plan.StateAt(contextfiber.StateOrdinal(state))
	if !ok {
		return contextfiber.StateCell{}, false, 0, false
	}
	context, contextOK := cell.ContextOrdinal()
	return cell, true, context, contextOK
}

func planLookup(plan interface {
	Lookup(contextfiber.ContextOrdinal, contextfiber.PointOrdinal) (contextfiber.StateOrdinal, bool)
}, context contextfiber.ContextOrdinal, point contextfiber.PointOrdinal) (int, bool) {
	state, ok := plan.Lookup(context, point)
	return int(state), ok
}

// stateGroupRow is one admitted executable producer coordinate.  The graph
// Group remains singular metadata; this row is the compact (StateOrdinal,
// GroupOrdinal) occurrence used by an epoch-local candidate cache.  A row is
// admitted only when the StateOrdinal's underlying graph point owns the Group
// output, so no StateCount x GroupCount product is materialized.
type stateGroupRow struct {
	state contextfiber.StateOrdinal
	group int
}

type stateGroupKey struct {
	state contextfiber.StateOrdinal
	group int
}

// stateGroupIndex is immutable runtime metadata for the compact producer
// rows.  The map is an index over admitted rows, not a dense Cartesian table.
// sealed is the row/key agreement verdict reached where the index is built, so
// an executor row lookup reads it instead of re-walking every admitted row.
type stateGroupIndex struct {
	rows   []stateGroupRow
	byKey  map[stateGroupKey]int
	sealed bool
}

func (index stateGroupIndex) valid() bool { return index.sealed }

func (index stateGroupIndex) agreesWithKeys() bool {
	if index.byKey == nil || len(index.rows) != len(index.byKey) {
		return false
	}
	for rowIndex, row := range index.rows {
		if row.group < 0 {
			return false
		}
		mapped, present := index.byKey[stateGroupKey{state: row.state, group: row.group}]
		if !present || mapped != rowIndex {
			return false
		}
	}
	return true
}

func sealStateGroupIndex(rows []stateGroupRow, byKey map[stateGroupKey]int) stateGroupIndex {
	index := stateGroupIndex{rows: rows, byKey: byKey}
	index.sealed = index.agreesWithKeys()
	return index
}

func buildStateGroupIndex(graph *equation.Graph, plan interface {
	StateCount() contextfiber.StateOrdinal
	StateAt(contextfiber.StateOrdinal) (contextfiber.StateCell, bool)
}, artifact bool, activePoints []bool) (stateGroupIndex, []bool, bool) {
	if graph == nil || len(activePoints) != graph.PointCount() {
		return stateGroupIndex{}, nil, false
	}
	stateCount := graph.PointCount()
	if artifact {
		if plan == nil {
			return stateGroupIndex{}, nil, false
		}
		stateCount = int(plan.StateCount())
		if stateCount == 0 {
			return stateGroupIndex{}, nil, false
		}
	} else {
		// The graph-point executor is deliberately a separate construction. Its
		// producer cache retains the historical one-row-per-graph-group shape so
		// the engine-only recurrence/activation laws do not acquire a fabricated
		// context plane. Mounted execution takes the compact state/group path
		// below instead.
		rows := make([]stateGroupRow, graph.GroupCount())
		byKey := make(map[stateGroupKey]int, graph.GroupCount())
		for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
			group, groupOK := graph.HyperedgeAt(groupIndex)
			output := group.Output()
			pointIndex, pointOK := graph.PointIndex(output)
			if !groupOK || !pointOK || pointIndex < 0 || pointIndex >= graph.PointCount() {
				return stateGroupIndex{}, nil, false
			}
			row := stateGroupRow{state: contextfiber.StateOrdinal(pointIndex), group: groupIndex}
			rows[groupIndex] = row
			byKey[stateGroupKey{state: row.state, group: row.group}] = groupIndex
		}
		index := sealStateGroupIndex(rows, byKey)
		return index, append([]bool(nil), activePoints...), index.valid()
	}
	activeStates := make([]bool, stateCount)
	rows := make([]stateGroupRow, 0)
	byKey := make(map[stateGroupKey]int)
	for state := 0; state < stateCount; state++ {
		pointIndex := state
		if artifact {
			cell, ok := plan.StateAt(contextfiber.StateOrdinal(state))
			pointOrdinal, pointOK := cell.PointOrdinal()
			if !ok || !pointOK || uint64(pointOrdinal) >= uint64(graph.PointCount()) {
				return stateGroupIndex{}, nil, false
			}
			pointIndex = int(pointOrdinal)
		}
		if activePoints[pointIndex] {
			activeStates[state] = true
		}
		point, ok := graph.PointAt(schedule.Node(pointIndex))
		if !ok {
			return stateGroupIndex{}, nil, false
		}
		for producerIndex := 0; producerIndex < graph.ProducerCount(point); producerIndex++ {
			group, groupOK := graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= graph.GroupCount() {
				return stateGroupIndex{}, nil, false
			}
			key := stateGroupKey{state: contextfiber.StateOrdinal(state), group: groupIndex}
			if _, duplicate := byKey[key]; duplicate {
				return stateGroupIndex{}, nil, false
			}
			byKey[key] = len(rows)
			rows = append(rows, stateGroupRow{state: key.state, group: key.group})
		}
	}
	index := sealStateGroupIndex(rows, byKey)
	return index, activeStates, index.valid()
}

func (index stateGroupIndex) row(state contextfiber.StateOrdinal, group int) (int, bool) {
	if !index.valid() || group < 0 {
		return 0, false
	}
	row, ok := index.byKey[stateGroupKey{state: state, group: group}]
	return row, ok
}

func (runtime *solverRuntime) stateCount() int {
	if runtime == nil || runtime.graph == nil {
		return 0
	}
	if runtime.artifactBacked {
		if runtime.executionPlan == nil || runtime.executionPlan.StateCount() == 0 {
			return 0
		}
		return int(runtime.executionPlan.StateCount())
	}
	return runtime.graph.PointCount()
}

// stateCell resolves only through the retained execution plan for mounted
// execution.  Non-artifact construction is a separate graph-point runtime and
// therefore has no fabricated ContextOrdinal.
func (runtime *solverRuntime) stateCell(state int) (contextfiber.StateCell, bool) {
	if runtime == nil || state < 0 {
		return contextfiber.StateCell{}, false
	}
	if runtime.artifactBacked {
		if runtime.executionPlan == nil {
			return contextfiber.StateCell{}, false
		}
		return runtime.executionPlan.StateAt(contextfiber.StateOrdinal(state))
	}
	return contextfiber.StateCell{}, false
}

func (runtime *solverRuntime) graphPointAtState(state int) (equation.Point, int, contextfiber.ContextOrdinal, bool) {
	if runtime == nil || runtime.graph == nil || state < 0 {
		return equation.Point{}, 0, 0, false
	}
	if !runtime.artifactBacked {
		point, ok := runtime.graph.PointAt(schedule.Node(state))
		if !ok {
			return equation.Point{}, 0, 0, false
		}
		return point, state, 0, true
	}
	cell, ok := runtime.stateCell(state)
	if !ok {
		return equation.Point{}, 0, 0, false
	}
	pointOrdinal, pointOK := cell.PointOrdinal()
	if !pointOK || uint64(pointOrdinal) >= uint64(runtime.graph.PointCount()) {
		return equation.Point{}, 0, 0, false
	}
	point, pointOK := runtime.graph.PointAt(schedule.Node(int(pointOrdinal)))
	if !pointOK || !runtime.graph.OwnsPoint(point) {
		return equation.Point{}, 0, 0, false
	}
	context, _ := runtime.stateContextAtState(state)
	return point, int(pointOrdinal), context, true
}

// stateContextAtState returns a mounted context only when the state cell owns
// one. Link-global cells deliberately have no ContextOrdinal; callers that
// need a transport address must use stateRowsForGraphPoint and fan out rather
// than treating zero as a fabricated context.
func (runtime *solverRuntime) stateContextAtState(state int) (contextfiber.ContextOrdinal, bool) {
	if runtime == nil || !runtime.artifactBacked || state < 0 {
		return 0, false
	}
	cell, ok := runtime.stateCell(state)
	if !ok {
		return 0, false
	}
	return cell.ContextOrdinal()
}

func (runtime *solverRuntime) stateForGraphPoint(state int, pointIndex int) (int, bool) {
	if runtime == nil || runtime.graph == nil || pointIndex < 0 || pointIndex >= runtime.graph.PointCount() {
		return 0, false
	}
	if !runtime.artifactBacked {
		return pointIndex, true
	}
	rows, rowsOK := runtime.stateRowsForGraphPoint(state, pointIndex)
	if !rowsOK || len(rows) != 1 {
		// A global source can fan out to several mounted target rows. Callers
		// requiring one transport address must refuse that ambiguity and use the
		// explicit fan-out helper instead of fabricating ContextOrdinal(0).
		return 0, false
	}
	return rows[0], true
}

// stateRowsForGraphPoint resolves one graph Point in the source state's
// execution context. Link-global targets are singleton rows; mounted targets
// inherit the source mounted context, while a Link-global source fans out to
// every eligible mounted target occurrence. This is the only structural
// global-to-mounted wake carrier.
func (runtime *solverRuntime) stateRowsForGraphPoint(sourceState, pointIndex int) ([]int, bool) {
	if runtime == nil || !runtime.artifactBacked || runtime.executionPlan == nil || pointIndex < 0 || pointIndex >= len(runtime.statePointRows) || sourceState < 0 || sourceState >= runtime.stateCount() {
		return nil, false
	}
	targetOwner, ownerOK := runtime.contextLayout.PointOwnerAt(contextfiber.PointOrdinal(pointIndex))
	if !ownerOK {
		return nil, false
	}
	if targetOwner.LinkGlobal() {
		row, rowOK := runtime.executionPlan.Lookup(0, contextfiber.PointOrdinal(pointIndex))
		if !rowOK {
			return nil, false
		}
		return []int{int(row)}, true
	}
	context, mounted := runtime.stateContextAtState(sourceState)
	if mounted {
		row, rowOK := runtime.executionPlan.Lookup(context, contextfiber.PointOrdinal(pointIndex))
		if !rowOK {
			return nil, false
		}
		return []int{int(row)}, true
	}
	// The source row is Link-global, so every target occurrence is a legal
	// structural destination. statePointRows is the compact inverse index and
	// is already sorted at plan assembly.
	rows := runtime.statePointRows[pointIndex]
	if len(rows) == 0 {
		return nil, false
	}
	result := append([]int(nil), rows...)
	return result, true
}
func (runtime *solverRuntime) activeState(state int) bool {
	if runtime == nil || state < 0 {
		return false
	}
	if !runtime.artifactBacked {
		return state < len(runtime.activePoints) && runtime.activePoints[state]
	}
	return state < len(runtime.activeStates) && runtime.activeStates[state]
}

func (epoch *executorEpoch) graphPoint(state int) (equation.Point, int, contextfiber.ContextOrdinal, bool) {
	if epoch == nil || epoch.runtime == nil {
		return equation.Point{}, 0, 0, false
	}
	return epoch.runtime.graphPointAtState(state)
}

func (epoch *executorEpoch) activeState(state int) bool {
	return epoch != nil && epoch.runtime != nil && epoch.runtime.activeState(state) && state < len(epoch.points)
}

func (epoch *executorEpoch) producerCache(state contextfiber.StateOrdinal, group int) (*producerEpoch, bool) {
	if epoch == nil || epoch.runtime == nil {
		return nil, false
	}
	row, ok := epoch.runtime.producerRows.row(state, group)
	if !ok || row < 0 || row >= len(epoch.producers) {
		return nil, false
	}
	return &epoch.producers[row], true
}

func (epoch *executorEpoch) producerRow(state contextfiber.StateOrdinal, group int) (stateGroupRow, bool) {
	if epoch == nil || epoch.runtime == nil {
		return stateGroupRow{}, false
	}
	row, ok := epoch.runtime.producerRows.row(state, group)
	if !ok || row < 0 || row >= len(epoch.runtime.producerRows.rows) {
		return stateGroupRow{}, false
	}
	return epoch.runtime.producerRows.rows[row], true
}

// liftStateRegions derives the runtime recurrence interface directly from the
// contextual WTO. The graph region catalog remains cold metadata for the
// singular graph; it is not required to have a corresponding Region for each
// StateOrdinal region because authenticated BoundEdges can create SCCs that
// do not exist in any one graph-point schedule.
func liftStateRegions(graph *equation.Graph, execution *schedule.Schedule, activeStates []bool, runtime *solverRuntime, factorRows []runtimeStateFactorRow, late bool) ([]runtimeRegion, [][]int, []int, []bool, []schedule.Event, bool) {
	if graph == nil || execution == nil || runtime == nil || !runtime.artifactBacked || len(activeStates) != runtime.stateCount() {
		return nil, nil, nil, nil, nil, false
	}
	stateCount := len(activeStates)
	regions := make([]runtimeRegion, execution.RegionCount())
	active := make([]bool, len(regions))
	contains := make([][]bool, len(regions))
	for _, row := range factorRows {
		if row.edge < 0 || row.source < 0 || row.source >= stateCount || row.target < 0 || row.target >= stateCount {
			return nil, nil, nil, nil, nil, false
		}
	}
	for regionIndex := range regions {
		static, ok := execution.RegionAt(regionIndex)
		if !ok || static.Head < 0 || int(static.Head) >= stateCount || static.Parent < schedule.NoRegion || static.Parent >= len(regions) || static.Enter < 0 || static.Exit <= static.Enter || static.Exit >= execution.EventCount() {
			return nil, nil, nil, nil, nil, false
		}
		members := make([]bool, stateCount)
		for eventIndex := static.Enter; eventIndex <= static.Exit; eventIndex++ {
			event, eventOK := execution.EventAt(eventIndex)
			if !eventOK {
				return nil, nil, nil, nil, nil, false
			}
			if event.Kind == schedule.EventNode {
				if event.Node < 0 || int(event.Node) >= stateCount {
					return nil, nil, nil, nil, nil, false
				}
				members[event.Node] = true
				if activeStates[event.Node] {
					active[regionIndex] = true
				}
			}
		}
		contains[regionIndex] = members
	}
	for index := range regions {
		if !active[index] {
			continue
		}
		parent, _ := execution.RegionAt(index)
		for parent.Parent != schedule.NoRegion {
			if parent.Parent < 0 || parent.Parent >= len(active) {
				return nil, nil, nil, nil, nil, false
			}
			active[parent.Parent] = true
			parent, _ = execution.RegionAt(parent.Parent)
		}
	}

	// stateInside resolves a graph-owned input in the exact source context of
	// one state head. A multi-row result would be an unproven global-to-mounted
	// merge for an ordinary Group input and is refused rather than choosing a
	// fabricated context.
	stateInside := func(regionIndex, stateHead, pointIndex int) (bool, bool) {
		if pointIndex < 0 || pointIndex >= graph.PointCount() || stateHead < 0 || stateHead >= stateCount {
			return false, false
		}
		rows, rowsOK := runtime.stateRowsForGraphPoint(stateHead, pointIndex)
		if !rowsOK || len(rows) != 1 {
			return false, false
		}
		state := rows[0]
		return contains[regionIndex][state], true
	}

	for regionIndex := range regions {
		static, _ := execution.RegionAt(regionIndex)
		stateHead := int(static.Head)
		headPoint, headPointIndex, _, headOK := runtime.graphPointAtState(stateHead)
		if !headOK || headPointIndex < 0 || headPointIndex >= graph.PointCount() {
			return nil, nil, nil, nil, nil, false
		}
		bound := runtimeRegion{active: active[regionIndex], head: stateHead, parent: static.Parent, points: make([]int, 0)}
		for eventIndex := static.Enter; eventIndex <= static.Exit; eventIndex++ {
			event, eventOK := execution.EventAt(eventIndex)
			if !eventOK {
				return nil, nil, nil, nil, nil, false
			}
			if event.Kind == schedule.EventNode {
				bound.points = append(bound.points, int(event.Node))
			}
		}
		// Classify every head producer from the exact contextual input rows. This
		// includes a head that has no graph Region at all, which is the essential
		// BoundEdge-only SCC case.
		for producerIndex := 0; producerIndex < graph.ProducerCount(headPoint); producerIndex++ {
			group, groupOK := graph.ProducerAt(headPoint, producerIndex)
			groupIndex, groupIndexed := graph.GroupIndex(group)
			if !groupOK || !groupIndexed || groupIndex < 0 || groupIndex >= len(runtime.producers) {
				return nil, nil, nil, nil, nil, false
			}
			inside := false
			for inputIndex := 0; inputIndex < group.InputCount(); inputIndex++ {
				input, inputOK := group.InputAt(inputIndex)
				inputPoint, inputIndexed := graph.PointIndex(input.Point())
				if !inputOK || !inputIndexed {
					return nil, nil, nil, nil, nil, false
				}
				inputInside, insideOK := stateInside(regionIndex, stateHead, inputPoint)
				if !insideOK {
					return nil, nil, nil, nil, nil, false
				}
				inside = inside || inputInside
			}
			if environment, present := group.EnvironmentInput(); present {
				inputPoint, inputIndexed := graph.PointIndex(environment.Point())
				if !inputIndexed {
					return nil, nil, nil, nil, nil, false
				}
				inputInside, insideOK := stateInside(regionIndex, stateHead, inputPoint)
				if !insideOK {
					return nil, nil, nil, nil, nil, false
				}
				inside = inside || inputInside
			}
			row, rowOK := runtime.producerRows.row(contextfiber.StateOrdinal(stateHead), groupIndex)
			if !rowOK {
				return nil, nil, nil, nil, nil, false
			}
			if inside {
				bound.back = append(bound.back, row)
			} else {
				bound.external = append(bound.external, row)
			}
		}

		// Environment edges have no producer row, but a head ingress still has
		// the same external/back split. Transport-only rows are recurrence-local
		// by construction and remain on the back side.
		for edgeIndex, edge := range runtime.environments {
			if edge.target != headPointIndex {
				continue
			}
			targetRows, targetOK := runtime.stateRowsForGraphPoint(stateHead, edge.target)
			if !targetOK || len(targetRows) != 1 || targetRows[0] != stateHead {
				return nil, nil, nil, nil, nil, false
			}
			inside, insideOK := stateInside(regionIndex, stateHead, edge.source)
			if !insideOK {
				return nil, nil, nil, nil, nil, false
			}
			if inside || graphEnvironmentTransportOnly(graph, edgeIndex) {
				bound.environmentBack = append(bound.environmentBack, edgeIndex)
			} else {
				bound.environmentExternal = append(bound.environmentExternal, edgeIndex)
			}
		}
		// Link-bound transports are already addressed by exact StateOrdinal. Keep
		// their ingress classification separate from graph EnvironmentEdges so a
		// target context cannot inherit a sibling's source value.
		var incomingTransports []int
		if len(runtime.contextTransportIncoming) != 0 {
			if stateHead < 0 || stateHead >= len(runtime.contextTransportIncoming) {
				return nil, nil, nil, nil, nil, false
			}
			incomingTransports = runtime.contextTransportIncoming[stateHead]
		}
		for _, transportIndex := range incomingTransports {
			if transportIndex < 0 || transportIndex >= len(runtime.contextTransports) {
				return nil, nil, nil, nil, nil, false
			}
			transport := runtime.contextTransports[transportIndex]
			if transport.to != stateHead || transport.from < 0 || transport.from >= stateCount || transport.targetPoint != headPointIndex {
				return nil, nil, nil, nil, nil, false
			}
			sourceState, sourceStateOK := runtime.contextTransportSourceState(stateHead, transport.sourcePoint)
			if !sourceStateOK || sourceState != transport.from {
				return nil, nil, nil, nil, nil, false
			}
			targetContext, targetContextOK := runtime.stateContextAtState(stateHead)
			_, sourcePoint, sourceContext, sourceStateOK := runtime.graphPointAtState(transport.from)
			if !targetContextOK || !sourceStateOK || sourcePoint != transport.sourcePoint || sourceContext != transport.sourceContext || targetContext != transport.targetContext {
				return nil, nil, nil, nil, nil, false
			}
			if contains[regionIndex][transport.from] {
				bound.contextBack = append(bound.contextBack, transportIndex)
			} else {
				bound.contextExternal = append(bound.contextExternal, transportIndex)
			}
		}
		sort.Ints(bound.contextExternal)
		sort.Ints(bound.contextBack)

		for rowIndex, row := range factorRows {
			if row.target != stateHead {
				continue
			}
			if row.edge < 0 || row.source < 0 || row.source >= stateCount {
				return nil, nil, nil, nil, nil, false
			}
			inside := contains[regionIndex][row.source]
			if inside {
				bound.factorBack = append(bound.factorBack, rowIndex)
			} else {
				bound.factorExternal = append(bound.factorExternal, rowIndex)
			}
		}

		// A state Region's value widening scope is the exact union of internal
		// producer occurrences. Route universes are added once per occurrence,
		// preserving the compact factor-row representation while covering both
		// graph-derived and BoundEdge-derived recurrence interfaces.
		var widenTargets, narrowTargets []carrier.Target
		for _, state := range bound.points {
			if state < 0 || state >= stateCount || !activeStates[state] {
				continue
			}
			point, _, _, pointOK := runtime.graphPointAtState(state)
			if !pointOK {
				return nil, nil, nil, nil, nil, false
			}
			for producerIndex := 0; producerIndex < graph.ProducerCount(point); producerIndex++ {
				group, groupOK := graph.ProducerAt(point, producerIndex)
				groupIndex, groupIndexed := graph.GroupIndex(group)
				if !groupOK || !groupIndexed || groupIndex < 0 || groupIndex >= len(runtime.producers) {
					return nil, nil, nil, nil, nil, false
				}
				inside := false
				for inputIndex := 0; inputIndex < group.InputCount(); inputIndex++ {
					input, inputOK := group.InputAt(inputIndex)
					inputPoint, inputIndexed := graph.PointIndex(input.Point())
					if !inputOK || !inputIndexed {
						return nil, nil, nil, nil, nil, false
					}
					inputInside, insideOK := stateInside(regionIndex, state, inputPoint)
					if !insideOK {
						return nil, nil, nil, nil, nil, false
					}
					inside = inside || inputInside
				}
				if environment, present := group.EnvironmentInput(); present {
					inputPoint, inputIndexed := graph.PointIndex(environment.Point())
					if !inputIndexed {
						return nil, nil, nil, nil, nil, false
					}
					inputInside, insideOK := stateInside(regionIndex, state, inputPoint)
					if !insideOK {
						return nil, nil, nil, nil, nil, false
					}
					inside = inside || inputInside
				}
				if !inside {
					continue
				}
				for _, occurrence := range runtime.producers[groupIndex].footprint {
					widenTargets = append(widenTargets, occurrence.targets...)
					narrowTargets = append(narrowTargets, occurrence.narrowTargets...)
					if occurrence.route && occurrence.routeFactor != nil && occurrence.routeFactor.hasRouteUniverse() {
						universe := occurrence.routeFactor.routeUniverse()
						widenTargets = append(widenTargets, universe...)
						if occurrence.narrowRoute {
							narrowTargets = append(narrowTargets, universe...)
						}
					}
				}
			}
		}
		widenTargets = compactRuntimeTargets(widenTargets)
		narrowTargets = compactRuntimeTargets(narrowTargets)
		var widen carrier.MergeScope
		var widenOK bool
		if late {
			widen, widenOK = runtime.carrier.SealRuntimeWidening(widenTargets)
		} else {
			widen, widenOK = runtime.carrier.SealWidening(widenTargets)
		}
		if !widenOK {
			return nil, nil, nil, nil, nil, false
		}
		var narrow carrier.MergeScope
		var narrowOK bool
		if late {
			narrow, narrowOK = runtime.carrier.SealRuntimeNarrowing(narrowTargets)
		} else {
			narrow, narrowOK = runtime.carrier.SealNarrowing(narrowTargets)
		}
		if !narrowOK {
			return nil, nil, nil, nil, nil, false
		}
		bound.widen, bound.narrow = widen, narrow
		// Discharge and Newton are optimizations whose exact closure bases are
		// graph-region-specific. A state-only SCC intentionally leaves both
		// unavailable; the state WTO still owns the complete exact fold.
		regions[regionIndex] = bound
	}
	children := make([][]int, len(regions))
	for index, region := range regions {
		if !active[index] {
			continue
		}
		if region.parent == schedule.NoRegion {
			continue
		}
		if region.parent < 0 || region.parent >= len(regions) || !active[region.parent] {
			return nil, nil, nil, nil, nil, false
		}
		children[region.parent] = append(children[region.parent], index)
	}
	pointRegion := make([]int, stateCount)
	for index := range pointRegion {
		pointRegion[index] = schedule.NoRegion
	}
	for eventIndex := 0; eventIndex < execution.EventCount(); eventIndex++ {
		event, eventOK := execution.EventAt(eventIndex)
		if !eventOK {
			return nil, nil, nil, nil, nil, false
		}
		if event.Kind == schedule.EventNode && event.Region != schedule.NoRegion {
			if event.Node < 0 || int(event.Node) >= stateCount || event.Region < 0 || event.Region >= len(regions) {
				return nil, nil, nil, nil, nil, false
			}
			pointRegion[event.Node] = event.Region
		}
	}
	events, eventsOK := filteredStateEvents(execution, activeStates, active)
	if !eventsOK {
		return nil, nil, nil, nil, nil, false
	}
	return regions, children, pointRegion, active, events, true
}

func graphEnvironmentTransportOnly(graph *equation.Graph, edgeIndex int) bool {
	if graph == nil || edgeIndex < 0 {
		return false
	}
	edge, ok := graph.EnvironmentEdgeAtIndex(edgeIndex)
	return ok && edge.TransportOnly()
}

// filteredStateEvents is the demanded event view over the immutable state
// schedule. Region brackets are retained only for reached regions; inactive
// contextual nodes never reach the executor's refresh path.
func filteredStateEvents(execution *schedule.Schedule, activeStates []bool, activeRegions []bool) ([]schedule.Event, bool) {
	if execution == nil || len(activeStates) != execution.NodeCount() || len(activeRegions) != execution.RegionCount() {
		return nil, false
	}
	events := make([]schedule.Event, 0, execution.EventCount())
	for index := 0; index < execution.EventCount(); index++ {
		event, ok := execution.EventAt(index)
		if !ok {
			return nil, false
		}
		switch event.Kind {
		case schedule.EventNode:
			if event.Node < 0 || int(event.Node) >= len(activeStates) {
				return nil, false
			}
			if activeStates[event.Node] {
				events = append(events, event)
			}
		case schedule.EventEnter, schedule.EventExit:
			if event.Region < 0 || event.Region >= len(activeRegions) {
				return nil, false
			}
			if activeRegions[event.Region] {
				events = append(events, event)
			}
		default:
			return nil, false
		}
	}
	return events, true
}
