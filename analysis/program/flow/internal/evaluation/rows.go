package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func validateRows(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	if err := validateValues(view, counts, forest); err != nil {
		return err
	}
	if err := validateAccessStorage(view, counts, forest); err != nil {
		return err
	}
	if err := validateOperators(view, counts, forest); err != nil {
		return err
	}
	if err := validateTables(view, counts, forest); err != nil {
		return err
	}
	if err := validateFunctionsCalls(view, counts, forest); err != nil {
		return err
	}
	if err := validateControl(view, counts, forest); err != nil {
		return err
	}
	return validateClaims(view, counts, forest)
}

func validateValues(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	values := view.Values()
	for ordinal := 1; ordinal <= values.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyValues, uint32(ordinal))
		owner, tail, ok := values.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/evaluation: invalid Values owner")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		length, ok := values.Len(term)
		if !ok || length < 0 {
			return errors.New("program/flow/evaluation: invalid Values extent")
		}
		for index := 0; index < length; index++ {
			member, memberOK := values.Member(term, index)
			if !memberOK || !valueOccurrence(counts, member) {
				return errors.New("program/flow/evaluation: invalid Values member")
			}
			if err := requireParent(forest, member, term); err != nil {
				return err
			}
		}
		if tail != 0 && !openOccurrence(counts, tail) {
			return errors.New("program/flow/evaluation: invalid Values tail")
		}
		if tail != 0 {
			if err := requireParent(forest, tail, term); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAccessStorage(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	access := view.Access()
	exact := access.Exact()
	for ordinal := 1; ordinal <= exact.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(ordinal))
		owner, base, source, fieldKind, ok := exact.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !valueOccurrence(counts, base) ||
			!staticLensSource(view, counts, source, fieldKind) {
			return errors.New("program/flow/evaluation: invalid exact Lens")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, base, term); err != nil {
			return err
		}
		if !staticReference(source, fieldKind) {
			if err := requireParent(forest, source, term); err != nil {
				return err
			}
		}
	}
	dynamic := access.Dynamic()
	for ordinal := 1; ordinal <= dynamic.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLensKey, uint32(ordinal))
		owner, base, key, ok := dynamic.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !valueOccurrence(counts, base) || !valueOccurrence(counts, key) {
			return errors.New("program/flow/evaluation: invalid dynamic Lens")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, base, term); err != nil {
			return err
		}
		if err := requireParent(forest, key, term); err != nil {
			return err
		}
	}
	if err := validateCells(view, counts); err != nil {
		return err
	}
	reads := view.Storage().Reads()
	for ordinal := 1; ordinal <= reads.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyRead, uint32(ordinal))
		owner, source, implicit, ok := reads.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !readSource(counts, source) {
			return errors.New("program/flow/evaluation: invalid Read")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if implicit && !globalCell(view, counts, source) {
			return errors.New("program/flow/evaluation: implicit Read is not global")
		}
		if hasFamily(counts, source, keyspace.FamilyLensExact) || hasFamily(counts, source, keyspace.FamilyLensKey) {
			if err := requireParent(forest, source, term); err != nil {
				return err
			}
		}
	}
	varargs := view.Storage().Varargs()
	for ordinal := 1; ordinal <= varargs.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyVararg, uint32(ordinal))
		owner, cell, ok := varargs.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !localCell(view, counts, cell) {
			return errors.New("program/flow/evaluation: invalid Vararg")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
	}
	storage := view.Storage()
	for ordinal := 1; ordinal <= storage.Binds().Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBind, uint32(ordinal))
		owner, values, ok := storage.Binds().Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !hasFamily(counts, values, keyspace.FamilyValues) {
			return errors.New("program/flow/evaluation: invalid Bind")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, values, term); err != nil {
			return err
		}
	}
	assigns := storage.Assigns()
	for ordinal := 1; ordinal <= assigns.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyAssign, uint32(ordinal))
		owner, values, ok := assigns.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !hasFamily(counts, values, keyspace.FamilyValues) {
			return errors.New("program/flow/evaluation: invalid Assign")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, values, term); err != nil {
			return err
		}
		writeCount, ok := assigns.WriteCount(term)
		if !ok || writeCount <= 0 {
			return errors.New("program/flow/evaluation: Assign has no Write")
		}
		for index := 0; index < writeCount; index++ {
			write, ok := assigns.WriteAt(term, index)
			if !ok || !hasFamily(counts, write, keyspace.FamilyWrite) {
				return errors.New("program/flow/evaluation: invalid Assign Write range")
			}
			parent, target, ok := storage.Writes().Get(write)
			if !ok || parent != term || !writableTarget(view, counts, target, owner, forest) {
				return errors.New("program/flow/evaluation: invalid Assign target")
			}
			if err := requireOwner(view, forest, owner, write); err != nil {
				return err
			}
			if err := requireParent(forest, write, term); err != nil {
				return err
			}
			if hasFamily(counts, target, keyspace.FamilyLensExact) || hasFamily(counts, target, keyspace.FamilyLensKey) {
				if err := requireParent(forest, target, write); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateCells(view authored.View, counts [keyspace.FamilyCount]int) error {
	cells := view.Storage().Cells()
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		cellKind, body, key, ok := cells.Get(term)
		if !ok || (cellKind != authored.CellLocal && cellKind != authored.CellGlobal) {
			return errors.New("program/flow/evaluation: invalid Cell")
		}
		if cellKind == authored.CellLocal {
			if !hasFamily(counts, body, keyspace.FamilyBody) || key != 0 {
				return errors.New("program/flow/evaluation: invalid local Cell")
			}
		} else if body != 0 || key == 0 {
			return errors.New("program/flow/evaluation: invalid global Cell")
		}
	}
	return nil
}

func validateOperators(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	operators := view.Operators()
	unaries := operators.Unaries()
	for ordinal := 1; ordinal <= unaries.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyUnary, uint32(ordinal))
		owner, op, operand, ok := unaries.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || op < kind.UnaryNeg || op > kind.UnaryBitNot || !valueOccurrence(counts, operand) {
			return errors.New("program/flow/evaluation: invalid Unary")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, operand, term); err != nil {
			return err
		}
	}
	binaries := operators.Binaries()
	for ordinal := 1; ordinal <= binaries.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBinary, uint32(ordinal))
		owner, op, left, right, ok := binaries.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || op < kind.BinaryAdd || op > kind.BinaryGreaterEqual ||
			!valueOccurrence(counts, left) || !valueOccurrence(counts, right) {
			return errors.New("program/flow/evaluation: invalid Binary")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, left, term); err != nil {
			return err
		}
		if err := requireParent(forest, right, term); err != nil {
			return err
		}
	}
	selects := operators.Selects()
	for ordinal := 1; ordinal <= selects.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilySelect, uint32(ordinal))
		owner, op, left, right, ok := selects.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || (op != kind.SelectAnd && op != kind.SelectOr) ||
			!valueOccurrence(counts, left) || !valueOccurrence(counts, right) {
			return errors.New("program/flow/evaluation: invalid Select")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, left, term); err != nil {
			return err
		}
		if err := requireParent(forest, right, term); err != nil {
			return err
		}
	}
	return nil
}

