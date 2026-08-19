// runtime_regions.go derives the static dependency edges, binds the regions and resolves demand membership.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// runtimeStaticDependencyEdges copies the already sealed Point influence
// relation once into dense form.  Selected structural edges are appended to
// this relation only while preparing an overlay; schedule.Prepare then gives
// one batched cycle proof for static plus previously selected plus new edges.
// Group input and designated environment input dependencies both target the
// Group output, while direct Environment/Factor rows target their own Point.
func runtimeStaticDependencyEdges(graph *equation.Graph) ([]schedule.Edge, map[[2]int]struct{}, bool) {
	if graph == nil || graph.Schedule() == nil || graph.PointCount() == 0 {
		return nil, nil, false
	}
	seen := make(map[[2]int]struct{})
	edges := make([]schedule.Edge, 0, graph.EnvironmentEdgeTotal()+graph.FactorEdgeTotal()+graph.GroupCount())
	appendEdge := func(source, target int) bool {
		if source < 0 || target < 0 || source >= graph.PointCount() || target >= graph.PointCount() {
			return false
		}
		key := [2]int{source, target}
		if _, duplicate := seen[key]; !duplicate {
			seen[key] = struct{}{}
			edges = append(edges, schedule.Edge{From: schedule.Node(source), To: schedule.Node(target)})
		}
		return true
	}
	for sourceIndex := 0; sourceIndex < graph.PointCount(); sourceIndex++ {
		source, sourceOK := graph.PointAt(schedule.Node(sourceIndex))
		if !sourceOK || !graph.OwnsPoint(source) {
			return nil, nil, false
		}
		for index := 0; index < graph.ConsumerCount(source); index++ {
			group, groupOK := graph.ConsumerAt(source, index)
			target, targetOK := group.Output(), graph.OwnsGroup(group)
			targetIndex, indexed := graph.PointIndex(target)
			if !groupOK || !targetOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
		for index := 0; index < graph.EnvironmentGroupCount(source); index++ {
			group, groupOK := graph.EnvironmentGroupAt(source, index)
			target, targetOK := group.Output(), graph.OwnsGroup(group)
			targetIndex, indexed := graph.PointIndex(target)
			if !groupOK || !targetOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
		for index := 0; index < graph.EnvironmentOutgoingCount(source); index++ {
			edge, edgeOK := graph.EnvironmentOutgoingAt(source, index)
			if edgeOK && edge.TransportOnly() {
				continue
			}
			targetIndex, indexed := graph.PointIndex(edge.Target())
			if !edgeOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
		for index := 0; index < graph.FactorOutgoingCount(source); index++ {
			edge, edgeOK := graph.FactorOutgoingAt(source, index)
			targetIndex, indexed := graph.PointIndex(edge.Target())
			if !edgeOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
	}
	sort.Slice(edges, func(left, right int) bool {
		if edges[left].From != edges[right].From {
			return edges[left].From < edges[right].From
		}
		return edges[left].To < edges[right].To
	})
	return edges, seen, true
}

// bindRuntimeRegions is the recurrence admission cut for the one already
// sealed Demand.  A disconnected Region has no runtime meaning for this
// Solve: it receives neither scopes nor an episode, so an unranked unrelated
// cycle cannot poison a demanded query.  Every active Region still binds its
// complete static M_K footprint before Work opens.
type runtimeRegionEdgeResolver struct {
	environment func(equation.EnvironmentEdgeNode) (int, bool)
	factor      func(equation.FactorEdgeNode) (int, bool)
	runtime     bool
}

func bindRuntimeRegions(graph *equation.Graph, active []bool, runtime *carrier.Composition, producers []runtimeProducer, plans runtimeReindexes) ([]runtimeRegion, [][]int, bool) {
	if graph == nil {
		return nil, nil, false
	}
	return bindRuntimeRegionsWithEdges(graph, active, runtime, producers, plans, runtimeRegionEdgeResolver{
		environment: graph.EnvironmentEdgeIndex,
		factor:      graph.FactorEdgeIndex,
	})
}

func bindRuntimeRegionsWithEdges(graph *equation.Graph, active []bool, runtime *carrier.Composition, producers []runtimeProducer, plans runtimeReindexes, edges runtimeRegionEdgeResolver) ([]runtimeRegion, [][]int, bool) {
	if graph == nil || runtime == nil || len(active) != graph.RegionCount() || len(producers) != graph.GroupCount() {
		return nil, nil, false
	}
	if edges.environment == nil || edges.factor == nil {
		return nil, nil, false
	}
	regions := make([]runtimeRegion, graph.RegionCount())
	for regionIndex := range regions {
		if !active[regionIndex] {
			continue
		}
		region, ok := graph.RegionAt(regionIndex)
		if !ok {
			return nil, nil, false
		}
		head, headOK := region.Head()
		headIndex, indexed := graph.PointIndex(head)
		if !headOK || !indexed {
			return nil, nil, false
		}
		bound := runtimeRegion{active: true, head: headIndex}
		parent, parentOK := region.Parent()
		if !parentOK || parent < schedule.NoRegion || parent >= graph.RegionCount() {
			return nil, nil, false
		}
		bound.parent = parent
		bound.points = make([]int, region.PointCount())
		for index := range bound.points {
			point, pointOK := region.PointAt(index)
			pointIndex, indexed := graph.PointIndex(point)
			if !pointOK || !indexed {
				return nil, nil, false
			}
			bound.points[index] = pointIndex
		}
		for index := 0; index < region.ExternalHeadProducerCount(); index++ {
			group, ok := region.ExternalHeadProducerAt(index)
			groupIndex, indexed := graph.GroupIndex(group)
			if !ok || !indexed || group.Output() != head {
				return nil, nil, false
			}
			bound.external = append(bound.external, groupIndex)
		}
		for index := 0; index < region.BackHeadProducerCount(); index++ {
			group, ok := region.BackHeadProducerAt(index)
			groupIndex, indexed := graph.GroupIndex(group)
			if !ok || !indexed || group.Output() != head {
				return nil, nil, false
			}
			bound.back = append(bound.back, groupIndex)
		}
		for index := 0; index < region.ExternalEnvironmentEdgeCount(); index++ {
			edge, edgeOK := region.ExternalEnvironmentEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.environment(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			bound.environmentExternal = append(bound.environmentExternal, edgeIndex)
		}
		for index := 0; index < region.BackEnvironmentEdgeCount(); index++ {
			edge, edgeOK := region.BackEnvironmentEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.environment(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			// A back edge is classified by its source/target placement in the
			// immutable WTO region, not by whether its boundary happens to be
			// coordinate identity.  The executor transports every environment
			// input before folding it, so a valid non-identity boundary remains a
			// lawful recurrence contribution here.
			bound.environmentBack = append(bound.environmentBack, edgeIndex)
		}
		// TransportOnly edges are intentionally absent from the immutable WTO
		// recurrence graph, but a self-transport targeting a region head still
		// contributes to that head's runtime RHS. Keep it in the same back
		// ingress row as ordinary environment back edges, so the operand plane
		// transposes it with them.
		for edgeIndex := 0; edgeIndex < graph.EnvironmentEdgeTotal(); edgeIndex++ {
			edge, edgeOK := graph.EnvironmentEdgeAtIndex(edgeIndex)
			if !edgeOK || !edge.TransportOnly() || edge.Target() != head {
				continue
			}
			boundIndex, indexed := edges.environment(edge)
			if !indexed || boundIndex < 0 {
				return nil, nil, false
			}
			bound.environmentBack = append(bound.environmentBack, boundIndex)
		}
		sort.Ints(bound.environmentBack)
		for index := 0; index < region.ExternalFactorEdgeCount(); index++ {
			edge, edgeOK := region.ExternalFactorEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.factor(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			bound.factorExternal = append(bound.factorExternal, edgeIndex)
		}
		for index := 0; index < region.BackFactorEdgeCount(); index++ {
			edge, edgeOK := region.BackFactorEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.factor(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			bound.factorBack = append(bound.factorBack, edgeIndex)
		}
		if len(bound.external)+len(bound.back) != graph.ProducerCount(head) {
			return nil, nil, false
		}
		// Factor membership is static CSR topology; consume every internal Group
		// once and attach only its occurrence-local authored targets. Route scopes
		// retain the owning Factor identity and flags here. Their O(R) universe is
		// expanded once below per active Region/Factor pair, never per Group.
		factorSeen := make(map[composition.Key]bool, region.FactorCount())
		regionRoutes := make(map[composition.Key]struct {
			factor      runtimeFactor
			route       bool
			narrowRoute bool
		}, region.FactorCount())
		for factorIndex := 0; factorIndex < region.FactorCount(); factorIndex++ {
			factor, ok := region.FactorAt(factorIndex)
			if !ok || !factor.Available() {
				return nil, nil, false
			}
			if _, duplicate := factorSeen[factor]; duplicate {
				return nil, nil, false
			}
			factorSeen[factor] = false
		}
		// A structural FactorEdge can be the only internal recurrence
		// contributor. It has no Group occurrence footprint, and it need not
		// target the WTO head, so account for the complete internal CSR row when
		// proving the Region's M_K set.
		for internalIndex := 0; internalIndex < region.InternalFactorEdgeCount(); internalIndex++ {
			edge, edgeOK := region.InternalFactorEdgeAt(internalIndex)
			if !edgeOK {
				return nil, nil, false
			}
			if _, known := factorSeen[edge.Factor()]; !known {
				return nil, nil, false
			}
			factorSeen[edge.Factor()] = true
		}
		targets, narrowTargets := make([]carrier.Target, 0), make([]carrier.Target, 0)
		for internalIndex := 0; internalIndex < region.InternalGroupCount(); internalIndex++ {
			group, groupOK := region.InternalHyperedgeAt(internalIndex)
			groupIndex, indexed := graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(producers) || producers[groupIndex].group.Key() != group.Key() {
				return nil, nil, false
			}
			if environmentInput, hasEnvironment := group.EnvironmentInput(); hasEnvironment {
				_, sourceIndexed := graph.PointIndex(environmentInput.Point())
				if !sourceIndexed {
					return nil, nil, false
				}
				// EnvironmentInput follows the same transport path as a
				// structural EnvironmentEdge.  Membership in the region determines
				// back-vs-external classification; transport identity is not a
				// recurrence admission requirement.
			}
			for _, occurrence := range producers[groupIndex].footprint {
				route := regionRoutes[occurrence.key]
				if _, known := factorSeen[occurrence.key]; !known {
					return nil, nil, false
				}
				factorSeen[occurrence.key] = true
				targets = append(targets, occurrence.targets...)
				narrowTargets = append(narrowTargets, occurrence.narrowTargets...)
				if occurrence.route {
					if occurrence.routeFactor == nil || compositionKeyOf(occurrence.routeFactor.semantic()) != occurrence.key {
						return nil, nil, false
					}
					if route.factor != nil && route.factor != occurrence.routeFactor {
						return nil, nil, false
					}
					route.factor = occurrence.routeFactor
					route.route = true
					route.narrowRoute = route.narrowRoute || occurrence.narrowRoute
					regionRoutes[occurrence.key] = route
				}
			}
		}
		for _, found := range factorSeen {
			if !found {
				return nil, nil, false
			}
		}
		// Expand each exact route universe at most once for this active Region.
		// runtimeFactor is the sealed owner capability, so the resulting targets
		// are still checked by the carrier's own composition/factor fence when
		// the recurrence scopes are sealed.
		for factorIndex := 0; factorIndex < region.FactorCount(); factorIndex++ {
			factor, ok := region.FactorAt(factorIndex)
			if !ok || !factor.Available() {
				return nil, nil, false
			}
			route := regionRoutes[factor]
			if !route.route {
				continue
			}
			if route.factor == nil || compositionKeyOf(route.factor.semantic()) != factor {
				return nil, nil, false
			}
			if !route.factor.hasRouteUniverse() {
				return nil, nil, false
			}
			universe := route.factor.routeUniverse()
			targets = append(targets, universe...)
			if route.narrowRoute {
				narrowTargets = append(narrowTargets, universe...)
			}
		}
		targets = compactRuntimeTargets(targets)
		narrowTargets = compactRuntimeTargets(narrowTargets)
		var widen, narrow carrier.MergeScope
		var widenOK, narrowOK bool
		if edges.runtime {
			widen, widenOK = runtime.SealRuntimeWidening(targets)
			narrow, narrowOK = runtime.SealRuntimeNarrowing(narrowTargets)
		} else {
			widen, widenOK = runtime.SealWidening(targets)
			narrow, narrowOK = runtime.SealNarrowing(narrowTargets)
		}
		if !widenOK || !narrowOK {
			return nil, nil, false
		}
		discharge, dischargeOK := sealRegionDischarge(graph, region, head, runtime, plans, edges.runtime)
		if !dischargeOK {
			return nil, nil, false
		}
		bound.widen, bound.narrow, bound.discharge = widen, narrow, discharge
		regions[regionIndex] = bound
	}
	// Parent is immutable graph topology.  The children row is only a compact
	// executor traversal aid for subtree cache invalidation; it never defines
	// recurrence membership or semantic evaluation.
	children := make([][]int, len(regions))
	for child, bound := range regions {
		if !active[child] {
			continue
		}
		parent := bound.parent
		if parent == schedule.NoRegion {
			continue
		}
		if parent < 0 || parent >= len(regions) || !active[parent] || !regions[parent].active {
			return nil, nil, false
		}
		children[parent] = append(children[parent], child)
	}
	return regions, children, true
}

// runtimeDemandMembership projects the existing equation.Demand into the two
// dense runtime masks.  It is a cold integrity check over the sole point WTO
// event stream, not a second dependency graph or activation mechanism.
func runtimeDemandMembership(graph *equation.Graph, points *equation.Demand) ([]bool, []bool, bool) {
	if graph == nil || points == nil || points.PointCount() == 0 || points.EventCount() == 0 {
		return nil, nil, false
	}
	activePoints := make([]bool, graph.PointCount())
	activeRegions := make([]bool, graph.RegionCount())
	entered := make([]bool, graph.RegionCount())
	exited := make([]bool, graph.RegionCount())
	stack := make([]int, 0, graph.RegionCount())
	for index := 0; index < points.EventCount(); index++ {
		event, _, ok := points.EventAt(index)
		if !ok {
			return nil, nil, false
		}
		switch event.Kind {
		case schedule.EventEnter:
			if event.Region < 0 || event.Region >= len(activeRegions) || entered[event.Region] {
				return nil, nil, false
			}
			region, regionOK := graph.RegionAt(event.Region)
			head, headOK := region.Head()
			headIndex, headIndexed := graph.PointIndex(head)
			parent, parentOK := region.Parent()
			if !regionOK || !headOK || !headIndexed || !parentOK || event.Node < 0 || headIndex != int(event.Node) {
				return nil, nil, false
			}
			if len(stack) == 0 {
				if parent != schedule.NoRegion {
					return nil, nil, false
				}
			} else if parent != stack[len(stack)-1] {
				return nil, nil, false
			}
			activeRegions[event.Region], entered[event.Region] = true, true
			stack = append(stack, event.Region)
		case schedule.EventExit:
			if event.Region < 0 || event.Region >= len(activeRegions) || !entered[event.Region] || exited[event.Region] || len(stack) == 0 || stack[len(stack)-1] != event.Region {
				return nil, nil, false
			}
			region, regionOK := graph.RegionAt(event.Region)
			head, headOK := region.Head()
			headIndex, headIndexed := graph.PointIndex(head)
			if !regionOK || !headOK || !headIndexed || event.Node < 0 || headIndex != int(event.Node) {
				return nil, nil, false
			}
			exited[event.Region] = true
			stack = stack[:len(stack)-1]
		case schedule.EventNode:
			if event.Node < 0 || int(event.Node) >= len(activePoints) {
				return nil, nil, false
			}
			if len(stack) == 0 {
				if event.Region != schedule.NoRegion {
					return nil, nil, false
				}
			} else if event.Region != stack[len(stack)-1] {
				return nil, nil, false
			}
			activePoints[event.Node] = true
		default:
			return nil, nil, false
		}
	}
	if len(stack) != 0 {
		return nil, nil, false
	}
	for index, active := range activeRegions {
		if active != (entered[index] && exited[index]) {
			return nil, nil, false
		}
	}
	for demandIndex := 0; demandIndex < points.PointCount(); demandIndex++ {
		point, pointOK := points.PointAt(demandIndex)
		pointIndex, indexed := graph.PointIndex(point)
		region, regionOK := graph.PointRegion(point)
		if !pointOK || !indexed || !regionOK || pointIndex < 0 || pointIndex >= len(activePoints) || !activePoints[pointIndex] {
			return nil, nil, false
		}
		for region != schedule.NoRegion {
			if region < 0 || region >= len(activeRegions) || !activeRegions[region] {
				return nil, nil, false
			}
			view, viewOK := graph.RegionAt(region)
			parent, parentOK := view.Parent()
			if !viewOK || !parentOK {
				return nil, nil, false
			}
			region = parent
		}
	}
	return activePoints, activeRegions, true
}

func runtimeContainsTarget(targets []carrier.Target, want carrier.Target) bool {
	for _, target := range targets {
		if target.Same(want) {
			return true
		}
	}
	return false
}

// unionRuntimeTargets returns base extended by every element of extra it does
// not already carry. base is never mutated: the binder folds member-owned
// immutable slices, so a union has to own its result.
func unionRuntimeTargets(base, extra []carrier.Target) []carrier.Target {
	if len(extra) == 0 {
		return base
	}
	result := append([]carrier.Target(nil), base...)
	for _, target := range extra {
		result = appendUniqueTarget(result, target)
	}
	return result
}

func compactRuntimeTargets(targets []carrier.Target) []carrier.Target {
	if len(targets) < 2 {
		return targets
	}
	sort.Slice(targets, func(left, right int) bool { return targets[left].Less(targets[right]) })
	end := 1
	for _, target := range targets[1:] {
		if !targets[end-1].Same(target) {
			targets[end] = target
			end++
		}
	}
	return targets[:end]
}

func lessRuntimeKey(left, right composition.Key) bool {
	for index := range left.ID {
		if left.ID[index] != right.ID[index] {
			return left.ID[index] < right.ID[index]
		}
	}
	return left.Version < right.Version
}
