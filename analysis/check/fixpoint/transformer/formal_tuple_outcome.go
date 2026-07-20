package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	typecovariant "github.com/wippyai/go-lua/analysis/type/covariant"
)

// formalOutcomeStep is the sole frozen N5 terminal transaction. The relation
// outcome remains the syntax owner; this plan binds its already-sealed sources
// and Output roots to the complete registered product layout exactly once.
type formalOutcomeStep struct {
	variable            relationVar
	code                *relationCode
	root                relationRootRef
	scope               loopMuTerm
	outcome             boundaryOutcomeRef
	transaction         factapply.ReturnTransaction
	sources             []formalQualifiedBinding
	valueAccess         state.TransferInputAccess
	valueFactorGroups   []formalFiberGroupDescriptor
	valuePlan           formalValueAccessPlan
	readOrdinals        []formalFiberOrdinal
	readPositions       formalOrdinalPositions
	projectionOrdinals  []formalFiberOrdinal
	projectionPositions formalOrdinalPositions
	writeOrdinals       []formalFiberOrdinal
	writePositions      formalOrdinalPositions
	laneWriteOrdinals   []formalFiberOrdinal
	laneWritePositions  formalOrdinalPositions
	targets             []factapply.ReturnFactorTarget[FormalSlot]
	demands             []formalQualifiedGuardDemand
	lanes               []formalSelectedFactorLane
	returnLanes         []int
	returnTopology      state.ReturnFactorTopology
	covariant           factapply.CovariantExposureTransaction
	covariantBindings   []factapply.CovariantFactorBinding[FormalSlot]
	covariantLanes      []int
	covariantTopology   state.CovariantFactorTopology
	terminal            formalFiberDescriptor
	sealed              bool
}

type formalOutcomeLeafWrite struct {
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

// freezeFormalCompleteOutcomeLane binds the complete registered N5 carrier.
// N5 observes existing heap skeletons and identity edges, so a scalar-slot
// footprint cannot soundly stand in for the full family. Performance work
// must lift this carrier algebraically over decision roots, not discard it.
func freezeFormalCompleteOutcomeLane(span formalFiberDescriptorSpan, group formalFiberGroupDescriptor) (formalSelectedFactorLane, error) {
	if !group.valid() || group.variable != span.variable || group.kind == formalFiberGroupValues {
		return formalSelectedFactorLane{}, errFormalComponentForeignOwner
	}
	plan := formalSelectedFactorLane{group: group}
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		plan.ordinary = true
		plan.ordinals = append(plan.ordinals, group.members...)
	case formalFiberGroupCoordinateLane:
		for _, family := range group.coordinateFamilies {
			entry := formalSelectedCoordinateFamily{
				family: family, selected: true,
				positions: make([]int, len(family.scalars)),
				slots:     make([]state.CoordinateSlot, len(family.scalars)),
			}
			plan.ordinals = append(plan.ordinals, family.skeleton)
			for index, ordinal := range family.scalars {
				if int(ordinal) < 0 || int(ordinal) >= span.count {
					return formalSelectedFactorLane{}, errFormalComponentMalformed
				}
				descriptor := span.forest.descriptors[span.first+int(ordinal)]
				if descriptor.role != formalFiberCoordinate || descriptor.coordinateKind != formalFiberCoordinateFamilyScalar ||
					!coordinateFamilySame(descriptor.family, family.family) {
					return formalSelectedFactorLane{}, errFormalComponentMalformed
				}
				entry.positions[index] = index
				entry.slots[index] = descriptor.coordinate
				plan.ordinals = append(plan.ordinals, ordinal)
			}
			plan.families = append(plan.families, entry)
		}
	default:
		return formalSelectedFactorLane{}, errFormalComponentMalformed
	}
	sort.Slice(plan.ordinals, func(i, j int) bool { return plan.ordinals[i] < plan.ordinals[j] })
	return plan, nil
}

// formalOutcomeLeafOutput is the exact declared publication surface of N5/N6.
// assigned is separate from leaves because physical zero is semantic Bottom,
// not an unwritten sentinel.
type formalOutcomeLeafOutput struct {
	ordinals  []formalFiberOrdinal
	positions formalOrdinalPositions
	leaves    []decisionLeaf
	assigned  []bool
}

