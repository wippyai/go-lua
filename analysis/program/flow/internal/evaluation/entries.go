package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

var entryFamilies = [...]keyspace.Family{
	keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat,
	keyspace.FamilyString, keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
	keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyBody,
	keyspace.FamilyRead, keyspace.FamilyVararg, keyspace.FamilyUnary, keyspace.FamilyBinary,
	keyspace.FamilySelect, keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyFunction,
	keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop, keyspace.FamilyTable,
	keyspace.FamilyTypeValue, keyspace.FamilyValueClaim, keyspace.FamilyTableField,
}

func sealEntries(ports *Ports, view authored.View, counts [keyspace.FamilyCount]int) error {
	state := [keyspace.FamilyCount][]uint8{}
	for _, family := range entryFamilies {
		if counts[family] != 0 {
			state[family] = make([]uint8, counts[family]+1)
		}
	}
	for _, family := range entryFamilies {
		for ordinal := 1; ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			if _, err := resolveEntry(ports, view, counts, &state, term); err != nil {
				return err
			}
		}
	}
	return nil
}

type entryFrame struct {
	term keyspace.Term
	// A Values row may contain more than 255 fixed members. Keep the child
	// cursor wide enough for the authored range; uint8 would wrap and revisit
	// the first members forever during iterative sealing.
	child int
}

func resolveEntry(ports *Ports, view authored.View, counts [keyspace.FamilyCount]int, state *[keyspace.FamilyCount][]uint8, root keyspace.Term) (keyspace.Term, error) {
	if !entryTerm(root, counts) {
		return 0, errors.New("program/flow/evaluation: term has no Entry port")
	}
	family, ordinal := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
	if ports.entry[family][ordinal] != 0 {
		return ports.entry[family][ordinal], nil
	}
	stack := []entryFrame{{term: root}}
	for len(stack) != 0 {
		at := len(stack) - 1
		frame := &stack[at]
		family, ordinal := keyspace.TermFamily(frame.term), keyspace.TermOrdinal(frame.term)
		if ports.entry[family][ordinal] != 0 {
			stack = stack[:at]
			continue
		}
		slot := &(*state)[family][ordinal]
		if *slot == 0 {
			*slot = 1
		}
		child, hasChild, err := entryChild(view, counts, frame.term, int(frame.child))
		if err != nil {
			return 0, err
		}
		if hasChild {
			frame.child++
			childFamily, childOrdinal := keyspace.TermFamily(child), keyspace.TermOrdinal(child)
			if !entryTerm(child, counts) || childOrdinal == 0 || uint64(childOrdinal) >= uint64(len((*state)[childFamily])) {
				return 0, errors.New("program/flow/evaluation: Entry child is not evaluable")
			}
			if (*state)[childFamily][childOrdinal] == 1 {
				return 0, errors.New("program/flow/evaluation: cyclic Entry relation")
			}
			if ports.entry[childFamily][childOrdinal] == 0 {
				stack = append(stack, entryFrame{term: child})
			}
			continue
		}
		entry, err := completeEntry(ports, view, counts, frame.term)
		if err != nil {
			return 0, err
		}
		ports.entry[family][ordinal] = entry
		*slot = 2
		stack = stack[:at]
	}
	entry := ports.entry[keyspace.TermFamily(root)][keyspace.TermOrdinal(root)]
	if entry == 0 {
		return 0, errors.New("program/flow/evaluation: unresolved Entry")
	}
	return entry, nil
}

func entryTerm(term keyspace.Term, counts [keyspace.FamilyCount]int) bool {
	if !validTerm(counts, term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat,
		keyspace.FamilyString, keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyBody,
		keyspace.FamilyRead, keyspace.FamilyVararg, keyspace.FamilyUnary, keyspace.FamilyBinary,
		keyspace.FamilySelect, keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyFunction,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop, keyspace.FamilyTable,
		keyspace.FamilyTypeValue, keyspace.FamilyValueClaim, keyspace.FamilyTableField:
		return true
	default:
		return false
	}
}

