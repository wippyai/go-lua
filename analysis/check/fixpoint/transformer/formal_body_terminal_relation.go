package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// formalBodyTerminalRelation is the sole post-fixpoint result of one lexical
// body. Normal terminals remain in their frozen outcome order for projection,
// while joined is their exact correlated union with the nonreturning terminal.
// It owns no route, caller, concrete State, or summary DTO.
type formalBodyTerminalRelation struct {
	owner        relationVar
	normal       []formalRelationTuple
	nonreturning formalRelationTuple
	joined       formalRelationTuple
}

func (r formalBodyTerminalRelation) validFor(execution *formalRelationExecution) bool {
	if execution == nil || execution.algebra == nil || execution.algebra.program == nil || r.owner == 0 ||
		int(r.owner) > len(execution.algebra.program.bodies) || r.joined.variable != r.owner && !r.joined.bottom() {
		return false
	}
	if !r.nonreturning.bottom() && r.nonreturning.variable != r.owner {
		return false
	}
	for _, terminal := range r.normal {
		if !terminal.bottom() && terminal.variable != r.owner {
			return false
		}
	}
	return true
}

// bodyTerminalRelation folds the completed terminal cells exactly once after
// the WTO solve. Bottom is the identity, so unreachable outcomes contribute
// nothing. The tuple join retains Care, outcome occurrence, and every product
// fiber in one decision relation; it never evaluates an equation or invokes a
// transfer.
func (e *formalRelationExecution) bodyTerminalRelation(ctx context.Context, bodyID lexicalidentity.StableLexicalBodyID) (formalBodyTerminalRelation, error) {
	if ctx == nil || e == nil || e.algebra == nil || e.algebra.program == nil || e.values == nil {
		return formalBodyTerminalRelation{}, fmt.Errorf("transformer: formal body terminal relation is unowned")
	}
	if err := ctx.Err(); err != nil {
		return formalBodyTerminalRelation{}, err
	}
	program := e.algebra.program
	owner, present := program.byBody[bodyID]
	if !present || owner == 0 || int(owner) > len(program.formalRegion.outcomes) || int(owner) > len(program.formalRegion.nonreturning) {
		return formalBodyTerminalRelation{}, fmt.Errorf("transformer: formal body terminal relation has no body %s", bodyID)
	}
	relation := formalBodyTerminalRelation{owner: owner, normal: make([]formalRelationTuple, len(program.formalRegion.outcomes[owner-1]))}
	mark := e.algebra.decisions.checkpoint()
	fail := func(err error) (formalBodyTerminalRelation, error) {
		e.algebra.decisions.rollback(mark)
		return formalBodyTerminalRelation{}, err
	}
	join := func(tuple formalRelationTuple) error {
		if tuple.bottom() {
			return nil
		}
		if tuple.variable != owner {
			return errFormalComponentForeignOwner
		}
		relation.joined = e.algebra.combine(formalComponentJoin, relation.joined, tuple)
		return e.algebra.err()
	}
	for index, cell := range program.formalRegion.outcomes[owner-1] {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		tuple, exact := e.values[cell]
		if !exact {
			return fail(fmt.Errorf("transformer: formal body terminal outcome %d is absent", index+1))
		}
		relation.normal[index] = tuple
		if err := join(tuple); err != nil {
			return fail(err)
		}
	}
	nonreturningCell := program.formalRegion.nonreturning[owner-1]
	nonreturning, exact := e.values[nonreturningCell]
	if !exact {
		return fail(fmt.Errorf("transformer: formal body nonreturning terminal is absent"))
	}
	relation.nonreturning = nonreturning
	if err := join(nonreturning); err != nil {
		return fail(err)
	}
	if !relation.validFor(e) {
		return fail(fmt.Errorf("transformer: formal body terminal relation failed closure"))
	}
	return relation, nil
}
