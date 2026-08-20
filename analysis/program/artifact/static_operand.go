package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// artifactStaticOperandAt admits one static-graph operand from the live
// Source/Flow/Static owners. The root Program does not retain or expose this
// projection; Artifact supplies its ProgramID at the construction seam.
func artifactStaticOperandAt(input *program.Program, programID identity.ContentID, term keyspace.Term) (staticquery.StaticOperand, bool) {
	if input == nil || !input.Available() || !programID.Available() || term == 0 {
		return staticquery.StaticOperand{}, false
	}
	return input.Static().StaticOperandAt(term, staticquery.StaticOperandResolver{
		Literal: func(term keyspace.Term) (identity.ContentID, keyspace.LiteralValue, bool) {
			family, literal, literalOK := sourceLiteral(input, term)
			ordinal := keyspace.TermOrdinal(term)
			if !literalOK || ordinal == 0 {
				return identity.ContentID{}, keyspace.LiteralValue{}, false
			}
			id, _, issued, sourceOK := artifactValueSourceIdentityAt(input, programID, family, int(ordinal-1))
			return id, literal, sourceOK && issued == term
		},
		Claim: func(term keyspace.Term) (keyspace.Term, bool) {
			owner, operand, _, ok := input.Flow().Authored().Claims().Get(term)
			return operand, ok && owner != 0 && operand != 0
		},
		TypeValue: func(term keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
			return artifactStaticTypeValueOperand(input, programID, term)
		},
		RuntimeSubject: func(term keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
			return artifactStaticRuntimeSubjectOperand(input, programID, term)
		},
	})
}

func artifactStaticTypeValueOperand(input *program.Program, programID identity.ContentID, term keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	typeValues := input.Flow().Authored().TypeValues()
	owner, ownerOK := typeValues.Get(term)
	if !ownerOK || owner == 0 || !input.Flow().Executable().Contains(term) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	target, targetOK := input.Static().Operands().TypeValues().Target(term)
	ref, refOK := input.Static().StaticTypes().Ref(target)
	referenceID, referenceOK := staticquery.TypeReferenceID(programID, ref)
	if !targetOK || !refOK || ref.Term() != target || !referenceOK {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	if _, bodyOK := input.Flow().FunctionBoundaries().ForBody(owner); !bodyOK {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	bodyPath, pathOK := input.Flow().BodyPath(owner)
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	sourceID, _, source, sourceOK := artifactValueSourceIdentityAt(input, programID, keyspace.FamilyTypeValue, int(ordinal-1))
	return sourceID, referenceID, bodyPath, pathOK && sourceOK && source == term
}

func artifactStaticRuntimeSubjectOperand(input *program.Program, programID identity.ContentID, term keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	reads := input.Flow().Authored().Storage().Reads()
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	readTerm, present := reads.At(int(ordinal - 1))
	owner, source, _, related := reads.Get(readTerm)
	if !present || readTerm != term || !related || owner == 0 || source == 0 || !input.Flow().Executable().Contains(term) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	cellTerm, cellTermOK := input.Flow().Authored().Storage().Cells().At(int(keyspace.TermOrdinal(source) - 1))
	cellID, cellOK := artifactStorageCellID(programID, input.Flow(), source)
	bodyPath, bodyID, bodyOK := input.Flow().BodyContextIDs(owner)
	readPath, readPathOK := input.Flow().SemanticTermPath(term)
	_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
	entry, entryOK := input.Flow().Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := input.Flow().Causal().Sites().ForTerm(finishTerm)
	readID, readOK := programschema.StorageReadIdentity(programID, bodyPath, bodyID, readPath, entry.ContextID(), finish.ContextID())
	return readID, cellID, bodyPath, readOK && cellOK && cellTermOK && cellTerm == source && bodyOK && bodyID.Available() && readPathOK && spanOK && entryOK && finishOK && entry.Available() && finish.Available()
}

// artifactStaticFrontier is the construction-only Source/Flow join formerly
// exposed by Program. It remains a private input to static graph rows.
func artifactStaticFrontier(input *program.Program, term keyspace.Term) (identity.ContentID, uint32, bool) {
	if input == nil || !input.Available() || term == 0 {
		return identity.ContentID{}, 0, false
	}
	bodyTerm, cursor, ok := input.Source().Index().Frontier(term)
	if !ok || cursor < 0 || uint64(cursor) > uint64(^uint32(0)) {
		return identity.ContentID{}, 0, false
	}
	bodyPath, bodyOK := input.Flow().BodyPath(bodyTerm)
	return bodyPath, uint32(cursor), bodyOK && bodyPath.Available()
}
