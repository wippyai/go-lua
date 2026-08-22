package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	demandpkg "github.com/wippyai/go-lua/analysis/engine/internal/demand"
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
	// execution and demand are the revised epoch-local views. Region rows are
	// bound to the existing carrier; no typed root or sealed table is migrated.
	execution       *schedule.Schedule
	executionDemand *equation.Demand
	demandEpoch     *demandpkg.Epoch
	selectedPoints  []int
	activePoints    []bool
	// directCatalog is the complete installed selected-direct descriptor set
	// after this frontier (indexed by runtime factor edge). It is retained only
	// until the prepared overlay is committed, then published in overlay.directAt.
	directCatalog map[int]equation.SelectedStructuralFactorEdge
	// step is the first inner refusal boundary this preparation recorded. It
	// travels to the caller as the site of an incomplete solve, so a refused
	// overlay never reports an unsited compile failure.
	step           solveBoundary
	directEdges    []equation.SelectedStructuralFactorEdge
	regions        []runtimeRegion
	regionChildren [][]int
	pointRegion    []int
	activeRegions  []bool
	// Artifact counterparts are compact StateOrdinal rows. The graph-shaped
	// fields above remain the non-artifact construction's view and detached
	// metadata; mounted installation consumes only these lifted rows.
	stateExecution       *schedule.Schedule
	stateExecutionEvents []schedule.Event
	stateTargets         []int
	stateSelected        []int
	stateActive          []bool
	stateFactorIncoming  [][]int
	stateFactorOutgoing  [][]int
	stateFactorRows      []runtimeStateFactorRow
	stateRegions         []runtimeRegion
	stateRegionChildren  [][]int
	statePointRegion     []int
	stateActiveRegions   []bool
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
func (runtime *solverRuntime) prepareSelectedFactorOverlay(delta []equation.AcceptedMember, published equation.Relation) (*preparedSelectedFactorOverlay, bool, solveBoundary) {
	if !runtimeSelectedOverlayEligible(runtime) {
		return nil, false, selectedOverlayRefused("eligibility")
	}
	if len(delta) == 0 || !canonicalAcceptedActivations(delta) || !validAcceptedActivations(runtime.topology, delta) || !published.OwnedBy(runtime.topology) {
		return nil, false, selectedOverlayRefused("delta")
	}
	selected, materialized := runtime.topology.SelectedStructuralFactorEdges(runtime.graph, delta)
	if !materialized || len(selected) == 0 {
		return nil, false, selectedOverlayRefused("materialize")
	}
	descriptors, dependencyEdges, dependencyAt, dependencyChanged, execution, valid := runtime.prevalidateSelectedFactorEdges(selected)
	if !valid {
		return nil, false, selectedOverlayRefused("prevalidate")
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
			return nil, false, selectedOverlayRefused("bind")
		}
		if installed, present := runtime.overlay.originAt[bound.origin]; present {
			if !runtime.validSelectedOrigin(installed, bound.origin) {
				return nil, false, selectedOverlayRefused("origin")
			}
			previous := runtime.factorEdges[installed]
			if !previous.input.pre.Entails(bound.edge.input.pre) {
				return nil, false, selectedOverlayRefused("precondition")
			}
			if previous.input.pre.Equal(bound.edge.input.pre) {
				continue
			}
			if !appendPreparedReplacement(prepared, installed, bound.edge) {
				return nil, false, selectedOverlayRefused("replacement")
			}
			prepared.directCatalog[installed] = descriptor.edge
			continue
		}
		if _, static := runtime.overlay.staticOrigins[bound.origin]; static {
			return nil, false, selectedOverlayRefused("static-origin")
		}
		if _, duplicate := prepared.newOrigins[bound.origin]; duplicate {
			return nil, false, selectedOverlayRefused("duplicate-origin")
		}
		bound.edge.index = prepared.previousEdgeCount + len(prepared.additions)
		prepared.newOrigins[bound.origin] = bound.edge.index
		prepared.additions = append(prepared.additions, preparedFactorAddition{edge: bound.edge, origin: bound.origin})
		prepared.directCatalog[bound.edge.index] = descriptor.edge
	}
	if len(prepared.additions) == 0 && len(prepared.replacements) == 0 {
		return nil, false, selectedOverlayRefused("empty-delta")
	}
	if !prepared.finalize(runtime) {
		return nil, false, prepared.refusal(selectedOverlayRefused("finalize"))
	}
	prepared.directEdges = directCatalogEdges(prepared.directCatalog)
	if !prepared.bindFeedbackRuntime(runtime) {
		return nil, false, prepared.refusal(selectedOverlayRefused("feedback"))
	}
	return prepared, true, boundaryNone
}

