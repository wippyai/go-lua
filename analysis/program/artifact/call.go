package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// CallOperandKind is the closed role of one authored Call operand.  The
// artifact copy deliberately uses the Artifact vocabulary rather than
// exporting the transformer proof type.
type CallOperandKind uint8

const (
	CallOperandInvalid CallOperandKind = iota
	CallOperandCallee
	CallOperandReceiver
	CallOperandActuals
)

func (kind CallOperandKind) valid() bool {
	return kind >= CallOperandCallee && kind <= CallOperandActuals
}

// CallOperandRow is one immutable, pointer-free Call operand. ID is the
// semantic operand identity issued by Program; ValueID is the corresponding
// reusable value identity when the operand is a Values member/root. SpanID is
// retained so consumers can authenticate the exact mounted Value without
// reopening Program or reconstructing an authored Term.
type CallOperandRow struct {
	id     identity.ContentID
	call   identity.ContentID
	value  identity.ContentID
	span   identity.ContentID
	kind   CallOperandKind
	sealed bool
}

func (row CallOperandRow) Available() bool {
	return row.sealed && row.id.Available() && row.call.Available() && row.value.Available() && row.span.Available() && row.kind.valid()
}
func (row CallOperandRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallOperandRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row CallOperandRow) ValueID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.value
}
func (row CallOperandRow) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}
func (row CallOperandRow) Kind() CallOperandKind {
	if !row.Available() {
		return CallOperandInvalid
	}
	return row.kind
}

// CallArgumentRow is one immutable ordered actual argument. ValueID is the
// argument's semantic identity; MemberID and ValuesID retain the exact
// parent/value joins needed by mounted Pack and Link consumers.
type CallArgumentRow struct {
	id       identity.ContentID
	call     identity.ContentID
	values   identity.ContentID
	member   identity.ContentID
	span     identity.ContentID
	position uint32
	sealed   bool
}

func (row CallArgumentRow) Available() bool {
	return row.sealed && row.id.Available() && row.call.Available() && row.values.Available() && row.member.Available() && row.span.Available()
}
func (row CallArgumentRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallArgumentRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row CallArgumentRow) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}
func (row CallArgumentRow) MemberID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.member
}
func (row CallArgumentRow) ValueID() identity.ContentID { return row.MemberID() }
func (row CallArgumentRow) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}
func (row CallArgumentRow) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}

// CallTypeArgumentRow is one immutable ordered Static type argument. The
// reference identity is the only type payload that crosses the Artifact
// boundary; no Static Term or transformer proof is retained.
type CallTypeArgumentRow struct {
	id        identity.ContentID
	call      identity.ContentID
	types     identity.ContentID
	reference identity.ContentID
	position  uint32
	sealed    bool
}

func (row CallTypeArgumentRow) Available() bool {
	return row.sealed && row.id.Available() && row.call.Available() && row.types.Available() && row.reference.Available()
}
func (row CallTypeArgumentRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallTypeArgumentRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row CallTypeArgumentRow) TypesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}
func (row CallTypeArgumentRow) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}
func (row CallTypeArgumentRow) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}

// CallRow is the complete immutable authored-call row. Its ranges index the
// Artifact-owned operand, argument, and type-argument columns; mounted code
// consumes these scalar IDs and never needs transient Program proofs.
type CallRow struct {
	id         identity.ContentID
	body       identity.ContentID
	span       identity.ContentID
	formal     identity.ContentID
	values     identity.ContentID
	valuesRoot identity.ContentID
	types      identity.ContentID
	callee     identity.ContentID
	actuals    identity.ContentID
	receiver   identity.ContentID
	tail       identity.ContentID
	operandStart,
	operandEnd uint32
	argumentStart,
	argumentEnd uint32
	typeArgumentStart,
	typeArgumentEnd uint32
	form        flow.CallForm
	hasReceiver bool
	hasTail     bool
	sealed      bool
}

