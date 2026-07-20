package factapply

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// ReturnFactorTarget binds one declared N5 result ordinal to the Values slot
// and structural path of the carrier receiving the transaction. Concrete and
// formal carriers differ only in this address vocabulary.
type ReturnFactorTarget[K comparable] struct {
	Index int
	Slot  K
	Path  keyspace.Key
}

// ReturnFactorTransaction is the complete factor-native input to canonical
// post-materialization N5. Sources are already evaluated exactly once. Values
// and Lanes are one complete registered topology spelling.
type ReturnFactorTransaction[K comparable] struct {
	Return   ReturnTransaction
	Sources  []product.Value
	Targets  []ReturnFactorTarget[K]
	Values   state.ValueFactor[K]
	Lanes    []ReturnFactorLane
	Domain   state.ProductDomain
	Keys     *keyspace.KeySpace
	Topology state.ReturnFactorTopology
}

// ReturnFactorResult is published only after every Values and registered-lane
// mutation succeeds. Failure and cancellation therefore expose no N5 prefix.
type ReturnFactorResult[K comparable] struct {
	Values state.ValueFactor[K]
	Lanes  []ReturnFactorLane
}

// ReturnFactorLane is the canonical registered carrier for N5. Ordinary
// lanes remain indivisible; coordinate lanes are represented directly by
// their family factors so the transaction never decomposes and recomposes a
// physical lane merely to mutate one family.
type ReturnFactorLane struct {
	Lane     state.ProductLane
	Ordinary state.LaneFactor
	Families []state.CoordinateFamilyFactor
}

