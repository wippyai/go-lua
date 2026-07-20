package transformer

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalExternalCallWire is one freeze-time binding from the canonical
// CallOutcomeInput layout to exact formal fibers. Execution follows these
// ordinals directly; it never scans product or point inventories.
type formalExternalCallWire struct {
	point            cfg.Point
	projection       state.CoordinateFormalPublicationProjection
	values           []formalFiberGroupMember
	valuesTop        formalFiberGroupMember
	lanes            []formalExternalCallInputLane
	typestateQueries []formalExternalCallTypestateResourceQuery
	diagnostics      formalFiberDescriptor
	ordinals         []formalFiberOrdinal
	readsDiag        bool
	readsReach       bool
}

type formalExternalCallTypestateResourceQuery struct {
	query     state.TypestateResourceQuery
	plan      state.TypestateResourceFormalQueryPlan
	typestate formalFiberGroupDescriptor
	path      formalExternalCallInputLane
	ordinals  []formalFiberOrdinal
}

type formalExternalCallInputLane struct {
	group      formalFiberGroupDescriptor
	coordinate state.CoordinateFormalBoundaryFactorPlan
	families   []formalExternalCallInputFamily
}

type formalExternalCallInputFamily struct {
	family    formalCoordinateFamilyFiberGroup
	positions []int
}

type formalExternalCallSiteKey struct {
	variable relationVar
	point    cfg.Point
}

type formalExternalCallValueOutput struct {
	root   FormalSlot
	member formalFiberGroupMember
}

// formalSelectedFactorLane is the exact non-Values carrier slice owned by
// one footprint-certified operator. Ordinary lanes have one indivisible member.
// Coordinate lanes retain only family skeletons and scalar coordinates named
// by the operator footprint; every other tuple member remains structural carry.
type formalSelectedFactorLane struct {
	group    formalFiberGroupDescriptor
	ordinary bool
	families []formalSelectedCoordinateFamily
	ordinals []formalFiberOrdinal
}

type formalSelectedCoordinateFamily struct {
	family    formalCoordinateFamilyFiberGroup
	selected  bool
	slots     []state.CoordinateSlot
	positions []int
}

