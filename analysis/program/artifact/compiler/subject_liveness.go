package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstorage "github.com/wippyai/go-lua/analysis/program/storage"
	"github.com/wippyai/go-lua/analysis/program/valuesource"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// copySubjectLivenessFailure projects Flow's neutral all-path suspension rows
// into Program identities. The compiler is the only seam allowed to join a
// Flow subject term to a mounted Value/Heap/Cell identity; Placement receives
// only the resulting Program family and never reconstructs this join.
func (compiler *compiler) copySubjectLivenessFailure() CompileFailure {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	projection := compiler.input.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	// Flow subjects are authored coordinates. More than one authored subject can
	// resolve to the same Program value (notably an open Values aggregate and
	// one of its finite members). Program owns one liveness judgment per
	// (yield route, kind, Program subject) coordinate, so collect every Flow
	// verdict before publishing the sealed Program row.
	projected := make(map[identity.ContentID]*subjectLivenessProjection, projection.LivenessCount())
	for index := 0; index < projection.LivenessCount(); index++ {
		flowRow, rowOK := projection.LivenessAt(index)
		if !rowOK || !flowRow.ID.Available() || !flowRow.YieldRoute.Available() || !flowRow.Subject.ID.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		// SubjectFlow always names the authored subject with Flow's semantic
		// path.  Authenticate that source coordinate before translating it into
		// the Program subject plane; the translated identity is intentionally
		// allowed to differ (Cells use StorageCellIdentity, Values use their
		// aggregate occurrence, and allocation-backed values use their own
		// artifact identity).
		flowSubjectID, flowSubjectOK := compiler.input.Flow().SemanticTermPath(flowRow.Subject.Term)
		subjects, subjectsOK := compiler.subjectLivenessCoordinates(programID, flowRow.Subject)
		_, stateOK := artifactSubjectLivenessState(flowRow.State)
		if !flowSubjectOK || flowSubjectID != flowRow.Subject.ID || !subjectsOK || !stateOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		for subjectIndex, subject := range subjects {
			id, idOK := lifecycle.SubjectLivenessIdentity(flowRow.YieldRoute, subject.kind, subject.id)
			if !idOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, subjectIndex, CompileReasonBodyUnavailable)
			}
			current, exists := projected[id]
			if exists {
				if current.yieldRoute != flowRow.YieldRoute ||
					current.yieldFromPath != flowRow.YieldFromPath ||
					current.yieldToPath != flowRow.YieldToPath ||
					current.subject != subject {
					return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, subjectIndex, CompileReasonBodyUnavailable)
				}
				current.states = append(current.states, flowRow.State)
				continue
			}
			projected[id] = &subjectLivenessProjection{
				yieldRoute:    flowRow.YieldRoute,
				yieldFromPath: flowRow.YieldFromPath,
				yieldToPath:   flowRow.YieldToPath,
				subject:       subject,
				states:        []subjectflow.LivenessState{flowRow.State},
			}
		}
	}
	ids := make([]identity.ContentID, 0, len(projected))
	for id := range projected {
		ids = append(ids, id)
	}
	identity.SortContentIDs(ids)
	compiler.publication.Lifecycle.SubjectLifetimes = make([]lifecycle.SubjectLiveness, 0, len(ids))
	for index, id := range ids {
		projection := projected[id]
		state, stateOK := artifactSubjectLivenessState(subjectflow.AggregateLiveness(projection.states))
		row, emitted := lifecycle.NewSubjectLiveness(
			id,
			projection.yieldRoute,
			projection.yieldFromPath,
			projection.yieldToPath,
			projection.subject.id,
			projection.subject.kind,
			state,
		)
		if !stateOK || !emitted {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		compiler.publication.Lifecycle.SubjectLifetimes = append(compiler.publication.Lifecycle.SubjectLifetimes, row)
	}
	return CompileFailure{}
}

type subjectLivenessCoordinate struct {
	kind lifecycle.SubjectLivenessKind
	id   identity.ContentID
}

