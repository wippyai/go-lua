package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

var finishFamilies = [...]keyspace.Family{
	keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat,
	keyspace.FamilyString, keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
	keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyLabel, keyspace.FamilyGoto,
	keyspace.FamilyBody, keyspace.FamilyCell, keyspace.FamilyRead, keyspace.FamilyVararg,
	keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilySelect, keyspace.FamilyBind,
	keyspace.FamilyAssign, keyspace.FamilyFunction, keyspace.FamilyCall, keyspace.FamilyBranch,
	keyspace.FamilyLoop, keyspace.FamilyTable, keyspace.FamilyTypeValue, keyspace.FamilyValueClaim,
	keyspace.FamilyWrite, keyspace.FamilyTableField,
}

func sealFinishes(ports *Ports, view authored.View, counts [keyspace.FamilyCount]int) error {
	for _, family := range finishFamilies {
		for ordinal := 1; ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			if !finishTerm(ports, view, counts, term) {
				return errors.New("program/flow/evaluation: invalid Finish relation")
			}
		}
	}
	return nil
}

func hasFinishFamily(counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	if !validTerm(counts, term) {
		return false
	}
	// Key/List metadata never has a Finish plane; all other authored value
	// occurrences do.  This explicit exclusion keeps static field names out of
	// contextual successor construction.
	return keyspace.TermFamily(term) != keyspace.FamilyKey
}

func finishTerm(ports *Ports, view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	if !validTerm(counts, term) {
		return false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	switch family {
	case keyspace.FamilyTable:
		count, ok := view.Tables().FieldCount(term)
		if !ok || count < 0 {
			return false
		}
		if count == 0 {
			ports.finish[family][ordinal] = term
			return true
		}
		field, ok := view.Tables().FieldAt(term, count-1)
		if !ok || !hasFamily(counts, field, keyspace.FamilyTableField) {
			return false
		}
		ports.finish[family][ordinal] = field
		return true
	case keyspace.FamilyAssign:
		writeCount, ok := view.Storage().Assigns().WriteCount(term)
		if !ok || writeCount <= 0 {
			return false
		}
		// Authored Write rows are a dense commit chain. The reverse commit
		// walk ends at the lowest ordinal, which is the assignment Finish port.
		write, ok := view.Storage().Assigns().WriteAt(term, 0)
		if !ok || !hasFamily(counts, write, keyspace.FamilyWrite) {
			return false
		}
		for index := 0; index < writeCount; index++ {
			candidate, candidateOK := view.Storage().Assigns().WriteAt(term, index)
			if !candidateOK || !hasFamily(counts, candidate, keyspace.FamilyWrite) {
				return false
			}
			parent, _, parentOK := view.Storage().Writes().Get(candidate)
			if !parentOK || parent != term {
				return false
			}
		}
		ports.finish[family][ordinal] = write
		return true
	case keyspace.FamilyCall:
		// Call is its own evaluation finish, even when used as a Values tail.
		ports.finish[family][ordinal] = term
		return true
	case keyspace.FamilySelect:
		// Select is the short-circuit control finish, never its right operand.
		ports.finish[family][ordinal] = term
		return true
	default:
		ports.finish[family][ordinal] = term
		return true
	}
}
