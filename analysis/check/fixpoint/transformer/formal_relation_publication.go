package transformer

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// formalPublishedCoordinate is one body-owned cell in a completed formal
// execution. Concrete callers cannot name it directly: publication must first
// select an exact point/edge resolver environment.
type formalPublishedCoordinate struct {
	view       *FormalRelationPublicationView
	cell       formalRelationCell
	inverse    state.CoordinateFormalPublicationProjection
	inverseErr error
}

func formalPublicationOutputCell(program *RelationProgram, variable relationVar, ref relationRootRef) formalRelationCell {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || ref == 0 {
		return formalRelationCell{}
	}
	code := program.bodies[variable-1].relation.code
	if code == nil || int(ref) >= len(code.nodes) {
		return formalRelationCell{}
	}
	node := code.nodes[ref]
	switch node.kind {
	case relationNodeNonreturning:
		return program.formalRegion.nonreturning[variable-1]
	case relationNodeOutcome:
		if node.outcome == 0 || int(node.outcome) > len(program.formalRegion.outcomes[variable-1]) {
			return formalRelationCell{}
		}
		return program.formalRegion.outcomes[variable-1][node.outcome-1]
	case relationNodeSequence:
		// A terminal CFG point reduces to its point-owned Steps followed by an
		// Outcome/Nonreturning node. N5 lives on the Outcome equation, so the
		// point output is that terminal cell, not the last pre-N5 Step. Ordinary
		// sequences continue to publish their last point-owned Step and never
		// traverse into another CFG point's relation root.
		if node.next != 0 && int(node.next) < len(code.nodes) {
			terminal := code.nodes[node.next]
			switch terminal.kind {
			case relationNodeOutcome:
				if terminal.outcome == 0 || int(terminal.outcome) > len(program.formalRegion.outcomes[variable-1]) {
					return formalRelationCell{}
				}
				return program.formalRegion.outcomes[variable-1][terminal.outcome-1]
			case relationNodeNonreturning:
				return program.formalRegion.nonreturning[variable-1]
			}
		}
		for index := len(node.steps) - 1; index >= 0; index-- {
			if relationCodeStepHasCoordinate(code, node.steps[index]) {
				return formalRelationCell{Variable: variable, Root: ref, Step: uint32(index + 1), Kind: formalRelationCellStep}
			}
		}
	}
	return formalRelationCell{Variable: variable, Root: ref, Kind: formalRelationCellNode}
}

func (c formalPublishedCoordinate) valid() bool {
	return c.view != nil && c.cell.valid() && c.cell.Variable == c.view.variable
}

// FormalRelationPublicationView materializes only cells owned by one lexical
// body. A selected concrete root can publish directly; a symbolic body cell
// fails structurally if a free formal root has no concrete edge binding.
type FormalRelationPublicationView struct {
	execution   *formalRelationExecution
	body        *relationProgramBody
	variable    relationVar
	ordinals    []formalFiberOrdinal
	groups      []formalFiberGroupDescriptor
	unbound     []formalFiberOrdinal
	diagnostic  formalFiberOrdinal
	callSites   map[cfg.Point][]formalRelationCell
	calls       []FormalLexicalCallDependency
	pointInput  map[cfg.Point][]formalPublishedCoordinate
	pointOutput map[cfg.Point][]formalPublishedCoordinate
	edgeNormal  map[cfg.Edge][]formalPublishedCoordinate
	pathFactors *formalPathObservationCache
}

type formalPathObservationCacheKey struct {
	point    cfg.Point
	boundary bool
}

// formalPathObservationCache keeps only the selected path-evidence factor for
// an observation coordinate. It is intentionally not a State cache: values,
// heap, and all unrelated lanes remain unmaterialized for the read-model.
type formalPathObservationCache struct {
	mu      sync.Mutex
	factors map[formalPathObservationCacheKey]state.LaneFactor
	present map[formalPathObservationCacheKey]bool
}

