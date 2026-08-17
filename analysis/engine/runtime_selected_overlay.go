package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
)

// preparedSelectedFactorOverlay is a stale-fenced, uninstalled structural
// delta.  It carries only the newly bound relations, changed factor rows, and
// touched CSR rows. Solver.relation remains the sole activation relation;
// this type owns no Member or premise catalogue.
//
// Preparation never mutates runtime or epoch state. Installation is then a
// no-failure semantic commit after its generation/count fence succeeds:
// maps may use ordinary Go backing growth, but no callback, carrier action,
// bounds-dependent operation, or recoverable validation remains.
type preparedSelectedFactorOverlay struct {
	runtime           *solverRuntime
	generation        identity.Generation
	previousEdgeCount int
	// grownFactorEdges is non-nil only when the runtime edge backing store must
	// grow. It amortizes the unavoidable full edge copy; ordinary frontiers
	// append only `additions` into reserved capacity.
	grownFactorEdges []runtimeFactorEdge
	additions        []preparedFactorAddition
	replacements     []preparedFactorReplacement
	// Only source/target CSR rows touched by this delta are cloned and sorted.
	// The runtime owns the outer CSR slices permanently, so unrelated Point
	// rows retain both their identity and backing storage across frontiers.
	incomingRows      []preparedFactorCSRRow
	outgoingRows      []preparedFactorCSRRow
	dependencyEdges   []schedule.Edge
	dependencyAt      map[[2]int]struct{}
	dependencyChanged bool
	// latePlans contains only relations newly issued after Work opened; static
	// plans and already installed late plans are read directly from runtime.
	latePlans  map[composition.Key]carrier.ReindexPlan
	newOrigins map[runtimeFactorOrigin]int
	targets    []int
	// execution is non-nil only when this selected delta introduces the first
	// feedback region over the same demanded Point set. Region rows are bound
	// to the existing carrier; no typed root is migrated.
	execution       *schedule.Schedule
	executionDemand *equation.Demand
	// directCatalog is the complete installed selected-direct descriptor set
	// after this frontier (indexed by runtime factor edge). It is retained only
	// until the prepared overlay is committed, then published in overlay.directAt.
	directCatalog  map[int]equation.SelectedStructuralFactorEdge
	directEdges    []equation.SelectedStructuralFactorEdge
	regions        []runtimeRegion
	regionChildren [][]int
	pointRegion    []int
	activeRegions  []bool
}

// preparedFactorCSRRow is one replacement for an existing runtime CSR row.
// Its point is sorted uniquely in a prepared overlay; installation assigns
// only these rows after every allocation and validation has completed.
type preparedFactorCSRRow struct {
	point int
	edges []int
}

type preparedFactorReplacement struct {
	index int
	edge  runtimeFactorEdge
}

// preparedFactorAddition keeps its cold origin beside, rather than inside,
// the hot factor edge.  originAt is the selected-edge seal after install; the
// executor never needs this identity to fold, wake, or stamp the edge.
type preparedFactorAddition struct {
	edge   runtimeFactorEdge
	origin runtimeFactorOrigin
}

type boundSelectedFactorEdge struct {
	edge   runtimeFactorEdge
	origin runtimeFactorOrigin
}

