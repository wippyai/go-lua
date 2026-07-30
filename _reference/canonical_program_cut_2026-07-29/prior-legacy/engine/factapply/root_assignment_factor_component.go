package factapply

import (
	"context"
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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

	pathReadAuthority  state.CoordinatePathEvidenceAuthority[statekey.Value]
	pathWriteAuthority state.CoordinatePathEvidenceAuthority[statekey.Value]
	fresh              []rootAssignmentFactorFreshQuery
}

type rootAssignmentFactorFreshQuery struct {
	path pathdom.Path
	slot statekey.Value
}

// RootAssignmentFactorComponentInventory supplies the already-sealed body
// coordinate universe plus the exact Values terms read by the canonical
// source program. It contains topology only; no runtime State is admitted.
type RootAssignmentFactorComponentInventory struct {
	Coordinates       state.CoordinateFactorInventory
	SourceValues      []statekey.Value
	FormalStableRoots []formal.Root
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
	Current           state.ProductFactorFrame
	PointEntry        state.ProductFactorFrame
	OutputBase        state.ProductFactorFrame
	Sources           []product.Value
	Context           context.Context
	FormalStableRoots []formal.Root
}

// ApplyComponent executes one complete N4 dependency hyperedge. Sources are
// the already-correlated results of the frozen source terms; every product
// operand is carried by one of the three independently sealed frames. The
// result owns Outputs only, so callers cannot republish a read-only factor.
func (c RootAssignmentFactorComponent) ApplyComponent(input RootAssignmentFactorComponentInput) (state.ProductFactorFrame, error) {
	if !c.Valid() {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: invalid RootAssignment component")
	}
	domain := c.program.plan.authority.domain
	if !domain.OwnsProductFactorFrame(c.current, input.Current) ||
		!domain.OwnsProductFactorFrame(c.pointEntry, input.PointEntry) ||
		!domain.OwnsProductFactorFrame(c.outputs, input.OutputBase) {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: foreign RootAssignment component frame")
	}
	switch c.kind {
	case RootAssignmentFactorComponentSource:
		return c.applySourceComponent(input)
	case RootAssignmentFactorComponentPath:
		return c.applyPathComponent(input)
	case RootAssignmentFactorComponentScalar:
		return c.applyScalarComponent(input)
	case RootAssignmentFactorComponentCompletion:
		return c.applyCompletionComponent(input)
	default:
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: unknown RootAssignment component")
	}
}

type rootAssignmentComponentSource struct {
	primary    product.Value
	composed   product.Value
	productive bool
	dynamic    RootAssignmentDynamicSourceTransaction
	hasDynamic bool
	object     PreparedGuardedObjectConstructor
}

func (c RootAssignmentFactorComponent) resolveSource(input RootAssignmentFactorComponentInput) (rootAssignmentComponentSource, error) {
	reg := c.program.plan.authority.domain.Registry()
	if len(input.Sources) != c.program.plan.transaction.SourceCount() {
		return rootAssignmentComponentSource{}, fmt.Errorf("factapply: incomplete RootAssignment component sources")
	}
	for _, source := range input.Sources {
		if !product.BelongsToRegistry(reg, source) {
			return rootAssignmentComponentSource{}, fmt.Errorf("factapply: foreign RootAssignment component source")
		}
	}
	out := rootAssignmentComponentSource{primary: input.Sources[0]}
	if object, present := c.program.plan.ObjectLiteralSourcePlan(); present {
		keys, _ := c.program.plan.PathKeySpace()
		prepared, err := object.PrepareGuardedObjectConstructor(c.program.plan.authority.domain, keys, input.Sources)
		if err != nil {
			return rootAssignmentComponentSource{}, err
		}
		primary, present := prepared.RootSourceValue()
		if !present {
			return rootAssignmentComponentSource{}, fmt.Errorf("factapply: RootAssignment object source is incomplete")
		}
		out.primary, out.object = primary, prepared
	}
	present, err := c.pointSourcePresent(input.PointEntry)
	if err != nil {
		return rootAssignmentComponentSource{}, err
	}
	if _, dynamicShape := c.program.plan.DynamicSourcePlan(); dynamicShape {
		out.dynamic, err = c.resolveDynamicSource(input)
		if err != nil {
			return rootAssignmentComponentSource{}, err
		}
		out.hasDynamic = true
		present = present || out.dynamic.DefinitelyPresent()
	}
	out.composed, out.productive, err = c.program.ComposeSource(out.primary, present)
	return out, err
}

