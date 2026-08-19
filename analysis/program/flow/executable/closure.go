package executable

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

type walker struct {
	view   authored.View
	source source.View
	forest *containment.Result
	counts [keyspace.FamilyCount]uint32
	result *Result
	work   []keyspace.Term
	entry  keyspace.Term
	// functionBodies is Seal-local scratch used only to distinguish the
	// entry's chunk-vararg Cell from a malformed Function-owned Vararg row.
	functionBodies []bool
}

func closeOperands(
	view authored.View,
	sourceView source.View,
	forest *containment.Result,
	counts [keyspace.FamilyCount]uint32,
	seed rootSeed,
) (*Result, error) {
	w := walker{
		view:   view,
		source: sourceView,
		forest: forest,
		counts: counts,
		result: seed.result,
		work:   seed.work,
		entry:  seed.entry,
	}
	var err error
	w.functionBodies, err = functionBodyMarks(view, counts)
	if err != nil {
		return nil, err
	}
	for len(w.work) != 0 {
		last := len(w.work) - 1
		term := w.work[last]
		w.work = w.work[:last]
		if !w.result.Contains(term) {
			continue
		}
		if err := w.close(term); err != nil {
			return nil, err
		}
	}
	return w.result, nil
}

func (w *walker) offer(term keyspace.Term) error {
	if term == 0 {
		return nil
	}
	if !validTerm(term, w.counts) {
		return errors.New("program/flow/executable: invalid executable operand")
	}
	if w.forest.Static(term) || !runtimeFamily(keyspace.TermFamily(term)) {
		return nil
	}
	if keyspace.TermFamily(term) == keyspace.FamilyCell {
		cellKind, body, key, ok := w.view.Storage().Cells().Get(term)
		if !ok || (cellKind == authored.CellLocal && (body == 0 || key != 0)) ||
			(cellKind == authored.CellGlobal && (body != 0 || key == 0)) {
			return errors.New("program/flow/executable: malformed Cell operand")
		}
		if cellKind == authored.CellGlobal {
			return nil
		}
	}
	if w.result.mark(term) {
		w.work = append(w.work, term)
	}
	return nil
}

func (w *walker) required(term keyspace.Term) error {
	if term == 0 {
		return errors.New("program/flow/executable: missing executable operand")
	}
	return w.offer(term)
}

func (w *walker) close(term keyspace.Term) error {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		return nil
	case keyspace.FamilyBody:
		if term == w.entry {
			return w.closeChunkVararg()
		}
		return nil
	case keyspace.FamilyTypeValue, keyspace.FamilyBreak, keyspace.FamilyGoto:
		return nil
	case keyspace.FamilyCell:
		return w.closeCell(term)
	case keyspace.FamilyRead:
		owner, sourceTerm, _, ok := w.view.Storage().Reads().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed Read")
		}
		return w.required(sourceTerm)
	case keyspace.FamilyVararg:
		owner, cell, ok := w.view.Storage().Varargs().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed Vararg")
		}
		return w.required(cell)
	case keyspace.FamilyLensExact:
		return w.closeExactLens(term)
	case keyspace.FamilyLensKey:
		owner, base, key, ok := w.view.Access().Dynamic().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed dynamic Lens")
		}
		if err := w.required(base); err != nil {
			return err
		}
		return w.required(key)
	case keyspace.FamilyUnary:
		owner, _, operand, ok := w.view.Operators().Unaries().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed Unary")
		}
		return w.required(operand)
	case keyspace.FamilyBinary:
		owner, _, left, right, ok := w.view.Operators().Binaries().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed Binary")
		}
		if err := w.required(left); err != nil {
			return err
		}
		return w.required(right)
	case keyspace.FamilySelect:
		owner, _, left, right, ok := w.view.Operators().Selects().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed Select")
		}
		if err := w.required(left); err != nil {
			return err
		}
		return w.required(right)
	case keyspace.FamilyValues:
		return w.closeValues(term)
	case keyspace.FamilyValueClaim:
		owner, operand, _, ok := w.view.Claims().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed ValueClaim")
		}
		return w.required(operand)
	case keyspace.FamilyBind:
		return w.closeBind(term)
	case keyspace.FamilyAssign:
		return w.closeAssign(term)
	case keyspace.FamilyWrite:
		return w.closeWrite(term)
	case keyspace.FamilyFunction:
		return w.closeFunction(term)
	case keyspace.FamilyCall:
		return w.closeCall(term)
	case keyspace.FamilyBranch:
		owner, condition, whenTrue, whenFalse, ok := w.view.Control().Branches().Get(term)
		if !ok || !bodyTerm(owner, w.counts) || !bodyTerm(whenTrue, w.counts) || !bodyTerm(whenFalse, w.counts) {
			return errors.New("program/flow/executable: malformed Branch")
		}
		return w.required(condition)
	case keyspace.FamilyLoop:
		return w.closeLoop(term)
	case keyspace.FamilyTable:
		return w.closeTable(term)
	case keyspace.FamilyTableField:
		return w.closeTableField(term)
	case keyspace.FamilyReturn:
		owner, values, ok := w.view.Control().Returns().Get(term)
		if !ok || !bodyTerm(owner, w.counts) {
			return errors.New("program/flow/executable: malformed Return")
		}
		return w.required(values)
	default:
		return errors.New("program/flow/executable: executable family has no operand law")
	}
}

