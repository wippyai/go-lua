package factapply

import (
	"context"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// applyValueRefinementFactorState is the concrete storage adapter for the one
// canonical ValueRefinementFactorProgram. It contains no refinement semantics:
// State is decomposed into registered factors, the carrier-neutral program is
// evaluated, and its detached factor image is recomposed atomically.
func applyValueRefinementFactorState(
	domain state.ProductDomain,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	input state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
	guard bool,
) (state.State, bool, error) {
	if !domain.Valid() || resolver == nil || !resolver.KeySpace().Valid() || targetPath.Symbol == 0 {
		return input, true, nil
	}
	if domain.Lattice().Equal(input, domain.Lattice().Bottom()) {
		return input, false, nil
	}
	target, ok := visibility.AddressAt(resolver, point, targetPath).VisibleKeyspaceKey()
	if !ok {
		target, ok = visibility.AddressAt(resolver, point, targetPath).RootOrVisibleKeyspaceKey()
	}
	if !ok {
		return input, true, nil
	}
	residual, values := state.DecomposeValueLane(domain.Lattice(), input)
	pathFamily, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		return input, true, nil
	}
	pathFactors, err := domain.DecomposeLanes(residual, []state.ProductLane{pathFamily.Lane()})
	if err != nil {
		return input, true, err
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(pathFactors[0], pathFamily, resolver.KeySpace())
	if err != nil {
		return input, true, err
	}
	inventory, err := domain.CoordinateFactorInventoryFromPreparedState(resolver.KeySpace(), input)
	if err != nil {
		return input, true, err
	}
	plan, err := domain.SealValueRefinementPlan(resolver.KeySpace(), target, inventory)
	if err != nil {
		return input, true, err
	}
	prepare := PrepareValueRefinementFactorProgram[statekey.Value]
	if guard {
		prepare = PrepareGuardValueRefinementFactorProgram[statekey.Value]
	}
	program, err := prepare(
		domain, plan, refinement,
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		typeValues, projectPath,
	)
	if err != nil {
		return input, true, err
	}
	factors, err := domain.DecomposeLanes(residual, program.Lanes())
	if err != nil {
		return input, true, err
	}
	mutation, err := domain.DecomposePathDescendantMutationFactors(residual, resolver.KeySpace())
	if err != nil {
		return input, true, err
	}
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, state.ValueLaneFactor{}, true,
		program.PathEvidenceAuthority(), mutation,
	)
	if err != nil {
		return input, true, err
	}
	frame, err := program.Apply(context.Background(), nil, ValueRefinementFactorFrame[statekey.Value]{
		Values: values, Factors: factors, Carrier: carrier, Reachable: true,
	})
	if err != nil {
		return input, true, err
	}
	if !frame.Reachable {
		return domain.Lattice().Bottom(), false, nil
	}
	nextSkeleton, nextScalars, _, mutationLanes, mutationCoordinates, _, err := frame.Carrier.Freeze()
	if err != nil {
		return input, true, err
	}
	pathFactor, err := domain.ReplaceCoordinateFamily(pathFactors[0], nextSkeleton, nextScalars)
	if err != nil {
		return input, true, err
	}
	patchByLane := make(map[state.ProductLane]state.LaneFactor, len(frame.Factors)+len(mutationLanes)+len(mutationCoordinates)+1)
	for _, factor := range append(append([]state.LaneFactor(nil), frame.Factors...), mutationLanes...) {
		patchByLane[factor.Lane()] = factor
	}
	patchByLane[pathFactor.Lane()] = pathFactor
	for _, coordinate := range mutationCoordinates {
		lane := coordinate.Family().Lane()
		base, baseErr := domain.DecomposeLanes(residual, []state.ProductLane{lane})
		if baseErr != nil || len(base) != 1 {
			return input, true, fmt.Errorf("factapply: value-refinement coordinate lane is absent")
		}
		patched, patchErr := domain.ReplaceCoordinateFamily(base[0], coordinate.Skeleton(), coordinate.Scalars())
		if patchErr != nil {
			return input, true, patchErr
		}
		patchByLane[lane] = patched
	}
	patch := make([]state.LaneFactor, 0, len(patchByLane))
	ids := make([]state.LaneID, 0, len(patchByLane))
	for _, lane := range domain.LaneInventory() {
		if factor, present := patchByLane[lane]; present {
			patch = append(patch, factor)
			ids = append(ids, lane.ID())
		}
	}
	delta, err := domain.ComposeSparse(patch)
	if err != nil {
		return input, true, err
	}
	residual, err = domain.PatchFactors(residual, delta, state.NewLaneSet(ids...))
	if err != nil {
		return input, true, err
	}
	return state.RecomposeValueLane(domain.Registry(), domain.Lattice(), residual, frame.Values), true, nil
}
