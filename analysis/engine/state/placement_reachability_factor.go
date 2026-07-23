package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// PlacementReachabilityPlan is one sealed least-reachability problem over the
// registered identity graph. It has no traversal cap: the finite graph and a
// visited set are the termination proof.
type PlacementReachabilityPlan struct {
	seal      *productDomainSeal
	keys      *keyspace.KeySpace
	seeds     []product.Value
	placement placement.Value
}

func (p PlacementReachabilityPlan) Valid() bool {
	return p.seal != nil && p.keys != nil && p.keys.Valid() && len(p.seeds) != 0 &&
		p.placement > placement.Bottom && p.placement <= placement.Unknown
}

func (d ProductDomain) PreparePlacementReachabilityPlan(keys *keyspace.KeySpace, seeds []product.Value, target placement.Value) (PlacementReachabilityPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || len(seeds) == 0 || target <= placement.Bottom || target > placement.Unknown {
		return PlacementReachabilityPlan{}, fmt.Errorf("%w: invalid placement reachability", ErrInvalidLaneFactor)
	}
	out := append([]product.Value(nil), seeds...)
	for _, seed := range out {
		if !product.BelongsToRegistry(d.reg, seed) {
			return PlacementReachabilityPlan{}, fmt.Errorf("%w: foreign placement seed", ErrInvalidLaneFactor)
		}
	}
	return PlacementReachabilityPlan{seal: d.seal, keys: keys, seeds: out, placement: target}, nil
}

// PlacementReachabilityLanes returns the complete registered identity-graph
// carrier. Adding or removing an axis changes only its family registration.
func (d ProductDomain) PlacementReachabilityLanes(plan PlacementReachabilityPlan) ([]ProductLane, error) {
	if !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign placement reachability", ErrInvalidLaneFactor)
	}
	return d.PlacementReachabilityPotentialLanes(), nil
}

// PlacementReachabilityPotentialLanes returns the registration-owned carrier
// envelope before a concrete reachability problem exists. Exact plans retain
// the same envelope: reachability is data dependent, while participation is a
// property of each coordinate family's registered return-identity roles.
func (d ProductDomain) PlacementReachabilityPotentialLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 2)
	for laneIndex := range d.factorLanes {
		runtime := &d.factorLanes[laneIndex]
		for familyIndex := range runtime.coordinates {
			family := &runtime.coordinates[familyIndex]
			if family.ops.returnIdentity.roles != 0 {
				out = append(out, runtime.lane)
				break
			}
		}
	}
	return out
}

type placementReachabilityLane struct {
	lane      ProductLane
	factor    LaneFactor
	families  []CoordinateFamily
	skeletons []CoordinateFamilySkeleton
	scalars   [][]CoordinateScalarFactor
}

