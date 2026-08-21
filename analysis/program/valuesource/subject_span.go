package valuesource

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// SubjectSpan returns the exact span identity used when a value term is
// admitted as an occurrence subject. Literal and TypeValue terms are joined
// through their canonical ValueSource row; every other term uses the
// Program-owned evaluation span directly.
func SubjectSpan(input *program.Program, term keyspace.Term) (identity.ContentID, bool) {
	if input == nil || !input.Available() || term == 0 {
		return identity.ContentID{}, false
	}
	switch family := keyspace.TermFamily(term); family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyTypeValue:
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 {
			return identity.ContentID{}, false
		}
		_, spanID, issued, ok := IdentityAt(input, family, int(ordinal-1))
		return spanID, ok && issued == term && spanID.Available()
	default:
		spanID, _, _, ok := input.EvaluationSpan(term)
		return spanID, ok && spanID.Available()
	}
}