// Publication returns a route-free body publication capability. It does not
// materialize a State until a named cell is requested.
func (e *formalRelationExecution) Publication(bodyID lexicalidentity.StableLexicalBodyID) (FormalRelationPublicationView, error) {
	if e == nil || e.algebra == nil || e.algebra.program == nil || e.values == nil {
		return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication is unowned")
	}
	program := e.algebra.program
	variable, present := program.byBody[bodyID]
	if !present || variable == 0 || int(variable) > len(program.bodies) {
		return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication has no body %s", bodyID)
	}
	span, present := program.formalFibers.span(variable)
	if !present {
		return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication has no product authority")
	}
	ordinals := make([]formalFiberOrdinal, 0, span.count)
	unbound := make([]formalFiberOrdinal, 0)
	var diagnostic formalFiberOrdinal = -1
	for _, descriptor := range span.descriptors() {
		if descriptor.role == formalFiberDiagnostics {
			ordinal, exact := span.ordinal(descriptor)
			if !exact || diagnostic >= 0 {
				return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication diagnostic inventory is malformed")
			}
			diagnostic = ordinal
		}
		if descriptor.role == formalFiberMiddleValue || descriptor.role == formalFiberMiddlePath {
			ordinal, exact := span.ordinal(descriptor)
			if !exact {
				return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication unbound inventory is malformed")
			}
			unbound = append(unbound, ordinal)
		}
	}
	groups := span.groupDescriptors()
	for _, group := range groups {
		ordinals = append(ordinals, group.members...)
	}
	if diagnostic < 0 {
		return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication has no diagnostic coordinate")
	}
	// Product publication partitions the complete registered product roots
	// under Care. Middle binding roots are existential syntax only: including
	// them would merely subdivide identical product rows, because every product
	// correlation is already retained by the shared DD partition.
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	for index := 1; index < len(ordinals); index++ {
		if ordinals[index-1] >= ordinals[index] {
			return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication product inventory overlaps")
		}
	}
	view := FormalRelationPublicationView{
		execution: e, body: &program.bodies[variable-1], variable: variable,
		ordinals: ordinals, groups: groups, unbound: unbound, diagnostic: diagnostic,
		callSites:  make(map[cfg.Point][]formalRelationCell),
		calls:      make([]FormalLexicalCallDependency, 0),
		pointInput: make(map[cfg.Point][]formalPublishedCoordinate), pointOutput: make(map[cfg.Point][]formalPublishedCoordinate),
		edgeNormal: make(map[cfg.Edge][]formalPublishedCoordinate),
		pathFactors: &formalPathObservationCache{
			factors: make(map[formalPathObservationCacheKey]state.LaneFactor),
			present: make(map[formalPathObservationCacheKey]bool),
		},
	}
	// The prepared call-site census is the publication inventory. Equation
	// syntax identifies each reachable producer, but cannot define the census:
	// allocation calls are deliberately represented by the canonical Effect
	// transaction rather than an ExternalCall instruction, and unreachable
	// syntax may have no surviving equation at all.
	for ordinal := 0; ordinal < view.body.graph.Size(); ordinal++ {
		point := cfg.Point(ordinal)
		if _, call := view.body.plan.Facts().CallSiteView(point); call {
			view.callSites[point] = nil
		}
	}
	// Equation order is the sealed WTO canonical order. Freeze the complete
	// incoming Apply-site inventory once per body publication so every point
	// reuses the same deterministic caller environment set.
	for _, equation := range program.formalTemplate.equations {
		operator, operatorPresent := equation.terminalOperator()
		if operatorPresent && operator.apply != nil {
			if operator.apply.owner == variable && operator.apply.linked != nil {
				point := operator.apply.linked.point
				view.callSites[point] = append(view.callSites[point], equation.Cell.cell)
				target := operator.apply.target
				if target == 0 || int(target) > len(program.bodies) {
					return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication has a foreign Apply target")
				}
				view.calls = append(view.calls, FormalLexicalCallDependency{
					Point: point, Occurrence: operator.apply.linked.occurrence,
					Target: program.bodies[target-1].body,
				})
			}
			continue
		}
		step, present := formalRelationStepOperator(operator)
		if !present || equation.Cell.cell.Variable != variable {
			continue
		}
		if step.kind == boundaryStepExternalCall && step.point > 0 {
			view.callSites[step.point] = append(view.callSites[step.point], equation.Cell.cell)
		}
	}
	sort.Slice(view.calls, func(i, j int) bool { return formalLexicalCallDependencyLess(view.calls[i], view.calls[j]) })
	for index := 1; index < len(view.calls); index++ {
		if view.calls[index-1] == view.calls[index] {
			return FormalRelationPublicationView{}, fmt.Errorf("transformer: formal publication repeats an Apply dependency")
		}
	}
	code := view.body.relation.code
	for _, publication := range code.publication.points {
		// Reduction maps a structurally published but unreachable CFG point to
		// ref zero. Preserve the map key as its explicit Bottom publication;
		// manufacturing a node cell would make unreachable syntax executable.
		if publication.ref == 0 {
			if _, declared := view.pointInput[publication.point]; !declared {
				view.pointInput[publication.point] = nil
			}
			if _, declared := view.pointOutput[publication.point]; !declared {
				view.pointOutput[publication.point] = nil
			}
			continue
		}
		inputInverse, inputErr := freezeFormalPointPublicationInverse(view.body, span, publication.point, formalPublicationPointInput)
		outputInverse, outputErr := freezeFormalPointPublicationInverse(view.body, span, publication.point, formalPublicationPointOutput)
		input := formalRelationCell{Variable: variable, Root: publication.ref, Kind: formalRelationCellNode}
		output := formalPublicationOutputCell(program, variable, publication.ref)
		view.pointInput[publication.point] = append(view.pointInput[publication.point], formalPublishedCoordinate{view: &view, cell: input, inverse: inputInverse, inverseErr: inputErr})
		view.pointOutput[publication.point] = append(view.pointOutput[publication.point], formalPublishedCoordinate{view: &view, cell: output, inverse: outputInverse, inverseErr: outputErr})
	}
	for _, publication := range code.publication.edges {
		if publication.ref == 0 {
			edge := cfg.Edge{From: publication.from, To: publication.to}
			if _, declared := view.edgeNormal[edge]; !declared {
				view.edgeNormal[edge] = nil
			}
			continue
		}
		inverse, inverseErr := freezeFormalPointPublicationInverse(view.body, span, publication.from, formalPublicationPointOutput)
		cell := formalPublicationOutputCell(program, variable, publication.ref)
		edge := cfg.Edge{From: publication.from, To: publication.to}
		view.edgeNormal[edge] = append(view.edgeNormal[edge], formalPublishedCoordinate{view: &view, cell: cell, inverse: inverse, inverseErr: inverseErr})
	}
	// Coordinates retain a pointer to the returned view. Rebind after all maps
	// are frozen so no coordinate points at an intermediate copy.
	for point := range view.pointInput {
		for index := range view.pointInput[point] {
			view.pointInput[point][index].view = &view
		}
	}
	for point := range view.pointOutput {
		for index := range view.pointOutput[point] {
			view.pointOutput[point][index].view = &view
		}
	}
	for edge := range view.edgeNormal {
		for index := range view.edgeNormal[edge] {
			view.edgeNormal[edge][index].view = &view
		}
	}
	return view, nil
}

