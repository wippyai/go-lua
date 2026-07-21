package transformer

import "fmt"

// projectLocalNonreturning transports one reachable complete-product tuple to
// its lexical body's nonreturning route. The route is an equation cell, not a
// product coordinate: Care, guards, and every registered factor therefore
// retain exact structural identity. Apply nonreturning composition is a
// different operation because it must instantiate a callee tuple at one exact
// caller Site; it remains fail-closed until the canonical Apply boundary owns
// that transaction.
func (a *formalTupleAlgebra) projectLocalNonreturning(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || operator.kind != formalRelationCellNonreturning || operator.code == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal local nonreturning operator is malformed")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() {
		return formalRelationTuple{}, nil
	}
	_, _, authority, ok := a.span(predecessor.variable)
	if !ok || authority.code != operator.code {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal local nonreturning predecessor has foreign ownership")
	}
	care, err := a.care(predecessor)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if care == decisionFalse {
		return formalRelationTuple{}, nil
	}
	return predecessor, nil
}

// applyNonreturning executes the same registered boundary-factor transaction
// as normal Apply, but with the terminal's declared result-publication arm
// absent. Mutated roots, heap/identity/path factors, typestate obligations and
// every other registered lane still cross the exact call boundary; diagnostics
// use the canonical callee-carry role owned by nonreturning application.
func (a *formalTupleAlgebra) applyNonreturning(
	operator formalRelationOperatorRef,
	predecessor, target formalRelationTuple,
) (formalRelationTuple, error) {
	if a == nil || operator.kind != formalRelationCellStep || operator.code == nil || operator.root == 0 ||
		int(operator.root) >= len(operator.code.nodes) || operator.step == 0 ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) ||
		!operator.apply.validFor(a.program, predecessor.variable) ||
		!operator.apply.validForFootprint(a.program, operator.footprint) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal nonreturning Apply operator is malformed")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, fmt.Errorf("transformer: formal nonreturning Apply predecessor is Bottom")
	}
	if err := a.validateTuple(target); err != nil || target.bottom() {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, fmt.Errorf("transformer: formal nonreturning Apply target is Bottom")
	}
	_, _, callerAuthority, callerOK := a.span(predecessor.variable)
	if !callerOK || callerAuthority.code != operator.code || operator.apply.owner != predecessor.variable ||
		target.variable != operator.apply.target {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal nonreturning Apply operand has foreign ownership")
	}
	// The nonreturning terminal crosses one live callee product before any N5
	// outcome occurrence exists.  Use the shared Apply correlation primitive
	// that preserves that product without imposing normal-return occurrence
	// publication.
	regions, err := a.formalApplyCorrelatedTargetRegions(operator, predecessor, []formalRelationTuple{target})
	if err != nil {
		return formalRelationTuple{}, err
	}
	publications := make([]formalApplyLeafPublication, 0, len(regions))
	for _, region := range regions {
		publication, reachable, regionErr := a.formalApplyRegionPublication(operator.apply, operator.footprint, region, formalApplyTerminalNonreturning)
		if regionErr != nil {
			return formalRelationTuple{}, regionErr
		}
		if reachable {
			publications = append(publications, publication)
		}
	}
	if len(publications) == 0 {
		return formalRelationTuple{}, nil
	}
	return a.publishFormalApplyLeafPublications(predecessor, publications)
}
