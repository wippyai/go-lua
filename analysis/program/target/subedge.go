package target

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
)

// freezeSubedges seals the one Target-owned internal-application relation.
// It resolves local route references by dense table coordinates only: a local
// cycle is finite structural recurrence, later compiled to Mu, and is never
// followed by Go recursion here.
func (d *operationDraft) freezeSubedges(input []SubedgeSpec) ([]subedgeDraft, error) {
	if _, err := checkedStoredLength("subedge table", len(input)); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		if err := validateSubedgeEntries(nil, d.callbacks); err != nil {
			return nil, err
		}
		return nil, nil
	}
	callbackRanks, err := callbackRanks(d.callbacks)
	if err != nil {
		return nil, err
	}
	outcomeOrdinals, err := outcomeOrdinals(d.outcomes)
	if err != nil {
		return nil, err
	}
	out := make([]subedgeDraft, len(input))
	for index, item := range input {
		edge, freezeErr := d.freezeSubedge(index, item, callbackRanks, outcomeOrdinals)
		if freezeErr != nil {
			return nil, freezeErr
		}
		out[index] = edge
	}
	sort.Slice(out, func(left, right int) bool {
		return compareSubedgeIdentity(out[left], out[right]) < 0
	})
	for index := 1; index < len(out); index++ {
		if out[index-1].role == out[index].role {
			return nil, errors.New("target: duplicate subedge role")
		}
	}
	ranks := make([]uint32, len(out))
	for rank := range out {
		if out[rank].source < 0 || out[rank].source >= len(ranks) {
			return nil, errors.New("target: malformed subedge source")
		}
		ranks[out[rank].source] = uint32(rank)
	}
	for index := range out {
		for terminal := range out[index].routes {
			route := &out[index].routes[terminal]
			if err := resolveSubedgeRouteSibling(route, ranks, out); err != nil {
				return nil, fmt.Errorf("target: subedge %d route %d: %w", index, terminal, err)
			}
		}
		if err := resolveSubedgeRouteSibling(&out[index].admissionRoute, ranks, out); err != nil {
			return nil, fmt.Errorf("target: subedge %d admission failure: %w", index, err)
		}
	}
	for index := range out {
		if err := d.validateSubedge(&out[index], out); err != nil {
			return nil, fmt.Errorf("target: subedge %d: %w", index, err)
		}
	}
	if err := validateSubedgeEntries(out, d.callbacks); err != nil {
		return nil, err
	}
	if err := validateSubedgeRecurrence(out, d.callbacks); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveSubedgeRouteSibling(route *subedgeRouteDraft, ranks []uint32, all []subedgeDraft) error {
	if route.route != RouteSubedge && !(route.route == RouteRejectYield && route.subedge != 0) {
		return nil
	}
	if route.subedge == 0 || uint64(route.subedge) > uint64(len(ranks)) {
		return errors.New("sibling outside scope")
	}
	source := int(route.subedge - 1)
	rank := ranks[source]
	if uint64(rank) >= uint64(len(all)) || all[rank].source != source {
		return errors.New("malformed sibling")
	}
	route.subedgeRank = rank
	return nil
}

func callbackRanks(callbacks []callbackDraft) ([]uint32, error) {
	ranks := make([]uint32, len(callbacks))
	for rank, callback := range callbacks {
		if callback.source < 0 || callback.source >= len(ranks) {
			return nil, errors.New("target: malformed callback source")
		}
		ranks[callback.source] = uint32(rank)
	}
	return ranks, nil
}

func outcomeOrdinals(outcomes []outcomeDraft) ([]uint32, error) {
	ordinals := make([]uint32, len(outcomes))
	for ordinal, outcome := range outcomes {
		if outcome.source < 0 || outcome.source >= len(ordinals) {
			return nil, errors.New("target: malformed outcome source")
		}
		ordinals[outcome.source] = uint32(ordinal)
	}
	return ordinals, nil
}

