package compiler

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/internal/framing"
)

// callConstruction is the short-lived scalar census used by both call
// columns and call occurrences.  It deliberately contains no Program proof
// object: every field is either an authored coordinate used during this cold
// pass or an identity copied into an Artifact row.
type callConstruction struct {
	term          keyspace.Term
	owner         keyspace.Term
	id            identity.ContentID
	bodyPath      identity.ContentID
	span          identity.ContentID
	formal        identity.ContentID
	values        identity.ContentID
	valuesRoot    identity.ContentID
	types         identity.ContentID
	callee        callOperandConstruction
	receiver      callOperandConstruction
	actuals       callOperandConstruction
	arguments     []callArgumentConstruction
	typeArguments []callTypeArgumentConstruction
	tail          identity.ContentID
	targetBody    identity.ContentID
	form          accessgeometry.CallForm
	executable    bool
	boundary      callBoundaryConstruction
	entry         causal.Site
	finish        causal.Site
}

type callOperandConstruction struct {
	id   identity.ContentID
	span identity.ContentID
	kind programschema.CallOperandKind
	term keyspace.Term
}

type callArgumentConstruction struct {
	id     identity.ContentID
	member identity.ContentID
	span   identity.ContentID
}

type callTypeArgumentConstruction struct {
	id        identity.ContentID
	reference identity.ContentID
	term      keyspace.Term
}

type callBoundaryConstruction struct {
	id   identity.ContentID
	arms []callArmConstruction
}

type callArmConstruction struct {
	id     identity.ContentID
	kind   causal.BoundaryArmKind
	route  identity.ContentID
	target identity.ContentID
	points []identity.ContentID
}

