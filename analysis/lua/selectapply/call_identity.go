package selectapply

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// callIdentityAt is the pre-Artifact site join used by select specialization.
// Lua lowering runs before Artifact publication, so it consumes only the
// canonical Flow/Static scalar inputs and the schema-owned pure equation.
func callIdentityAt(input *program.Program, index int) (identity.ContentID, bool) {
	if input == nil || !input.Available() || index < 0 {
		return identity.ContentID{}, false
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
		return identity.ContentID{}, false
	}
	identities, identitiesOK := programschema.CallIdentities(programschema.CallIdentityInput{
		ProgramID: input.ContentID(), Call: call, Form: form, Body: bodyContext, Span: spanID,
		Callee: callee, Receiver: receiver, Actuals: actuals, Values: valuesID,
		TypeArgumentCount: typeCount, TypeArguments: typeArguments,
	})
	return identities.Call, identitiesOK && identities.Call.Available()
}
