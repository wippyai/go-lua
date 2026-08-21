package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/valuesource"
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
	compiler.subjectLifetimes = make([]lifecycle.SubjectLiveness, 0, projection.LivenessCount())
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
		kind, kindOK := artifactSubjectLivenessKind(flowRow.Subject.Kind)
		subjectID, subjectOK := compiler.subjectLivenessID(programID, flowRow.Subject)
		state, stateOK := artifactSubjectLivenessState(flowRow.State)
		if !flowSubjectOK || flowSubjectID != flowRow.Subject.ID || !kindOK || !subjectOK || !stateOK || !subjectID.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		id, idOK := lifecycle.SubjectLivenessIdentity(flowRow.YieldRoute, kind, subjectID)
		row, emitted := lifecycle.NewSubjectLiveness(id, flowRow.YieldRoute, flowRow.YieldFromPath, flowRow.YieldToPath, subjectID, kind, state)
		if !idOK || !emitted {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		compiler.subjectLifetimes = append(compiler.subjectLifetimes, row)
	}
	return CompileFailure{}
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
		return lifecycle.SubjectLivenessUnknown, false
	}
}

func (compiler *compiler) subjectLivenessID(programID identity.ContentID, subject subjectflow.Subject) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !programID.Available() || !subject.ID.Available() || subject.Term == 0 {
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
	if term == 0 {
		return identity.ContentID{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	switch family {
	case keyspace.FamilyFunction, keyspace.FamilyTable:
		if id, ok := compiler.input.Flow().AllocationID(term); ok {
			return id, true
		}
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		if ordinal != 0 {
			sourceID, _, issued, ok := valuesource.IdentityAt(compiler.input, family, int(ordinal-1))
			if ok && issued == term && sourceID.Available() {
				return sourceID, true
			}
		}
	case keyspace.FamilyTypeValue:
		// TypeValue is a static-only subject. Preserve its owner-issued
		// semantic path so the Program row remains auditable; no Placement
		// allocation consumer will project this non-allocation identity.
		return compiler.input.Flow().SemanticTermPath(term)
	}
	// A runtime value that has no dedicated artifact semantic occurrence is
	// still projected by its exact EvaluationSpan. If even that geometry is
	// absent, preserve the Flow semantic path as an opaque Program subject;
	// mounted consumers will conservatively return Unknown rather than
	// inventing a Value coordinate.
	if span, _, _, ok := compiler.input.EvaluationSpan(term); ok && span.Available() {
		return span, true
	}
	return compiler.input.Flow().SemanticTermPath(term)
}
