package factapply

import (
	"context"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// applyPathEqualityFactorState is the concrete storage adapter for the one
// canonical PathEqualityFactorProgram. It owns no equality semantics: State
// is decomposed into registered factors, the closed program is evaluated, and
// the detached factor image is recomposed atomically.
func applyPathEqualityFactorState(
	domain state.ProductDomain,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	point cfg.Point,
	input state.State,
	leftPath, rightPath pathdom.Path,
) (state.State, bool, error) {
	if !domain.Valid() || resolver == nil || !resolver.KeySpace().Valid() || leftPath.Symbol == 0 || rightPath.Symbol == 0 {
		return input, true, nil
	}
	if domain.Lattice().Equal(input, domain.Lattice().Bottom()) {
		return input, false, nil
	}
	left, leftOK := visibility.AddressAt(resolver, point, leftPath).RootOrVisibleKeyspaceKey()
	right, rightOK := visibility.AddressAt(resolver, point, rightPath).RootOrVisibleKeyspaceKey()
	if !leftOK || !rightOK || left == right {
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
	union := make([]state.CoordinateSlot, len(scalars))
	for index := range scalars {
		union[index] = scalars[index].Slot()
	}
	plan, err := domain.SealPathEqualityFactorPlan(resolver.KeySpace(), left, right, union)
	if err != nil {
		return input, true, err
	}
	program, err := PreparePathEqualityFactorProgram(
		domain, plan,
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		typeValues,
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
		skeleton, scalars, state.ValueLaneFactor{}, true, program.PathEvidenceAuthority(), mutation,
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
	nextSkeleton, nextScalars, _, nextInvalidation, _, _, err := frame.Carrier.Freeze()
	if err != nil {
		return input, true, err
	}
	pathFactor, err := domain.ReplaceCoordinateFamily(pathFactors[0], nextSkeleton, nextScalars)
	if err != nil {
		return input, true, err
	}
	for _, replacement := range nextInvalidation {
		for index, lane := range program.Lanes() {
			if replacement.Lane() == lane {
				frame.Factors[index] = replacement
				break
			}
		}
	}
	patch := append(append([]state.LaneFactor(nil), frame.Factors...), pathFactor)
	delta, err := domain.ComposeSparse(patch)
	if err != nil {
		return input, true, err
	}
	ids := make([]state.LaneID, 0, len(program.Lanes())+1)
	for _, lane := range program.Lanes() {
		ids = append(ids, lane.ID())
	}
	ids = append(ids, pathFamily.Lane().ID())
	residual, err = domain.PatchFactors(residual, delta, state.NewLaneSet(ids...))
	if err != nil {
		return input, true, err
	}
	return state.RecomposeValueLane(domain.Registry(), domain.Lattice(), residual, frame.Values), true, nil
}