func validateTables(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	tables := view.Tables()
	fields := view.Fields()
	seen := make([]bool, fields.Count()+1)
	for ordinal := 1; ordinal <= tables.Count(); ordinal++ {
		table := keyspace.MakeTerm(keyspace.FamilyTable, uint32(ordinal))
		owner, ok := tables.Get(table)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/evaluation: invalid Table")
		}
		if err := requireOwner(view, forest, owner, table); err != nil {
			return err
		}
		fieldCount, ok := tables.FieldCount(table)
		if !ok || fieldCount < 0 {
			return errors.New("program/flow/evaluation: invalid Table field extent")
		}
		for index := 0; index < fieldCount; index++ {
			field, ok := tables.FieldAt(table, index)
			if !ok || !hasFamily(counts, field, keyspace.FamilyTableField) {
				return errors.New("program/flow/evaluation: invalid Table field order")
			}
			fieldOrdinal := keyspace.TermOrdinal(field)
			if fieldOrdinal == 0 || uint64(fieldOrdinal) > uint64(fields.Count()) || seen[fieldOrdinal] {
				return errors.New("program/flow/evaluation: duplicate Table field")
			}
			seen[fieldOrdinal] = true
			fieldTable, key, values, fieldKind, ok := fields.Get(field)
			if !ok || fieldTable != table || !hasFamily(counts, values, keyspace.FamilyValues) || !fieldKey(view, counts, key, fieldKind) {
				return errors.New("program/flow/evaluation: invalid TableField")
			}
			if err := requireOwner(view, forest, owner, field); err != nil {
				return err
			}
			if err := requireParent(forest, field, table); err != nil {
				return err
			}
			if !staticReference(key, fieldKind) {
				if err := requireParent(forest, key, field); err != nil {
					return err
				}
			}
			if err := requireParent(forest, values, field); err != nil {
				return err
			}
		}
	}
	for ordinal := 1; ordinal <= fields.Count(); ordinal++ {
		if !seen[ordinal] {
			return errors.New("program/flow/evaluation: orphan TableField")
		}
	}
	return nil
}

