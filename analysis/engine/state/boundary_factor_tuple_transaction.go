package state

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BoundaryFactorTuple is one complete factor-native product. Values and
// Factors are deliberately separate physical carriers but one semantic value;
// Factors must follow ProductDomain.NonValuesLaneInventory exactly.
type BoundaryFactorTuple[K comparable] struct {
	Values  ValueFactor[K]
	Factors []LaneFactor
}

// BoundaryFactorRootTarget separates structural root identity from optional
// Values publication. Pass-by-value roots participate in every factor law but
// deliberately omit their caller scalar write.
type BoundaryFactorRootTarget[K comparable] struct {
	Slot        K
	WriteScalar bool
}

// ApplyBoundaryFactorTuple is the sole complete-product boundary executor.
// The plan owns structural projection/rebase, the generic value transport owns
// slot identity, and every non-Values lane dispatches only through registered
// ordinary/coordinate laws.  All outputs are scratch until every lane has
// succeeded, so failure or cancellation returns no partial tuple.
func ApplyBoundaryFactorTuple[S, T comparable](
	ctx context.Context,
	plan BoundaryFactorTransportPlan,
	values BoundaryValueFactorTransport[S, T],
	destination BoundaryFactorTuple[T],
	source BoundaryFactorTuple[S],
	sourceRoots []product.Value,
	targetRoots []BoundaryFactorRootTarget[T],
) (BoundaryFactorTuple[T], error) {
	if ctx == nil || plan.seal == nil || !plan.domain.Valid() || !plan.sourceDomain.Valid() || !values.valid() ||
		values.owner != plan.seal || values.domain.seal != plan.domain.seal || len(targetRoots) != len(plan.targets) {
		return BoundaryFactorTuple[T]{}, fmt.Errorf("%w: boundary factor tuple is unowned", ErrInvalidLaneFactor)
	}
	if err := ctx.Err(); err != nil {
		return BoundaryFactorTuple[T]{}, err
	}
	destinationWidth := plan.domain.NonValuesLaneCount()
	sourceWidth := plan.sourceDomain.NonValuesLaneCount()
	if len(destination.Factors) != destinationWidth || len(source.Factors) != sourceWidth || destinationWidth != sourceWidth {
		return BoundaryFactorTuple[T]{}, fmt.Errorf("%w: boundary factor tuple inventory is incomplete", ErrIncompleteLaneFactors)
	}
	for index := 0; index < destinationWidth; index++ {
		destinationLane, destinationOK := plan.domain.NonValuesLaneAt(index)
		sourceLane, sourceOK := plan.sourceDomain.NonValuesLaneAt(index)
		if !destinationOK || !sourceOK || destination.Factors[index].Lane() != destinationLane ||
			source.Factors[index].Lane().ID() != destinationLane.ID() ||
			source.Factors[index].Lane() != sourceLane {
			return BoundaryFactorTuple[T]{}, fmt.Errorf("%w: boundary factor tuple lane %d drifted", ErrIncompleteLaneFactors, index)
		}
	}
	if len(sourceRoots) != len(plan.sourceTargets) {
		return BoundaryFactorTuple[T]{}, fmt.Errorf("%w: boundary root tuple width %d, want %d", ErrIncompleteLaneFactors, len(sourceRoots), len(plan.sourceTargets))
	}
	rootValues := make([]product.Value, len(plan.targets))
	rootPresent := make([]bool, len(plan.targets))
	valueDomain := product.Domain(plan.domain.reg)
	for sourceIndex, value := range sourceRoots {
		contributions, err := plan.RebaseRootSource(sourceIndex, value)
		if err != nil {
			return BoundaryFactorTuple[T]{}, err
		}
		for _, contribution := range contributions {
			if contribution.Target < 0 || contribution.Target >= len(rootValues) {
				return BoundaryFactorTuple[T]{}, ErrInvalidLaneFactor
			}
			if rootPresent[contribution.Target] {
				rootValues[contribution.Target] = valueDomain.Join(rootValues[contribution.Target], contribution.Value)
			} else {
				rootValues[contribution.Target] = contribution.Value
				rootPresent[contribution.Target] = true
			}
		}
	}
	for target, present := range rootPresent {
		if !present {
			return BoundaryFactorTuple[T]{}, fmt.Errorf("%w: boundary destination root %d has no scalar contribution", ErrIncompleteLaneFactors, target)
		}
	}
	establishesReachability := false
	for target, schema := range plan.targets {
		if schema.Path.Kind != keyspace.KindInvalid || !product.Equal(plan.domain.reg, rootValues[target], product.Bottom(plan.domain.reg)) {
			establishesReachability = true
			break
		}
	}

	nextFactors := append([]LaneFactor(nil), destination.Factors...)
	for index := 0; index < destinationWidth; index++ {
		if err := ctx.Err(); err != nil {
			return BoundaryFactorTuple[T]{}, err
		}
		sourceRuntime, err := plan.sourceDomain.validateFactor(source.Factors[index])
		if err != nil {
			return BoundaryFactorTuple[T]{}, err
		}
		if len(sourceRuntime.coordinates) != 0 {
			nextFactors[index], err = plan.applyCoordinateFactor(
				nextFactors[index], source.Factors[index], rootValues, establishesReachability,
			)
			if err != nil {
				return BoundaryFactorTuple[T]{}, err
			}
			continue
		}
		patch, err := plan.PrepareFactor(source.Factors[index], rootValues, establishesReachability)
		if err != nil {
			return BoundaryFactorTuple[T]{}, err
		}
		nextFactors[index], err = patch.ApplyLane(nextFactors[index])
		if err != nil {
			return BoundaryFactorTuple[T]{}, err
		}
	}
	rootWrites := make([]BoundaryValueSlotContribution[T], 0, len(rootValues))
	for index, target := range targetRoots {
		if target.WriteScalar {
			rootWrites = append(rootWrites, BoundaryValueSlotContribution[T]{Slot: target.Slot, Value: rootValues[index]})
		}
	}
	nextValues, err := values.ApplyBoundary(destination.Values, source.Values, rootWrites)
	if err != nil {
		return BoundaryFactorTuple[T]{}, err
	}
	return BoundaryFactorTuple[T]{Values: nextValues, Factors: nextFactors}, nil
}