func (c RootAssignmentFactorComponent) pointSourcePresent(frame state.ProductFactorFrame) (bool, error) {
	proof, present := c.program.plan.SourcePresenceProof()
	if !present {
		return false, nil
	}
	domain := c.program.plan.authority.domain
	family, present := domain.PathEvidenceCoordinateFamily()
	if !present {
		return false, fmt.Errorf("factapply: RootAssignment point presence has no owner")
	}
	factor, present := rootAssignmentFrameCoordinate(frame, family)
	if !present {
		return false, fmt.Errorf("factapply: RootAssignment point presence factor is absent")
	}
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		factor.Skeleton(), factor.Scalars(), state.ValueLaneFactor{}, false,
		c.pathReadAuthority, state.PathDescendantMutationFactors{},
	)
	if err != nil {
		return false, err
	}
	return carrier.HasProof(proof), nil
}

func (c RootAssignmentFactorComponent) resolveDynamicSource(input RootAssignmentFactorComponentInput) (RootAssignmentDynamicSourceTransaction, error) {
	domain := c.program.plan.authority.domain
	plan, present := c.program.plan.DynamicSourcePlan()
	if !present {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic source is absent")
	}
	dependencies, present, err := c.program.plan.DynamicSourceDependencies()
	if err != nil || !present {
		if err == nil {
			err = fmt.Errorf("factapply: RootAssignment dynamic dependencies are absent")
		}
		return RootAssignmentDynamicSourceTransaction{}, err
	}
	lanes := dependencies.InputLanes()
	factors := make([]state.LaneFactor, len(lanes))
	for index, lane := range lanes {
		var found bool
		factors[index], found = rootAssignmentFrameOrdinary(input.Current, lane)
		if !found {
			return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic input %q is absent", lane.ID())
		}
	}
	bound, err := domain.BindRootAssignmentDynamicSourceInputs(dependencies, factors)
	if err != nil {
		return RootAssignmentDynamicSourceTransaction{}, err
	}
	dynamicFacts, factsOK := bound.DynamicIndexFactor()
	memberships, membershipsOK := bound.KeyMembershipFactor()
	if !factsOK || !membershipsOK {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic inputs are incomplete")
	}
	keySource, present := plan.KeyValueInput()
	if !present {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic key is absent")
	}
	keyOrdinal, present := c.program.plan.transaction.SourceOrdinal(keySource)
	if !present || keyOrdinal < 0 || keyOrdinal >= len(input.Sources) {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment dynamic key source is absent")
	}
	reg := domain.Registry()
	inputs := RootAssignmentDynamicSourceInputs{
		KeyValue:           input.Sources[keyOrdinal],
		HasKeyValue:        !product.Equal(reg, input.Sources[keyOrdinal], product.Bottom(reg)),
		DynamicIndexFactor: dynamicFacts, KeyMembershipFactor: memberships,
	}
	if _, base, hasModulo := plan.ModuloLengthPresenceInput(); hasModulo {
		ordinal, found := c.program.plan.transaction.SourceOrdinal(base)
		if !found || ordinal < 0 || ordinal >= len(input.Sources) {
			return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment modulo source is absent")
		}
		inputs.ModuloBaseValue = input.Sources[ordinal]
		inputs.HasModuloBaseValue = !product.Equal(reg, input.Sources[ordinal], product.Bottom(reg))
	}
	if query, present, queryErr := plan.TableNonEmptyQuery(); queryErr != nil {
		return RootAssignmentDynamicSourceTransaction{}, queryErr
	} else if present {
		resolve := func(get func() (state.CoordinateSlot, bool)) (state.CoordinateScalarFactor, error) {
			slot, slotPresent := get()
			if !slotPresent {
				return state.CoordinateScalarFactor{}, fmt.Errorf("factapply: RootAssignment table query slot is absent")
			}
			factor, factorPresent := rootAssignmentFrameCoordinate(input.Current, slot.Family())
			if !factorPresent {
				return state.CoordinateScalarFactor{}, fmt.Errorf("factapply: RootAssignment table query family is absent")
			}
			scalar, found, factorErr := rootAssignmentCoordinateFactorAt(domain, factor.Skeleton(), factor.Scalars(), slot)
			if factorErr != nil {
				return state.CoordinateScalarFactor{}, factorErr
			}
			if !found {
				return state.CoordinateScalarFactor{}, fmt.Errorf("factapply: RootAssignment table query coordinate is absent")
			}
			return scalar, nil
		}
		length, queryErr := resolve(query.LenFloorSlot)
		if queryErr != nil {
			return RootAssignmentDynamicSourceTransaction{}, queryErr
		}
		refinement, queryErr := resolve(query.RefinementSlot)
		if queryErr != nil {
			return RootAssignmentDynamicSourceTransaction{}, queryErr
		}
		member, queryErr := resolve(query.StaticMemberSlot)
		if queryErr != nil {
			return RootAssignmentDynamicSourceTransaction{}, queryErr
		}
		queryInputs := RootAssignmentTableNonEmptyInputs{LenFloor: length, Refinement: refinement, StaticMember: member}
		if rootSlot, hasRoot := query.RootValueSlot(); hasRoot {
			root, found := rootAssignmentFrameValue(c.current, input.Current, rootSlot)
			if !found {
				return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: RootAssignment table root value is absent")
			}
			queryInputs.HasRootValue, queryInputs.RootValue = true, root
		}
		inputs.TableDefinitelyNonEmpty, queryErr = query.DefinitelyNonEmpty(c.program.plan.authority.paths.typeValues, queryInputs)
		if queryErr != nil {
			return RootAssignmentDynamicSourceTransaction{}, queryErr
		}
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return c.program.ResolveDynamicSource(ctx, inputs)
}

func (c RootAssignmentFactorComponent) applySourceComponent(input RootAssignmentFactorComponentInput) (state.ProductFactorFrame, error) {
	source, err := c.resolveSource(input)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	domain := c.program.plan.authority.domain
	output := input.OutputBase
	if source.object.Valid() {
		constructor, rows, present := source.object.ObjectConstructor()
		if !present {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment object constructor is absent")
		}
		output, err = domain.ApplyObjectConstructorFrame(constructor, rows, c.outputs, output)
		if err != nil {
			return state.ProductFactorFrame{}, err
		}
	}
	transaction, err := domain.BeginProductFactorFrameTransaction(c.outputs, output)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	if source.productive && !input.Current.ValuesTop() {
		published, present, publishErr := c.program.ApplyValuePublication(source.composed, true, false)
		if publishErr != nil || !present {
			if publishErr == nil {
				publishErr = fmt.Errorf("factapply: RootAssignment value publication is absent")
			}
			return state.ProductFactorFrame{}, publishErr
		}
		target, present := c.program.plan.TargetValueSlot()
		if !present {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment target value is absent")
		}
		if err := transaction.WriteValue(target, published); err != nil {
			return state.ProductFactorFrame{}, err
		}
	}
	return transaction.Finish()
}

func (c RootAssignmentFactorComponent) applyPathComponent(input RootAssignmentFactorComponentInput) (state.ProductFactorFrame, error) {
	source, err := c.resolveSource(input)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	domain := c.program.plan.authority.domain
	ordinary := make(map[state.ProductLane]state.LaneFactor)
	for index, lane := range c.outputs.OrdinaryLanes() {
		ordinary[lane] = input.OutputBase.OrdinaryFactors()[index]
	}
	coordinates := make(map[state.CoordinateFamily]state.CoordinateFamilyFactor)
	for _, factor := range input.OutputBase.CoordinateFactors() {
		coordinates[factor.Family()] = factor
	}
	var pathResult RootAssignmentPathFactorResult
	if source.productive && c.program.ownsStage(RootAssignmentFactorStagePathMutation) {
		factors, factorErr := c.pathMutationFactors(input.Current)
		if factorErr != nil {
			return state.ProductFactorFrame{}, factorErr
		}
		oldValue, present := rootAssignmentFrameValue(c.current, input.Current, mustRootAssignmentTarget(c.program.plan))
		if !present {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment path old value is absent")
		}
		pathResult, err = c.program.ApplyPathMutation(RootAssignmentPathFactorInput{
			Factors: factors, Authority: c.pathWriteAuthority, OldValue: oldValue,
			Composed: source.composed, Dynamic: source.dynamic, HasDynamic: source.hasDynamic,
			FormalStableRoots: input.FormalStableRoots,
		})
		if err != nil {
			return state.ProductFactorFrame{}, err
		}
		for _, factor := range pathResult.Factors.LaneFactors() {
			if _, selected := ordinary[factor.Lane()]; selected {
				ordinary[factor.Lane()] = factor
			}
		}
		for _, factor := range pathResult.Factors.CoordinateFactors() {
			if _, selected := coordinates[factor.Family()]; selected {
				projected, projectErr := rootAssignmentProjectCoordinateFactor(domain, c.outputs, factor)
				if projectErr != nil {
					return state.ProductFactorFrame{}, projectErr
				}
				coordinates[factor.Family()] = projected
			}
		}
	}
	if source.hasDynamic {
		for _, lane := range domain.RootAssignmentDynamicSourceLanes() {
			current, present := ordinary[lane]
			if !present {
				return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment dynamic output %q is absent", lane.ID())
			}
			ordinary[lane], err = c.program.ApplyDynamicSource(source.dynamic, current)
			if err != nil {
				return state.ProductFactorFrame{}, err
			}
		}
	}
	if source.productive {
		for _, equality := range pathResult.Equalities {
			for _, lane := range domain.PathEqualityQuotientLanes() {
				current, present := ordinary[lane]
				if !present {
					return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment equality output %q is absent", lane.ID())
				}
				ordinary[lane], err = c.program.ApplyEqualityFactor(equality, current)
				if err != nil {
					return state.ProductFactorFrame{}, err
				}
			}
		}
	}
	transaction, err := domain.BeginProductFactorFrameTransaction(c.outputs, input.OutputBase)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	for _, lane := range c.outputs.OrdinaryLanes() {
		if err := transaction.WriteOrdinary(lane, ordinary[lane]); err != nil {
			return state.ProductFactorFrame{}, err
		}
	}
	for _, factor := range input.OutputBase.CoordinateFactors() {
		if err := transaction.WriteCoordinate(factor.Family(), coordinates[factor.Family()]); err != nil {
			return state.ProductFactorFrame{}, err
		}
	}
	return transaction.Finish()
}

func rootAssignmentProjectCoordinateFactor(
	domain state.ProductDomain,
	selection state.ProductFactorSelection,
	factor state.CoordinateFamilyFactor,
) (state.CoordinateFamilyFactor, error) {
	slots, err := selection.CoordinateFactors().FamilySlots(factor.Family())
	if err != nil {
		return state.CoordinateFamilyFactor{}, err
	}
	shape, err := domain.SealCoordinateFamilyShape(factor.Skeleton(), slots)
	if err != nil {
		return state.CoordinateFamilyFactor{}, err
	}
	selected := make([]state.CoordinateScalarFactor, 0, len(slots))
	for _, scalar := range factor.Scalars() {
		for _, slot := range slots {
			equal, equalErr := domain.CoordinateSlotEqual(scalar.Slot(), slot)
			if equalErr != nil {
				return state.CoordinateFamilyFactor{}, equalErr
			}
			if equal {
				selected = append(selected, scalar)
				break
			}
		}
	}
	return domain.SealCoordinateFamilyFactor(shape.Skeleton(), selected)
}

func (c RootAssignmentFactorComponent) pathMutationFactors(frame state.ProductFactorFrame) (state.PathSubtreeMutationFactors, error) {
	domain := c.program.plan.authority.domain
	topology, err := domain.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		return state.PathSubtreeMutationFactors{}, err
	}
	lanes := make([]state.LaneFactor, len(topology.Lanes()))
	for index, lane := range topology.Lanes() {
		var present bool
		lanes[index], present = rootAssignmentFrameOrdinary(frame, lane)
		if !present {
			return state.PathSubtreeMutationFactors{}, fmt.Errorf("factapply: RootAssignment path lane %q is absent", lane.ID())
		}
	}
	coordinates := make([]state.CoordinateFamilyFactor, len(topology.Families()))
	for index, family := range topology.Families() {
		var present bool
		coordinates[index], present = rootAssignmentFrameCoordinate(frame, family)
		if !present {
			return state.PathSubtreeMutationFactors{}, fmt.Errorf("factapply: RootAssignment path family %q is absent", family.ID())
		}
	}
	return domain.SealPathSubtreeMutationFactors(lanes, coordinates)
}

