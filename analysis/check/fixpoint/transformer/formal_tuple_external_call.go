package transformer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
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
	hasValuesTop     bool
	lanes            []formalExternalCallInputLane
	dynamicDirectory []formalExternalCallDynamicDirectory
	typestateQueries []formalExternalCallTypestateResourceQuery
	diagnostics      formalFiberDescriptor
	// staticOrdinals are sealed once and contain only value/guard-independent
	// frame inputs. dynamicOrdinals remain owned by the demand adapter: they
	// must not be pulled into the first sparse partition merely because the
	// frozen input layout has a dynamic-read lane.
	staticOrdinals  []formalFiberOrdinal
	dynamicOrdinals []formalFiberOrdinal
	ordinals        []formalFiberOrdinal
	readsDiag       bool
	readsReach      bool
}

// formalExternalCallDynamicDirectory is the freeze-time bridge from the
// ProductDomain-owned demand vocabulary to this wire's physical directory.
// It deliberately records no runtime demand: later execution must ask the
// DynamicReadDemandCursor and fail if that cursor names a coordinate absent
// from this sealed directory.
type formalExternalCallDynamicDirectory struct {
	lane       state.ProductLane
	group      formalFiberGroupDescriptor
	rekey      state.CoordinateFormalRootRekey
	coordinate state.CoordinateFormalBoundaryFactorPlan
	families   []formalExternalCallInputFamily
}