// BindReturnFactorLanes is the concrete carrier edge for canonical N5. Formal
// tuple executors bind the same ReturnFactorLane shape directly from family
// fibers and therefore do not call this physical-lane adapter.
func BindReturnFactorLanes(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	topology state.ReturnFactorTopology,
	factors []state.LaneFactor,
) ([]ReturnFactorLane, error) {
	if !domain.Valid() || keys == nil || !keys.Valid() || !topology.ValidFor(domain) || len(factors) != topology.Len() {
		return nil, fmt.Errorf("factapply: return factor binding is malformed")
	}
	out := make([]ReturnFactorLane, len(factors))
	for index, factor := range factors {
		lane, ok := topology.Lane(index)
		if !ok || factor.Lane() != lane {
			return nil, fmt.Errorf("factapply: return factor %d has foreign lane ownership", index)
		}
		families, err := domain.CoordinateFamilies(lane)
		if err != nil {
			return nil, err
		}
		out[index].Lane = lane
		if len(families) == 0 {
			out[index].Ordinary = factor
			continue
		}
		out[index].Families = make([]state.CoordinateFamilyFactor, len(families))
		for familyIndex, family := range families {
			skeleton, scalars, decomposeErr := domain.DecomposeCoordinateFamily(factor, family, keys)
			if decomposeErr != nil {
				return nil, decomposeErr
			}
			out[index].Families[familyIndex], err = domain.SealCoordinateFamilyFactor(skeleton, scalars)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// ComposeReturnFactorLanes is the concrete publication edge. Coordinate
// families are composed once after the complete N5 transaction.
func ComposeReturnFactorLanes(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	topology state.ReturnFactorTopology,
	lanes []ReturnFactorLane,
) ([]state.LaneFactor, error) {
	if !domain.Valid() || keys == nil || !keys.Valid() || !topology.ValidFor(domain) || len(lanes) != topology.Len() {
		return nil, fmt.Errorf("factapply: return factor publication is malformed")
	}
	out := make([]state.LaneFactor, len(lanes))
	for index, input := range lanes {
		lane, ok := topology.Lane(index)
		if !ok || input.Lane != lane {
			return nil, fmt.Errorf("factapply: return factor %d has foreign lane ownership", index)
		}
		families, err := domain.CoordinateFamilies(lane)
		if err != nil {
			return nil, err
		}
		if len(families) == 0 {
			if input.Ordinary.Lane() != lane || len(input.Families) != 0 {
				return nil, fmt.Errorf("factapply: ordinary return factor %d is malformed", index)
			}
			out[index] = input.Ordinary
			continue
		}
		if len(input.Families) != len(families) {
			return nil, fmt.Errorf("factapply: coordinate return factor %d is malformed", index)
		}
		skeletons := make([]state.CoordinateFamilySkeleton, len(families))
		scalars := make([][]state.CoordinateScalarFactor, len(families))
		for familyIndex, family := range families {
			if input.Families[familyIndex].Family() != family {
				return nil, fmt.Errorf("factapply: coordinate return family %d/%d is malformed", index, familyIndex)
			}
			skeletons[familyIndex] = input.Families[familyIndex].Skeleton()
			scalars[familyIndex] = input.Families[familyIndex].Scalars()
		}
		out[index], err = domain.ComposeCoordinateFamilies(lane, keys, skeletons, scalars)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type returnCoordinateLane struct {
	lane      state.ProductLane
	families  []state.CoordinateFamily
	skeletons []state.CoordinateFamilySkeleton
	scalars   [][]state.CoordinateScalarFactor
}

// ApplyReturnFactorTransaction executes the one canonical post-materialized
// N5 law over the registered product. It contains no axis switch, State
// reconstruction, inventory discovery, or work/depth budget.
func ApplyReturnFactorTransaction[K comparable](ctx context.Context, authority *ReturnAuthority, in ReturnFactorTransaction[K]) (ReturnFactorResult[K], error) {
	fail := func(err error) (ReturnFactorResult[K], error) {
		return ReturnFactorResult[K]{Values: in.Values, Lanes: in.Lanes}, err
	}
	if ctx == nil || authority == nil || !authority.Valid() || !in.Return.Valid() ||
		!in.Domain.Valid() || in.Domain.Registry() == nil || in.Keys == nil || !in.Keys.Valid() ||
		!in.Topology.ValidFor(in.Domain) || len(in.Sources) != in.Return.SourceCount() || len(in.Lanes) != in.Topology.Len() {
		return fail(fmt.Errorf("factapply: invalid factor-native return transaction"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	reg := in.Domain.Registry()
	for _, value := range in.Sources {
		if !product.BelongsToRegistry(reg, value) {
			return fail(fmt.Errorf("factapply: return source belongs to a foreign product"))
		}
	}
	targets := make(map[int]ReturnFactorTarget[K], len(in.Targets))
	for _, target := range in.Targets {
		if target.Index < 0 || target.Path.Kind == keyspace.KindInvalid {
			return fail(fmt.Errorf("factapply: return target has invalid output identity"))
		}
		if _, duplicate := targets[target.Index]; duplicate {
			return fail(fmt.Errorf("factapply: duplicate return target %d", target.Index))
		}
		targets[target.Index] = target
	}
	if len(targets) != in.Return.ResultTargetCount() {
		return fail(fmt.Errorf("factapply: incomplete return target schema"))
	}
	for index := 0; index < in.Return.ResultTargetCount(); index++ {
		target, ok := in.Return.ResultTarget(index)
		if !ok {
			return fail(fmt.Errorf("factapply: malformed return target schema"))
		}
		if _, present := targets[target]; !present {
			return fail(fmt.Errorf("factapply: missing return target %d", target))
		}
	}

	// Coordinate families are the native N5 carrier. The dense topology is
	// validated once; no physical lane is decomposed inside the transaction.
	lanes := make([]ReturnFactorLane, len(in.Lanes))
	coordinate := make([]returnCoordinateLane, len(lanes))
	for index, input := range in.Lanes {
		lane, ok := in.Topology.Lane(index)
		if !ok || input.Lane != lane {
			return fail(fmt.Errorf("factapply: return factor %d has foreign lane ownership", index))
		}
		families, err := in.Domain.CoordinateFamilies(lane)
		if err != nil {
			return fail(err)
		}
		coordinate[index].lane = lane
		coordinate[index].families = families
		coordinate[index].skeletons = make([]state.CoordinateFamilySkeleton, len(families))
		coordinate[index].scalars = make([][]state.CoordinateScalarFactor, len(families))
		lanes[index] = ReturnFactorLane{Lane: lane, Ordinary: input.Ordinary}
		if len(families) == 0 {
			if input.Ordinary.Lane() != lane || len(input.Families) != 0 {
				return fail(fmt.Errorf("factapply: ordinary return factor %d is malformed", index))
			}
			continue
		}
		if input.Ordinary.Lane() == lane || len(input.Families) != len(families) {
			return fail(fmt.Errorf("factapply: coordinate return factor %d is malformed", index))
		}
		lanes[index].Families = make([]state.CoordinateFamilyFactor, len(families))
		for familyIndex, family := range families {
			factor := input.Families[familyIndex]
			if factor.Family() != family || factor.Skeleton().KeySpace() != in.Keys {
				return fail(fmt.Errorf("factapply: return family %d/%d is malformed", index, familyIndex))
			}
			sealed, sealErr := in.Domain.SealCoordinateFamilyFactor(factor.Skeleton(), factor.Scalars())
			if sealErr != nil {
				return fail(sealErr)
			}
			coordinate[index].skeletons[familyIndex] = sealed.Skeleton()
			coordinate[index].scalars[familyIndex] = sealed.Scalars()
		}
	}

	// Bind results first. The source spelling, rather than any projected result
	// spelling, seeds returned-object reachability exactly as in concrete N5.
	values := state.ValueFactor[K]{Top: in.Values.Top}
	if !in.Values.Top && len(in.Values.Values) != 0 {
		values.Values = make(map[K]product.Value, len(in.Values.Values)+len(targets))
		for slot, value := range in.Values.Values {
			values.Values[slot] = value
		}
	}
	seedValues := make([]product.Value, 0, in.Return.ResultBindingCount())
	for index := 0; index < in.Return.ResultBindingCount(); index++ {
		source, target, ok := in.Return.ResultBinding(index)
		if !ok || source < 0 || source >= len(in.Sources) {
			return fail(fmt.Errorf("factapply: malformed return binding %d", index))
		}
		value := in.Sources[source]
		seedValues = append(seedValues, value)
		projects, ok := in.Return.ResultBindingProjectsHeap(index)
		if !ok {
			return fail(fmt.Errorf("factapply: malformed return projection %d", index))
		}
		if projects {
			projected, projectErr := projectReturnContainerFromFactors(authority, in.Domain, coordinate, value)
			if projectErr != nil {
				return fail(projectErr)
			}
			value = projected
		}
		if !values.Top {
			if values.Values == nil {
				values.Values = make(map[K]product.Value, len(targets))
			}
			values.Values[targets[target].Slot] = value
		}
	}

	// Least reachability over the finite identity graph. A queue executes each
	// newly reached vertex once; there is no artificial cap and cancellation is
	// observationally atomic through the fail closure above.
	seedRoots := make(map[identity.Term]bool)
	reachable := make(map[identity.Term]bool)
	adjacent := make(map[identity.Term][]identity.Term)
	for laneIndex := range coordinate {
		for familyIndex, skeleton := range coordinate[laneIndex].skeletons {
			if err := in.Domain.VisitCoordinateReturnIdentitySkeletonObservations(skeleton, func(observation state.CoordinateReturnIdentityObservation) bool {
				switch observation.Role() {
				case state.CoordinateReturnIdentitySeed:
					seedRoots[observation.Root()] = true
				case state.CoordinateReturnIdentitySkeletonEdge:
					adjacent[observation.Root()] = append(adjacent[observation.Root()], observation.Target())
				}
				return true
			}); err != nil {
				return fail(err)
			}
			for _, scalar := range coordinate[laneIndex].scalars[familyIndex] {
				if err := in.Domain.VisitCoordinateReturnIdentityScalarObservations(scalar, func(observation state.CoordinateReturnIdentityObservation) bool {
					if observation.Role() == state.CoordinateReturnIdentityScalarEdge {
						adjacent[observation.Root()] = append(adjacent[observation.Root()], observation.Target())
					}
					return true
				}); err != nil {
					return fail(err)
				}
			}
		}
	}
	queue := make([]identity.Term, 0, len(seedValues))
	for _, value := range seedValues {
		root, exact := product.Get(reg, value, identity.Key).Term()
		if !exact || !root.Valid() || reachable[root] {
			continue
		}
		if seedRoots[root] {
			reachable[root] = true
			queue = append(queue, root)
		}
	}
	for head := 0; head < len(queue); head++ {
		if head&255 == 0 {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
		}
		for _, to := range adjacent[queue[head]] {
			if !reachable[to] {
				reachable[to] = true
				queue = append(queue, to)
			}
		}
	}
	identities := make([]identity.Term, 0, len(reachable))
	for root := range reachable {
		identities = append(identities, root)
	}
	sort.Slice(identities, func(i, j int) bool { return identity.Less(identities[i], identities[j]) })
	for _, root := range identities {
		for laneIndex := range coordinate {
			for familyIndex, family := range coordinate[laneIndex].families {
				roles, err := in.Domain.CoordinateReturnIdentityRoles(family)
				if err != nil {
					return fail(err)
				}
				if !roles.Has(state.CoordinateReturnIdentityPublisher) {
					continue
				}
				slot, handled, err := in.Domain.CoordinateReturnIdentityTermSlot(family, in.Keys, root)
				if err != nil {
					return fail(err)
				}
				if !handled {
					continue
				}
				scalars := coordinate[laneIndex].scalars[familyIndex]
				position, found, err := returnCoordinatePosition(in.Domain, scalars, slot)
				if err != nil {
					return fail(err)
				}
				var current state.CoordinateScalarFactor
				if found {
					current = scalars[position]
				} else {
					current, err = in.Domain.CoordinateDefault(coordinate[laneIndex].skeletons[familyIndex], slot)
					if err != nil {
						return fail(err)
					}
				}
				published, err := in.Domain.PublishCoordinateReturnIdentity(current)
				if err != nil {
					return fail(err)
				}
				if found {
					scalars[position] = published
				} else {
					scalars = append(scalars, state.CoordinateScalarFactor{})
					copy(scalars[position+1:], scalars[position:])
					scalars[position] = published
				}
				coordinate[laneIndex].scalars[familyIndex] = scalars
			}
		}
	}

	if len(targets) >= 2 {
		rows := make([]CallReturnPresenceRowTarget, 0, len(targets))
		for index := 0; index < in.Return.ResultTargetCount(); index++ {
			target, _ := in.Return.ResultTarget(index)
			value := product.Bottom(reg)
			if values.Top {
				value = product.Top()
			} else if found, ok := values.Values[targets[target].Slot]; ok {
				value = found
			}
			rows = append(rows, CallReturnPresenceRowTarget{Index: target, Path: targets[target].Path, Value: value})
		}
		plan, err := authority.paths.PrepareCallReturnPresenceRowInKeySpace(reg, in.Keys, in.Return.Point(), rows)
		if err != nil {
			return fail(err)
		}
		family, ok := in.Domain.PathEvidenceCoordinateFamily()
		if !ok {
			return fail(fmt.Errorf("factapply: return presence has no registered coordinate authority"))
		}
		laneIndex := -1
		familyIndex := -1
		for index := range coordinate {
			if coordinate[index].lane == family.Lane() {
				laneIndex = index
				for candidate, owned := range coordinate[index].families {
					if owned == family {
						familyIndex = candidate
					}
				}
			}
		}
		if laneIndex < 0 || familyIndex < 0 {
			return fail(fmt.Errorf("factapply: return presence coordinate family is outside the product"))
		}
		coordinate[laneIndex].skeletons[familyIndex], coordinate[laneIndex].scalars[familyIndex], err = plan.ApplyCoordinates(
			in.Domain, coordinate[laneIndex].skeletons[familyIndex], coordinate[laneIndex].scalars[familyIndex],
		)
		if err != nil {
			return fail(err)
		}
	}

	for index := range coordinate {
		if len(coordinate[index].families) == 0 {
			continue
		}
		for familyIndex := range coordinate[index].families {
			factor, err := in.Domain.SealCoordinateFamilyFactor(
				coordinate[index].skeletons[familyIndex], coordinate[index].scalars[familyIndex],
			)
			if err != nil {
				return fail(err)
			}
			lanes[index].Families[familyIndex] = factor
		}
	}
	return ReturnFactorResult[K]{Values: values, Lanes: lanes}, nil
}

func projectReturnContainerFromFactors(authority *ReturnAuthority, domain state.ProductDomain, lanes []returnCoordinateLane, value product.Value) (product.Value, error) {
	reg := domain.Registry()
	rootTerm, exact := product.Get(reg, value, identity.Key).Term()
	if !exact || !rootTerm.Valid() {
		return value, nil
	}
	owner, ok := domain.ReturnIdentityContainerFamily()
	if !ok {
		return value, nil
	}
	for laneIndex := range lanes {
		for familyIndex, family := range lanes[laneIndex].families {
			if family != owner {
				continue
			}
			skeleton := lanes[laneIndex].skeletons[familyIndex]
			for _, scalar := range lanes[laneIndex].scalars[familyIndex] {
				var container product.Value
				found := false
				if err := domain.VisitCoordinateReturnIdentityScalarObservations(scalar, func(observation state.CoordinateReturnIdentityObservation) bool {
					if observation.Role() == state.CoordinateReturnIdentityContainer && observation.Root() == rootTerm {
						container, found = observation.Value(), true
						return false
					}
					return true
				}); err != nil {
					return product.Value{}, err
				}
				if !found {
					continue
				}
				var visitErr error
				projected, projectedOK := authority.ProjectFactoredHeapContainer(reg, value, container, func(visit func(fact dynamicindex.Fact)) {
					_, visitErr = domain.VisitCoordinateReturnContainerFacts(skeleton, rootTerm, visit)
				})
				if visitErr != nil {
					return product.Value{}, visitErr
				}
				if projectedOK {
					return projected, nil
				}
			}
		}
	}
	return value, nil
}

func returnCoordinatePosition(domain state.ProductDomain, scalars []state.CoordinateScalarFactor, slot state.CoordinateSlot) (int, bool, error) {
	for index, scalar := range scalars {
		equal, err := domain.CoordinateSlotEqual(scalar.Slot(), slot)
		if err != nil {
			return 0, false, err
		}
		if equal {
			return index, true, nil
		}
		less, err := domain.CoordinateSlotLess(scalar.Slot(), slot)
		if err != nil {
			return 0, false, err
		}
		if !less {
			return index, false, nil
		}
	}
	return len(scalars), false, nil
}