// ApplyPlacementReachabilityFactors computes and publishes the least closure
// over exact lane factors. Cancellation returns the original factor slice.
func (d ProductDomain) ApplyPlacementReachabilityFactors(ctx context.Context, plan PlacementReachabilityPlan, factors []LaneFactor) ([]LaneFactor, error) {
	lanes, err := d.PlacementReachabilityLanes(plan)
	if err != nil || len(factors) != len(lanes) {
		if err == nil {
			err = ErrIncompleteLaneFactors
		}
		return nil, err
	}
	fail := func(err error) ([]LaneFactor, error) { return append([]LaneFactor(nil), factors...), err }
	if ctx == nil {
		return fail(fmt.Errorf("%w: nil placement context", ErrInvalidLaneFactor))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	rows := make([]placementReachabilityLane, len(lanes))
	for laneIndex, lane := range lanes {
		if factors[laneIndex].lane != lane {
			return fail(ErrIncompleteLaneFactors)
		}
		families, familyErr := d.CoordinateFamilies(lane)
		if familyErr != nil {
			return fail(familyErr)
		}
		row := placementReachabilityLane{lane: lane, factor: factors[laneIndex], families: families, skeletons: make([]CoordinateFamilySkeleton, len(families)), scalars: make([][]CoordinateScalarFactor, len(families))}
		for familyIndex, family := range families {
			row.skeletons[familyIndex], row.scalars[familyIndex], familyErr = d.DecomposeCoordinateFamily(factors[laneIndex], family, plan.keys)
			if familyErr != nil {
				return fail(familyErr)
			}
		}
		rows[laneIndex] = row
	}

	adjacent := make(map[identity.Term][]identity.Term)
	for laneIndex := range rows {
		for familyIndex, skeleton := range rows[laneIndex].skeletons {
			if err := d.VisitCoordinateReturnIdentitySkeletonObservations(skeleton, func(observation CoordinateReturnIdentityObservation) bool {
				if observation.Role() == CoordinateReturnIdentitySkeletonEdge {
					adjacent[observation.Root()] = append(adjacent[observation.Root()], observation.Target())
				}
				return true
			}); err != nil {
				return fail(err)
			}
			for _, scalar := range rows[laneIndex].scalars[familyIndex] {
				if err := d.VisitCoordinateReturnIdentityScalarObservations(scalar, func(observation CoordinateReturnIdentityObservation) bool {
					if observation.Role() == CoordinateReturnIdentityScalarEdge {
						adjacent[observation.Root()] = append(adjacent[observation.Root()], observation.Target())
					}
					return true
				}); err != nil {
					return fail(err)
				}
			}
		}
	}
	reachable := make(map[identity.Term]bool)
	queue := make([]identity.Term, 0, len(plan.seeds))
	for _, seed := range plan.seeds {
		root, exact := product.Get(d.reg, seed, identity.Key).Term()
		if !exact || !root.Valid() || reachable[root] {
			continue
		}
		// Stored values acquire placement even when the heap body is absent;
		// an available body contributes outgoing edges, but is not a gate on
		// the seed coordinate itself.
		reachable[root] = true
		queue = append(queue, root)
	}
	for head := 0; head < len(queue); head++ {
		if head&255 == 0 {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
		}
		for _, next := range adjacent[queue[head]] {
			if !reachable[next] {
				reachable[next] = true
				queue = append(queue, next)
			}
		}
	}
	identities := make([]identity.Term, 0, len(reachable))
	for root := range reachable {
		identities = append(identities, root)
	}
	sort.Slice(identities, func(i, j int) bool { return identity.Less(identities[i], identities[j]) })
	changedRows := make([]bool, len(rows))
	for _, root := range identities {
		for laneIndex := range rows {
			for familyIndex, family := range rows[laneIndex].families {
				roles, roleErr := d.CoordinateReturnIdentityRoles(family)
				if roleErr != nil {
					return fail(roleErr)
				}
				if !roles.Has(CoordinateReturnIdentityPublisher) {
					continue
				}
				slot, handled, slotErr := d.CoordinateReturnIdentityTermSlot(family, plan.keys, root)
				if slotErr != nil {
					return fail(slotErr)
				}
				if !handled {
					continue
				}
				position, found, positionErr := coordinateScalarPosition(d, rows[laneIndex].scalars[familyIndex], slot)
				if positionErr != nil {
					return fail(positionErr)
				}
				var current CoordinateScalarFactor
				if found {
					current = rows[laneIndex].scalars[familyIndex][position]
				} else {
					current, slotErr = d.CoordinateDefault(rows[laneIndex].skeletons[familyIndex], slot)
					if slotErr != nil {
						return fail(slotErr)
					}
				}
				published, publishErr := d.PublishCoordinateIdentityPlacement(current, plan.placement)
				if publishErr != nil {
					return fail(publishErr)
				}
				equal, equalErr := d.CoordinateScalarEqual(current, published)
				if equalErr != nil {
					return fail(equalErr)
				}
				if equal {
					continue
				}
				scalars := rows[laneIndex].scalars[familyIndex]
				if found {
					scalars[position] = published
				} else {
					scalars = append(scalars, CoordinateScalarFactor{})
					copy(scalars[position+1:], scalars[position:])
					scalars[position] = published
				}
				rows[laneIndex].scalars[familyIndex] = scalars
				changedRows[laneIndex] = true
			}
		}
	}
	out := append([]LaneFactor(nil), factors...)
	for index := range rows {
		if !changedRows[index] {
			continue
		}
		out[index], err = d.ComposeCoordinateFamilies(rows[index].lane, plan.keys, rows[index].skeletons, rows[index].scalars)
		if err != nil {
			return fail(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return out, nil
}

func (d ProductDomain) ApplyPlacementReachability(ctx context.Context, plan PlacementReachabilityPlan, input State) (State, error) {
	lanes, err := d.PlacementReachabilityLanes(plan)
	if err != nil {
		return State{}, err
	}
	factors, err := d.DecomposeLanes(input, lanes)
	if err != nil {
		return State{}, err
	}
	next, err := d.ApplyPlacementReachabilityFactors(ctx, plan, factors)
	if err != nil {
		return input, err
	}
	return d.PatchLaneFactors(input, next)
}

func coordinateScalarPosition(d ProductDomain, scalars []CoordinateScalarFactor, slot CoordinateSlot) (int, bool, error) {
	for index, scalar := range scalars {
		equal, err := d.CoordinateSlotEqual(scalar.slot, slot)
		if err != nil {
			return 0, false, err
		}
		if equal {
			return index, true, nil
		}
		less, err := d.CoordinateSlotLess(scalar.slot, slot)
		if err != nil {
			return 0, false, err
		}
		if !less {
			return index, false, nil
		}
	}
	return len(scalars), false, nil
}
