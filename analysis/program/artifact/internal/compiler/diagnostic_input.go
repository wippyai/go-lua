package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// CompileFailure is the child compiler's private failure transport.  It is
// deliberately independent of artifact's public diagnostic type: the child
// cannot import its parent without creating a package cycle.
type CompileFailure struct {
	stage  CompileStage
	kind   CompileRowKind
	reason CompileReason
	row    int
	subrow int
}

type CompileStage string

const (
	CompileStageAuthority    CompileStage = "authority"
	CompileStageValues       CompileStage = "values"
	CompileStageBodyOutcomes CompileStage = "body-outcomes"
	CompileStageRoutes       CompileStage = "routes"
	CompileStageOccurrences  CompileStage = "occurrences"
)

type CompileRowKind string

const (
	CompileRowAuthority   CompileRowKind = "authority"
	CompileRowValues      CompileRowKind = "values"
	CompileRowBody        CompileRowKind = "body"
	CompileRowOutcome     CompileRowKind = "outcome"
	CompileRowReturnValue CompileRowKind = "return-value"
	CompileRowRoute       CompileRowKind = "route"
	CompileRowOccurrence  CompileRowKind = "occurrence"
)

type CompileReason string

const (
	CompileReasonProgramUnavailable     CompileReason = "program-unavailable"
	CompileReasonValuesUnavailable      CompileReason = "values-unavailable"
	CompileReasonValuesBody             CompileReason = "values-body"
	CompileReasonValuesIdentity         CompileReason = "values-identity"
	CompileReasonValuesMember           CompileReason = "values-member"
	CompileReasonValuesTail             CompileReason = "values-tail"
	CompileReasonValuesDuplicate        CompileReason = "values-duplicate"
	CompileReasonBodyUnavailable        CompileReason = "body-unavailable"
	CompileReasonBodyForeign            CompileReason = "body-foreign"
	CompileReasonBodyIdentity           CompileReason = "body-identity"
	CompileReasonBodyDuplicate          CompileReason = "body-duplicate"
	CompileReasonBodyRange              CompileReason = "body-range"
	CompileReasonOutcomeUnavailable     CompileReason = "outcome-unavailable"
	CompileReasonOutcomeAttachment      CompileReason = "outcome-attachment"
	CompileReasonOutcomeShape           CompileReason = "outcome-shape"
	CompileReasonOutcomeForeign         CompileReason = "outcome-foreign"
	CompileReasonOutcomeIdentity        CompileReason = "outcome-identity"
	CompileReasonOutcomeDuplicate       CompileReason = "outcome-duplicate"
	CompileReasonOutcomeKind            CompileReason = "outcome-kind"
	CompileReasonOutcomeTarget          CompileReason = "outcome-target"
	CompileReasonOutcomePropagation     CompileReason = "outcome-propagation"
	CompileReasonOutcomeReference       CompileReason = "outcome-reference"
	CompileReasonOutcomeRange           CompileReason = "outcome-range"
	CompileReasonOutcomeReturn          CompileReason = "outcome-return"
	CompileReasonReturnValueUnavailable CompileReason = "return-value-unavailable"
	CompileReasonReturnValueReference   CompileReason = "return-value-reference"
	CompileReasonRouteUnavailable       CompileReason = "route-unavailable"
	CompileReasonRouteGuard             CompileReason = "route-guard"
	CompileReasonOccurrenceUnavailable  CompileReason = "occurrence-unavailable"
	CompileReasonOccurrenceStorageRead  CompileReason = "occurrence-storage-read"
	CompileReasonOccurrenceCall         CompileReason = "occurrence-call"
)

func compileFailure(stage CompileStage, kind CompileRowKind, row, subrow int, reason CompileReason) CompileFailure {
	failure := CompileFailure{stage: stage, kind: kind, reason: reason, row: row, subrow: subrow}
	if !failure.Available() {
		return CompileFailure{}
	}
	return failure
}

func (failure CompileFailure) Available() bool {
	return failure.stage != "" && failure.kind != "" && failure.reason != "" && failure.row >= -1 && failure.subrow >= -1
}

func (failure CompileFailure) Stage() CompileStage     { return failure.stage }
func (failure CompileFailure) RowKind() CompileRowKind { return failure.kind }
func (failure CompileFailure) Reason() CompileReason   { return failure.reason }
func (failure CompileFailure) Row() (int, bool) {
	return failure.row, failure.Available() && failure.row >= 0
}
func (failure CompileFailure) Subrow() (int, bool) {
	return failure.subrow, failure.Available() && failure.subrow >= 0
}

// StorageRead is the minimal immutable input needed by the diagnostic phase
// for an implicit global read. The child does not import artifact's storage draft.
type StorageRead struct {
	ID   identity.ContentID
	Cell identity.ContentID
}

