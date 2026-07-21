package factapply

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// RootAssignmentFactorComponentKind names one indivisible N4 dependency
// hyperedge. The kinds are semantic barriers, not execution modes: together
// they are the one RootAssignmentFactorProgram, while each may be lifted over
// only the factors it declares.
type RootAssignmentFactorComponentKind uint8

const (
	RootAssignmentFactorComponentInvalid RootAssignmentFactorComponentKind = iota
	RootAssignmentFactorComponentSource
	RootAssignmentFactorComponentPath
	RootAssignmentFactorComponentScalar
	RootAssignmentFactorComponentCompletion
)

// RootAssignmentFactorComponent is a ProductDomain-sealed N4 hyperedge.
// Current and PointEntry are independent input authorities. Outputs are an
// independent authority so read-only factors cannot be republished by
// construction. All slices and semantic fields are private and immutable.
type RootAssignmentFactorComponent struct {
	program    RootAssignmentFactorProgram
	kind       RootAssignmentFactorComponentKind
	stages     []RootAssignmentFactorStage
	current    state.ProductFactorSelection
	pointEntry state.ProductFactorSelection
	outputs    state.ProductFactorSelection

	ordinary state.ProductLane
	family   state.CoordinateFamily
	demands  []state.RootAssignmentScalarCoordinateDemand
}

// RootAssignmentFactorComponentInventory supplies the already-sealed body
// coordinate universe plus the exact Values terms read by the canonical
// source program. It contains topology only; no runtime State is admitted.
type RootAssignmentFactorComponentInventory struct {
	Coordinates  state.CoordinateFactorInventory
	SourceValues []statekey.Value
}

func (c RootAssignmentFactorComponent) Valid() bool {
	if !c.program.Valid() || c.kind == RootAssignmentFactorComponentInvalid || len(c.stages) == 0 {
		return false
	}
	domain := c.program.plan.authority.domain
	return domain.OwnsProductFactorSelection(c.current) &&
		domain.OwnsProductFactorSelection(c.pointEntry) &&
		domain.OwnsProductFactorSelection(c.outputs)
}

func (c RootAssignmentFactorComponent) Kind() RootAssignmentFactorComponentKind { return c.kind }

func (c RootAssignmentFactorComponent) Stages() []RootAssignmentFactorStage {
	if !c.Valid() {
		return nil
	}
	return append([]RootAssignmentFactorStage(nil), c.stages...)
}

func (c RootAssignmentFactorComponent) CurrentInputs() (state.ProductFactorSelection, bool) {
	return c.current, c.Valid()
}

func (c RootAssignmentFactorComponent) PointEntryInputs() (state.ProductFactorSelection, bool) {
	return c.pointEntry, c.Valid()
}

func (c RootAssignmentFactorComponent) Outputs() (state.ProductFactorSelection, bool) {
	return c.outputs, c.Valid()
}

// RootAssignmentFactorComponentInput binds the three separately authorized
// frames of one component evaluation. OutputBase is the output projection of
// the original current carrier; it is never inferred from a read frame.
type RootAssignmentFactorComponentInput struct {
	Current    state.ProductFactorFrame
	PointEntry state.ProductFactorFrame
	OutputBase state.ProductFactorFrame
}

