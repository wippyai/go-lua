package target

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
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
	if route.route != RouteSubedge && (route.route != RouteRejectYield || route.subedge == 0) {
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
		if item.Admission != schematype.CallableAdmissionOrdinary {
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
			item.Admission != schematype.CallableAdmissionInvalid || !emptyValuesSpec(item.Arguments) || len(item.Outcomes) != 0 {
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
			!item.Admission.Available() {
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
		if item.Callee.Callback != 0 || !emptyCapturedInitialRead(item.Callee.Read) || !item.Admission.Available() {
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
		return errors.New("call family has invalid callee")
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
			return errors.New("rule origin carries direct input")
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
		sourceType, sourceOK := d.declarations[d.input.types[origin.Source.Ordinal]]
		destinationType, destinationOK := d.declarations[destination]
		if !sourceOK || !destinationOK {
			return errors.New("fixed argument origin type relation: type declaration is not admitted")
		}
		assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
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
	if !edge.admission.Available() {
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
			sourceType, sourceOK := d.declarations[result.types[index]]
			destinationType, destinationOK := d.declarations[destination.types[uint32(index)+offset]]
			if !sourceOK || !destinationOK {
				return errors.New("fixed placement type relation: type declaration is not admitted")
			}
			assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
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
	if values.tail != ValuesClosed || len(values.types) != 1 || len(values.suffix) != 0 {
		return false
	}
	_, admitted := d.declarations[values.types[0]]
	return admitted
}