func (d *operationDraft) freezeSubedge(source int, item SubedgeSpec, callbackRanks, outcomeOrdinals []uint32) (subedgeDraft, error) {
	if !validSubedgeFamily(item.Family) {
		return subedgeDraft{}, errors.New("invalid family")
	}
	if item.Role == 0 {
		return subedgeDraft{}, errors.New("zero semantic role")
	}
	edge := subedgeDraft{source: source, role: item.Role, family: item.Family}
	if item.Family == SubedgeFamilyCall {
		if err := d.freezeCallCallee(&edge, item, callbackRanks); err != nil {
			return subedgeDraft{}, err
		}
	} else {
		if !emptySubedgeCallee(item.Callee) {
			return subedgeDraft{}, errors.New("non-Call family carries callee")
		}
		if item.Admission != OrdinaryCallable {
			return subedgeDraft{}, errors.New("non-Call family requires ordinary callable admission")
		}
		edge.admission = item.Admission
		arguments, outcomes, endpointsErr := d.freezeInlineEndpoints(item.Arguments, item.Outcomes)
		if endpointsErr != nil {
			return subedgeDraft{}, endpointsErr
		}
		if err := validateClosedFamilyABI(item.Family, arguments); err != nil {
			return subedgeDraft{}, err
		}
		edge.arguments, edge.outcomes = arguments, outcomes
	}
	origins, originsErr := d.freezeSubedgeArgumentOrigins(item.ArgumentOrigins, edge.arguments)
	if originsErr != nil {
		return subedgeDraft{}, originsErr
	}
	if item.RuleEntry && (len(origins) != 0 || argumentSegmentCount(edge.arguments) != 0) {
		return subedgeDraft{}, errors.New("RuleEntry requires an empty argument product")
	}
	edge.ruleEntry = item.RuleEntry
	edge.argumentOrigins = origins
	routes, routesErr := d.freezeSubedgeRoutes(item.Routes, outcomeOrdinals)
	if routesErr != nil {
		return subedgeDraft{}, routesErr
	}
	edge.routes = routes
	failure, failureRoute, failureErr := d.freezeSubedgeAdmissionFailure(item.AdmissionFailure, outcomeOrdinals)
	if failureErr != nil {
		return subedgeDraft{}, failureErr
	}
	edge.admissionFailure, edge.admissionRoute = failure, failureRoute
	return edge, nil
}

func (d *operationDraft) freezeCallCallee(edge *subedgeDraft, item SubedgeSpec, callbackRanks []uint32) error {
	switch item.Callee.Kind {
	case SubedgeCalleeCallback:
		if item.Callee.Callback == 0 || uint64(item.Callee.Callback) > uint64(len(callbackRanks)) ||
			!emptyCapturedInitialRead(item.Callee.Read) || !zeroLiteral(item.Callee.MetaKey) ||
			item.Admission != AdmissionInvalid || !emptyValuesSpec(item.Arguments) || len(item.Outcomes) != 0 {
			return errors.New("malformed callback callee union")
		}
		source := int(item.Callee.Callback - 1)
		rank := callbackRanks[source]
		if uint64(rank) >= uint64(len(d.callbacks)) || d.callbacks[rank].source != source {
			return errors.New("callback callee outside scope")
		}
		callback := d.callbacks[rank]
		edge.callee, edge.callback, edge.callbackRank = SubedgeCalleeCallback, item.Callee.Callback, rank
		edge.arguments, edge.outcomes, edge.admission = callback.arguments, callback.outcomes, callback.admission
		return nil
	case SubedgeCalleeCapturedInitialRead:
		if item.Callee.Callback != 0 || !zeroLiteral(item.Callee.MetaKey) || item.Callee.Read.Root == "" ||
			!validAdmission(item.Admission) {
			return errors.New("malformed captured initial read callee union")
		}
		key, err := normalizeRequiredExactKey(item.Callee.Read.Key)
		if err != nil {
			return fmt.Errorf("captured initial read key: %w", err)
		}
		arguments, outcomes, endpointsErr := d.freezeInlineEndpoints(item.Arguments, item.Outcomes)
		if endpointsErr != nil {
			return endpointsErr
		}
		edge.callee, edge.readRoot, edge.readKey = SubedgeCalleeCapturedInitialRead, item.Callee.Read.Root, key
		edge.arguments, edge.outcomes, edge.admission = arguments, outcomes, item.Admission
		return nil
	case SubedgeCalleeMetaKey:
		if item.Callee.Callback != 0 || !emptyCapturedInitialRead(item.Callee.Read) || !validAdmission(item.Admission) {
			return errors.New("malformed meta-key callee union")
		}
		key, err := normalizeRequiredExactKey(item.Callee.MetaKey)
		if err != nil {
			return fmt.Errorf("meta key: %w", err)
		}
		arguments, outcomes, endpointsErr := d.freezeInlineEndpoints(item.Arguments, item.Outcomes)
		if endpointsErr != nil {
			return endpointsErr
		}
		edge.callee, edge.metaKey = SubedgeCalleeMetaKey, key
		edge.arguments, edge.outcomes, edge.admission = arguments, outcomes, item.Admission
		return nil
	default:
		return errors.New("Call family has invalid callee")
	}
}