// prepareSelectedFactorOverlay materializes a canonical delta only when all
// descriptors are structural FactorEdges over exact base-graph Points, all
// Points are demanded, and the combined static+installed+new relation stays
// acyclic. A selected origin may widen its precondition in place exactly when
// old.pre entails new.pre; that replaces, rather than duplicates, the bound
// transport and forces an exact target fold.
func (runtime *solverRuntime) prepareSelectedFactorOverlay(delta []equation.AcceptedMember, published equation.Relation) (*preparedSelectedFactorOverlay, bool) {
	if !runtimeSelectedOverlayEligible(runtime) {
		return nil, false
	}
	if len(delta) == 0 || !canonicalAcceptedActivations(delta) || !validAcceptedActivations(runtime.topology, delta) || !published.OwnedBy(runtime.topology) {
		return nil, false
	}
	selected, materialized := runtime.topology.SelectedStructuralFactorEdges(runtime.graph, delta)
	if !materialized || len(selected) == 0 {
		return nil, false
	}
	descriptors, dependencyEdges, dependencyAt, dependencyChanged, execution, valid := runtime.prevalidateSelectedFactorEdges(selected)
	if !valid {
		return nil, false
	}

	prepared := &preparedSelectedFactorOverlay{
		runtime:           runtime,
		generation:        runtime.overlay.generation,
		previousEdgeCount: len(runtime.factorEdges),
		dependencyEdges:   dependencyEdges,
		dependencyAt:      dependencyAt,
		dependencyChanged: dependencyChanged,
		execution:         execution,
		latePlans:         make(map[composition.Key]carrier.ReindexPlan),
		newOrigins:        make(map[runtimeFactorOrigin]int),
		directCatalog:     cloneDirectCatalog(runtime.overlay.directAt),
	}
	for _, descriptor := range descriptors {
		bound, valid := runtime.bindSelectedFactorEdge(descriptor, prepared.latePlans)
		if !valid {
			return nil, false
		}
		if installed, present := runtime.overlay.originAt[bound.origin]; present {
			if !runtime.validSelectedOrigin(installed, bound.origin) {
				return nil, false
			}
			previous := runtime.factorEdges[installed]
			if !previous.input.pre.Entails(bound.edge.input.pre) {
				return nil, false
			}
			if previous.input.pre.Equal(bound.edge.input.pre) {
				continue
			}
			if !appendPreparedReplacement(prepared, installed, bound.edge) {
				return nil, false
			}
			prepared.directCatalog[installed] = descriptor.edge
			continue
		}
		if _, static := runtime.overlay.staticOrigins[bound.origin]; static {
			return nil, false
		}
		if _, duplicate := prepared.newOrigins[bound.origin]; duplicate {
			return nil, false
		}
		bound.edge.index = prepared.previousEdgeCount + len(prepared.additions)
		prepared.newOrigins[bound.origin] = bound.edge.index
		prepared.additions = append(prepared.additions, preparedFactorAddition{edge: bound.edge, origin: bound.origin})
		prepared.directCatalog[bound.edge.index] = descriptor.edge
	}
	if len(prepared.additions) == 0 && len(prepared.replacements) == 0 {
		return nil, false
	}
	if !prepared.finalize(runtime) {
		return nil, false
	}
	prepared.directEdges = directCatalogEdges(prepared.directCatalog)
	if prepared.execution != nil && prepared.execution.RegionCount() != 0 && !prepared.bindFeedbackRuntime(runtime) {
		return nil, false
	}
	return prepared, true
}

func cloneDirectCatalog(source map[int]equation.SelectedStructuralFactorEdge) map[int]equation.SelectedStructuralFactorEdge {
	result := make(map[int]equation.SelectedStructuralFactorEdge, len(source))
	for index, edge := range source {
		result[index] = edge
	}
	return result
}

func directCatalogEdges(catalog map[int]equation.SelectedStructuralFactorEdge) []equation.SelectedStructuralFactorEdge {
	if len(catalog) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(catalog))
	for index := range catalog {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	result := make([]equation.SelectedStructuralFactorEdge, 0, len(indexes))
	for _, index := range indexes {
		result = append(result, catalog[index])
	}
	return result
}

func (runtime *solverRuntime) validSelectedOrigin(index int, origin runtimeFactorOrigin) bool {
	known, present := runtime.overlay.originAt[origin]
	return runtime != nil && present && known == index && index >= 0 && index < len(runtime.factorEdges) && runtime.factorEdges[index].index == index
}

func appendPreparedReplacement(prepared *preparedSelectedFactorOverlay, index int, edge runtimeFactorEdge) bool {
	if prepared == nil || index < 0 {
		return false
	}
	for replacementIndex := range prepared.replacements {
		prior := &prepared.replacements[replacementIndex]
		if prior.index != index {
			continue
		}
		if !prior.edge.input.pre.Entails(edge.input.pre) {
			return false
		}
		if prior.edge.input.pre.Equal(edge.input.pre) {
			return true
		}
		prior.edge = edge
		return true
	}
	prepared.replacements = append(prepared.replacements, preparedFactorReplacement{index: index, edge: edge})
	return true
}

