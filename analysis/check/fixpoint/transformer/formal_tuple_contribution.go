package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

// formalContributionStep is the one frozen N-prefix diagnostic transaction.
// syntax is detached immutable publication syntax; diagnostics is its already
// closed body-relative recursive carrier. Execution never reopens relationCode
// or reconstructs State.
type formalContributionStep struct {
	program     *RelationProgram
	variable    relationVar
	code        *relationCode
	ref         boundaryContributionRef
	syntax      semanticContribution
	diagnostics callpayload.DiagnosticOutput
	active      bool
	descriptor  formalFiberDescriptor
}

func (s *formalContributionStep) validFor(program *RelationProgram, variable relationVar, code *relationCode) bool {
	if s == nil || s.program != program || s.variable != variable || s.code != code || s.ref == 0 ||
		program == nil || variable == 0 || int(variable) > len(program.bodies) ||
		int(s.ref) >= len(code.contributions) || s.descriptor.role != formalFiberDiagnostics ||
		s.descriptor.variable != variable || !s.diagnostics.Valid(program.registry) {
		return false
	}
	span, ok := program.formalFibers.span(variable)
	_, owned := span.ordinal(s.descriptor)
	return ok && owned && s.active == semanticContributionCarriesRecursiveDiagnostics(s.syntax)
}

func freezeFormalContributionStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalContributionStep, error) {
	if program == nil || program.formalFibers == nil || variable == 0 || int(variable) > len(program.bodies) ||
		operator.code == nil || operator.kind != formalRelationCellStep || operator.root == 0 || operator.step == 0 {
		return nil, fmt.Errorf("transformer: formal Contribution freeze is unowned")
	}
	step, ok := formalRelationStepOperator(operator)
	if !ok {
		return nil, fmt.Errorf("transformer: formal Contribution operator is malformed")
	}
	if step.kind != boundaryStepContribution {
		return nil, nil
	}
	code := operator.code
	if step.contribution == 0 || int(step.contribution) >= len(code.contributions) {
		return nil, fmt.Errorf("transformer: formal Contribution syntax is unowned")
	}
	body := &program.bodies[variable-1]
	if body.variable != variable || body.relation.code != code {
		return nil, fmt.Errorf("transformer: formal Contribution body is foreign")
	}
	syntax := code.contributions[step.contribution].clone()
	diagnostics, err := materializeBoundaryPrefixDiagnostics(body, syntax)
	if err != nil {
		return nil, err
	}
	span, ok := program.formalFibers.span(variable)
	if !ok {
		return nil, fmt.Errorf("transformer: formal Contribution has no fiber span")
	}
	var descriptor formalFiberDescriptor
	for _, candidate := range span.descriptors() {
		if candidate.role != formalFiberDiagnostics {
			continue
		}
		if descriptor.role != formalFiberInvalid {
			return nil, fmt.Errorf("transformer: formal Contribution diagnostics fiber is ambiguous")
		}
		descriptor = candidate
	}
	if descriptor.role == formalFiberInvalid {
		return nil, fmt.Errorf("transformer: formal Contribution diagnostics fiber is missing")
	}
	out := &formalContributionStep{
		program: program, variable: variable, code: code, ref: step.contribution,
		syntax: syntax, diagnostics: diagnostics.Clone(),
		active: semanticContributionCarriesRecursiveDiagnostics(syntax), descriptor: descriptor,
	}
	if !out.validFor(program, variable, code) {
		return nil, fmt.Errorf("transformer: formal Contribution transaction did not seal")
	}
	return out, nil
}