func (row CallRow) Available() bool {
	if !row.sealed || !row.id.Available() || !row.body.Available() || !row.span.Available() || !row.formal.Available() ||
		!row.values.Available() || !row.valuesRoot.Available() || !row.types.Available() || !row.callee.Available() || !row.actuals.Available() ||
		(row.form != flow.CallFormPlain && row.form != flow.CallFormMethod) || row.hasReceiver != row.receiver.Available() ||
		(row.form == flow.CallFormMethod) != row.hasReceiver || row.hasTail != row.tail.Available() ||
		row.argumentEnd < row.argumentStart || row.operandEnd < row.operandStart || row.typeArgumentEnd < row.typeArgumentStart {
		return false
	}
	return true
}
func (row CallRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row CallRow) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}
func (row CallRow) FormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

// ValuesID is the CallValues semantic identity used by Pack/Link joins.
func (row CallRow) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}

// ValuesRootID is the Artifact Values row identity for the actual-values root.
func (row CallRow) ValuesRootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesRoot
}
func (row CallRow) TypeArgumentsID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}
func (row CallRow) CalleeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.callee
}
func (row CallRow) ActualsID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.actuals
}
func (row CallRow) Form() flow.CallForm {
	if !row.Available() {
		return 0
	}
	return row.form
}
func (row CallRow) ReceiverID() (identity.ContentID, bool) {
	return row.receiver, row.Available() && row.hasReceiver
}
func (row CallRow) TailID() (identity.ContentID, bool) {
	return row.tail, row.Available() && row.hasTail
}
func (row CallRow) OperandCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.operandEnd - row.operandStart)
}
func (row CallRow) ArgumentCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.argumentEnd - row.argumentStart)
}
func (row CallRow) TypeArgumentCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.typeArgumentEnd - row.typeArgumentStart)
}

func (compiler *compiler) copyCallRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	calls := compiler.input.Flow().Authored().Calls().Count()
	compiler.calls = make([]CallRow, 0, calls)
	compiler.callOperands = compiler.callOperands[:0]
	compiler.callArguments = compiler.callArguments[:0]
	compiler.callTypeArguments = compiler.callTypeArguments[:0]
	for index := 0; index < calls; index++ {
		call, ok := compiler.callConstruction(index)
		if !ok {
			continue
		}
		row := CallRow{id: call.id, body: call.bodyPath, span: call.span, formal: call.formal, values: call.values, valuesRoot: call.valuesRoot, types: call.types, form: call.form, tail: identity.ContentID{},
			operandStart: uint32(len(compiler.callOperands)), argumentStart: uint32(len(compiler.callArguments)), typeArgumentStart: uint32(len(compiler.callTypeArguments)), sealed: true}
		if call.tail.Available() {
			row.tail, row.hasTail = call.tail, true
		}
		appendOperand := func(operand callOperandConstruction) bool {
			value := CallOperandRow{id: operand.id, call: call.id, value: operand.id, span: operand.span, kind: operand.kind, sealed: true}
			if !value.Available() {
				return false
			}
			compiler.callOperands = append(compiler.callOperands, value)
			return true
		}
		if !appendOperand(call.callee) || !appendOperand(call.actuals) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		row.callee, row.actuals = call.callee.id, call.actuals.id
		if call.receiver.id.Available() {
			if !appendOperand(call.receiver) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
			}
			row.receiver, row.hasReceiver = call.receiver.id, true
		}
		for argumentIndex, argument := range call.arguments {
			argumentRow := CallArgumentRow{id: argument.id, call: call.id, values: call.values, member: argument.member, span: argument.span, position: uint32(argumentIndex), sealed: true}
			if !argumentRow.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			compiler.callArguments = append(compiler.callArguments, argumentRow)
		}
		for typeIndex, argument := range call.typeArguments {
			argumentRow := CallTypeArgumentRow{id: argument.id, call: call.id, types: call.types, reference: argument.reference, position: uint32(typeIndex), sealed: true}
			if !argumentRow.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
			compiler.callTypeArguments = append(compiler.callTypeArguments, argumentRow)
		}
		row.operandEnd = uint32(len(compiler.callOperands))
		row.argumentEnd = uint32(len(compiler.callArguments))
		row.typeArgumentEnd = uint32(len(compiler.callTypeArguments))
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		compiler.calls = append(compiler.calls, row)
	}
	return CompileFailure{}
}

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
	form          flow.CallForm
	executable    bool
	boundary      callBoundaryConstruction
	entry         flow.Site
	finish        flow.Site
}