func newFormalOutcomeLeafOutput(ordinals []formalFiberOrdinal, positions formalOrdinalPositions) (formalOutcomeLeafOutput, error) {
	if len(ordinals) == 0 || len(positions.positions) == 0 || !positions.validFor(len(positions.positions), ordinals) {
		return formalOutcomeLeafOutput{}, errFormalComponentMalformed
	}
	for index, ordinal := range ordinals {
		position, present := positions.position(ordinal)
		if index > 0 && ordinals[index-1] >= ordinal || !present || position != index {
			return formalOutcomeLeafOutput{}, errFormalComponentMalformed
		}
	}
	return formalOutcomeLeafOutput{
		ordinals:  ordinals,
		positions: positions,
		leaves:    make([]decisionLeaf, len(ordinals)),
		assigned:  make([]bool, len(ordinals)),
	}, nil
}

func (o *formalOutcomeLeafOutput) set(ordinal formalFiberOrdinal, leaf decisionLeaf) error {
	if o == nil {
		return errFormalComponentMalformed
	}
	index, present := o.positions.position(ordinal)
	if !present || index >= len(o.ordinals) || o.ordinals[index] != ordinal || o.assigned[index] {
		return fmt.Errorf("transformer: formal Outcome write ordinal %d is undeclared or duplicated", ordinal)
	}
	o.leaves[index] = leaf
	o.assigned[index] = true
	return nil
}

func (o *formalOutcomeLeafOutput) setGroup(group formalFiberGroupDescriptor, leaves []decisionLeaf) error {
	if o == nil || len(group.members) != len(leaves) {
		return errFormalComponentMalformed
	}
	for index, ordinal := range group.members {
		if err := o.set(ordinal, leaves[index]); err != nil {
			return err
		}
	}
	return nil
}

func (o formalOutcomeLeafOutput) complete() ([]decisionLeaf, error) {
	if len(o.ordinals) != len(o.leaves) || len(o.leaves) != len(o.assigned) {
		return nil, errFormalComponentMalformed
	}
	for index, assigned := range o.assigned {
		if !assigned {
			return nil, fmt.Errorf("transformer: formal Outcome omitted declared write ordinal %d", o.ordinals[index])
		}
	}
	return o.leaves, nil
}

// sparseFormalOutcomeLeafWrites returns only the physical fibers changed by
// one correlated N5/N6 transaction region. The predecessor DD is the
// canonical structural carry for every omitted fiber; conditioning an
// unchanged leaf would rebuild the same function through a redundant ITE.
func sparseFormalOutcomeLeafWrites(view formalSparseLeafView, ordinals []formalFiberOrdinal, leaves []decisionLeaf) ([]formalOutcomeLeafWrite, error) {
	if len(ordinals) != len(leaves) {
		return nil, errFormalComponentMalformed
	}
	writes := make([]formalOutcomeLeafWrite, 0, len(leaves))
	for index, ordinal := range ordinals {
		prior, present := view.leaf(ordinal)
		if !present {
			return nil, errFormalComponentMalformed
		}
		if prior != leaves[index] {
			writes = append(writes, formalOutcomeLeafWrite{ordinal: ordinal, leaf: leaves[index]})
		}
	}
	return writes, nil
}

func (p *formalOutcomeStep) valid(operator formalRelationOperatorRef) bool {
	return p != nil && p.sealed && p.variable != 0 && p.code == operator.code && p.root == operator.root &&
		p.scope == operator.scope && p.outcome == operator.outcome && p.transaction.Valid() && p.terminal.role == formalFiberOutcome &&
		p.terminal.outcome == p.outcome
}

func freezeFormalOutcomeStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalOutcomeStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellOutcome ||
		operator.code == nil || operator.root == 0 || int(operator.root) >= len(operator.code.nodes) ||
		operator.outcome == 0 || int(operator.outcome) >= len(operator.code.outcomes) {
		return nil, fmt.Errorf("Outcome has no sealed relation ownership")
	}
	body := &program.bodies[variable-1]
	if body.variable != variable || body.relation.code != operator.code || body.returns == nil || !body.returns.Valid() {
		return nil, fmt.Errorf("Outcome has no canonical N5 authority")
	}
	node := operator.code.nodes[operator.root]
	if node.kind != relationNodeOutcome || node.outcome != operator.outcome {
		return nil, fmt.Errorf("Outcome occurrence does not own its declared result")
	}
	outcome := operator.code.outcomes[operator.outcome]
	if !outcome.returnTransaction.valid(operator.code.terms, operator.code.shape) {
		return nil, fmt.Errorf("Outcome has no sealed N5 source transaction")
	}
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() {
		return nil, fmt.Errorf("Outcome has no formal product schema")
	}
	valuesGroup, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("Outcome has no complete Values group")
	}
	returnTopology, err := body.productDomain.SealReturnFactorTopology()
	if err != nil {
		return nil, fmt.Errorf("Outcome N5 factor topology: %w", err)
	}
	var covariantTopology state.CovariantFactorTopology
	covariantActive := outcome.covariant.HasStateSteps()
	if covariantActive {
		covariantTopology, err = body.productDomain.SealCovariantFactorTopology()
		if err != nil {
			return nil, fmt.Errorf("Outcome N6 factor topology: %w", err)
		}
	}
	selected := make(map[state.LaneOrdinal]bool, returnTopology.Len()+covariantTopology.Len())
	for index := 0; index < returnTopology.Len(); index++ {
		lane, _ := returnTopology.Lane(index)
		selected[lane.Ordinal()] = true
	}
	for index := 0; covariantActive && index < covariantTopology.Len(); index++ {
		lane, _ := covariantTopology.Lane(index)
		selected[lane.Ordinal()] = true
	}
	var unionLanes []state.ProductLane
	for _, lane := range body.productDomain.NonValuesLaneInventory() {
		if selected[lane.Ordinal()] {
			unionLanes = append(unionLanes, lane)
		}
	}
	lanes := make([]formalSelectedFactorLane, len(unionLanes))
	groups := span.groupDescriptors()
	for index := range lanes {
		lane := unionLanes[index]
		var group formalFiberGroupDescriptor
		found := false
		for _, candidate := range groups {
			if candidate.kind != formalFiberGroupValues && candidate.lane == lane {
				group, found = candidate, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("Outcome N5 lane %q is outside the formal product", lane.ID())
		}
		lanes[index], err = freezeFormalCompleteOutcomeLane(span, group)
		if err != nil {
			return nil, fmt.Errorf("Outcome lane %q carrier: %w", lane.ID(), err)
		}
	}
	positions := func(length int, at func(int) (state.ProductLane, bool)) ([]int, error) {
		out := make([]int, length)
		for index := range out {
			lane, ok := at(index)
			if !ok {
				return nil, fmt.Errorf("incomplete factor topology")
			}
			out[index] = -1
			for unionIndex, union := range unionLanes {
				if union == lane {
					out[index] = unionIndex
					break
				}
			}
			if out[index] < 0 {
				return nil, fmt.Errorf("lane %q is outside terminal factor union", lane.ID())
			}
		}
		return out, nil
	}
	returnLanes, err := positions(returnTopology.Len(), returnTopology.Lane)
	if err != nil {
		return nil, fmt.Errorf("Outcome N5 topology: %w", err)
	}
	covariantLanes, err := positions(covariantTopology.Len(), covariantTopology.Lane)
	if err != nil {
		return nil, fmt.Errorf("Outcome N6 topology: %w", err)
	}
	sources := make([]formalQualifiedBinding, len(outcome.returnTransaction.sources))
	var demands []formalQualifiedGuardDemand
	for index, term := range outcome.returnTransaction.sources {
		sources[index] = formalQualifiedBinding{
			value: relationArenaValueRef{owner: variable, arena: operator.code.terms, term: term},
			scope: operator.scope,
		}
		guards, err := reachableValueTermGuards(operator.code.terms, term)
		if err != nil {
			return nil, fmt.Errorf("Outcome source %d guards: %w", index, err)
		}
		for _, guard := range guards {
			demands = append(demands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard})
		}
	}
	valueAccess, valueFactorGroups, err := freezeFormalValueFactorAccess(program, variable, outcome.returnTransaction.sources...)
	if err != nil {
		return nil, fmt.Errorf("Outcome N5 value access: %w", err)
	}
	var terminal formalFiberDescriptor
	terminalFound := false
	for _, descriptor := range span.descriptors() {
		if descriptor.role != formalFiberOutcome || descriptor.outcome != operator.outcome {
			continue
		}
		if terminalFound {
			return nil, fmt.Errorf("Outcome has duplicate terminal fibers")
		}
		terminal, terminalFound = descriptor, true
	}
	if !terminalFound {
		return nil, fmt.Errorf("Outcome has no terminal fiber")
	}
	if _, ok := span.ordinal(terminal); !ok {
		return nil, fmt.Errorf("Outcome terminal is outside the formal product")
	}
	targets := make([]factapply.ReturnFactorTarget[FormalSlot], outcome.returnTransaction.transaction.ResultTargetCount())
	for index := range targets {
		target, ok := outcome.returnTransaction.transaction.ResultTarget(index)
		if !ok || target < 0 || uint32(target) >= body.relation.Shape().Results {
			return nil, fmt.Errorf("Outcome result target %d is outside Shape.Results", target)
		}
		slot, ok := program.formalSlots.Slot(body.body, Root{Kind: RootResult, Index: uint32(target)})
		if !ok {
			return nil, fmt.Errorf("Outcome result target %d has no formal Output slot", target)
		}
		concrete := body.keys.FromPath(pathdom.Path{Root: fmt.Sprintf("ret[%d]", target)})
		path, err := body.productDomain.RekeyStructuralKeyFormal(span.rekey, concrete)
		if err != nil || path.Kind == keyspace.KindInvalid {
			if err == nil {
				err = fmt.Errorf("invalid formal Output path")
			}
			return nil, fmt.Errorf("Outcome result target %d: %w", target, err)
		}
		targets[index] = factapply.ReturnFactorTarget[FormalSlot]{Index: target, Slot: slot, Path: path}
	}
	var covariantBindings []factapply.CovariantFactorBinding[FormalSlot]
	if covariantActive {
		covariantBindings, err = freezeFormalCovariantBindings(program, variable, span, outcome.covariant)
		if err != nil {
			return nil, fmt.Errorf("Outcome N6 bindings: %w", err)
		}
	}
	valueReadSlots, err := freezeFormalValueTermSlots(program, variable, outcome.returnTransaction.sources...)
	if err != nil {
		return nil, fmt.Errorf("Outcome N5 Values access: %w", err)
	}
	valueWriteSlots := make([]FormalSlot, 0, len(targets)+len(covariantBindings))
	for _, target := range targets {
		valueWriteSlots = append(valueWriteSlots, target.Slot)
	}
	for _, binding := range covariantBindings {
		if binding.Kind == factapply.CovariantFactorBindingNoop {
			continue
		}
		valueReadSlots = append(valueReadSlots, binding.Source)
		valueWriteSlots = append(valueWriteSlots, binding.Source)
	}
	valuePlan, err := sealFormalValueAccessPlan(valuesGroup, valueReadSlots, valueWriteSlots)
	if err != nil {
		return nil, fmt.Errorf("Outcome Values capability: %w", err)
	}
	sealOrdinals := func(input []formalFiberOrdinal) []formalFiberOrdinal {
		seen := make(map[formalFiberOrdinal]struct{}, len(input))
		for _, ordinal := range input {
			seen[ordinal] = struct{}{}
		}
		out := make([]formalFiberOrdinal, 0, len(seen))
		for ordinal := range seen {
			out = append(out, ordinal)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}
	laneWriteOrdinals := make([]formalFiberOrdinal, 0)
	for _, lane := range lanes {
		laneWriteOrdinals = append(laneWriteOrdinals, lane.ordinals...)
	}
	laneWriteOrdinals = sealOrdinals(laneWriteOrdinals)
	laneWritePositions, err := sealFormalOrdinalPositions(span.count, laneWriteOrdinals)
	if err != nil {
		return nil, fmt.Errorf("Outcome lane publication positions: %w", err)
	}
	writeOrdinals := append(append([]formalFiberOrdinal(nil), valuePlan.writeOrdinals...), laneWriteOrdinals...)
	writeOrdinals = sealOrdinals(writeOrdinals)
	writePositions, err := sealFormalOrdinalPositions(span.count, writeOrdinals)
	if err != nil {
		return nil, fmt.Errorf("Outcome publication positions: %w", err)
	}
	readOrdinals := append([]formalFiberOrdinal(nil), valuePlan.readOrdinals...)
	for _, group := range valueFactorGroups {
		readOrdinals = append(readOrdinals, group.members...)
	}
	readOrdinals = append(readOrdinals, laneWriteOrdinals...)
	readOrdinals = sealOrdinals(readOrdinals)
	readPositions, err := sealFormalOrdinalPositions(span.count, readOrdinals)
	if err != nil {
		return nil, fmt.Errorf("Outcome read positions: %w", err)
	}
	// Write-only Values coordinates are physical outputs, not semantic inputs.
	// Give the DAG-native transaction a fixed output address for them without
	// adding their predecessor roots to the correlated input cone.
	projectionOrdinals := append(append([]formalFiberOrdinal(nil), readOrdinals...), writeOrdinals...)
	projectionOrdinals = sealOrdinals(projectionOrdinals)
	projectionPositions, err := sealFormalOrdinalPositions(span.count, projectionOrdinals)
	if err != nil {
		return nil, fmt.Errorf("Outcome projection positions: %w", err)
	}
	return &formalOutcomeStep{
		variable: variable, code: operator.code, root: operator.root, scope: operator.scope, outcome: operator.outcome,
		transaction: outcome.returnTransaction.transaction.Clone(), sources: sources,
		valueAccess: valueAccess, valueFactorGroups: valueFactorGroups, valuePlan: valuePlan, targets: targets, demands: demands,
		readOrdinals: readOrdinals, readPositions: readPositions,
		projectionOrdinals: projectionOrdinals, projectionPositions: projectionPositions,
		writeOrdinals: writeOrdinals, writePositions: writePositions,
		laneWriteOrdinals: laneWriteOrdinals, laneWritePositions: laneWritePositions,
		lanes: lanes, returnLanes: returnLanes, returnTopology: returnTopology,
		covariant: outcome.covariant.Clone(), covariantBindings: covariantBindings,
		covariantLanes: covariantLanes, covariantTopology: covariantTopology,
		terminal: terminal, sealed: true,
	}, nil
}