type formalSelectedFactorOutput struct {
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

// formalPreparedExternalCallSite is the one forest-owned specialization of a
// provider site. It is created before the product inventory seals so every
// capability-declared correlation coordinate is part of the fixed tuple, and
// is then referenced by the equation template without preparing the provider
// a second time.
type formalPreparedExternalCallSite struct {
	provider    callpayload.CallOutcomeSiteProgram
	capability  callpayload.CallOutcomeCapability
	correlation factapply.CallOutcomeCorrelationFactorProgram
}

func (i *formalFiberInventory) externalCallSite(variable relationVar, point cfg.Point) (formalPreparedExternalCallSite, bool) {
	if i == nil || variable == 0 || point == 0 {
		return formalPreparedExternalCallSite{}, false
	}
	value, ok := i.externalCalls[formalExternalCallSiteKey{variable: variable, point: point}]
	return value, ok
}

func (i *formalFiberInventory) externalCallCoordinateSlots(variable relationVar) []state.CoordinateSlot {
	if i == nil || variable == 0 {
		return nil
	}
	points := make([]cfg.Point, 0, len(i.externalCalls))
	for key := range i.externalCalls {
		if key.variable == variable {
			points = append(points, key.point)
		}
	}
	sort.Slice(points, func(left, right int) bool { return points[left] < points[right] })
	var out []state.CoordinateSlot
	for _, point := range points {
		site := i.externalCalls[formalExternalCallSiteKey{variable: variable, point: point}]
		out = append(out, site.correlation.CoordinateSlots()...)
	}
	return out
}

// freezeFormalExternalCallSites prepares providers in canonical lexical-point
// order before fibers seal. Repeated relation syntax for one point references
// the same site program; it can never multiply provider preparation.
func freezeFormalExternalCallSites(
	program *RelationProgram,
	inventory *formalFiberInventory,
	body *relationProgramBody,
	variable relationVar,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
) error {
	if program == nil || inventory == nil || body == nil || body.variable != variable || body.relation.code == nil {
		return fmt.Errorf("transformer: formal ExternalCall site inventory is unowned")
	}
	points := make(map[cfg.Point]struct{})
	for _, node := range body.relation.code.nodes {
		for _, step := range node.steps {
			if step.kind == boundaryStepExternalCall && step.point > 0 {
				points[step.point] = struct{}{}
			}
		}
	}
	ordered := make([]cfg.Point, 0, len(points))
	for point := range points {
		ordered = append(ordered, point)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	if len(ordered) == 0 {
		return nil
	}
	if body.plan == nil || body.graph == nil || formalKeys == nil || !formalKeys.Valid() ||
		body.pathSemantics == nil || !body.pathSemantics.Valid() || !body.productDomain.Valid() {
		return fmt.Errorf("transformer: formal ExternalCall site inventory is unowned")
	}
	formalAuthority, err := body.pathSemantics.ProjectFormal(body.productDomain, rekey, formalKeys)
	if err != nil {
		return err
	}
	for _, point := range ordered {
		site, present := body.plan.Facts().CallSiteView(point)
		if !present {
			return fmt.Errorf("transformer: formal ExternalCall point %d has no call-site payload", point)
		}
		node := transfer.NodeContext{
			Context: context.Background(), Registry: program.registry, Graph: body.graph,
			Node: body.graph.Node(point), Point: point,
		}
		provider, prepareErr := body.externalCalls.PrepareSite(node, site)
		if prepareErr != nil {
			return prepareErr
		}
		capability := provider.Capability()
		boundaryPaths, boundaryErr := formalAuthority.CallBoundaryPathBindings(body.plan.Facts(), site)
		if boundaryErr != nil {
			return boundaryErr
		}
		correlationRoots, correlationRootErr := formalExternalCallCorrelationRoots(
			body, rekey, formalKeys, point, capability.CorrelationShapes(),
		)
		if correlationRootErr != nil {
			return correlationRootErr
		}
		correlation, correlationErr := factapply.PrepareCallOutcomeCorrelationFactorProgramAtBoundary(
			formalAuthority, body.productDomain, point, boundaryPaths, correlationRoots, capability.CorrelationShapes(),
		)
		if correlationErr != nil {
			return correlationErr
		}
		key := formalExternalCallSiteKey{variable: variable, point: point}
		if _, duplicate := inventory.externalCalls[key]; duplicate {
			return fmt.Errorf("transformer: formal ExternalCall point %d was prepared twice", point)
		}
		inventory.externalCalls[key] = formalPreparedExternalCallSite{
			provider: provider, capability: capability, correlation: correlation,
		}
	}
	return nil
}

// formalExternalCallCorrelationRoots is the formal image of the canonical
// point-owned CallResult relation. The same concrete carrier is already part
// of the body's sealed structural rekey; this function only selects the slots
// named by the provider capability and transports them through that authority.
func formalExternalCallCorrelationRoots(
	body *relationProgramBody,
	rekey state.CoordinateFormalRootRekey,
	formalKeys *keyspace.KeySpace,
	point cfg.Point,
	shapes []callpayload.CallOutcomeCorrelationShape,
) (state.BoundaryRoots, error) {
	if body == nil || !body.productDomain.Valid() || body.keys == nil || !body.keys.Valid() ||
		formalKeys == nil || !formalKeys.Valid() || point == 0 {
		return nil, fmt.Errorf("transformer: formal ExternalCall correlation roots are unowned")
	}
	indices := make(map[int]struct{}, len(shapes)*2)
	for _, shape := range shapes {
		if shape.ReturnIndex >= 0 {
			indices[shape.ReturnIndex] = struct{}{}
		}
		switch shape.Kind {
		case callpayload.CallOutcomeReturnConditionSlot, callpayload.CallOutcomeReturnPresence:
			if shape.TargetIndex >= 0 {
				indices[shape.TargetIndex] = struct{}{}
			}
		}
	}
	ordered := make([]int, 0, len(indices))
	for index := range indices {
		ordered = append(ordered, index)
	}
	sort.Ints(ordered)
	roots := make(state.BoundaryRoots, 0, len(ordered))
	for _, index := range ordered {
		slot, concrete, err := frameCallResultCarrier(body.keys, body.body, point, uint32(index))
		if err != nil {
			return nil, err
		}
		formalPath, err := body.productDomain.RekeyStructuralKeyFormal(rekey, concrete)
		if err != nil || formalPath.Kind == keyspace.KindInvalid || formalKeys.FormatReadOnly(formalPath) == "" {
			return nil, fmt.Errorf("transformer: formal ExternalCall result %d has no structural root", index)
		}
		roots = append(roots, state.BoundaryRoot{Slot: slot, Path: formalPath})
	}
	return roots, nil
}

// formalExternalCallStep is the single provider+ordinary-transfer transaction
// for one external call. Provider input and product output use different root
// vocabularies but share the same sealed TransferAccess contract.
type formalExternalCallStep struct {
	program           *RelationProgram
	body              *relationProgramBody
	variable          relationVar
	point             cfg.Point
	site              factflow.CallSiteView
	provider          callpayload.CallOutcomeSiteProgram
	input             callpayload.ExternalCallInputProgram[statekey.Value]
	factor            factapply.ExternalCallFactorProgram[FormalSlot]
	normal            factapply.NormalReturnFactorCodec[FormalSlot]
	normalValues      []formalFiberGroupMember
	correlation       factapply.CallOutcomeCorrelationFactorProgram
	correlationLane   int
	hasCorrelations   bool
	wires             []formalExternalCallWire
	valuesTop         formalFiberGroupMember
	results           []formalFiberGroupMember
	valueOutputs      []formalExternalCallValueOutput
	valueOutputByRoot map[FormalSlot]int
	outputLanes       []formalSelectedFactorLane
	outputReads       []formalFiberOrdinal
	outcome           formalCallOutcomeFiber
	diagnostics       formalFiberDescriptor
	operands          callOutcomeOperandTerms
	access            []valueAccessTerm
	guardDemands      []formalQualifiedGuardDemand
	sealed            bool
}

type formalExternalCallProvider struct {
	input        callpayload.ExternalCallInputProgram[statekey.Value]
	wires        []formalExternalCallWire
	guardDemands []formalQualifiedGuardDemand
}

func (p *formalExternalCallStep) valid(operator formalRelationOperatorRef) bool {
	if p == nil || !p.sealed || p.program == nil || p.body == nil || p.variable == 0 ||
		p.body.variable != p.variable || p.body.relation.code != operator.code ||
		p.point == 0 || !p.input.Valid() || len(p.wires) != p.input.InputCount() ||
		p.hasCorrelations != (p.correlationLane >= 0) ||
		len(p.results) != p.factor.ResultCount() || !p.outcome.valid() ||
		len(p.valueOutputs) != len(p.valueOutputByRoot) ||
		len(p.outputLanes) != p.factor.LaneCount() {
		return false
	}
	for _, wire := range p.wires {
		if wire.point == 0 || !p.body.productDomain.OwnsCoordinateFormalPublicationProjection(wire.projection) {
			return false
		}
	}
	return true
}

// freezeFormalExternalCallStep closes the complete equation operand row after
// influences have been sealed. Published reads remain separate wires in their
// canonical (ReadPoint, source-cell) order.
func freezeFormalExternalCallStep(
	program *RelationProgram,
	variable relationVar,
	operator formalRelationOperatorRef,
	equation formalRelationEquation,
) (*formalExternalCallStep, error) {
	step, ok := formalRelationStepOperator(operator)
	if !ok || step.kind != boundaryStepExternalCall {
		return nil, nil
	}
	if program == nil || variable == 0 || int(variable) > len(program.bodies) ||
		operator.kind != formalRelationCellStep || equation.Cell.cell.Variable != variable {
		return nil, fmt.Errorf("transformer: formal ExternalCall freeze is unowned")
	}
	body := &program.bodies[variable-1]
	if body.plan == nil || body.graph == nil ||
		body.pathSemantics == nil || !body.pathSemantics.Valid() || !body.productDomain.Valid() {
		return nil, fmt.Errorf("transformer: formal ExternalCall has no canonical authority")
	}
	site, ok := body.plan.Facts().CallSiteView(step.point)
	if !ok {
		return nil, fmt.Errorf("transformer: formal ExternalCall point %d has no call-site payload", step.point)
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		return nil, err
	}
	inputPoints := make([]cfg.Point, 1, 1+len(operands.PublishedReads))
	inputPoints[0] = step.point
	for _, read := range operands.PublishedReads {
		inputPoints = append(inputPoints, read.ReadPoint)
	}
	prefix, err := relationBoundaryPrefixStep(operator.code, step)
	if err != nil {
		return nil, err
	}
	preparedSite, prepared := program.formalFibers.externalCallSite(variable, step.point)
	if !prepared {
		return nil, fmt.Errorf("transformer: formal ExternalCall point %d has no prepared site", step.point)
	}
	provider, capability := preparedSite.provider, preparedSite.capability
	contract, err := externalCallTransferAccess(body, prefix, inputPoints, len(inputPoints), 0, capability)
	if err != nil {
		return nil, err
	}
	factorProgram, err := factapply.PrepareExternalCallFactorProgram(
		body.productDomain, contract, step.point, capability.ResultIndices(),
		func(point, result uint32) (FormalSlot, bool) {
			return formalMiddleSlotForStateKey(program, body, statekey.CallResult(point, result))
		},
	)
	if err != nil {
		return nil, err
	}
	span, ok := program.formalFibers.span(variable)
	if !ok {
		return nil, fmt.Errorf("transformer: formal ExternalCall has no product span")
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("transformer: formal ExternalCall has no Values group")
	}
	valuesTop, ok := values.top()
	if !ok {
		return nil, fmt.Errorf("transformer: formal ExternalCall Values has no Top fiber")
	}
	groups := make(map[state.LaneID]formalFiberGroupDescriptor)
	for _, group := range span.groupDescriptors() {
		if group.kind != formalFiberGroupValues {
			groups[group.lane.ID()] = group
		}
	}
	var diagnostic formalFiberDescriptor
	for _, descriptor := range span.descriptors() {
		if descriptor.role == formalFiberDiagnostics {
			if diagnostic.role != formalFiberInvalid {
				return nil, fmt.Errorf("transformer: formal ExternalCall diagnostics fiber is ambiguous")
			}
			diagnostic = descriptor
		}
	}
	if diagnostic.role == formalFiberInvalid {
		return nil, fmt.Errorf("transformer: formal ExternalCall diagnostics fiber is absent")
	}
	formalAuthority, err := body.pathSemantics.ProjectFormal(body.productDomain, span.rekey, span.keys)
	if err != nil {
		return nil, err
	}
	boundaryPaths, err := body.pathSemantics.CallBoundaryPathBindings(body.plan.Facts(), site)
	if err != nil {
		return nil, err
	}
	heapRoots, err := body.productDomain.HeapObjectRootSlotsFromCoordinateInventory(operator.footprint.inventory)
	if err != nil {
		return nil, err
	}
	normal, err := factapply.PrepareNormalReturnFactorCodec(
		formalAuthority, body.productDomain, contract, step.point, boundaryPaths,
		operator.footprint.inventory, heapRoots,
		func(dependency statekey.ValueDependency) (FormalSlot, bool) {
			return formalLiveValueSlotForDependency(program, body, dependency)
		},
	)
	if err != nil {
		return nil, err
	}
	normalRoots := normal.ValueRoots()
	normalValues := make([]formalFiberGroupMember, len(normalRoots))
	for index, root := range normalRoots {
		member, present := values.slot(root)
		if !present {
			return nil, fmt.Errorf("transformer: formal ExternalCall normal-return root is outside Values")
		}
		normalValues[index] = member
	}
	providerInput, err := freezeFormalExternalCallProvider(
		program, body, variable, operator, prefix, inputPoints, contract,
		span, values, valuesTop, groups, diagnostic,
	)
	if err != nil {
		return nil, fmt.Errorf("transformer: formal ExternalCall provider: %w", err)
	}
	factorLanes := factorProgram.Lanes()
	resultBindings := factorProgram.ResultBindings()
	outputLanes := make([]formalSelectedFactorLane, len(factorLanes))
	var outputReads []formalFiberOrdinal
	valuesTopOrdinal, exact := valuesTop.address(values.descriptor)
	if !exact {
		return nil, fmt.Errorf("transformer: formal ExternalCall Values top is unaddressable")
	}
	outputReads = append(outputReads, valuesTopOrdinal)
	resultMembers := make([]formalFiberGroupMember, len(resultBindings))
	for index, binding := range resultBindings {
		member, present := values.slot(binding.Root)
		if !present {
			return nil, fmt.Errorf("transformer: formal ExternalCall result root is outside Values")
		}
		ordinal, addressable := member.address(values.descriptor)
		if !addressable {
			return nil, fmt.Errorf("transformer: formal ExternalCall result root is unaddressable")
		}
		resultMembers[index] = member
		outputReads = append(outputReads, ordinal)
	}
	valueOutputs := make([]formalExternalCallValueOutput, 0, len(normalRoots)+len(resultBindings))
	valueOutputByRoot := make(map[FormalSlot]int, cap(valueOutputs))
	appendValueOutput := func(root FormalSlot, member formalFiberGroupMember) {
		if _, duplicate := valueOutputByRoot[root]; duplicate {
			return
		}
		valueOutputByRoot[root] = len(valueOutputs)
		valueOutputs = append(valueOutputs, formalExternalCallValueOutput{root: root, member: member})
	}
	for index, root := range normalRoots {
		appendValueOutput(root, normalValues[index])
	}
	for index, binding := range resultBindings {
		appendValueOutput(binding.Root, resultMembers[index])
	}
	for _, member := range normalValues {
		ordinal, addressable := member.address(values.descriptor)
		if !addressable {
			return nil, fmt.Errorf("transformer: formal ExternalCall normal-return root is unaddressable")
		}
		outputReads = append(outputReads, ordinal)
	}
	for index, lane := range factorLanes {
		group, exact := groups[lane.ID()]
		if !exact || group.lane != lane {
			return nil, fmt.Errorf("transformer: formal ExternalCall output lane %q is absent", lane.ID())
		}
		outputLanes[index], err = freezeFormalSelectedFactorLane(
			body.productDomain, span, group, operator.footprint.inventory,
		)
		if err != nil {
			return nil, fmt.Errorf("transformer: formal ExternalCall output lane %q: %w", lane.ID(), err)
		}
		outputReads = append(outputReads, outputLanes[index].ordinals...)
	}
	normalLanes := normal.Lanes()
	if len(normalLanes) != len(factorLanes) {
		return nil, fmt.Errorf("transformer: formal ExternalCall normal-return lane width differs from outer transaction")
	}
	for index := range normalLanes {
		if normalLanes[index] != factorLanes[index] {
			return nil, fmt.Errorf("transformer: formal ExternalCall normal-return lane order differs at %d", index)
		}
	}
	correlationLane := -1
	hasCorrelations := len(capability.CorrelationShapes()) != 0
	if hasCorrelations {
		for index, lane := range factorLanes {
			if lane == preparedSite.correlation.Lane() {
				correlationLane = index
				break
			}
		}
	}
	if hasCorrelations && correlationLane < 0 {
		return nil, fmt.Errorf("transformer: formal ExternalCall correlation lane is outside output ownership")
	}
	sort.Slice(outputReads, func(i, j int) bool { return outputReads[i] < outputReads[j] })
	write := 0
	for _, ordinal := range outputReads {
		if write == 0 || outputReads[write-1] != ordinal {
			outputReads[write], write = ordinal, write+1
		}
	}
	outputReads = outputReads[:write]
	outcome, ok := span.callOutcomeFiber(step.point)
	if !ok {
		return nil, fmt.Errorf("transformer: formal ExternalCall point %d has no outcome fiber", step.point)
	}
	diagnosticOrdinal, exact := span.ordinal(diagnostic)
	if !exact {
		return nil, fmt.Errorf("transformer: formal ExternalCall diagnostics fiber is unaddressable")
	}
	outputReads = append(outputReads, diagnosticOrdinal, outcome.ordinal)
	sort.Slice(outputReads, func(i, j int) bool { return outputReads[i] < outputReads[j] })
	write = 0
	for _, ordinal := range outputReads {
		if write == 0 || outputReads[write-1] != ordinal {
			outputReads[write], write = ordinal, write+1
		}
	}
	outputReads = outputReads[:write]
	// Provider evaluation and transactional publication are distinct products.
	// A provider observes only its declared finite input fibers. Output reads
	// are correlated with the resulting outcome decision diagram afterwards;
	// including them here would re-run the provider once for every irrelevant
	// spelling of its mutable destination.
	plan := &formalExternalCallStep{
		program: program, body: body, variable: variable, point: step.point, site: site, provider: provider,
		input: providerInput.input, wires: providerInput.wires, factor: factorProgram, normal: normal, normalValues: normalValues,
		correlation: preparedSite.correlation, correlationLane: correlationLane, hasCorrelations: hasCorrelations,
		valuesTop: valuesTop, results: resultMembers, outputLanes: outputLanes, outputReads: outputReads,
		valueOutputs: valueOutputs, valueOutputByRoot: valueOutputByRoot,
		outcome: outcome, diagnostics: diagnostic, operands: prefix.operands.clone(), access: cloneValueAccessTerms(prefix.access),
		guardDemands: providerInput.guardDemands, sealed: true,
	}
	if !plan.valid(operator) {
		return nil, fmt.Errorf("transformer: formal ExternalCall transaction did not seal")
	}
	return plan, nil
}

func freezeFormalExternalCallProvider(
	program *RelationProgram,
	body *relationProgramBody,
	variable relationVar,
	operator formalRelationOperatorRef,
	step boundaryPrefixStep,
	inputPoints []cfg.Point,
	contract state.TransferAccess,
	span formalFiberDescriptorSpan,
	values formalValuesFiberGroup,
	valuesTop formalFiberGroupMember,
	groups map[state.LaneID]formalFiberGroupDescriptor,
	diagnostic formalFiberDescriptor,
) (formalExternalCallProvider, error) {
	input, err := callpayload.PrepareExternalCallInputProgram(
		body.productDomain, contract, inputPoints, 0,
		func(slot statekey.Value) (statekey.Value, bool) { return slot, slot != 0 },
	)
	if err != nil {
		return formalExternalCallProvider{}, err
	}
	wires := make([]formalExternalCallWire, input.InputCount())
	for wireIndex := range wires {
		layout, exact := input.Layout(wireIndex)
		if !exact {
			return formalExternalCallProvider{}, fmt.Errorf("input wire %d is absent", wireIndex)
		}
		environment := formalPublicationPointOutput
		if layout.Point() == step.point {
			environment = formalPublicationPointInput
		}
		projection, err := freezeFormalPointPublicationInverse(body, span, layout.Point(), environment)
		if err != nil {
			return formalExternalCallProvider{}, fmt.Errorf("input wire %d projection: %w", wireIndex, err)
		}
		wire := formalExternalCallWire{point: layout.Point(), valuesTop: valuesTop, projection: projection, readsDiag: layout.ReadsDiagnostics(), readsReach: layout.ReadsReachable()}
		for _, root := range layout.ValueRoots() {
			slot, exact := formalMiddleSlotForStateKey(program, body, root)
			if !exact {
				return formalExternalCallProvider{}, fmt.Errorf("input root %d has no formal slot", root)
			}
			member, exact := values.slot(slot)
			if !exact {
				return formalExternalCallProvider{}, fmt.Errorf("input root is outside Values")
			}
			wire.values = append(wire.values, member)
		}
		for _, lane := range layout.Lanes() {
			group, exact := groups[lane.ID()]
			if !exact || group.lane != lane {
				return formalExternalCallProvider{}, fmt.Errorf("input lane %q is absent", lane.ID())
			}
			inputLane := formalExternalCallInputLane{group: group}
			if group.kind == formalFiberGroupCoordinateLane {
				inputLane.coordinate, err = body.productDomain.SealCoordinateFormalBoundaryFactorPlan(projection, group.lane)
				if err != nil {
					return formalExternalCallProvider{}, fmt.Errorf("input lane %q projection plan: %w", lane.ID(), err)
				}
				for _, familyLayout := range inputLane.coordinate.FamilyLayouts() {
					var family formalCoordinateFamilyFiberGroup
					found := false
					for _, candidate := range group.coordinateFamilies {
						if coordinateFamilySame(candidate.family, familyLayout.Family()) {
							family, found = candidate, true
							break
						}
					}
					if !found {
						return formalExternalCallProvider{}, fmt.Errorf("input family is outside lane %q", lane.ID())
					}
					positions := make([]int, len(familyLayout.Slots()))
					for slotIndex, slot := range familyLayout.Slots() {
						position, exact := freezeFormalBranchCoordinatePosition(body.productDomain, span, family, slot)
						if !exact {
							return formalExternalCallProvider{}, fmt.Errorf("input coordinate is outside frozen family")
						}
						positions[slotIndex] = position
					}
					inputLane.families = append(inputLane.families, formalExternalCallInputFamily{family: family, positions: positions})
				}
			}
			wire.lanes = append(wire.lanes, inputLane)
		}
		for _, query := range layout.TypestateResourceQueries() {
			queryPlan, queryErr := body.productDomain.SealTypestateResourceFormalQueryPlan(query, projection)
			if queryErr != nil {
				return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query: %w", queryErr)
			}
			typestateGroup, exact := groups[queryPlan.TypestateLane().ID()]
			if !exact || typestateGroup.lane != queryPlan.TypestateLane() || typestateGroup.kind != formalFiberGroupOrdinaryLane {
				return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query lane is absent")
			}
			sourceLanes := query.SourceLanes()
			if len(sourceLanes) != 2 {
				return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query source topology is malformed")
			}
			pathGroup, exact := groups[sourceLanes[1].ID()]
			if !exact || pathGroup.lane != sourceLanes[1] || pathGroup.kind != formalFiberGroupCoordinateLane {
				return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query path lane is absent")
			}
			queryInput := formalExternalCallTypestateResourceQuery{
				query: query, plan: queryPlan, typestate: typestateGroup,
				path: formalExternalCallInputLane{group: pathGroup},
			}
			for _, familyLayout := range queryPlan.PathFamilyLayouts() {
				var family formalCoordinateFamilyFiberGroup
				found := false
				for _, candidate := range pathGroup.coordinateFamilies {
					if coordinateFamilySame(candidate.family, familyLayout.Family()) {
						family, found = candidate, true
						break
					}
				}
				if !found {
					return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query family is outside path lane")
				}
				positions := make([]int, len(familyLayout.Slots()))
				for slotIndex, slot := range familyLayout.Slots() {
					position, positionExact := freezeFormalBranchCoordinatePosition(body.productDomain, span, family, slot)
					if !positionExact {
						return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query coordinate is outside frozen family")
					}
					positions[slotIndex] = position
				}
				queryInput.path.families = append(queryInput.path.families, formalExternalCallInputFamily{family: family, positions: positions})
			}
			queryInput.ordinals = append(queryInput.ordinals, typestateGroup.members...)
			for _, family := range queryInput.path.families {
				queryInput.ordinals = append(queryInput.ordinals, family.family.skeleton)
				for _, position := range family.positions {
					queryInput.ordinals = append(queryInput.ordinals, family.family.scalars[position])
				}
			}
			sort.Slice(queryInput.ordinals, func(i, j int) bool { return queryInput.ordinals[i] < queryInput.ordinals[j] })
			wire.typestateQueries = append(wire.typestateQueries, queryInput)
		}
		wire.ordinals = append(wire.ordinals, valuesTop.ordinal)
		for _, member := range wire.values {
			wire.ordinals = append(wire.ordinals, member.ordinal)
		}
		for _, lane := range wire.lanes {
			if lane.group.kind == formalFiberGroupOrdinaryLane {
				wire.ordinals = append(wire.ordinals, lane.group.members...)
				continue
			}
			for _, family := range lane.families {
				wire.ordinals = append(wire.ordinals, family.family.skeleton)
				for _, position := range family.positions {
					wire.ordinals = append(wire.ordinals, family.family.scalars[position])
				}
			}
		}
		if wire.readsDiag {
			wire.diagnostics = diagnostic
			ordinal, exact := span.ordinal(diagnostic)
			if !exact {
				return formalExternalCallProvider{}, fmt.Errorf("diagnostics ordinal is absent")
			}
			wire.ordinals = append(wire.ordinals, ordinal)
		}
		sort.Slice(wire.ordinals, func(i, j int) bool { return wire.ordinals[i] < wire.ordinals[j] })
		write := 0
		for _, ordinal := range wire.ordinals {
			if write == 0 || wire.ordinals[write-1] != ordinal {
				wire.ordinals[write], write = ordinal, write+1
			}
		}
		wire.ordinals = wire.ordinals[:write]
		wires[wireIndex] = wire
	}
	var guardDemands []formalQualifiedGuardDemand
	step.operands.each(func(term ValueTerm) bool {
		guards, guardErr := reachableValueTermGuards(operator.code.terms, term)
		if guardErr != nil {
			err = guardErr
			return false
		}
		for _, guard := range guards {
			guardDemands = append(guardDemands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard})
		}
		return true
	})
	if err != nil {
		return formalExternalCallProvider{}, err
	}
	return formalExternalCallProvider{input: input, wires: wires, guardDemands: guardDemands}, nil
}

func freezeFormalSelectedFactorLane(
	domain state.ProductDomain,
	span formalFiberDescriptorSpan,
	group formalFiberGroupDescriptor,
	footprint state.CoordinateFactorInventory,
) (formalSelectedFactorLane, error) {
	if !domain.Valid() || !group.valid() || group.variable != span.variable ||
		!footprint.ValidFor(domain, span.keys) || group.kind == formalFiberGroupValues {
		return formalSelectedFactorLane{}, errFormalComponentForeignOwner
	}
	plan := formalSelectedFactorLane{group: group}
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		plan.ordinary = true
		plan.ordinals = append(plan.ordinals, group.members...)
	case formalFiberGroupCoordinateLane:
		consumed := 0
		for _, family := range (formalCoordinateLaneFiberGroup{descriptor: group}).families() {
			slots, err := footprint.FamilySlots(family.family)
			if err != nil {
				return formalSelectedFactorLane{}, err
			}
			entry := formalSelectedCoordinateFamily{family: family, selected: len(slots) != 0, slots: slots}
			if entry.selected {
				entry.positions = make([]int, len(slots))
				plan.ordinals = append(plan.ordinals, family.skeleton)
				for index, slot := range slots {
					position, exact := freezeFormalBranchCoordinatePosition(domain, span, family, slot)
					if !exact || position < 0 || position >= len(family.scalars) {
						return formalSelectedFactorLane{}, fmt.Errorf("coordinate footprint slot is outside the frozen family")
					}
					entry.positions[index] = position
					plan.ordinals = append(plan.ordinals, family.scalars[position])
				}
				consumed += len(slots)
			}
			plan.families = append(plan.families, entry)
		}
		laneSlots := 0
		for _, slot := range footprint.Slots() {
			if slot.Family().Lane() == group.lane {
				laneSlots++
			}
		}
		if consumed != laneSlots {
			return formalSelectedFactorLane{}, fmt.Errorf("coordinate footprint is outside the lane family inventory")
		}
	default:
		return formalSelectedFactorLane{}, errFormalComponentMalformed
	}
	sort.Slice(plan.ordinals, func(i, j int) bool { return plan.ordinals[i] < plan.ordinals[j] })
	for index := 1; index < len(plan.ordinals); index++ {
		if plan.ordinals[index-1] == plan.ordinals[index] {
			return formalSelectedFactorLane{}, fmt.Errorf("coordinate footprint repeats a physical fiber")
		}
	}
	return plan, nil
}