type callOperandConstruction struct {
	id   identity.ContentID
	span identity.ContentID
	kind CallOperandKind
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
	kind   flow.BoundaryArmKind
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
	form := flow.CallFormPlain
	if receiverTerm != 0 {
		form = flow.CallFormMethod
	}
	callee, calleeOK := compiler.callOperand(index, term, calleeTerm, CallOperandCallee)
	receiver := callOperandConstruction{}
	receiverOK := true
	if receiverTerm != 0 {
		receiver, receiverOK = compiler.callOperand(index, term, receiverTerm, CallOperandReceiver)
	}
	actuals, actualsOK := compiler.callOperand(index, term, actualsTerm, CallOperandActuals)
	if !calleeOK || !receiverOK || !actualsOK {
		return callConstruction{}, false
	}

	valuesRoot, members, tail, width, valuesOK := compiler.callValues(actualsTerm)
	if !valuesOK || width != len(members) || !valuesRoot.Available() {
		return callConstruction{}, false
	}
	valuesSemanticID, valuesSemanticOK := compiler.input.CallValuesIDAt(index)
	if !valuesSemanticOK || !valuesSemanticID.Available() {
		return callConstruction{}, false
	}

	arguments := make([]callArgumentConstruction, width)
	authoredValues := flowView.Authored().Values()
	for argumentIndex := 0; argumentIndex < width; argumentIndex++ {
		memberTerm, memberOK := authoredValues.Member(actualsTerm, argumentIndex)
		memberSpan, _, _, memberSpanOK := compiler.input.EvaluationSpan(memberTerm)
		memberID := members[argumentIndex]
		argumentID, argumentOK := compiler.input.CallArgumentIDAt(index, argumentIndex)
		if !memberOK || !memberSpanOK || !memberID.Available() || !argumentOK || !argumentID.Available() {
			return callConstruction{}, false
		}
		arguments[argumentIndex] = callArgumentConstruction{id: argumentID, member: memberID, span: memberSpan}
	}

	contracts := compiler.input.Static().Contracts().Calls()
	typeCount, typeCountOK := contracts.TypeArgumentCount(term)
	if !typeCountOK || typeCount < 0 {
		return callConstruction{}, false
	}
	typesID, typesOK := compiler.input.CallTypeArgumentsIDAt(index)
	if !typesOK || !typesID.Available() {
		return callConstruction{}, false
	}
	typeArguments := make([]callTypeArgumentConstruction, typeCount)
	staticTypes := compiler.input.Static().StaticTypes()
	for typeIndex := 0; typeIndex < typeCount; typeIndex++ {
		typeTerm, typeOK := contracts.TypeArgumentAt(term, typeIndex)
		ref, refOK := staticTypes.Ref(typeTerm)
		referenceID, referenceOK := program.StaticTypeReferenceID(compiler.input.ContentID(), ref)
		argumentID, argumentOK := compiler.input.CallTypeArgumentIDAt(index, typeIndex)
		if !typeOK || !refOK || ref.Term() != typeTerm || !referenceOK || !referenceID.Available() || !argumentOK || !argumentID.Available() {
			return callConstruction{}, false
		}
		typeArguments[typeIndex] = callTypeArgumentConstruction{id: argumentID, reference: referenceID, term: typeTerm}
	}
	formalID, formalOK := compiler.input.CallFormalIDAt(index)
	if !formalOK || !formalID.Available() {
		return callConstruction{}, false
	}
	callID, callOK := compiler.input.CallIDAt(index)
	if !callOK || !callID.Available() {
		return callConstruction{}, false
	}
	result := callConstruction{term: term, owner: owner, id: callID, bodyPath: bodyPath, span: spanID,
		formal: formalID, values: valuesSemanticID, valuesRoot: valuesRoot, types: typesID, callee: callee, receiver: receiver, actuals: actuals,
		arguments: arguments, typeArguments: typeArguments, tail: tail, form: form, executable: flowView.Executable().Contains(term), entry: entry, finish: finish}
	result.boundary = compiler.callBoundary(term, spanID)
	if result.executable && !result.boundary.id.Available() {
		return callConstruction{}, false
	}
	return result, true
}