// projectOutcome completes canonical N5 before terminal occurrence
// publication. Reachability is frozen from that stabilized product tuple, so
// Apply never re-evaluates return syntax or reconstructs State.
func (a *formalTupleAlgebra) projectOutcome(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || !operator.outcomeTransaction.valid(operator) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Outcome has no complete N5 transaction")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() {
		return predecessor, nil
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || authority.code != operator.code || predecessor.variable != operator.outcomeTransaction.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Outcome predecessor has foreign ownership")
	}
	care, err := a.care(predecessor)
	if err != nil || care == decisionFalse {
		return formalRelationTuple{}, err
	}
	plan := operator.outcomeTransaction
	if a.evalTrace != nil && a.evalTrace.active != nil {
		detail := a.evalTrace.active
		detail.outcomePlan = plan
		detail.outcomeReadRoots += len(plan.readOrdinals)
		roots := make(map[decisionRef]struct{}, len(plan.readOrdinals))
		variables := make(map[uint32]struct{})
		for _, ordinal := range plan.readOrdinals {
			value, readErr := directory.valueAt(predecessor.root, ordinal)
			if readErr != nil {
				return formalRelationTuple{}, readErr
			}
			root := decisionRef(value)
			roots[root] = struct{}{}
			if int(root) < len(a.decisions.nodes) && !a.decisions.nodes[root].terminal {
				detail.outcomeNonterminalRoots++
				variables[a.decisions.nodes[root].variable] = struct{}{}
			}
		}
		detail.outcomeDistinctRoots += len(roots)
		detail.outcomeDistinctTopVariables += len(variables)
		seen := make(map[decisionRef]struct{}, len(roots))
		stack := make([]decisionRef, 0, len(roots))
		for root := range roots {
			stack = append(stack, root)
		}
		for len(stack) != 0 {
			last := len(stack) - 1
			root := stack[last]
			stack = stack[:last]
			if _, present := seen[root]; present || int(root) >= len(a.decisions.nodes) {
				continue
			}
			seen[root] = struct{}{}
			node := a.decisions.nodes[root]
			if node.terminal {
				continue
			}
			variables[node.variable] = struct{}{}
			stack = append(stack, node.low, node.high)
		}
		detail.outcomeSupportNodes += len(seen)
		detail.outcomeSupportVariables += len(variables)
		detail.outcomeSupportOrdinals = make(map[uint32][]formalFiberOrdinal, len(variables))
		for _, ordinal := range plan.readOrdinals {
			value, _ := directory.valueAt(predecessor.root, ordinal)
			localSeen := make(map[decisionRef]struct{})
			local := []decisionRef{decisionRef(value)}
			localVariables := make(map[uint32]struct{})
			for len(local) != 0 {
				last := len(local) - 1
				root := local[last]
				local = local[:last]
				if _, present := localSeen[root]; present || int(root) >= len(a.decisions.nodes) {
					continue
				}
				localSeen[root] = struct{}{}
				node := a.decisions.nodes[root]
				if !node.terminal {
					localVariables[node.variable] = struct{}{}
					local = append(local, node.low, node.high)
				}
			}
			for rank := range localVariables {
				detail.outcomeSupportOrdinals[rank] = append(detail.outcomeSupportOrdinals[rank], ordinal)
			}
		}
		for rank := range variables {
			detail.outcomeSupportRanks = append(detail.outcomeSupportRanks, rank)
		}
		sort.Slice(detail.outcomeSupportRanks, func(i, j int) bool { return detail.outcomeSupportRanks[i] < detail.outcomeSupportRanks[j] })
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	projectionRoots := make([]decisionRef, len(plan.projectionOrdinals))
	for index, ordinal := range plan.projectionOrdinals {
		// A write-only coordinate is deliberately represented by a constant in
		// the input vector. Its actual predecessor root remains structural carry
		// and is restored once below. It therefore cannot enlarge the semantic
		// product traversal merely because the old physical value varies.
		if _, read := plan.readPositions.position(ordinal); !read {
			projectionRoots[index] = decisionFalse
			continue
		}
		prior, readErr := directory.valueAt(predecessor.root, ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		projectionRoots[index] = decisionRef(prior)
	}
	demandRoots := make([]decisionRef, len(plan.demands))
	for index, demand := range plan.demands {
		demandRoots[index], err = a.decisionForGuard(demand.owner, demand.scope, demand.arena, demand.guard)
		if err != nil {
			return fail(err)
		}
	}
	transactionRoots := make([]decisionRef, 0, len(projectionRoots)+len(demandRoots)+len(plan.writeOrdinals))
	transactionRoots = append(transactionRoots, projectionRoots...)
	transactionRoots = append(transactionRoots, demandRoots...)
	assignmentOffset := len(transactionRoots)
	transactionRoots = append(transactionRoots, make([]decisionRef, len(plan.writeOrdinals))...)
	transformedRoots, err := a.decisions.applyVectorUnderCare(
		a.ctx, care, care, decisionFalse, transactionRoots, transactionRoots,
		func(input, unreachable []decisionLeaf) ([]decisionLeaf, error) {
			if len(input) != len(transactionRoots) || len(unreachable) != 0 {
				return nil, errDecisionMalformed
			}
			regionGuard := decisionTrue
			for index, root := range demandRoots {
				leaf := input[len(plan.projectionOrdinals)+index]
				if leaf > 1 {
					return nil, errDecisionMalformed
				}
				literal := root
				if leaf == 0 {
					var literalErr error
					literal, literalErr = formalDecisionBooleanNot(a, root)
					if literalErr != nil {
						return nil, literalErr
					}
				}
				var guardErr error
				regionGuard, guardErr = a.decisions.apply(a.ctx, uint8(decisionAnd), true, regionGuard, literal, decisionLeafAnd)
				if guardErr != nil {
					return nil, guardErr
				}
			}
			readLeaves := make([]decisionLeaf, len(plan.readOrdinals))
			for index, ordinal := range plan.readOrdinals {
				position, present := plan.projectionPositions.position(ordinal)
				if !present || position >= len(input) {
					return nil, errFormalComponentMalformed
				}
				readLeaves[index] = input[position]
			}
			view := formalSparseLeafView{
				algebra: a, variable: predecessor.variable, span: span, authority: authority,
				body: &a.program.bodies[predecessor.variable-1], guard: regionGuard,
				ordinals: plan.readOrdinals, positions: plan.readPositions, leaves: readLeaves,
				derived: input[len(plan.projectionOrdinals):assignmentOffset],
			}
			writes, leafErr := a.applyFormalOutcomeLeaf(plan, view)
			if leafErr != nil {
				return nil, leafErr
			}
			if a.evalTrace != nil && a.evalTrace.active != nil {
				a.evalTrace.active.outcomeRegions++
				a.evalTrace.active.outcomeWrites += len(writes)
			}
			output := append([]decisionLeaf(nil), input...)
			for _, write := range writes {
				position, present := plan.projectionPositions.position(write.ordinal)
				if !present || position >= len(output) {
					return nil, errFormalComponentMalformed
				}
				writePosition, writable := plan.writePositions.position(write.ordinal)
				if !writable || assignmentOffset+writePosition >= len(output) {
					return nil, errFormalComponentMalformed
				}
				output[position] = write.leaf
				output[assignmentOffset+writePosition] = decisionLeaf(decisionTrue)
			}
			return output, nil
		},
	)
	if err != nil || len(transformedRoots) != len(transactionRoots) {
		if err == nil {
			err = errDecisionMalformed
		}
		return fail(err)
	}
	publication := make([]formalFiberWrite, 0, len(plan.writeOrdinals))
	for writeIndex, ordinal := range plan.writeOrdinals {
		position, present := plan.projectionPositions.position(ordinal)
		if !present || position >= len(transformedRoots) {
			return fail(errFormalComponentMalformed)
		}
		prior, readErr := directory.valueAt(predecessor.root, ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		assignment := transformedRoots[assignmentOffset+writeIndex]
		root, conditionErr := a.decisions.condition(a.ctx, assignment, transformedRoots[position], decisionRef(prior))
		if conditionErr != nil {
			return fail(conditionErr)
		}
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, root); err != nil {
			return fail(err)
		}
		if formalFiberValue(root) != prior {
			publication = append(publication, formalFiberWrite{ordinal: ordinal, value: formalFiberValue(root)})
		}
	}
	completed := predecessor
	if len(publication) != 0 {
		delta, sealErr := directory.sealDelta(publication)
		if sealErr != nil {
			return fail(sealErr)
		}
		root, _, applyErr := directory.applyDelta(predecessor.root, delta)
		if applyErr != nil {
			return fail(applyErr)
		}
		completed = a.normalize(formalRelationTuple{variable: predecessor.variable, root: root})
	}
	leaf, err := authority.internOutcomeOccurrence(formalQualifiedOutcomeOccurrence{
		code: operator.code, ref: operator.outcome, root: operator.root, scope: operator.scope,
	})
	if err != nil {
		return fail(err)
	}
	return a.writeScalar(completed, plan.terminal, a.decisions.terminal(leaf))
}

func (a *formalTupleAlgebra) applyFormalOutcomeLeaf(plan *formalOutcomeStep, view formalSparseLeafView) ([]formalOutcomeLeafWrite, error) {
	// The N5/N6 authorities consume only the frozen read projection. Missing
	// inputs remain absent and therefore fail closed at the evaluator seam.
	evaluator, err := a.newSparseTupleLeafEvaluator(view)
	if err != nil {
		return nil, err
	}
	values, err := plan.valuePlan.materialize(view)
	if err != nil {
		return nil, fmt.Errorf("transformer: formal N5 input Values ownership: %w", err)
	}
	capability, err := evaluator.materializeValueFactorAccess(plan.valueAccess, plan.valueFactorGroups)
	if err != nil {
		return nil, fmt.Errorf("transformer: formal N5 value factors: %w", err)
	}
	sources := make([]product.Value, len(plan.sources))
	for index, binding := range plan.sources {
		evaluated, evalErr := evaluator.evaluateWithFactorAccess(binding, capability)
		if evalErr != nil {
			return nil, fmt.Errorf("transformer: formal N5 source %d: %w", index, evalErr)
		}
		sources[index] = evaluated.value
	}
	returnFactors := make([]factapply.ReturnFactorLane, len(plan.returnLanes))
	for index, position := range plan.returnLanes {
		returnFactors[index], err = a.materializeFormalOutcomeLane(evaluator, plan.lanes[position])
		if err != nil {
			return nil, fmt.Errorf("transformer: formal N5 input lane %s ownership: %w", plan.lanes[position].group.lane.ID(), err)
		}
	}
	result, err := factapply.ApplyReturnFactorTransaction(a.ctx, evaluator.body.returns, factapply.ReturnFactorTransaction[FormalSlot]{
		Return: plan.transaction, Sources: sources, Targets: plan.targets, Values: values, Lanes: returnFactors,
		Domain: evaluator.authority.product, Keys: evaluator.span.keys, Topology: plan.returnTopology,
	})
	if err != nil {
		return nil, fmt.Errorf("transformer: formal N5 return transaction: %w", err)
	}
	values = result.Values
	returnByPosition := make(map[int]factapply.ReturnFactorLane, len(plan.returnLanes))
	for index, position := range plan.returnLanes {
		returnByPosition[position] = result.Lanes[index]
	}
	covariantByPosition := make(map[int]state.LaneFactor, len(plan.covariantLanes))
	if plan.covariant.HasStateSteps() {
		covariantFactors := make([]state.LaneFactor, len(plan.covariantLanes))
		view := formalOutcomeEvaluatorView(a, evaluator)
		for index, position := range plan.covariantLanes {
			if returned, ok := returnByPosition[position]; ok {
				covariantFactors[index], err = composeFormalOutcomeLane(evaluator.authority.product, evaluator.span.keys, returned)
			} else {
				covariantFactors[index], err = a.materializeFormalSelectedLane(view, plan.lanes[position])
			}
			if err != nil {
				return nil, fmt.Errorf("transformer: formal N6 input lane %s ownership: %w", plan.lanes[position].group.lane.ID(), err)
			}
		}
		covariant, covariantErr := factapply.ApplyCovariantExposureFactors(a.ctx, typecovariant.WidenRecord,
			factapply.CovariantFactorTransaction[FormalSlot]{
				Transaction: plan.covariant, Bindings: plan.covariantBindings, Values: values, Factors: covariantFactors,
				Domain: evaluator.authority.product, Keys: evaluator.span.keys, Topology: plan.covariantTopology,
				Token: cancellation.FromContext(a.ctx).Token(),
			})
		if covariantErr != nil {
			return nil, fmt.Errorf("transformer: formal N5 covariant transaction: %w", covariantErr)
		}
		values = covariant.Values
		for index, position := range plan.covariantLanes {
			covariantByPosition[position] = covariant.Factors[index]
		}
	}
	valueWrites, err := plan.valuePlan.factorPublication(view, values)
	if err != nil {
		return nil, fmt.Errorf("transformer: formal N5 Values publication ownership: %w", err)
	}
	writes := make([]formalOutcomeLeafWrite, 0, len(valueWrites)+len(plan.laneWriteOrdinals))
	for _, write := range valueWrites {
		writes = append(writes, formalOutcomeLeafWrite{ordinal: write.ordinal, leaf: write.leaf})
	}
	var out formalOutcomeLeafOutput
	if len(plan.laneWriteOrdinals) != 0 {
		out, err = newFormalOutcomeLeafOutput(plan.laneWriteOrdinals, plan.laneWritePositions)
		if err != nil {
			return nil, err
		}
	}
	for index, lane := range plan.lanes {
		var factor state.LaneFactor
		if covariantFactor, ok := covariantByPosition[index]; ok {
			factor = covariantFactor
		} else {
			returned, ok := returnByPosition[index]
			if !ok {
				return nil, fmt.Errorf("transformer: formal Outcome lane %s has no transaction owner", lane.group.lane.ID())
			}
			var factorErr error
			factor, factorErr = composeFormalOutcomeLane(evaluator.authority.product, evaluator.span.keys, returned)
			if factorErr != nil {
				return nil, factorErr
			}
		}
		outputs, factorErr := a.factorFormalSelectedLane(evaluator.authority, evaluator.span, lane, factor)
		if factorErr != nil {
			return nil, fmt.Errorf("transformer: formal Outcome lane %s publication ownership: %w", lane.group.lane.ID(), factorErr)
		}
		for _, write := range outputs {
			if err := out.set(write.ordinal, write.leaf); err != nil {
				return nil, err
			}
		}
	}
	if len(plan.laneWriteOrdinals) != 0 {
		leaves, completeErr := out.complete()
		if completeErr != nil {
			return nil, completeErr
		}
		laneWrites, sparseErr := sparseFormalOutcomeLeafWrites(view, plan.laneWriteOrdinals, leaves)
		if sparseErr != nil {
			return nil, sparseErr
		}
		writes = append(writes, laneWrites...)
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].ordinal < writes[j].ordinal })
	for index := 1; index < len(writes); index++ {
		if writes[index-1].ordinal >= writes[index].ordinal {
			return nil, errFormalComponentMalformed
		}
	}
	return writes, nil
}