func (d *operationDraft) freezeInlineEndpoints(arguments ValuesSpec, terminals []TerminalSpec) (valuesDraft, [5]valuesDraft, error) {
	result, err := d.freezeValues(arguments, false)
	if err != nil {
		return valuesDraft{}, [5]valuesDraft{}, fmt.Errorf("arguments: %w", err)
	}
	if len(terminals) != 5 {
		return valuesDraft{}, [5]valuesDraft{}, errors.New("inline callee has incomplete terminals")
	}
	var outcomes [5]valuesDraft
	seen := [5]bool{}
	for index, terminal := range terminals {
		kind, valid := crossActivationOutcomeIndex(terminal.Kind)
		if !valid || seen[kind] {
			return valuesDraft{}, [5]valuesDraft{}, fmt.Errorf("inline terminal %d is invalid or duplicate", index)
		}
		values, freezeErr := d.freezeValues(terminal.Values, false)
		if freezeErr != nil {
			return valuesDraft{}, [5]valuesDraft{}, fmt.Errorf("inline terminal %d: %w", index, freezeErr)
		}
		seen[kind], outcomes[kind] = true, values
	}
	return result, outcomes, nil
}

func validateClosedFamilyABI(family SubedgeFamily, arguments valuesDraft) error {
	var want int
	switch family {
	case SubedgeFamilyLength:
		want = 1
	case SubedgeFamilyIndexGet, SubedgeFamilyEqual, SubedgeFamilyLess:
		want = 2
	case SubedgeFamilyIndexSet:
		want = 3
	default:
		return errors.New("invalid non-Call family")
	}
	if arguments.tail != ValuesClosed || len(arguments.suffix) != 0 || len(arguments.types) != want {
		return fmt.Errorf("family %d requires exactly %d closed arguments", family, want)
	}
	return nil
}

func (d *operationDraft) freezeSubedgeArgumentOrigins(input []ArgumentOrigin, arguments valuesDraft) ([]subedgeArgumentOriginDraft, error) {
	want := len(arguments.types) + len(arguments.suffix)
	if arguments.tail == ValuesVariable {
		want++
	}
	// An empty set deliberately means this contextual endpoint is wholly fed by
	// sibling/admission routes. That alternative is checked after local refs
	// resolve; accepting a partial set here would create an implicit merge.
	if len(input) == 0 {
		return nil, nil
	}
	if len(input) != want {
		return nil, errors.New("argument origins are incomplete")
	}
	if _, err := checkedStoredLength("subedge argument origin table", len(input)); err != nil {
		return nil, err
	}
	out := make([]subedgeArgumentOriginDraft, len(input))
	for index, item := range input {
		if err := d.validateArgumentOrigin(item, arguments); err != nil {
			return nil, fmt.Errorf("argument origin %d: %w", index, err)
		}
		out[index] = subedgeArgumentOriginDraft{
			segment: item.Segment, index: item.Index, kind: item.Kind, source: item.Source,
		}
	}
	sort.Slice(out, func(left, right int) bool { return compareArgumentOrigin(out[left], out[right]) < 0 })
	for index := range out {
		if index != 0 && out[index-1].segment == out[index].segment && out[index-1].index == out[index].index {
			return nil, errors.New("duplicate argument origin")
		}
		if !argumentOriginExpected(out[index], arguments) {
			return nil, errors.New("argument origin does not name a Values segment")
		}
	}
	return out, nil
}