func (a *formalTupleAlgebra) observeFormalTypestateResourceQuery(
	view formalSparseLeafView,
	query formalExternalCallTypestateResourceQuery,
) (state.TypestateResourceObservation, error) {
	typestateFactor, err := view.laneFactor(query.typestate)
	if err != nil {
		return state.TypestateResourceObservation{}, err
	}
	families := make([]state.CoordinateFormalBoundaryFamilyOperands, len(query.path.families))
	for familyIndex, familyPlan := range query.path.families {
		skeletonLeaf, present := view.leaf(familyPlan.family.skeleton)
		if !present {
			return state.TypestateResourceObservation{}, errFormalComponentMalformed
		}
		if skeletonLeaf == 0 {
			families[familyIndex].Skeleton, err = view.authority.product.CoordinateSkeletonBottom(familyPlan.family.family, view.span.keys)
		} else {
			terminal, terminalErr := view.authority.terminal(skeletonLeaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateSkeleton ||
				!coordinateFamilySame(terminal.skeleton.Family(), familyPlan.family.family) {
				return state.TypestateResourceObservation{}, errFormalComponentMalformed
			}
			families[familyIndex].Skeleton = terminal.skeleton
		}
		if err != nil {
			return state.TypestateResourceObservation{}, err
		}
		families[familyIndex].Scalars = make([]state.CoordinateScalarFactor, len(familyPlan.positions))
		for scalarIndex, position := range familyPlan.positions {
			if position < 0 || position >= len(familyPlan.family.scalars) {
				return state.TypestateResourceObservation{}, errFormalComponentMalformed
			}
			leaf, scalarPresent := view.leaf(familyPlan.family.scalars[position])
			if !scalarPresent {
				return state.TypestateResourceObservation{}, errFormalComponentMalformed
			}
			if leaf == 0 {
				continue
			}
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
				return state.TypestateResourceObservation{}, errFormalComponentMalformed
			}
			families[familyIndex].Scalars[scalarIndex] = terminal.scalar
		}
	}
	return view.authority.product.ObserveFormalTypestateResourceQuery(query.plan, typestateFactor, families)
}

