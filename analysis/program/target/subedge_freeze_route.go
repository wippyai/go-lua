package target

import (
	"errors"
	"fmt"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func resolveSubedgeRouteSibling(route *subedgeRouteDraft, ranks []uint32, all []subedgeDraft) error {
	if route.route != vocabulary.RouteSubedge && (route.route != vocabulary.RouteRejectYield || route.subedge == 0) {
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

func (d *operationDraft) freezeSubedgeAdmissionFailure(input vocabulary.AdmissionFailureSpec, outcomeOrdinals []uint32) (valuesDraft, subedgeRouteDraft, error) {
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
	case vocabulary.RouteOutcome:
		if uint64(input.Route.Outcome) >= uint64(len(outcomeOrdinals)) || input.Route.Subedge != 0 {
			return valuesDraft{}, subedgeRouteDraft{}, errors.New("admission failure owner outcome outside scope or mixed")
		}
		route.outcome = outcomeOrdinals[input.Route.Outcome]
	case vocabulary.RouteSubedge:
		if input.Route.Subedge == 0 || input.Route.Outcome != 0 {
			return valuesDraft{}, subedgeRouteDraft{}, errors.New("admission failure lacks sibling")
		}
	default:
		return valuesDraft{}, subedgeRouteDraft{}, errors.New("admission failure has invalid disposition")
	}
	return source, route, nil
}

func (d *operationDraft) freezeSubedgeRoutes(input []vocabulary.SubedgeRouteSpec, outcomeOrdinals []uint32) ([5]subedgeRouteDraft, error) {
	if len(input) != 5 {
		return [5]subedgeRouteDraft{}, errors.New("incomplete terminal routes")
	}
	var out [5]subedgeRouteDraft
	seen := [5]bool{}
	for index, item := range input {
		kind, valid := vocabulary.CrossActivationOutcomeIndex(item.Kind)
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
		case vocabulary.RouteOutcome:
			if uint64(item.Outcome) >= uint64(len(outcomeOrdinals)) {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d owner outcome outside scope", index)
			}
			if item.Subedge != 0 {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d has mixed destinations", index)
			}
			row.outcome = outcomeOrdinals[item.Outcome]
		case vocabulary.RouteSubedge:
			if item.Subedge == 0 || item.Outcome != 0 {
				return [5]subedgeRouteDraft{}, fmt.Errorf("route %d lacks sibling", index)
			}
		case vocabulary.RouteRejectYield:
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
		case vocabulary.RouteContinue, vocabulary.RoutePropagateYield:
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
		case vocabulary.RouteOutcome:
			if route.subedge != 0 || int(route.outcome) >= len(d.outcomes) {
				return fmt.Errorf("terminal %d malformed owner route", index)
			}
			destination := d.outcomes[route.outcome].values
			route.destination = destination
			if err := d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case vocabulary.RouteSubedge:
			if route.outcome != 0 || uint64(route.subedgeRank) >= uint64(len(all)) {
				return fmt.Errorf("terminal %d malformed sibling route", index)
			}
			destination := all[route.subedgeRank].arguments
			route.destination = destination
			if err := d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case vocabulary.RouteContinue:
			if route.outcome != 0 || route.subedge != 0 || route.placement != vocabulary.PlacementInvalid || route.offset != 0 {
				return fmt.Errorf("terminal %d malformed continuation route", index)
			}
			if err := d.validateProjectedResult(source, route.result, route.adjustment); err != nil {
				return fmt.Errorf("terminal %d: %w", index, err)
			}
		case vocabulary.RoutePropagateYield:
			if index != 3 || route.outcome != 0 || route.subedge != 0 || route.placement != vocabulary.PlacementInvalid || route.offset != 0 || route.adjustment != vocabulary.AdjustmentPreserve {
				return fmt.Errorf("terminal %d malformed PropagateYield route", index)
			}
			if compareValues(source, route.result) != 0 {
				return errors.New("PropagateYield changes its exact Values")
			}
		case vocabulary.RouteRejectYield:
			if index != 3 || route.adjustment != vocabulary.AdjustmentExact || route.placement != vocabulary.PlacementFixed || route.offset != 0 || !d.canonicalRejectedYield(route.result) {
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
	case vocabulary.RouteOutcome:
		if route.subedge != 0 || int(route.outcome) >= len(d.outcomes) {
			return errors.New("malformed owner route")
		}
		destination := d.outcomes[route.outcome].values
		route.destination = destination
		return d.validateRouteTransport(source, route.result, destination, route.adjustment, route.placement, route.offset)
	case vocabulary.RouteSubedge:
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

func (d *operationDraft) validateRouteTransport(source, result, destination valuesDraft, adjustment vocabulary.Adjustment, placement vocabulary.Placement, offset uint32) error {
	if err := d.validateProjectedResult(source, result, adjustment); err != nil {
		return err
	}
	return d.validateResultPlacement(result, destination, adjustment, placement, offset)
}

func (d *operationDraft) validateResultPlacement(result, destination valuesDraft, adjustment vocabulary.Adjustment, placement vocabulary.Placement, offset uint32) error {
	switch placement {
	case vocabulary.PlacementTail:
		if adjustment != vocabulary.AdjustmentPreserve || offset != 0 || !pureValuesTail(result) ||
			destination.tail != vocabulary.ValuesVariable || destination.varID != result.varID || len(destination.suffix) != 0 {
			return errors.New("invalid tail placement")
		}
	case vocabulary.PlacementFixed:
		if result.tail != vocabulary.ValuesClosed || destination.tail != vocabulary.ValuesClosed || uint64(offset) > uint64(len(destination.types)) ||
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
	return len(values.types) == 0 && len(values.suffix) == 0 && values.tail == vocabulary.ValuesVariable
}

func (d *operationDraft) canonicalRejectedYield(values valuesDraft) bool {
	// The Lua-domain authoring layer owns the exact rejection literal. Target
	// retains only the structural route shape here.
	if values.tail != vocabulary.ValuesClosed || len(values.types) != 1 || len(values.suffix) != 0 {
		return false
	}
	_, admitted := d.declarations[values.types[0]]
	return admitted
}
