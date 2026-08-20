package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// copyCallTargetsFailure captures the exact closure-allocation mapping once
// from the canonical Flow allocation and function-boundary rows. Call later
// consumes only these immutable IDs and never scans Program construction state.
func (compiler *compiler) copyCallTargetsFailure() CompileFailure {
	if compiler == nil || len(compiler.bodies) == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	bodyByContext := make(map[identity.ContentID]programschema.Body, len(compiler.bodies))
	for index, body := range compiler.bodies {
		if !body.Available() || !body.ContextID().Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		bodyByContext[body.ContextID()] = body
	}
	rows := make([]programschema.CallTarget, 0)
	seenAllocations := make(map[identity.ContentID]struct{})
	seenBodies := make(map[identity.ContentID]struct{})
	flowView := compiler.input.Flow()
	boundaries := flowView.FunctionBoundaries()
	for index, allocation := range compiler.allocationRows {
		if allocation.role != allocationClosure {
			continue
		}
		boundary, boundaryOK := boundaries.For(allocation.term)
		functionTerm, functionTermOK := boundary.Function()
		bodyTerm, bodyTermOK := boundary.Body()
		body, bodyOK := compiler.input.Body(bodyTerm)
		_, functionOK := body.Function()
		formal, formalOK := flowView.CallBodyTarget(boundary)
		allocationID, bodyID := allocation.template, body.PathID()
		context := body.ContextID()
		copied, copiedOK := bodyByContext[context]
		functionID, functionIDOK := copied.FunctionContextID()
		formalID, formalIDOK := copied.CallFormalID()
		flowFormalID, flowFormalIDOK := formal.ID()
		owner, authoredBody, _, authoredOK := flowView.Authored().Functions().Get(allocation.term)
		if !boundaryOK || !boundaries.OwnsFunction(boundary) || !functionTermOK || functionTerm != allocation.term || !bodyTermOK || owner == 0 || authoredBody != bodyTerm || !authoredOK || !bodyOK || !functionOK || !functionIDOK || !formalOK || !formalIDOK || !flowFormalIDOK || !allocationID.Available() || !bodyID.Available() || !context.Available() || !functionID.Available() || !formalID.Available() || !copiedOK || !copied.Callable() || copied.ID() != bodyID || copied.ContextID() != context || formalID != flowFormalID {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenAllocations[allocationID]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenBodies[context]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenAllocations[allocationID], seenBodies[context] = struct{}{}, struct{}{}
		rows = append(rows, programschema.CallTarget{Allocation: allocationID, Body: bodyID, Context: context, Function: functionID, Formal: formalID})
	}
	compiler.callTargets = rows
	return CompileFailure{}
}