// Input is the cycle-safe handoff from the root compiler.  All retained
// inputs are canonical Program rows or owner-issued scalar IDs; no root
// Artifact, compiler draft, or publication index crosses this boundary.
type Input struct {
	Program   *program.Program
	ProgramID identity.ContentID

	Values        []programschema.Values
	ValuesMembers []programschema.ValuesMember

	Calls         []programschema.Call
	CallArguments []programschema.CallArgument
	Bodies        []programschema.Body
	Boundaries    []programschema.FunctionBoundary
	Formals       []programschema.FunctionFormal

	PointIDsBySite map[identity.ContentID][]identity.ContentID
	StorageReads   []StorageRead
}

type compiler struct {
	input                                    *program.Program
	programID                                identity.ContentID
	values                                   []programschema.Values
	valuesMembers                            []programschema.ValuesMember
	staticExpressions                        []programschema.StaticExpression
	staticInputs                             []programschema.StaticInput
	staticTypeNodes                          []programschema.StaticTypeNode
	staticTypeNodeUnionMembers               []programschema.StaticTypeNodeUnionMember
	staticTypeNodeIntersectionMembers        []programschema.StaticTypeNodeIntersectionMember
	staticTypeNodeGenericArguments           []programschema.StaticTypeNodeGenericArgument
	staticTypeNodeAliasParameters            []programschema.StaticTypeNodeAliasParameter
	staticTypeNodeInterfaceExtends           []programschema.StaticTypeNodeInterfaceExtend
	staticTypeNodeInterfaceMembers           []programschema.StaticTypeNodeInterfaceMember
	staticTypeNodeTypeFunctionTypeParameters []programschema.StaticTypeNodeTypeFunctionTypeParameter
	staticTypeNodeTypeFunctionParameters     []programschema.StaticTypeNodeTypeFunctionParameter
	staticTypeNodeTypeFunctionReturns        []programschema.StaticTypeNodeTypeFunctionReturn
	staticTypeNodeRecordFields               []programschema.StaticTypeNodeRecordField
	staticTypeNodeReferenceSourceKeys        []programschema.StaticTypeNodeReferenceSourceKey
	staticTypeNodeReferenceCanonicalKeys     []programschema.StaticTypeNodeReferenceCanonicalKey
	bodies                                   []programschema.Body
	bodyEntries                              []programschema.BodyEntry
	bodyRoots                                []programschema.BodyRoot
	outcomes                                 []programschema.Outcome
	outcomeReturnValues                      []programschema.OutcomeReturnValue
	outcomePoints                            []programschema.OutcomePoint
	calls                                    []programschema.Call
	callArguments                            []programschema.CallArgument
	boundaries                               []programschema.FunctionBoundary
	formals                                  []programschema.FunctionFormal
	pointIDsBySite                           map[identity.ContentID][]identity.ContentID
	storageReads                             []StorageRead

	diagnosticObservations    []programschema.DiagnosticObservation
	diagnosticEvidence        []programschema.DiagnosticEvidence
	diagnosticPaths           []programschema.DiagnosticPath
	diagnosticObservationByID map[identity.ContentID]int
}

