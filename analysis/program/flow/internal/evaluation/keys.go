package evaluation

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Field names are metadata. FieldExact has a deliberately closed grammar:
// scalar literals or UnaryNeg over an integer/float literal. An executable
// UnaryNeg occurrence is a runtime value and must enter the same left-to-right
// evaluation walk as FieldKey.
func (walker *Session) staticLensSource(term keyspace.Term, fieldKind kind.FieldKind) bool {
	if fieldKind == kind.FieldName {
		return keyspace.TermFamily(term) == keyspace.FamilyKey && keyspace.TermOrdinal(term) != 0
	}
	if fieldKind != kind.FieldExact {
		return false
	}
	return walker.exactFieldSource(term)
}

func (walker *Session) fieldKey(term keyspace.Term, fieldKind kind.FieldKind) bool {
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		return keyspace.TermFamily(term) == keyspace.FamilyKey && keyspace.TermOrdinal(term) != 0
	case kind.FieldExact:
		return walker.exactFieldSource(term)
	case kind.FieldKey:
		return walker.valueTerm(term)
	default:
		return false
	}
}

// runtimeFieldOperand admits only the UnaryNeg branch of the exact-key
// grammar. Ordinary event sessions walk it directly. Every SealPending pass,
// including its parent discovery, uses the sealed Executable proof: canonical
// Containment already validated the complete dead/static grammar and parent
// forest before that proof could exist. A local static-candidate predicate
// must never turn an executable UnaryNeg into metadata, and scalar literals
// are always metadata.
func (walker *Session) runtimeFieldOperand(term keyspace.Term, fieldKind kind.FieldKind) bool {
	if walker == nil || fieldKind != kind.FieldExact || !walker.exactFieldSource(term) || keyspace.TermFamily(term) != keyspace.FamilyUnary {
		return false
	}
	// Ordinary event sessions have no executable proof; a syntactically valid
	// UnaryNeg exact key is nevertheless an evaluated operand and must be
	// walked. Pending never reconstructs a non-executable edge as a second
	// structural authority; scalar literals remain metadata in every mode.
	if walker.pending == nil {
		return true
	}
	return walker.pending.executable != nil && walker.pending.executable.Executable(term)
}

func (walker *Session) exactFieldSource(term keyspace.Term) bool {
	if walker == nil || !walker.validTerm(term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		return true
	case keyspace.FamilyUnary:
		_, op, operand, ok := walker.view.Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg || !walker.validTerm(operand) {
			return false
		}
		family := keyspace.TermFamily(operand)
		return family == keyspace.FamilyInteger || family == keyspace.FamilyFloat
	default:
		return false
	}
}

func (walker *Session) valueTerm(term keyspace.Term) bool {
	return walker != nil && walker.validTerm(term) && flowrole.ValueOccurrenceFamily(keyspace.TermFamily(term))
}