// node names a formal relation node for formal-only projections such as
// diagnostics. It is intentionally private: a raw node has no concrete
// resolver environment.
func (v *FormalRelationPublicationView) node(ref relationRootRef) (formalPublishedCoordinate, bool) {
	if v == nil || v.execution == nil || v.execution.algebra == nil || ref == 0 {
		return formalPublishedCoordinate{}, false
	}
	cell := formalRelationCell{Variable: v.variable, Root: ref, Kind: formalRelationCellNode}
	_, present := v.execution.values[cell]
	coordinate := formalPublishedCoordinate{view: v, cell: cell, inverseErr: fmt.Errorf("transformer: raw formal node has no concrete publication environment")}
	return coordinate, present && coordinate.valid()
}

// PointInput publishes the indexed relation input at point under that point's
// exact input resolver environment.
func (v *FormalRelationPublicationView) PointInput(ctx context.Context, point cfg.Point, index int) (state.State, bool, error) {
	if v == nil || index < 0 || index >= len(v.pointInput[point]) {
		return state.State{}, false, nil
	}
	coordinate := v.pointInput[point][index]
	coordinate.view = v
	value, err := v.joinState(ctx, coordinate)
	return value, true, err
}

// PlannedNodeOutput publishes the indexed planned node output under that
// point's exact output resolver environment.
func (v *FormalRelationPublicationView) PlannedNodeOutput(ctx context.Context, point cfg.Point, index int) (state.State, bool, error) {
	if v == nil || index < 0 || index >= len(v.pointOutput[point]) {
		return state.State{}, false, nil
	}
	coordinate := v.pointOutput[point][index]
	coordinate.view = v
	value, err := v.joinState(ctx, coordinate)
	return value, true, err
}

// EdgeNormal publishes the indexed normal edge output under its source
// point's exact output resolver environment.
func (v *FormalRelationPublicationView) EdgeNormal(ctx context.Context, edge cfg.Edge, index int) (state.State, bool, error) {
	if v == nil || index < 0 || index >= len(v.edgeNormal[edge]) {
		return state.State{}, false, nil
	}
	coordinate := v.edgeNormal[edge][index]
	coordinate.view = v
	value, err := v.joinState(ctx, coordinate)
	return value, true, err
}

// PointInputAll reads every formal producer for a point input. Point readers
// must not select coordinate zero: other producers carry independent product
// payloads such as implication, numeric, placement, effect, and evidence
// facts.
func (v *FormalRelationPublicationView) PointInputAll(ctx context.Context, point cfg.Point) (state.State, bool, error) {
	if v == nil || v.body == nil {
		return state.State{}, false, fmt.Errorf("transformer: formal point-input publication is unowned")
	}
	coordinates, declared := v.pointInput[point]
	if !declared {
		return state.State{}, false, fmt.Errorf("transformer: formal point-input %d is undeclared", point)
	}
	if len(coordinates) == 0 {
		return v.body.domain.Bottom(), false, nil
	}
	return v.joinPublishedCoordinates(ctx, coordinates)
}

// PlannedNodeOutputAll reads every formal producer for a point output.
func (v *FormalRelationPublicationView) PlannedNodeOutputAll(ctx context.Context, point cfg.Point) (state.State, bool, error) {
	if v == nil || v.body == nil {
		return state.State{}, false, fmt.Errorf("transformer: formal point-output publication is unowned")
	}
	coordinates, declared := v.pointOutput[point]
	if !declared {
		return state.State{}, false, fmt.Errorf("transformer: formal point-output %d is undeclared", point)
	}
	if len(coordinates) == 0 {
		return v.body.domain.Bottom(), false, nil
	}
	return v.joinPublishedCoordinates(ctx, coordinates)
}

// EdgeNormalAll reads every formal producer for a normal edge.
func (v *FormalRelationPublicationView) EdgeNormalAll(ctx context.Context, edge cfg.Edge) (state.State, bool, error) {
	if v == nil || v.body == nil {
		return state.State{}, false, fmt.Errorf("transformer: formal normal-edge publication is unowned")
	}
	coordinates, declared := v.edgeNormal[edge]
	if !declared {
		return state.State{}, false, fmt.Errorf("transformer: formal normal edge %d->%d is undeclared", edge.From, edge.To)
	}
	if len(coordinates) == 0 {
		return v.body.domain.Bottom(), false, nil
	}
	return v.joinPublishedCoordinates(ctx, coordinates)
}

// joinPublishedCoordinates is the Result read-model's only correlation
// forgetting boundary. Summary projection consumes direct formal readers and
// never calls this State-bearing helper.
func (v *FormalRelationPublicationView) joinPublishedCoordinates(
	ctx context.Context,
	coordinates []formalPublishedCoordinate,
) (state.State, bool, error) {
	if len(coordinates) == 0 {
		return state.State{}, false, fmt.Errorf("transformer: formal result publication has no named coordinate")
	}
	joined := v.body.domain.Bottom()
	reachable := false
	for _, coordinate := range coordinates {
		coordinate.view = v
		value, err := v.joinState(ctx, coordinate)
		if err != nil {
			return state.State{}, false, err
		}
		if v.body.domain.Equal(value, v.body.domain.Bottom()) {
			continue
		}
		if !reachable {
			joined, reachable = value, true
		} else {
			joined = v.body.domain.Join(joined, value)
		}
	}
	return state.NormalizeForDomain(v.body.domain, joined), reachable, nil
}