// applyFormalContribution sequences one immutable diagnostic event through
// the exact guarded diagnostic root. It touches neither product groups nor
// other symbolic fibers; the persistent directory path-copies one leaf.
func (a *formalTupleAlgebra) applyFormalContribution(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || a.ctx == nil || operator.contribution == nil ||
		!operator.contribution.validFor(a.program, operator.contribution.variable, operator.code) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Contribution execution is unowned")
	}
	if err := a.ctx.Err(); err != nil {
		return formalRelationTuple{}, err
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() {
		return predecessor, nil
	}
	if predecessor.variable != operator.contribution.variable {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	if !operator.contribution.active {
		return predecessor, nil
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	ordinal, ordinalOK := span.ordinal(operator.contribution.descriptor)
	if !ok || !ordinalOK || predecessor.root.owner != directory {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	value, err := directory.valueAt(predecessor.root, ordinal)
	if err != nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Contribution diagnostic read: %w", err)
	}
	root, err := a.componentRoot(authority, operator.contribution.descriptor, decisionRef(value))
	if err != nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Contribution diagnostic root: %w", err)
	}
	mark := a.decisions.checkpoint()
	mapped, err := a.decisions.mapLeavesTransient(a.ctx, root, func(leaf decisionLeaf) (decisionLeaf, error) {
		leaf, leafErr := a.componentLeaf(authority, operator.contribution.descriptor, leaf)
		if leafErr != nil {
			return 0, fmt.Errorf("transformer: formal Contribution diagnostic default: %w", leafErr)
		}
		terminal, terminalErr := authority.terminal(leaf)
		if terminalErr != nil || terminal.kind != formalComponentDiagnostics {
			if terminalErr != nil {
				return 0, fmt.Errorf("transformer: formal Contribution diagnostic terminal: %w", terminalErr)
			}
			return 0, errFormalComponentMalformed
		}
		composed := composeBoundaryDiagnostics(
			a.program.registry, terminal.diagnostics, operator.contribution.diagnostics, true,
		)
		result, internErr := authority.internDiagnostics(composed)
		if internErr != nil {
			return 0, fmt.Errorf("transformer: formal Contribution diagnostic intern: %w", internErr)
		}
		return result, nil
	})
	if err != nil {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	result, err := a.writeScalar(predecessor, operator.contribution.descriptor, mapped)
	if err != nil {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Contribution diagnostic publication: %w", err)
	}
	return result, nil
}

// formalDiagnosticOutput closes only Care+Diagnostics into one route-free
// recursive diagnostic value. No product fiber is read or materialized.
func (a *formalTupleAlgebra) formalDiagnosticOutput(ctx context.Context, tuple formalRelationTuple) (callpayload.DiagnosticOutput, bool, error) {
	if a == nil || ctx == nil {
		return callpayload.DiagnosticOutput{}, false, fmt.Errorf("transformer: formal diagnostic projection is unowned")
	}
	if err := a.validateTuple(tuple); err != nil {
		return callpayload.DiagnosticOutput{}, false, err
	}
	if tuple.bottom() {
		return callpayload.DiagnosticOutput{}, false, nil
	}
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory {
		return callpayload.DiagnosticOutput{}, false, errFormalComponentForeignOwner
	}
	var descriptor formalFiberDescriptor
	for _, candidate := range span.descriptors() {
		if candidate.role == formalFiberDiagnostics {
			if descriptor.role != formalFiberInvalid {
				return callpayload.DiagnosticOutput{}, false, errFormalComponentMalformed
			}
			descriptor = candidate
		}
	}
	ordinal, ordinalOK := span.ordinal(descriptor)
	if descriptor.role == formalFiberInvalid || !ordinalOK {
		return callpayload.DiagnosticOutput{}, false, errFormalComponentMalformed
	}
	value, err := directory.valueAt(tuple.root, ordinal)
	if err != nil {
		return callpayload.DiagnosticOutput{}, false, err
	}
	root, err := a.componentRoot(authority, descriptor, decisionRef(value))
	if err != nil {
		return callpayload.DiagnosticOutput{}, false, err
	}
	care, err := a.care(tuple)
	if err != nil {
		return callpayload.DiagnosticOutput{}, false, err
	}
	regions, err := a.decisions.partitionLeafTuplesUnderCare(ctx, care, []decisionRef{root})
	if err != nil {
		return callpayload.DiagnosticOutput{}, false, err
	}
	domain := callpayload.DiagnosticOutputLattice(a.program.registry)
	joined, reachable := domain.Bottom(), false
	for _, region := range regions {
		if len(region.leaves) != 1 || region.care == decisionFalse {
			return callpayload.DiagnosticOutput{}, false, errDecisionMalformed
		}
		terminal, terminalErr := authority.terminal(region.leaves[0])
		if terminalErr != nil || terminal.kind != formalComponentDiagnostics {
			if terminalErr != nil {
				return callpayload.DiagnosticOutput{}, false, terminalErr
			}
			return callpayload.DiagnosticOutput{}, false, errFormalComponentMalformed
		}
		if !reachable {
			joined, reachable = terminal.diagnostics.Clone(), true
		} else {
			joined = domain.Join(joined, terminal.diagnostics)
		}
	}
	return joined.Normalize(a.program.registry), reachable, nil
}

func (a *formalTupleAlgebra) formalDiagnosticLeaf(evaluator formalTupleLeafEvaluator) (callpayload.DiagnosticOutput, formalFiberDescriptor, error) {
	if a == nil || !evaluator.valid() || evaluator.algebra != a {
		return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, errFormalComponentForeignOwner
	}
	var descriptor formalFiberDescriptor
	descriptorOrdinal := -1
	for ordinal, candidate := range evaluator.span.descriptors() {
		if candidate.role != formalFiberDiagnostics {
			continue
		}
		if descriptor.role != formalFiberInvalid {
			return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, errFormalComponentMalformed
		}
		descriptor = candidate
		descriptorOrdinal = ordinal
	}
	if descriptorOrdinal < 0 {
		return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, errFormalComponentMalformed
	}
	selected, present := evaluator.leaves.leaf(formalFiberOrdinal(descriptorOrdinal))
	if !present {
		return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, errFormalComponentMalformed
	}
	leaf, err := a.componentLeaf(evaluator.authority, descriptor, selected)
	if err != nil {
		return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, err
	}
	terminal, err := evaluator.authority.terminal(leaf)
	if err != nil || terminal.kind != formalComponentDiagnostics {
		if err != nil {
			return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, err
		}
		return callpayload.DiagnosticOutput{}, formalFiberDescriptor{}, errFormalComponentMalformed
	}
	return terminal.diagnostics.Clone(), descriptor, nil
}

// applyFormalApplicationDiagnostics is the sole target→caller recursive
// diagnostic transport. It applies the body-relative lift and sequencing laws
// directly in the canonical formal carrier.
func (a *formalTupleAlgebra) applyFormalApplicationDiagnostics(
	step *formalApplyStep,
	region formalApplyCorrelatedRegion,
	leaves []decisionLeaf,
	role boundaryCallDiagnosticRole,
) error {
	if a == nil || step == nil || !step.validFor(a.program, step.owner) ||
		!region.caller.valid() || !region.target.valid() || len(leaves) != region.caller.span.count {
		return errFormalComponentForeignOwner
	}
	callerDiagnostics, callerDescriptor, err := a.formalDiagnosticLeaf(region.caller)
	if err != nil {
		return err
	}
	targetDiagnostics, _, err := a.formalDiagnosticLeaf(region.target)
	if err != nil {
		return err
	}
	var result callpayload.DiagnosticOutput
	switch role {
	case boundaryCallDiagnosticCompose:
		lifted, liftErr := liftBoundaryApplicationDiagnostics(
			&a.program.bodies[step.owner-1], &a.program.bodies[step.target-1], step.linked, targetDiagnostics,
		)
		if liftErr != nil {
			return liftErr
		}
		result = composeBoundaryDiagnostics(a.program.registry, callerDiagnostics, lifted, true)
	case boundaryCallDiagnosticCalleeCarry:
		result = targetDiagnostics
	case boundaryCallDiagnosticKnown:
		// Definition publication does not sequence either body's diagnostics.
		// It records only that the boundary suspension status is known.
		result = callpayload.DiagnosticOutput{SuspensionKnown: true}
	default:
		return fmt.Errorf("transformer: formal Apply diagnostic role is unowned")
	}
	leaf, err := region.caller.authority.internDiagnostics(result)
	if err != nil {
		return err
	}
	ordinal, ok := region.caller.span.ordinal(callerDescriptor)
	if !ok || int(ordinal) >= len(leaves) {
		return errFormalComponentMalformed
	}
	leaves[ordinal] = leaf
	return nil
}
