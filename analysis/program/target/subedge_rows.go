package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

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
