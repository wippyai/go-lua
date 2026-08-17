package flow

import (
	"errors"

	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func denseTerm(term keyspace.Term, family keyspace.Family, next int) bool {
	return term != 0 && keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) == uint32(next)
}

func countedFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return term != 0 && keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[family]
}

func body(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return countedFamily(counts, term, keyspace.FamilyBody)
}

func reject(message string) error { return errors.New("program/lower/collector: " + message) }

func (r *Rows) AdmitValues(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term, fixed []keyspace.Term, tail keyspace.Term) error {
	if r == nil || !body(counts, owner) || !denseTerm(term, keyspace.FamilyValues, len(r.values.rows)+1) {
		return reject("invalid Values admission")
	}
	for _, operand := range fixed {
		if !flowrole.ValueOccurrence(counts, operand) {
			return reject("invalid Values fixed operand")
		}
	}
	if tail != 0 && !flowrole.OpenOccurrence(counts, tail) {
		return reject("invalid Values tail")
	}
	span, ok := rangeFor(len(r.values.terms), len(fixed))
	if !ok {
		return reject("Values fixed operand range overflow")
	}
	row := programflow.Value{Owner: owner, Fixed: span, Tail: tail}
	if _, ok := r.AppendValue(row, fixed); !ok {
		return reject("Values fixed operand range overflow")
	}
	return nil
}

func (r *Rows) AdmitExactLens(counts [keyspace.FamilyCount]uint32, term, owner, base, key keyspace.Term, fieldKind flowkind.FieldKind, candidatePresent bool) error {
	if r == nil || !body(counts, owner) || !flowrole.ValueOccurrence(counts, base) || !flowrole.FieldSourceFamily(counts, key, fieldKind) || (fieldKind != flowkind.FieldName && fieldKind != flowkind.FieldExact) {
		return reject("invalid exact Lens admission")
	}
	if !denseTerm(term, keyspace.FamilyLensExact, len(r.access.exact)+1) {
		return reject("noncanonical exact Lens term")
	}
	if fieldKind == flowkind.FieldExact && !candidatePresent {
		return reject("exact Lens key has no exact candidate")
	}
	r.AppendExactLens(programflow.ExactLens{Owner: owner, Base: base, Source: key, Kind: fieldKind})
	return nil
}

func (r *Rows) AdmitDynamicLens(counts [keyspace.FamilyCount]uint32, term, owner, base, key keyspace.Term) error {
	if r == nil || !body(counts, owner) || !flowrole.ValueOccurrence(counts, base) || !flowrole.ValueOccurrence(counts, key) || !denseTerm(term, keyspace.FamilyLensKey, len(r.access.dynamic)+1) {
		return reject("invalid dynamic Lens admission")
	}
	r.AppendDynamicLens(programflow.DynamicLens{Owner: owner, Base: base, Key: key})
	return nil
}

// AdmitCall performs the row-local method-call proof and stores the Call.
// The caller supplies the authoritative census because Rows deliberately does
// not own Collector lifecycle or family denominators.
func (r *Rows) AdmitCall(counts [keyspace.FamilyCount]uint32, term, owner, callee, receiver, actuals keyspace.Term) error {
	if r == nil || !body(counts, owner) || !flowrole.ValueOccurrence(counts, callee) || !countedFamily(counts, actuals, keyspace.FamilyValues) || !denseTerm(term, keyspace.FamilyCall, len(r.calls.rows)+1) {
		return reject("invalid Call admission")
	}
	if receiver != 0 {
		if !flowrole.ValueOccurrence(counts, receiver) || !countedFamily(counts, callee, keyspace.FamilyRead) {
			return reject("invalid Call admission")
		}
		readOrdinal := keyspace.TermOrdinal(callee)
		if readOrdinal == 0 || int(readOrdinal) > len(r.storage.reads) {
			return reject("invalid Call admission")
		}
		read := r.storage.reads[readOrdinal-1]
		if read.Owner != owner || !countedFamily(counts, read.Source, keyspace.FamilyLensExact) {
			return reject("invalid Call admission")
		}
		lensOrdinal := keyspace.TermOrdinal(read.Source)
		if lensOrdinal == 0 || int(lensOrdinal) > len(r.access.exact) {
			return reject("invalid Call admission")
		}
		lens := r.access.exact[lensOrdinal-1]
		if lens.Owner != owner || lens.Base != receiver || lens.Kind != flowkind.FieldName {
			return reject("invalid Call admission")
		}
	}
	r.AppendCall(programflow.Call{Owner: owner, Callee: callee, Receiver: receiver, Actuals: actuals})
	return nil
}

