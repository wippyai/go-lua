package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (c *Contract) appendSubedgeRoute(route subedgeRouteDraft, ids []vocabulary.SubedgeID, values map[string]vocabulary.Values) (subedgeRouteRow, error) {
	item := subedgeRouteRow{
		route: route.route, adjustment: route.adjustment,
		placement: route.placement, offset: route.offset, outcome: route.outcome,
	}
	result, resultErr := lookupDraftValues(values, route.result)
	if resultErr != nil {
		return subedgeRouteRow{}, resultErr
	}
	item.result = result
	if route.route == vocabulary.RouteSubedge || (route.route == vocabulary.RouteRejectYield && route.subedge != 0) {
		if route.subedge == 0 || int(route.subedge) > len(ids) || ids[route.subedge-1] == 0 {
			return subedgeRouteRow{}, errors.New("target: unresolved sibling subedge")
		}
		item.subedge = ids[route.subedge-1]
	}
	if route.route == vocabulary.RouteOutcome || route.route == vocabulary.RouteSubedge || route.route == vocabulary.RouteRejectYield {
		destination, destinationErr := lookupDraftValues(values, route.destination)
		if destinationErr != nil {
			return subedgeRouteRow{}, destinationErr
		}
		item.destination = destination
	}
	return item, nil
}

func exactKeyHandle(keys map[keyspace.LiteralValue]vocabulary.ExactKey, value keyspace.LiteralValue) (vocabulary.ExactKey, error) {
	key, ok := keys[value]
	if !ok || key == 0 {
		return 0, errors.New("target: unresolved exact key")
	}
	return key, nil
}