func formalOutcomeEvaluatorView(a *formalTupleAlgebra, evaluator formalTupleLeafEvaluator) formalSparseLeafView {
	return formalSparseLeafView{
		algebra: a, variable: evaluator.variable, span: evaluator.span, authority: evaluator.authority,
		body: evaluator.body, guard: evaluator.guard,
		ordinals: evaluator.leaves.ordinals, positions: evaluator.leaves.positions, leaves: evaluator.leaves.leaves,
	}
}

func (a *formalTupleAlgebra) materializeFormalOutcomeLane(evaluator formalTupleLeafEvaluator, lane formalSelectedFactorLane) (factapply.ReturnFactorLane, error) {
	out := factapply.ReturnFactorLane{Lane: lane.group.lane}
	factor, err := a.materializeFormalSelectedLane(formalOutcomeEvaluatorView(a, evaluator), lane)
	if err != nil {
		return factapply.ReturnFactorLane{}, err
	}
	families, err := evaluator.authority.product.CoordinateFamilies(lane.group.lane)
	if err != nil {
		return factapply.ReturnFactorLane{}, err
	}
	if len(families) == 0 {
		out.Ordinary = factor
		return out, nil
	}
	out.Families = make([]state.CoordinateFamilyFactor, len(families))
	for familyIndex, family := range families {
		skeleton, scalars, decomposeErr := evaluator.authority.product.DecomposeCoordinateFamily(factor, family, evaluator.span.keys)
		if decomposeErr != nil {
			return factapply.ReturnFactorLane{}, decomposeErr
		}
		sealed, sealErr := evaluator.authority.product.SealCoordinateFamilyFactor(skeleton, scalars)
		if sealErr != nil {
			return factapply.ReturnFactorLane{}, sealErr
		}
		out.Families[familyIndex] = sealed
	}
	return out, nil
}

func composeFormalOutcomeLane(domain state.ProductDomain, keys *keyspace.KeySpace, lane factapply.ReturnFactorLane) (state.LaneFactor, error) {
	if len(lane.Families) == 0 {
		return lane.Ordinary, nil
	}
	skeletons := make([]state.CoordinateFamilySkeleton, len(lane.Families))
	scalars := make([][]state.CoordinateScalarFactor, len(lane.Families))
	for index, family := range lane.Families {
		skeletons[index] = family.Skeleton()
		scalars[index] = family.Scalars()
	}
	return domain.ComposeCoordinateFamilies(lane.Lane, keys, skeletons, scalars)
}