// ApplyComponent executes an independently factorable scalar N4 hyperedge and
// publishes through the typed output transaction. The correlated source,
// path, and completion arms consume additional semantic operands and are
// implemented by their canonical phase evaluators rather than this unary
// factor operation.
func (c RootAssignmentFactorComponent) ApplyComponent(input RootAssignmentFactorComponentInput) (state.ProductFactorFrame, error) {
	if !c.Valid() || c.kind != RootAssignmentFactorComponentScalar {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment component is not independently scalar")
	}
	domain := c.program.plan.authority.domain
	if !domain.OwnsProductFactorFrame(c.current, input.Current) ||
		!domain.OwnsProductFactorFrame(c.pointEntry, input.PointEntry) ||
		!domain.OwnsProductFactorFrame(c.outputs, input.OutputBase) {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: foreign RootAssignment component frame")
	}
	transaction, err := domain.BeginProductFactorFrameTransaction(c.outputs, input.OutputBase)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	if c.ordinary.ID() != "" {
		current, point := input.Current.OrdinaryFactors(), input.PointEntry.OrdinaryFactors()
		if len(current) != 1 || len(point) != 1 {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: incomplete RootAssignment scalar lane frames")
		}
		output, applyErr := c.program.ApplyScalarFactor(point[0], current[0])
		if applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
		if applyErr = transaction.WriteOrdinary(c.ordinary, output); applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
		return transaction.Finish()
	}
	if len(c.demands) == 0 {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment scalar coordinate has no demands")
	}
	currentFactors, pointFactors := input.Current.CoordinateFactors(), input.PointEntry.CoordinateFactors()
	if len(currentFactors) != 1 || len(pointFactors) > 1 || currentFactors[0].Family() != c.family ||
		len(pointFactors) == 1 && pointFactors[0].Family() != c.family {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: incomplete RootAssignment scalar coordinate frames")
	}
	current := currentFactors[0]
	nextSkeleton, nextScalars := current.Skeleton(), current.Scalars()
	var point state.CoordinateFamilyFactor
	if len(pointFactors) == 1 {
		point = pointFactors[0]
	}
	for _, demand := range c.demands {
		target, targetOK, factorErr := rootAssignmentCoordinateFactorAt(domain, nextSkeleton, nextScalars, demand.Target())
		if factorErr != nil {
			return state.ProductFactorFrame{}, factorErr
		}
		if !targetOK {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment scalar target is outside its component")
		}
		var source state.CoordinateScalarFactor
		sourceSlot, hasSource := demand.PointSource()
		if hasSource {
			if len(pointFactors) != 1 {
				return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment scalar point source is absent")
			}
			source, targetOK, factorErr = rootAssignmentCoordinateFactorAt(domain, point.Skeleton(), point.Scalars(), sourceSlot)
			if factorErr != nil || !targetOK {
				if factorErr == nil {
					factorErr = fmt.Errorf("factapply: RootAssignment scalar point source is outside its component")
				}
				return state.ProductFactorFrame{}, factorErr
			}
		}
		nextSkeleton, target, factorErr = c.program.ApplyScalarCoordinate(nextSkeleton, target, source, hasSource)
		if factorErr != nil {
			return state.ProductFactorFrame{}, factorErr
		}
		nextScalars, factorErr = replaceRootAssignmentCoordinateFactor(domain, nextScalars, target)
		if factorErr != nil {
			return state.ProductFactorFrame{}, factorErr
		}
	}
	output, err := domain.SealCoordinateFamilyFactor(nextSkeleton, nextScalars)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	if err = transaction.WriteCoordinate(c.family, output); err != nil {
		return state.ProductFactorFrame{}, err
	}
	return transaction.Finish()
}

func rootAssignmentCoordinateFactorAt(
	domain state.ProductDomain,
	skeleton state.CoordinateFamilySkeleton,
	scalars []state.CoordinateScalarFactor,
	slot state.CoordinateSlot,
) (state.CoordinateScalarFactor, bool, error) {
	for _, scalar := range scalars {
		equal, err := domain.CoordinateSlotEqual(scalar.Slot(), slot)
		if err != nil {
			return state.CoordinateScalarFactor{}, false, err
		}
		if equal {
			return scalar, true, nil
		}
	}
	factor, err := domain.CoordinateDefault(skeleton, slot)
	return factor, err == nil, err
}

func replaceRootAssignmentCoordinateFactor(
	domain state.ProductDomain,
	scalars []state.CoordinateScalarFactor,
	update state.CoordinateScalarFactor,
) ([]state.CoordinateScalarFactor, error) {
	out := append([]state.CoordinateScalarFactor(nil), scalars...)
	for index, scalar := range out {
		equal, err := domain.CoordinateSlotEqual(scalar.Slot(), update.Slot())
		if err != nil {
			return nil, err
		}
		if equal {
			out[index] = update
			return out, nil
		}
	}
	out = append(out, update)
	sort.Slice(out, func(i, j int) bool {
		less, _ := domain.CoordinateSlotLess(out[i].Slot(), out[j].Slot())
		return less
	})
	return out, nil
}