func (c RootAssignmentFactorComponent) applyScalarComponent(input RootAssignmentFactorComponentInput) (state.ProductFactorFrame, error) {
	if len(input.Sources) != 0 {
		source, err := c.resolveSource(input)
		if err != nil {
			return state.ProductFactorFrame{}, err
		}
		if !source.productive {
			return input.OutputBase, nil
		}
	}
	domain := c.program.plan.authority.domain
	transaction, err := domain.BeginProductFactorFrameTransaction(c.outputs, input.OutputBase)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	if c.ordinary.ID() != "" {
		current, currentPresent := rootAssignmentFrameOrdinary(input.Current, c.ordinary)
		point, pointPresent := rootAssignmentFrameOrdinary(input.PointEntry, c.ordinary)
		if !currentPresent || !pointPresent {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: incomplete RootAssignment scalar lane frames")
		}
		output, applyErr := c.program.ApplyScalarFactor(point, current)
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
	current, currentPresent := rootAssignmentFrameCoordinate(input.Current, c.family)
	if !currentPresent {
		return state.ProductFactorFrame{}, fmt.Errorf("factapply: incomplete RootAssignment scalar coordinate frames")
	}
	nextSkeleton, nextScalars := current.Skeleton(), current.Scalars()
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
			point, pointPresent := rootAssignmentFrameCoordinate(input.PointEntry, sourceSlot.Family())
			if !pointPresent {
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
		nextScalars, factorErr = replaceRootAssignmentCoordinateFactor(domain, nextSkeleton, nextScalars, target)
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

func (c RootAssignmentFactorComponent) applyCompletionComponent(input RootAssignmentFactorComponentInput) (state.ProductFactorFrame, error) {
	source, err := c.resolveSource(input)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	if !source.productive {
		return input.OutputBase, nil
	}
	domain := c.program.plan.authority.domain
	predicates := make([]FreshEmptyPredicate, len(c.fresh))
	if len(c.fresh) != 0 {
		family, present := domain.RootAssignmentCoordinateFamily()
		if !present {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment fresh-empty owner is absent")
		}
		factor, present := rootAssignmentFrameCoordinate(input.Current, family)
		if !present {
			return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment fresh-empty skeleton is absent")
		}
		for index, query := range c.fresh {
			value, valuePresent := rootAssignmentFrameValue(c.current, input.Current, query.slot)
			if !valuePresent {
				return state.ProductFactorFrame{}, fmt.Errorf("factapply: RootAssignment fresh-empty value is absent")
			}
			fresh, freshErr := c.program.EvaluateFreshEmpty(factor.Skeleton(), value)
			if freshErr != nil {
				return state.ProductFactorFrame{}, freshErr
			}
			predicates[index] = FreshEmptyPredicate{Path: query.path.Clone(), Fresh: fresh}
		}
	}
	completion, err := c.program.PrepareCompletion(domain.Registry(), source.primary, predicates)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	transaction, err := domain.BeginProductFactorFrameTransaction(c.outputs, input.OutputBase)
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	for index, lane := range c.outputs.OrdinaryLanes() {
		current := input.OutputBase.OrdinaryFactors()[index]
		next, applyErr := c.program.ApplyCompletionFactor(completion, current)
		if applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
		if applyErr = transaction.WriteOrdinary(lane, next); applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
	}
	for _, current := range input.OutputBase.CoordinateFactors() {
		slot, present, slotErr := domain.RootAssignmentCompletionCoordinateSlot(completion, current.Family(), mustRootAssignmentKeys(c.program.plan))
		if slotErr != nil {
			return state.ProductFactorFrame{}, slotErr
		}
		if !present {
			continue
		}
		scalar, scalarPresent, scalarErr := rootAssignmentCoordinateFactorAt(domain, current.Skeleton(), current.Scalars(), slot)
		if scalarErr != nil || !scalarPresent {
			if scalarErr == nil {
				scalarErr = fmt.Errorf("factapply: RootAssignment completion coordinate is absent")
			}
			return state.ProductFactorFrame{}, scalarErr
		}
		skeleton, scalar, applyErr := c.program.ApplyCompletionCoordinate(completion, current.Skeleton(), scalar)
		if applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
		scalars, applyErr := replaceRootAssignmentCoordinateFactor(domain, skeleton, current.Scalars(), scalar)
		if applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
		next, applyErr := domain.SealCoordinateFamilyFactor(skeleton, scalars)
		if applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
		if applyErr = transaction.WriteCoordinate(current.Family(), next); applyErr != nil {
			return state.ProductFactorFrame{}, applyErr
		}
	}
	return transaction.Finish()
}

func rootAssignmentFrameOrdinary(frame state.ProductFactorFrame, lane state.ProductLane) (state.LaneFactor, bool) {
	for _, factor := range frame.OrdinaryFactors() {
		if factor.Lane() == lane {
			return factor, true
		}
	}
	return state.LaneFactor{}, false
}

func rootAssignmentFrameCoordinate(frame state.ProductFactorFrame, family state.CoordinateFamily) (state.CoordinateFamilyFactor, bool) {
	for _, factor := range frame.CoordinateFactors() {
		if factor.Family() == family {
			return factor, true
		}
	}
	return state.CoordinateFamilyFactor{}, false
}

func rootAssignmentFrameValue(selection state.ProductFactorSelection, frame state.ProductFactorFrame, slot statekey.Value) (product.Value, bool) {
	values := selection.ValueFactors()
	position := sort.Search(len(values), func(index int) bool { return values[index] >= slot })
	frameValues := frame.Values()
	if position >= len(values) || values[position] != slot || position >= len(frameValues) {
		return product.Value{}, false
	}
	return frameValues[position], true
}

func mustRootAssignmentTarget(plan ResolvedRootAssignmentPlan) statekey.Value {
	target, _ := plan.TargetValueSlot()
	return target
}

func mustRootAssignmentKeys(plan ResolvedRootAssignmentPlan) *keyspace.KeySpace {
	keys, _ := plan.PathKeySpace()
	return keys
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
	skeleton state.CoordinateFamilySkeleton,
	scalars []state.CoordinateScalarFactor,
	update state.CoordinateScalarFactor,
) ([]state.CoordinateScalarFactor, error) {
	support, err := domain.CoordinateScalarSupport(skeleton, update.Slot())
	if err != nil {
		return nil, err
	}
	out := append([]state.CoordinateScalarFactor(nil), scalars...)
	for index, scalar := range out {
		equal, err := domain.CoordinateSlotEqual(scalar.Slot(), update.Slot())
		if err != nil {
			return nil, err
		}
		if equal {
			if support == state.CoordinateScalarForbidden {
				return append(out[:index], out[index+1:]...), nil
			}
			out[index] = update
			return out, nil
		}
	}
	if support == state.CoordinateScalarForbidden {
		return out, nil
	}
	out = append(out, update)
	sort.Slice(out, func(i, j int) bool {
		less, _ := domain.CoordinateSlotLess(out[i].Slot(), out[j].Slot())
		return less
	})
	return out, nil
}

// RootAssignmentFactorComponents seals the four exact N4 hyperedges. Source
// values are semantic operands of ApplyComponent; ProductFactorSelection owns
// only the registered factor evidence needed to interpret those values.
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
	seal := func(lanes []state.ProductLane, coordinates []state.CoordinateSlot, values []statekey.Value, valuesTop bool, skeletons ...state.CoordinateFamily) (state.ProductFactorSelection, error) {
		coordinateInventory, sealErr := domain.SealCoordinateFactorInventory(keys, coordinates)
		if sealErr != nil {
			return state.ProductFactorSelection{}, sealErr
		}
		coordinateInventory, sealErr = domain.CloseCoordinateFactorInventory(keys, coordinateInventory)
		if sealErr != nil {
			return state.ProductFactorSelection{}, sealErr
		}
		return domain.SealProductFactorSelection(canonicalRootAssignmentLanes(lanes), coordinateInventory, canonicalRootAssignmentValues(values), valuesTop, skeletons...)
	}

	target, ok := p.plan.TargetValueSlot()
	if !ok {
		return nil, fmt.Errorf("factapply: RootAssignment component has no target Values factor")
	}
	_ = inventory.SourceValues // source terms are correlated semantic operands.

	var pathCurrent, pathPoint, pathWrites []state.CoordinateSlot
	if family, present := domain.PathValueFamily(); present {
		familySlots, familyErr := inventory.Coordinates.FamilySlots(family)
		if familyErr != nil {
			return nil, familyErr
		}
		schedule, scheduleErr := p.plan.PathDependenciesWithFormalRoots(domain, familySlots, inventory.FormalStableRoots)
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
		pathCurrent = append(pathCurrent, pathWrites...)
	}

	var dynamicInputs, dynamicOutputs []state.ProductLane
	if dependencies, present, dependencyErr := p.plan.DynamicSourceDependencies(); dependencyErr != nil {
		return nil, dependencyErr
	} else if present {
		dynamicInputs = dependencies.InputLanes()
		dynamicOutputs = domain.RootAssignmentDynamicSourceLanes()
	}
	equality := domain.PathEqualityQuotientLanes()
	var dynamicCoordinates []state.CoordinateSlot
	var auxiliaryValues []statekey.Value
	if dynamic, present := p.plan.DynamicSourcePlan(); present {
		if query, queryPresent, queryErr := dynamic.TableNonEmptyQuery(); queryErr != nil {
			return nil, queryErr
		} else if queryPresent {
			for _, get := range []func() (state.CoordinateSlot, bool){query.LenFloorSlot, query.RefinementSlot, query.StaticMemberSlot} {
				if slot, slotPresent := get(); slotPresent {
					dynamicCoordinates = append(dynamicCoordinates, slot)
				}
			}
			if slot, slotPresent := query.RootValueSlot(); slotPresent {
				auxiliaryValues = append(auxiliaryValues, slot)
			}
		}
	}

	queries, err := p.plan.FactorCompletionFreshEmptyPaths()
	if err != nil {
		return nil, err
	}
	fresh := make([]rootAssignmentFactorFreshQuery, len(queries))
	for index, query := range queries {
		if query.Symbol == 0 || len(query.Segments) != 0 {
			return nil, fmt.Errorf("factapply: RootAssignment fresh-empty query is not a root")
		}
		fresh[index] = rootAssignmentFactorFreshQuery{path: query.Clone(), slot: statekey.SymbolValue(query.Symbol)}
		auxiliaryValues = append(auxiliaryValues, fresh[index].slot)
	}
	auxiliaryValues = canonicalRootAssignmentValues(auxiliaryValues)

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
	appendComponent := func(component RootAssignmentFactorComponent) error {
		if _, present := p.plan.SourcePresenceProof(); present {
			component.pathReadAuthority, err = state.SealCoordinatePathEvidenceAuthority(
				domain, keys, nil, nil, component.pointEntry.CoordinateFactors(), empty,
				false, false, func(statekey.Value) bool { return false },
			)
			if err != nil {
				return err
			}
		}
		if component.kind == RootAssignmentFactorComponentPath && p.ownsStage(RootAssignmentFactorStagePathMutation) {
			component.pathWriteAuthority, err = state.SealCoordinatePathEvidenceAuthority(
				domain, keys, nil, nil, component.current.CoordinateFactors(), component.outputs.CoordinateFactors(),
				false, true, func(statekey.Value) bool { return false },
			)
			if err != nil {
				return err
			}
		}
		components = append(components, component)
		return nil
	}
	sourceCurrent, err := seal(dynamicInputs, dynamicCoordinates, auxiliaryValues, true)
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
	if err := appendComponent(RootAssignmentFactorComponent{
		program: p, kind: RootAssignmentFactorComponentSource, stages: sourceStages,
		current: sourceCurrent, pointEntry: sourcePoint, outputs: sourceOutputs,
	}); err != nil {
		return nil, err
	}

	pathLanes := append(append([]state.ProductLane(nil), dynamicInputs...), dynamicOutputs...)
	pathLanes = append(pathLanes, equality...)
	pathOutputLanes := append(append([]state.ProductLane(nil), dynamicOutputs...), equality...)
	if len(pathWrites) != 0 || len(pathOutputLanes) != 0 {
		pathCoordinates := append(append([]state.CoordinateSlot(nil), pathCurrent...), dynamicCoordinates...)
		pathValues := append(append([]statekey.Value(nil), auxiliaryValues...), target)
		var pathSkeletons []state.CoordinateFamily
		if topology, topologyErr := domain.SealPathSubtreeMutationFactorTopology(); topologyErr != nil {
			return nil, topologyErr
		} else {
			for _, family := range topology.Families() {
				selected := false
				for _, slot := range pathCoordinates {
					if slot.Family() == family {
						selected = true
						break
					}
				}
				if !selected {
					pathSkeletons = append(pathSkeletons, family)
				}
			}
		}
		pathInput, pathErr := seal(pathLanes, pathCoordinates, pathValues, false, pathSkeletons...)
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
		if pathErr = appendComponent(RootAssignmentFactorComponent{
			program: p, kind: RootAssignmentFactorComponentPath, stages: stages,
			current: pathInput, pointEntry: pointInput, outputs: pathOutput,
		}); pathErr != nil {
			return nil, pathErr
		}
	}

	if transaction, present := p.plan.ScalarFactorTransaction(); present {
		for _, lane := range domain.RootAssignmentScalarLanes() {
			currentLanes := append(append([]state.ProductLane(nil), dynamicInputs...), lane)
			current, scalarErr := seal(currentLanes, dynamicCoordinates, auxiliaryValues, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			point, scalarErr := seal([]state.ProductLane{lane}, pathPoint, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			outputs, scalarErr := seal([]state.ProductLane{lane}, nil, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			if scalarErr = appendComponent(RootAssignmentFactorComponent{
				program: p, kind: RootAssignmentFactorComponentScalar,
				stages:  []RootAssignmentFactorStage{RootAssignmentFactorStageScalarTransfer},
				current: current, pointEntry: point, outputs: outputs, ordinary: lane,
			}); scalarErr != nil {
				return nil, scalarErr
			}
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
			currentSlots = append(currentSlots, dynamicCoordinates...)
			current, scalarErr := seal(dynamicInputs, currentSlots, auxiliaryValues, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			pointSlots = append(pointSlots, pathPoint...)
			point, scalarErr := seal(nil, pointSlots, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			outputs, scalarErr := seal(nil, currentSlots, nil, false)
			if scalarErr != nil {
				return nil, scalarErr
			}
			if scalarErr = appendComponent(RootAssignmentFactorComponent{
				program: p, kind: RootAssignmentFactorComponentScalar,
				stages:  []RootAssignmentFactorStage{RootAssignmentFactorStageScalarTransfer},
				current: current, pointEntry: point, outputs: outputs, family: family, demands: demands,
			}); scalarErr != nil {
				return nil, scalarErr
			}
		}
	}

	completionLanes := domain.RootAssignmentCompletionLanes()
	var completionSlots []state.CoordinateSlot
	if targetPath, present := p.plan.TargetPathKey(); present {
		completionSlots, err = domain.RootAssignmentCompletionCoordinateTargetSlots(keys, targetPath)
		if err != nil {
			return nil, err
		}
	}
	if len(completionLanes) != 0 || len(completionSlots) != 0 {
		currentLanes := append(append([]state.ProductLane(nil), dynamicInputs...), completionLanes...)
		currentCoordinates := append(append([]state.CoordinateSlot(nil), dynamicCoordinates...), completionSlots...)
		var skeletons []state.CoordinateFamily
		if len(fresh) != 0 {
			family, present := domain.RootAssignmentCoordinateFamily()
			if !present {
				return nil, fmt.Errorf("factapply: RootAssignment fresh-empty family is absent")
			}
			skeletons = append(skeletons, family)
		}
		current, completionErr := seal(currentLanes, currentCoordinates, auxiliaryValues, false, skeletons...)
		if completionErr != nil {
			return nil, completionErr
		}
		point, completionErr := seal(nil, pathPoint, nil, false)
		if completionErr != nil {
			return nil, completionErr
		}
		outputs, completionErr := seal(completionLanes, completionSlots, nil, false)
		if completionErr != nil {
			return nil, completionErr
		}
		if completionErr = appendComponent(RootAssignmentFactorComponent{
			program: p, kind: RootAssignmentFactorComponentCompletion,
			stages:  []RootAssignmentFactorStage{RootAssignmentFactorStageFreshEmpty, RootAssignmentFactorStageCompletion},
			current: current, pointEntry: point, outputs: outputs, fresh: fresh,
		}); completionErr != nil {
			return nil, completionErr
		}
	}
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