// CallOutcome publishes the exact alternatives produced at one lexical call
// point and collapses them only at the DTO boundary. The semantic fiber keeps
// physical absence distinct from an executed call whose sole alternative is
// the zero outcome. This Result boundary then totalizes every declared lexical
// call point: an unreachable or nonreturning call with no normal alternative
// publishes the zero DTO without fabricating a semantic-fiber producer.
//
// The solved tuple is read directly; no equation, boundary transfer, or
// route-owned projection is replayed during publication.
func (v *FormalRelationPublicationView) CallOutcome(ctx context.Context, point cfg.Point) (callpayload.CallOutcome, bool, error) {
	if ctx == nil || v == nil || v.execution == nil || v.execution.algebra == nil || point == 0 {
		return callpayload.CallOutcome{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return callpayload.CallOutcome{}, false, err
	}
	cells, declared := v.callSites[point]
	if !declared {
		return callpayload.CallOutcome{}, false, nil
	}
	outputs, outputDeclared := v.pointOutput[point]
	if !outputDeclared {
		// Low-level formal-fiber tests may construct a publication capability
		// without the lexical point-output inventory. Emitted Apply/ExternalCall
		// cells remain self-contained; only allocation projection and producer
		// totality require the body output coordinate.
		if len(cells) != 0 {
			return v.callOutcomeFromPublishedOutput(ctx, point, state.State{}, false, false, cells)
		}
		return callpayload.CallOutcome{}, false, fmt.Errorf("transformer: call point %d has no published output coordinate", point)
	}
	output, outputReachable := v.body.domain.Bottom(), false
	if len(outputs) != 0 {
		var err error
		output, outputReachable, err = v.joinPublishedCoordinates(ctx, outputs)
		if err != nil {
			return callpayload.CallOutcome{}, false, err
		}
	}
	pointReachable := outputReachable
	if inputs, declared := v.pointInput[point]; declared {
		pointReachable = false
		if len(inputs) != 0 {
			var err error
			_, pointReachable, err = v.joinPublishedCoordinates(ctx, inputs)
			if err != nil {
				return callpayload.CallOutcome{}, false, err
			}
		}
	}
	return v.callOutcomeFromPublishedOutput(ctx, point, output, pointReachable, outputReachable, cells)
}

func (v *FormalRelationPublicationView) callOutcomeFromPublishedOutput(
	ctx context.Context,
	point cfg.Point,
	output state.State,
	pointReachable bool,
	outputReachable bool,
	cells []formalRelationCell,
) (callpayload.CallOutcome, bool, error) {
	if ctx == nil || v == nil || v.execution == nil || v.execution.algebra == nil || point == 0 {
		return callpayload.CallOutcome{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return callpayload.CallOutcome{}, false, err
	}
	if allocation, exact := v.body.plan.SignatureAllocationOperation(point); exact {
		if !pointReachable {
			return callpayload.CallOutcome{}, true, nil
		}
		if !outputReachable {
			return callpayload.CallOutcome{}, false, fmt.Errorf("transformer: reachable allocation call point %d has no published output", point)
		}
		outcome, err := v.allocationCallOutcomeFromPublishedOutput(point, allocation, output)
		return outcome, true, err
	}
	if len(cells) == 0 {
		if !pointReachable {
			return callpayload.CallOutcome{}, true, nil
		}
		return callpayload.CallOutcome{}, false, fmt.Errorf("transformer: reachable call point %d has no formal producer", point)
	}
	span, spanPresent := v.execution.algebra.program.formalFibers.span(v.variable)
	if !spanPresent {
		return callpayload.CallOutcome{}, false, fmt.Errorf("transformer: formal call-outcome publication has no product authority")
	}
	fiber, externalFiberPresent := span.callOutcomeFiber(point)
	var alternatives callpayload.CallOutcomeAlternativeSet
	for _, cell := range cells {
		if internal, present := v.execution.internalCallOutcomes[cell]; present {
			alternatives = alternatives.Join(v.execution.algebra.program.registry, internal)
			continue
		}
		if !externalFiberPresent {
			return callpayload.CallOutcome{}, false, fmt.Errorf("transformer: external call-outcome cell %+v has no semantic fiber", cell)
		}
		tuple, exact := v.execution.values[cell]
		if !exact {
			return callpayload.CallOutcome{}, false, fmt.Errorf("transformer: formal call-outcome cell %+v is absent", cell)
		}
		current, err := v.execution.algebra.callOutcomeAlternatives(ctx, tuple, fiber)
		if err != nil {
			return callpayload.CallOutcome{}, false, err
		}
		alternatives = alternatives.Join(v.execution.algebra.program.registry, current)
	}
	if alternatives.Empty() {
		return callpayload.CallOutcome{}, true, nil
	}
	return alternatives.Collapse(v.execution.algebra.program.registry), true, nil
}

func (v *FormalRelationPublicationView) allocationCallOutcomeFromPublishedOutput(
	point cfg.Point,
	operation operationplan.SignatureAllocationOperation,
	output state.State,
) (callpayload.CallOutcome, error) {
	if v == nil || v.body == nil || v.body.rootAllocations == nil || operation.Site().Ordinal != uint32(point) {
		return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d has no exact transaction authority", point)
	}
	template := operation.Template()
	allocation := v.body.relation.arena.AllocationTemplate(operation)
	if !v.body.relation.arena.validAllocation(allocation) {
		return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d has no sealed allocation term", point)
	}
	node := v.body.relation.arena.allocations[allocation]
	objects := make(map[identity.ID]heapidentity.TableObject, len(template.Objects))
	placements := make(map[identity.ID]placement.Value, len(template.Objects))
	var result callpayload.CallResult
	resultExact := false
	for _, objectTemplate := range template.Objects {
		term, present := node.identities[objectTemplate.ID]
		allocationTemplate, found := term.Allocation()
		if !present || !found {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d has no symbolic object %q", point, objectTemplate.ID)
		}
		id, exact := v.body.rootAllocations.RebaseAllocation(allocationTemplate)
		if !exact {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d object %q is outside the root quotient", point, objectTemplate.ID)
		}
		object := output.ReadHeapTableObject(v.execution.algebra.program.registry, id)
		if heapidentity.ObjectDomain(v.execution.algebra.program.registry).Equal(object, heapidentity.BottomObject(v.execution.algebra.program.registry)) {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d object %q is absent from published output", point, objectTemplate.ID)
		}
		objectID, objectExact := identityvalue.ExactID(v.execution.algebra.program.registry, object.Root())
		if !objectExact || objectID != id {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d object %q has a divergent root", point, objectTemplate.ID)
		}
		placementValue := output.ReadPlacement(id)
		if placementValue == placement.Bottom {
			return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d object %q has no published placement", point, objectTemplate.ID)
		}
		objects[id], placements[id] = object, placementValue
		if objectTemplate.ID == template.Root {
			result = callpayload.CallResult{Index: template.ReturnIndex, Value: object.Root()}
			resultExact = true
		}
	}
	if !resultExact {
		return callpayload.CallOutcome{}, fmt.Errorf("transformer: allocation call point %d has no published root result", point)
	}
	out := callpayload.CallOutcome{
		Results: []callpayload.CallResult{result}, HeapTableObjects: objects, Placements: placements,
	}
	out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(v.execution.algebra.program.registry, out)
	if err := validateRelationCallOutcomeCanonicalLanes(out); err != nil {
		return callpayload.CallOutcome{}, err
	}
	return callpayload.NormalizeCallOutcome(v.execution.algebra.program.registry, out), nil
}

// Outcome names one body terminal outcome.
func (v *FormalRelationPublicationView) outcome(ref boundaryOutcomeRef) (formalPublishedCoordinate, bool) {
	if v == nil || v.execution == nil || v.execution.algebra == nil || ref == 0 || int(v.variable) > len(v.execution.algebra.program.formalRegion.outcomes) ||
		int(ref) >= len(v.execution.algebra.program.formalRegion.outcomes[v.variable-1]) {
		return formalPublishedCoordinate{}, false
	}
	cell := v.execution.algebra.program.formalRegion.outcomes[v.variable-1][ref]
	_, present := v.execution.values[cell]
	coordinate := formalPublishedCoordinate{view: v, cell: cell}
	return coordinate, present && coordinate.valid()
}

// Nonreturning names the body-level no-normal-return join.
func (v *FormalRelationPublicationView) nonreturning() (formalPublishedCoordinate, bool) {
	if v == nil || v.execution == nil || v.execution.algebra == nil || int(v.variable) > len(v.execution.algebra.program.formalRegion.nonreturning) {
		return formalPublishedCoordinate{}, false
	}
	cell := v.execution.algebra.program.formalRegion.nonreturning[v.variable-1]
	_, present := v.execution.values[cell]
	coordinate := formalPublishedCoordinate{view: v, cell: cell}
	return coordinate, present && coordinate.valid()
}

// joinDiagnosticOutput projects Care+Diagnostics directly from the stabilized
// formal coordinate. It never materializes product fibers, so a body-owned
// symbolic terminal can publish diagnostics before a concrete invocation edge
// exists.
func (v *FormalRelationPublicationView) joinDiagnosticOutput(ctx context.Context, coordinate formalPublishedCoordinate) (callpayload.DiagnosticOutput, bool, error) {
	if ctx == nil || v == nil || coordinate.view != v || !coordinate.valid() || v.execution == nil || v.execution.algebra == nil {
		return callpayload.DiagnosticOutput{}, false, fmt.Errorf("transformer: formal publication coordinate is unowned")
	}
	tuple, present := v.execution.values[coordinate.cell]
	if !present {
		return callpayload.DiagnosticOutput{}, false, fmt.Errorf("transformer: formal publication coordinate is absent")
	}
	return v.execution.algebra.formalDiagnosticOutput(ctx, tuple)
}

// formalPublicationFactorAccumulator is the correlation-forgetting boundary
// for already-projected complete leaf tuples. It is deliberately ephemeral:
// the only durable value it can publish is one composed State.
type formalPublicationFactorAccumulator struct {
	domain  state.ProductDomain
	values  state.ValueLaneFactor
	factors []state.LaneFactor
	joined  bool
}

type formalPublicationProjectionFactorKey struct {
	lane state.LaneOrdinal
	hash uint64
}

type formalPublicationProjectionFactorEntry struct {
	leaves []decisionLeaf
	factor state.LaneFactor
}

type formalPublicationProjectionValuesEntry struct {
	leaves []decisionLeaf
	values state.ValueLaneFactor
}

// formalPublicationProjectionCache belongs to one exact point inverse. It
// retains only already-projected registered factors; allocation quotienting
// still consumes every complete correlated row below.
type formalPublicationProjectionCache struct {
	factors map[formalPublicationProjectionFactorKey][]formalPublicationProjectionFactorEntry
	values  map[uint64][]formalPublicationProjectionValuesEntry
}

// formalPublicationValuesProjection removes only identity-independent Values
// slots from the correlated publication row. Their existential join is exact
// because allocation quotienting observes identity support only; slots which
// can carry an identity remain correlated with every non-Values factor.
type formalPublicationValuesProjection struct {
	collapsed map[formalFiberOrdinal]decisionLeaf
}

func (v *FormalRelationPublicationView) publicationProductProjection(
	ctx context.Context,
	tuple formalRelationTuple,
) ([]formalFiberOrdinal, formalPublicationValuesProjection, error) {
	if ctx == nil || v == nil || v.execution == nil || v.execution.algebra == nil || tuple.bottom() {
		return nil, formalPublicationValuesProjection{}, errFormalComponentForeignOwner
	}
	a := v.execution.algebra
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory || span.variable != v.variable {
		return nil, formalPublicationValuesProjection{}, errFormalComponentForeignOwner
	}
	var valuesGroup formalFiberGroupDescriptor
	for _, group := range v.groups {
		if group.kind == formalFiberGroupValues {
			valuesGroup = group
			break
		}
	}
	if !valuesGroup.valid() || valuesGroup.valueTopPosition < 0 || valuesGroup.valueTopPosition >= len(valuesGroup.members) {
		return nil, formalPublicationValuesProjection{}, errFormalComponentMalformed
	}
	care, err := a.care(tuple)
	if err != nil {
		return nil, formalPublicationValuesProjection{}, err
	}
	topOrdinal := valuesGroup.members[valuesGroup.valueTopPosition]
	topValue, err := directory.valueAt(tuple.root, topOrdinal)
	if err != nil {
		return nil, formalPublicationValuesProjection{}, err
	}
	topRoot := decisionRef(topValue)
	bottom := product.Bottom(authority.product.Registry())
	domain := product.Domain(authority.product.Registry())
	projection := formalPublicationValuesProjection{collapsed: make(map[formalFiberOrdinal]decisionLeaf)}
	excluded := make(map[formalFiberOrdinal]struct{})
	for _, slot := range valuesGroup.valueSlots {
		if slot.position < 0 || slot.position >= len(valuesGroup.members) {
			return nil, formalPublicationValuesProjection{}, errFormalComponentMalformed
		}
		ordinal := valuesGroup.members[slot.position]
		value, readErr := directory.valueAt(tuple.root, ordinal)
		if readErr != nil {
			return nil, formalPublicationValuesProjection{}, readErr
		}
		rows, partitionErr := a.decisions.partitionLeafTuplesUnderCare(ctx, care, []decisionRef{topRoot, decisionRef(value)})
		if partitionErr != nil {
			return nil, formalPublicationValuesProjection{}, partitionErr
		}
		joined := bottom
		identityBearing := false
		for _, row := range rows {
			if len(row.leaves) != 2 || row.leaves[0] > 1 || row.leaves[1] == 1 {
				return nil, formalPublicationValuesProjection{}, errDecisionMalformed
			}
			current := bottom
			if row.leaves[0] == 1 {
				current = product.Top()
			} else if row.leaves[1] != 0 {
				terminal, terminalErr := authority.terminal(row.leaves[1])
				if terminalErr != nil || terminal.kind != formalComponentGroundValue {
					if terminalErr != nil {
						return nil, formalPublicationValuesProjection{}, terminalErr
					}
					return nil, formalPublicationValuesProjection{}, errFormalComponentMalformed
				}
				current = terminal.ground
			}
			hasIdentity, identityErr := authority.product.ValueHasIdentitySupport(current)
			if identityErr != nil {
				return nil, formalPublicationValuesProjection{}, identityErr
			}
			identityBearing = identityBearing || hasIdentity
			joined = domain.Join(joined, current)
		}
		if identityBearing {
			continue
		}
		leaf := decisionLeaf(0)
		if !product.Equal(authority.product.Registry(), joined, bottom) {
			leaf, err = authority.internGroundValue(joined)
			if err != nil {
				return nil, formalPublicationValuesProjection{}, err
			}
		}
		projection.collapsed[ordinal] = leaf
		excluded[ordinal] = struct{}{}
	}
	ordinals := make([]formalFiberOrdinal, 0, len(v.ordinals)-len(excluded))
	for _, ordinal := range v.ordinals {
		if _, independent := excluded[ordinal]; !independent {
			ordinals = append(ordinals, ordinal)
		}
	}
	return ordinals, projection, nil
}

func (a *formalPublicationFactorAccumulator) join(values state.ValueLaneFactor, factors []state.LaneFactor) error {
	if a == nil || !a.domain.Valid() || len(factors) != a.domain.NonValuesLaneCount() {
		return state.ErrIncompleteLaneFactors
	}
	if !a.joined {
		a.values = values
		a.factors = factors
		a.joined = true
		return nil
	}
	if len(factors) != len(a.factors) {
		return state.ErrIncompleteLaneFactors
	}
	joinedValues, joinedFactors, err := a.domain.JoinFactorTuples(a.values, a.factors, values, factors)
	if err != nil {
		return fmt.Errorf("transformer: join published factor tuple: %w", err)
	}
	a.values, a.factors = joinedValues, joinedFactors
	return nil
}

func (a *formalPublicationFactorAccumulator) compose() (state.State, bool, error) {
	if a == nil || !a.domain.Valid() {
		return state.State{}, false, state.ErrIncompleteLaneFactors
	}
	if !a.joined {
		return a.domain.Lattice().Bottom(), false, nil
	}
	value, err := a.domain.ComposeFactorTuple(a.values, a.factors)
	return value, true, err
}

func (v *FormalRelationPublicationView) joinState(ctx context.Context, coordinate formalPublishedCoordinate) (state.State, error) {
	if ctx == nil || v == nil || coordinate.view != v || !coordinate.valid() || v.execution == nil || v.execution.algebra == nil {
		return state.State{}, fmt.Errorf("transformer: formal publication coordinate is unowned: cell=%+v view-match=%t", coordinate.cell, coordinate.view == v)
	}
	if err := ctx.Err(); err != nil {
		return state.State{}, err
	}
	if coordinate.inverseErr != nil {
		return state.State{}, coordinate.inverseErr
	}
	if !v.body.productDomain.OwnsCoordinateFormalPublicationProjection(coordinate.inverse) {
		return state.State{}, fmt.Errorf("transformer: formal publication has no exact cell inverse")
	}
	if v.execution.algebra.entrySubstitution == nil {
		return state.State{}, fmt.Errorf("transformer: formal point publication has no selected root environment")
	}
	tuple, present := v.execution.values[coordinate.cell]
	if !present || tuple.bottom() {
		return v.body.domain.Bottom(), nil
	}
	// This is the explicit body-publication abstraction: Care restricts the
	// selected root's concrete guard before any leaf is materialized. Only here
	// may correlated concrete leaves be existentially joined.
	ordinals, valuesProjection, err := v.publicationProductProjection(ctx, tuple)
	if err != nil {
		return state.State{}, err
	}
	partitions, err := v.execution.algebra.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{
		tuple: tuple, ordinals: ordinals,
	}}, nil)
	if err != nil {
		return state.State{}, err
	}
	accumulator := formalPublicationFactorAccumulator{domain: v.body.productDomain}
	cache := formalPublicationProjectionCache{
		factors: make(map[formalPublicationProjectionFactorKey][]formalPublicationProjectionFactorEntry),
		values:  make(map[uint64][]formalPublicationProjectionValuesEntry),
	}
	for _, partition := range partitions {
		if err := ctx.Err(); err != nil {
			return state.State{}, err
		}
		if len(partition.views) != 1 || partition.guard == decisionFalse {
			return state.State{}, errDecisionMalformed
		}
		leaf := partition.views[0]
		values, factors, projectErr := v.projectLeafFactorTuple(ctx, leaf, coordinate.inverse, &cache, valuesProjection)
		if projectErr != nil {
			return state.State{}, projectErr
		}
		if err := accumulator.join(values, factors); err != nil {
			return state.State{}, err
		}
	}
	concrete, joined, err := accumulator.compose()
	if err != nil {
		return state.State{}, err
	}
	if !joined {
		return v.body.domain.Bottom(), nil
	}
	return state.NormalizeForDomain(v.body.domain, concrete), nil
}