func validateFunctionsCalls(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	functions := view.Functions()
	for ordinal := 1; ordinal <= functions.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(ordinal))
		owner, body, vararg, ok := functions.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !hasFamily(counts, body, keyspace.FamilyBody) || owner == body {
			return errors.New("program/flow/evaluation: invalid Function")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, body, owner); err != nil {
			return err
		}
		if vararg != 0 && !localCellInBody(view, counts, vararg, body) {
			return errors.New("program/flow/evaluation: invalid Function Vararg")
		}
	}
	calls := view.Calls()
	for ordinal := 1; ordinal <= calls.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyCall, uint32(ordinal))
		owner, callee, receiver, actuals, ok := calls.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !valueOccurrence(counts, callee) || !hasFamily(counts, actuals, keyspace.FamilyValues) {
			return errors.New("program/flow/evaluation: invalid Call")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, callee, term); err != nil {
			return err
		}
		if err := requireParent(forest, actuals, term); err != nil {
			return err
		}
		if receiver != 0 && (!valueOccurrence(counts, receiver) || !methodCallee(view, counts, owner, callee, receiver)) {
			return errors.New("program/flow/evaluation: invalid Call receiver")
		}
	}
	return nil
}

func validateControl(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	control := view.Control()
	returns := control.Returns()
	for ordinal := 1; ordinal <= returns.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyReturn, uint32(ordinal))
		owner, values, ok := returns.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !hasFamily(counts, values, keyspace.FamilyValues) {
			return errors.New("program/flow/evaluation: invalid Return")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, values, term); err != nil {
			return err
		}
	}
	breaks := control.Breaks()
	for ordinal := 1; ordinal <= breaks.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBreak, uint32(ordinal))
		owner, ok := breaks.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/evaluation: invalid Break")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
	}
	labels := control.Labels()
	for ordinal := 1; ordinal <= labels.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLabel, uint32(ordinal))
		owner, ok := labels.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/evaluation: invalid Label")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
	}
	gotos := control.Gotos()
	for ordinal := 1; ordinal <= gotos.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyGoto, uint32(ordinal))
		owner, target, ok := gotos.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !hasFamily(counts, target, keyspace.FamilyLabel) {
			return errors.New("program/flow/evaluation: invalid Goto")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
	}
	branches := control.Branches()
	for ordinal := 1; ordinal <= branches.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBranch, uint32(ordinal))
		owner, condition, whenTrue, whenFalse, ok := branches.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !valueOccurrence(counts, condition) ||
			!hasFamily(counts, whenTrue, keyspace.FamilyBody) || !hasFamily(counts, whenFalse, keyspace.FamilyBody) ||
			owner == whenTrue || owner == whenFalse || whenTrue == whenFalse {
			return errors.New("program/flow/evaluation: invalid Branch")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, condition, term); err != nil {
			return err
		}
		if err := requireParent(forest, whenTrue, owner); err != nil {
			return err
		}
		if err := requireParent(forest, whenFalse, owner); err != nil {
			return err
		}
	}
	loops := control.Loops()
	for ordinal := 1; ordinal <= loops.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLoop, uint32(ordinal))
		owner, body, loopKind, controlTerm, ok := loops.Get(term)
		validControl := false
		switch loopKind {
		case kind.LoopWhile, kind.LoopRepeat:
			validControl = valueOccurrence(counts, controlTerm)
		case kind.LoopNumericFor, kind.LoopGenericFor:
			validControl = hasFamily(counts, controlTerm, keyspace.FamilyValues)
		}
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || !hasFamily(counts, body, keyspace.FamilyBody) || owner == body || !validControl {
			return errors.New("program/flow/evaluation: invalid Loop")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, body, owner); err != nil {
			return err
		}
		if err := requireParent(forest, controlTerm, term); err != nil {
			return err
		}
		cellCount, ok := loops.CellCount(term)
		if !ok || cellCount < 0 {
			return errors.New("program/flow/evaluation: invalid Loop cell extent")
		}
		for index := 0; index < cellCount; index++ {
			cell, ok := loops.CellAt(term, index)
			if !ok || !localCellInBody(view, counts, cell, body) {
				return errors.New("program/flow/evaluation: invalid Loop Cell")
			}
		}
	}
	return nil
}