func (d *operationDraft) validateArgumentOrigin(origin ArgumentOrigin, arguments valuesDraft) error {
	if !argumentOriginExpected(subedgeArgumentOriginDraft{segment: origin.Segment, index: origin.Index}, arguments) {
		return errors.New("segment outside argument Values")
	}
	switch origin.Kind {
	case ArgumentSourceRule:
		if origin.Source != (InputSource{}) {
			return errors.New("Rule origin carries direct input")
		}
		return nil
	case ArgumentSourceInput:
		return d.validateDirectArgumentOrigin(origin, arguments)
	default:
		return errors.New("invalid argument source")
	}
}

func (d *operationDraft) validateDirectArgumentOrigin(origin ArgumentOrigin, arguments valuesDraft) error {
	switch origin.Segment {
	case ArgumentFixed, ArgumentSuffix:
		if origin.Source.Kind != InputSourceValueFormal || uint64(origin.Source.Ordinal) >= uint64(len(d.input.types)) {
			return errors.New("fixed argument origin is not an owner ValueFormal")
		}
		destination, ok := argumentSegmentType(arguments, origin.Segment, origin.Index)
		if !ok {
			return errors.New("fixed argument origin is type-incompatible")
		}
		assignable, relationErr := d.typeAssignable(d.input.types[origin.Source.Ordinal], destination)
		if relationErr != nil {
			return fmt.Errorf("fixed argument origin type relation: %w", relationErr)
		}
		if !assignable {
			return errors.New("fixed argument origin is type-incompatible")
		}
		return nil
	case ArgumentTail:
		if origin.Source.Kind != InputSourceValuesVar || d.input.tail != ValuesVariable || arguments.tail != ValuesVariable ||
			origin.Source.Ordinal != uint32(d.input.varID) || arguments.varID != d.input.varID {
			return errors.New("tail argument origin is not the owner input tail")
		}
		if d.input.tailType != arguments.tailType {
			return errors.New("tail argument origin is type-incompatible")
		}
		return nil
	default:
		return errors.New("invalid argument segment")
	}
}

func argumentOriginExpected(origin subedgeArgumentOriginDraft, values valuesDraft) bool {
	switch origin.segment {
	case ArgumentFixed:
		return uint64(origin.index) < uint64(len(values.types))
	case ArgumentSuffix:
		return uint64(origin.index) < uint64(len(values.suffix))
	case ArgumentTail:
		return origin.index == 0 && values.tail == ValuesVariable
	default:
		return false
	}
}

func argumentSegmentType(values valuesDraft, segment ArgumentSegment, index uint32) (string, bool) {
	switch segment {
	case ArgumentFixed:
		if uint64(index) < uint64(len(values.types)) {
			return values.types[index], true
		}
	case ArgumentSuffix:
		if uint64(index) < uint64(len(values.suffix)) {
			return values.suffix[index], true
		}
	}
	return "", false
}

func compareArgumentOrigin(left, right subedgeArgumentOriginDraft) int {
	if left.segment < right.segment {
		return -1
	}
	if left.segment > right.segment {
		return 1
	}
	if left.index < right.index {
		return -1
	}
	if left.index > right.index {
		return 1
	}
	return 0
}

func (d *operationDraft) freezeSubedgeAdmissionFailure(input AdmissionFailureSpec, outcomeOrdinals []uint32) (valuesDraft, subedgeRouteDraft, error) {
	source, sourceErr := d.freezeValues(input.Values, false)
	if sourceErr != nil {
		return valuesDraft{}, subedgeRouteDraft{}, fmt.Errorf("admission failure Values: %w", sourceErr)
	}
	route := subedgeRouteDraft{
		route: input.Route.Route, adjustment: input.Route.Adjustment,
		placement: input.Route.Placement, offset: input.Route.Offset, subedge: input.Route.Subedge,
	}
	result, resultErr := d.freezeValues(input.Route.Result, false)
	if resultErr != nil {
		return valuesDraft{}, subedgeRouteDraft{}, fmt.Errorf("admission failure Result: %w", resultErr)
	}
	route.result = result
	switch route.route {
	case RouteOutcome:
		if uint64(input.Route.Outcome) >= uint64(len(outcomeOrdinals)) || input.Route.Subedge != 0 {
			return valuesDraft{}, subedgeRouteDraft{}, errors.New("admission failure owner outcome outside scope or mixed")
		}
		route.outcome = outcomeOrdinals[input.Route.Outcome]
	case RouteSubedge:
		if input.Route.Subedge == 0 || input.Route.Outcome != 0 {
			return valuesDraft{}, subedgeRouteDraft{}, errors.New("admission failure lacks sibling")
		}
	default:
		return valuesDraft{}, subedgeRouteDraft{}, errors.New("admission failure has invalid disposition")
	}
	return source, route, nil
}