func functionBodyMarks(view authored.View, counts [keyspace.FamilyCount]uint32) ([]bool, error) {
	marks := make([]bool, counts[keyspace.FamilyBody]+1)
	functions := view.Functions()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyFunction]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyFunction, ordinal)
		if got, ok := functions.At(int(ordinal - 1)); !ok || got != term {
			return nil, errors.New("program/flow/executable: malformed Function ordinal")
		}
		owner, body, _, ok := functions.Get(term)
		if !ok || !bodyTerm(owner, counts) || !bodyTerm(body, counts) || owner == body {
			return nil, errors.New("program/flow/executable: malformed Function Body")
		}
		bodyOrdinal := keyspace.TermOrdinal(body)
		if marks[bodyOrdinal] {
			return nil, errors.New("program/flow/executable: duplicate Function Body")
		}
		marks[bodyOrdinal] = true
	}
	return marks, nil
}

func (w *walker) closeChunkVararg() error {
	varargs := w.view.Storage().Varargs()
	cells := w.view.Storage().Cells()
	var chunk keyspace.Term
	for ordinal := uint32(1); ordinal <= w.counts[keyspace.FamilyVararg]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyVararg, ordinal)
		if got, ok := varargs.At(int(ordinal - 1)); !ok || got != term {
			return errors.New("program/flow/executable: malformed Vararg ordinal")
		}
		owner, cell, ok := varargs.Get(term)
		if !ok || !bodyTerm(owner, w.counts) || !termInFamily(cell, keyspace.FamilyCell, w.counts) {
			return errors.New("program/flow/executable: malformed chunk Vararg row")
		}
		cellKind, body, key, ok := cells.Get(cell)
		if !ok || cellKind != authored.CellLocal || key != 0 || !bodyTerm(body, w.counts) {
			return errors.New("program/flow/executable: chunk Vararg does not name a local Cell")
		}
		if body != w.entry {
			continue
		}
		ownerOrdinal := keyspace.TermOrdinal(owner)
		if uint64(ownerOrdinal) >= uint64(len(w.functionBodies)) || w.functionBodies[ownerOrdinal] {
			return errors.New("program/flow/executable: Function-owned Vararg claims the chunk Cell")
		}
		if chunk != 0 && chunk != cell {
			return errors.New("program/flow/executable: conflicting chunk Vararg Cells")
		}
		chunk = cell
	}
	return w.offer(chunk)
}