func validateClaims(view authored.View, counts [keyspace.FamilyCount]int, forest *containment.Result) error {
	claims := view.Claims()
	for ordinal := 1; ordinal <= claims.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyValueClaim, uint32(ordinal))
		owner, operand, claimKind, ok := claims.Get(term)
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) || claimKind < kind.ValueClaimTypeAs || claimKind > kind.ValueClaimNonNil || !valueOccurrence(counts, operand) {
			return errors.New("program/flow/evaluation: invalid ValueClaim")
		}
		if err := requireOwner(view, forest, owner, term); err != nil {
			return err
		}
		if err := requireParent(forest, operand, term); err != nil {
			return err
		}
	}
	types := view.TypeValues()
	for ordinal := 1; ordinal <= types.Count(); ordinal++ {
		owner, ok := types.Get(keyspace.MakeTerm(keyspace.FamilyTypeValue, uint32(ordinal)))
		if !ok || !hasFamily(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/evaluation: invalid TypeValue")
		}
	}
	return nil
}

func staticLensSource(view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term, fieldKind kind.FieldKind) bool {
	if fieldKind == kind.FieldName {
		return hasFamily(counts, term, keyspace.FamilyKey)
	}
	if fieldKind != kind.FieldExact {
		return false
	}
	return exactKeyTerm(view, term, fieldKind, counts)
}

func fieldKey(view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term, fieldKind kind.FieldKind) bool {
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		return hasFamily(counts, term, keyspace.FamilyKey)
	case kind.FieldExact:
		return exactKeyTerm(view, term, fieldKind, counts)
	case kind.FieldKey:
		return valueOccurrence(counts, term)
	default:
		return false
	}
}

func exactKeyTerm(view authored.View, term keyspace.Term, fieldKind kind.FieldKind, counts [keyspace.FamilyCount]int) bool {
	if fieldKind != kind.FieldExact || !validTerm(counts, term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		return true
	case keyspace.FamilyUnary:
		_, op, operand, ok := view.Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg {
			return false
		}
		return hasFamily(counts, operand, keyspace.FamilyInteger) || hasFamily(counts, operand, keyspace.FamilyFloat)
	default:
		return false
	}
}

func readSource(counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	return validTerm(counts, term) && flowrole.AddressableFamily(keyspace.TermFamily(term))
}

func localCell(view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	if !hasFamily(counts, term, keyspace.FamilyCell) {
		return false
	}
	kindValue, _, _, ok := view.Storage().Cells().Get(term)
	return ok && kindValue == authored.CellLocal
}

func localCellInBody(view authored.View, counts [keyspace.FamilyCount]int, term, body keyspace.Term) bool {
	if !localCell(view, counts, term) {
		return false
	}
	_, owner, _, ok := view.Storage().Cells().Get(term)
	return ok && owner == body
}

func globalCell(view authored.View, counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	if !hasFamily(counts, term, keyspace.FamilyCell) {
		return false
	}
	kindValue, _, _, ok := view.Storage().Cells().Get(term)
	return ok && kindValue == authored.CellGlobal
}

