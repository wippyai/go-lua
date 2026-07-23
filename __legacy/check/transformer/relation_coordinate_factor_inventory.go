package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type relationCoordinateFactorProducer struct {
	point     cfg.Point
	inventory state.CoordinateFactorInventory
}

// relationCoordinateFactorInventory is the point-relative static coordinate
// theorem for one lexical CFG. Producers are retained once; At forms the
// exact union of producers that can reach the queried point.
type relationCoordinateFactorInventory struct {
	body      *relationProgramBody
	empty     state.CoordinateFactorInventory
	producers []relationCoordinateFactorProducer
	reachable *cfg.Reachability
}

func (i relationCoordinateFactorInventory) At(point cfg.Point) (state.CoordinateFactorInventory, error) {
	if i.body == nil || i.reachable == nil || !i.empty.ValidFor(i.body.productDomain, i.empty.KeySpace()) ||
		int(point) < 0 || int(point) >= i.body.graph.Size() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("transformer: point-relative coordinate inventory is unowned")
	}
	inputs := []state.CoordinateFactorInventory{i.empty}
	for _, producer := range i.producers {
		if i.reachable.CanReach(producer.point, point) {
			inputs = append(inputs, producer.inventory)
		}
	}
	seed, err := i.body.pathSemantics.UnionCoordinateFactorInventories(i.body.productDomain, inputs...)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	return i.body.pathSemantics.CloseCoordinateFactorInventory(i.body.productDomain, seed)
}

func freezeRelationCoordinateFactorInventory(body *relationProgramBody) (relationCoordinateFactorInventory, error) {
	if body == nil || body.graph == nil || body.plan == nil || !body.productDomain.Valid() ||
		body.pathSemantics == nil || !body.pathSemantics.Valid() {
		return relationCoordinateFactorInventory{}, fmt.Errorf("transformer: relation coordinate inventory is unowned")
	}
	empty, err := body.pathSemantics.SealCoordinateFactorInventory(body.productDomain, nil)
	if err != nil {
		return relationCoordinateFactorInventory{}, err
	}
	out := relationCoordinateFactorInventory{body: body, empty: empty, reachable: cfg.NewReachability(body.graph)}
	appendProducer := func(point cfg.Point, inventory state.CoordinateFactorInventory) error {
		closed, closeErr := body.pathSemantics.CloseCoordinateFactorInventory(body.productDomain, inventory)
		if closeErr != nil {
			return closeErr
		}
		out.producers = append(out.producers, relationCoordinateFactorProducer{point: point, inventory: closed})
		return nil
	}
	if body.initialStatePlan.Valid() {
		for index := 0; index < body.initialStatePlan.Len(); index++ {
			coordinate, prepared, present := body.initialStatePlan.Seed(index)
			if !present || int(coordinate) < 0 || int(coordinate) >= body.graph.Size() {
				return relationCoordinateFactorInventory{}, fmt.Errorf("transformer: initial coordinate producer %d is malformed", index)
			}
			inventory, inventoryErr := body.pathSemantics.CoordinateFactorInventoryFromPreparedState(body.productDomain, prepared)
			if inventoryErr != nil {
				return relationCoordinateFactorInventory{}, inventoryErr
			}
			if err := appendProducer(cfg.Point(coordinate), inventory); err != nil {
				return relationCoordinateFactorInventory{}, err
			}
		}
	}
	facts := body.plan.Facts()
	for raw := 0; raw < body.graph.Size(); raw++ {
		point := cfg.Point(raw)
		if len(facts.PathValuePresenceImplications(point)) == 0 {
			continue
		}
		transaction := factapply.PlanPathValuePresenceImplicationTransaction(facts, point)
		plan, planErr := body.pathSemantics.PreparePathValuePresenceImplications(body.productDomain.Registry(), transaction)
		if planErr != nil {
			return relationCoordinateFactorInventory{}, planErr
		}
		inventory, inventoryErr := plan.CoordinateFactorInventory(body.productDomain)
		if inventoryErr != nil {
			return relationCoordinateFactorInventory{}, inventoryErr
		}
		if err := appendProducer(point, inventory); err != nil {
			return relationCoordinateFactorInventory{}, err
		}
	}
	return out, nil
}