func formalExternalCallCanonicalOrdinals(in []formalFiberOrdinal) []formalFiberOrdinal {
	out := append([]formalFiberOrdinal(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, ordinal := range out {
		if write == 0 || out[write-1] != ordinal {
			out[write], write = ordinal, write+1
		}
	}
	return out[:write]
}

// formalExternalCallDynamicDemandSource follows a cursor slot backward through
// the wire's sealed formal-boundary image.  The resulting position is a
// frozen leaf coordinate, not a second runtime directory: absent or
// non-unique images fail before a sparse leaf is selected.
func formalExternalCallDynamicDemandSource(
	domain state.ProductDomain,
	span formalFiberDescriptorSpan,
	directory formalExternalCallDynamicDirectory,
	target state.CoordinateSlot,
) (*formalExternalCallInputFamily, state.CoordinateSlot, int, error) {
	for familyIndex := range directory.families {
		family := &directory.families[familyIndex]
		if family.family.family != target.Family() {
			continue
		}
		source, found, err := domain.FormalBoundarySourceSlot(directory.coordinate, target)
		if err != nil {
			return nil, state.CoordinateSlot{}, -1, err
		}
		if !found {
			source, err = domain.RekeyCoordinateSlotFormal(directory.rekey, target)
			if err != nil {
				return family, state.CoordinateSlot{}, -1, nil
			}
		}
		for position, candidate := range family.slots {
			equal, equalErr := domain.CoordinateSlotEqual(candidate, source)
			if equalErr != nil {
				return nil, state.CoordinateSlot{}, -1, equalErr
			}
			if equal {
				if position >= len(family.positions) {
					return nil, state.CoordinateSlot{}, -1, errFormalComponentMalformed
				}
				return family, source, family.positions[position], nil
			}
		}
		position, exact := formalCoordinatePosition(domain, span, family.family, source)
		if !exact {
			// The rekey can identify a dynamic point-local slot whose formal
			// spelling is not a materialized leaf fiber. Its only lawful answer
			// is the registered scalar default under the selected skeleton; the
			// caller still fails closed for a missing lane or family.
			return family, source, -1, nil
		}
		return family, source, position, nil
	}
	return nil, state.CoordinateSlot{}, -1, nil
}

// demandOrdinals lowers one exact ProductDomain demand round through the
// freeze-time directory. It intentionally has no fallback to wire.ordinals:
// a missing lane, family, or slot is a malformed frozen adapter rather than a
// request for a Bottom/default component.
func (w formalExternalCallWire) demandOrdinals(domain state.ProductDomain, span formalFiberDescriptorSpan, demands state.DynamicReadDemandSet) ([]formalFiberOrdinal, error) {
	if !domain.Valid() {
		return nil, errFormalComponentMalformed
	}
	var out []formalFiberOrdinal
	for _, lane := range demands.OrdinaryLanes() {
		found := false
		for _, directory := range w.dynamicDirectory {
			if directory.lane != lane {
				continue
			}
			found = true
			out = append(out, directory.group.members...)
		}
		if !found {
			return nil, fmt.Errorf("transformer: dynamic-read demand lane %q is outside frozen wire", lane.ID())
		}
	}
	for _, demand := range demands.CoordinateDemands() {
		familyFound := false
		for _, directory := range w.dynamicDirectory {
			for familyIndex := range directory.families {
				inputFamily := &directory.families[familyIndex]
				if inputFamily.family.family != demand.Family() {
					continue
				}
				familyFound = true
				// A scalar-only round still needs its formal skeleton to derive the
				// registered omitted scalar when the image has no leaf fiber.
				if demand.NeedsSkeleton() || len(demand.Slots()) != 0 {
					out = append(out, inputFamily.family.skeleton)
				}
				for _, slot := range demand.Slots() {
					_, _, position, err := formalExternalCallDynamicDemandSource(domain, span, directory, slot)
					if err != nil {
						return nil, err
					}
					if position < -1 || position >= len(inputFamily.family.scalars) {
						return nil, fmt.Errorf("transformer: dynamic-read demand slot is outside frozen family %q", demand.Family().ID())
					}
					if position < 0 {
						continue
					}
					if position < -1 || position >= len(inputFamily.family.scalars) {
						return nil, errFormalComponentMalformed
					}
					if position < 0 {
						continue
					}
					out = append(out, inputFamily.family.scalars[position])
				}
			}
		}
		if !familyFound {
			return nil, fmt.Errorf("transformer: dynamic-read demand family %q is outside frozen wire", demand.Family().ID())
		}
	}
	return formalExternalCallCanonicalOrdinals(out), nil
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
	slots     []state.CoordinateSlot
}

type formalExternalCallSiteKey struct {
	variable relationVar
	point    cfg.Point
}

type formalExternalCallValueOutput struct {
	root   FormalSlot
	member formalFiberGroupMember
}

// formalExternalCallProofSeedBinding is the occurrence authority for one
// provider-leaf production declaration. It retains the exact boundary binding
// for the typed producer; its certificate is derived later by the registered
// normal-return N3 program, never by treating a refinement as a direct proof.
// The tuple is intentionally provider-path-qualified so a declaration from a
// sibling provider or sibling occurrence cannot witness this proof.
type formalExternalCallProofSeedBinding struct {
	variable   relationVar
	point      cfg.Point
	path       []uint32
	seed       callpayload.CallOutcomeProofSeed
	production callpayload.CallOutcomeProductionProgram
	refinement pathdom.Path
}

func (b formalExternalCallProofSeedBinding) valid() bool {
	_, _, productionOK := b.production.NormalReturnPathRefinement()
	return b.variable != 0 && b.point != 0 && len(b.path) != 0 &&
		!b.refinement.IsEmpty() && productionOK
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
	proofSeeds  []formalExternalCallProofSeedBinding
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
		proofSeeds, proofErr := freezeFormalExternalCallProofSeeds(variable, point, provider, boundaryPaths, nil)
		if proofErr != nil {
			return proofErr
		}
		key := formalExternalCallSiteKey{variable: variable, point: point}
		if _, duplicate := inventory.externalCalls[key]; duplicate {
			return fmt.Errorf("transformer: formal ExternalCall point %d was prepared twice", point)
		}
		inventory.externalCalls[key] = formalPreparedExternalCallSite{
			provider: provider, capability: capability, correlation: correlation, proofSeeds: proofSeeds,
		}
	}
	return nil
}

func freezeFormalExternalCallProofSeeds(
	variable relationVar,
	point cfg.Point,
	provider callpayload.CallOutcomeSiteProgram,
	bindings callboundary.PathBindings,
	path []uint32,
) ([]formalExternalCallProofSeedBinding, error) {
	if variable == 0 || point == 0 {
		return nil, fmt.Errorf("transformer: formal ExternalCall proof seed is unowned")
	}
	components := provider.ComponentCount()
	if components != 0 {
		var out []formalExternalCallProofSeedBinding
		for index := 0; index < components; index++ {
			child, exact := provider.Component(index)
			if !exact {
				return nil, fmt.Errorf("transformer: formal ExternalCall proof provider component %d is absent", index)
			}
			childPath := append(append([]uint32(nil), path...), uint32(index))
			childSeeds, err := freezeFormalExternalCallProofSeeds(variable, point, child, bindings, childPath)
			if err != nil {
				return nil, err
			}
			out = append(out, childSeeds...)
		}
		return out, nil
	}
	if len(path) == 0 {
		path = []uint32{0}
	}
	seeds := make([]formalExternalCallProofSeedBinding, 0, provider.ProofSeedCount())
	for index := 0; index < provider.ProofSeedCount(); index++ {
		seed, exact := provider.ProofSeed(index)
		production := seed.ProductionProgram()
		productionPath, _, productionOK := production.NormalReturnPathRefinement()
		bound, boundOK := bindings.Substitute(productionPath)
		if !exact || !productionOK || !boundOK || bound.IsEmpty() {
			return nil, fmt.Errorf("transformer: formal ExternalCall proof seed %d is outside occurrence boundary bindings", index)
		}
		binding := formalExternalCallProofSeedBinding{
			variable: variable, point: point, path: append([]uint32(nil), path...), seed: seed,
			production: production, refinement: bound.Clone(),
		}
		if !binding.valid() {
			return nil, fmt.Errorf("transformer: formal ExternalCall proof seed %d did not seal", index)
		}
		seeds = append(seeds, binding)
	}
	return seeds, nil
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
	provider          formalExternalCallProvider
	factor            factapply.ExternalCallFactorProgram[FormalSlot]
	normal            factapply.NormalReturnFactorCodec[FormalSlot]
	normalValues      []formalFiberGroupMember
	correlation       factapply.CallOutcomeCorrelationFactorProgram
	correlationLane   int
	hasCorrelations   bool
	valuesTop         formalFiberGroupMember
	results           []formalFiberGroupMember
	valueOutputs      []formalExternalCallValueOutput
	valueOutputByRoot map[FormalSlot]int
	outputLanes       []formalClosedSelectedFactor
	outputReads       []formalFiberOrdinal
	outcome           formalCallOutcomeFiber
	diagnostics       formalFiberDescriptor
	operands          callOutcomeOperandTerms
	access            []valueAccessTerm
	publishedPoints   []cfg.Point
	sealed            bool
}

type formalExternalCallProvider struct {
	site         callpayload.CallOutcomeSiteProgram
	path         []uint32
	input        callpayload.ExternalCallInputProgram[statekey.Value]
	wires        []formalExternalCallWire
	guardDemands []formalQualifiedGuardDemand
	operands     callOutcomeOperandTerms
	access       []valueAccessTerm
	children     []formalExternalCallProvider
}

func (p *formalExternalCallStep) valid(operator formalRelationOperatorRef) bool {
	if p == nil || !p.sealed || p.program == nil || p.body == nil || p.variable == 0 ||
		p.body.variable != p.variable || p.body.relation.code != operator.code ||
		p.point == 0 || !p.provider.valid() ||
		p.hasCorrelations != (p.correlationLane >= 0) ||
		len(p.results) != p.factor.ResultCount() || !p.outcome.valid() ||
		len(p.valueOutputs) != len(p.valueOutputByRoot) ||
		len(p.outputLanes) != p.factor.LaneCount() {
		return false
	}
	return true
}

func (p *formalExternalCallProvider) valid() bool {
	if p == nil {
		return false
	}
	componentCount := p.site.ComponentCount()
	if componentCount == 0 {
		if len(p.path) == 0 || !p.input.Valid() || len(p.wires) != p.input.InputCount() || len(p.children) != 0 {
			return false
		}
		for _, wire := range p.wires {
			if wire.point == 0 {
				return false
			}
		}
		return true
	}
	if componentCount != len(p.children) || p.input.Valid() || len(p.wires) != 0 || len(p.guardDemands) != 0 || len(p.path) == 0 {
		return false
	}
	for index := range p.children {
		if !p.children[index].valid() {
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
	normalValueDependencies := make([]statekey.ValueDependency, 0, len(span.liveValues))
	for dependency := range span.liveValues {
		normalValueDependencies = append(normalValueDependencies, dependency)
	}
	sort.Slice(normalValueDependencies, func(i, j int) bool {
		left, leftOK := span.liveValues[normalValueDependencies[i]].CanonicalBytes()
		right, rightOK := span.liveValues[normalValueDependencies[j]].CanonicalBytes()
		if !leftOK || !rightOK {
			return leftOK
		}
		return bytes.Compare(left[:], right[:]) < 0
	})
	normal, err := factapply.PrepareNormalReturnFactorCodec(
		formalAuthority, body.productDomain, contract, step.point, boundaryPaths,
		operator.footprint.inventory, heapRoots, normalValueDependencies,
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
		program, body, variable, operator, prefix, inputPoints, provider,
		span, values, valuesTop, groups, diagnostic,
	)
	if err != nil {
		return nil, fmt.Errorf("transformer: formal ExternalCall provider: %w", err)
	}
	factorLanes := factorProgram.Lanes()
	resultBindings := factorProgram.ResultBindings()
	outputLanes := make([]formalClosedSelectedFactor, len(factorLanes))
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
		outputLanes[index], err = sealFormalClosedSelectedFactor(
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
		program: program, body: body, variable: variable, point: step.point, site: site,
		factor: factorProgram, normal: normal, normalValues: normalValues,
		correlation: preparedSite.correlation, correlationLane: correlationLane, hasCorrelations: hasCorrelations,
		valuesTop: valuesTop, results: resultMembers, outputLanes: outputLanes, outputReads: outputReads,
		valueOutputs: valueOutputs, valueOutputByRoot: valueOutputByRoot,
		outcome: outcome, diagnostics: diagnostic, operands: prefix.operands.clone(), access: cloneValueAccessTerms(prefix.access),
		publishedPoints: append([]cfg.Point(nil), inputPoints[1:]...),
		sealed:          true,
	}
	plan.provider = providerInput
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
	provider callpayload.CallOutcomeSiteProgram,
	span formalFiberDescriptorSpan,
	values formalValuesFiberGroup,
	valuesTop formalFiberGroupMember,
	groups map[state.LaneID]formalFiberGroupDescriptor,
	diagnostic formalFiberDescriptor,
) (formalExternalCallProvider, error) {
	return freezeFormalExternalCallProviderAtPath(
		program, body, variable, operator, step, inputPoints, provider,
		span, values, valuesTop, groups, diagnostic, []uint32{0},
	)
}

func freezeFormalExternalCallProviderAtPath(
	program *RelationProgram,
	body *relationProgramBody,
	variable relationVar,
	operator formalRelationOperatorRef,
	step boundaryPrefixStep,
	inputPoints []cfg.Point,
	provider callpayload.CallOutcomeSiteProgram,
	span formalFiberDescriptorSpan,
	values formalValuesFiberGroup,
	valuesTop formalFiberGroupMember,
	groups map[state.LaneID]formalFiberGroupDescriptor,
	diagnostic formalFiberDescriptor,
	path []uint32,
) (formalExternalCallProvider, error) {
	componentCount := provider.ComponentCount()
	if componentCount != 0 {
		children := make([]formalExternalCallProvider, componentCount)
		for index := 0; index < componentCount; index++ {
			component, exact := provider.Component(index)
			if !exact {
				return formalExternalCallProvider{}, fmt.Errorf("component %d is absent", index)
			}
			childPath := append(append([]uint32(nil), path...), uint32(index))
			child, err := freezeFormalExternalCallProviderAtPath(
				program, body, variable, operator, step, inputPoints, component,
				span, values, valuesTop, groups, diagnostic, childPath,
			)
			if err != nil {
				return formalExternalCallProvider{}, fmt.Errorf("component %d: %w", index, err)
			}
			children[index] = child
		}
		return formalExternalCallProvider{site: provider, path: append([]uint32(nil), path...), children: children}, nil
	}
	capability := provider.Capability()
	observation := capability.InputObservation()
	operands := step.operands.selectObservation(observation)
	access := selectFormalExternalCallProviderAccess(step.access, operands, observation)
	leafPoints, err := selectFormalExternalCallProviderPoints(inputPoints, step.point, access, capability)
	if err != nil {
		return formalExternalCallProvider{}, err
	}
	operandContract, err := externalCallTransferAccessWithAccess(body, step, access, leafPoints, len(leafPoints), 0, capability)
	if err != nil {
		return formalExternalCallProvider{}, err
	}
	providerContract, err := externalCallTransferAccessWithoutDynamicOperandLanes(body, step, access, leafPoints, len(leafPoints), 0, capability)
	if err != nil {
		return formalExternalCallProvider{}, err
	}
	input, err := callpayload.PrepareExternalCallInputProgram(
		body.productDomain, providerContract, leafPoints, 0,
		func(slot statekey.Value) (statekey.Value, bool) { return slot, slot != 0 },
	)
	if err != nil {
		return formalExternalCallProvider{}, err
	}
	operandInput, err := callpayload.PrepareExternalCallInputProgram(
		body.productDomain, operandContract, leafPoints, 0,
		func(slot statekey.Value) (statekey.Value, bool) { return slot, slot != 0 },
	)
	if err != nil || operandInput.InputCount() != input.InputCount() {
		if err == nil {
			err = errFormalComponentMalformed
		}
		return formalExternalCallProvider{}, err
	}
	dynamicLanes, err := body.productDomain.DynamicReadPotentialLanes()
	if err != nil {
		return formalExternalCallProvider{}, fmt.Errorf("dynamic-read lane directory: %w", err)
	}
	wires := make([]formalExternalCallWire, input.InputCount())
	for wireIndex := range wires {
		layout, exact := input.Layout(wireIndex)
		if !exact {
			return formalExternalCallProvider{}, fmt.Errorf("input wire %d is absent", wireIndex)
		}
		operandLayout, exact := operandInput.Layout(wireIndex)
		if !exact || operandLayout.Point() != layout.Point() {
			return formalExternalCallProvider{}, fmt.Errorf("operand input wire %d is absent", wireIndex)
		}
		environment := formalPublicationPointOutput
		if layout.Point() == step.point {
			environment = formalPublicationPointInput
		}
		projection, err := freezeFormalPointPublicationInverse(body, span, layout.Point(), environment)
		if err != nil {
			return formalExternalCallProvider{}, fmt.Errorf("input wire %d projection: %w", wireIndex, err)
		}
		wire := formalExternalCallWire{point: layout.Point(), projection: projection, readsDiag: layout.ReadsDiagnostics(), readsReach: layout.ReadsReachable()}
		if len(layout.ValueRoots()) != 0 {
			wire.valuesTop, wire.hasValuesTop = valuesTop, true
		}
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
						position, exact := formalCoordinatePosition(body.productDomain, span, family, slot)
						if !exact {
							return formalExternalCallProvider{}, fmt.Errorf("input coordinate is outside frozen family")
						}
						positions[slotIndex] = position
					}
					inputLane.families = append(inputLane.families, formalExternalCallInputFamily{family: family, positions: positions, slots: familyLayout.Slots()})
				}
			}
			wire.lanes = append(wire.lanes, inputLane)
		}
		// Operand dynamic-read authority is deliberately separate from the
		// complete provider frame.  The cursor below owns when these roots are
		// selected; the provider input itself contains only static/capability
		// lanes unless it explicitly declared one through its capability.
		for _, lane := range operandLayout.Lanes() {
			if !dynamicLanes.Has(lane.ID()) {
				continue
			}
			group, exact := groups[lane.ID()]
			if !exact || group.lane != lane {
				return formalExternalCallProvider{}, fmt.Errorf("dynamic input lane %q is absent", lane.ID())
			}
			inputLane := formalExternalCallInputLane{group: group}
			if group.kind == formalFiberGroupCoordinateLane {
				inputLane.coordinate, err = body.productDomain.SealCoordinateFormalBoundaryFactorPlan(projection, group.lane)
				if err != nil {
					return formalExternalCallProvider{}, fmt.Errorf("dynamic input lane %q projection plan: %w", lane.ID(), err)
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
						return formalExternalCallProvider{}, fmt.Errorf("dynamic input family is outside lane %q", lane.ID())
					}
					positions := make([]int, len(familyLayout.Slots()))
					for slotIndex, slot := range familyLayout.Slots() {
						position, exact := formalCoordinatePosition(body.productDomain, span, family, slot)
						if !exact {
							return formalExternalCallProvider{}, fmt.Errorf("dynamic input coordinate is outside frozen family")
						}
						positions[slotIndex] = position
					}
					inputLane.families = append(inputLane.families, formalExternalCallInputFamily{family: family, positions: positions, slots: familyLayout.Slots()})
				}
			}
			wire.dynamicDirectory = append(wire.dynamicDirectory, formalExternalCallDynamicDirectory{lane: lane, group: group, rekey: span.rekey, coordinate: inputLane.coordinate, families: inputLane.families})
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
					position, positionExact := formalCoordinatePosition(body.productDomain, span, family, slot)
					if !positionExact {
						return formalExternalCallProvider{}, fmt.Errorf("keyed typestate query coordinate is outside frozen family")
					}
					positions[slotIndex] = position
				}
				queryInput.path.families = append(queryInput.path.families, formalExternalCallInputFamily{family: family, positions: positions, slots: familyLayout.Slots()})
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
		if wire.hasValuesTop {
			wire.staticOrdinals = append(wire.staticOrdinals, valuesTop.ordinal)
		}
		for _, member := range wire.values {
			wire.staticOrdinals = append(wire.staticOrdinals, member.ordinal)
		}
		for _, lane := range wire.lanes {
			target := &wire.staticOrdinals
			if lane.group.kind == formalFiberGroupOrdinaryLane {
				*target = append(*target, lane.group.members...)
				continue
			}
			for _, family := range lane.families {
				*target = append(*target, family.family.skeleton)
				for _, position := range family.positions {
					*target = append(*target, family.family.scalars[position])
				}
			}
		}
		for _, directory := range wire.dynamicDirectory {
			if directory.group.kind == formalFiberGroupOrdinaryLane {
				wire.dynamicOrdinals = append(wire.dynamicOrdinals, directory.group.members...)
				continue
			}
			for _, family := range directory.families {
				wire.dynamicOrdinals = append(wire.dynamicOrdinals, family.family.skeleton)
				for _, position := range family.positions {
					wire.dynamicOrdinals = append(wire.dynamicOrdinals, family.family.scalars[position])
				}
			}
		}
		// Typestate queries are independently sealed query programs rather than
		// operand-owned DynamicRead reads; retain their historical behavior in
		// the static materialization phase.
		for _, query := range wire.typestateQueries {
			wire.staticOrdinals = append(wire.staticOrdinals, query.ordinals...)
		}
		if wire.readsDiag {
			wire.diagnostics = diagnostic
			ordinal, exact := span.ordinal(diagnostic)
			if !exact {
				return formalExternalCallProvider{}, fmt.Errorf("diagnostics ordinal is absent")
			}
			wire.staticOrdinals = append(wire.staticOrdinals, ordinal)
		}
		wire.staticOrdinals = formalExternalCallCanonicalOrdinals(wire.staticOrdinals)
		wire.dynamicOrdinals = formalExternalCallCanonicalOrdinals(wire.dynamicOrdinals)
		wire.ordinals = formalExternalCallCanonicalOrdinals(append(append([]formalFiberOrdinal(nil), wire.staticOrdinals...), wire.dynamicOrdinals...))
		wires[wireIndex] = wire
	}
	var guardDemands []formalQualifiedGuardDemand
	operands.each(func(term ValueTerm) bool {
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
	return formalExternalCallProvider{
		site: provider, path: append([]uint32(nil), path...), input: input, wires: wires,
		guardDemands: guardDemands, operands: operands, access: access,
	}, nil
}

// selectFormalExternalCallProviderAccess retains exactly the canonical source
// reads reachable from roles named by one leaf certificate.  The access row is
// already frozen by the relation compiler; this is a stable subset, not a
// runtime scan or a newly allocated semantic plan.
func selectFormalExternalCallProviderAccess(
	access []valueAccessTerm,
	operands callOutcomeOperandTerms,
	observation callpayload.CallOutcomeInputObservation,
) []valueAccessTerm {
	// The compatibility declaration is intentionally the whole compiler-sealed
	// DAG row.  Its internal ValueTerms include transitive read nodes which are
	// not themselves public CallOutcomeInput roles; filtering those by top-term
	// identity would silently sever dynamic operand evaluation.
	if observation.ObservesCallee() && observation.ObservesReceiver() && observation.AllArguments() {
		return cloneValueAccessTerms(access)
	}
	selected := make(map[ValueTerm]struct{}, len(operands.arguments)+2)
	operands.each(func(term ValueTerm) bool { selected[term] = struct{}{}; return true })
	out := make([]valueAccessTerm, 0, len(access))
	for _, item := range access {
		if _, ok := selected[item.term]; ok {
			out = append(out, item)
		}
	}
	return out
}

// selectFormalExternalCallProviderPoints preserves the site's canonical point
// order while omitting a historical wire unless a selected source role or the
// leaf's lane/query capability actually observes it.
func selectFormalExternalCallProviderPoints(
	inputPoints []cfg.Point,
	primary cfg.Point,
	access []valueAccessTerm,
	capability callpayload.CallOutcomeCapability,
) ([]cfg.Point, error) {
	points := make([]cfg.Point, 0, len(inputPoints))
	for _, point := range inputPoints {
		include := point == primary
		if !include {
			lanes, err := capability.ReadInputLanes(point)
			if err != nil {
				return nil, err
			}
			include = lanes.Len() != 0
		}
		if !include {
			for _, item := range access {
				if item.hasPoint && item.point == point {
					include = true
					break
				}
			}
		}
		if include {
			points = append(points, point)
		}
	}
	if len(points) == 0 || points[0] != primary {
		return nil, fmt.Errorf("transformer: ExternalCall leaf lost primary input point")
	}
	return points, nil
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
	provider *formalExternalCallProvider,
	views []formalSparseLeafView,
) (callpayload.ExternalCallInputFrame[statekey.Value], error) {
	if a == nil || plan == nil || !plan.sealed || provider == nil || len(provider.children) != 0 ||
		len(views) != len(provider.wires) {
		return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
	}
	operands := make([]callpayload.ExternalCallInputWireOperands, len(views))
	for index, view := range views {
		wire := provider.wires[index]
		if view.variable != plan.variable || view.authority == nil {
			return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentForeignOwner
		}
		topLeaf := decisionLeaf(0)
		if wire.hasValuesTop {
			topOrdinal, exact := wire.valuesTop.address(wire.valuesTop.group)
			if !exact {
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
			}
			var present bool
			topLeaf, present = view.leaf(topOrdinal)
			if !present || topLeaf > 1 {
				return callpayload.ExternalCallInputFrame[statekey.Value]{}, errFormalComponentMalformed
			}
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
	return provider.input.BindFrame(operands)
}

// materializeFormalExternalCallDynamicDirectory answers one frozen dynamic
// lane directory from a sparse view.  The caller must have selected every
// requested ordinal first: absent is malformed, while a selected zero leaf is
// the domain-owned Bottom/default spelling and is handled only through the
// coordinate formal boundary plan.
func (a *formalTupleAlgebra) materializeFormalExternalCallDynamicDirectory(
	plan *formalExternalCallStep,
	wire formalExternalCallWire,
	view formalSparseLeafView,
	directory formalExternalCallDynamicDirectory,
) (state.LaneFactor, error) {
	if a == nil || plan == nil || view.authority == nil || directory.group.lane != directory.lane {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	if directory.group.kind == formalFiberGroupOrdinaryLane {
		factor, err := view.laneFactor(directory.group)
		if err != nil {
			return state.LaneFactor{}, err
		}
		return plan.body.productDomain.RekeyOrdinaryLaneFactorFormalPublication(wire.projection, factor)
	}
	if directory.group.kind != formalFiberGroupCoordinateLane {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	families := make([]state.CoordinateFormalBoundaryFamilyOperands, len(directory.families))
	for familyIndex, familyPlan := range directory.families {
		skeletonLeaf, present := view.leaf(familyPlan.family.skeleton)
		if !present {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		var err error
		if skeletonLeaf == 0 {
			families[familyIndex].Skeleton, err = view.authority.product.CoordinateSkeletonBottom(familyPlan.family.family, view.span.keys)
		} else {
			terminal, terminalErr := view.authority.terminal(skeletonLeaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateSkeleton ||
				!coordinateFamilySame(terminal.skeleton.Family(), familyPlan.family.family) {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			families[familyIndex].Skeleton = terminal.skeleton
		}
		if err != nil {
			return state.LaneFactor{}, err
		}
		families[familyIndex].Scalars = make([]state.CoordinateScalarFactor, len(familyPlan.positions))
		for scalarIndex, position := range familyPlan.positions {
			if position < 0 || position >= len(familyPlan.family.scalars) {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			leaf, present := view.leaf(familyPlan.family.scalars[position])
			if !present {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			if leaf == 0 {
				continue
			}
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			families[familyIndex].Scalars[scalarIndex] = terminal.scalar
		}
	}
	return plan.body.productDomain.ApplyCoordinateFormalBoundaryFactorPlan(directory.coordinate, families)
}

// formalExternalCallDynamicEvidence preserves the existing complete-frame
// behavior while routing it through the cursor callback.  It is intentionally
// also the fail-closed bridge used by the staged evaluator: the next change
// narrows its selected directories to cursor demands, but a missing directory
// is already an error rather than a fabricated State/default.
func (a *formalTupleAlgebra) formalExternalCallDynamicEvidence(
	plan *formalExternalCallStep,
	provider *formalExternalCallProvider,
	views []formalSparseLeafView,
	point cfg.Point,
	query state.DynamicReadQuery,
) (state.DynamicReadEvidence, error) {
	if a == nil || plan == nil || provider == nil || len(views) != len(provider.wires) {
		return state.DynamicReadEvidence{}, errFormalComponentMalformed
	}
	var factors []state.LaneFactor
	for index, wire := range provider.wires {
		if wire.point != point {
			continue
		}
		for _, directory := range wire.dynamicDirectory {
			factor, err := a.materializeFormalExternalCallDynamicDirectory(plan, wire, views[index], directory)
			if err != nil {
				return state.DynamicReadEvidence{}, fmt.Errorf("transformer: materialize dynamic external-call lane %q: %w", directory.lane.ID(), err)
			}
			found := -1
			for factorIndex := range factors {
				if factors[factorIndex].Lane() == factor.Lane() {
					found = factorIndex
					break
				}
			}
			if found < 0 {
				factors = append(factors, factor)
				continue
			}
			joined, err := plan.body.productDomain.LaneJoin(factors[found], factor)
			if err != nil {
				return state.DynamicReadEvidence{}, err
			}
			factors[found] = joined
		}
	}
	if len(factors) == 0 {
		return state.DynamicReadEvidence{}, fmt.Errorf("transformer: dynamic external-call point %d has no frozen directory", point)
	}
	return plan.body.productDomain.ProjectDynamicReadEvidenceFactors(query, factors)
}

func (a *formalTupleAlgebra) formalExternalCallDynamicBatch(
	plan *formalExternalCallStep,
	wire formalExternalCallWire,
	view formalSparseLeafView,
	cursor state.DynamicReadDemandCursor,
) (state.DynamicReadEvidenceBatch, error) {
	demands := cursor.Demands()
	batch := state.DynamicReadEvidenceBatch{}
	find := func(lane state.ProductLane) (*formalExternalCallDynamicDirectory, error) {
		for index := range wire.dynamicDirectory {
			if wire.dynamicDirectory[index].lane == lane {
				return &wire.dynamicDirectory[index], nil
			}
		}
		return nil, fmt.Errorf("transformer: dynamic-read demand lane %q is outside frozen wire", lane.ID())
	}
	for _, lane := range demands.OrdinaryLanes() {
		directory, err := find(lane)
		if err != nil {
			return state.DynamicReadEvidenceBatch{}, err
		}
		factor, err := a.materializeFormalExternalCallDynamicDirectory(plan, wire, view, *directory)
		if err != nil {
			return state.DynamicReadEvidenceBatch{}, err
		}
		projected, projectErr := cursor.ProjectOrdinaryDemand(lane, factor)
		if projectErr != nil {
			return state.DynamicReadEvidenceBatch{}, projectErr
		}
		batch.Ordinary = append(batch.Ordinary, projected)
	}
	for _, demand := range demands.CoordinateDemands() {
		var directory *formalExternalCallDynamicDirectory
		var family *formalExternalCallInputFamily
		for directoryIndex := range wire.dynamicDirectory {
			candidate := &wire.dynamicDirectory[directoryIndex]
			for familyIndex := range candidate.families {
				if candidate.families[familyIndex].family.family == demand.Family() {
					directory, family = candidate, &candidate.families[familyIndex]
					break
				}
			}
			if family != nil {
				break
			}
		}
		if directory == nil || family == nil {
			return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: dynamic-read demand family %q is outside frozen wire", demand.Family().ID())
		}
		answer := state.DynamicReadCoordinateBatch{Family: demand.Family(), HasSkeleton: demand.NeedsSkeleton()}
		leaf, present := view.leaf(family.family.skeleton)
		if !present {
			return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: demanded dynamic-read skeleton was not selected")
		}
		var sourceSkeleton state.CoordinateFamilySkeleton
		if leaf == 0 {
			var err error
			sourceSkeleton, err = view.authority.product.CoordinateSkeletonBottom(family.family.family, view.span.keys)
			if err != nil {
				return state.DynamicReadEvidenceBatch{}, err
			}
		} else {
			terminal, err := view.authority.terminal(leaf)
			if err != nil || terminal.kind != formalComponentCoordinateSkeleton || terminal.skeleton.Family() != demand.Family() {
				return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: demanded dynamic-read skeleton has malformed formal terminal")
			}
			sourceSkeleton = terminal.skeleton
		}
		sourceScalars := make([]state.CoordinateScalarFactor, len(demand.Slots()))
		for slotIndex, target := range demand.Slots() {
			mappedFamily, source, position, sourceErr := formalExternalCallDynamicDemandSource(plan.body.productDomain, view.span, *directory, target)
			if sourceErr != nil {
				return state.DynamicReadEvidenceBatch{}, sourceErr
			}
			if mappedFamily == nil || mappedFamily != family || position < -1 || position >= len(family.family.scalars) {
				return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: demanded dynamic-read slot is outside frozen family")
			}
			if position < 0 {
				defaultScalar, defaultErr := plan.body.productDomain.CoordinateDefault(sourceSkeleton, source)
				if defaultErr != nil {
					return state.DynamicReadEvidenceBatch{}, defaultErr
				}
				sourceScalars[slotIndex] = defaultScalar
				continue
			}
			scalarLeaf, scalarPresent := view.leaf(family.family.scalars[position])
			if !scalarPresent {
				return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: demanded dynamic-read scalar was not selected")
			}
			if scalarLeaf == 0 {
				defaultScalar, defaultErr := plan.body.productDomain.CoordinateDefault(sourceSkeleton, source)
				if defaultErr != nil {
					return state.DynamicReadEvidenceBatch{}, defaultErr
				}
				sourceScalars[slotIndex] = defaultScalar
				continue
			}
			terminal, terminalErr := view.authority.terminal(scalarLeaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
				return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: demanded dynamic-read scalar has malformed formal terminal")
			}
			sourceScalars[slotIndex] = terminal.scalar
		}
		targetSkeleton, targetScalars, rekeyErr := plan.body.productDomain.RekeyFormalBoundaryCoordinateEvidence(
			directory.coordinate, demand.Family(), sourceSkeleton, sourceScalars,
		)
		if rekeyErr != nil || len(targetScalars) != len(demand.Slots()) {
			if rekeyErr == nil {
				rekeyErr = errFormalComponentMalformed
			}
			return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: rekey dynamic-read coordinate evidence: %w", rekeyErr)
		}
		if demand.NeedsSkeleton() {
			answer.Skeleton = targetSkeleton
		}
		for slotIndex, target := range demand.Slots() {
			equal, equalErr := plan.body.productDomain.CoordinateSlotEqual(targetScalars[slotIndex].Slot(), target)
			if equalErr != nil || !equal {
				if equalErr == nil {
					equalErr = errFormalComponentMalformed
				}
				return state.DynamicReadEvidenceBatch{}, fmt.Errorf("transformer: rekeyed dynamic-read coordinate does not match cursor demand: %w", equalErr)
			}
			answer.Scalars = append(answer.Scalars, targetScalars[slotIndex])
		}
		batch.Coordinate = append(batch.Coordinate, answer)
	}
	return batch, nil
}

type formalExternalCallDemandOutcome struct {
	guard   decisionRef
	outcome callpayload.CallOutcome
}

type formalExternalCallResolvedDemand struct {
	point    cfg.Point
	evidence state.DynamicReadEvidence
}

// evaluateFormalExternalCallProviderDemandDriven resumes one suspended
// operand query under its parent guard.  Each cursor round adds only the
// frozen ordinals named by that round; a demand miss is returned as an error
// before the cursor can advance.  This is an acyclic finite traversal of the
// registered demand protocol, not a solver iteration.
func (a *formalTupleAlgebra) evaluateFormalExternalCallProviderDemandDriven(
	plan *formalExternalCallStep,
	provider *formalExternalCallProvider,
	tuples []formalRelationTuple,
	parent decisionRef,
	ordinals [][]formalFiberOrdinal,
	point cfg.Point,
	cursor state.DynamicReadDemandCursor,
	resolved []formalExternalCallResolvedDemand,
) ([]formalExternalCallDemandOutcome, error) {
	if len(tuples) != len(provider.wires) || len(ordinals) != len(provider.wires) || cursor.Complete() {
		return nil, errFormalComponentMalformed
	}
	index := -1
	for candidate, wire := range provider.wires {
		if wire.point == point {
			if index >= 0 {
				return nil, fmt.Errorf("transformer: dynamic external-call point %d is ambiguous", point)
			}
			index = candidate
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("transformer: dynamic external-call point %d is absent", point)
	}
	span, _, _, spanOK := a.span(plan.variable)
	if !spanOK {
		return nil, errFormalComponentForeignOwner
	}
	demanded, err := provider.wires[index].demandOrdinals(plan.body.productDomain, span, cursor.Demands())
	if err != nil {
		return nil, fmt.Errorf("transformer: lower dynamic-read demand ordinals: %w", err)
	}
	nextOrdinals := make([][]formalFiberOrdinal, len(ordinals))
	for wireIndex := range ordinals {
		nextOrdinals[wireIndex] = append([]formalFiberOrdinal(nil), ordinals[wireIndex]...)
	}
	nextOrdinals[index] = formalExternalCallCanonicalOrdinals(append(nextOrdinals[index], demanded...))
	projections := make([]formalSparseTupleProjection, len(tuples))
	for wireIndex := range tuples {
		projections[wireIndex] = formalSparseTupleProjection{tuple: tuples[wireIndex], ordinals: nextOrdinals[wireIndex]}
		for _, query := range provider.wires[wireIndex].typestateQueries {
			root, queryErr := a.compileFormalTypestateResourceQuery(tuples[wireIndex], query)
			if queryErr != nil {
				return nil, queryErr
			}
			projections[wireIndex].derived = append(projections[wireIndex].derived, root)
		}
	}
	regions, err := a.partitionSparseLeafViewsUnderGuard(projections, nil, parent)
	if err != nil {
		return nil, err
	}
	var out []formalExternalCallDemandOutcome
	for _, region := range regions {
		if len(region.views) != len(provider.wires) || region.guard == decisionFalse {
			return nil, errDecisionMalformed
		}
		branch := cursor
		batch, err := a.formalExternalCallDynamicBatch(plan, provider.wires[index], region.views[index], branch)
		if err != nil {
			return nil, fmt.Errorf("transformer: materialize dynamic-read demand batch: %w", err)
		}
		if err := branch.Resume(batch); err != nil {
			return nil, fmt.Errorf("transformer: resume dynamic-read demand cursor: %w", err)
		}
		if !branch.Complete() {
			next, err := a.evaluateFormalExternalCallProviderDemandDriven(plan, provider, tuples, region.guard, nextOrdinals, point, branch, resolved)
			if err != nil {
				return nil, err
			}
			out = append(out, next...)
			continue
		}
		evidence, complete := branch.Evidence()
		if !complete {
			return nil, errFormalComponentMalformed
		}
		completed := append(append([]formalExternalCallResolvedDemand(nil), resolved...), formalExternalCallResolvedDemand{point: point, evidence: evidence})
		used := 0
		outcome, err := a.evaluateFormalExternalCallProviderWithDynamicEvidence(
			plan, provider, region.views,
			func(candidate cfg.Point, _ state.DynamicReadQuery) (state.DynamicReadEvidence, bool, error) {
				if used >= len(completed) || candidate != completed[used].point {
					return state.DynamicReadEvidence{}, false, nil
				}
				evidence := completed[used].evidence
				used++
				return evidence, true, nil
			},
		)
		if err != nil {
			var suspended externalCallOperandSuspension
			if errors.As(err, &suspended) {
				nested, nestedErr := plan.body.productDomain.BeginDynamicReadDemandCursor(suspended.query)
				if nestedErr != nil {
					return nil, nestedErr
				}
				return a.evaluateFormalExternalCallProviderDemandDriven(plan, provider, tuples, region.guard, nextOrdinals, suspended.point, nested, completed)
			}
			return nil, err
		}
		out = append(out, formalExternalCallDemandOutcome{guard: region.guard, outcome: outcome})
	}
	return out, nil
}

// evaluateFormalExternalCallProvider is the sole formal provider invocation.
// It consumes a sealed factor frame and returns normalized scratch outcome
// syntax; publication happens only after the complete factor transaction.
func (a *formalTupleAlgebra) evaluateFormalExternalCallProvider(
	plan *formalExternalCallStep,
	provider *formalExternalCallProvider,
	views []formalSparseLeafView,
) (callpayload.CallOutcome, error) {
	return a.evaluateFormalExternalCallProviderWithDynamicEvidence(plan, provider, views, nil)
}

func (a *formalTupleAlgebra) evaluateFormalExternalCallProviderWithDynamicEvidence(
	plan *formalExternalCallStep,
	provider *formalExternalCallProvider,
	views []formalSparseLeafView,
	resolveDynamic externalCallDynamicEvidence,
) (callpayload.CallOutcome, error) {
	if a == nil || provider == nil {
		return callpayload.CallOutcome{}, errFormalComponentMalformed
	}
	// A provider path identifies a leaf only within one ExternalCall step.  It
	// is deliberately reused by every step's retained composition tree, so the
	// evaluation-local cache must also bind the owning relation variable and
	// call point.  Demand-driven callers have already materialized their exact
	// evidence into views; the legacy complete-frame caller may still discover
	// dynamic evidence, and is therefore not cacheable before that discovery.
	cacheable := resolveDynamic != nil
	cacheHash := uint64(0)
	if cacheable {
		cacheHash = formalExternalCallLeafOutcomeCacheHash(plan, provider, views)
		for _, entry := range a.externalCallLeafOutcomes[cacheHash] {
			if formalExternalCallLeafOutcomeCacheEqual(entry, plan, provider, views) {
				return entry.outcome, nil
			}
		}
	}
	frame, err := a.bindFormalExternalCallInput(plan, provider, views)
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
	if resolveDynamic == nil {
		resolveDynamic = func(point cfg.Point, query state.DynamicReadQuery) (state.DynamicReadEvidence, bool, error) {
			evidence, evidenceErr := a.formalExternalCallDynamicEvidence(plan, provider, views, point, query)
			return evidence, evidenceErr == nil, evidenceErr
		}
	}
	operands, err := evaluateExternalCallOperandsWithDynamicEvidence(
		plan.body, provider.operands, provider.access, frame, concreteExternalCallDynamicQuery(plan.body),
		resolveDynamic,
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
	outcome, err := provider.site.Evaluate(node, input)
	if err != nil {
		return callpayload.CallOutcome{}, err
	}
	if cacheable {
		if a.externalCallLeafOutcomes == nil {
			a.externalCallLeafOutcomes = make(map[uint64][]formalExternalCallLeafOutcomeCacheEntry)
		}
		a.externalCallLeafOutcomes[cacheHash] = append(a.externalCallLeafOutcomes[cacheHash], formalExternalCallLeafOutcomeCacheEntryFor(plan, provider, views, outcome))
	}
	return outcome, nil
}

// countDistinctFormalExternalCallInputs is trace-only cardinality accounting
// for the exact provider relation. Region guard identity is deliberately not
// part of the key: only the representation-interned projection terminals,
// derived observation terminals, and demanded guard truth leaves can affect
// provider evaluation. Hash collisions are resolved by complete vector
// equality, so the count never relies on a probabilistic identity.
func (a *formalTupleAlgebra) countDistinctFormalExternalCallInputs(
	plan *formalExternalCallStep,
	provider *formalExternalCallProvider,
	regions []formalSparseLeafPartition,
) (int, error) {
	if a == nil || plan == nil || !plan.sealed || provider == nil || len(provider.children) != 0 {
		return 0, errFormalComponentForeignOwner
	}
	buckets := make(map[uint64][][]decisionLeaf)
	distinct := 0
	for _, region := range regions {
		if len(region.views) != len(provider.wires) || len(region.views) == 0 {
			return 0, errFormalComponentMalformed
		}
		width := len(provider.guardDemands)
		for _, view := range region.views {
			width += len(view.leaves) + len(view.derived)
		}
		vector := make([]decisionLeaf, 0, width)
		for _, view := range region.views {
			vector = append(vector, view.leaves...)
			vector = append(vector, view.derived...)
		}
		for _, demand := range provider.guardDemands {
			truth, falsity, exact := region.views[0].exactGuard(
				demand.owner, demand.arena, demand.scope, demand.guard,
			)
			if !exact || truth == falsity {
				return 0, errDecisionMalformed
			}
			if truth {
				vector = append(vector, 1)
			} else {
				vector = append(vector, 0)
			}
		}
		hash := formalFactorLeafHash(vector)
		duplicate := false
		for _, prior := range buckets[hash] {
			if formalFactorLeavesEqual(prior, vector) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		buckets[hash] = append(buckets[hash], vector)
		distinct++
	}
	return distinct, nil
}

type formalExternalCallProviderRelation struct {
	care    decisionRef
	outcome decisionRef
}

type formalExternalCallLeafOutcomeCacheView struct {
	variable relationVar
	ordinals []formalFiberOrdinal
	leaves   []decisionLeaf
	derived  []decisionLeaf
}

type formalExternalCallLeafOutcomeCacheEntry struct {
	body     *relationProgramBody
	variable relationVar
	point    cfg.Point
	path     []uint32
	views    []formalExternalCallLeafOutcomeCacheView
	outcome  callpayload.CallOutcome
}

func formalExternalCallLeafOutcomeCacheHash(plan *formalExternalCallStep, provider *formalExternalCallProvider, views []formalSparseLeafView) uint64 {
	parts := make([]decisionLeaf, 0, 2+len(provider.path)+len(views)*2)
	parts = append(parts, decisionLeaf(plan.variable), decisionLeaf(plan.point))
	for _, part := range provider.path {
		parts = append(parts, decisionLeaf(part))
	}
	for _, view := range views {
		parts = append(parts, decisionLeaf(view.variable), decisionLeaf(len(view.ordinals)))
		for index, ordinal := range view.ordinals {
			parts = append(parts, decisionLeaf(ordinal), view.leaves[index])
		}
		parts = append(parts, decisionLeaf(len(view.derived)))
		parts = append(parts, view.derived...)
	}
	return formalFactorLeafHash(parts)
}

func formalExternalCallLeafOutcomeCacheEqual(entry formalExternalCallLeafOutcomeCacheEntry, plan *formalExternalCallStep, provider *formalExternalCallProvider, views []formalSparseLeafView) bool {
	if entry.body != plan.body || entry.variable != plan.variable || entry.point != plan.point || len(entry.path) != len(provider.path) || len(entry.views) != len(views) {
		return false
	}
	for index := range entry.path {
		if entry.path[index] != provider.path[index] {
			return false
		}
	}
	for index, view := range views {
		cached := entry.views[index]
		if cached.variable != view.variable || !slices.Equal(cached.ordinals, view.ordinals) ||
			!formalFactorLeavesEqual(cached.leaves, view.leaves) || !formalFactorLeavesEqual(cached.derived, view.derived) {
			return false
		}
	}
	return true
}

func formalExternalCallLeafOutcomeCacheEntryFor(plan *formalExternalCallStep, provider *formalExternalCallProvider, views []formalSparseLeafView, outcome callpayload.CallOutcome) formalExternalCallLeafOutcomeCacheEntry {
	entry := formalExternalCallLeafOutcomeCacheEntry{body: plan.body, variable: plan.variable, point: plan.point, path: append([]uint32(nil), provider.path...), outcome: outcome, views: make([]formalExternalCallLeafOutcomeCacheView, len(views))}
	for index, view := range views {
		entry.views[index] = formalExternalCallLeafOutcomeCacheView{
			variable: view.variable,
			ordinals: append([]formalFiberOrdinal(nil), view.ordinals...),
			leaves:   append([]decisionLeaf(nil), view.leaves...),
			derived:  append([]decisionLeaf(nil), view.derived...),
		}
	}
	return entry
}

// evaluateFormalExternalCallProviderRelation lifts the retained provider
// composition tree. Leaves observe only their own site capability; composed
// nodes combine the resulting guarded relations through the exact same
// directional merge law used by CallOutcomeSiteProgram.Evaluate.
func (a *formalTupleAlgebra) evaluateFormalExternalCallProviderRelation(
	plan *formalExternalCallStep,
	provider *formalExternalCallProvider,
	predecessor formalRelationTuple,
	published []formalRelationTuple,
	authority *formalComponentTerminalAuthority,
	directory *formalFiberDirectoryArena,
	trace *formalRelationEvalTraceDetail,
	path string,
) (formalExternalCallProviderRelation, error) {
	if a == nil || plan == nil || provider == nil || !provider.valid() || authority == nil || directory == nil {
		return formalExternalCallProviderRelation{}, errFormalComponentMalformed
	}
	if len(provider.children) != 0 {
		// Keep the retained composition tree streaming. In particular, do not
		// materialize a child-relation slice on the fixed-point path: each child
		// is lifted and merged immediately in its original program order.
		// MergeComponentPrefix is deliberately prefix-sensitive, so neither this
		// fold nor the DD merge below may be reassociated or reordered.
		firstPath := ""
		if trace != nil {
			firstPath = fmt.Sprintf("%s.0", path)
		}
		first, err := a.evaluateFormalExternalCallProviderRelation(
			plan, &provider.children[0], predecessor, published, authority, directory, trace, firstPath,
		)
		if err != nil {
			return formalExternalCallProviderRelation{}, err
		}
		care, outcome := first.care, first.outcome
		node := transfer.NodeContext{
			Context: a.ctx, Registry: plan.program.registry, Graph: plan.body.graph,
			Node: plan.body.graph.Node(plan.point), Point: plan.point,
		}
		for index := 1; index < len(provider.children); index++ {
			childPath := ""
			if trace != nil {
				childPath = fmt.Sprintf("%s.%d", path, index)
			}
			child, childErr := a.evaluateFormalExternalCallProviderRelation(
				plan, &provider.children[index], predecessor, published, authority, directory, trace, childPath,
			)
			if childErr != nil {
				return formalExternalCallProviderRelation{}, childErr
			}
			var err error
			jointCare, err := a.decisions.apply(a.ctx, uint8(decisionAnd), true, care, child.care, decisionLeafAnd)
			if err != nil {
				return formalExternalCallProviderRelation{}, err
			}
			if jointCare == decisionFalse {
				return formalExternalCallProviderRelation{care: decisionFalse, outcome: decisionFalse}, nil
			}
			prefixCount := index + 1
			merged, err := a.decisions.applyVectorUnderCare(
				a.ctx, jointCare, care, child.care,
				[]decisionRef{outcome}, []decisionRef{child.outcome},
				func(left, right []decisionLeaf) ([]decisionLeaf, error) {
					if len(left) != 1 || len(right) != 1 {
						return nil, errDecisionMalformed
					}
					leftTerminal, terminalErr := authority.terminal(left[0])
					if terminalErr != nil || leftTerminal.kind != formalComponentRawCallOutcome {
						return nil, errFormalComponentMalformed
					}
					rightTerminal, terminalErr := authority.terminal(right[0])
					if terminalErr != nil || rightTerminal.kind != formalComponentRawCallOutcome {
						return nil, errFormalComponentMalformed
					}
					merged, mergeErr := provider.site.MergeComponentPrefix(
						node, prefixCount, leftTerminal.rawCallOutcome, rightTerminal.rawCallOutcome,
					)
					if mergeErr != nil {
						return nil, mergeErr
					}
					leaf, internErr := authority.internRawCallOutcome(merged)
					if internErr != nil {
						return nil, internErr
					}
					return []decisionLeaf{leaf}, nil
				},
			)
			if err != nil || len(merged) != 1 {
				if err == nil {
					err = errDecisionMalformed
				}
				return formalExternalCallProviderRelation{}, err
			}
			care, outcome = jointCare, merged[0]
		}
		return formalExternalCallProviderRelation{care: care, outcome: outcome}, nil
	}

	var componentTrace *formalExternalCallProviderEvalTrace
	if trace != nil {
		roles := provider.site.Capability().FieldRoles()
		names := make([]string, len(roles))
		for index, role := range roles {
			names[index] = role.FieldName
		}
		trace.externalCallProviderComponents = append(trace.externalCallProviderComponents, formalExternalCallProviderEvalTrace{
			path: fmt.Sprint(provider.path), owner: provider.site.Owner(), capability: fmt.Sprintf("%q", names), inputs: len(provider.wires),
			wires: len(provider.wires), guards: len(provider.guardDemands),
		})
		componentTrace = &trace.externalCallProviderComponents[len(trace.externalCallProviderComponents)-1]
		for _, wire := range provider.wires {
			componentTrace.valueRoots += len(wire.values)
			componentTrace.laneGroups += len(wire.lanes)
			componentTrace.ordinals += len(wire.ordinals)
		}
	}
	var inputMark formalRelationEvalTracePhaseMark
	if trace != nil {
		inputMark = beginFormalRelationEvalTracePhase(a)
		trace.externalCallProviderInputs += len(provider.wires)
	}
	projections := make([]formalSparseTupleProjection, len(provider.wires))
	tuples := make([]formalRelationTuple, len(provider.wires))
	initialOrdinals := make([][]formalFiberOrdinal, len(provider.wires))
	usedPublished := make([]bool, len(published))
	for index, wire := range provider.wires {
		tuple := predecessor
		if index == 0 {
			if wire.point != plan.point {
				return formalExternalCallProviderRelation{}, errFormalComponentMalformed
			}
		} else {
			found := -1
			for candidate, point := range plan.publishedPoints {
				if !usedPublished[candidate] && point == wire.point {
					found = candidate
					break
				}
			}
			if found < 0 || found >= len(published) {
				return formalExternalCallProviderRelation{}, errFormalComponentMalformed
			}
			usedPublished[found], tuple = true, published[found]
		}
		tuples[index] = tuple
		initialOrdinals[index] = wire.staticOrdinals
		projections[index] = formalSparseTupleProjection{tuple: tuple, ordinals: wire.staticOrdinals}
		for _, query := range wire.typestateQueries {
			root, err := a.compileFormalTypestateResourceQuery(tuple, query)
			if err != nil {
				return formalExternalCallProviderRelation{}, err
			}
			projections[index].derived = append(projections[index].derived, root)
		}
	}
	regions, err := a.partitionSparseLeafViewsUnderCare(projections, provider.guardDemands)
	if trace != nil {
		var roots []decisionRef
		for _, projection := range projections {
			for _, ordinal := range projection.ordinals {
				if value, readErr := directory.valueAt(projection.tuple.root, ordinal); readErr == nil {
					roots = append(roots, decisionRef(value))
				}
			}
			roots = append(roots, projection.derived...)
		}
		trace.externalCallProviderRoots += len(roots)
		if len(roots) > trace.externalCallProviderMaxChildRoots {
			trace.externalCallProviderMaxChildRoots = len(roots)
		}
		trace.externalCallProviderSupport = mergeFormalRelationTraceSupportRanks(
			trace.externalCallProviderSupport, formalRelationTraceSupportRanks(&a.decisions, roots...),
		)
		trace.externalCallProviderRegions += len(regions)
		componentTrace.occurrenceRoots = len(roots)
		uniqueRoots := make(map[decisionRef]struct{}, len(roots))
		for _, root := range roots {
			uniqueRoots[root] = struct{}{}
		}
		componentTrace.uniqueRoots = len(uniqueRoots)
		componentTrace.support = formalRelationTraceSupportRanks(&a.decisions, roots...)
		componentTrace.regions = len(regions)
		if err == nil {
			distinct, distinctErr := a.countDistinctFormalExternalCallInputs(plan, provider, regions)
			if distinctErr != nil {
				err = distinctErr
			} else {
				trace.externalCallDistinctProviderInputs += distinct
				componentTrace.distinct = distinct
			}
		}
		finishFormalRelationEvalTracePhase(a, &trace.externalCallInput, inputMark)
	}
	if err != nil {
		return formalExternalCallProviderRelation{}, err
	}
	care, outcomeRoot := decisionFalse, decisionFalse
	publishOutcome := func(guard decisionRef, outcome callpayload.CallOutcome) error {
		var outcomeMark formalRelationEvalTracePhaseMark
		if trace != nil {
			outcomeMark = beginFormalRelationEvalTracePhase(a)
		}
		leaf, providerErr := authority.internRawCallOutcome(outcome)
		if providerErr != nil {
			return providerErr
		}
		outcomeRoot, providerErr = a.decisions.condition(a.ctx, guard, a.decisions.terminal(leaf), outcomeRoot)
		if providerErr != nil {
			return providerErr
		}
		care, providerErr = a.decisions.apply(a.ctx, uint8(decisionOr), true, care, guard, decisionLeafOr)
		if providerErr != nil {
			return providerErr
		}
		if trace != nil {
			finishFormalRelationEvalTracePhase(a, &trace.externalCallProviderOutcome, outcomeMark)
		}
		return nil
	}
	for _, region := range regions {
		if len(region.views) != len(provider.wires) || region.guard == decisionFalse {
			return formalExternalCallProviderRelation{}, errDecisionMalformed
		}
		var providerMark formalRelationEvalTracePhaseMark
		if trace != nil {
			providerMark = beginFormalRelationEvalTracePhase(a)
		}
		outcome, providerErr := a.evaluateFormalExternalCallProviderWithDynamicEvidence(
			plan, provider, region.views,
			func(cfg.Point, state.DynamicReadQuery) (state.DynamicReadEvidence, bool, error) {
				return state.DynamicReadEvidence{}, false, nil
			},
		)
		if trace != nil {
			finishFormalRelationEvalTracePhase(a, &trace.externalCallProvider, providerMark)
		}
		var suspended externalCallOperandSuspension
		if errors.As(providerErr, &suspended) {
			cursor, cursorErr := plan.body.productDomain.BeginDynamicReadDemandCursor(suspended.query)
			if cursorErr != nil {
				return formalExternalCallProviderRelation{}, cursorErr
			}
			outcomes, demandErr := a.evaluateFormalExternalCallProviderDemandDriven(
				plan, provider, tuples, region.guard, initialOrdinals, suspended.point, cursor, nil,
			)
			if demandErr != nil {
				return formalExternalCallProviderRelation{}, demandErr
			}
			for _, demanded := range outcomes {
				if trace != nil {
					trace.externalCallProviderEvals++
					trace.externalCallProviderChildEvals++
					componentTrace.evals++
				}
				if publishErr := publishOutcome(demanded.guard, demanded.outcome); publishErr != nil {
					return formalExternalCallProviderRelation{}, publishErr
				}
			}
			continue
		}
		if providerErr != nil {
			return formalExternalCallProviderRelation{}, providerErr
		}
		if trace != nil {
			trace.externalCallProviderEvals++
			trace.externalCallProviderChildEvals++
			componentTrace.evals++
		}
		if publishErr := publishOutcome(region.guard, outcome); publishErr != nil {
			return formalExternalCallProviderRelation{}, publishErr
		}
	}
	return formalExternalCallProviderRelation{care: care, outcome: outcomeRoot}, nil
}

func mergeFormalRelationTraceSupportRanks(left, right []uint32) []uint32 {
	out := append(append([]uint32(nil), left...), right...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, rank := range out {
		if write == 0 || out[write-1] != rank {
			out[write], write = rank, write+1
		}
	}
	return out[:write]
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
		factor, err := a.materializeFormalClosedSelectedFactor(primary, lane)
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
	if a == nil || plan == nil || !plan.valid(operator) || predecessor.variable != plan.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal ExternalCall execution is unowned")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		return predecessor, err
	}
	// Leaf outcomes are memoized only within this closed ExternalCall evaluation.
	a.externalCallLeafOutcomes = make(map[uint64][]formalExternalCallLeafOutcomeCacheEntry)
	for _, tuple := range published {
		if tuple.variable != plan.variable {
			return formalRelationTuple{}, errFormalComponentForeignOwner
		}
	}
	span, directory, authority, ok := a.span(plan.variable)
	if !ok || predecessor.root.owner != directory {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	var traceDetail *formalRelationEvalTraceDetail
	if a.evalTrace != nil && a.evalTrace.active != nil {
		traceDetail = a.evalTrace.active
		traceDetail.externalCallPlan = plan
	}
	rawProviderTerminalsBefore := 0
	if traceDetail != nil {
		rawProviderTerminalsBefore = authority.rawCallOutcomeTerminalCount()
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	providerRelation, err := a.evaluateFormalExternalCallProviderRelation(
		plan, &plan.provider, predecessor, published, authority, directory, traceDetail, "0",
	)
	if err != nil {
		return fail(err)
	}
	providerCare := providerRelation.care
	if providerCare == decisionFalse {
		return formalRelationTuple{}, nil
	}
	// Provider syntax remains raw through every ordered composition edge. Only
	// the completed root is converted to the canonical semantic outcome set,
	// exactly matching concrete Evaluate followed by boundary normalization.
	normalized, err := a.decisions.applyVectorUnderCare(
		a.ctx, providerCare, providerCare, decisionFalse,
		[]decisionRef{providerRelation.outcome}, []decisionRef{providerRelation.outcome},
		func(left, right []decisionLeaf) ([]decisionLeaf, error) {
			if len(left) != 1 || len(right) != 0 {
				return nil, errDecisionMalformed
			}
			terminal, terminalErr := authority.terminal(left[0])
			if terminalErr != nil || terminal.kind != formalComponentRawCallOutcome {
				return nil, errFormalComponentMalformed
			}
			leaf, terminalErr := authority.internCallOutcomes(callpayload.NewCallOutcomeAlternativeSet(
				plan.program.registry, terminal.rawCallOutcome,
			))
			if terminalErr != nil {
				return nil, terminalErr
			}
			return []decisionLeaf{leaf}, nil
		},
	)
	if err != nil || len(normalized) != 1 {
		if err == nil {
			err = errDecisionMalformed
		}
		return fail(err)
	}
	providerOutcome := normalized[0]
	if traceDetail != nil {
		traceDetail.externalCallRawProviderTerminals = authority.rawCallOutcomeTerminalCount() - rawProviderTerminalsBefore
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
	var commitMark formalRelationEvalTracePhaseMark
	if traceDetail != nil {
		traceDetail.externalCallOutcomeSupport = formalRelationTraceSupportRanks(&a.decisions, providerOutcome)
		traceDetail.externalCallCommitRoots = len(commitRoots)
		traceDetail.externalCallCommitSupport = formalRelationTraceSupportRanks(&a.decisions, commitRoots...)
		commitMark = beginFormalRelationEvalTracePhase(a)
	}
	commitRegions, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, providerCare, commitRoots)
	if traceDetail != nil {
		traceDetail.externalCallCommitRegions = len(commitRegions)
		finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallCommitPartition, commitMark)
	}
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
		var publishMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			publishMark = beginFormalRelationEvalTracePhase(a)
			traceDetail.externalCallPublicationConditions++
		}
		affected[index].root, publishErr = a.decisions.condition(
			a.ctx, guard, a.decisions.terminal(leaf), affected[index].root,
		)
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallPublication, publishMark)
		}
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
		var outerMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			outerMark = beginFormalRelationEvalTracePhase(a)
		}
		outer, leafErr := a.applyFormalExternalCallOuter(plan, primary, outcome)
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallOuter, outerMark)
		}
		if leafErr != nil {
			return fail(leafErr)
		}
		var normalMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			normalMark = beginFormalRelationEvalTracePhase(a)
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
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallNormal, normalMark)
		}
		if leafErr != nil {
			return fail(leafErr)
		}
		if !next.Reachable {
			continue
		}
		if plan.hasCorrelations {
			var correlationMark formalRelationEvalTracePhaseMark
			if traceDetail != nil {
				correlationMark = beginFormalRelationEvalTracePhase(a)
			}
			if plan.correlationLane < 0 || plan.correlationLane >= len(next.Factors) {
				return fail(errFormalComponentMalformed)
			}
			next.Factors[plan.correlationLane], leafErr = plan.correlation.Apply(next.Factors[plan.correlationLane], outcome)
			if traceDetail != nil {
				finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallCorrelation, correlationMark)
			}
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
			outputs, factorErr := a.factorFormalClosedSelectedFactor(authority, span, lane, next.Factors[index])
			if factorErr != nil {
				return fail(factorErr)
			}
			for _, output := range outputs {
				if factorErr = publish(region.care, output.ordinal, output.leaf); factorErr != nil {
					return fail(factorErr)
				}
			}
		}
		var diagnosticsMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			diagnosticsMark = beginFormalRelationEvalTracePhase(a)
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
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallDiagnostics, diagnosticsMark)
		}
		if leafErr = publish(region.care, diagnosticOrdinal, diagnosticLeaf); leafErr != nil {
			return fail(leafErr)
		}
		var ledgerMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			ledgerMark = beginFormalRelationEvalTracePhase(a)
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
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallLedger, ledgerMark)
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
		var publishMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			publishMark = beginFormalRelationEvalTracePhase(a)
			traceDetail.externalCallDeltaWrites = len(writes)
		}
		delta, sealErr := directory.sealDelta(writes)
		if sealErr != nil {
			return fail(sealErr)
		}
		root, _, applyErr := directory.applyDelta(predecessor.root, delta)
		if applyErr != nil {
			return fail(applyErr)
		}
		result = formalRelationTuple{variable: predecessor.variable, root: root}
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.externalCallPublication, publishMark)
		}
	}
	result, err = a.writeCare(result, liveCare)
	if err != nil {
		return fail(err)
	}
	return a.normalize(result), nil
}