func (d *operationDraft) freezeSubedgeRoutes(input []SubedgeRouteSpec, outcomeOrdinals []uint32) ([5]subedgeRouteDraft, error) {
	if len(input) != 5 {
		return [5]subedgeRouteDraft{}, errors.New("incomplete terminal routes")
	}
	var out [5]subedgeRouteDraft
	seen := [5]bool{}
	for index, item := range input {
		kind, valid := crossActivationOutcomeIndex(item.Kind)
		if !valid || seen[kind] {
			return [5]subedgeRouteDraft{}, fmt.Errorf("route %d has invalid or duplicate terminal", index)
		}
		row := subedgeRouteDraft{route: item.Route, adjustment: item.Adjustment, placement: item.Placement, offset: item.Offset, subedge: item.Subedge}
		result, freezeErr := d.freezeValues(item.Result, false)
		if freezeErr != nil {
			return [5]subedgeRouteDraft{}, fmt.Errorf("route %d result: %w", index, freezeErr)
		}
		row.result = result
		switch row.route {
		case RouteOutcome:
			if uint64(item.Outcome) >= uint64(len(outcomeOrdinals)) {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d owner outcome outside scope", index)
			}
			if item.Subedge != 0 {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d has mixed destinations", index)
			}
			row.outcome = outcomeOrdinals[item.Outcome]
		case RouteSubedge:
			if item.Subedge == 0 || item.Outcome != 0 {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d lacks sibling", index)
			}
		case RouteRejectYield:
			if item.Subedge != 0 {
				if item.Outcome != 0 {
					return [5]subedgeRouteDraft{}, fmt.Errorf("route %d C-boundary sibling carries owner outcome", index)
				}
			} else {
				if uint64(item.Outcome) >= uint64(len(outcomeOrdinals)) {
					return [5]subedgeRouteDraft{}, fmt.Errorf("route %d owner outcome outside scope", index)
				}
				row.outcome = outcomeOrdinals[item.Outcome]
			}
		case RouteContinue, RoutePropagateYield:
			if item.Outcome != 0 || item.Subedge != 0 {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d carries destination", index)
			}
		default:
			return [5]subedgeRouteDraft{}, fmt.Errorf("route %d has invalid disposition", index)
		}
		seen[kind], out[kind] = true, row
	}
	return out, nil
}