func (w *walker) closeCell(term keyspace.Term) error {
	cellKind, body, key, ok := w.view.Storage().Cells().Get(term)
	if !ok {
		return errors.New("program/flow/executable: malformed Cell")
	}
	switch cellKind {
	case authored.CellGlobal:
		if body != 0 || key == 0 {
			return errors.New("program/flow/executable: malformed global Cell")
		}
	case authored.CellLocal:
		if !bodyTerm(body, w.counts) || key != 0 {
			return errors.New("program/flow/executable: malformed local Cell")
		}
	default:
		return errors.New("program/flow/executable: unknown Cell kind")
	}
	return nil
}

func (w *walker) closeExactLens(term keyspace.Term) error {
	owner, base, sourceTerm, fieldKind, ok := w.view.Access().Exact().Get(term)
	if !ok || !bodyTerm(owner, w.counts) ||
		(fieldKind < kind.FieldList || fieldKind > kind.FieldKey) {
		return errors.New("program/flow/executable: malformed exact Lens")
	}
	if err := w.required(base); err != nil {
		return err
	}
	if fieldKind == kind.FieldExact {
		return w.required(sourceTerm)
	}
	return nil
}

func (w *walker) closeValues(term keyspace.Term) error {
	owner, tail, ok := w.view.Values().Get(term)
	if !ok || !bodyTerm(owner, w.counts) {
		return errors.New("program/flow/executable: malformed Values")
	}
	length, lengthOK := w.view.Values().Len(term)
	if !lengthOK || length < 0 {
		return errors.New("program/flow/executable: malformed Values range")
	}
	for index := 0; index < length; index++ {
		member, memberOK := w.view.Values().Member(term, index)
		if !memberOK {
			return errors.New("program/flow/executable: malformed Values member")
		}
		if err := w.required(member); err != nil {
			return err
		}
	}
	return w.offer(tail)
}

