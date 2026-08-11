// Package role owns Flow's small, closed operand-role vocabulary.
//
// These predicates are deliberately structural and allocation-free.  They
// answer only which canonical Term families can occupy a role; row-local
// relations (for example a unary operator's operand or a loop's Values
// shape) remain with the authored Flow validator.
package role

import (
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// ValueOccurrence is the closed family set that can occur as one scalar
// value.  The count vector is authoritative for the Term's dense ordinal;
// family membership alone is insufficient because a foreign or future
// ordinal must fail closed.
func ValueOccurrence(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	if !validTerm(counts, term) {
		return false
	}
	return ValueOccurrenceFamily(keyspace.TermFamily(term))
}

// ValueOccurrenceFamily is the family-only half of ValueOccurrence. It is
// intentionally narrow so artifact decoders and validators that carry a
// different local cardinality type can reject foreign families before they
// perform their owner-specific ordinal check.
func ValueOccurrenceFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyRead,
		keyspace.FamilyVararg, keyspace.FamilyUnary, keyspace.FamilyBinary,
		keyspace.FamilySelect, keyspace.FamilyFunction, keyspace.FamilyCall,
		keyspace.FamilyTable, keyspace.FamilyTypeValue, keyspace.FamilyValueClaim:
		return true
	default:
		return false
	}
}

// OpenOccurrence is the smaller value family that can retain Lua's final
// multi-result expansion.  Every other value occurrence is scalar,
// including TypeValue and ValueClaim.
func OpenOccurrence(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(counts, term) && OpenOccurrenceFamily(keyspace.TermFamily(term))
}

// OpenOccurrenceFamily is the family-only half of OpenOccurrence.
func OpenOccurrenceFamily(family keyspace.Family) bool {
	return family == keyspace.FamilyCall || family == keyspace.FamilyVararg
}

// Addressable is the closed Flow source/target family set for a cell or a
// lens.  Read and write rows share this exact family boundary; their
// owner/base/source relations remain row-owned checks.
func Addressable(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(counts, term) && AddressableFamily(keyspace.TermFamily(term))
}

// AddressableFamily is the family-only half of Addressable.
func AddressableFamily(family keyspace.Family) bool {
	return family == keyspace.FamilyCell || family == keyspace.FamilyLensExact || family == keyspace.FamilyLensKey
}

// FieldSourceFamily reports whether a Term family may be the source/key
// operand for the authored table-field mode.  FieldExact intentionally admits
// Unary as a family-level candidate; authored Build must still prove that the
// Unary row is UnaryNeg over an Integer or Float before accepting it as an
// exact key.  That row-dependent proof cannot be moved into this stateless
// predicate.
func FieldSourceFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, fieldKind kind.FieldKind) bool {
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		return hasFamily(counts, term, keyspace.FamilyKey)
	case kind.FieldExact:
		if !validTerm(counts, term) {
			return false
		}
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
			keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyUnary:
			return true
		default:
			return false
		}
	case kind.FieldKey:
		return ValueOccurrence(counts, term)
	default:
		return false
	}
}

// LoopControlFamily reports the family boundary for a loop control operand.
// Scalar While/Repeat controls are ValueOccurrences; NumericFor and
// GenericFor controls are Values rows whose width/tail semantics remain an
// authored-row proof.
func LoopControlFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, loopKind kind.LoopKind) bool {
	switch loopKind {
	case kind.LoopWhile, kind.LoopRepeat:
		return ValueOccurrence(counts, term)
	case kind.LoopNumericFor, kind.LoopGenericFor:
		return hasFamily(counts, term, keyspace.FamilyValues)
	default:
		return false
	}
}

func validTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount &&
		ordinal != 0 && ordinal <= counts[family]
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}