func writableTarget(view authored.View, counts [keyspace.FamilyCount]int, target, owner keyspace.Term, forest *containment.Result) bool {
	if hasFamily(counts, target, keyspace.FamilyCell) {
		kindValue, body, _, ok := view.Storage().Cells().Get(target)
		if !ok {
			return false
		}
		if kindValue == authored.CellGlobal {
			return true
		}
		// A local Cell is writable from its declaring Body and every nested
		// lexical Body.  The sealed containment proof is the sole authority
		// for that ancestry; equality with the assignment row's Body rejects
		// valid closure/loop writes.
		return kindValue == authored.CellLocal && forest != nil && forest.Contains(body, owner)
	}
	if hasFamily(counts, target, keyspace.FamilyLensExact) {
		rowOwner, _, _, _, ok := view.Access().Exact().Get(target)
		return ok && rowOwner == owner
	}
	if hasFamily(counts, target, keyspace.FamilyLensKey) {
		rowOwner, _, _, ok := view.Access().Dynamic().Get(target)
		return ok && rowOwner == owner
	}
	return false
}

func methodCallee(view authored.View, counts [keyspace.FamilyCount]int, owner, callee, receiver keyspace.Term) bool {
	if !hasFamily(counts, callee, keyspace.FamilyRead) {
		return false
	}
	readOwner, sourceTerm, _, ok := view.Storage().Reads().Get(callee)
	if !ok || readOwner != owner || !hasFamily(counts, sourceTerm, keyspace.FamilyLensExact) {
		return false
	}
	lensOwner, base, _, fieldKind, ok := view.Access().Exact().Get(sourceTerm)
	return ok && lensOwner == owner && base == receiver && fieldKind == kind.FieldName
}

// staticReference identifies Source/static metadata used as a key spelling,
// rather than an evaluated child. Such terms have typed cardinality validation
// but must not be assigned a synthetic parent by this seal.
func staticReference(term keyspace.Term, fieldKind kind.FieldKind) bool {
	if fieldKind == kind.FieldName || fieldKind == kind.FieldList {
		return keyspace.TermFamily(term) == keyspace.FamilyKey
	}
	if fieldKind == kind.FieldExact {
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString:
			return true
		}
	}
	return false
}

func requireParent(forest *containment.Result, child, parent keyspace.Term) error {
	if forest == nil {
		return errors.New("program/flow/evaluation: containment proof is unavailable")
	}
	got, ok := forest.Parent(child)
	if !ok || got != parent {
		return errors.New("program/flow/evaluation: structural child crosses containment parent")
	}
	return nil
}

// requireOwner checks the row's declared Body against the already-sealed
// containment proof. It queries the canonical relation directly and retains
// no owner table or inferred graph in this package.
func requireOwner(view authored.View, forest *containment.Result, owner, term keyspace.Term) error {
	if forest == nil || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
		return errors.New("program/flow/evaluation: row owner crosses containment")
	}
	// Walk the canonical parent proof once. The first Body is the structural
	// owner; a Repeat Loop encountered before that Body is the one deliberate
	// evaluation frontier whose condition is owned by the Loop Body instead.
	// This simultaneously enforces Repeat exclusivity and avoids a
	// Contains/prewalk plus nearest-Body second scan.
	var nearestBody, repeatBody keyspace.Term
	for current := term; ; {
		parent, ok := forest.Parent(current)
		if !ok {
			return errors.New("program/flow/evaluation: row owner crosses containment")
		}
		if nearestBody == 0 && keyspace.TermFamily(parent) == keyspace.FamilyLoop {
			_, body, loopKind, _, rowOK := view.Control().Loops().Get(parent)
			if rowOK && loopKind == kind.LoopRepeat {
				repeatBody = body
			}
		}
		if keyspace.TermFamily(parent) == keyspace.FamilyBody {
			nearestBody = parent
			break
		}
		current = parent
	}
	// A Repeat control is structurally under the enclosing Body, but its
	// evaluation owner is the Repeat Body. Only that direct frontier may use
	// the alternate owner; a nested Body encountered first never inherits it.
	if repeatBody != 0 && repeatBody != nearestBody {
		if owner == repeatBody {
			return nil
		}
		return errors.New("program/flow/evaluation: row owner crosses containment")
	}
	if owner == nearestBody {
		return nil
	}
	return errors.New("program/flow/evaluation: row owner crosses containment")
}