type subjectLivenessProjection struct {
	yieldRoute    identity.ContentID
	yieldFromPath identity.ContentID
	yieldToPath   identity.ContentID
	subject       subjectLivenessCoordinate
	states        []subjectflow.LivenessState
}

// subjectLivenessCoordinates performs the sole Flow-subject to Program-value
// ownership join. Most subjects project one coordinate. A Call subject is an
// abstract producer until Program has sealed its consumer-side CallResult
// geometry, so each finite result slot projects its own ValueID. Discarded and
// open outputs have no finite scalar Value coordinate and publish no scalar
// liveness row; they are not represented by the Call occurrence identity.
func (compiler *compiler) subjectLivenessCoordinates(programID identity.ContentID, subject subjectflow.Subject) ([]subjectLivenessCoordinate, bool) {
	if subject.Kind == subjectflow.SubjectValues {
		if compiler == nil || compiler.input == nil || !programID.Available() || compiler.key.ProgramID() != programID ||
			compiler.input.ContentID() != programID || keyspace.TermFamily(subject.Term) != keyspace.FamilyValues || !subject.ID.Available() {
			return nil, false
		}
		aggregateID, aggregateIDOK := compiler.input.Flow().ValuesOccurrenceID(subject.Term)
		if !aggregateIDOK || !aggregateID.Available() {
			return nil, false
		}
		var aggregateIndex int
		var aggregateFound bool
		for index, aggregate := range compiler.publication.Values {
			if aggregate.ID() != aggregateID {
				continue
			}
			if aggregateFound {
				return nil, false
			}
			aggregateIndex, aggregateFound = index, true
		}
		if !aggregateFound {
			return nil, false
		}
		aggregate := compiler.publication.Values[aggregateIndex]
		if _, open := aggregate.Tail(); !open {
			return []subjectLivenessCoordinate{{kind: lifecycle.SubjectLivenessValues, id: aggregate.ID()}}, true
		}
		// An open tail has no finite scalar denominator. Preserve the known
		// aggregate/member coordinates, but do not turn its Call/Vararg producer
		// into a fabricated Value. Pack owns that future runtime-width proof.
		offset, count, spanOK := aggregate.MemberSpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.publication.ValuesMembers)) {
			return nil, false
		}
		coordinates := make([]subjectLivenessCoordinate, 0, count+1)
		coordinates = append(coordinates, subjectLivenessCoordinate{kind: lifecycle.SubjectLivenessValue, id: aggregate.ID()})
		for index := uint32(0); index < count; index++ {
			member := compiler.publication.ValuesMembers[offset+index]
			if !member.Available() {
				return nil, false
			}
			coordinates = append(coordinates, subjectLivenessCoordinate{kind: lifecycle.SubjectLivenessValue, id: member.ID()})
		}
		return coordinates, true
	}
	if keyspace.TermFamily(subject.Term) != keyspace.FamilyCall {
		kind, kindOK := artifactSubjectLivenessKind(subject.Kind)
		id, idOK := compiler.subjectLivenessID(programID, subject)
		if !kindOK || !idOK || !id.Available() {
			return nil, false
		}
		return []subjectLivenessCoordinate{{kind: kind, id: id}}, true
	}
	if compiler == nil || compiler.input == nil || subject.Kind != subjectflow.SubjectValue ||
		!programID.Available() || compiler.key.ProgramID() != programID || compiler.input.ContentID() != programID ||
		keyspace.TermOrdinal(subject.Term) == 0 {
		return nil, false
	}
	ordinal := keyspace.TermOrdinal(subject.Term)
	identities, callOK := compiler.input.CallIdentityAt(int(ordinal - 1))
	if !callOK || !identities.Call.Available() {
		return nil, false
	}
	var resultIndex int
	var resultFound bool
	for index, result := range compiler.publication.CallResults {
		if result.CallID() != identities.Call {
			continue
		}
		if resultFound {
			return nil, false
		}
		resultIndex, resultFound = index, true
	}
	if !resultFound {
		// A statement Call or a consumer admitting zero results has no Value.
		return nil, true
	}
	result := compiler.publication.CallResults[resultIndex]
	open, openOK := result.ResultsOpen()
	offset, count, spanOK := result.SlotSpan()
	if !openOK || !spanOK || open {
		// Open results belong to the Pack/Values producer plane; no finite
		// scalar coordinate may be fabricated for them here.
		return nil, openOK && spanOK && open
	}
	if uint64(offset)+uint64(count) > uint64(len(compiler.publication.CallResultSlots)) {
		return nil, false
	}
	coordinates := make([]subjectLivenessCoordinate, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := uint32(0); index < count; index++ {
		slot := compiler.publication.CallResultSlots[offset+index]
		if !slot.Available() || slot.CallID() != identities.Call {
			return nil, false
		}
		valueID, valueOK := slot.ValueID()
		if !valueOK {
			// Structural consumers do not issue a materialized Value coordinate.
			continue
		}
		if _, duplicate := seen[valueID]; duplicate {
			return nil, false
		}
		seen[valueID] = struct{}{}
		coordinates = append(coordinates, subjectLivenessCoordinate{kind: lifecycle.SubjectLivenessValue, id: valueID})
	}
	return coordinates, true
}