// CompileDiagnostics owns one construction phase and returns its canonical
// publication columns. The root later composes these columns into the single
// schema Publication and seals it; no child publication or index is retained.
func CompileDiagnostics(input Input) (programschema.Publication, CompileFailure) {
	if input.Program == nil || !input.Program.Available() || !input.ProgramID.Available() {
		return programschema.Publication{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	transaction := compiler{
		input: input.Program, programID: input.ProgramID,
		values: input.Values, valuesMembers: input.ValuesMembers,
		calls: input.Calls, callArguments: input.CallArguments,
		bodies: input.Bodies, boundaries: input.Boundaries, formals: input.Formals,
		pointIDsBySite: input.PointIDsBySite, storageReads: input.StorageReads,
		diagnosticObservationByID: make(map[identity.ContentID]int),
	}
	if failure := transaction.copyDiagnosticObservationsFailure(); failure.Available() {
		return programschema.Publication{}, failure
	}
	return programschema.Publication{
		DiagnosticObservations: transaction.diagnosticObservations,
		DiagnosticEvidence:     transaction.diagnosticEvidence,
		DiagnosticPaths:        transaction.diagnosticPaths,
	}, CompileFailure{}
}

func (compiler *compiler) pointIDs(site causal.Site) []identity.ContentID {
	if compiler == nil || !site.Available() {
		return nil
	}
	ids := compiler.pointIDsBySite[site.ContextID()]
	return append([]identity.ContentID(nil), ids...)
}

func (compiler *compiler) storageReadAt(index int) (StorageRead, bool) {
	if compiler == nil || index < 0 || index >= len(compiler.storageReads) {
		return StorageRead{}, false
	}
	row := compiler.storageReads[index]
	return row, row.ID.Available() && row.Cell.Available()
}

type callConstruction struct {
	id            identity.ContentID
	bodyPath      identity.ContentID
	targetBody    identity.ContentID
	tail          identity.ContentID
	form          accessgeometry.CallForm
	typeArguments []struct{}
	arguments     []callArgumentConstruction
}

type callArgumentConstruction struct {
	member identity.ContentID
	span   identity.ContentID
}

func (compiler *compiler) callConstruction(index int) (callConstruction, bool) {
	if compiler == nil || index < 0 || index >= len(compiler.calls) {
		return callConstruction{}, false
	}
	row := compiler.calls[index]
	if !row.Available() {
		return callConstruction{}, false
	}
	bodyPath := row.BodyID()
	if !bodyPath.Available() {
		return callConstruction{}, false
	}
	form := accessgeometry.CallFormPlain
	if row.Form() == programschema.CallFormMethod {
		form = accessgeometry.CallFormMethod
	} else if row.Form() != programschema.CallFormPlain {
		return callConstruction{}, false
	}
	call := callConstruction{id: row.ID(), bodyPath: bodyPath, form: form, typeArguments: make([]struct{}, row.TypeArgumentCount())}
	if target, ok := row.DirectTargetBody(); ok {
		call.targetBody = target
	}
	if tail, ok := row.TailID(); ok {
		call.tail = tail
	}
	offset, count, ok := row.ArgumentSpan()
	if !ok || uint64(offset)+uint64(count) > uint64(len(compiler.callArguments)) {
		return callConstruction{}, false
	}
	call.arguments = make([]callArgumentConstruction, count)
	for position := uint32(0); position < count; position++ {
		argument := compiler.callArguments[offset+position]
		if !argument.Available() || argument.CallID() != row.ID() || argument.Index() != position {
			return callConstruction{}, false
		}
		call.arguments[position] = callArgumentConstruction{member: argument.MemberID(), span: argument.SpanID()}
	}
	return call, true
}

func (compiler *compiler) functionBoundaryForBody(bodyID identity.ContentID) (programschema.FunctionBoundary, bool) {
	if compiler == nil || !bodyID.Available() {
		return programschema.FunctionBoundary{}, false
	}
	for _, boundary := range compiler.boundaries {
		if boundary.Available() && boundary.BodyID() == bodyID {
			return boundary, true
		}
	}
	return programschema.FunctionBoundary{}, false
}

func (compiler *compiler) functionFormalAt(boundary programschema.FunctionBoundary, index int) (programschema.FunctionFormal, bool) {
	if compiler == nil || index < 0 {
		return programschema.FunctionFormal{}, false
	}
	offset, count, ok := boundary.FormalSpan()
	if !ok || index >= int(count) || uint64(offset)+uint64(index) >= uint64(len(compiler.formals)) {
		return programschema.FunctionFormal{}, false
	}
	formal := compiler.formals[offset+uint32(index)]
	position, positionOK := formal.Position()
	return formal, formal.Available() && positionOK && position == index
}

func (compiler *compiler) valueRowForTerm(term keyspace.Term) (programschema.Values, bool) {
	if compiler == nil || keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 {
		return programschema.Values{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	if index < 0 || index >= len(compiler.values) {
		return programschema.Values{}, false
	}
	row := compiler.values[index]
	return row, row.Available()
}

func (compiler *compiler) valueMemberAt(row programschema.Values, index int) (programschema.ValuesMember, bool) {
	if compiler == nil || !row.Available() || index < 0 {
		return programschema.ValuesMember{}, false
	}
	offset, count, spanOK := row.MemberSpan()
	if !spanOK || index >= int(count) || uint64(offset)+uint64(index) >= uint64(len(compiler.valuesMembers)) {
		return programschema.ValuesMember{}, false
	}
	member := compiler.valuesMembers[int(offset)+index]
	return member, member.Available()
}

func declaredStaticTypeID(programID identity.ContentID, view staticquery.View, cell keyspace.Term) (identity.ContentID, bool) {
	if !programID.Available() || !view.Available() || cell == 0 {
		return identity.ContentID{}, false
	}
	declarations := view.Declarations().DeclaredTypes()
	declaration, declarationOK := declarations.ForCell(cell)
	declaredCell, target, rowOK := declarations.Get(declaration)
	ref, refOK := view.StaticTypes().Ref(target)
	id, idOK := staticquery.TypeReferenceID(programID, ref)
	if !declarationOK || !rowOK || declaredCell != cell || !refOK || ref.Term() != target || !idOK {
		return identity.ContentID{}, false
	}
	return id, true
}

func (compiler *compiler) diagnosticStorageBindIdentityAt(index int) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !compiler.programID.Available() || index < 0 {
		return identity.ContentID{}, false
	}
	input, view := compiler.input, compiler.input.Flow()
	binds := view.Authored().Storage().Binds()
	term, present := binds.At(index)
	owner, values, related := binds.Get(term)
	width, widthOK := input.Source().Binds().Len(term)
	if !present || !related || !widthOK || width < 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return identity.ContentID{}, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
	entry, entryOK := view.Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	if !bodyOK || !bodyPath.Available() || !bodyID.Available() || !spanOK || !entryOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, false
	}
	return programschema.StorageBindIdentity(compiler.programID, bodyPath, width, bodyID, entry.ContextID(), finish.ContextID())
}