func (compiler *compiler) callOperand(index int, call, term keyspace.Term, kind CallOperandKind) (callOperandConstruction, bool) {
	span, _, _, spanOK := compiler.input.EvaluationSpan(term)
	id := identity.ContentID{}
	var idOK bool
	switch kind {
	case CallOperandCallee:
		id, idOK = compiler.input.CallCalleeIDAt(index)
	case CallOperandReceiver:
		id, idOK = compiler.input.CallReceiverIDAt(index)
	case CallOperandActuals:
		id, idOK = compiler.input.CallActualsIDAt(index)
	}
	operand := callOperandConstruction{id: id, span: span, kind: kind, term: term}
	return operand, spanOK && idOK && id.Available() && kind.valid()
}

func (compiler *compiler) callSpan(term keyspace.Term) (flow.Site, flow.Site, identity.ContentID, bool) {
	flowView := compiler.input.Flow()
	spanID, entryTerm, finishTerm, spanOK := compiler.input.EvaluationSpan(term)
	sites := flowView.Causal().Sites()
	entry, entrySiteOK := sites.ForTerm(entryTerm)
	finish, finishSiteOK := sites.ForTerm(finishTerm)
	if !spanOK || !entrySiteOK || !finishSiteOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
		return flow.Site{}, flow.Site{}, identity.ContentID{}, false
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
	rowID, rowOK := compiler.input.ValuesOccurrenceID(term)
	if !rowOK || !rowID.Available() {
		return identity.ContentID{}, nil, identity.ContentID{}, 0, false
	}
	members := make([]identity.ContentID, width)
	for index := range members {
		memberID, memberOK := compiler.input.ValuesMemberID(term, index)
		members[index] = memberID
		if !memberOK || !memberID.Available() {
			return identity.ContentID{}, nil, identity.ContentID{}, 0, false
		}
	}
	tail := identity.ContentID{}
	if tailTerm != 0 {
		var tailOK bool
		tail, tailOK = compiler.input.ValuesTailID(term)
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
	for _, armKind := range [...]flow.BoundaryArmKind{flow.BoundaryResume, flow.BoundarySelectTrue, flow.BoundarySelectFalse, flow.BoundaryTail, flow.BoundaryThrow, flow.BoundaryYield, flow.BoundaryCancel} {
		successor, armOK := flowView.Causal().Boundaries().Arm(term, armKind)
		if !armOK || successor.From != term || successor.Arm != armKind {
			continue
		}
		routeIdentity, routeOK := successor.Identity()
		target, targetOK := flowView.Causal().Sites().ForTerm(successor.To)
		routeDigest := routeIdentity.Digest()
		targetID := target.ContextID()
		if !routeOK || !targetOK || !routeIdentity.Available() || routeIdentity.Provenance() != flowView.Provenance() || !compiler.input.OwnsSite(target) || !routeDigest.Available() || !targetID.Available() {
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
	boundaryID := artifactTransformerRoleID("program/transformer/call-boundary", compiler.input.ContentID(), func(writer *framing.Writer) bool {
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
		arm.id = artifactTransformerRoleID("program/transformer/call-arm", compiler.input.ContentID(), func(writer *framing.Writer) bool {
			return writer.Bytes(boundaryID[:]) == nil && writer.Uint(uint64(arm.kind)) == nil && writer.Bytes(arm.route[:]) == nil && writer.Bytes(arm.target[:]) == nil
		})
		if !arm.id.Available() {
			return callBoundaryConstruction{}
		}
	}
	return callBoundaryConstruction{id: boundaryID, arms: arms}
}

func artifactTransformerRoleID(domain string, owner identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
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
