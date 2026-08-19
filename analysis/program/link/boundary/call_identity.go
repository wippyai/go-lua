package boundary

import (
	"github.com/wippyai/go-lua/analysis/program"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// boundaryCallIdentitiesAt is the narrow pre-Artifact join required while
// Boundary seals its mounted semantic value directory. It consumes canonical
// Flow/Static rows and delegates every scalar equation to the schema codec;
// no Program call-identity API or Boundary-local identity table is retained.
func boundaryCallIdentitiesAt(input *program.Program, index int) (programschema.CallIdentitySet, bool) {
	if input == nil || !input.Available() || index < 0 {
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