// callConstruction reads only the canonical Flow/Authored, Source, Static,
// and Flow causal query surfaces.  It is intentionally shared by the row and
// occurrence stages so an ID can never drift between those two projections.
func (compiler *compiler) callConstruction(index int) (callConstruction, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 {
		return callConstruction{}, false
	}
	flowView := compiler.input.Flow()
	calls := flowView.Authored().Calls()
	term, termOK := calls.At(index)
	owner, calleeTerm, receiverTerm, actualsTerm, rowOK := calls.Get(term)
	if !termOK || !rowOK || !validArtifactCallTerm(term) || !validArtifactBodyTerm(owner) || !validArtifactValueTerm(actualsTerm) ||
		!validArtifactTerm(calleeTerm) || (receiverTerm != 0 && !validArtifactTerm(receiverTerm)) {
		return callConstruction{}, false
	}

	// Source is the canonical lexical containment denominator.  Flow's call
	// owner is required to agree with it before any body scalar is copied.
	sourceOwner, _, _, sourceOK := compiler.input.Source().Index().Position(term)
	if !sourceOK || sourceOwner != owner {
		return callConstruction{}, false
	}
	boundaries := flowView.FunctionBoundaries()
	bodyBoundary, bodyOK := boundaries.ForBody(owner)
	bodyTerm, bodyTermOK := bodyBoundary.Body()
	bodyPath, bodyPathOK := flowView.BodyPath(owner)
	if !bodyOK || !bodyBoundary.Available() || !bodyTermOK || bodyTerm != owner || !bodyPathOK || !bodyPath.Available() {
		return callConstruction{}, false
	}

	entry, finish, spanID, spanOK := compiler.callSpan(term)
	if !spanOK {
		return callConstruction{}, false
	}
	programID := compiler.key.ProgramID()
	// CompileDetailed always supplies the same root through CompileKey. The
	// input fallback keeps this construction seam usable by its direct cold
	// tests, which intentionally omit the surrounding compile-key wrapper.
	if !programID.Available() {
		programID = compiler.input.ContentID()
	}
	form := accessgeometry.CallFormPlain
	if receiverTerm != 0 {
		form = accessgeometry.CallFormMethod
	}
	coldForm, coldFormOK := coldCallForm(form)
	if !coldFormOK {
		return callConstruction{}, false
	}
	contracts := compiler.input.Static().Contracts().Calls()
	typeCount, typeCountOK := contracts.TypeArgumentCount(term)
	typeSequenceID, typeSequenceOK := contracts.TypeArgumentID(term)
	if !typeCountOK || typeCount < 0 || !typeSequenceOK || !typeSequenceID.Available() {
		return callConstruction{}, false
	}
	typeTerms := make([]keyspace.Term, typeCount)
	for typeIndex := range typeTerms {
		typeTerms[typeIndex], typeCountOK = contracts.TypeArgumentAt(term, typeIndex)
		if !typeCountOK {
			return callConstruction{}, false
		}
	}
	callPath, callPathOK := flowView.CallPath(term)
	valuesWidth, valuesOpen, valuesShapeOK := flowView.ValuesShape(actualsTerm)
	if !callPathOK || !callPath.Available() || !valuesShapeOK || valuesWidth < 0 || valuesWidth > int(^uint32(0)) {
		return callConstruction{}, false
	}
	// ValuesOccurrenceID is the final root-call input and is read from Flow's
	// canonical Values owner before the scalar result is admitted.
	valuesSemanticID, valuesSemanticOK := flowView.ValuesOccurrenceID(actualsTerm)
	if !valuesSemanticOK || !valuesSemanticID.Available() {
		return callConstruction{}, false
	}
	identities, identitiesOK := programschema.CallIdentities(programschema.CallIdentityInput{
		ProgramID: programID, Call: term, Form: coldForm, Body: bodyBoundary.ContextID(), Span: spanID,
		Callee: calleeTerm, Receiver: receiverTerm, Actuals: actualsTerm, Values: valuesSemanticID,
		TypeArgumentCount: typeCount, TypeArguments: typeSequenceID, BodyPath: bodyPath, CallPath: callPath,
		ValuesWidth: valuesWidth, ValuesOpen: valuesOpen, TypeArgumentTerms: typeTerms, FormalGeometryKnown: true,
	})
	if !identitiesOK || !identities.Call.Available() || !identities.Formal.Available() ||
		!identities.TypeArguments.Available() || len(identities.TypeArgumentAt) != typeCount {
		return callConstruction{}, false
	}
	callee, calleeOK := compiler.callOperand(term, calleeTerm, programschema.CallOperandCallee, identities.Callee)
	receiver := callOperandConstruction{}
	receiverOK := true
	if receiverTerm != 0 {
		receiver, receiverOK = compiler.callOperand(term, receiverTerm, programschema.CallOperandReceiver, identities.Receiver)
	}
	actuals, actualsOK := compiler.callOperand(term, actualsTerm, programschema.CallOperandActuals, identities.Actuals)
	if !calleeOK || !receiverOK || !actualsOK {
		return callConstruction{}, false
	}

	valuesRoot, members, tail, width, valuesOK := compiler.callValues(actualsTerm)
	if !valuesOK || width != len(members) || !valuesRoot.Available() {
		return callConstruction{}, false
	}
	arguments := make([]callArgumentConstruction, width)
	authoredValues := flowView.Authored().Values()
	for argumentIndex := 0; argumentIndex < width; argumentIndex++ {
		memberTerm, memberOK := authoredValues.Member(actualsTerm, argumentIndex)
		memberSpan, _, _, memberSpanOK := compiler.input.EvaluationSpan(memberTerm)
		memberID := members[argumentIndex]
		argumentID, argumentOK := flowView.CallArgumentID(term, argumentIndex)
		if !memberOK || !memberSpanOK || !memberID.Available() || !argumentOK || !argumentID.Available() {
			return callConstruction{}, false
		}
		arguments[argumentIndex] = callArgumentConstruction{id: argumentID, member: memberID, span: memberSpan}
	}

	typesID := identities.TypeArguments
	typeArguments := make([]callTypeArgumentConstruction, typeCount)
	staticTypes := compiler.input.Static().StaticTypes()
	for typeIndex := 0; typeIndex < typeCount; typeIndex++ {
		typeTerm := typeTerms[typeIndex]
		typeOK := typeTerm != 0
		ref, refOK := staticTypes.Ref(typeTerm)
		referenceID, referenceOK := staticquery.TypeReferenceID(programID, ref)
		argumentID := identities.TypeArgumentAt[typeIndex]
		argumentOK := argumentID.Available()
		if !typeOK || !refOK || ref.Term() != typeTerm || !referenceOK || !referenceID.Available() || !argumentOK || !argumentID.Available() {
			return callConstruction{}, false
		}
		typeArguments[typeIndex] = callTypeArgumentConstruction{id: argumentID, reference: referenceID, term: typeTerm}
	}
	formalID := identities.Formal
	callID := identities.Call
	result := callConstruction{term: term, owner: owner, id: callID, bodyPath: bodyPath, span: spanID,
		formal: formalID, values: valuesSemanticID, valuesRoot: valuesRoot, types: typesID, callee: callee, receiver: receiver, actuals: actuals,
		arguments: arguments, typeArguments: typeArguments, tail: tail, form: form, executable: flowView.Executable().Contains(term), entry: entry, finish: finish}
	if function, functionOK := flowView.DirectFunctions().Call(term); functionOK {
		targetBoundary, targetOK := boundaries.For(function)
		targetBody, targetBodyOK := targetBoundary.Body()
		targetPath, targetPathOK := flowView.BodyPath(targetBody)
		if !targetOK || !targetBodyOK || !targetPathOK || !targetPath.Available() {
			return callConstruction{}, false
		}
		result.targetBody = targetPath
	}
	result.boundary = compiler.callBoundary(term, spanID)
	if result.executable && !result.boundary.id.Available() {
		return callConstruction{}, false
	}
	return result, true
}

func (compiler *compiler) callOperand(call, term keyspace.Term, kind programschema.CallOperandKind, id identity.ContentID) (callOperandConstruction, bool) {
	span, _, _, spanOK := compiler.input.EvaluationSpan(term)
	operand := callOperandConstruction{id: id, span: span, kind: kind, term: term}
	return operand, spanOK && id.Available() && kind.Valid()
}

