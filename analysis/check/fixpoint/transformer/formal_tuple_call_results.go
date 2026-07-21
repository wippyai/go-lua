package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalCallResultsStep binds the formal tuple carrier to the canonical
// CallResults phase transaction. Materialization changes only Values;
// postconditions additionally own their registered factor lanes.
type formalCallResultsStep struct {
	phase         factapply.CallResultPhase
	materialize   factapply.CallResultMaterializeFactorProgram[FormalSlot]
	program       factapply.CallResultPostconditionFactorProgram[FormalSlot]
	values        formalFiberGroupDescriptor
	lanes         []formalFiberGroupDescriptor
	readOrdinals  []formalFiberOrdinal
	writeOrdinals []formalFiberOrdinal
}

func freezeFormalCallResultsStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalCallResultsStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepCallResults ||
		(step.resultPhase != factapply.CallResultPhaseMaterialize && step.resultPhase != factapply.CallResultPhasePostconditions) {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || !body.productDomain.Valid() || body.relation.code != operator.code {
		return nil, fmt.Errorf("CallResults has no formal product ownership")
	}
	inventory := span.coordinates
	if step.resultPhase == factapply.CallResultPhasePostconditions &&
		(body.pathSemantics == nil || !body.pathSemantics.Valid() || !inventory.ValidFor(body.productDomain, span.keys)) {
		return nil, fmt.Errorf("CallResults N3 has no frozen coordinate inventory")
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("CallResults has no formal Values group")
	}
	plan := formalCallResultsStep{phase: step.resultPhase, values: values.descriptor}
	if step.resultPhase == factapply.CallResultPhaseMaterialize {
		prepared, err := factapply.PrepareCallResultMaterializeFactorProgram(body.productDomain.Registry(), step.result, func(point, result uint32) (FormalSlot, bool) {
			return formalMiddleSlotForStateKey(program, body, statekey.CallResult(point, result))
		})
		if err != nil {
			return nil, err
		}
		plan.materialize = prepared
	} else {
		prepared, err := factapply.PrepareFormalCallResultPostconditionFactorProgram(
			body.pathSemantics, body.productDomain, step.result, inventory, span.rekey, span.keys,
			func(dependency statekey.ValueDependency) (FormalSlot, bool) {
				return formalLiveValueSlotForDependency(program, body, dependency)
			},
		)
		if err != nil {
			return nil, err
		}
		plan.program = prepared
		groups := span.groupDescriptors()
		plan.lanes = make([]formalFiberGroupDescriptor, len(prepared.Lanes()))
		for index, lane := range prepared.Lanes() {
			found := false
			for _, group := range groups {
				if group.kind != formalFiberGroupValues && group.lane == lane {
					plan.lanes[index], found = group, true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("CallResults N3 lane %q is outside the formal product", lane.ID())
			}
		}
	}
	seal := func(groups ...formalFiberGroupDescriptor) []formalFiberOrdinal {
		var out []formalFiberOrdinal
		for _, group := range groups {
			out = append(out, group.members...)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		write := 0
		for _, ordinal := range out {
			if write == 0 || out[write-1] != ordinal {
				out[write], write = ordinal, write+1
			}
		}
		return out[:write]
	}
	all := append([]formalFiberGroupDescriptor{values.descriptor}, plan.lanes...)
	ordinals := seal(all...)
	plan.readOrdinals = ordinals
	plan.writeOrdinals = append([]formalFiberOrdinal(nil), ordinals...)
	return &plan, nil
}

func (a *formalTupleAlgebra) applyFormalCallResults(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.callResults
	if a == nil || plan == nil || operator.kind != formalRelationCellStep || operator.code == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal CallResults N3 has no complete factor transaction")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		return predecessor, err
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	regions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{tuple: predecessor, ordinals: plan.readOrdinals}}, nil)
	if err != nil {
		return formalRelationTuple{}, err
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	type affectedRoot struct {
		ordinal formalFiberOrdinal
		root    decisionRef
	}
	affected := make([]affectedRoot, len(plan.writeOrdinals))
	for index, ordinal := range plan.writeOrdinals {
		affected[index].ordinal = ordinal
	}
	for _, region := range regions {
		if len(region.views) != 1 {
			return fail(errDecisionMalformed)
		}
		view := region.views[0]
		valueLeaves := make([]decisionLeaf, len(plan.values.members))
		for index, ordinal := range plan.values.members {
			leaf, present := view.leaf(ordinal)
			if !present {
				return fail(errFormalComponentMalformed)
			}
			valueLeaves[index] = leaf
		}
		values, leafErr := a.materializeValuesGroup(authority, plan.values, valueLeaves)
		if leafErr != nil {
			return fail(leafErr)
		}
		factors := make([]state.LaneFactor, len(plan.lanes))
		for index, group := range plan.lanes {
			factors[index], leafErr = view.laneFactor(group)
			if leafErr != nil {
				return fail(leafErr)
			}
		}
		nextValues := values
		nextFactors := factors
		switch plan.phase {
		case factapply.CallResultPhaseMaterialize:
			nextValues, leafErr = plan.materialize.Apply(a.ctx, nil, values)
		case factapply.CallResultPhasePostconditions:
			var next factapply.CallResultPostconditionFactorFrame[FormalSlot]
			next, leafErr = plan.program.Apply(a.ctx, nil, factapply.CallResultPostconditionFactorFrame[FormalSlot]{Values: values, Factors: factors, Reachable: true})
			nextValues, nextFactors = next.Values, next.Factors
			if !next.Reachable {
				continue
			}
		default:
			return fail(errFormalComponentMalformed)
		}
		if leafErr != nil {
			return fail(leafErr)
		}
		valueLeaves, leafErr = a.factorValuesGroup(authority, plan.values, nextValues)
		if leafErr != nil {
			return fail(leafErr)
		}
		publish := func(ordinal formalFiberOrdinal, leaf decisionLeaf) error {
			index := sort.Search(len(affected), func(i int) bool { return affected[i].ordinal >= ordinal })
			if index >= len(affected) || affected[index].ordinal != ordinal {
				return errFormalComponentMalformed
			}
			var err error
			affected[index].root, err = a.decisions.condition(a.ctx, region.guard, a.decisions.terminal(leaf), affected[index].root)
			return err
		}
		for index, ordinal := range plan.values.members {
			if leafErr = publish(ordinal, valueLeaves[index]); leafErr != nil {
				return fail(leafErr)
			}
		}
		for index, group := range plan.lanes {
			leaves, factorErr := a.factorFormalSparseLane(authority, span, group, nextFactors[index])
			if factorErr != nil {
				return fail(factorErr)
			}
			for memberIndex, ordinal := range group.members {
				if factorErr = publish(ordinal, leaves[memberIndex]); factorErr != nil {
					return fail(factorErr)
				}
			}
		}
	}
	writes := make([]formalFiberWrite, 0, len(affected))
	for _, candidate := range affected {
		descriptor := span.forest.descriptors[span.first+int(candidate.ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, candidate.root); err != nil {
			return fail(err)
		}
		prior, err := directory.valueAt(predecessor.root, candidate.ordinal)
		if err != nil {
			return fail(err)
		}
		if prior != formalFiberValue(candidate.root) {
			writes = append(writes, formalFiberWrite{ordinal: candidate.ordinal, value: formalFiberValue(candidate.root)})
		}
	}
	if len(writes) == 0 {
		return predecessor, nil
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return fail(err)
	}
	root, _, err := directory.applyDelta(predecessor.root, delta)
	if err != nil {
		return fail(err)
	}
	return a.normalize(formalRelationTuple{variable: predecessor.variable, root: root}), nil
}
