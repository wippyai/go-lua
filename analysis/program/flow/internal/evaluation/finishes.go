package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
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
	return sealContextualFinishes(ports, view, counts)
}

// sealContextualFinishes records the authored continuation of an occurrence
// when that occurrence is consumed by a surrounding value expression.  The
// default Finish relation above remains the operation's own commit anchor for
// standalone terms; these rows are the one canonical producer-side
// continuation witness for nested values (for example Call -> ValueClaim and
// ValueClaim -> its enclosing Values row).  No relation is inferred from
// Source spans or from a second graph.
func sealContextualFinishes(ports *Ports, view authored.View, counts [keyspace.FamilyCount]int) error {
	set := func(from, to keyspace.Term) error {
		if !validTerm(counts, from) || !validTerm(counts, to) {
			return errors.New("program/flow/evaluation: contextual Finish term is unavailable")
		}
		family, ordinal := keyspace.TermFamily(from), keyspace.TermOrdinal(from)
		if family == keyspace.FamilyTable || family == keyspace.FamilyAssign {
			// These are structural commit anchors with exact formulas above.
			return nil
		}
		if ordinal == 0 || int(ordinal) >= len(ports.finish[family]) {
			return errors.New("program/flow/evaluation: contextual Finish slot is unavailable")
		}
		current := ports.finish[family][ordinal]
		if current != from && current != to {
			return errors.New("program/flow/evaluation: authored term has multiple Finish successors")
		}
		ports.finish[family][ordinal] = to
		return nil
	}

	// Lens and operator operands are evaluated in authored order. Exact
	// FieldName keys are static metadata and therefore intentionally have no
	// Finish plane; bracket exact keys retain their scalar occurrence here.
	exact := view.Access().Exact()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyLensExact]; ordinal++ {
		lens := keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(ordinal))
		_, base, source, fieldKind, ok := exact.Get(lens)
		if !ok {
			return errors.New("program/flow/evaluation: exact Lens row is unavailable")
		}
		if fieldKind == kind.FieldName {
			// A FieldName key owns no Finish plane and no causal vertex, so it
			// is never a continuation target. The base continues directly into
			// its lens; routing it into the static key would leave the base
			// evaluation Span unresolvable.
			if err := set(base, lens); err != nil {
				return err
			}
			continue
		}
		if err := set(base, source); err != nil {
			return err
		}
		if hasFinishFamily(counts, source) {
			if err := set(source, lens); err != nil {
				return err
			}
		}
	}
	dynamic := view.Access().Dynamic()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyLensKey]; ordinal++ {
		lens := keyspace.MakeTerm(keyspace.FamilyLensKey, uint32(ordinal))
		_, base, key, ok := dynamic.Get(lens)
		if !ok {
			return errors.New("program/flow/evaluation: dynamic Lens row is unavailable")
		}
		if err := set(base, key); err != nil {
			return err
		}
		if err := set(key, lens); err != nil {
			return err
		}
	}
	reads := view.Storage().Reads()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyRead]; ordinal++ {
		read := keyspace.MakeTerm(keyspace.FamilyRead, uint32(ordinal))
		_, subject, _, ok := reads.Get(read)
		if !ok {
			return errors.New("program/flow/evaluation: Read row is unavailable")
		}
		if hasFinishFamily(counts, subject) && (keyspace.TermFamily(subject) == keyspace.FamilyLensExact || keyspace.TermFamily(subject) == keyspace.FamilyLensKey) {
			if err := set(subject, read); err != nil {
				return err
			}
		}
	}

	unaries := view.Operators().Unaries()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyUnary]; ordinal++ {
		unary := keyspace.MakeTerm(keyspace.FamilyUnary, uint32(ordinal))
		_, _, operand, ok := unaries.Get(unary)
		if !ok {
			return errors.New("program/flow/evaluation: Unary row is unavailable")
		}
		if err := set(operand, unary); err != nil {
			return err
		}
	}
	binaries := view.Operators().Binaries()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyBinary]; ordinal++ {
		binary := keyspace.MakeTerm(keyspace.FamilyBinary, uint32(ordinal))
		_, _, left, right, ok := binaries.Get(binary)
		if !ok {
			return errors.New("program/flow/evaluation: Binary row is unavailable")
		}
		if err := set(left, right); err != nil {
			return err
		}
		if err := set(right, binary); err != nil {
			return err
		}
	}
	claims := view.Claims()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyValueClaim]; ordinal++ {
		claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, uint32(ordinal))
		_, operand, _, ok := claims.Get(claim)
		if !ok {
			return errors.New("program/flow/evaluation: ValueClaim row is unavailable")
		}
		if err := set(operand, claim); err != nil {
			return err
		}
	}
	calls := view.Calls()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyCall]; ordinal++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, uint32(ordinal))
		_, _, _, _, ok := calls.Get(call)
		if !ok {
			return errors.New("program/flow/evaluation: Call row is unavailable")
		}
	}

	// A Values row is the enclosing consumer for each fixed scalar producer.
	// Open Call/Vararg tails retain their own producer finish; their typed
	// boundary/adjustment owners consume them outside this scalar sequence.
	values := view.Values()
	for ordinal := 1; ordinal <= counts[keyspace.FamilyValues]; ordinal++ {
		row := keyspace.MakeTerm(keyspace.FamilyValues, uint32(ordinal))
		_, tail, ok := values.Get(row)
		if !ok {
			return errors.New("program/flow/evaluation: Values row is unavailable")
		}
		length, ok := values.Len(row)
		if !ok || length < 0 {
			return errors.New("program/flow/evaluation: Values extent is unavailable")
		}
		var previous keyspace.Term
		for index := 0; index < length; index++ {
			member, memberOK := values.Member(row, index)
			if !memberOK {
				return errors.New("program/flow/evaluation: Values member is unavailable")
			}
			if !contextualValueFamily(keyspace.TermFamily(member)) {
				continue
			}
			if previous != 0 {
				if err := set(previous, member); err != nil {
					return err
				}
			}
			previous = member
		}
		if tail != 0 && keyspace.TermFamily(tail) != keyspace.FamilyCall && keyspace.TermFamily(tail) != keyspace.FamilyVararg {
			if previous != 0 {
				if err := set(previous, tail); err != nil {
					return err
				}
			}
			previous = tail
		}
		if previous != 0 {
			// A final Select must retain its own short-circuit finish.  Its
			// causal evaluator emits the Select -> Values continuation, while
			// the guarded arms target that Select anchor.  Replacing this finish
			// with Values would collapse the route into a self-edge, which the
			// causal layer correctly discards.  A non-final Select was already
			// contextualized to the next authored member above.
			if keyspace.TermFamily(previous) != keyspace.FamilySelect {
				if err := set(previous, row); err != nil {
					return err
				}
			}
		}
	}

	// Parent rows retain their own commit formulas.  Causal assembly consumes
	// this scalar sequence and the authored parent relation separately, so no
	// duplicate Values -> parent edge is introduced here.
	return nil
}

func contextualValueFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyRead, keyspace.FamilyVararg, keyspace.FamilyUnary,
		keyspace.FamilyBinary, keyspace.FamilySelect, keyspace.FamilyFunction,
		keyspace.FamilyCall, keyspace.FamilyTable, keyspace.FamilyTypeValue,
		keyspace.FamilyValueClaim:
		return true
	default:
		return false
	}
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