func (a *formalTupleAlgebra) compileFormalTypestateResourceQuery(
	tuple formalRelationTuple,
	query formalExternalCallTypestateResourceQuery,
) (decisionRef, error) {
	regions, err := a.partitionSparseLeafViewsUnderCare(
		[]formalSparseTupleProjection{{tuple: tuple, ordinals: query.ordinals}}, nil,
	)
	if err != nil {
		return decisionFalse, err
	}
	_, _, authority, ok := a.span(tuple.variable)
	if !ok {
		return decisionFalse, errFormalComponentForeignOwner
	}
	root := decisionFalse
	for _, region := range regions {
		if len(region.views) != 1 || region.guard == decisionFalse {
			return decisionFalse, errDecisionMalformed
		}
		observation, observeErr := a.observeFormalTypestateResourceQuery(region.views[0], query)
		if observeErr != nil {
			return decisionFalse, observeErr
		}
		leaf, internErr := authority.internTypestateResourceObservation(observation)
		if internErr != nil {
			return decisionFalse, internErr
		}
		root, err = a.decisions.condition(a.ctx, region.guard, a.decisions.terminal(leaf), root)
		if err != nil {
			return decisionFalse, err
		}
	}
	return root, nil
}

// bindFormalExternalCallInput materializes exactly the provider-declared
// roots and factors from one correlated leaf region. The views are already
// under live Care; ReadsReachable therefore observes true without introducing
// a second reachability component.
func (a *formalTupleAlgebra) bindFormalExternalCallInput(
	plan *formalExternalCallStep,
	views []formalSparseLeafView,
) (callpayload.ExternalCallInputFrame[statekey.Value], error) {
	if a == nil || plan == nil || !plan.sealed || len(views) != len(plan.wires) {
		return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
	}
	operands := make([]callpayload.ExternalCallInputWireOperands, len(views))
	for index, view := range views {
		wire := plan.wires[index]
		if view.variable != plan.variable || view.authority == nil {
			return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentForeignOwner
		}
		topOrdinal, exact := wire.valuesTop.address(wire.valuesTop.group)
		if !exact {
			return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
		}
		topLeaf, present := view.leaf(topOrdinal)
		if !present || topLeaf > 1 {
			return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
		}
		input := callpayload.ExternalCallInputWireOperands{
			ValuesTop:                     topLeaf == 1,
			Factors:                       make([]state.LaneFactor, len(wire.lanes)),
			TypestateResourceObservations: make([]state.TypestateResourceObservation, len(wire.typestateQueries)),
			Reachable:                     wire.readsReach,
		}
		if len(view.derived) != len(wire.typestateQueries) {
			return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
		}
		for queryIndex, leaf := range view.derived {
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentTypestateResourceObservation ||
				!terminal.typestateResourceObservation.ValidFor(wire.typestateQueries[queryIndex].query) {
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
			}
			input.TypestateResourceObservations[queryIndex] = terminal.typestateResourceObservation
		}
		if !input.ValuesTop {
			input.Values = make([]product.Value, len(wire.values))
			for valueIndex, member := range wire.values {
				value, valueExact := view.value(member, wire.valuesTop)
				if !valueExact {
					return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
				}
				input.Values[valueIndex] = value
			}
		}
		for laneIndex, lane := range wire.lanes {
			var factor state.LaneFactor
			var factorErr error
			switch lane.group.kind {
			case formalFiberGroupOrdinaryLane:
				factor, factorErr = view.laneFactor(lane.group)
				if factorErr != nil {
					break
				}
				factor, factorErr = plan.body.productDomain.RekeyOrdinaryLaneFactorFormalPublication(wire.projection, factor)
			case formalFiberGroupCoordinateLane:
				families := make([]state.CoordinateFormalBoundaryFamilyOperands, len(lane.families))
				for familyIndex, familyPlan := range lane.families {
					skeletonLeaf, present := view.leaf(familyPlan.family.skeleton)
					if !present {
						factorErr = errFormalComponentMalformed
						break
					}
					if skeletonLeaf == 0 {
						families[familyIndex].Skeleton, factorErr = view.authority.product.CoordinateSkeletonBottom(familyPlan.family.family, view.span.keys)
					} else {
						terminal, terminalErr := view.authority.terminal(skeletonLeaf)
						if terminalErr != nil || terminal.kind != formalComponentCoordinateSkeleton ||
							!coordinateFamilySame(terminal.skeleton.Family(), familyPlan.family.family) {
							factorErr = errFormalComponentMalformed
							break
						}
						families[familyIndex].Skeleton = terminal.skeleton
					}
					families[familyIndex].Scalars = make([]state.CoordinateScalarFactor, len(familyPlan.positions))
					for scalarIndex, position := range familyPlan.positions {
						if position < 0 || position >= len(familyPlan.family.scalars) {
							factorErr = errFormalComponentMalformed
							break
						}
						leaf, scalarPresent := view.leaf(familyPlan.family.scalars[position])
						if !scalarPresent {
							factorErr = errFormalComponentMalformed
							break
						}
						if leaf == 0 {
							continue
						}
						terminal, terminalErr := view.authority.terminal(leaf)
						if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
							factorErr = errFormalComponentMalformed
							break
						}
						families[familyIndex].Scalars[scalarIndex] = terminal.scalar
					}
					if factorErr != nil {
						break
					}
				}
				if factorErr == nil {
					factor, factorErr = plan.body.productDomain.ApplyCoordinateFormalBoundaryFactorPlan(lane.coordinate, families)
				}
			default:
				factorErr = errFormalComponentMalformed
			}
			if factorErr != nil {
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, fmt.Errorf("transformer: materialize ExternalCall input lane %q: %w", lane.group.lane.ID(), factorErr)
			}
			input.Factors[laneIndex] = factor
		}
		if wire.readsDiag {
			ordinal, addressable := view.span.ordinal(wire.diagnostics)
			leaf, leafPresent := view.leaf(ordinal)
			if !addressable || !leafPresent {
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
			}
			leaf, leafErr := a.componentLeaf(view.authority, wire.diagnostics, leaf)
			if leafErr != nil {
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, leafErr
			}
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentDiagnostics {
				if terminalErr != nil {
					return callpayload.ExternalCallInputFrame[statekey.Value]{}, terminalErr
				}
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
			}
			input.Diagnostics = terminal.diagnostics.Clone()
		}
		operands[index] = input
	}
	return plan.input.BindFrame(operands)
}

