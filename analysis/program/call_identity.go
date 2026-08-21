package program

import (
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// CallIdentityAt is the canonical pre-Artifact Call identity join. It reads
// only the existing Flow/Static scalar inputs for one authored Call and
// delegates the identity equation to the published schema codec; Program
// retains no Call identity table of its own.
func (input *Program) CallIdentityAt(index int) (programschema.CallIdentitySet, bool) {
	if !input.Available() || index < 0 {
		return programschema.CallIdentitySet{}, false
	}
	flowView := input.Flow()
	calls := flowView.Authored().Calls()
	call, callOK := calls.At(index)
	owner, callee, receiver, actuals, rowOK := calls.Get(call)
	bodyBoundary, bodyOK := flowView.FunctionBoundaries().ForBody(owner)
	bodyContext := bodyBoundary.ContextID()
	spanID, _, _, spanOK := input.EvaluationSpan(call)
	valuesID, valuesOK := flowView.ValuesOccurrenceID(actuals)
	contracts := input.Static().Contracts().Calls()
	typeCount, typeCountOK := contracts.TypeArgumentCount(call)
	typeArguments, typeArgumentsOK := contracts.TypeArgumentID(call)
	form := programschema.CallFormPlain
	if receiver != 0 {
		form = programschema.CallFormMethod
	}
	if !callOK || !rowOK || !bodyOK || !bodyBoundary.Available() || !bodyContext.Available() || !spanOK || !spanID.Available() ||
		!valuesOK || !valuesID.Available() || !typeCountOK || typeCount < 0 || !typeArgumentsOK || !typeArguments.Available() {
		return programschema.CallIdentitySet{}, false
	}
	identities, identitiesOK := programschema.CallIdentities(programschema.CallIdentityInput{
		ProgramID: input.ContentID(), Call: call, Form: form, Body: bodyContext, Span: spanID,
		Callee: callee, Receiver: receiver, Actuals: actuals, Values: valuesID,
		TypeArgumentCount: typeCount, TypeArguments: typeArguments,
	})
	return identities, identitiesOK && identities.Call.Available()
}
