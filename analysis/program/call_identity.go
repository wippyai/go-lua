package program

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
	bodyPath, bodyPathOK := flowView.BodyPath(owner)
	callPath, callPathOK := flowView.CallPath(call)
	valuesWidth, valuesOpen, valuesShapeOK := flowView.ValuesShape(actuals)
	var typeTerms []keyspace.Term
	typeTermsOK := typeCountOK && typeCount >= 0
	if typeTermsOK {
		typeTerms = make([]keyspace.Term, typeCount)
		for typeIndex := range typeTerms {
			typeTerms[typeIndex], typeTermsOK = contracts.TypeArgumentAt(call, typeIndex)
			if !typeTermsOK {
				break
			}
		}
	}
	form := programschema.CallFormPlain
	if receiver != 0 {
		form = programschema.CallFormMethod
	}
	if !callOK || !rowOK || !bodyOK || !bodyBoundary.Available() || !bodyContext.Available() || !spanOK || !spanID.Available() ||
		!valuesOK || !valuesID.Available() || !typeCountOK || typeCount < 0 || !typeArgumentsOK || !typeArguments.Available() {
		return programschema.CallIdentitySet{}, false
	}
	formalGeometryKnown := bodyPathOK && callPathOK && valuesShapeOK && valuesWidth >= 0 &&
		uint64(valuesWidth) <= uint64(^uint32(0)) && typeTermsOK
	identities, identitiesOK := programschema.CallIdentities(programschema.CallIdentityInput{
		ProgramID: input.ContentID(), Call: call, Form: form, Body: bodyContext, Span: spanID,
		Callee: callee, Receiver: receiver, Actuals: actuals, Values: valuesID,
		TypeArgumentCount: typeCount, TypeArguments: typeArguments, BodyPath: bodyPath, CallPath: callPath,
		ValuesWidth: valuesWidth, ValuesOpen: valuesOpen, TypeArgumentTerms: typeTerms,
		FormalGeometryKnown: formalGeometryKnown,
	})
	complete := identitiesOK && identities.Call.Available() && identities.TypeArguments.Available() &&
		identities.Formal.Available() && len(identities.TypeArgumentAt) == typeCount
	if complete {
		for _, argumentID := range identities.TypeArgumentAt {
			if !argumentID.Available() {
				complete = false
				break
			}
		}
	}
	return identities, complete
}