// evaluateFormalExternalCallProvider is the sole formal provider invocation.
// It consumes a sealed factor frame and returns normalized scratch outcome
// syntax; publication happens only after the complete factor transaction.
func (a *formalTupleAlgebra) evaluateFormalExternalCallProvider(
	plan *formalExternalCallStep,
	views []formalSparseLeafView,
) (callpayload.CallOutcome, error) {
	frame, err := a.bindFormalExternalCallInput(plan, views)
	if err != nil {
		return callpayload.CallOutcome{}, err
	}
	if len(views) == 0 {
		return callpayload.CallOutcome{}, errFormalComponentMalformed
	}
	// The sealed wire projection above is the formal-to-concrete boundary for
	// the complete provider frame. Operand dynamic reads must therefore use the
	// same concrete address vocabulary as those factors; rebuilding a formal
	// query here would split one observation across two keyspaces.
	operands, err := evaluateExternalCallOperands(
		plan.body, plan.operands, plan.access, frame, concreteExternalCallDynamicQuery(plan.body),
	)
	if err != nil {
		return callpayload.CallOutcome{}, err
	}
	input, err := frame.BindCallOutcomeInput(operands)
	if err != nil {
		return callpayload.CallOutcome{}, err
	}
	node := transfer.NodeContext{
		Context: a.ctx, Registry: plan.program.registry, Graph: plan.body.graph,
		Node: plan.body.graph.Node(plan.point), Point: plan.point,
	}
	outcome, err := plan.provider.Evaluate(node, input)
	if err != nil {
		return callpayload.CallOutcome{}, err
	}
	return callpayload.NormalizeCallOutcome(plan.program.registry, outcome), nil
}