// RootAssignmentFactorComponents seals the exact source/path/scalar N4
// hyperedges. Completion is sealed separately once its skeleton-only
// fresh-empty input can be represented without promoting a coordinate family
// to a whole-lane dependency.
func (p RootAssignmentFactorProgram) RootAssignmentFactorComponents(
	inventory RootAssignmentFactorComponentInventory,
) ([]RootAssignmentFactorComponent, error) {
	if !p.Valid() {
		return nil, fmt.Errorf("factapply: invalid RootAssignment component program")
	}
	domain := p.plan.authority.domain
	keys, ok := p.plan.PathKeySpace()
	if !ok || !inventory.Coordinates.ValidFor(domain, keys) {
		return nil, fmt.Errorf("factapply: foreign RootAssignment component inventory")
	}
	empty, err := domain.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		return nil, err
	}
	seal := func(lanes []state.ProductLane, coordinates []state.CoordinateSlot, values []statekey.Value, valuesTop bool) (state.ProductFactorSelection, error) {
		coordinateInventory, sealErr := domain.SealCoordinateFactorInventory(keys, coordinates)
		if sealErr != nil {
			return state.ProductFactorSelection{}, sealErr
		}
		return domain.SealProductFactorSelection(canonicalRootAssignmentLanes(lanes), coordinateInventory, canonicalRootAssignmentValues(values), valuesTop)
	}
	emptySelection := func() (state.ProductFactorSelection, error) {
		return domain.SealProductFactorSelection(nil, empty, nil, false)
	}

	target, ok := p.plan.TargetValueSlot()
	if !ok {
		return nil, fmt.Errorf("factapply: RootAssignment component has no target Values factor")
	}
	sourceValues := canonicalRootAssignmentValues(inventory.SourceValues)
	currentValues := append(append([]statekey.Value(nil), sourceValues...), target)
	currentValues = canonicalRootAssignmentValues(currentValues)

	var pathCurrent, pathPoint, pathWrites []state.CoordinateSlot
	if family, present := domain.PathValueFamily(); present {
		familySlots, familyErr := inventory.Coordinates.FamilySlots(family)
		if familyErr != nil {
			return nil, familyErr
		}
		schedule, scheduleErr := p.plan.PathDependencies(domain, familySlots)
		if scheduleErr != nil {
			return nil, scheduleErr
		}
		coordinatePlan, present := schedule.CoordinatePlan()
		if !present {
			return nil, fmt.Errorf("factapply: RootAssignment path component is unsealed")
		}
		pathCurrent = coordinatePlan.Coordinates()
		for _, id := range coordinatePlan.IDs() {
			dependency, dependencyOK := coordinatePlan.Dependency(id)
			if !dependencyOK {
				return nil, fmt.Errorf("factapply: RootAssignment path dependency %d is absent", id)
			}
			pathPoint = append(pathPoint, dependency.CoordinateReads()...)
			pathWrites = append(pathWrites, dependency.CoordinateWrites()...)
		}
	}

	var dynamicInputs, dynamicOutputs []state.ProductLane
	if dependencies, present, dependencyErr := p.plan.DynamicSourceDependencies(); dependencyErr != nil {
		return nil, dependencyErr
	} else if present {
		dynamicInputs = dependencies.InputLanes()
		dynamicOutputs = domain.RootAssignmentDynamicSourceLanes()
	}
	equality := domain.PathEqualityQuotientLanes()

	var objectCoordinates []state.CoordinateSlot
	if object, present := p.plan.ObjectLiteralSourcePlan(); present {
		constructor, constructorErr := object.PrepareObjectConstructorPlan(domain, keys)
		if constructorErr != nil {
			return nil, constructorErr
		}
		objectCoordinates, err = domain.ObjectConstructorCoordinateWrites(constructor)
		if err != nil {
			return nil, err
		}
	}

	components := make([]RootAssignmentFactorComponent, 0, 4)
	sourceCurrent, err := seal(dynamicInputs, objectCoordinates, currentValues, true)
	if err != nil {
		return nil, err
	}
	sourcePoint, err := seal(nil, pathPoint, nil, false)
	if err != nil {
		return nil, err
	}
	sourceOutputs, err := seal(nil, objectCoordinates, []statekey.Value{target}, false)
	if err != nil {
		return nil, err
	}
	sourceStages := []RootAssignmentFactorStage{RootAssignmentFactorStageSourceComposition, RootAssignmentFactorStageValuePublication}
	if p.ownsStage(RootAssignmentFactorStageObjectMaterialization) {
		sourceStages = append([]RootAssignmentFactorStage{RootAssignmentFactorStageObjectMaterialization}, sourceStages...)
	}
	components = append(components, RootAssignmentFactorComponent{
		program: p, kind: RootAssignmentFactorComponentSource, stages: sourceStages,
		current: sourceCurrent, pointEntry: sourcePoint, outputs: sourceOutputs,
	})

	pathLanes := append(append([]state.ProductLane(nil), dynamicInputs...), dynamicOutputs...)
	pathLanes = append(pathLanes, equality...)
	pathOutputLanes := append(append([]state.ProductLane(nil), dynamicOutputs...), equality...)
	if len(pathWrites) != 0 || len(pathOutputLanes) != 0 {
		pathInput, pathErr := seal(pathLanes, pathCurrent, currentValues, false)
		if pathErr != nil {
			return nil, pathErr
		}
		pointInput, pathErr := seal(nil, pathPoint, nil, false)
		if pathErr != nil {
			return nil, pathErr
		}
		pathOutput, pathErr := seal(pathOutputLanes, pathWrites, nil, false)
		if pathErr != nil {
			return nil, pathErr
		}
		stages := []RootAssignmentFactorStage{RootAssignmentFactorStageSourceComposition}
		for _, stage := range p.stages {
			if stage == RootAssignmentFactorStagePathMutation || stage == RootAssignmentFactorStageDynamicSource || stage == RootAssignmentFactorStageEqualityQuotient {
				stages = append(stages, stage)
			}
		}
		components = append(components, RootAssignmentFactorComponent{
			program: p, kind: RootAssignmentFactorComponentPath, stages: stages,
			current: pathInput, pointEntry: pointInput, outputs: pathOutput,
		})
	}

	if transaction, present := p.plan.ScalarFactorTransaction(); present {
		for _, lane := range domain.RootAssignmentScalarLanes() {
			current, scalarErr := seal([]state.ProductLane{lane}, nil, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			point, scalarErr := seal([]state.ProductLane{lane}, nil, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			outputs, scalarErr := seal([]state.ProductLane{lane}, nil, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			components = append(components, RootAssignmentFactorComponent{
				program: p, kind: RootAssignmentFactorComponentScalar,
				stages:  []RootAssignmentFactorStage{RootAssignmentFactorStageScalarTransfer},
				current: current, pointEntry: point, outputs: outputs, ordinary: lane,
			})
		}
		for _, family := range domain.RootAssignmentScalarCoordinateFamilies() {
			familySlots, familyErr := inventory.Coordinates.FamilySlots(family)
			if familyErr != nil {
				return nil, familyErr
			}
			demands, demandErr := domain.RootAssignmentScalarCoordinateDemands(transaction, family, keys, familySlots)
			if demandErr != nil {
				return nil, demandErr
			}
			if len(demands) == 0 {
				continue
			}
			currentSlots := make([]state.CoordinateSlot, 0, len(demands))
			pointSlots := make([]state.CoordinateSlot, 0, len(demands))
			for _, demand := range demands {
				currentSlots = append(currentSlots, demand.Target())
				if source, sourceOK := demand.PointSource(); sourceOK {
					pointSlots = append(pointSlots, source)
				}
			}
			current, scalarErr := seal(nil, currentSlots, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			point, scalarErr := seal(nil, pointSlots, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			outputs, scalarErr := seal(nil, currentSlots, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			components = append(components, RootAssignmentFactorComponent{
				program: p, kind: RootAssignmentFactorComponentScalar,
				stages:  []RootAssignmentFactorStage{RootAssignmentFactorStageScalarTransfer},
				current: current, pointEntry: point, outputs: outputs, family: family, demands: demands,
			})
		}
	}

	_ = emptySelection // completion consumes this once skeleton-only selection is expressible.
	return components, nil
}

func canonicalRootAssignmentLanes(input []state.ProductLane) []state.ProductLane {
	out := append([]state.ProductLane(nil), input...)
	sort.Slice(out, func(i, j int) bool { return out[i].Ordinal() < out[j].Ordinal() })
	write := 0
	for _, lane := range out {
		if write == 0 || out[write-1].Ordinal() != lane.Ordinal() {
			out[write] = lane
			write++
		}
	}
	return out[:write]
}

func canonicalRootAssignmentValues(input []statekey.Value) []statekey.Value {
	out := append([]statekey.Value(nil), input...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, slot := range out {
		if slot != 0 && (write == 0 || out[write-1] != slot) {
			out[write] = slot
			write++
		}
	}
	return out[:write]
}
