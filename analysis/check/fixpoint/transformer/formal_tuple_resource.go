package transformer

import "fmt"

// formalResourceTransaction is the one recursive lexical-world equation:
//
//	Resource(owner) = join(Outcome(owner), DefinitionFeedback(owner))
//
// Every operand is already a guarded complete-product tuple in the owner's
// formal keyspace. Therefore the canonical boundary law is the tuple lattice
// join itself; applying a call frame, reconstructing State, or retransporting
// factors would change the semantics rather than implement it.
type formalResourceTransaction struct {
	ref   formalRelationResourceRef
	owner relationVar
}

func (t *formalResourceTransaction) validFor(program *RelationProgram, operator formalRelationOperatorRef) bool {
	if t == nil || program == nil || operator.kind != formalRelationCellResource ||
		operator.region != program.formalRegion || operator.resource == 0 ||
		t.ref != operator.resource || int(t.ref) >= len(operator.region.resources) ||
		t.owner == 0 || int(t.owner) > len(program.bodies) {
		return false
	}
	resource := operator.region.resources[t.ref]
	return resource.cell.valid() && resource.cell.Kind == formalRelationCellResource &&
		resource.cell.Variable == t.owner && resource.owner == t.owner
}

func freezeFormalResourceTransaction(program *RelationProgram, operator formalRelationOperatorRef) (*formalResourceTransaction, error) {
	if program == nil || program.formalRegion == nil || operator.kind != formalRelationCellResource ||
		operator.region != program.formalRegion || operator.resource == 0 ||
		int(operator.resource) >= len(operator.region.resources) {
		return nil, fmt.Errorf("resource transaction is unowned")
	}
	resource := operator.region.resources[operator.resource]
	transaction := &formalResourceTransaction{ref: operator.resource, owner: resource.owner}
	if !transaction.validFor(program, operator) {
		return nil, fmt.Errorf("resource transaction failed closure")
	}
	return transaction, nil
}

// evaluateFormalResourceEquation consumes the complete N12/N13 operand row
// atomically in the sole WTO evaluator. Bottom feedback is simply an
// unstabilized recursive contribution; available owner outcomes and prior
// feedback continue the ascending chain without waiting or manufacturing a
// separate execution path.
func evaluateFormalResourceEquation(
	algebra *formalTupleAlgebra,
	equation formalRelationEquation,
	read func(formalRelationCell) formalRelationTuple,
) formalRelationTuple {
	transaction := equation.Operator.resourceTransaction
	if algebra == nil || transaction == nil || !transaction.validFor(algebra.program, equation.Operator) {
		algebra.fail(fmt.Errorf("transformer: formal Resource transaction is malformed"))
		return formalRelationTuple{}
	}
	resource := equation.Operator.region.resources[transaction.ref]
	wantSeeds := len(equation.Operator.region.outcomes[transaction.owner-1])
	wantFeedback := len(resource.members)
	seedCount, feedbackCount := 0, 0
	result := formalRelationTuple{}
	for _, input := range equation.Inputs {
		switch input.Influence {
		case formalRelationInfluenceResourceSeed:
			seedCount++
		case formalRelationInfluenceResourceFeedback:
			feedbackCount++
		default:
			algebra.fail(fmt.Errorf("transformer: formal Resource has foreign influence %d", input.Influence))
			return formalRelationTuple{}
		}
		value := read(input.Source.cell)
		if value.bottom() {
			continue
		}
		if err := algebra.validateTuple(value); err != nil || value.variable != transaction.owner {
			if err != nil {
				algebra.fail(err)
			} else {
				algebra.fail(fmt.Errorf("transformer: formal Resource operand has foreign ownership"))
			}
			return formalRelationTuple{}
		}
		if result.bottom() {
			result = value
		} else {
			result = algebra.combine(formalComponentJoin, result, value)
			if algebra.err() != nil {
				return formalRelationTuple{}
			}
		}
	}
	if seedCount != wantSeeds || feedbackCount != wantFeedback {
		algebra.fail(fmt.Errorf(
			"transformer: formal Resource operand row is incomplete (seed=%d/%d feedback=%d/%d)",
			seedCount, wantSeeds, feedbackCount, wantFeedback,
		))
		return formalRelationTuple{}
	}
	return algebra.normalize(result)
}