type selectedFactorDescriptor struct {
	edge   equation.SelectedStructuralFactorEdge
	source int
	target int
}

func (runtime *solverRuntime) prevalidateSelectedFactorEdges(selected []equation.SelectedStructuralFactorEdge) ([]selectedFactorDescriptor, []schedule.Edge, map[[2]int]struct{}, bool, *schedule.Schedule, bool) {
	if runtime == nil || runtime.graph == nil || len(selected) == 0 {
		return nil, nil, nil, false, nil, false
	}
	result := make([]selectedFactorDescriptor, 0, len(selected))
	seen := make(map[composition.Key]struct{}, len(selected))
	for _, edge := range selected {
		if !edge.Available() || !runtime.graph.OwnsPoint(edge.Source()) || !runtime.graph.OwnsPoint(edge.Target()) || edge.Input().Point().Key() != edge.Source().Key() {
			return nil, nil, nil, false, nil, false
		}
		source, sourceOK := runtime.graph.PointIndex(edge.Source())
		target, targetOK := runtime.graph.PointIndex(edge.Target())
		factor, factorOK := runtime.overlay.factorByKey[edge.Factor()]
		slot, slotOK := shape.Slot(0), false
		if factorOK && factor != nil {
			slot, slotOK = factor.runtimeSlot()
		}
		if !sourceOK || !targetOK || !factorOK || factor == nil || compositionKeyOf(factor.semantic()) != edge.Factor() || !slotOK || slot < 0 || int(slot) >= runtime.carrier.Count() || source < 0 || target < 0 || source >= runtime.graph.PointCount() || target >= runtime.graph.PointCount() {
			return nil, nil, nil, false, nil, false
		}
		if _, duplicate := seen[edge.Key()]; duplicate {
			return nil, nil, nil, false, nil, false
		}
		seen[edge.Key()] = struct{}{}
		result = append(result, selectedFactorDescriptor{edge: edge, source: source, target: target})
	}
	dependencies, dependencyAt, changed, execution, scheduled := runtime.combinedSelectedDependencyEdges(result)
	if !scheduled {
		return nil, nil, nil, false, nil, false
	}
	return result, dependencies, dependencyAt, changed, execution, true
}

// combinedSelectedDependencyEdges performs one batched WTO construction. If
// every candidate endpoint relation already exists, the earlier acyclicity
// proof remains valid and no relation slice is copied.
func (runtime *solverRuntime) combinedSelectedDependencyEdges(selected []selectedFactorDescriptor) ([]schedule.Edge, map[[2]int]struct{}, bool, *schedule.Schedule, bool) {
	if runtime == nil || runtime.graph == nil || runtime.overlay.dependencyAt == nil {
		return nil, nil, false, nil, false
	}
	newEdges := make([]schedule.Edge, 0, len(selected))
	newAt := make(map[[2]int]struct{}, len(selected))
	for _, descriptor := range selected {
		key := [2]int{descriptor.source, descriptor.target}
		if _, exists := runtime.overlay.dependencyAt[key]; exists {
			continue
		}
		if _, exists := newAt[key]; exists {
			continue
		}
		newAt[key] = struct{}{}
		newEdges = append(newEdges, schedule.Edge{From: schedule.Node(descriptor.source), To: schedule.Node(descriptor.target)})
	}
	if len(newEdges) == 0 {
		return runtime.overlay.dependencyEdges, nil, false, runtime.graph.Schedule(), true
	}
	// A genuinely new endpoint pair needs a fresh combined acyclicity proof.
	// Clone the cold membership cache only on that path; replacements and
	// already-known endpoints remain O(delta) without retained-edge scans.
	known := make(map[[2]int]struct{}, len(runtime.overlay.dependencyAt)+len(newAt))
	for key := range runtime.overlay.dependencyAt {
		known[key] = struct{}{}
	}
	for key := range newAt {
		known[key] = struct{}{}
	}
	edges := make([]schedule.Edge, len(runtime.overlay.dependencyEdges), len(runtime.overlay.dependencyEdges)+len(newEdges))
	copy(edges, runtime.overlay.dependencyEdges)
	edges = append(edges, newEdges...)
	sort.Slice(edges, func(left, right int) bool {
		if edges[left].From != edges[right].From {
			return edges[left].From < edges[right].From
		}
		return edges[left].To < edges[right].To
	})
	prepared, err := schedule.Prepare(runtime.graph.PointCount(), edges)
	if err != nil || prepared == nil {
		return nil, nil, false, nil, false
	}
	return edges, known, true, prepared, true
}

