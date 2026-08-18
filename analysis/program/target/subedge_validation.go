package target

import (
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
)

func validateSubedgeEntries(edges []subedgeDraft, callbacks []callbackDraft) error {
	callbackEdges := make([]int, len(callbacks))
	inbound := make([][]*subedgeRouteDraft, len(edges))
	for index := range edges {
		edge := &edges[index]
		if edge.callee == vocabulary.SubedgeCalleeCallback {
			if uint64(edge.callbackRank) >= uint64(len(callbackEdges)) {
				return errors.New("target: malformed callback rank")
			}
			callbackEdges[edge.callbackRank]++
		}
		for terminal := range edge.routes {
			collectInboundSubedgeRoute(inbound, &edge.routes[terminal])
		}
		collectInboundSubedgeRoute(inbound, &edge.admissionRoute)
	}
	for index := range edges {
		edge := &edges[index]
		if edge.callee == vocabulary.SubedgeCalleeCallback && callbackEdges[edge.callbackRank] != 1 {
			return errors.New("target: callback has multiple direct subedges")
		}
		if len(edge.argumentOrigins) != 0 {
			if edge.ruleEntry {
				return errors.New("target: argument origins carry redundant RuleEntry")
			}
			if len(inbound[index]) != 0 {
				return errors.New("target: route-fed arguments also carry direct origins")
			}
			continue
		}
		if edge.ruleEntry {
			// A nullary RuleEntry has no operand segment to merge. It remains a
			// direct owner-Rule root even when a resolved route re-enters the same
			// empty-product application as a finite Mu head.
			continue
		}
		if len(inbound[index]) == 0 {
			return errors.New("target: subedge has no entry authority")
		}
		for _, route := range inbound[index] {
			if !routeCompletelyFeedsArguments(*route, edge.arguments) {
				return errors.New("target: route-fed arguments are partial")
			}
		}
	}
	for rank, callback := range callbacks {
		count := callbackEdges[rank]
		if count > 1 {
			return errors.New("target: callback has multiple direct subedges")
		}
		if !retainedCallbackLifecycle(callback.lifecycle) && count != 1 {
			return errors.New("target: Sync callback lacks direct Subedge")
		}
	}
	return nil
}

// validateSubedgeRecurrence discharges lifecycle multiplicity against the
// resolved local application graph. It has no execution, recursion depth, or
// visit budget: iterative reachability and SCC decomposition terminate from
// the finite sealed Subedge table alone.
func validateSubedgeRecurrence(edges []subedgeDraft, callbacks []callbackDraft) error {
	if len(edges) == 0 {
		return nil
	}
	outgoing := make([][]int, len(edges))
	incoming := make([][]int, len(edges))
	addRoute := func(from int, route subedgeRouteDraft) error {
		if route.route != vocabulary.RouteSubedge && (route.route != vocabulary.RouteRejectYield || route.subedge == 0) {
			return nil
		}
		if uint64(route.subedgeRank) >= uint64(len(edges)) {
			return errors.New("target: malformed recurrence sibling")
		}
		to := int(route.subedgeRank)
		outgoing[from] = append(outgoing[from], to)
		incoming[to] = append(incoming[to], from)
		return nil
	}
	for index, edge := range edges {
		if err := addRoute(index, edge.admissionRoute); err != nil {
			return err
		}
		for _, route := range edge.routes {
			if err := addRoute(index, route); err != nil {
				return err
			}
		}
	}

	// Direct argument authority and the explicit nullary RuleEntry are the only
	// operation-local entry set. A route-fed-only island is not executable.
	reachable := make([]bool, len(edges))
	work := make([]int, 0, len(edges))
	for index, edge := range edges {
		if len(edge.argumentOrigins) == 0 && !edge.ruleEntry {
			continue
		}
		reachable[index] = true
		work = append(work, index)
	}
	for len(work) != 0 {
		index := work[len(work)-1]
		work = work[:len(work)-1]
		for _, successor := range outgoing[index] {
			if reachable[successor] {
				continue
			}
			reachable[successor] = true
			work = append(work, successor)
		}
	}
	for index := range edges {
		if !reachable[index] {
			return fmt.Errorf("target: Subedge role %d has no executable entry authority", edges[index].role)
		}
	}

	// Kosaraju's two finite graph walks are iterative to keep cyclic authoring
	// structure entirely outside Go call-stack behavior.
	seen := make([]bool, len(edges))
	order := make([]int, 0, len(edges))
	type frame struct{ edge, next int }
	for start := range edges {
		if !reachable[start] || seen[start] {
			continue
		}
		seen[start] = true
		stack := []frame{{edge: start}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < len(outgoing[top.edge]) {
				successor := outgoing[top.edge][top.next]
				top.next++
				if reachable[successor] && !seen[successor] {
					seen[successor] = true
					stack = append(stack, frame{edge: successor})
				}
				continue
			}
			order = append(order, top.edge)
			stack = stack[:len(stack)-1]
		}
	}

	assigned := make([]bool, len(edges))
	for orderIndex := len(order) - 1; orderIndex >= 0; orderIndex-- {
		start := order[orderIndex]
		if assigned[start] {
			continue
		}
		component := make([]int, 0, 1)
		stack := []int{start}
		assigned[start] = true
		for len(stack) != 0 {
			index := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, index)
			for _, predecessor := range incoming[index] {
				if reachable[predecessor] && !assigned[predecessor] {
					assigned[predecessor] = true
					stack = append(stack, predecessor)
				}
			}
		}
		if len(component) == 1 && !subedgeSelfReenters(component[0], outgoing) {
			continue
		}
		for _, index := range component {
			edge := edges[index]
			if edge.callee != vocabulary.SubedgeCalleeCallback || uint64(edge.callbackRank) >= uint64(len(callbacks)) {
				continue
			}
			if onceCallbackLifecycle(callbacks[edge.callbackRank].lifecycle) {
				return fmt.Errorf("target: Once callback direct Subedge role %d re-enters through a reachable cycle", edge.role)
			}
		}
	}
	return nil
}