func artifactSubjectLivenessKind(kind subjectflow.SubjectKind) (lifecycle.SubjectLivenessKind, bool) {
	switch kind {
	case subjectflow.SubjectRoot:
		return lifecycle.SubjectLivenessRoot, true
	case subjectflow.SubjectCell:
		return lifecycle.SubjectLivenessCell, true
	case subjectflow.SubjectValue:
		return lifecycle.SubjectLivenessValue, true
	case subjectflow.SubjectValues:
		return lifecycle.SubjectLivenessValues, true
	default:
		return lifecycle.SubjectLivenessInvalid, false
	}
}

func artifactSubjectLivenessState(state subjectflow.LivenessState) (lifecycle.SubjectLivenessState, bool) {
	switch state {
	case subjectflow.LivenessUnknown:
		return lifecycle.SubjectLivenessUnknown, true
	case subjectflow.LivenessLive:
		return lifecycle.SubjectLivenessLive, true
	case subjectflow.LivenessDiesBefore:
		return lifecycle.SubjectLivenessDiesBefore, true
	default:
		// Unknown is an authenticated Flow verdict. An out-of-vocabulary state
		// has no Program projection and must remain invalid even if a caller
		// accidentally drops the boolean.
		return lifecycle.SubjectLivenessState(0), false
	}
}

func (compiler *compiler) subjectLivenessID(programID identity.ContentID, subject subjectflow.Subject) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !programID.Available() ||
		compiler.key.ProgramID() != programID || compiler.input.ContentID() != programID || !subject.ID.Available() || subject.Term == 0 {
		return identity.ContentID{}, false
	}
	flowView := compiler.input.Flow()
	if !flowSubjectKindTerm(subject.Kind, subject.Term) {
		return identity.ContentID{}, false
	}
	switch subject.Kind {
	case subjectflow.SubjectRoot:
		return flowView.BodyPath(subject.Term)
	case subjectflow.SubjectCell:
		// StorageCellIdentity is the canonical Program/Cell bridge. Verify
		// the authored Cell relation before admitting a term from Flow.
		if keyspace.TermFamily(subject.Term) != keyspace.FamilyCell {
			return identity.ContentID{}, false
		}
		if _, _, _, ok := flowView.Authored().Storage().Cells().Get(subject.Term); !ok {
			return identity.ContentID{}, false
		}
		return lifecycle.StorageCellIdentity(programID, subject.Term)
	case subjectflow.SubjectValues:
		if keyspace.TermFamily(subject.Term) != keyspace.FamilyValues {
			return identity.ContentID{}, false
		}
		return flowView.ValuesOccurrenceID(subject.Term)
	case subjectflow.SubjectValue:
		return compiler.valueLivenessID(programID, subject.Term)
	default:
		return identity.ContentID{}, false
	}
}