func (runtime *solverRuntime) bindSelectedFactorEdge(descriptor selectedFactorDescriptor, issued map[composition.Key]carrier.ReindexPlan) (boundSelectedFactorEdge, bool) {
	if runtime == nil || runtime.carrier == nil || issued == nil || descriptor.source < 0 || descriptor.target < 0 || descriptor.source >= len(runtime.pointScopes) || descriptor.target >= len(runtime.pointScopes) {
		return boundSelectedFactorEdge{}, false
	}
	edge, input := descriptor.edge, descriptor.edge.Input()
	factor, factorOK := runtime.overlay.factorByKey[edge.Factor()]
	slot, slotOK := shape.Slot(0), false
	if factorOK && factor != nil {
		slot, slotOK = factor.runtimeSlot()
	}
	sourceScope, sourceScoped := runtime.overlay.reindexes.scope(input.Source().Scope())
	targetScope, targetScoped := runtime.overlay.reindexes.scope(input.Target().Scope())
	if !edge.Available() || !input.Available() || !factorOK || factor == nil || compositionKeyOf(factor.semantic()) != edge.Factor() || !slotOK || slot < 0 || int(slot) >= runtime.carrier.Count() || !sourceScoped || !targetScoped || !sourceScope.Same(runtime.pointScopes[descriptor.source]) || !targetScope.Same(runtime.pointScopes[descriptor.target]) {
		return boundSelectedFactorEdge{}, false
	}
	planKey := input.Reindex().Key()
	plan, planOK := runtime.overlay.reindexes.plan(input.Reindex())
	if !planOK {
		plan, planOK = runtime.overlay.latePlans[planKey]
	}
	if !planOK {
		plan, planOK = issued[planKey]
	}
	if !planOK {
		plan, planOK = lowerRuntimeReindexLate(runtime.carrier, input.Reindex(), runtime.overlay.reindexes.scopes, runtime.overlay.reindexes.decisions)
		if planOK {
			issued[planKey] = plan
		}
	}
	pre, preOK := runtimeFormula(runtime.carrier, sourceScope, input.Pre(), runtime.overlay.reindexes.decisions)
	post, postOK := runtimeFormula(runtime.carrier, targetScope, input.Post(), runtime.overlay.reindexes.decisions)
	bound := runtimeInput{key: input.Key(), provenance: input.Provenance(), pre: pre, plan: plan, post: post}
	origin, originOK := runtimeFactorEdgeOrigin(descriptor.source, descriptor.target, edge.Factor(), input.Provenance(), planKey, post)
	if !planKey.Available() || !planOK || !preOK || !postOK || !bound.valid() || !originOK {
		return boundSelectedFactorEdge{}, false
	}
	return boundSelectedFactorEdge{edge: runtimeFactorEdge{key: edge.Key(), factor: edge.Factor(), source: descriptor.source, target: descriptor.target, input: bound, slot: slot}, origin: origin}, true
}

func (prepared *preparedSelectedFactorOverlay) finalize(runtime *solverRuntime) bool {
	if prepared == nil || runtime == nil || prepared.runtime != runtime || prepared.previousEdgeCount != len(runtime.factorEdges) {
		return false
	}
	for index := range prepared.replacements {
		replacement := &prepared.replacements[index]
		if replacement.index < 0 || replacement.index >= prepared.previousEdgeCount {
			return false
		}
		replacement.edge.index = replacement.index
	}
	if !prepared.prepareEdgeBacking(runtime) || !prepared.prepareTouchedCSR(runtime) || !prepared.collectTargets(runtime.activePoints) {
		return false
	}
	return true
}

