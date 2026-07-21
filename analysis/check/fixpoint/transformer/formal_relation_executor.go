package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// formalRelationExecution owns one completed run of the canonical formal
// equation system. Tuple roots are run-local capabilities, so the algebra must
// remain owned beside the solution that refers into its arenas.
type formalRelationExecution struct {
	algebra              *formalTupleAlgebra
	values               map[formalRelationCell]formalRelationTuple
	internalCallOutcomes map[formalRelationCell]callpayload.CallOutcomeAlternativeSet
}

// executeFormalRelation is the syntax-only executor used by transformer law
// tests. Production root invocation must use executeFormalRootRelation so a
// selected body and concrete entry cannot silently fall back to template
// constants.
//
// executeFormalRelation solves the already-frozen formal relation equations
// with the generic WTO engine. Evaluation is deliberately a pure interpretation
// of formalRelationEquation.Inputs: syntax, scheduler cells, and publications
// are never consulted to discover another dependency at run time.
//
// Every live Step executes through its frozen registered capability. A live
// influence without one fails the entire transaction; no partial map or
// fallback result is published.
func executeFormalRelation(ctx context.Context, program *RelationProgram) (*formalRelationExecution, error) {
	return executeFormalRelationWithRootEntry(ctx, program, nil)
}

// executeFormalRootRelation invokes the frozen equation system for one
// selected production root. The concrete entry is consumed at this boundary
// and immediately transposed into run-owned formal factors; neither the
// execution nor the sealed template retains State.
func executeFormalRootRelation(ctx context.Context, program *RelationProgram, bodyID lexicalidentity.StableLexicalBodyID, entry state.State) (*formalRelationExecution, error) {
	if ctx == nil {
		return nil, fmt.Errorf("transformer: formal relation execution has no context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rootEntry, err := freezeFormalRootEntrySeed(program, bodyID, entry)
	if err != nil {
		return nil, err
	}
	return executeFormalRelationWithRootEntry(ctx, program, &rootEntry)
}

func executeFormalRelationWithRootEntry(ctx context.Context, program *RelationProgram, rootEntry *formalRootEntrySeed) (*formalRelationExecution, error) {
	if ctx == nil {
		return nil, fmt.Errorf("transformer: formal relation execution has no context")
	}
	if err := validateFormalRelationExecutionProgram(program); err != nil {
		return nil, err
	}
	algebra, err := newFormalTupleAlgebra(ctx, program)
	if err != nil {
		return nil, err
	}
	if rootEntry != nil {
		if !rootEntry.validFor(program) {
			return nil, fmt.Errorf("transformer: formal relation execution has an invalid root entry")
		}
		algebra.rootEntry = rootEntry
	}
	template, region := program.formalTemplate, program.formalRegion
	evalTrace := newFormalRelationEvalTrace()
	algebra.evalTrace = evalTrace
	system := solve.EquationSystem[formalRelationCell, formalRelationTuple]{
		Lattice: algebra.lattice(),
		Cells:   region.cells,
		Evaluate: func(cell formalRelationCell, read func(formalRelationCell) formalRelationTuple) formalRelationTuple {
			if algebra.err() != nil {
				return formalRelationTuple{}
			}
			equation, ok := template.equation(cell)
			if !ok {
				algebra.fail(fmt.Errorf("transformer: formal relation execution has no equation for cell %+v", cell))
				return formalRelationTuple{}
			}
			var result formalRelationTuple
			if evalTrace == nil {
				result = evaluateFormalRelationEquation(algebra, equation, read)
			} else {
				result = evalTrace.evaluate(algebra, equation, read)
			}
			if failure := algebra.err(); failure != nil {
				// The tuple algebra records the first semantic failure because the
				// lattice callbacks cannot return errors. Retain the equation owner
				// at that boundary so an ownership invariant is actionable instead
				// of collapsing to a package-wide sentinel.
				algebra.firstError = fmt.Errorf("transformer: formal equation cell %+v: %w", cell, failure)
			}
			return result
		},
		WidenAt: func(cell formalRelationCell) bool { return region.widen[cell] },
	}
	values, err := solve.SolveWTOContext(ctx, system, region.plan)
	if err != nil {
		return nil, err
	}
	if err := algebra.err(); err != nil {
		return nil, err
	}
	execution := &formalRelationExecution{algebra: algebra, values: values}
	internalCallOutcomes, err := execution.detachFormalApplyCallOutcomes(ctx)
	if err != nil {
		return nil, err
	}
	execution.internalCallOutcomes = internalCallOutcomes
	return execution, nil
}

func validateFormalRelationExecutionProgram(program *RelationProgram) error {
	if program == nil || program.formalTemplate == nil || !program.formalTemplate.validFor(program) {
		return fmt.Errorf("transformer: formal relation execution has no sealed equation system")
	}
	return nil
}

func evaluateFormalRelationEquation(
	algebra *formalTupleAlgebra,
	equation formalRelationEquation,
	read func(formalRelationCell) formalRelationTuple,
) formalRelationTuple {
	if equation.Cell.cell.Kind == formalRelationCellStep {
		return evaluateFormalRelationStepEquation(algebra, equation, read)
	}
	if equation.Cell.cell.Kind == formalRelationCellDefinition {
		return evaluateFormalDefinitionEquation(algebra, equation, read)
	}
	if equation.Cell.cell.Kind == formalRelationCellResource {
		return evaluateFormalResourceEquation(algebra, equation, read)
	}
	candidate := formalRelationTuple{}
	join := func(value formalRelationTuple) bool {
		if value.bottom() {
			return true
		}
		candidate = algebra.combine(formalComponentJoin, candidate, value)
		return algebra.err() == nil
	}

	if equation.Operator.rootInput != nil {
		// instantiateRootEquation is the complete root base, including its
		// already-frozen Seeds. Joining them here as well would duplicate work.
		root, err := algebra.instantiateRootEquation(equation)
		if err != nil {
			algebra.fail(err)
			return formalRelationTuple{}
		}
		if !join(root) {
			return formalRelationTuple{}
		}
	} else {
		for _, seed := range equation.Seeds {
			constant, err := algebra.instantiateConstant(seed)
			if err != nil {
				algebra.fail(err)
				return formalRelationTuple{}
			}
			if !join(constant) {
				return formalRelationTuple{}
			}
		}
	}

	for _, transaction := range equation.ApplyNonreturning {
		predecessor := read(transaction.Predecessor.cell)
		target := read(transaction.Target.cell)
		if predecessor.bottom() || target.bottom() {
			continue
		}
		projected, err := algebra.applyNonreturning(transaction.Operator, predecessor, target)
		if err != nil {
			algebra.fail(err)
			return formalRelationTuple{}
		}
		if !join(projected) {
			return formalRelationTuple{}
		}
	}

	pairedApplyInputs := 0
	for _, input := range equation.Inputs {
		if input.Influence == formalRelationInfluenceApplyNonreturningPredecessor ||
			input.Influence == formalRelationInfluenceCalleeNonreturning {
			// Consumed simultaneously by the freeze-time Site transaction above.
			pairedApplyInputs++
			continue
		}
		// Every source was frozen and legality-checked before the solve began;
		// paired Apply terminals were consumed above as one typed transaction.
		value := read(input.Source.cell)
		if value.bottom() {
			continue
		}
		if formalRelationOutcomeInput(equation, input) {
			projected, err := algebra.projectOutcome(equation.Operator, value)
			if err != nil {
				algebra.fail(err)
				return formalRelationTuple{}
			}
			if !join(projected) {
				return formalRelationTuple{}
			}
			continue
		}
		if formalRelationLocalNonreturningInput(equation, input) {
			projected, err := algebra.projectLocalNonreturning(equation.Operator, value)
			if err != nil {
				algebra.fail(err)
				return formalRelationTuple{}
			}
			if !join(projected) {
				return formalRelationTuple{}
			}
			continue
		}
		controlled, handled, err := evaluateFormalControlInput(algebra, equation, input, value)
		if err != nil {
			algebra.fail(err)
			return formalRelationTuple{}
		}
		if handled {
			if !join(controlled) {
				return formalRelationTuple{}
			}
			continue
		}
		if !formalRelationIdentityInput(equation, input) {
			algebra.fail(fmt.Errorf("transformer: live formal influence %d at cell %+v is not implemented", input.Influence, equation.Cell.cell))
			return formalRelationTuple{}
		}
		if !join(value) {
			return formalRelationTuple{}
		}
	}
	if pairedApplyInputs != 2*len(equation.ApplyNonreturning) {
		algebra.fail(fmt.Errorf("transformer: formal nonreturning Apply transaction is incomplete"))
		return formalRelationTuple{}
	}
	result := algebra.normalize(candidate)
	if equation.Cell.cell.Kind == formalRelationCellOutcome && !result.bottom() {
		// The Outcome equation—not an individual occurrence—is the canonical
		// producer consumed by Apply. Multiple CFG returns may join here, so its
		// stabilized per-lane spellings must be registered after that join. Lanes
		// are registered independently: execution capability remains owned by the
		// exact Apply/Definition region and no cross-lane product is formed here.
		if err := algebra.cacheFormalOutcomeFactorSpellings(result); err != nil {
			algebra.fail(err)
			return formalRelationTuple{}
		}
	}
	return result
}

// evaluateFormalRelationStepEquation consumes the complete frozen operand row
// once and invokes exactly one frozen Step capability.  Dependency operands
// are readiness authorities, not additional flow states: joining them into the
// predecessor would conflate distinct lexical frames and manufacture facts.
func evaluateFormalRelationStepEquation(
	algebra *formalTupleAlgebra,
	equation formalRelationEquation,
	read func(formalRelationCell) formalRelationTuple,
) formalRelationTuple {
	if len(equation.StepStages) == 0 {
		algebra.fail(fmt.Errorf("transformer: formal Step equation has no frozen stage vector"))
		return formalRelationTuple{}
	}
	if equation.StepStages[len(equation.StepStages)-1].Operator.stepCapability == formalRelationStepCapabilityApply {
		delete(algebra.applyObservations, equation.Cell.cell)
	}
	var current formalRelationTuple
	for index, stage := range equation.StepStages {
		predecessor := current
		if index == 0 {
			predecessor = read(stage.Operands.Flow.Source.cell)
			if predecessor.bottom() {
				return formalRelationTuple{}
			}
		}
		current = evaluateFormalRelationStepStage(algebra, equation.Cell.cell, stage, predecessor, read)
		if current.bottom() || algebra.err() != nil {
			return formalRelationTuple{}
		}
	}
	return current
}

func evaluateFormalRelationStepStage(
	algebra *formalTupleAlgebra,
	owner formalRelationCell,
	stage formalRelationStepStage,
	predecessor formalRelationTuple,
	read func(formalRelationCell) formalRelationTuple,
) formalRelationTuple {
	operands, operator := stage.Operands, stage.Operator
	// Every non-flow operand is part of the closed dependency equation.  Bottom
	// means its producer has not established the transaction yet.
	ready := func(inputs []formalRelationTemplateInput) ([]formalRelationTuple, bool) {
		values := make([]formalRelationTuple, len(inputs))
		for index, input := range inputs {
			values[index] = read(input.Source.cell)
			if values[index].bottom() {
				return nil, false
			}
		}
		return values, true
	}
	var nodeEntry formalRelationTuple
	if operands.HasNodeEntry {
		nodeEntry = read(operands.NodeEntry.Source.cell)
		if nodeEntry.bottom() {
			return formalRelationTuple{}
		}
	}
	published, ok := ready(operands.PublishedReads)
	if !ok {
		return formalRelationTuple{}
	}
	outcomes, ok := ready(operands.CalleeOutcomes)
	if !ok {
		return formalRelationTuple{}
	}
	if _, ok := ready(operands.ClosureDefinitions); !ok {
		return formalRelationTuple{}
	}

	var result formalRelationTuple
	var err error
	switch operator.stepCapability {
	case formalRelationStepCapabilityApply:
		var observation formalApplyObservation
		result, observation, err = algebra.applyOutcome(operator, predecessor, outcomes)
		if err == nil {
			witness := formalApplyObservationWitness{
				predecessorCell: operands.Flow.Source.cell, predecessorValue: predecessor,
				outcomeCells:  make([]formalRelationCell, len(operands.CalleeOutcomes)),
				outcomeValues: append([]formalRelationTuple(nil), outcomes...), observation: observation,
			}
			for index, input := range operands.CalleeOutcomes {
				witness.outcomeCells[index] = input.Source.cell
			}
			algebra.applyObservations[owner] = witness
		}
	case formalRelationStepCapabilityPathReplacement:
		result, err = algebra.applyFormalPathReplacement(operator, predecessor)
	case formalRelationStepCapabilityPathInvalidation:
		result, err = algebra.applyFormalPathInvalidation(operator, predecessor)
	case formalRelationStepCapabilityIndexMutation:
		result, err = algebra.applyFormalIndexMutation(operator, predecessor)
	case formalRelationStepCapabilityAllocationTemplate:
		result, err = algebra.applyFormalAllocationTemplate(operator, predecessor)
	case formalRelationStepCapabilityObjectMaterialization:
		result, err = algebra.applyFormalObjectMaterialization(operator, predecessor)
	case formalRelationStepCapabilityEnvironmentWrite:
		result, err = algebra.applyFormalEnvironmentWrite(operator, predecessor)
	case formalRelationStepCapabilityChannelSelect:
		result, err = algebra.applyFormalChannelSelect(operator, predecessor)
	case formalRelationStepCapabilityBranchRelations:
		result, err = algebra.applyFormalBranchRelations(operator, predecessor)
	case formalRelationStepCapabilityCallResults:
		result, err = algebra.applyFormalCallResults(operator, predecessor)
	case formalRelationStepCapabilityPresenceImplications:
		result, err = algebra.applyFormalPresenceImplications(operator, predecessor)
	case formalRelationStepCapabilityLoopControl:
		// Loop Steps publish their predecessor unchanged. Their outgoing typed
		// control influence is the sole owner of feedback closure versus exact
		// exit preservation, so the distinction cannot be duplicated here.
		result = predecessor
	case formalRelationStepCapabilityGenericFor:
		result, err = algebra.applyFormalGenericFor(operator, predecessor, nodeEntry)
	case formalRelationStepCapabilityRootAssignment:
		result, err = algebra.applyFormalRootAssignmentPlan(operator, predecessor, nodeEntry)
	case formalRelationStepCapabilityCovariantExposure:
		result, err = algebra.applyFormalCovariantExposure(operator, operator.covariantExposure, predecessor, nodeEntry)
	case formalRelationStepCapabilityContribution:
		result, err = algebra.applyFormalContribution(operator, predecessor)
	case formalRelationStepCapabilityExternalCall:
		result, err = algebra.applyFormalExternalCall(operator, predecessor, published)
	default:
		step, _ := formalRelationStepOperator(operator)
		err = fmt.Errorf("transformer: sealed formal Step has invalid capability %d for boundary kind %d at cell %+v", operator.stepCapability, step.kind, stage.Cell)
	}
	if err != nil {
		step, _ := formalRelationStepOperator(operator)
		algebra.fail(fmt.Errorf("transformer: formal Step capability=%d boundary=%d point=%d: %w", operator.stepCapability, step.kind, step.point, err))
		return formalRelationTuple{}
	}
	return algebra.normalize(result)
}

func formalRelationOutcomeInput(equation formalRelationEquation, input formalRelationTemplateInput) bool {
	return equation.Cell.cell.Kind == formalRelationCellOutcome && input.Influence == formalRelationInfluenceFlow
}

func formalRelationLocalNonreturningInput(equation formalRelationEquation, input formalRelationTemplateInput) bool {
	return equation.Cell.cell.Kind == formalRelationCellNonreturning &&
		input.Influence == formalRelationInfluenceLocalNonreturning
}

func formalRelationIdentityInput(equation formalRelationEquation, input formalRelationTemplateInput) bool {
	switch input.Influence {
	case formalRelationInfluenceFlow:
		// A Step owns an actual transfer operator, while an Outcome owns normal
		// terminal projection. Treating either predecessor as its result would
		// silently erase that operator. Only Node entry flow is exact identity.
		return equation.Cell.cell.Kind == formalRelationCellNode
	default:
		return false
	}
}