func (a *formalTupleAlgebra) materializeFormalSelectedLane(
	view formalSparseLeafView,
	plan formalSelectedFactorLane,
) (state.LaneFactor, error) {
	if a == nil || view.algebra != a || view.authority == nil || !plan.group.valid() ||
		plan.group.variable != view.variable {
		return state.LaneFactor{}, errFormalComponentForeignOwner
	}
	if plan.ordinary {
		return view.laneFactor(plan.group)
	}
	if plan.group.kind != formalFiberGroupCoordinateLane || len(plan.families) != len(plan.group.coordinateFamilies) {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	skeletons := make([]state.CoordinateFamilySkeleton, len(plan.families))
	scalars := make([][]state.CoordinateScalarFactor, len(plan.families))
	for familyIndex, family := range plan.families {
		var err error
		if !family.selected {
			skeletons[familyIndex], err = view.authority.product.CoordinateSkeletonBottom(family.family.family, view.span.keys)
			if err != nil {
				return state.LaneFactor{}, err
			}
			continue
		}
		leaf, present := view.leaf(family.family.skeleton)
		if !present {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		if leaf == 0 {
			skeletons[familyIndex], err = view.authority.product.CoordinateSkeletonBottom(family.family.family, view.span.keys)
		} else {
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateSkeleton ||
				!coordinateFamilySame(terminal.skeleton.Family(), family.family.family) {
				if terminalErr != nil {
					return state.LaneFactor{}, terminalErr
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			skeletons[familyIndex] = terminal.skeleton
		}
		if err != nil {
			return state.LaneFactor{}, err
		}
		for slotIndex, position := range family.positions {
			if position < 0 || position >= len(family.family.scalars) || slotIndex >= len(family.slots) {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			leaf, present := view.leaf(family.family.scalars[position])
			if !present {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			if leaf == 0 {
				continue
			}
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
				if terminalErr != nil {
					return state.LaneFactor{}, terminalErr
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			equal, equalErr := view.authority.product.CoordinateSlotEqual(terminal.scalar.Slot(), family.slots[slotIndex])
			if equalErr != nil || !equal {
				if equalErr != nil {
					return state.LaneFactor{}, equalErr
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			omitted, omitErr := view.authority.product.CoordinateScalarIsOmitted(skeletons[familyIndex], terminal.scalar)
			if omitErr != nil {
				return state.LaneFactor{}, omitErr
			}
			if !omitted {
				scalars[familyIndex] = append(scalars[familyIndex], terminal.scalar)
			}
		}
	}
	return view.authority.product.ComposeCoordinateFamilies(plan.group.lane, view.span.keys, skeletons, scalars)
}

func (a *formalTupleAlgebra) factorFormalSelectedLane(
	authority *formalComponentTerminalAuthority,
	span formalFiberDescriptorSpan,
	plan formalSelectedFactorLane,
	factor state.LaneFactor,
) ([]formalSelectedFactorOutput, error) {
	if a == nil || authority == nil || !plan.group.valid() || plan.group.variable != span.variable ||
		factor.Lane() != plan.group.lane {
		return nil, errFormalComponentForeignOwner
	}
	if plan.ordinary {
		leaves, err := a.factorFormalSparseLane(authority, span, plan.group, factor)
		if err != nil || len(leaves) != len(plan.ordinals) {
			if err != nil {
				return nil, err
			}
			return nil, errFormalComponentMalformed
		}
		out := make([]formalSelectedFactorOutput, len(leaves))
		for index := range leaves {
			out[index] = formalSelectedFactorOutput{ordinal: plan.ordinals[index], leaf: leaves[index]}
		}
		return out, nil
	}
	byOrdinal := make(map[formalFiberOrdinal]decisionLeaf, len(plan.ordinals))
	for _, ordinal := range plan.ordinals {
		byOrdinal[ordinal] = 0
	}
	for _, family := range plan.families {
		skeleton, scalars, err := authority.product.DecomposeCoordinateFamily(factor, family.family.family, span.keys)
		if err != nil {
			return nil, err
		}
		bottom, err := authority.product.CoordinateSkeletonBottom(family.family.family, span.keys)
		if err != nil {
			return nil, err
		}
		sameBottom, err := authority.product.CoordinateSkeletonRepresentationEqual(skeleton, bottom)
		if err != nil {
			return nil, err
		}
		if !family.selected {
			if !sameBottom || len(scalars) != 0 {
				return nil, fmt.Errorf("transformer: formal selected factor escaped its coordinate footprint")
			}
			continue
		}
		if !sameBottom {
			leaf, internErr := authority.internCoordinateSkeleton(skeleton)
			if internErr != nil {
				return nil, internErr
			}
			byOrdinal[family.family.skeleton] = leaf
		}
		scalarIndex := 0
		for slotIndex, slot := range family.slots {
			if scalarIndex >= len(scalars) {
				break
			}
			scalar := scalars[scalarIndex]
			equal, equalErr := authority.product.CoordinateSlotEqual(slot, scalar.Slot())
			if equalErr != nil {
				return nil, equalErr
			}
			if !equal {
				less, lessErr := authority.product.CoordinateSlotLess(scalar.Slot(), slot)
				if lessErr != nil {
					return nil, lessErr
				}
				if less {
					return nil, fmt.Errorf("transformer: formal selected factor escaped its coordinate footprint")
				}
				continue
			}
			if slotIndex >= len(family.positions) || family.positions[slotIndex] < 0 || family.positions[slotIndex] >= len(family.family.scalars) {
				return nil, errFormalComponentMalformed
			}
			leaf, internErr := authority.internCoordinateScalar(scalar)
			if internErr != nil {
				return nil, internErr
			}
			byOrdinal[family.family.scalars[family.positions[slotIndex]]] = leaf
			scalarIndex++
		}
		if scalarIndex != len(scalars) {
			return nil, fmt.Errorf("transformer: formal selected factor escaped its coordinate footprint")
		}
	}
	out := make([]formalSelectedFactorOutput, len(plan.ordinals))
	for index, ordinal := range plan.ordinals {
		out[index] = formalSelectedFactorOutput{ordinal: ordinal, leaf: byOrdinal[ordinal]}
	}
	return out, nil
}

// applyFormalExternalCallOuter executes the carrier-neutral outer fragment on
// one primary correlated leaf. Result mutations are retained as exact root
// updates and are not published here: the caller commits them together with
// every post-return phase or rolls the entire decision checkpoint back.
func (a *formalTupleAlgebra) applyFormalExternalCallOuter(
	plan *formalExternalCallStep,
	primary formalSparseLeafView,
	outcome callpayload.CallOutcome,
) (factapply.ExternalCallFactorFrame[FormalSlot], error) {
	if a == nil || plan == nil || !plan.sealed || primary.variable != plan.variable || primary.authority == nil {
		return factapply.ExternalCallFactorFrame[FormalSlot]{}, errFormalComponentForeignOwner
	}
	factors := make([]state.LaneFactor, len(plan.outputLanes))
	for index, lane := range plan.outputLanes {
		factor, err := a.materializeFormalSelectedLane(primary, lane)
		if err != nil {
			return factapply.ExternalCallFactorFrame[FormalSlot]{}, err
		}
		factors[index] = factor
	}
	result, err := plan.factor.Apply(a.ctx, nil, factapply.ExternalCallFactorFrame[FormalSlot]{Factors: factors}, outcome)
	if err != nil {
		return factapply.ExternalCallFactorFrame[FormalSlot]{}, err
	}
	if len(result.ResultMutations) != len(plan.results) {
		return factapply.ExternalCallFactorFrame[FormalSlot]{}, errFormalComponentMalformed
	}
	bindings := plan.factor.ResultBindings()
	for index, mutation := range result.ResultMutations {
		binding := bindings[index]
		if mutation.Root != binding.Root || !product.BelongsToRegistry(plan.program.registry, mutation.Value) {
			return factapply.ExternalCallFactorFrame[FormalSlot]{}, errFormalComponentMalformed
		}
	}
	return result, nil
}

// applyFormalExternalCall executes one closed external-call equation. Every
// provider alternative is evaluated under the shared correlated Care region;
// all product, diagnostic and outcome writes are accumulated from scratch and
// committed through one decision-arena checkpoint.
func (a *formalTupleAlgebra) applyFormalExternalCall(
	operator formalRelationOperatorRef,
	predecessor formalRelationTuple,
	published []formalRelationTuple,
) (formalRelationTuple, error) {
	plan := operator.externalCall
	if a == nil || plan == nil || !plan.valid(operator) || predecessor.variable != plan.variable ||
		len(published)+1 != len(plan.wires) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal ExternalCall execution is unowned")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		return predecessor, err
	}
	for _, tuple := range published {
		if tuple.variable != plan.variable {
			return formalRelationTuple{}, errFormalComponentForeignOwner
		}
	}
	span, directory, authority, ok := a.span(plan.variable)
	if !ok || predecessor.root.owner != directory {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	projections := make([]formalSparseTupleProjection, len(plan.wires))
	projections[0] = formalSparseTupleProjection{tuple: predecessor, ordinals: plan.wires[0].ordinals}
	for _, query := range plan.wires[0].typestateQueries {
		root, queryErr := a.compileFormalTypestateResourceQuery(predecessor, query)
		if queryErr != nil {
			return fail(queryErr)
		}
		projections[0].derived = append(projections[0].derived, root)
	}
	for index, tuple := range published {
		projections[index+1] = formalSparseTupleProjection{tuple: tuple, ordinals: plan.wires[index+1].ordinals}
		for _, query := range plan.wires[index+1].typestateQueries {
			root, queryErr := a.compileFormalTypestateResourceQuery(tuple, query)
			if queryErr != nil {
				return fail(queryErr)
			}
			projections[index+1].derived = append(projections[index+1].derived, root)
		}
	}
	inputRegions, err := a.partitionSparseLeafViewsUnderCare(projections, plan.guardDemands)
	if err != nil {
		return fail(err)
	}
	// First evaluate the provider over exactly its declared input product. The
	// resulting decision diagram is an immutable semantic relation from input
	// guards to normalized outcomes. Mutable destination fibers are deliberately
	// absent from this phase: they cannot change the provider result.
	providerCare, providerOutcome := decisionFalse, decisionFalse
	for _, region := range inputRegions {
		if len(region.views) != len(plan.wires) || region.guard == decisionFalse {
			return fail(errDecisionMalformed)
		}
		outcome, providerErr := a.evaluateFormalExternalCallProvider(plan, region.views)
		if providerErr != nil {
			return fail(providerErr)
		}
		leaf, providerErr := authority.internCallOutcomes(callpayload.NewCallOutcomeAlternativeSet(plan.program.registry, outcome))
		if providerErr != nil {
			return fail(providerErr)
		}
		providerOutcome, providerErr = a.decisions.condition(a.ctx, region.guard, a.decisions.terminal(leaf), providerOutcome)
		if providerErr != nil {
			return fail(providerErr)
		}
		providerCare, providerErr = a.decisions.apply(a.ctx, uint8(decisionOr), true, providerCare, region.guard, decisionLeafOr)
		if providerErr != nil {
			return fail(providerErr)
		}
	}
	if providerCare == decisionFalse {
		return formalRelationTuple{}, nil
	}
	// Then correlate the compact outcome relation with exactly the prior fibers
	// required by the atomic destination transaction. This preserves every
	// input/output correlation without multiplying provider execution by output
	// spelling.
	commitRoots := make([]decisionRef, 0, len(plan.outputReads)+1)
	for _, ordinal := range plan.outputReads {
		value, readErr := directory.valueAt(predecessor.root, ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		commitRoots = append(commitRoots, decisionRef(value))
	}
	commitRoots = append(commitRoots, providerOutcome)
	commitRegions, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, providerCare, commitRoots)
	if err != nil {
		return fail(err)
	}
	type affectedRoot struct {
		ordinal formalFiberOrdinal
		root    decisionRef
	}
	affected := make([]affectedRoot, len(plan.outputReads))
	for index, ordinal := range plan.outputReads {
		affected[index].ordinal = ordinal
	}
	publish := func(guard decisionRef, ordinal formalFiberOrdinal, leaf decisionLeaf) error {
		index := sort.Search(len(affected), func(i int) bool { return affected[i].ordinal >= ordinal })
		if index >= len(affected) || affected[index].ordinal != ordinal {
			return errFormalComponentMalformed
		}
		var publishErr error
		affected[index].root, publishErr = a.decisions.condition(
			a.ctx, guard, a.decisions.terminal(leaf), affected[index].root,
		)
		return publishErr
	}
	liveCare := decisionFalse
	valueRoots := plan.normal.ValueRoots()
	outputPositions, err := sealFormalOrdinalPositions(span.count, plan.outputReads)
	if err != nil {
		return fail(err)
	}
	for _, region := range commitRegions {
		if len(region.leaves) != len(plan.outputReads)+1 || region.care == decisionFalse {
			return fail(errDecisionMalformed)
		}
		primary := formalSparseLeafView{
			algebra: a, variable: plan.variable, span: span, authority: authority,
			body: plan.body, guard: region.care, ordinals: plan.outputReads,
			positions: outputPositions,
			leaves:    region.leaves[:len(plan.outputReads)],
		}
		outcomeTerminal, leafErr := authority.terminal(region.leaves[len(plan.outputReads)])
		if leafErr != nil || outcomeTerminal.kind != formalComponentCallOutcomes {
			if leafErr != nil {
				return fail(leafErr)
			}
			return fail(errFormalComponentMalformed)
		}
		outcomes := outcomeTerminal.callOutcomes.Outcomes()
		if len(outcomes) != 1 {
			return fail(errFormalComponentMalformed)
		}
		outcome := outcomes[0]
		outer, leafErr := a.applyFormalExternalCallOuter(plan, primary, outcome)
		if leafErr != nil {
			return fail(leafErr)
		}
		topOrdinal, exact := plan.valuesTop.address(plan.valuesTop.group)
		topLeaf, present := primary.leaf(topOrdinal)
		if !exact || !present || topLeaf > 1 {
			return fail(errFormalComponentMalformed)
		}
		values := state.ValueFactor[FormalSlot]{Top: topLeaf == 1}
		if !values.Top {
			for index, member := range plan.normalValues {
				value, valueExact := primary.value(member, plan.valuesTop)
				if !valueExact || index >= len(valueRoots) {
					return fail(errFormalComponentMalformed)
				}
				if !product.Equal(plan.program.registry, value, product.Bottom(plan.program.registry)) {
					if values.Values == nil {
						values.Values = make(map[FormalSlot]product.Value)
					}
					values.Values[valueRoots[index]] = value
				}
			}
			for _, mutation := range outer.ResultMutations {
				if product.Equal(plan.program.registry, mutation.Value, product.Bottom(plan.program.registry)) {
					delete(values.Values, mutation.Root)
				} else {
					if values.Values == nil {
						values.Values = make(map[FormalSlot]product.Value)
					}
					values.Values[mutation.Root] = mutation.Value
				}
			}
		}
		next, leafErr := plan.normal.Decode(a.ctx, nil, factapply.NormalReturnFactorFrame[FormalSlot]{
			Values: values, Factors: outer.Factors, Reachable: true,
		}, outcome.NormalReturnFacts)
		if leafErr != nil {
			return fail(leafErr)
		}
		if !next.Reachable {
			continue
		}
		if plan.hasCorrelations {
			if plan.correlationLane < 0 || plan.correlationLane >= len(next.Factors) {
				return fail(errFormalComponentMalformed)
			}
			next.Factors[plan.correlationLane], leafErr = plan.correlation.Apply(next.Factors[plan.correlationLane], outcome)
			if leafErr != nil {
				return fail(leafErr)
			}
		}
		liveCare, leafErr = a.decisions.apply(
			a.ctx, uint8(decisionOr), true, liveCare, region.care, decisionLeafOr,
		)
		if leafErr != nil {
			return fail(leafErr)
		}
		if next.Values.Top && len(next.Values.Values) != 0 {
			return fail(errFormalComponentMalformed)
		}
		if len(next.Values.Values) > len(plan.valueOutputs) {
			return fail(errFormalComponentForeignOwner)
		}
		for root := range next.Values.Values {
			if _, owned := plan.valueOutputByRoot[root]; !owned {
				return fail(errFormalComponentForeignOwner)
			}
		}
		nextTopLeaf := decisionLeaf(0)
		if next.Values.Top {
			nextTopLeaf = 1
		}
		topOrdinal, addressable := plan.valuesTop.address(plan.valuesTop.group)
		if !addressable {
			return fail(errFormalComponentMalformed)
		}
		if leafErr = publish(region.care, topOrdinal, nextTopLeaf); leafErr != nil {
			return fail(leafErr)
		}
		bottom := product.Bottom(plan.program.registry)
		for _, output := range plan.valueOutputs {
			leaf := decisionLeaf(0)
			if !next.Values.Top {
				value, present := next.Values.Values[output.root]
				if present && !product.Equal(plan.program.registry, value, bottom) {
					leaf, leafErr = authority.internGroundValue(value)
					if leafErr != nil {
						return fail(leafErr)
					}
				}
			}
			ordinal, addressable := output.member.address(plan.valuesTop.group)
			if !addressable {
				return fail(errFormalComponentMalformed)
			}
			if leafErr = publish(region.care, ordinal, leaf); leafErr != nil {
				return fail(leafErr)
			}
		}
		for index, lane := range plan.outputLanes {
			outputs, factorErr := a.factorFormalSelectedLane(authority, span, lane, next.Factors[index])
			if factorErr != nil {
				return fail(factorErr)
			}
			for _, output := range outputs {
				if factorErr = publish(region.care, output.ordinal, output.leaf); factorErr != nil {
					return fail(factorErr)
				}
			}
		}
		diagnosticOrdinal, diagnosticExact := span.ordinal(plan.diagnostics)
		diagnosticLeaf, diagnosticPresent := primary.leaf(diagnosticOrdinal)
		if !diagnosticExact || !diagnosticPresent {
			return fail(errFormalComponentMalformed)
		}
		diagnosticLeaf, leafErr = a.componentLeaf(authority, plan.diagnostics, diagnosticLeaf)
		if leafErr != nil {
			return fail(leafErr)
		}
		diagnosticTerminal, leafErr := authority.terminal(diagnosticLeaf)
		if leafErr != nil || diagnosticTerminal.kind != formalComponentDiagnostics {
			if leafErr != nil {
				return fail(leafErr)
			}
			return fail(errFormalComponentMalformed)
		}
		diagnosticLeaf, leafErr = authority.internDiagnostics(composeBoundaryDiagnostics(
			plan.program.registry, diagnosticTerminal.diagnostics, outer.Diagnostics, true,
		))
		if leafErr != nil {
			return fail(leafErr)
		}
		if leafErr = publish(region.care, diagnosticOrdinal, diagnosticLeaf); leafErr != nil {
			return fail(leafErr)
		}
		priorOutcome, priorPresent := primary.leaf(plan.outcome.ordinal)
		if !priorPresent {
			return fail(errFormalComponentMalformed)
		}
		priorOutcome, leafErr = a.componentLeaf(authority, plan.outcome.descriptor, priorOutcome)
		if leafErr != nil {
			return fail(leafErr)
		}
		current, leafErr := authority.terminal(priorOutcome)
		if leafErr != nil || current.kind != formalComponentCallOutcomes {
			if leafErr != nil {
				return fail(leafErr)
			}
			return fail(errFormalComponentMalformed)
		}
		alternatives := current.callOutcomes.Join(
			plan.program.registry,
			callpayload.NewCallOutcomeAlternativeSet(plan.program.registry, outcome),
		)
		outcomeLeaf, leafErr := authority.internCallOutcomes(alternatives)
		if leafErr != nil {
			return fail(leafErr)
		}
		if leafErr = publish(region.care, plan.outcome.ordinal, outcomeLeaf); leafErr != nil {
			return fail(leafErr)
		}
	}
	if liveCare == decisionFalse {
		return formalRelationTuple{}, nil
	}
	writes := make([]formalFiberWrite, 0, len(affected))
	for _, candidate := range affected {
		descriptor := span.forest.descriptors[span.first+int(candidate.ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, candidate.root); err != nil {
			return fail(err)
		}
		prior, readErr := directory.valueAt(predecessor.root, candidate.ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		if prior != formalFiberValue(candidate.root) {
			writes = append(writes, formalFiberWrite{ordinal: candidate.ordinal, value: formalFiberValue(candidate.root)})
		}
	}
	result := predecessor
	if len(writes) != 0 {
		delta, sealErr := directory.sealDelta(writes)
		if sealErr != nil {
			return fail(sealErr)
		}
		root, _, applyErr := directory.applyDelta(predecessor.root, delta)
		if applyErr != nil {
			return fail(applyErr)
		}
		result = formalRelationTuple{variable: predecessor.variable, root: root}
	}
	result, err = a.writeCare(result, liveCare)
	if err != nil {
		return fail(err)
	}
	return a.normalize(result), nil
}