// bindFeedbackRuntime uses the prepared schedule plus the exact direct-edge
// delta as a recurrence certificate. Dense Point/Group identities and every
// typed carrier root remain owned by the live runtime; no accepted Graph is
// reconstructed or used as a fake oracle.
func (prepared *preparedSelectedFactorOverlay) bindFeedbackRuntime(runtime *solverRuntime) bool {
	if prepared == nil || runtime == nil || runtime.graph == nil || runtime.carrier == nil || prepared.runtime != runtime || prepared.execution == nil || prepared.execution.RegionCount() == 0 || len(prepared.directEdges) != len(prepared.directCatalog) {
		return false
	}
	oracle, ok := runtime.graph.ActivationGraphOverlay(prepared.execution, prepared.directEdges)
	if !ok || oracle == nil || oracle.Schedule() == nil || oracle.RegionCount() == 0 || oracle.PointCount() != runtime.graph.PointCount() || oracle.GroupCount() != runtime.graph.GroupCount() || oracle.EnvironmentEdgeTotal() != runtime.graph.EnvironmentEdgeTotal() || oracle.FactorEdgeTotal() != prepared.previousEdgeCount+len(prepared.additions) {
		return false
	}
	for index := 0; index < oracle.PointCount(); index++ {
		oldPoint, oldOK := runtime.graph.PointAt(schedule.Node(index))
		newPoint, newOK := oracle.PointAt(schedule.Node(index))
		if !oldOK || !newOK || oldPoint.Key() != newPoint.Key() {
			return false
		}
	}
	for index := 0; index < oracle.GroupCount(); index++ {
		group, groupOK := oracle.HyperedgeAt(index)
		if !groupOK || index >= len(runtime.producers) || runtime.producers[index].group.Key() != group.Key() {
			return false
		}
	}
	demanded, demandedOK := oracle.Demand()
	activePoints, active, activeOK := runtimeDemandMembership(oracle, demanded)
	if !demandedOK || demanded == nil || !activeOK || len(activePoints) != len(runtime.activePoints) || len(active) != oracle.RegionCount() {
		return false
	}
	for index, live := range activePoints {
		// Reordering live rows into a newly discovered WTO region is lawful.
		// Awakening an uninitialized Point or producer family is not: that path
		// retains the canonical cold demand/compiler transition.
		if live != runtime.activePoints[index] {
			return false
		}
	}
	factorAt := make(map[composition.Key]int, oracle.FactorEdgeTotal())
	for index, edge := range runtime.factorEdges {
		factorAt[edge.key] = index
	}
	for _, replacement := range prepared.replacements {
		if replacement.index < 0 || replacement.index >= prepared.previousEdgeCount {
			return false
		}
		factorAt[replacement.edge.key] = replacement.index
	}
	for offset, addition := range prepared.additions {
		index := prepared.previousEdgeCount + offset
		if _, duplicate := factorAt[addition.edge.key]; duplicate {
			return false
		}
		factorAt[addition.edge.key] = index
	}
	regions, children, bound := bindRuntimeRegionsWithEdges(oracle, active, runtime.carrier, runtime.producers, runtimeRegionEdgeResolver{
		runtime: true,
		environment: func(edge equation.EnvironmentEdgeNode) (int, bool) {
			index, indexed := oracle.EnvironmentEdgeIndex(edge)
			old, oldOK := runtime.graph.EnvironmentEdgeAtIndex(index)
			return index, indexed && oldOK && old.Key() == edge.Key()
		},
		factor: func(edge equation.FactorEdgeNode) (int, bool) {
			index, indexed := factorAt[edge.Key()]
			return index, indexed
		},
	})
	if !bound {
		return false
	}
	pointRegion := make([]int, oracle.PointCount())
	for index := range pointRegion {
		point, pointOK := oracle.PointAt(schedule.Node(index))
		region, regionOK := oracle.PointRegion(point)
		if !pointOK || !regionOK || region < schedule.NoRegion || region >= len(regions) {
			return false
		}
		pointRegion[index] = region
	}
	influenced := false
	for _, selected := range active {
		influenced = influenced || selected
	}
	if !influenced {
		return false
	}
	prepared.execution = oracle.Schedule()
	prepared.executionDemand = demanded
	prepared.regions = regions
	prepared.regionChildren = children
	prepared.pointRegion = pointRegion
	prepared.activeRegions = active
	return true
}