func (d *operationDraft) validateSubedge(edge *subedgeDraft, all []subedgeDraft) error {
	if !validAdmission(edge.admission) {
		return errors.New("invalid effective admission")
	}
	if err := d.validateAdmissionFailure(edge.admissionFailure, &edge.admissionRoute, all); err != nil {
		return fmt.Errorf("admission failure: %w", err)
	}
	for index := range edge.routes {
		route := &edge.routes[index]
		source := edge.outcomes[index]
		switch route.route {
		case RouteOutcome:
			if route.subedge != 0 || int(route.outcome) >= len(d.outcomes) {
				return fmt.Errorf("terminal %d malformed owner route", index)
			}
			destination := d.outcomes[route.outcome].values
			route.destination = destination
			if err := d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case RouteSubedge:
			if route.outcome != 0 || uint64(route.subedgeRank) >= uint64(len(all)) {
				return fmt.Errorf("terminal %d malformed sibling route", index)
			}
			destination := all[route.subedgeRank].arguments
			route.destination = destination
			if err := d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case RouteContinue:
			if route.outcome != 0 || route.subedge != 0 || route.placement != PlacementInvalid || route.offset != 0 {
				return fmt.Errorf("terminal %d malformed continuation route", index)
			}
			if err := d.validateProjectedResult(source, route.result, route.adjustment); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case RoutePropagateYield:
			if index != 3 || route.outcome != 0 || route.subedge != 0 || route.placement != PlacementInvalid || route.offset != 0 || route.adjustment != AdjustmentPreserve {
				return fmt.Errorf("terminal %d malformed PropagateYield route", index)
			}
			if compareValues(source, route.result) != 0 {
				return errors.New("PropagateYield changes its exact Values")
			}
		case RouteRejectYield:
			if index != 3 || route.adjustment != AdjustmentExact || route.placement != PlacementFixed || route.offset != 0 || !d.canonicalRejectedYield(route.result) {
				return fmt.Errorf("terminal %d malformed RejectYield route", index)
			}
			if route.subedge != 0 {
				if route.outcome != 0 || uint64(route.subedgeRank) >= uint64(len(all)) {
					return fmt.Errorf("terminal %d malformed C-boundary sibling route", index)
				}
				destination := all[route.subedgeRank].arguments
				route.destination = destination
				if err := d.validateResultPlacement(route.result, destination, route.adjustment, route.placement, route.offset); err != nil {
					return fmt.Errorf("terminal %d C-boundary sibling: %w", index, err)
				}
				continue
			}
			if int(route.outcome) >= len(d.outcomes) || d.outcomes[route.outcome].kind != flowkind.OutcomeThrow {
				return errors.New("RejectYield does not name owner Throw")
			}
			destination := d.outcomes[route.outcome].values
			route.destination = destination
			if err := d.validateResultPlacement(route.result, destination, route.adjustment, route.placement, route.offset); err != nil {
				return fmt.Errorf("terminal %d C-boundary owner: %w", index, err)
			}
		default:
			return fmt.Errorf("terminal %d has invalid route", index)
		}
	}
	return nil
}

func (d *operationDraft) validateAdmissionFailure(source valuesDraft, route *subedgeRouteDraft, all []subedgeDraft) error {
	switch route.route {
	case RouteOutcome:
		if route.subedge != 0 || int(route.outcome) >= len(d.outcomes) {
			return errors.New("malformed owner route")
		}
		destination := d.outcomes[route.outcome].values
		route.destination = destination
		return d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset)
	case RouteSubedge:
		if route.outcome != 0 || uint64(route.subedgeRank) >= uint64(len(all)) {
			return errors.New("malformed sibling route")
		}
		destination := all[route.subedgeRank].arguments
		route.destination = destination
		return d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset)
	default:
		return errors.New("invalid disposition")
	}
}

func (d *operationDraft) validateRouteTransport(source, result, destination valuesDraft, adjustment Adjustment, placement Placement, offset uint32) error {
	if err := d.validateProjectedResult(source, result, adjustment); err != nil {
		return err
	}
	return d.validateResultPlacement(result, destination, adjustment, placement, offset)
}

func (d *operationDraft) validateResultPlacement(result, destination valuesDraft, adjustment Adjustment, placement Placement, offset uint32) error {
	switch placement {
	case PlacementTail:
		if adjustment != AdjustmentPreserve || offset != 0 || !pureValuesTail(result) ||
			destination.tail != ValuesVariable || destination.varID != result.varID || len(destination.suffix) != 0 {
			return errors.New("invalid tail placement")
		}
	case PlacementFixed:
		if result.tail != ValuesClosed || destination.tail != ValuesClosed || uint64(offset) > uint64(len(destination.types)) ||
			uint64(len(result.types)) > uint64(len(destination.types))-uint64(offset) {
			return errors.New("invalid fixed placement")
		}
		for index := range result.types {
			assignable, relationErr := d.typeAssignable(result.types[index], destination.types[uint32(index)+offset])
			if relationErr != nil {
				return fmt.Errorf("fixed placement type relation: %w", relationErr)
			}
			if !assignable {
				return errors.New("fixed placement rejects result type")
			}
		}
	default:
		return errors.New("invalid placement")
	}
	return nil
}

func pureValuesTail(values valuesDraft) bool {
	return len(values.types) == 0 && len(values.suffix) == 0 && values.tail == ValuesVariable
}