func (w *walker) closeBind(term keyspace.Term) error {
	owner, values, ok := w.view.Storage().Binds().Get(term)
	if !ok || !bodyTerm(owner, w.counts) {
		return errors.New("program/flow/executable: malformed Bind")
	}
	if err := w.required(values); err != nil {
		return err
	}
	order := w.source.Binds()
	length, lengthOK := order.Len(term)
	if !lengthOK || length < 0 {
		return errors.New("program/flow/executable: Bind cell order is unavailable")
	}
	for index := 0; index < length; index++ {
		cell, cellOK := order.At(term, index)
		if !cellOK {
			return errors.New("program/flow/executable: malformed Bind cell order")
		}
		if err := w.required(cell); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) closeAssign(term keyspace.Term) error {
	owner, values, ok := w.view.Storage().Assigns().Get(term)
	if !ok || !bodyTerm(owner, w.counts) {
		return errors.New("program/flow/executable: malformed Assign")
	}
	if err := w.required(values); err != nil {
		return err
	}
	assigns := w.view.Storage().Assigns()
	length, lengthOK := assigns.WriteCount(term)
	if !lengthOK || length < 0 {
		return errors.New("program/flow/executable: Assign write range is unavailable")
	}
	for index := 0; index < length; index++ {
		write, writeOK := assigns.WriteAt(term, index)
		if !writeOK {
			return errors.New("program/flow/executable: malformed Assign write range")
		}
		if err := w.required(write); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) closeWrite(term keyspace.Term) error {
	assign, target, ok := w.view.Storage().Writes().Get(term)
	if !ok || !termInFamily(assign, keyspace.FamilyAssign, w.counts) {
		return errors.New("program/flow/executable: malformed Write")
	}
	return w.required(target)
}

func (w *walker) closeFunction(term keyspace.Term) error {
	owner, body, vararg, ok := w.view.Functions().Get(term)
	if !ok || !bodyTerm(owner, w.counts) || !bodyTerm(body, w.counts) || owner == body {
		return errors.New("program/flow/executable: malformed Function")
	}
	if err := w.required(body); err != nil {
		return err
	}
	if err := w.offer(vararg); err != nil {
		return err
	}
	formals := w.source.Formals()
	formalCount, formalOK := formals.Len(term)
	if !formalOK || formalCount < 0 {
		return errors.New("program/flow/executable: Function formal order is unavailable")
	}
	for index := 0; index < formalCount; index++ {
		formal, formalOK := formals.At(term, index)
		if !formalOK {
			return errors.New("program/flow/executable: malformed Function formal order")
		}
		if err := w.required(formal); err != nil {
			return err
		}
	}
	functions := w.view.Functions()
	captureCount, captureOK := functions.CaptureCount(term)
	if !captureOK || captureCount < 0 {
		return errors.New("program/flow/executable: malformed Function capture range")
	}
	for index := 0; index < captureCount; index++ {
		inner, outer, captureOK := functions.CaptureAt(term, index)
		if !captureOK {
			return errors.New("program/flow/executable: malformed Function capture")
		}
		if err := w.required(inner); err != nil {
			return err
		}
		if err := w.required(outer); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) closeCall(term keyspace.Term) error {
	owner, callee, receiver, actuals, ok := w.view.Calls().Get(term)
	if !ok || !bodyTerm(owner, w.counts) {
		return errors.New("program/flow/executable: malformed Call")
	}
	if err := w.required(callee); err != nil {
		return err
	}
	if err := w.offer(receiver); err != nil {
		return err
	}
	return w.required(actuals)
}

func (w *walker) closeLoop(term keyspace.Term) error {
	owner, body, loopKind, control, ok := w.view.Control().Loops().Get(term)
	if !ok || !bodyTerm(owner, w.counts) || !bodyTerm(body, w.counts) || owner == body ||
		loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
		return errors.New("program/flow/executable: malformed Loop")
	}
	if err := w.required(control); err != nil {
		return err
	}
	loops := w.view.Control().Loops()
	cellCount, cellOK := loops.CellCount(term)
	if !cellOK || cellCount < 0 {
		return errors.New("program/flow/executable: malformed Loop cell range")
	}
	for index := 0; index < cellCount; index++ {
		cell, cellOK := loops.CellAt(term, index)
		if !cellOK {
			return errors.New("program/flow/executable: malformed Loop cell")
		}
		if err := w.required(cell); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) closeTable(term keyspace.Term) error {
	owner, ok := w.view.Tables().Get(term)
	if !ok || !bodyTerm(owner, w.counts) {
		return errors.New("program/flow/executable: malformed Table")
	}
	tables := w.view.Tables()
	fieldCount, fieldOK := tables.FieldCount(term)
	if !fieldOK || fieldCount < 0 {
		return errors.New("program/flow/executable: malformed Table field range")
	}
	for index := 0; index < fieldCount; index++ {
		field, fieldOK := tables.FieldAt(term, index)
		if !fieldOK {
			return errors.New("program/flow/executable: malformed Table field range")
		}
		if err := w.required(field); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) closeTableField(term keyspace.Term) error {
	table, key, values, fieldKind, ok := w.view.Fields().Get(term)
	if !ok || !termInFamily(table, keyspace.FamilyTable, w.counts) ||
		fieldKind < kind.FieldList || fieldKind > kind.FieldKey {
		return errors.New("program/flow/executable: malformed TableField")
	}
	if err := w.offer(key); err != nil {
		return err
	}
	return w.required(values)
}

func bodyTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	return termInFamily(term, keyspace.FamilyBody, counts)
}

func termInFamily(term keyspace.Term, family keyspace.Family, counts [keyspace.FamilyCount]uint32) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}

func runtimeFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyValues,
		keyspace.FamilyLensExact, keyspace.FamilyLensKey, keyspace.FamilyReturn,
		keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyBody,
		keyspace.FamilyCell, keyspace.FamilyRead, keyspace.FamilyVararg,
		keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilySelect,
		keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyFunction,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyTable, keyspace.FamilyTypeValue, keyspace.FamilyValueClaim,
		keyspace.FamilyWrite, keyspace.FamilyTableField:
		return true
	default:
		return false
	}
}
