package project

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// callIdentityAt is the pre-Artifact Project admission join. Project must
// recognize executable authored Calls before an Artifact exists, so it reads
// the canonical Flow/Static scalar inputs once and delegates the identity
// equation to the published schema codec.
func callIdentityAt(input *program.Program, index int) (identity.ContentID, bool) {
	if input == nil || !input.Available() || index < 0 {
		return identity.ContentID{}, false
	}
	flowView := input.Flow()
	calls := flowView.Authored().Calls()
	call, callOK := calls.At(index)
	owner, callee, receiver, actuals, rowOK := calls.Get(call)
	if !callOK || !rowOK {
		return identity.ContentID{}, false
	}
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
	if !bodyOK || !bodyBoundary.Available() || !bodyContext.Available() || !spanOK || !spanID.Available() ||
		!valuesOK || !valuesID.Available() || !typeCountOK || typeCount < 0 || !typeArgumentsOK || !typeArguments.Available() {
		return identity.ContentID{}, false
	}
	identities, identitiesOK := programschema.CallIdentities(programschema.CallIdentityInput{
		ProgramID: input.ContentID(), Call: call, Form: form, Body: bodyContext, Span: spanID,
		Callee: callee, Receiver: receiver, Actuals: actuals, Values: valuesID,
		TypeArgumentCount: typeCount, TypeArguments: typeArguments,
	})
	return identities.Call, identitiesOK && identities.Call.Available()
}