// projectLeafValues materializes exactly the selected Values group. It is the
// direct-observation counterpart of projectLeafFactorTuple: callers that need
// a scalar return or path value must not pay to rekey every retained relation
// completion lane only to discard it before a State is composed.
func (v *FormalRelationPublicationView) projectLeafValues(
	ctx context.Context,
	leaf formalSparseLeafView,
	cache *formalPublicationProjectionCache,
	valuesProjection formalPublicationValuesProjection,
) (state.ValueLaneFactor, error) {
	if cache == nil || cache.values == nil {
		return state.ValueLaneFactor{}, errFormalComponentForeignOwner
	}
	if err := ctx.Err(); err != nil {
		return state.ValueLaneFactor{}, err
	}
	for _, group := range v.groups {
		if group.kind != formalFiberGroupValues {
			continue
		}
		leaves := make([]decisionLeaf, len(group.members))
		for index, ordinal := range group.members {
			value, present := leaf.leaf(ordinal)
			if !present {
				value, present = valuesProjection.collapsed[ordinal]
			}
			if !present {
				return state.ValueLaneFactor{}, errFormalComponentMalformed
			}
			leaves[index] = value
		}
		hash := formalFactorLeafHash(leaves)
		for _, entry := range cache.values[hash] {
			if formalFactorLeavesEqual(entry.leaves, leaves) {
				return entry.values, nil
			}
		}
		formalValues, err := leaf.algebra.materializeValuesGroup(leaf.authority, group, leaves)
		if err != nil {
			return state.ValueLaneFactor{}, fmt.Errorf("transformer: publish Values factor: %w", err)
		}
		formalValues, err = formalStateValues(formalValues)
		if err != nil {
			return state.ValueLaneFactor{}, err
		}
		values, err := leaf.span.valueRekey.Apply(formalValues)
		if err != nil {
			return state.ValueLaneFactor{}, err
		}
		cache.values[hash] = append(cache.values[hash], formalPublicationProjectionValuesEntry{
			leaves: append([]decisionLeaf(nil), leaves...), values: values,
		})
		return values, nil
	}
	return state.ValueLaneFactor{}, errFormalComponentMalformed
}

