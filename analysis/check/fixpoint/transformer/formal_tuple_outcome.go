package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
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
	variable          relationVar
	code              *relationCode
	root              relationRootRef
	scope             loopMuTerm
	outcome           boundaryOutcomeRef
	transaction       factapply.ReturnTransaction
	sources           []formalQualifiedBinding
	valueAccess       state.TransferInputAccess
	valueFactorGroups []formalFiberGroupDescriptor
	bindingValues     formalValueAccessPlan
	bindingLift       formalClosedFactorLift
	bindingActive     bool
	container         formalCoordinateFamilyFiberGroup
	hasContainer      bool
	identity          formalReturnIdentityStep
	targets           []factapply.ReturnFactorTarget[FormalSlot]
	demands           []formalQualifiedGuardDemand
	presence          formalCoordinateFamilyFiberGroup
	presenceValues    formalValueAccessPlan
	presenceLift      formalClosedFactorLift
	presenceActive    bool
	covariant         factapply.CovariantExposureTransaction
	covariantBindings []factapply.CovariantFactorBinding[FormalSlot]
	covariantValues   formalValueAccessPlan
	covariantGroups   []formalFiberGroupDescriptor
	covariantLift     formalClosedFactorLift
	covariantTopology state.CovariantFactorTopology
	terminal          formalFiberDescriptor
	sealed            bool
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
	groups := span.groupDescriptors()
	findFamily := func(target state.CoordinateFamily) (formalCoordinateFamilyFiberGroup, bool) {
		for _, group := range groups {
			for _, family := range group.coordinateFamilies {
				if coordinateFamilySame(family.family, target) {
					return family, true
				}
			}
		}
		return formalCoordinateFamilyFiberGroup{}, false
	}
	unionOrdinals := func(input []formalFiberOrdinal) []formalFiberOrdinal {
		sort.Slice(input, func(i, j int) bool { return input[i] < input[j] })
		write := 0
		for _, ordinal := range input {
			if write == 0 || input[write-1] != ordinal {
				input[write] = ordinal
				write++
			}
		}
		return input[:write]
	}
	var err error
	var covariantTopology state.CovariantFactorTopology
	covariantActive := outcome.covariant.HasStateSteps()
	if covariantActive {
		covariantTopology, err = body.productDomain.SealCovariantFactorTopology()
		if err != nil {
			return nil, fmt.Errorf("Outcome N6 factor topology: %w", err)
		}
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
	valueReadSlots, err := freezeFormalValueTermSlots(program, variable, outcome.returnTransaction.sources...)
	if err != nil {
		return nil, fmt.Errorf("Outcome N5 Values access: %w", err)
	}
	valueWriteSlots := make([]FormalSlot, 0, len(targets))
	for _, target := range targets {
		valueWriteSlots = append(valueWriteSlots, target.Slot)
	}
	bindingValues, err := sealFormalValueAccessPlan(valuesGroup, valueReadSlots, valueWriteSlots)
	if err != nil {
		return nil, fmt.Errorf("Outcome binding Values capability: %w", err)
	}
	bindingReads := append([]formalFiberOrdinal(nil), bindingValues.readOrdinals...)
	for _, group := range valueFactorGroups {
		bindingReads = append(bindingReads, group.members...)
	}
	var container formalCoordinateFamilyFiberGroup
	hasContainer := false
	for index := 0; index < outcome.returnTransaction.transaction.ResultBindingCount(); index++ {
		projects, ok := outcome.returnTransaction.transaction.ResultBindingProjectsHeap(index)
		if !ok {
			return nil, fmt.Errorf("Outcome result binding %d is malformed", index)
		}
		if projects {
			hasContainer = true
		}
	}
	if hasContainer {
		owner, ok := body.productDomain.ReturnIdentityContainerFamily()
		if !ok {
			return nil, fmt.Errorf("Outcome projected binding has no container family")
		}
		container, ok = findFamily(owner)
		if !ok {
			return nil, fmt.Errorf("Outcome container family is outside the formal product")
		}
		bindingReads = append(bindingReads, container.skeleton)
		bindingReads = append(bindingReads, container.scalars...)
	}
	bindingActive := outcome.returnTransaction.transaction.ResultBindingCount() != 0
	var bindingLift formalClosedFactorLift
	if bindingActive {
		bindingReads = unionOrdinals(bindingReads)
		bindingLift, err = sealFormalClosedFactorLift(span, [][]formalFiberOrdinal{bindingReads}, bindingValues.writeOrdinals)
		if err != nil {
			return nil, fmt.Errorf("Outcome binding lift: %w", err)
		}
	}

	identityPlan, err := freezeFormalReturnIdentityStep(body.productDomain, span)
	if err != nil {
		return nil, fmt.Errorf("Outcome identity closure: %w", err)
	}

	presenceActive := len(targets) >= 2
	var presence formalCoordinateFamilyFiberGroup
	var presenceValues formalValueAccessPlan
	var presenceLift formalClosedFactorLift
	if presenceActive {
		family, ok := body.productDomain.PathEvidenceCoordinateFamily()
		if !ok {
			return nil, fmt.Errorf("Outcome presence has no coordinate family")
		}
		presence, ok = findFamily(family)
		if !ok {
			return nil, fmt.Errorf("Outcome presence family is outside the formal product")
		}
		presenceSlots := make([]FormalSlot, len(targets))
		for index := range targets {
			presenceSlots[index] = targets[index].Slot
		}
		presenceValues, err = sealFormalValueAccessPlan(valuesGroup, presenceSlots, nil)
		if err != nil {
			return nil, fmt.Errorf("Outcome presence Values capability: %w", err)
		}
		presenceReads := append([]formalFiberOrdinal(nil), presenceValues.readOrdinals...)
		presenceReads = append(presenceReads, presence.skeleton)
		presenceReads = append(presenceReads, presence.scalars...)
		presenceWrites := append([]formalFiberOrdinal{presence.skeleton}, presence.scalars...)
		presenceReads = unionOrdinals(presenceReads)
		presenceWrites = unionOrdinals(presenceWrites)
		presenceLift, err = sealFormalClosedFactorLift(span, [][]formalFiberOrdinal{presenceReads}, presenceWrites)
		if err != nil {
			return nil, fmt.Errorf("Outcome presence lift: %w", err)
		}
	}

	var covariantBindings []factapply.CovariantFactorBinding[FormalSlot]
	var covariantValues formalValueAccessPlan
	var covariantGroups []formalFiberGroupDescriptor
	var covariantLift formalClosedFactorLift
	if covariantActive {
		covariantBindings, err = freezeFormalCovariantBindings(program, variable, span, outcome.covariant)
		if err != nil {
			return nil, fmt.Errorf("Outcome N6 bindings: %w", err)
		}
		covariantSlots := make([]FormalSlot, 0, len(covariantBindings))
		for _, binding := range covariantBindings {
			if binding.Kind != factapply.CovariantFactorBindingNoop {
				covariantSlots = append(covariantSlots, binding.Source)
			}
		}
		covariantValues, err = sealFormalValueAccessPlan(valuesGroup, covariantSlots, covariantSlots)
		if err != nil {
			return nil, fmt.Errorf("Outcome N6 Values capability: %w", err)
		}
		covariantReads := append([]formalFiberOrdinal(nil), covariantValues.readOrdinals...)
		covariantWrites := append([]formalFiberOrdinal(nil), covariantValues.writeOrdinals...)
		for index := 0; index < covariantTopology.Len(); index++ {
			lane, ok := covariantTopology.Lane(index)
			if !ok {
				return nil, fmt.Errorf("Outcome N6 topology is incomplete")
			}
			found := false
			for _, group := range groups {
				if group.kind != formalFiberGroupValues && group.lane == lane {
					covariantGroups = append(covariantGroups, group)
					covariantReads = append(covariantReads, group.members...)
					covariantWrites = append(covariantWrites, group.members...)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("Outcome N6 lane %q is outside the formal product", lane.ID())
			}
		}
		covariantReads = unionOrdinals(covariantReads)
		covariantWrites = unionOrdinals(covariantWrites)
		covariantLift, err = sealFormalClosedFactorLift(span, [][]formalFiberOrdinal{covariantReads}, covariantWrites)
		if err != nil {
			return nil, fmt.Errorf("Outcome N6 lift: %w", err)
		}
	}
	return &formalOutcomeStep{
		variable: variable, code: operator.code, root: operator.root, scope: operator.scope, outcome: operator.outcome,
		transaction: outcome.returnTransaction.transaction.Clone(), sources: sources,
		valueAccess: valueAccess, valueFactorGroups: valueFactorGroups,
		bindingValues: bindingValues, bindingLift: bindingLift, bindingActive: bindingActive,
		container: container, hasContainer: hasContainer, identity: identityPlan,
		targets: targets, demands: demands,
		presence: presence, presenceValues: presenceValues, presenceLift: presenceLift, presenceActive: presenceActive,
		covariant: outcome.covariant.Clone(), covariantBindings: covariantBindings, covariantValues: covariantValues,
		covariantGroups: covariantGroups, covariantLift: covariantLift, covariantTopology: covariantTopology,
		terminal: terminal, sealed: true,
	}, nil
}

// projectOutcome completes canonical N5 before terminal occurrence
// publication. Reachability is frozen from that stabilized product tuple, so
// Apply never re-evaluates return syntax or reconstructs State.
func (a *formalTupleAlgebra) applyFormalOutcomeBindings(
	plan *formalOutcomeStep,
	predecessor formalRelationTuple,
) (formalRelationTuple, []factapply.ReturnIdentityCondition[decisionRef], error) {
	if !plan.bindingActive {
		return predecessor, nil, nil
	}
	seeds := make([]factapply.ReturnIdentityCondition[decisionRef], 0)
	completed, err := a.applyFormalClosedFactorLift(plan.bindingLift, []formalRelationTuple{predecessor}, plan.demands, decisionTrue,
		func(guard decisionRef, views []formalSparseLeafView) ([]formalClosedFactorLeafWrite, error) {
			if len(views) != 1 {
				return nil, errFormalComponentMalformed
			}
			view := views[0]
			evaluator, err := a.newSparseTupleLeafEvaluator(view)
			if err != nil {
				return nil, err
			}
			values, err := plan.bindingValues.materialize(view)
			if err != nil {
				return nil, fmt.Errorf("transformer: formal Outcome binding Values: %w", err)
			}
			capability, err := evaluator.materializeValueFactorAccess(plan.valueAccess, plan.valueFactorGroups)
			if err != nil {
				return nil, fmt.Errorf("transformer: formal Outcome source factors: %w", err)
			}
			sources := make([]product.Value, len(plan.sources))
			for index, binding := range plan.sources {
				evaluated, evalErr := evaluator.evaluateWithFactorAccess(binding, capability)
				if evalErr != nil {
					return nil, fmt.Errorf("transformer: formal Outcome source %d: %w", index, evalErr)
				}
				sources[index] = evaluated.value
			}
			var container state.CoordinateFamilyFactor
			if plan.hasContainer {
				container, err = a.materializeFormalPresenceCoordinateFamily(view, plan.container, nil)
				if err != nil {
					return nil, fmt.Errorf("transformer: formal Outcome container: %w", err)
				}
			}
			bound, sourceValues, err := factapply.ApplyReturnResultBindings(
				evaluator.body.returns, evaluator.authority.product, plan.transaction, sources, plan.targets, values, container,
			)
			if err != nil {
				return nil, fmt.Errorf("transformer: formal Outcome result binding: %w", err)
			}
			for _, value := range sourceValues {
				root, exact := product.Get(evaluator.authority.product.Registry(), value, identity.Key).Term()
				if exact && root.Valid() {
					seeds = append(seeds, formalReturnIdentitySource(root, guard))
				}
			}
			valueWrites, err := plan.bindingValues.factorPublication(view, bound)
			if err != nil {
				return nil, fmt.Errorf("transformer: formal Outcome binding publication: %w", err)
			}
			writes := make([]formalClosedFactorLeafWrite, len(valueWrites))
			for index, write := range valueWrites {
				writes[index] = formalClosedFactorLeafWrite{ordinal: write.ordinal, leaf: write.leaf}
			}
			return writes, nil
		})
	return completed, seeds, err
}

func (a *formalTupleAlgebra) applyFormalOutcomePresence(plan *formalOutcomeStep, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if !plan.presenceActive {
		return predecessor, nil
	}
	return a.applyFormalClosedFactorLift(plan.presenceLift, []formalRelationTuple{predecessor}, nil, decisionTrue,
		func(_ decisionRef, views []formalSparseLeafView) ([]formalClosedFactorLeafWrite, error) {
			if len(views) != 1 {
				return nil, errFormalComponentMalformed
			}
			view := views[0]
			values, err := plan.presenceValues.materialize(view)
			if err != nil {
				return nil, err
			}
			factor, err := a.materializeFormalPresenceCoordinateFamily(view, plan.presence, nil)
			if err != nil {
				return nil, err
			}
			factor, err = factapply.ApplyReturnPresenceFactor(
				view.body.returns, view.span.keys, plan.transaction, plan.targets, values,
				view.authority.product, factor,
			)
			if err != nil {
				return nil, fmt.Errorf("transformer: formal Outcome presence: %w", err)
			}
			published, err := a.factorFormalPresenceCoordinateFamily(view.authority, view.span, plan.presence, factor)
			if err != nil {
				return nil, err
			}
			writes := make([]formalClosedFactorLeafWrite, len(published))
			for index, write := range published {
				writes[index] = formalClosedFactorLeafWrite{ordinal: write.ordinal, leaf: write.leaf}
			}
			sort.Slice(writes, func(i, j int) bool { return writes[i].ordinal < writes[j].ordinal })
			return writes, nil
		})
}

func (a *formalTupleAlgebra) applyFormalOutcomeCovariant(plan *formalOutcomeStep, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if !plan.covariant.HasStateSteps() {
		return predecessor, nil
	}
	return a.applyFormalClosedFactorLift(plan.covariantLift, []formalRelationTuple{predecessor}, nil, decisionTrue,
		func(_ decisionRef, views []formalSparseLeafView) ([]formalClosedFactorLeafWrite, error) {
			if len(views) != 1 {
				return nil, errFormalComponentMalformed
			}
			view := views[0]
			values, err := plan.covariantValues.materialize(view)
			if err != nil {
				return nil, err
			}
			factors := make([]state.LaneFactor, len(plan.covariantGroups))
			for index, group := range plan.covariantGroups {
				factors[index], err = view.laneFactor(group)
				if err != nil {
					return nil, err
				}
			}
			result, err := factapply.ApplyCovariantExposureFactors(a.ctx, typecovariant.WidenRecord,
				factapply.CovariantFactorTransaction[FormalSlot]{
					Transaction: plan.covariant, Bindings: plan.covariantBindings, Values: values, Factors: factors,
					Domain: view.authority.product, Keys: view.span.keys, Topology: plan.covariantTopology,
					Token: cancellation.FromContext(a.ctx).Token(),
				})
			if err != nil {
				return nil, err
			}
			valueWrites, err := plan.covariantValues.factorPublication(view, result.Values)
			if err != nil {
				return nil, err
			}
			writes := make([]formalClosedFactorLeafWrite, 0, len(valueWrites)+len(plan.covariantLift.writes))
			for _, write := range valueWrites {
				writes = append(writes, formalClosedFactorLeafWrite{ordinal: write.ordinal, leaf: write.leaf})
			}
			if len(result.Factors) != len(plan.covariantGroups) {
				return nil, errFormalComponentMalformed
			}
			for index, group := range plan.covariantGroups {
				leaves, factorErr := a.factorFormalSparseLane(view.authority, view.span, group, result.Factors[index])
				if factorErr != nil || len(leaves) != len(group.members) {
					if factorErr == nil {
						factorErr = errFormalComponentMalformed
					}
					return nil, factorErr
				}
				for memberIndex, ordinal := range group.members {
					writes = append(writes, formalClosedFactorLeafWrite{ordinal: ordinal, leaf: leaves[memberIndex]})
				}
			}
			sort.Slice(writes, func(i, j int) bool { return writes[i].ordinal < writes[j].ordinal })
			return writes, nil
		})
}

// projectOutcome is the single ordered N5/N6 transaction: result binding,
// guarded identity closure/publication, return presence, covariant exposure,
// then occurrence publication. Every stage calls the same factor law as the
// concrete carrier; the formal layer owns only guarded root lifting.
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
	plan := operator.outcomeTransaction
	if !ok || predecessor.root.owner != directory || authority.code != operator.code || predecessor.variable != plan.variable {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	completed, sources, err := a.applyFormalOutcomeBindings(plan, predecessor)
	if err != nil {
		return fail(err)
	}
	completed, err = a.applyFormalReturnIdentityClosure(completed, plan.identity, sources)
	if err != nil {
		return fail(err)
	}
	completed, err = a.applyFormalOutcomePresence(plan, completed)
	if err != nil {
		return fail(err)
	}
	completed, err = a.applyFormalOutcomeCovariant(plan, completed)
	if err != nil {
		return fail(err)
	}
	leaf, err := authority.internOutcomeOccurrence(formalQualifiedOutcomeOccurrence{
		code: operator.code, ref: operator.outcome, root: operator.root, scope: operator.scope,
	})
	if err != nil {
		return fail(err)
	}
	if int(plan.terminal.global)-span.first < 0 {
		return fail(errFormalComponentMalformed)
	}
	return a.writeScalar(completed, plan.terminal, a.decisions.terminal(leaf))
}