func (prepared *preparedSelectedFactorOverlay) prepareEdgeBacking(runtime *solverRuntime) bool {
	nextCount := prepared.previousEdgeCount + len(prepared.additions)
	if nextCount < prepared.previousEdgeCount {
		return false
	}
	if len(prepared.additions) == 0 || cap(runtime.factorEdges) >= nextCount {
		return true
	}
	capacity := cap(runtime.factorEdges) * 2
	if capacity < nextCount {
		capacity = nextCount
	}
	if capacity <= 0 {
		return false
	}
	prepared.grownFactorEdges = make([]runtimeFactorEdge, nextCount, capacity)
	copy(prepared.grownFactorEdges, runtime.factorEdges)
	for additionIndex, addition := range prepared.additions {
		prepared.grownFactorEdges[prepared.previousEdgeCount+additionIndex] = addition.edge
	}
	for _, replacement := range prepared.replacements {
		prepared.grownFactorEdges[replacement.index] = replacement.edge
	}
	return true
}

func (prepared *preparedSelectedFactorOverlay) prepareTouchedCSR(runtime *solverRuntime) bool {
	if prepared == nil || runtime == nil || runtime.graph == nil || len(runtime.factorIncoming) != runtime.graph.PointCount() || len(runtime.overlay.factorOutgoing) != runtime.graph.PointCount() {
		return false
	}
	incoming := make(map[int][]int, len(prepared.additions)+len(prepared.replacements))
	outgoing := make(map[int][]int, len(prepared.additions)+len(prepared.replacements))
	cloneRow := func(rows [][]int, copied map[int][]int, point int) bool {
		if point < 0 || point >= len(rows) {
			return false
		}
		if _, already := copied[point]; !already {
			copied[point] = append([]int(nil), rows[point]...)
		}
		return true
	}
	for _, addition := range prepared.additions {
		if !cloneRow(runtime.overlay.factorOutgoing, outgoing, addition.edge.source) || !cloneRow(runtime.factorIncoming, incoming, addition.edge.target) {
			return false
		}
		outgoing[addition.edge.source] = append(outgoing[addition.edge.source], addition.edge.index)
		incoming[addition.edge.target] = append(incoming[addition.edge.target], addition.edge.index)
	}
	for _, replacement := range prepared.replacements {
		if replacement.index < 0 || replacement.index >= len(runtime.factorEdges) {
			return false
		}
		previous := runtime.factorEdges[replacement.index]
		if previous.source != replacement.edge.source || previous.target != replacement.edge.target ||
			!cloneRow(runtime.overlay.factorOutgoing, outgoing, replacement.edge.source) || !cloneRow(runtime.factorIncoming, incoming, replacement.edge.target) {
			return false
		}
	}
	replacements := make(map[int]runtimeFactorEdge, len(prepared.replacements))
	for _, replacement := range prepared.replacements {
		replacements[replacement.index] = replacement.edge
	}
	edgeAt := func(index int) (runtimeFactorEdge, bool) {
		if replacement, replaced := replacements[index]; replaced {
			return replacement, true
		}
		if index >= prepared.previousEdgeCount {
			local := index - prepared.previousEdgeCount
			if local >= 0 && local < len(prepared.additions) {
				return prepared.additions[local].edge, true
			}
		}
		if index >= 0 && index < len(runtime.factorEdges) {
			return runtime.factorEdges[index], true
		}
		return runtimeFactorEdge{}, false
	}
	var incomingOK, outgoingOK bool
	prepared.incomingRows, incomingOK = sortedPreparedFactorCSRRows(incoming, edgeAt)
	prepared.outgoingRows, outgoingOK = sortedPreparedFactorCSRRows(outgoing, edgeAt)
	return incomingOK && outgoingOK
}