// selectedOverlayRefused names one refusal step of the selected-edge overlay:
// the pipeline that compiles an accepted activation revision into a prepared
// structural delta. The step name enters the site digest, so an incomplete
// solve that stopped here reports which overlay boundary refused instead of an
// unsited compile failure.
func selectedOverlayRefused(step string) solveBoundary {
	return refused(SolveFailureFamilyCompile, "selected-overlay/"+step)
}

// refusal returns the deepest step this preparation recorded, falling back to
// the caller's own step when no inner boundary named itself.
func (prepared *preparedSelectedFactorOverlay) refusal(outer solveBoundary) solveBoundary {
	if prepared == nil || !prepared.step.available() {
		return outer
	}
	return prepared.step
}

// refuse records the first inner refusal step of one preparation and reports
// the false its caller returns.
func (prepared *preparedSelectedFactorOverlay) refuse(step string) bool {
	if prepared != nil && !prepared.step.available() {
		prepared.step = selectedOverlayRefused(step)
	}
	return false
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

// validSelectedActivationContext authenticates the complete transition tuple
// against the sealed Link directory's activation relation and resolves exactly
// one source/target StateOrdinal pair. It never searches same-module contexts
// or fans out a graph-point edge.
func (runtime *solverRuntime) validSelectedActivationContext(context equation.ActivationContext, source, target int) bool {
	if runtime == nil || !runtime.artifactBacked {
		return context.Empty()
	}
	if runtime.graph == nil || runtime.executionPlan == nil || !runtime.executionPlan.Available() || !context.Available() || source < 0 || target < 0 || source >= runtime.graph.PointCount() || target >= runtime.graph.PointCount() {
		return false
	}
	from, fromOK := runtime.contexts.Context(context.FromContextID)
	to, toOK := runtime.contexts.Context(context.ToContextID)
	transition, transitionOK := runtime.contexts.ActivationEdge(context.FromContextID, context.ToContextID)
	if !fromOK || !toOK || !from.Available() || !to.Available() || !transitionOK || !transition.Available() ||
		transition.ID() != context.TransitionID || transition.LinkID() != runtime.contexts.LinkID() {
		return false
	}
	sourceOwner, sourceOwnerOK := runtime.contextLayout.PointOwnerAt(contextfiber.PointOrdinal(source))
	targetOwner, targetOwnerOK := runtime.contextLayout.PointOwnerAt(contextfiber.PointOrdinal(target))
	if !sourceOwnerOK || !targetOwnerOK || !sourceOwner.Mounted() || !targetOwner.Mounted() ||
		sourceOwner.ModuleKey() != from.ModuleKey() || targetOwner.ModuleKey() != to.ModuleKey() {
		return false
	}
	fromOrdinal, fromOrdinalOK := runtime.contextIndex.ContextOrdinal(context.FromContextID)
	toOrdinal, toOrdinalOK := runtime.contextIndex.ContextOrdinal(context.ToContextID)
	if !fromOrdinalOK || !toOrdinalOK {
		return false
	}
	sourceState, sourceStateOK := runtime.executionPlan.Lookup(fromOrdinal, contextfiber.PointOrdinal(source))
	targetState, targetStateOK := runtime.executionPlan.Lookup(toOrdinal, contextfiber.PointOrdinal(target))
	if !sourceStateOK || !targetStateOK {
		return false
	}
	sourceCell, sourceCellOK := runtime.executionPlan.StateAt(sourceState)
	targetCell, targetCellOK := runtime.executionPlan.StateAt(targetState)
	sourceContext, sourceContextOK := sourceCell.ContextOrdinal()
	targetContext, targetContextOK := targetCell.ContextOrdinal()
	sourcePoint, sourcePointOK := sourceCell.PointOrdinal()
	targetPoint, targetPointOK := targetCell.PointOrdinal()
	return sourceCellOK && targetCellOK && sourceContextOK && targetContextOK && sourcePointOK && targetPointOK &&
		sourceContext == fromOrdinal && targetContext == toOrdinal && sourcePoint == contextfiber.PointOrdinal(source) && targetPoint == contextfiber.PointOrdinal(target)
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
		record, factorOK := runtime.program.factorRecordByKey(edge.Factor())
		slot := record.slot
		if !sourceOK || !targetOK || !factorOK || slot < 0 || int(slot) >= runtime.carrier.Count() || source < 0 || target < 0 || source >= runtime.graph.PointCount() || target >= runtime.graph.PointCount() {
			return nil, nil, nil, false, nil, false
		}
		if !runtime.validSelectedActivationContext(edge.Context(), source, target) {
			return nil, nil, nil, false, nil, false
		}
		// ContextTransport is the sole Link-owned authority for an exact
		// cross-context Point pair. A selected structural factor is a later
		// overlay admission and cannot add a second carrier for that pair, even
		// when its factor or provenance differs from the authenticated transport.
		if runtime.contextTransportOwnsPointPair(source, target) {
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

// contextTransportOwnsPointPair reports exact graph-point ownership by the
// immutable Link transport table. It intentionally ignores module names,
// contexts, factors, and provenance: selected admission is refused whenever
// the pair is already semantically owned, so no second authority can be
// installed through a dynamic overlay.
func (runtime *solverRuntime) contextTransportOwnsPointPair(source, target int) bool {
	if runtime == nil || source < 0 || target < 0 {
		return false
	}
	for _, transport := range runtime.contextTransports {
		if transport.sourcePoint == source && transport.targetPoint == target {
			return true
		}
	}
	return false
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
		execution := runtime.execution
		if execution == nil {
			execution = runtime.graph.Schedule()
		}
		return runtime.overlay.dependencyEdges, nil, false, execution, true
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
	record, factorOK := runtime.program.factorRecordByKey(edge.Factor())
	slot := record.slot
	sourceScope, sourceScoped := runtime.overlay.reindexes.scope(input.Source().Scope())
	targetScope, targetScoped := runtime.overlay.reindexes.scope(input.Target().Scope())
	if !edge.Available() || !input.Available() || !factorOK || slot < 0 || int(slot) >= runtime.carrier.Count() || !sourceScoped || !targetScoped || !sourceScope.Same(runtime.pointScopes[descriptor.source]) || !targetScope.Same(runtime.pointScopes[descriptor.target]) {
		return boundSelectedFactorEdge{}, false
	}
	planKey := input.Reindex().Key()
	context := edge.Context()
	var fromContext, toContext contextfiber.ContextOrdinal
	if context.Available() {
		var fromOK, toOK bool
		fromContext, fromOK = runtime.contextIndex.ContextOrdinal(context.FromContextID)
		toContext, toOK = runtime.contextIndex.ContextOrdinal(context.ToContextID)
		if !fromOK || !toOK {
			return boundSelectedFactorEdge{}, false
		}
	}
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
	origin, originOK := runtimeFactorEdgeOrigin(descriptor.source, descriptor.target, edge.Factor(), input.Provenance(), planKey, post, context)
	if !planKey.Available() || !planOK || !preOK || !postOK || !bound.valid() || !originOK {
		return boundSelectedFactorEdge{}, false
	}
	return boundSelectedFactorEdge{edge: runtimeFactorEdge{key: edge.Key(), factor: edge.Factor(), source: descriptor.source, target: descriptor.target, input: bound, slot: slot, context: context, fromContext: fromContext, toContext: toContext}, origin: origin}, true
}

func (prepared *preparedSelectedFactorOverlay) finalize(runtime *solverRuntime) bool {
	if prepared == nil || runtime == nil || prepared.runtime != runtime || prepared.previousEdgeCount != len(runtime.factorEdges) {
		return prepared.refuse("finalize-instance")
	}
	for index := range prepared.replacements {
		replacement := &prepared.replacements[index]
		if replacement.index < 0 || replacement.index >= prepared.previousEdgeCount {
			return prepared.refuse("finalize-replacement-index")
		}
		replacement.edge.index = replacement.index
	}
	if !prepared.prepareEdgeBacking(runtime) || !prepared.prepareTouchedCSR(runtime) || !prepared.collectTargets(runtime.activePoints) {
		return prepared.refuse("finalize-backing")
	}
	return true
}

// bindFeedbackRuntime uses the prepared schedule plus the exact direct-edge
// delta as a recurrence certificate. Dense Point/Group identities and every
// typed carrier root remain owned by the live runtime; no accepted Graph is
// reconstructed or used as a fake oracle.
func (prepared *preparedSelectedFactorOverlay) bindFeedbackRuntime(runtime *solverRuntime) bool {
	if prepared == nil || runtime == nil || runtime.graph == nil || runtime.carrier == nil || prepared.runtime != runtime || prepared.execution == nil || len(prepared.directEdges) != len(prepared.directCatalog) {
		return prepared.refuse("feedback-instance")
	}
	oracle, ok := runtime.graph.ActivationGraphOverlay(prepared.execution, prepared.directEdges)
	if !ok || oracle == nil || oracle.Schedule() == nil || oracle.PointCount() != runtime.graph.PointCount() || oracle.GroupCount() != runtime.graph.GroupCount() || oracle.EnvironmentEdgeTotal() != runtime.graph.EnvironmentEdgeTotal() || oracle.FactorEdgeTotal() != runtime.graph.FactorEdgeTotal()+len(prepared.directEdges) {
		return prepared.refuse("feedback-oracle")
	}
	for index := 0; index < oracle.PointCount(); index++ {
		oldPoint, oldOK := runtime.graph.PointAt(schedule.Node(index))
		newPoint, newOK := oracle.PointAt(schedule.Node(index))
		if !oldOK || !newOK || oldPoint.Key() != newPoint.Key() {
			return prepared.refuse("feedback-point-identity")
		}
	}
	for index := 0; index < oracle.GroupCount(); index++ {
		group, groupOK := oracle.HyperedgeAt(index)
		if !groupOK || index >= len(runtime.producers) || runtime.producers[index].group.Key() != group.Key() {
			return prepared.refuse("feedback-group-identity")
		}
	}
	// Keep all currently live points as exact roots and add every selected
	// target. DemandWithPoints then follows the same immutable Group,
	// environment, factor, and activation reverse relation on the overlay.
	// This is the widening cut: newly reachable points/groups are selected
	// without reconstructing a compiler or carrier composition.
	roots := make([]equation.Point, 0, len(runtime.activePoints)+len(prepared.directEdges))
	for index, active := range runtime.activePoints {
		if !active {
			continue
		}
		point, pointOK := oracle.PointAt(schedule.Node(index))
		if !pointOK {
			return prepared.refuse("feedback-root-point")
		}
		roots = append(roots, point)
	}
	for _, selected := range prepared.directEdges {
		if !selected.Available() || !oracle.OwnsPoint(selected.Target()) {
			return prepared.refuse("feedback-selected-target")
		}
		roots = append(roots, selected.Target())
	}
	demanded, demandedOK := oracle.DemandWithPoints(roots)
	activePoints, active, activeOK := runtimeDemandMembership(oracle, demanded)
	if !demandedOK || demanded == nil || !activeOK || len(activePoints) != len(runtime.activePoints) || len(active) != oracle.RegionCount() {
		return prepared.refuse("feedback-demand")
	}
	for index, live := range activePoints {
		// Accepted revisions are demand-widening. Existing points can never be
		// dropped; newly live rows receive prepared epoch state below.
		if runtime.activePoints[index] && !live {
			return prepared.refuse("feedback-demand-narrows")
		}
	}
	selectedPoints := make([]int, 0, demanded.PointCount())
	for index := 0; index < demanded.PointCount(); index++ {
		point, pointOK := demanded.PointAt(index)
		pointIndex, indexed := oracle.PointIndex(point)
		if !pointOK || !indexed {
			return prepared.refuse("feedback-demand-point")
		}
		selectedPoints = append(selectedPoints, pointIndex)
	}
	factorAt := make(map[composition.Key]int, oracle.FactorEdgeTotal())
	for index, edge := range runtime.factorEdges {
		factorAt[edge.key] = index
	}
	for _, replacement := range prepared.replacements {
		if replacement.index < 0 || replacement.index >= prepared.previousEdgeCount {
			return prepared.refuse("feedback-replacement-index")
		}
		factorAt[replacement.edge.key] = replacement.index
	}
	for offset, addition := range prepared.additions {
		index := prepared.previousEdgeCount + offset
		if _, duplicate := factorAt[addition.edge.key]; duplicate {
			return prepared.refuse("feedback-duplicate-key")
		}
		factorAt[addition.edge.key] = index
	}
	regions, children, bound := bindRuntimeRegionsWithEdges(oracle, active, runtime.carrier, runtime.producers, runtime.overlay.reindexes, runtimeRegionEdgeResolver{
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
		return prepared.refuse("feedback-regions")
	}
	pointRegion := make([]int, oracle.PointCount())
	for index := range pointRegion {
		point, pointOK := oracle.PointAt(schedule.Node(index))
		region, regionOK := oracle.PointRegion(point)
		if !pointOK || !regionOK || region < schedule.NoRegion || region >= len(regions) {
			return prepared.refuse("feedback-point-region")
		}
		pointRegion[index] = region
	}
	prepared.execution = oracle.Schedule()
	prepared.executionDemand = demanded
	prepared.selectedPoints = selectedPoints
	prepared.activePoints = activePoints
	prepared.regions = regions
	prepared.regionChildren = children
	prepared.pointRegion = pointRegion
	prepared.activeRegions = active
	if runtime.artifactBacked {
		if !prepared.bindArtifactStateOverlay(runtime) {
			return prepared.refuse("feedback-state-overlay")
		}
	}
	return true
}

// bindArtifactStateOverlay lifts one accepted graph overlay through the
// retained execution plan.  Graph oracle rows above are cold derivation
// metadata only; every schedule, demand row, factor occurrence, and wake
// target used by a mounted epoch is produced here in StateOrdinal space.
func (prepared *preparedSelectedFactorOverlay) bindArtifactStateOverlay(runtime *solverRuntime) bool {
	if prepared == nil || runtime == nil || !runtime.artifactBacked || runtime.executionPlan == nil || !runtime.executionPlan.Available() {
		return prepared.refuse("state-instance")
	}
	stateCount := runtime.stateCount()
	if stateCount <= 0 || len(runtime.statePointRows) != runtime.graph.PointCount() {
		return prepared.refuse("state-shape")
	}
	active := append([]bool(nil), runtime.activeStates...)
	if len(active) != stateCount {
		return prepared.refuse("state-active-width")
	}
	targetStates := make(map[int]struct{}, len(prepared.additions)+len(prepared.replacements))
	// Every selected edge carries its complete transition tuple. Resolve its
	// one source and one target StateOrdinal directly; no graph-point row scan
	// or same-module context fan-out is legal in the artifact runtime.
	for _, edge := range selectedOverlayEdges(prepared) {
		context := edge.context
		if !context.Available() {
			return prepared.refuse("state-edge-context")
		}
		from, fromOK := runtime.contextIndex.ContextOrdinal(context.FromContextID)
		to, toOK := runtime.contextIndex.ContextOrdinal(context.ToContextID)
		if !fromOK || !toOK {
			return prepared.refuse("state-context-ordinal")
		}
		sourceState, sourceOK := runtime.executionPlan.Lookup(from, contextfiber.PointOrdinal(edge.source))
		targetState, targetOK := runtime.executionPlan.Lookup(to, contextfiber.PointOrdinal(edge.target))
		if !sourceOK || !targetOK || int(sourceState) >= stateCount || int(targetState) >= stateCount {
			return prepared.refuse("state-edge-lookup")
		}
		// The graph overlay roots this edge's target and reaches its source
		// through the edge itself. The lifted state rows carry that same
		// relation, so the target enters the demand set here and the closure
		// below admits the producer behind it.
		active[int(targetState)] = true
		targetStates[int(targetState)] = struct{}{}
	}

	// Build the compact factor transpose over the complete next edge backing.
	nextFactors := make([]runtimeFactorEdge, len(runtime.factorEdges)+len(prepared.additions))
	copy(nextFactors, runtime.factorEdges)
	for index, addition := range prepared.additions {
		nextFactors[len(runtime.factorEdges)+index] = addition.edge
	}
	for _, replacement := range prepared.replacements {
		if replacement.index < 0 || replacement.index >= len(nextFactors) {
			return prepared.refuse("state-replacement-index")
		}
		nextFactors[replacement.index] = replacement.edge
	}
	incoming, outgoing, factorRows, _, factorOK := buildStateFactorIndex(runtime.graph, runtime.executionPlan, nextFactors, true)
	if !factorOK {
		return prepared.refuse("state-factor-index")
	}

	// The immutable base plan supplies all static contextual edges. Selected
	// direct edges are lifted only where their endpoint owners prove an exact
	// state pair; mounted-to-global remains closed without a merge witness.
	edges := runtime.executionPlan.Edges()
	seen := make(map[[2]int]struct{}, len(edges)+len(prepared.additions)+len(prepared.replacements))
	for _, edge := range edges {
		seen[[2]int{int(edge.From), int(edge.To)}] = struct{}{}
	}
	for _, addition := range prepared.additions {
		pairs, pairsOK := runtime.liftGraphPairStates(addition.edge.source, addition.edge.target, addition.edge.context)
		if !pairsOK {
			return prepared.refuse("state-addition-lift")
		}
		for _, pair := range pairs {
			key := [2]int{int(pair.From), int(pair.To)}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			edges = append(edges, pair)
		}
	}
	for _, replacement := range prepared.replacements {
		pairs, pairsOK := runtime.liftGraphPairStates(replacement.edge.source, replacement.edge.target, replacement.edge.context)
		if !pairsOK {
			return prepared.refuse("state-replacement-lift")
		}
		for _, pair := range pairs {
			key := [2]int{int(pair.From), int(pair.To)}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			edges = append(edges, pair)
		}
	}
	sort.Slice(edges, func(left, right int) bool {
		if edges[left].From != edges[right].From {
			return edges[left].From < edges[right].From
		}
		return edges[left].To < edges[right].To
	})
	execution, err := schedule.Prepare(stateCount, edges)
	if err != nil || execution == nil {
		return prepared.refuse("state-schedule")
	}
	// One demand authority closes both planes: the assembled frontier and this
	// widened overlay reach their fixed point over the same region-interval and
	// contextual-predecessor relation, here over the edge set the selected rows
	// just extended.
	if !closeStateDemand(execution, edges, active) {
		return prepared.refuse("state-demand-closure")
	}
	selected := make([]int, 0, stateCount)
	for stateIndex, admitted := range active {
		if admitted {
			selected = append(selected, stateIndex)
		}
	}
	targets := make([]int, 0, len(targetStates))
	for target := range targetStates {
		if target < 0 || target >= stateCount || !active[target] {
			return prepared.refuse("state-target")
		}
		targets = append(targets, target)
	}
	sort.Ints(targets)
	targets = uniqueInts(targets)
	if len(targets) == 0 {
		return prepared.refuse("state-target-empty")
	}
	stateRegions, stateChildren, pointRegion, activeRegions, stateEvents, lifted := liftStateRegions(runtime.graph, execution, active, runtime, factorRows, true)
	if !lifted {
		return prepared.refuse("state-regions")
	}
	prepared.stateExecution = execution
	prepared.stateExecutionEvents = stateEvents
	prepared.stateTargets = targets
	prepared.stateSelected = selected
	prepared.stateActive = active
	prepared.stateFactorIncoming = incoming
	prepared.stateFactorOutgoing = outgoing
	prepared.stateFactorRows = factorRows
	prepared.stateRegions = stateRegions
	prepared.stateRegionChildren = stateChildren
	prepared.statePointRegion = pointRegion
	prepared.stateActiveRegions = activeRegions
	return true
}

func selectedOverlayEdges(prepared *preparedSelectedFactorOverlay) []runtimeFactorEdge {
	if prepared == nil {
		return nil
	}
	result := make([]runtimeFactorEdge, 0, len(prepared.additions)+len(prepared.replacements))
	for _, addition := range prepared.additions {
		result = append(result, addition.edge)
	}
	for _, replacement := range prepared.replacements {
		result = append(result, replacement.edge)
	}
	return result
}

func uniqueInts(values []int) []int {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func (runtime *solverRuntime) liftGraphPairStates(sourcePoint, targetPoint int, context equation.ActivationContext) ([]schedule.Edge, bool) {
	if runtime == nil || !runtime.artifactBacked || runtime.executionPlan == nil || !context.Available() || !runtime.validSelectedActivationContext(context, sourcePoint, targetPoint) {
		return nil, false
	}
	from, fromOK := runtime.contextIndex.ContextOrdinal(context.FromContextID)
	to, toOK := runtime.contextIndex.ContextOrdinal(context.ToContextID)
	if !fromOK || !toOK {
		return nil, false
	}
	sourceState, sourceOK := runtime.executionPlan.Lookup(from, contextfiber.PointOrdinal(sourcePoint))
	targetState, targetOK := runtime.executionPlan.Lookup(to, contextfiber.PointOrdinal(targetPoint))
	if !sourceOK || !targetOK {
		return nil, false
	}
	return []schedule.Edge{{From: schedule.Node(sourceState), To: schedule.Node(targetState)}}, true
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
		seen[addition.edge.target] = struct{}{}
	}
	for _, replacement := range prepared.replacements {
		if replacement.edge.target < 0 || replacement.edge.target >= len(active) {
			return false
		}
		seen[replacement.edge.target] = struct{}{}
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
		runtime.graph.Schedule() != nil &&
		runtimeSelectedOverlayRowsValid(runtime)
}

func runtimeSelectedOverlayRowsValid(runtime *solverRuntime) bool {
	return runtimeSelectedOverlayShapeValid(runtime) &&
		runtimeSelectedOverlayIndexCachesValid(runtime)
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
	return runtime.program.valid() && overlay.staticOrigins != nil && overlay.originAt != nil && overlay.directAt != nil &&
		overlay.dependencyAt != nil && overlay.reindexes.scopes != nil &&
		overlay.reindexes.plans != nil && overlay.reindexes.decisions != nil &&
		overlay.latePlans != nil && overlay.generation.Available()
}

func validAcceptedActivations(topology *equation.Topology, accepted []equation.AcceptedMember) bool {
	return topology != nil && topology.ValidAccepted(accepted)
}
