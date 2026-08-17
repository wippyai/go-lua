package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ValueSubjectID returns the canonical scalar subject identity consumed by a
// computation for one authored value term. Positionless literal and TypeValue
// terms use their Source-owned evaluation anchor; every directly evaluated
// term uses its existing Program span. The query never manufactures geometry
// in a consumer.
//
// A Read is one of the directly evaluated terms. The Read family spans both
// cell reads and index reads, and the storage cell plane owns only the former,
// so admitting a subject through the storage-read row would withhold every
// index read from its consuming computation.
func (program *Program) ValueSubjectID(term keyspace.Term) (identity.ContentID, bool) {
	if !program.scalarIdentityAvailable() || term == 0 {
		return identity.ContentID{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return identity.ContentID{}, false
	}
	index := int(ordinal - 1)
	switch family := keyspace.TermFamily(term); family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyTypeValue:
		_, spanID, issued, ok := program.ValueSourceIDAt(family, index)
		return spanID, ok && issued == term && spanID.Available()
	default:
		spanID, _, _, ok := program.EvaluationSpan(term)
		return spanID, ok && spanID.Available()
	}
}