// ModuleRequest is the narrow Values witness at the Module observation
// boundary. It returns an authored String term, never a raw request payload.
func (r *Rows) ModuleRequest(counts [keyspace.FamilyCount]uint32, call keyspace.Term) (keyspace.Term, bool) {
	if r == nil || !countedFamily(counts, call, keyspace.FamilyCall) {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(call)
	if ordinal == 0 || int(ordinal) > len(r.calls.rows) {
		return 0, false
	}
	callRow := r.calls.rows[ordinal-1]
	values, ok := r.ValueAt(int(keyspace.TermOrdinal(callRow.Actuals) - 1))
	if !countedFamily(counts, callRow.Actuals, keyspace.FamilyValues) || !ok || values.Tail != 0 || values.Fixed.End-values.Fixed.Start != 1 {
		return 0, false
	}
	if values.Fixed.Start >= values.Fixed.End || int(values.Fixed.Start) >= len(r.values.terms) {
		return 0, false
	}
	request := r.values.terms[values.Fixed.Start]
	if !countedFamily(counts, request, keyspace.FamilyString) {
		return 0, false
	}
	return request, true
}

func (r *Rows) AdmitReturn(counts [keyspace.FamilyCount]uint32, term, owner, values keyspace.Term) error {
	if r == nil || !body(counts, owner) || !countedFamily(counts, values, keyspace.FamilyValues) || !denseTerm(term, keyspace.FamilyReturn, len(r.control.returns)+1) {
		return reject("invalid Return admission")
	}
	r.AppendReturn(programflow.Return{Owner: owner, Values: values})
	return nil
}

func (r *Rows) AdmitBreak(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term) error {
	if r == nil || !body(counts, owner) || !denseTerm(term, keyspace.FamilyBreak, len(r.control.breaks)+1) {
		return reject("invalid Break admission")
	}
	r.AppendBreak(programflow.Break{Owner: owner})
	return nil
}

func (r *Rows) AdmitLabel(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term) error {
	if r == nil || !body(counts, owner) || !denseTerm(term, keyspace.FamilyLabel, len(r.control.labels)+1) {
		return reject("invalid Label admission")
	}
	r.AppendLabel(programflow.Label{Owner: owner})
	return nil
}

func (r *Rows) AdmitGoto(counts [keyspace.FamilyCount]uint32, term, owner, target keyspace.Term) error {
	if r == nil || !body(counts, owner) || !countedFamily(counts, target, keyspace.FamilyLabel) || !denseTerm(term, keyspace.FamilyGoto, len(r.control.gotos)+1) {
		return reject("invalid Goto admission")
	}
	r.AppendGoto(programflow.Goto{Owner: owner, Target: target})
	return nil
}

func (r *Rows) AdmitBranch(counts [keyspace.FamilyCount]uint32, term, owner, condition, whenTrue, whenFalse keyspace.Term) error {
	if r == nil || !body(counts, owner) || !flowrole.ValueOccurrence(counts, condition) || !body(counts, whenTrue) || !body(counts, whenFalse) || owner == whenTrue || owner == whenFalse || whenTrue == whenFalse || !denseTerm(term, keyspace.FamilyBranch, len(r.control.branches)+1) {
		return reject("invalid Branch admission")
	}
	r.AppendBranch(programflow.Branch{Owner: owner, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse})
	return nil
}

func (r *Rows) AdmitLoop(counts [keyspace.FamilyCount]uint32, term, owner, bodyTerm, control keyspace.Term, cells []keyspace.Term, loopKind flowkind.LoopKind) error {
	if r == nil || !body(counts, owner) || !body(counts, bodyTerm) || owner == bodyTerm || loopKind < flowkind.LoopWhile || loopKind > flowkind.LoopGenericFor || !flowrole.LoopControlFamily(counts, control, loopKind) || !denseTerm(term, keyspace.FamilyLoop, len(r.control.loops)+1) {
		return reject("invalid Loop admission")
	}
	if loopKind == flowkind.LoopNumericFor || loopKind == flowkind.LoopGenericFor {
		values, ok := r.ValueAt(int(keyspace.TermOrdinal(control) - 1))
		if !ok {
			return reject("invalid Loop admission")
		}
		width := values.Fixed.End - values.Fixed.Start
		if loopKind == flowkind.LoopNumericFor && (width != 2 && width != 3 || values.Tail != 0) {
			return reject("invalid Loop admission")
		}
		if loopKind == flowkind.LoopGenericFor && values.Fixed.Start == values.Fixed.End && values.Tail == 0 {
			return reject("invalid Loop admission")
		}
	}
	for index, cell := range cells {
		if !r.localCellInBody(counts, cell, bodyTerm) {
			return reject("invalid Loop Cell")
		}
		for _, previous := range cells[:index] {
			if previous == cell {
				return reject("duplicate Loop Cell")
			}
		}
	}
	if _, ok := r.AppendLoop(programflow.Loop{Owner: owner, Body: bodyTerm, Kind: loopKind, Control: control}, cells); !ok {
		return reject("Loop Cell range overflow")
	}
	return nil
}

func (r *Rows) localCell(counts [keyspace.FamilyCount]uint32, cell keyspace.Term) bool {
	if r == nil || !countedFamily(counts, cell, keyspace.FamilyCell) {
		return false
	}
	row, ok := r.CellAt(int(keyspace.TermOrdinal(cell) - 1))
	return ok && row.Kind == programflow.CellLocal
}

func (r *Rows) localCellInBody(counts [keyspace.FamilyCount]uint32, cell, bodyTerm keyspace.Term) bool {
	if !r.localCell(counts, cell) {
		return false
	}
	row, ok := r.CellAt(int(keyspace.TermOrdinal(cell) - 1))
	return ok && row.Body == bodyTerm
}

func (r *Rows) globalCell(counts [keyspace.FamilyCount]uint32, cell keyspace.Term) bool {
	if r == nil || !countedFamily(counts, cell, keyspace.FamilyCell) {
		return false
	}
	row, ok := r.CellAt(int(keyspace.TermOrdinal(cell) - 1))
	return ok && row.Kind == programflow.CellGlobal
}

func (r *Rows) AdmitCell(counts [keyspace.FamilyCount]uint32, term, bodyTerm keyspace.Term) error {
	if r == nil || !body(counts, bodyTerm) || !denseTerm(term, keyspace.FamilyCell, len(r.storage.cells)+1) {
		return reject("invalid Cell body")
	}
	r.AppendCell(programflow.Cell{Kind: programflow.CellLocal, Body: bodyTerm})
	return nil
}

func (r *Rows) AdmitRead(counts [keyspace.FamilyCount]uint32, term, owner, subject keyspace.Term, implicit bool) error {
	if r == nil || !body(counts, owner) || !flowrole.Addressable(counts, subject) || (implicit && !r.globalCell(counts, subject)) || !denseTerm(term, keyspace.FamilyRead, len(r.storage.reads)+1) {
		return reject("invalid Read admission")
	}
	r.AppendRead(programflow.Read{Owner: owner, Source: subject, Implicit: implicit})
	return nil
}

func (r *Rows) AdmitVararg(counts [keyspace.FamilyCount]uint32, term, owner, cell keyspace.Term) error {
	if r == nil || !body(counts, owner) || !r.localCell(counts, cell) || !denseTerm(term, keyspace.FamilyVararg, len(r.storage.varargs)+1) {
		return reject("invalid Vararg admission")
	}
	r.AppendVararg(programflow.Vararg{Owner: owner, Cell: cell})
	return nil
}

func (r *Rows) AdmitBind(counts [keyspace.FamilyCount]uint32, term, owner, values keyspace.Term, cells []keyspace.Term, sourceCellsAlreadyOrdered bool) error {
	if r == nil || !body(counts, owner) || !countedFamily(counts, values, keyspace.FamilyValues) || !denseTerm(term, keyspace.FamilyBind, len(r.storage.binds)+1) || sourceCellsAlreadyOrdered {
		return reject("invalid Bind admission")
	}
	seen := make(map[keyspace.Term]struct{}, len(cells))
	for _, cell := range cells {
		if !r.localCellInBody(counts, cell, owner) {
			return reject("invalid Bind Cell")
		}
		if _, duplicate := seen[cell]; duplicate {
			return reject("duplicate Bind Cell")
		}
		seen[cell] = struct{}{}
	}
	if !rangeOK(0, len(cells)) {
		return reject("Bind Cell range overflow")
	}
	r.AppendBind(programflow.Bind{Owner: owner, Values: values})
	return nil
}

func (r *Rows) AdmitAssign(counts [keyspace.FamilyCount]uint32, term, owner, values keyspace.Term, targets []keyspace.Term, targetSpansValid bool) error {
	if r == nil || !body(counts, owner) || !countedFamily(counts, values, keyspace.FamilyValues) || len(targets) == 0 || !targetSpansValid || !denseTerm(term, keyspace.FamilyAssign, len(r.storage.assigns)+1) {
		return reject("invalid Assign admission")
	}
	for _, target := range targets {
		if !flowrole.Addressable(counts, target) {
			return reject("invalid Assign target")
		}
	}
	r.AppendAssign(programflow.Assign{Owner: owner, Values: values})
	return nil
}

func (r *Rows) AdmitWrite(counts [keyspace.FamilyCount]uint32, term, assign, target keyspace.Term) error {
	if r == nil || !countedFamily(counts, assign, keyspace.FamilyAssign) || !flowrole.Addressable(counts, target) || !denseTerm(term, keyspace.FamilyWrite, len(r.storage.writes)+1) {
		return reject("invalid Write admission")
	}
	r.AppendWrite(programflow.Write{Assign: assign, Target: target})
	return nil
}

func (r *Rows) AdmitTable(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term) error {
	if r == nil || !body(counts, owner) || !denseTerm(term, keyspace.FamilyTable, len(r.tables.rows)+1) {
		return reject("invalid Table owner")
	}
	r.AppendTable(programflow.Table{Owner: owner})
	return nil
}

func (r *Rows) AdmitTableField(counts [keyspace.FamilyCount]uint32, term, table, key, values keyspace.Term, fieldKind flowkind.FieldKind, candidatePresent bool) error {
	if r == nil || !countedFamily(counts, table, keyspace.FamilyTable) || !flowrole.FieldSourceFamily(counts, key, fieldKind) || !countedFamily(counts, values, keyspace.FamilyValues) || fieldKind < flowkind.FieldList || fieldKind > flowkind.FieldKey || !denseTerm(term, keyspace.FamilyTableField, len(r.tables.fields)+1) {
		return reject("invalid TableField admission")
	}
	if fieldKind == flowkind.FieldExact && !candidatePresent && keyspace.TermFamily(key) != keyspace.FamilyNil {
		return reject("exact TableField key has no exact candidate")
	}
	r.AppendTableField(programflow.Field{Table: table, Key: key, Values: values, Kind: fieldKind})
	return nil
}

func (r *Rows) AdmitTableFill(counts [keyspace.FamilyCount]uint32, table keyspace.Term, fields []keyspace.Term) error {
	if r == nil || !countedFamily(counts, table, keyspace.FamilyTable) {
		return reject("invalid Table fill")
	}
	tableIndex := int(keyspace.TermOrdinal(table) - 1)
	if tableIndex < 0 || tableIndex >= len(r.tables.rows) || r.tables.filled[tableIndex] {
		return reject("Table filled twice")
	}
	for _, field := range fields {
		fieldRow, ok := r.TableFieldAt(int(keyspace.TermOrdinal(field) - 1))
		if !countedFamily(counts, field, keyspace.FamilyTableField) || !ok || fieldRow.Table != table {
			return reject("invalid Table field")
		}
	}
	fieldRange, ok := r.AppendTableOrder(fields)
	if !ok || !r.SetTableFields(tableIndex, fieldRange) || !r.SetTableFilled(tableIndex, true) {
		return reject("Table field range overflow")
	}
	return nil
}

func (r *Rows) AdmitFunction(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term) error {
	if r == nil || !body(counts, owner) || !denseTerm(term, keyspace.FamilyFunction, len(r.functions.rows)+1) {
		return reject("invalid Function owner")
	}
	r.AppendFunction(programflow.Function{Owner: owner})
	return nil
}

// AdmitFunctionFill closes one executable Function row and owns its capture
// range. The Source formal-order check is deliberately supplied as an
// immutable witness because Source remains the sole owner of Cell order.
func (r *Rows) AdmitFunctionFill(counts [keyspace.FamilyCount]uint32, function, bodyTerm, vararg keyspace.Term, formals []keyspace.Term, captures []programflow.Capture, sourceCellsAlreadyOrdered bool) error {
	if r == nil || !countedFamily(counts, function, keyspace.FamilyFunction) || !body(counts, bodyTerm) || function == bodyTerm || sourceCellsAlreadyOrdered {
		return reject("invalid Function fill")
	}
	functionIndex := int(keyspace.TermOrdinal(function) - 1)
	if functionIndex < 0 || functionIndex >= len(r.functions.rows) {
		return reject("invalid Function fill")
	}
	row := r.functions.rows[functionIndex]
	if row.Body != 0 {
		return reject("Function filled twice")
	}
	if vararg != 0 && !r.localCellInBody(counts, vararg, bodyTerm) {
		return reject("invalid Function vararg")
	}
	seenFormals := make(map[keyspace.Term]struct{}, len(formals))
	for _, formal := range formals {
		if !r.localCellInBody(counts, formal, bodyTerm) {
			return reject("invalid Function formal")
		}
		if _, duplicate := seenFormals[formal]; duplicate {
			return reject("duplicate Function formal Cell")
		}
		seenFormals[formal] = struct{}{}
	}
	for _, capture := range captures {
		outer, outerOK := r.CellAt(int(keyspace.TermOrdinal(capture.Outer) - 1))
		if !r.localCellInBody(counts, capture.Inner, bodyTerm) || !r.localCell(counts, capture.Outer) || !outerOK || outer.Body == bodyTerm {
			return reject("invalid Function capture")
		}
	}
	for index, capture := range captures {
		for _, previous := range captures[:index] {
			if previous.Inner == capture.Inner || previous.Outer == capture.Outer {
				return reject("duplicate Function capture")
			}
		}
	}
	captureRange, ok := r.AppendCaptures(captures)
	if !ok {
		return reject("Function capture range overflow")
	}
	row.Body, row.Vararg, row.Captures = bodyTerm, vararg, captureRange
	if !r.SetFunction(functionIndex, row) {
		return reject("invalid Function fill")
	}
	return nil
}

func (r *Rows) AdmitUnary(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term, op flowkind.UnaryOp, operand keyspace.Term) error {
	if r == nil || !body(counts, owner) || op < flowkind.UnaryNeg || op > flowkind.UnaryBitNot || !flowrole.ValueOccurrence(counts, operand) || !denseTerm(term, keyspace.FamilyUnary, len(r.operators.unaries)+1) {
		return reject("invalid Unary admission")
	}
	r.AppendUnary(programflow.Unary{Owner: owner, Op: op, Operand: operand})
	return nil
}

func (r *Rows) AdmitBinary(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term, op flowkind.BinaryOp, left, right keyspace.Term) error {
	if r == nil || !body(counts, owner) || op < flowkind.BinaryAdd || op > flowkind.BinaryGreaterEqual || !flowrole.ValueOccurrence(counts, left) || !flowrole.ValueOccurrence(counts, right) || !denseTerm(term, keyspace.FamilyBinary, len(r.operators.binaries)+1) {
		return reject("invalid Binary admission")
	}
	r.AppendBinary(programflow.Binary{Owner: owner, Op: op, Left: left, Right: right})
	return nil
}

func (r *Rows) AdmitSelect(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term, op flowkind.SelectOp, left, right keyspace.Term) error {
	if r == nil || !body(counts, owner) || (op != flowkind.SelectAnd && op != flowkind.SelectOr) || !flowrole.ValueOccurrence(counts, left) || !flowrole.ValueOccurrence(counts, right) || !denseTerm(term, keyspace.FamilySelect, len(r.operators.selects)+1) {
		return reject("invalid Select admission")
	}
	r.AppendSelect(programflow.Select{Owner: owner, Op: op, Left: left, Right: right})
	return nil
}

func (r *Rows) AdmitClaim(counts [keyspace.FamilyCount]uint32, term, owner keyspace.Term, claimKind flowkind.ValueClaimKind, operand, target keyspace.Term, allowMissing, targetValid bool) error {
	if r == nil || !body(counts, owner) || claimKind < flowkind.ValueClaimTypeAs || claimKind > flowkind.ValueClaimNonNil || !flowrole.ValueOccurrence(counts, operand) || !denseTerm(term, keyspace.FamilyValueClaim, len(r.operands.claims)+1) {
		return reject("invalid ValueClaim admission")
	}
	if claimKind == flowkind.ValueClaimNonNil && target != 0 {
		return reject("NonNil ValueClaim has target")
	}
	if claimKind != flowkind.ValueClaimNonNil && target == 0 && !allowMissing {
		return reject("ValueClaim target is missing")
	}
	if claimKind != flowkind.ValueClaimNonNil && target != 0 && !targetValid {
		return reject("invalid ValueClaim target")
	}
	r.AppendClaim(programflow.ValueClaim{Owner: owner, Operand: operand, Kind: claimKind})
	return nil
}

func (r *Rows) AdmitTypeValue(counts [keyspace.FamilyCount]uint32, term, owner, target keyspace.Term, targetValid bool) error {
	if r == nil || !body(counts, owner) || !targetValid || !denseTerm(term, keyspace.FamilyTypeValue, len(r.operands.typeValues)+1) {
		return reject("invalid TypeValue admission")
	}
	r.AppendTypeValue(programflow.TypeValue{Owner: owner})
	return nil
}

func rangeOK(poolLen, add int) bool {
	if poolLen < 0 || add < 0 {
		return false
	}
	return uint64(poolLen)+uint64(add) <= uint64(keyspace.MaxTermOrdinal)
}