func entryChild(view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term, index int) (keyspace.Term, bool, error) {
	if index < 0 || !entryTerm(term, counts) {
		return 0, false, errors.New("program/flow/evaluation: invalid Entry child query")
	}
	one := func(child keyspace.Term) (keyspace.Term, bool, error) {
		if index == 0 {
			return child, true, nil
		}
		return 0, false, nil
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyRead:
		_, sourceTerm, _, ok := view.Storage().Reads().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Read for Entry")
		}
		if hasFamily(counts, sourceTerm, keyspace.FamilyCell) {
			return 0, false, nil
		}
		return one(sourceTerm)
	case keyspace.FamilyLensExact:
		_, base, _, _, ok := view.Access().Exact().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid exact Lens for Entry")
		}
		return one(base)
	case keyspace.FamilyLensKey:
		_, base, _, ok := view.Access().Dynamic().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid dynamic Lens for Entry")
		}
		return one(base)
	case keyspace.FamilyUnary, keyspace.FamilyValueClaim:
		var child keyspace.Term
		var ok bool
		if keyspace.TermFamily(term) == keyspace.FamilyUnary {
			_, _, child, ok = view.Operators().Unaries().Get(term)
		} else {
			_, child, _, ok = view.Claims().Get(term)
		}
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid unary/claim for Entry")
		}
		return one(child)
	case keyspace.FamilyBinary:
		_, _, left, right, ok := view.Operators().Binaries().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Binary for Entry")
		}
		if index == 0 {
			return left, true, nil
		}
		if index == 1 {
			return right, true, nil
		}
		return 0, false, nil
	case keyspace.FamilySelect:
		_, _, left, right, ok := view.Operators().Selects().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Select for Entry")
		}
		if index == 0 {
			return left, true, nil
		}
		if index == 1 {
			return right, true, nil
		}
		return 0, false, nil
	case keyspace.FamilyValues:
		_, tail, ok := view.Values().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Values for Entry")
		}
		length, ok := view.Values().Len(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Values extent for Entry")
		}
		if index < length {
			member, ok := view.Values().Member(term, index)
			if !ok {
				return 0, false, errors.New("program/flow/evaluation: missing Values member")
			}
			return member, true, nil
		}
		if index == length && tail != 0 {
			return tail, true, nil
		}
		return 0, false, nil
	case keyspace.FamilyBind:
		_, values, ok := view.Storage().Binds().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Bind for Entry")
		}
		return one(values)
	case keyspace.FamilyAssign:
		owner, _, ok := view.Storage().Assigns().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Assign for Entry")
		}
		writes := view.Storage().Assigns()
		count, ok := writes.WriteCount(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Assign writes for Entry")
		}
		nonCell := 0
		for at := 0; at < count; at++ {
			write, ok := writes.WriteAt(term, at)
			if !ok {
				return 0, false, errors.New("program/flow/evaluation: missing Assign Write")
			}
			_, target, ok := view.Storage().Writes().Get(write)
			if !ok {
				return 0, false, errors.New("program/flow/evaluation: missing Write target")
			}
			if !hasFamily(counts, target, keyspace.FamilyCell) {
				if nonCell == index {
					return target, true, nil
				}
				nonCell++
			}
		}
		_, values, ok := view.Storage().Assigns().Get(term)
		if !ok || owner == 0 {
			return 0, false, errors.New("program/flow/evaluation: invalid Assign values")
		}
		return one(values)
	case keyspace.FamilyCall:
		_, callee, _, _, ok := view.Calls().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Call for Entry")
		}
		return one(callee)
	case keyspace.FamilyTableField:
		_, key, values, fieldKind, ok := view.Fields().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid TableField for Entry")
		}
		if fieldKind == kind.FieldKey || fieldKind == kind.FieldExact {
			return one(key)
		}
		return one(values)
	case keyspace.FamilyReturn:
		_, values, ok := view.Control().Returns().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Return for Entry")
		}
		return one(values)
	case keyspace.FamilyBranch:
		_, condition, _, _, ok := view.Control().Branches().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Branch for Entry")
		}
		return one(condition)
	case keyspace.FamilyLoop:
		_, _, loopKind, control, ok := view.Control().Loops().Get(term)
		if !ok {
			return 0, false, errors.New("program/flow/evaluation: invalid Loop for Entry")
		}
		if loopKind == kind.LoopRepeat {
			return 0, false, nil
		}
		return one(control)
	default:
		return 0, false, nil
	}
}

func completeEntry(ports *Ports, view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term) (keyspace.Term, error) {
	family := keyspace.TermFamily(term)
	switch family {
	case keyspace.FamilyTable, keyspace.FamilyBody, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat,
		keyspace.FamilyString, keyspace.FamilyVararg, keyspace.FamilyFunction, keyspace.FamilyTypeValue:
		return term, nil
	case keyspace.FamilyLoop:
		_, body, loopKind, _, ok := view.Control().Loops().Get(term)
		if !ok {
			return 0, errors.New("program/flow/evaluation: invalid Loop completion")
		}
		if loopKind == kind.LoopRepeat {
			return body, nil
		}
	}
	child, has, err := entryChild(view, counts, term, 0)
	if err != nil {
		return 0, err
	}
	if !has {
		return term, nil
	}
	entry, ok := ports.Entry(child)
	if !ok {
		return 0, errors.New("program/flow/evaluation: Entry child is unresolved")
	}
	return entry, nil
}