func (compiler *compiler) callSpan(term keyspace.Term) (causal.Site, causal.Site, identity.ContentID, bool) {
	flowView := compiler.input.Flow()
	spanID, entryTerm, finishTerm, spanOK := compiler.input.EvaluationSpan(term)
	sites := flowView.Causal().Sites()
	entry, entrySiteOK := sites.ForTerm(entryTerm)
	finish, finishSiteOK := sites.ForTerm(finishTerm)
	if !spanOK || !entrySiteOK || !finishSiteOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
		return causal.Site{}, causal.Site{}, identity.ContentID{}, false
	}
	return entry, finish, spanID, spanID.Available()
}

func (compiler *compiler) callValues(term keyspace.Term) (identity.ContentID, []identity.ContentID, identity.ContentID, int, bool) {
	values := compiler.input.Flow().Authored().Values()
	width, widthOK := values.Len(term)
	_, tailTerm, rowOK := values.Get(term)
	if !widthOK || !rowOK || width < 0 {
		return identity.ContentID{}, nil, identity.ContentID{}, 0, false
	}
	rowID, rowOK := compiler.input.Flow().ValuesOccurrenceID(term)
	if !rowOK || !rowID.Available() {
		return identity.ContentID{}, nil, identity.ContentID{}, 0, false
	}
	members := make([]identity.ContentID, width)
	for index := range members {
		memberID, memberOK := compiler.input.Flow().ValuesMemberID(term, index)
		members[index] = memberID
		if !memberOK || !memberID.Available() {
			return identity.ContentID{}, nil, identity.ContentID{}, 0, false
		}
	}
	tail := identity.ContentID{}
	if tailTerm != 0 {
		var tailOK bool
		tail, tailOK = compiler.input.Flow().ValuesTailID(term)
		if !tailOK || !tail.Available() {
			return identity.ContentID{}, nil, identity.ContentID{}, 0, false
		}
	}
	return rowID, members, tail, width, true
}

func (compiler *compiler) callBoundary(term keyspace.Term, spanID identity.ContentID) callBoundaryConstruction {
	flowView := compiler.input.Flow()
	boundary, boundaryOK := flowView.Causal().Boundaries().For(term)
	if !boundaryOK || boundary.Call != term || !spanID.Available() {
		return callBoundaryConstruction{}
	}
	var arms []callArmConstruction
	for _, armKind := range [...]causal.BoundaryArmKind{causal.BoundaryResume, causal.BoundarySelectTrue, causal.BoundarySelectFalse, causal.BoundaryTail, causal.BoundaryThrow, causal.BoundaryYield, causal.BoundaryCancel} {
		successor, armOK := flowView.Causal().Boundaries().Arm(term, armKind)
		if !armOK || successor.From != term || successor.Arm != armKind {
			continue
		}
		routeIdentity, routeOK := successor.Identity()
		target, targetOK := flowView.Causal().Sites().ForTerm(successor.To)
		routeDigest := routeIdentity.Digest
		targetID := target.ContextID()
		if !routeOK || !targetOK || !routeIdentity.Issued() || routeIdentity.Provenance() != flowView.Provenance() || !compiler.input.OwnsSite(target) || !routeDigest.Available() || !targetID.Available() {
			return callBoundaryConstruction{}
		}
		points := compiler.pointIDs(target)
		if len(points) == 0 {
			return callBoundaryConstruction{}
		}
		arms = append(arms, callArmConstruction{kind: armKind, route: routeDigest, target: targetID, points: points})
	}
	if len(arms) == 0 {
		return callBoundaryConstruction{}
	}
	boundaryID := artifactRoleID("program/transformer/call-boundary", compiler.input.ContentID(), func(writer *framing.Writer) bool {
		if writer.Bytes(spanID[:]) != nil || writer.Count(uint64(len(arms))) != nil {
			return false
		}
		for _, arm := range arms {
			if writer.Uint(uint64(arm.kind)) != nil || writer.Bytes(arm.route[:]) != nil || writer.Bytes(arm.target[:]) != nil {
				return false
			}
		}
		return true
	})
	if !boundaryID.Available() {
		return callBoundaryConstruction{}
	}
	for index := range arms {
		arm := &arms[index]
		arm.id = artifactRoleID("program/transformer/call-arm", compiler.input.ContentID(), func(writer *framing.Writer) bool {
			return writer.Bytes(boundaryID[:]) == nil && writer.Uint(uint64(arm.kind)) == nil && writer.Bytes(arm.route[:]) == nil && writer.Bytes(arm.target[:]) == nil
		})
		if !arm.id.Available() {
			return callBoundaryConstruction{}
		}
	}
	return callBoundaryConstruction{id: boundaryID, arms: arms}
}

func artifactRoleID(domain string, owner identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
	if !owner.Available() || write == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(owner[:]) != nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func validArtifactTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0
}

func validArtifactCallTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyCall && keyspace.TermOrdinal(term) != 0
}

func validArtifactBodyTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0
}

func validArtifactValueTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyValues && keyspace.TermOrdinal(term) != 0
}