func (d *operationDraft) canonicalRejectedYield(values valuesDraft) bool {
	// The Lua-domain authoring layer owns the exact rejection literal. Target
	// retains only the structural route shape here.
	return values.tail == ValuesClosed && len(values.types) == 1 && len(values.suffix) == 0 && d.hasType(values.types[0])
}

func validateSubedgeEntries(edges []subedgeDraft, callbacks []callbackDraft) error {
	callbackEdges := make([]int, len(callbacks))
	inbound := make([][]*subedgeRouteDraft, len(edges))
	for index := range edges {
		edge := &edges[index]
		if edge.callee == SubedgeCalleeCallback {
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
		if edge.callee == SubedgeCalleeCallback && callbackEdges[edge.callbackRank] != 1 {
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
		if route.route != RouteSubedge && !(route.route == RouteRejectYield && route.subedge != 0) {
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
			if edge.callee != SubedgeCalleeCallback || uint64(edge.callbackRank) >= uint64(len(callbacks)) {
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
	if route.route != RouteSubedge && !(route.route == RouteRejectYield && route.subedge != 0) {
		return
	}
	if uint64(route.subedgeRank) >= uint64(len(inbound)) {
		return
	}
	inbound[route.subedgeRank] = append(inbound[route.subedgeRank], route)
}

func argumentSegmentCount(values valuesDraft) int {
	count := len(values.types) + len(values.suffix)
	if values.tail == ValuesVariable {
		count++
	}
	return count
}

func routeCompletelyFeedsArguments(route subedgeRouteDraft, destination valuesDraft) bool {
	switch route.placement {
	case PlacementFixed:
		return route.offset == 0 && route.result.tail == ValuesClosed && destination.tail == ValuesClosed &&
			len(route.result.types) == len(destination.types) && len(route.result.suffix) == 0 && len(destination.suffix) == 0
	case PlacementTail:
		return route.adjustment == AdjustmentPreserve && pureValuesTail(route.result) && pureValuesTail(destination) &&
			route.result.varID == destination.varID
	default:
		return false
	}
}

func validSubedgeFamily(family SubedgeFamily) bool {
	return family >= SubedgeFamilyCall && family <= SubedgeFamilyLess
}

func zeroLiteral(value keyspace.LiteralValue) bool { return value == (keyspace.LiteralValue{}) }

func normalizeRequiredExactKey(value keyspace.LiteralValue) (keyspace.LiteralValue, error) {
	normalized, ok := scalar.Normalize(value)
	if !ok {
		return keyspace.LiteralValue{}, errors.New("not an exact Lua key")
	}
	return normalized, nil
}

func emptyCapturedInitialRead(read CapturedInitialReadSpec) bool {
	return read.Root == "" && zeroLiteral(read.Key)
}

func emptySubedgeCallee(callee SubedgeCalleeSpec) bool {
	return callee.Kind == SubedgeCalleeInvalid && callee.Callback == 0 &&
		emptyCapturedInitialRead(callee.Read) && zeroLiteral(callee.MetaKey)
}

func emptyValuesSpec(values ValuesSpec) bool {
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

func (c *Contract) appendSubedges(owner Operation, input []subedgeDraft, callbacks []CallbackID, values map[string]Values, keys map[keyspace.LiteralValue]ExactKey) (indexRange, error) {
	rangeOut, err := checkedStoredRange("subedge table", len(c.subedges), len(input))
	if err != nil {
		return indexRange{}, err
	}
	ids := make([]SubedgeID, len(input))
	for index := range input {
		if input[index].source < 0 || input[index].source >= len(ids) {
			return indexRange{}, errors.New("target: malformed subedge source")
		}
		handle, handleErr := checkedStoredHandle("subedge table", len(c.subedges)+index)
		if handleErr != nil {
			return indexRange{}, handleErr
		}
		ids[input[index].source] = SubedgeID(handle)
		input[index].sealed = SubedgeID(handle)
	}
	for index := range input {
		edge := input[index]
		arguments, valuesErr := lookupDraftValues(values, edge.arguments)
		if valuesErr != nil {
			return indexRange{}, valuesErr
		}
		row := subedgeRow{
			owner: owner, role: edge.role, family: edge.family, callee: edge.callee,
			admission: edge.admission, arguments: arguments, ruleEntry: edge.ruleEntry,
		}
		origins, originsErr := c.appendSubedgeArgumentOrigins(edge.argumentOrigins)
		if originsErr != nil {
			return indexRange{}, originsErr
		}
		row.argumentOrigins = origins
		if edge.callee == SubedgeCalleeCallback {
			if edge.callback == 0 || int(edge.callback) > len(callbacks) || callbacks[edge.callback-1] == 0 {
				return indexRange{}, errors.New("target: unresolved callback subedge callee")
			}
			row.callback = callbacks[edge.callback-1]
			callbackIndex := uint32(row.callback) - 1
			if c.callbacks[callbackIndex].subedge != 0 {
				return indexRange{}, errors.New("target: callback has multiple direct subedges")
			}
			c.callbacks[callbackIndex].subedge = ids[edge.source]
		}
		if edge.callee == SubedgeCalleeCapturedInitialRead {
			if edge.readRootID == 0 {
				return indexRange{}, errors.New("target: unresolved captured initial read root")
			}
			key, keyErr := exactKeyHandle(keys, edge.readKey)
			if keyErr != nil {
				return indexRange{}, keyErr
			}
			row.readRoot, row.readKey = edge.readRootID, key
		}
		if edge.callee == SubedgeCalleeMetaKey {
			key, keyErr := exactKeyHandle(keys, edge.metaKey)
			if keyErr != nil {
				return indexRange{}, keyErr
			}
			row.metaKey = key
		}
		for terminal := range edge.outcomes {
			value, terminalErr := lookupDraftValues(values, edge.outcomes[terminal])
			if terminalErr != nil {
				return indexRange{}, terminalErr
			}
			row.outcomes[terminal] = value
		}
		failure, failureErr := lookupDraftValues(values, edge.admissionFailure)
		if failureErr != nil {
			return indexRange{}, failureErr
		}
		row.admissionFailure = failure
		admissionRoute, admissionRouteErr := c.appendSubedgeRoute(edge.admissionRoute, ids, values)
		if admissionRouteErr != nil {
			return indexRange{}, admissionRouteErr
		}
		row.admissionRoute = admissionRoute
		for terminal, route := range edge.routes {
			item, itemErr := c.appendSubedgeRoute(route, ids, values)
			if itemErr != nil {
				return indexRange{}, itemErr
			}
			row.routes[terminal] = item
		}
		c.subedges = append(c.subedges, row)
	}
	return rangeOut, nil
}

func (c *Contract) appendSubedgeArgumentOrigins(input []subedgeArgumentOriginDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("subedge argument origin table", len(c.subedgeOrigins), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, origin := range input {
		c.subedgeOrigins = append(c.subedgeOrigins, subedgeArgumentOriginRow{
			segment: origin.segment, index: origin.index, kind: origin.kind, source: origin.source,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendSubedgeRoute(route subedgeRouteDraft, ids []SubedgeID, values map[string]Values) (subedgeRouteRow, error) {
	item := subedgeRouteRow{
		route: route.route, adjustment: route.adjustment,
		placement: route.placement, offset: route.offset, outcome: route.outcome,
	}
	result, resultErr := lookupDraftValues(values, route.result)
	if resultErr != nil {
		return subedgeRouteRow{}, resultErr
	}
	item.result = result
	if route.route == RouteSubedge || (route.route == RouteRejectYield && route.subedge != 0) {
		if route.subedge == 0 || int(route.subedge) > len(ids) || ids[route.subedge-1] == 0 {
			return subedgeRouteRow{}, errors.New("target: unresolved sibling subedge")
		}
		item.subedge = ids[route.subedge-1]
	}
	if route.route == RouteOutcome || route.route == RouteSubedge || route.route == RouteRejectYield {
		destination, destinationErr := lookupDraftValues(values, route.destination)
		if destinationErr != nil {
			return subedgeRouteRow{}, destinationErr
		}
		item.destination = destination
	}
	return item, nil
}

func exactKeyHandle(keys map[keyspace.LiteralValue]ExactKey, value keyspace.LiteralValue) (ExactKey, error) {
	key, ok := keys[value]
	if !ok || key == 0 {
		return 0, errors.New("target: unresolved exact key")
	}
	return key, nil
}