// projectLeafFactorTuple projects one complete correlated decision leaf into
// the concrete point product without constructing State. Allocation quotient
// precedes the caller's componentwise fold because its universal must fibers
// depend on the complete identity support of this exact correlated leaf.
func (v *FormalRelationPublicationView) projectLeafFactorTuple(
	ctx context.Context,
	leaf formalSparseLeafView,
	inverse state.CoordinateFormalPublicationProjection,
	cache *formalPublicationProjectionCache,
	valuesProjection formalPublicationValuesProjection,
) (state.ValueLaneFactor, []state.LaneFactor, error) {
	if cache == nil || cache.factors == nil || cache.values == nil {
		return state.ValueLaneFactor{}, nil, errFormalComponentForeignOwner
	}
	span := leaf.span
	// Middle binding fibers are quantified syntax, not State lanes. The caller
	// partitions every registered product root under shared Care, which retains
	// their exact correlation while existentially eliminating binder-only
	// distinctions. A syntax-only execution still cannot publish: joinState
	// requires a selected entry substitution before this boundary is reached.
	groups := v.groups
	if len(groups) == 0 {
		return state.ValueLaneFactor{}, nil, errFormalComponentMalformed
	}
	values := state.ValueLaneFactor{}
	factors := make([]state.LaneFactor, 0, v.body.productDomain.NonValuesLaneCount())
	for _, group := range groups {
		leaves := make([]decisionLeaf, len(group.members))
		for index, ordinal := range group.members {
			value, present := leaf.leaf(ordinal)
			if !present && group.kind == formalFiberGroupValues {
				value, present = valuesProjection.collapsed[ordinal]
			}
			if !present {
				return state.ValueLaneFactor{}, nil, errFormalComponentMalformed
			}
			leaves[index] = value
		}
		switch group.kind {
		case formalFiberGroupValues:
			hash := formalFactorLeafHash(leaves)
			cached := false
			for _, entry := range cache.values[hash] {
				if formalFactorLeavesEqual(entry.leaves, leaves) {
					values, cached = entry.values, true
					break
				}
			}
			if cached {
				continue
			}
			formalValues, err := leaf.algebra.materializeValuesGroup(leaf.authority, group, leaves)
			if err != nil {
				return state.ValueLaneFactor{}, nil, fmt.Errorf("transformer: publish Values factor: %w", err)
			}
			formalValues, err = formalStateValues(formalValues)
			if err != nil {
				return state.ValueLaneFactor{}, nil, err
			}
			values, err = span.valueRekey.Apply(formalValues)
			if err != nil {
				return state.ValueLaneFactor{}, nil, err
			}
			cache.values[hash] = append(cache.values[hash], formalPublicationProjectionValuesEntry{
				leaves: append([]decisionLeaf(nil), leaves...), values: values,
			})
		case formalFiberGroupOrdinaryLane:
			key := formalPublicationProjectionFactorKey{lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
			cached := false
			for _, entry := range cache.factors[key] {
				if formalFactorLeavesEqual(entry.leaves, leaves) {
					factors = append(factors, entry.factor)
					cached = true
					break
				}
			}
			if cached {
				continue
			}
			factor, err := leaf.laneFactor(group)
			if err == nil {
				factor, err = v.body.productDomain.RekeyOrdinaryLaneFactorFormalPublication(inverse, factor)
			}
			if err != nil {
				return state.ValueLaneFactor{}, nil, fmt.Errorf("transformer: publish ordinary lane %q: %w", group.lane.ID(), err)
			}
			cache.factors[key] = append(cache.factors[key], formalPublicationProjectionFactorEntry{
				leaves: append([]decisionLeaf(nil), leaves...), factor: factor,
			})
			factors = append(factors, factor)
		case formalFiberGroupCoordinateLane:
			key := formalPublicationProjectionFactorKey{lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
			cached := false
			for _, entry := range cache.factors[key] {
				if formalFactorLeavesEqual(entry.leaves, leaves) {
					factors = append(factors, entry.factor)
					cached = true
					break
				}
			}
			if cached {
				continue
			}
			factor, err := leaf.laneFactor(group)
			if err == nil {
				factor, err = v.body.productDomain.RekeyCoordinateLaneFactorFormalPublication(inverse, factor)
			}
			if err != nil {
				return state.ValueLaneFactor{}, nil, fmt.Errorf("transformer: publish coordinate lane %q: %w", group.lane.ID(), err)
			}
			cache.factors[key] = append(cache.factors[key], formalPublicationProjectionFactorEntry{
				leaves: append([]decisionLeaf(nil), leaves...), factor: factor,
			})
			factors = append(factors, factor)
		default:
			return state.ValueLaneFactor{}, nil, errFormalComponentMalformed
		}
	}
	// The stabilized relation retains lexical allocation templates. Root
	// publication is their sole existential-elimination boundary: apply the
	// body's frozen root quotient to the complete factored product so every
	// registered identity-bearing lane receives the same deterministic image.
	values, factors, err := state.ApplyAllocationIdentityQuotientTuple(
		ctx, v.body.productDomain, v.body.keys, v.body.rootAllocations, values, factors,
	)
	if err != nil {
		return state.ValueLaneFactor{}, nil, fmt.Errorf("transformer: publish root allocation quotient: %w", err)
	}
	return values, factors, nil
}