func subedgeSelfReenters(index int, outgoing [][]int) bool {
	for _, successor := range outgoing[index] {
		if successor == index {
			return true
		}
	}
	return false
}

func collectInboundSubedgeRoute(inbound [][]*subedgeRouteDraft, route *subedgeRouteDraft) {
	if route.route != vocabulary.RouteSubedge && (route.route != vocabulary.RouteRejectYield || route.subedge == 0) {
		return
	}
	if uint64(route.subedgeRank) >= uint64(len(inbound)) {
		return
	}
	inbound[route.subedgeRank] = append(inbound[route.subedgeRank], route)
}

func argumentSegmentCount(values valuesDraft) int {
	count := len(values.types) + len(values.suffix)
	if values.tail == vocabulary.ValuesVariable {
		count++
	}
	return count
}

func routeCompletelyFeedsArguments(route subedgeRouteDraft, destination valuesDraft) bool {
	switch route.placement {
	case vocabulary.PlacementFixed:
		return route.offset == 0 && route.result.tail == vocabulary.ValuesClosed && destination.tail == vocabulary.ValuesClosed &&
			len(route.result.types) == len(destination.types) && len(route.result.suffix) == 0 && len(destination.suffix) == 0
	case vocabulary.PlacementTail:
		return route.adjustment == vocabulary.AdjustmentPreserve && pureValuesTail(route.result) && pureValuesTail(destination) &&
			route.result.varID == destination.varID
	default:
		return false
	}
}

func zeroLiteral(value keyspace.LiteralValue) bool { return value == (keyspace.LiteralValue{}) }

func normalizeRequiredExactKey(value keyspace.LiteralValue) (keyspace.LiteralValue, error) {
	normalized, ok := scalar.Normalize(value)
	if !ok {
		return keyspace.LiteralValue{}, errors.New("not an exact Lua key")
	}
	return normalized, nil
}

func emptyCapturedInitialRead(read vocabulary.CapturedInitialReadSpec) bool {
	return read.Root == "" && zeroLiteral(read.Key)
}

func emptySubedgeCallee(callee vocabulary.SubedgeCalleeSpec) bool {
	return callee.Kind == vocabulary.SubedgeCalleeInvalid && callee.Callback == 0 &&
		emptyCapturedInitialRead(callee.Read) && zeroLiteral(callee.MetaKey)
}

func emptyValuesSpec(values vocabulary.ValuesSpec) bool {
	return len(values.Fixed) == 0 && values.Tail == 0 && values.Var == 0 && !values.TailType.Available() && len(values.Suffix) == 0
}

func compareSubedgeIdentity(left, right subedgeDraft) int {
	if left.role < right.role {
		return -1
	}
	if left.role > right.role {
		return 1
	}
	if left.family < right.family {
		return -1
	}
	if left.family > right.family {
		return 1
	}
	if left.callee < right.callee {
		return -1
	}
	if left.callee > right.callee {
		return 1
	}
	if left.callbackRank < right.callbackRank {
		return -1
	}
	if left.callbackRank > right.callbackRank {
		return 1
	}
	if left.readRoot < right.readRoot {
		return -1
	}
	if left.readRoot > right.readRoot {
		return 1
	}
	if order := compareNormalizedKey(left.readKey, right.readKey); order != 0 {
		return order
	}
	if order := compareNormalizedKey(left.metaKey, right.metaKey); order != 0 {
		return order
	}
	if left.admission < right.admission {
		return -1
	}
	if left.admission > right.admission {
		return 1
	}
	if order := compareValues(left.arguments, right.arguments); order != 0 {
		return order
	}
	for index := range left.outcomes {
		if order := compareValues(left.outcomes[index], right.outcomes[index]); order != 0 {
			return order
		}
	}
	return 0
}

func compareNormalizedKey(left, right keyspace.LiteralValue) int {
	if zeroLiteral(left) {
		if zeroLiteral(right) {
			return 0
		}
		return -1
	}
	if zeroLiteral(right) {
		return 1
	}
	order, ok := scalar.Compare(left, right)
	if !ok {
		panic("target: unnormalized exact key")
	}
	return order
}
