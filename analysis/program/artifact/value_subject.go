package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/internal/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// valueSubjectID is the construction-only subject join used by occurrence
// admission. Literal/TypeValue subjects reuse the exact source span geometry;
// all other subjects use their existing direct EvaluationSpan. No subject row
// or Program identity API is retained by Artifact.
func (compiler *compiler) valueSubjectID(term keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || term == 0 {
		return identity.ContentID{}, false
	}
	switch family := keyspace.TermFamily(term); family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyTypeValue:
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 {
			return identity.ContentID{}, false
		}
		programID := compiler.key.ProgramID()
		if !programID.Available() {
			programID = compiler.input.ContentID()
		}
		_, spanID, issued, ok := artifactcompiler.ValueSourceIdentityAt(compiler.input, programID, family, int(ordinal-1))
		return spanID, ok && issued == term && spanID.Available()
	default:
		spanID, _, _, ok := compiler.input.EvaluationSpan(term)
		return spanID, ok && spanID.Available()
	}
}