// flowSubjectKindTerm is the compiler-side preimage guard. Flow publishes a
// distinct subject plane for authored Cells and Values; accepting a Cell as a
// generic Value would make a mounted consumer guess which coordinate family
// owns the row. Runtime value families remain opaque paths unless their
// dedicated allocation/source identity is available.
func flowSubjectKindTerm(kind subjectflow.SubjectKind, term keyspace.Term) bool {
	if term == 0 {
		return false
	}
	switch kind {
	case subjectflow.SubjectRoot:
		return keyspace.TermFamily(term) == keyspace.FamilyBody
	case subjectflow.SubjectCell:
		return keyspace.TermFamily(term) == keyspace.FamilyCell
	case subjectflow.SubjectValues:
		return keyspace.TermFamily(term) == keyspace.FamilyValues
	case subjectflow.SubjectValue:
		family := keyspace.TermFamily(term)
		return family != keyspace.FamilyInvalid && family != keyspace.FamilyCell && family != keyspace.FamilyValues && family != keyspace.FamilyBody
	default:
		return false
	}
}

func (compiler *compiler) valueLivenessID(programID identity.ContentID, term keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !programID.Available() ||
		compiler.key.ProgramID() != programID || compiler.input.ContentID() != programID || term == 0 {
		return identity.ContentID{}, false
	}
	input, flowView := compiler.input, compiler.input.Flow()
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	switch family {
	case keyspace.FamilyFunction, keyspace.FamilyTable:
		if id, ok := flowView.AllocationID(term); ok {
			return id, true
		}
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		if ordinal != 0 {
			sourceID, _, issued, ok := valuesource.IdentityAt(input, family, int(ordinal-1))
			if ok && issued == term && sourceID.Available() {
				return sourceID, true
			}
		}
	case keyspace.FamilyRead:
		// A storage read is owned by StorageReadIdentity. An index read is an
		// evaluated result and reuses its owner-issued Span identity; it must not
		// acquire a second compiler-private result identity.
		if ordinal != 0 {
			readID, issued, ok := programstorage.ReadIdentityAt(input, int(ordinal-1))
			if ok && issued == term && readID.Available() {
				return readID, true
			}
			if id, ok := compiler.indexReadResultLivenessID(term); ok {
				return id, true
			}
		}
	case keyspace.FamilyCall:
		// A Call occurrence is not a Value coordinate. Its result identities
		// exist only after copyCallRowsFailure seals consumer-side geometry and
		// are expanded by subjectLivenessCoordinates.
		return identity.ContentID{}, false
	case keyspace.FamilyVararg:
		// A storage Vararg is mounted only through the Values-tail equation.
		// The reverse lookup is required because Flow exposes ValuesTailID on
		// the owning Values row, while SubjectFlow carries the tail producer.
		if id, ok := compiler.valuesTailLivenessID(term); ok {
			return id, true
		}
	case keyspace.FamilyTypeValue:
		// TypeValue is a value-source occurrence, not a generic Flow path. The
		// source owner authenticates the executable TypeValue target and emits
		// the identity that Link mounts for its Value coordinate.
		if ordinal != 0 {
			sourceID, _, issued, ok := valuesource.IdentityAt(input, family, int(ordinal-1))
			if ok && issued == term && sourceID.Available() {
				return sourceID, true
			}
		}
	case keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilySelect, keyspace.FamilyValueClaim:
		// Link publishes these three computation rows by their exact existing
		// Span context. Keep this as an explicit owner whitelist; no other
		// family is allowed to borrow Span geometry as a substitute identity.
		if id, ok := compiler.computationLivenessID(term); ok {
			return id, true
		}
	}
	// Missing owner issuance is a construction failure. Returning a neutral
	// Flow path here would create a semantic row that Link cannot mount and
	// would silently compensate for a missing value owner.
	return identity.ContentID{}, false
}