// sortedPreparedFactorCSRRows turns only the copied point rows into stable
// install records. It intentionally ranges the touched map, never the dense
// Point row, so a small accepted frontier stays O(delta).
func sortedPreparedFactorCSRRows(rows map[int][]int, edgeAt func(int) (runtimeFactorEdge, bool)) ([]preparedFactorCSRRow, bool) {
	if rows == nil || edgeAt == nil {
		return nil, false
	}
	points := make([]int, 0, len(rows))
	for point := range rows {
		points = append(points, point)
	}
	sort.Ints(points)
	result := make([]preparedFactorCSRRow, 0, len(points))
	for _, point := range points {
		edges := rows[point]
		for _, edgeIndex := range edges {
			edge, ok := edgeAt(edgeIndex)
			if !ok || edge.index != edgeIndex || !edge.key.Available() {
				return nil, false
			}
		}
		sort.Slice(edges, func(left, right int) bool {
			leftEdge, _ := edgeAt(edges[left])
			rightEdge, _ := edgeAt(edges[right])
			return lessRuntimeKey(leftEdge.key, rightEdge.key)
		})
		result = append(result, preparedFactorCSRRow{point: point, edges: edges})
	}
	return result, true
}

func validPreparedFactorCSRRows(rows []preparedFactorCSRRow, pointCount, edgeCount int) bool {
	previous := -1
	for _, row := range rows {
		if row.point < 0 || row.point >= pointCount || row.point <= previous {
			return false
		}
		previous = row.point
		for _, edge := range row.edges {
			if edge < 0 || edge >= edgeCount {
				return false
			}
		}
	}
	return true
}

func (prepared *preparedSelectedFactorOverlay) collectTargets(active []bool) bool {
	if prepared == nil || len(active) == 0 {
		return false
	}
	seen := make(map[int]struct{}, len(prepared.additions)+len(prepared.replacements))
	for _, addition := range prepared.additions {
		if addition.edge.target < 0 || addition.edge.target >= len(active) {
			return false
		}
		if active[addition.edge.target] {
			seen[addition.edge.target] = struct{}{}
		}
	}
	for _, replacement := range prepared.replacements {
		if replacement.edge.target < 0 || replacement.edge.target >= len(active) {
			return false
		}
		if active[replacement.edge.target] {
			seen[replacement.edge.target] = struct{}{}
		}
	}
	prepared.targets = make([]int, 0, len(seen))
	for target := range seen {
		prepared.targets = append(prepared.targets, target)
	}
	sort.Ints(prepared.targets)
	return true
}

func runtimeSelectedOverlayEligible(runtime *solverRuntime) bool {
	return runtime != nil && runtime.graph != nil && runtime.carrier != nil && runtime.points != nil && runtime.topology != nil &&
		runtime.graph.RegionCount() == 0 && runtime.graph.Schedule() != nil && runtime.graph.Schedule().RegionCount() == 0 &&
		runtimeSelectedOverlayRowsValid(runtime)
}

func runtimeSelectedOverlayRowsValid(runtime *solverRuntime) bool {
	return runtimeSelectedOverlayShapeValid(runtime) &&
		runtimeSelectedOverlayIndexCachesValid(runtime) &&
		runtimeSelectedOverlayRecurrenceFree(runtime)
}

func runtimeSelectedOverlayShapeValid(runtime *solverRuntime) bool {
	return runtime != nil && runtime.graph != nil &&
		len(runtime.factorIncoming) == runtime.graph.PointCount() &&
		len(runtime.overlay.factorOutgoing) == runtime.graph.PointCount()
}

func runtimeSelectedOverlayIndexCachesValid(runtime *solverRuntime) bool {
	if runtime == nil {
		return false
	}
	overlay := runtime.overlay
	return overlay.factorByKey != nil && overlay.staticOrigins != nil && overlay.originAt != nil && overlay.directAt != nil &&
		overlay.dependencyAt != nil && overlay.reindexes.scopes != nil &&
		overlay.reindexes.plans != nil && overlay.reindexes.decisions != nil &&
		overlay.latePlans != nil && overlay.generation.Available()
}

func runtimeSelectedOverlayRecurrenceFree(runtime *solverRuntime) bool {
	return runtime != nil && len(runtime.regions) == 0 &&
		len(runtime.activeRegions) == 0 && len(runtime.regionChildren) == 0
}