// applyCoordinateFactor executes every registered coordinate family through
// the same sparse family lift used by guarded execution.  A complete tuple is
// merely a dense carrier for those family coordinates; it is not a second
// license to reinterpret the lane through its historical whole-lane boundary
// callbacks.
func (p BoundaryFactorTransportPlan) applyCoordinateFactor(
	destination, source LaneFactor,
	roots []product.Value,
	establishesReachability bool,
) (LaneFactor, error) {
	sourceRuntime, err := p.sourceDomain.validateFactor(source)
	if err != nil || len(sourceRuntime.coordinates) == 0 {
		return LaneFactor{}, fmt.Errorf("%w: coordinate source factor", ErrInvalidLaneFactor)
	}
	targetRuntime, err := p.domain.validateFactor(destination)
	if err != nil || targetRuntime.lane.id != sourceRuntime.lane.id || len(targetRuntime.coordinates) != len(sourceRuntime.coordinates) {
		return LaneFactor{}, fmt.Errorf("%w: coordinate destination factor", ErrInvalidLaneFactor)
	}

	result := destination
	for index, sourceCoordinate := range sourceRuntime.coordinates {
		targetCoordinate := targetRuntime.coordinates[index]
		if targetCoordinate.family.id != sourceCoordinate.family.id {
			return LaneFactor{}, fmt.Errorf("%w: coordinate family inventory differs across boundary", ErrIncompleteLaneFactors)
		}
		sourceSkeleton, sourceScalars, decomposeErr := p.sourceDomain.DecomposeCoordinateFamily(
			source, sourceCoordinate.family, p.projectCtx.keys,
		)
		if decomposeErr != nil {
			return LaneFactor{}, decomposeErr
		}
		sourceShape, shapeErr := p.sourceDomain.SealCoordinateFamilyShape(sourceSkeleton, boundaryCoordinateScalarSlots(sourceScalars))
		if shapeErr != nil {
			return LaneFactor{}, shapeErr
		}
		destinationSkeleton, destinationScalars, decomposeErr := p.domain.DecomposeCoordinateFamily(
			result, targetCoordinate.family, p.keys,
		)
		if decomposeErr != nil {
			return LaneFactor{}, decomposeErr
		}
		destinationShape, shapeErr := p.domain.SealCoordinateFamilyShape(destinationSkeleton, boundaryCoordinateScalarSlots(destinationScalars))
		if shapeErr != nil {
			return LaneFactor{}, shapeErr
		}
		lift, liftErr := p.PrepareCoordinateBoundaryFamilyLift(sourceShape, destinationShape, establishesReachability)
		if liftErr != nil {
			return LaneFactor{}, liftErr
		}
		result, err = lift.Apply(result, source, roots)
		if err != nil {
			return LaneFactor{}, err
		}
	}
	return result, nil
}

func boundaryCoordinateScalarSlots(values []CoordinateScalarFactor) []CoordinateSlot {
	out := make([]CoordinateSlot, len(values))
	for index := range values {
		out[index] = values[index].slot
	}
	return out
}