func (compiler *compiler) indexReadResultLivenessID(term keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() ||
		keyspace.TermFamily(term) != keyspace.FamilyRead || keyspace.TermOrdinal(term) == 0 {
		return identity.ContentID{}, false
	}
	candidates := compiler.input.Flow().Candidates()
	if candidates == nil || !candidates.IndexGet().Contains(term) {
		return identity.ContentID{}, false
	}
	span, spanOK := compiler.input.Span(term)
	if !spanOK || !compiler.input.OwnsSpan(span) {
		return identity.ContentID{}, false
	}
	resultID := span.ContextID()
	return resultID, resultID.Available()
}

// valuesTailLivenessID resolves the authored Values rows whose open tail is
// term. Multiple rows may reuse a tail producer, but every owner must issue
// the same ValuesTailID; disagreement is ambiguous and fails closed. No new
// reverse index or term-path identity is retained by Artifact.
func (compiler *compiler) valuesTailLivenessID(term keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() ||
		keyspace.TermFamily(term) != keyspace.FamilyVararg || keyspace.TermOrdinal(term) == 0 {
		return identity.ContentID{}, false
	}
	flowView := compiler.input.Flow()
	if _, _, varargOK := flowView.Authored().Storage().Varargs().Get(term); !varargOK {
		return identity.ContentID{}, false
	}
	values := flowView.Authored().Values()
	var result identity.ContentID
	matches := 0
	for index := 0; index < values.Count(); index++ {
		valueTerm, termOK := values.At(index)
		_, tail, rowOK := values.Get(valueTerm)
		if !termOK || !rowOK {
			return identity.ContentID{}, false
		}
		if tail != term {
			continue
		}
		id, idOK := flowView.ValuesTailID(valueTerm)
		if !idOK || !id.Available() {
			return identity.ContentID{}, false
		}
		matches++
		if matches == 1 {
			result = id
			continue
		}
		if id != result {
			return identity.ContentID{}, false
		}
	}
	return result, matches > 0 && result.Available()
}

// computationLivenessID is deliberately a closed whitelist. Link's sealed
// semantic directory publishes Unary, the Binary primitive classes lowered
// by copyComputations, Select, and ValueClaim by existing Span context;
// unsupported computation families have no mounted identity.
func (compiler *compiler) computationLivenessID(term keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || term == 0 ||
		!compiler.input.Flow().Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	operators := compiler.input.Flow().Authored().Operators()
	claims := compiler.input.Flow().Authored().Claims()
	var related bool
	var owner programschema.OccurrenceKind
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyUnary:
		_, _, _, related = operators.Unaries().Get(term)
	case keyspace.FamilyBinary:
		if compiler.input.Flow().Candidates().Concat().Contains(term) {
			related = true
			owner = programschema.OccurrenceBinaryConcat
			break
		}
		primitive, primitiveOK := compiler.input.Flow().BinaryPrimitives().Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		op := operation.Op
		related = primitiveOK && sourceOK && source == term && operationOK &&
			(flowkind.IsBinaryArithmetic(op) || op == flowkind.BinaryEqual || op == flowkind.BinaryNotEqual || flowkind.IsBinaryOrder(op))
	case keyspace.FamilySelect:
		_, _, _, _, related = operators.Selects().Get(term)
	case keyspace.FamilyValueClaim:
		_, _, _, related = claims.Get(term)
	default:
		return identity.ContentID{}, false
	}
	if !related {
		return identity.ContentID{}, false
	}
	span, spanOK := compiler.input.Span(term)
	if !spanOK || !compiler.input.OwnsSpan(span) {
		return identity.ContentID{}, false
	}
	id := span.ContextID()
	if !id.Available() {
		return identity.ContentID{}, false
	}
	// Unary, primitive Binary, Select and ValueClaim are already authenticated
	// by their typed Flow owner above. Concat has no Flow operator/primitive
	// row: its Program occurrence is the owner that makes the candidate's Span
	// result mountable. Refuse an absent or duplicate owner instead of treating
	// candidate membership as a judgment.
	if !owner.Valid() {
		return id, true
	}
	matches := 0
	for _, row := range compiler.publication.Occurrences {
		if row.Kind() == owner && row.ID() == id {
			matches++
		}
	}
	return id, matches == 1
}
