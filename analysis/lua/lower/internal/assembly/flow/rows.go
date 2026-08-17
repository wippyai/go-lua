// Package flow owns the mutable Flow construction rows for the Lua lowerer.
// It is a row owner, not a Collector facade: callers supply already-admitted
// typed rows and this package stores, copies, and freezes them. Cross-owner
// checks stay at the assembly orchestration boundary.
package flow

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

type valuesRows struct {
	rows  []programflow.Value
	terms []keyspace.Term
}

type accessRows struct {
	exact   []programflow.ExactLens
	dynamic []programflow.DynamicLens
}

type storageRows struct {
	cells        []programflow.Cell
	globalCensus bind.GlobalCensus
	reads        []programflow.Read
	varargs      []programflow.Vararg
	binds        []programflow.Bind
	assigns      []programflow.Assign
	writes       []programflow.Write
}

type tableRows struct {
	rows   []programflow.Table
	fields []programflow.Field
	order  []keyspace.Term
	filled []bool
}

type functionRows struct {
	rows     []programflow.Function
	captures []programflow.Capture
}

type controlRows struct {
	returns   []programflow.Return
	breaks    []programflow.Break
	labels    []programflow.Label
	gotos     []programflow.Goto
	branches  []programflow.Branch
	loops     []programflow.Loop
	loopCells []keyspace.Term
}

type operatorRows struct {
	unaries  []programflow.Unary
	binaries []programflow.Binary
	selects  []programflow.Select
}

type operandRows struct {
	claims     []programflow.ValueClaim
	typeValues []programflow.TypeValue
}

type callRows struct{ rows []programflow.Call }

// Rows is the complete Flow construction plane. Its row groups are private;
// a sibling owner can receive only an explicit copied row or an accessor.
type Rows struct {
	values    valuesRows
	access    accessRows
	storage   storageRows
	tables    tableRows
	functions functionRows
	calls     callRows
	control   controlRows
	operators operatorRows
	operands  operandRows
}

func (r *Rows) Reset() {
	if r != nil {
		*r = Rows{}
	}
}

func rangeFor(poolLen, add int) (programflow.Range, bool) {
	if poolLen < 0 || add < 0 {
		return programflow.Range{}, false
	}
	end := uint64(poolLen) + uint64(add)
	if end > uint64(keyspace.MaxTermOrdinal) {
		return programflow.Range{}, false
	}
	return programflow.Range{Start: uint32(poolLen), End: uint32(end)}, true
}

func cloneTerms(values []keyspace.Term) []keyspace.Term {
	return append([]keyspace.Term(nil), values...)
}

func (r *Rows) SetGlobalCensus(census bind.GlobalCensus) {
	if r != nil {
		r.storage.globalCensus = census
	}
}

func (r *Rows) GlobalCensus() bind.GlobalCensus {
	if r == nil {
		return bind.GlobalCensus{}
	}
	return r.storage.globalCensus
}

func (r *Rows) InitGlobalCells(count int) bool {
	if r == nil || count < 0 || uint64(count) > uint64(keyspace.MaxTermOrdinal) {
		return false
	}
	r.storage.cells = make([]programflow.Cell, count)
	return true
}

func (r *Rows) SetCell(index int, row programflow.Cell) bool {
	if r == nil || index < 0 || index >= len(r.storage.cells) {
		return false
	}
	r.storage.cells[index] = row
	return true
}

func (r *Rows) AppendCell(row programflow.Cell) {
	if r != nil {
		r.storage.cells = append(r.storage.cells, row)
	}
}

func (r *Rows) CellAt(index int) (programflow.Cell, bool) {
	if r == nil || index < 0 || index >= len(r.storage.cells) {
		return programflow.Cell{}, false
	}
	return r.storage.cells[index], true
}

func (r *Rows) AppendRead(row programflow.Read) {
	if r != nil {
		r.storage.reads = append(r.storage.reads, row)
	}
}

func (r *Rows) AppendVararg(row programflow.Vararg) {
	if r != nil {
		r.storage.varargs = append(r.storage.varargs, row)
	}
}

func (r *Rows) AppendBind(row programflow.Bind) {
	if r != nil {
		r.storage.binds = append(r.storage.binds, row)
	}
}

func (r *Rows) BindAt(index int) (programflow.Bind, bool) {
	if r == nil || index < 0 || index >= len(r.storage.binds) {
		return programflow.Bind{}, false
	}
	return r.storage.binds[index], true
}

func (r *Rows) AppendAssign(row programflow.Assign) {
	if r != nil {
		r.storage.assigns = append(r.storage.assigns, row)
	}
}

func (r *Rows) AppendWrite(row programflow.Write) {
	if r != nil {
		r.storage.writes = append(r.storage.writes, row)
	}
}

func (r *Rows) AppendValue(row programflow.Value, terms []keyspace.Term) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	span, ok := rangeFor(len(r.values.terms), len(terms))
	if !ok {
		return programflow.Range{}, false
	}
	r.values.terms = append(r.values.terms, terms...)
	row.Fixed = span
	r.values.rows = append(r.values.rows, row)
	return span, true
}

func (r *Rows) ValueAt(index int) (programflow.Value, bool) {
	if r == nil || index < 0 || index >= len(r.values.rows) {
		return programflow.Value{}, false
	}
	return r.values.rows[index], true
}

func (r *Rows) ValueTermAt(index int) (keyspace.Term, bool) {
	if r == nil || index < 0 || index >= len(r.values.terms) {
		return 0, false
	}
	return r.values.terms[index], true
}

func (r *Rows) AppendExactLens(row programflow.ExactLens) {
	if r != nil {
		r.access.exact = append(r.access.exact, row)
	}
}

func (r *Rows) AppendDynamicLens(row programflow.DynamicLens) {
	if r != nil {
		r.access.dynamic = append(r.access.dynamic, row)
	}
}

func (r *Rows) AppendTable(row programflow.Table) {
	if r != nil {
		r.tables.rows = append(r.tables.rows, row)
		r.tables.filled = append(r.tables.filled, false)
	}
}

func (r *Rows) SetTableFields(index int, fields programflow.Range) bool {
	if r == nil || index < 0 || index >= len(r.tables.rows) {
		return false
	}
	r.tables.rows[index].Fields = fields
	return true
}

func (r *Rows) SetTableFilled(index int, value bool) bool {
	if r == nil || index < 0 || index >= len(r.tables.filled) {
		return false
	}
	r.tables.filled[index] = value
	return true
}

func (r *Rows) AppendTableField(row programflow.Field) {
	if r != nil {
		r.tables.fields = append(r.tables.fields, row)
	}
}

func (r *Rows) TableFieldAt(index int) (programflow.Field, bool) {
	if r == nil || index < 0 || index >= len(r.tables.fields) {
		return programflow.Field{}, false
	}
	return r.tables.fields[index], true
}

func (r *Rows) AppendTableOrder(terms []keyspace.Term) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	rangeValue, ok := rangeFor(len(r.tables.order), len(terms))
	if !ok {
		return programflow.Range{}, false
	}
	r.tables.order = append(r.tables.order, terms...)
	return rangeValue, true
}

func (r *Rows) AppendFunction(row programflow.Function) {
	if r != nil {
		r.functions.rows = append(r.functions.rows, row)
	}
}

func (r *Rows) FunctionAt(index int) (programflow.Function, bool) {
	if r == nil || index < 0 || index >= len(r.functions.rows) {
		return programflow.Function{}, false
	}
	return r.functions.rows[index], true
}

func (r *Rows) SetFunction(index int, row programflow.Function) bool {
	if r == nil || index < 0 || index >= len(r.functions.rows) {
		return false
	}
	r.functions.rows[index] = row
	return true
}

func (r *Rows) AppendCaptures(captures []programflow.Capture) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	result, ok := rangeFor(len(r.functions.captures), len(captures))
	if !ok {
		return programflow.Range{}, false
	}
	r.functions.captures = append(r.functions.captures, captures...)
	return result, true
}

func (r *Rows) AppendCall(row programflow.Call) {
	if r != nil {
		r.calls.rows = append(r.calls.rows, row)
	}
}

// OwnerAt returns the Body owner carried by a direct Flow row. It is a
// read-only cross-owner witness used by Source orchestration; it never hands
// out a mutable row or a sibling store.
func (r *Rows) OwnerAt(family keyspace.Family, index int) (keyspace.Term, bool) {
	if r == nil || index < 0 {
		return 0, false
	}
	switch family {
	case keyspace.FamilyBind:
		if index < len(r.storage.binds) {
			return r.storage.binds[index].Owner, true
		}
	case keyspace.FamilyAssign:
		if index < len(r.storage.assigns) {
			return r.storage.assigns[index].Owner, true
		}
	case keyspace.FamilyCall:
		if index < len(r.calls.rows) {
			return r.calls.rows[index].Owner, true
		}
	case keyspace.FamilyBranch:
		if index < len(r.control.branches) {
			return r.control.branches[index].Owner, true
		}
	case keyspace.FamilyLoop:
		if index < len(r.control.loops) {
			return r.control.loops[index].Owner, true
		}
	case keyspace.FamilyReturn:
		if index < len(r.control.returns) {
			return r.control.returns[index].Owner, true
		}
	case keyspace.FamilyBreak:
		if index < len(r.control.breaks) {
			return r.control.breaks[index].Owner, true
		}
	case keyspace.FamilyGoto:
		if index < len(r.control.gotos) {
			return r.control.gotos[index].Owner, true
		}
	case keyspace.FamilyLabel:
		if index < len(r.control.labels) {
			return r.control.labels[index].Owner, true
		}
	}
	return 0, false
}

func (r *Rows) AppendReturn(row programflow.Return) {
	if r != nil {
		r.control.returns = append(r.control.returns, row)
	}
}

func (r *Rows) AppendBreak(row programflow.Break) {
	if r != nil {
		r.control.breaks = append(r.control.breaks, row)
	}
}

func (r *Rows) AppendLabel(row programflow.Label) {
	if r != nil {
		r.control.labels = append(r.control.labels, row)
	}
}

func (r *Rows) AppendGoto(row programflow.Goto) {
	if r != nil {
		r.control.gotos = append(r.control.gotos, row)
	}
}

func (r *Rows) AppendBranch(row programflow.Branch) {
	if r != nil {
		r.control.branches = append(r.control.branches, row)
	}
}

func (r *Rows) AppendLoop(row programflow.Loop, cells []keyspace.Term) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	result, ok := rangeFor(len(r.control.loopCells), len(cells))
	if !ok {
		return programflow.Range{}, false
	}
	r.control.loopCells = append(r.control.loopCells, cells...)
	row.Cells = result
	r.control.loops = append(r.control.loops, row)
	return result, true
}

func (r *Rows) AppendUnary(row programflow.Unary) {
	if r != nil {
		r.operators.unaries = append(r.operators.unaries, row)
	}
}

func (r *Rows) UnaryAt(index int) (programflow.Unary, bool) {
	if r == nil || index < 0 || index >= len(r.operators.unaries) {
		return programflow.Unary{}, false
	}
	return r.operators.unaries[index], true
}

func (r *Rows) AppendBinary(row programflow.Binary) {
	if r != nil {
		r.operators.binaries = append(r.operators.binaries, row)
	}
}

func (r *Rows) AppendSelect(row programflow.Select) {
	if r != nil {
		r.operators.selects = append(r.operators.selects, row)
	}
}

func (r *Rows) AppendClaim(row programflow.ValueClaim) {
	if r != nil {
		r.operands.claims = append(r.operands.claims, row)
	}
}

func (r *Rows) ClaimAt(index int) (programflow.ValueClaim, bool) {
	if r == nil || index < 0 || index >= len(r.operands.claims) {
		return programflow.ValueClaim{}, false
	}
	return r.operands.claims[index], true
}

func (r *Rows) AppendTypeValue(row programflow.TypeValue) {
	if r != nil {
		r.operands.typeValues = append(r.operands.typeValues, row)
	}
}

// Freeze copies the complete Flow rows and resolves only the binder-owned
// global Cell keys through the Source preimage supplied by assembly core.
func (r *Rows) Freeze(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) (programflow.Input, error) {
	if r == nil || !preimage.Identity().ContentID().Available() {
		return programflow.Input{}, errors.New("program/lower/collector: unavailable Source Preimage")
	}
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return programflow.Input{}, errors.New("program/lower/collector: invalid Flow count denominator")
	}
	if err := r.checkCounts(counts); err != nil {
		return programflow.Input{}, err
	}
	if err := r.resolveBreakTargets(preimage, counts); err != nil {
		return programflow.Input{}, err
	}
	input := programflow.Input{Counts: counts}
	input.Values = programflow.ValuesInput{Rows: append([]programflow.Value(nil), r.values.rows...), Terms: cloneTerms(r.values.terms)}
	input.Access = programflow.AccessInput{Exact: append([]programflow.ExactLens(nil), r.access.exact...), Dynamic: append([]programflow.DynamicLens(nil), r.access.dynamic...)}
	input.Storage = programflow.StorageInput{
		Cells: append([]programflow.Cell(nil), r.storage.cells...), Reads: append([]programflow.Read(nil), r.storage.reads...), Varargs: append([]programflow.Vararg(nil), r.storage.varargs...), Binds: append([]programflow.Bind(nil), r.storage.binds...), Assigns: append([]programflow.Assign(nil), r.storage.assigns...), Writes: append([]programflow.Write(nil), r.storage.writes...),
	}
	input.Tables = programflow.TablesInput{Rows: append([]programflow.Table(nil), r.tables.rows...), Fields: append([]programflow.Field(nil), r.tables.fields...), Order: cloneTerms(r.tables.order)}
	input.Functions = programflow.FunctionsInput{Rows: append([]programflow.Function(nil), r.functions.rows...), Captures: append([]programflow.Capture(nil), r.functions.captures...)}
	input.Calls = append([]programflow.Call(nil), r.calls.rows...)
	input.Control = programflow.ControlInput{Returns: append([]programflow.Return(nil), r.control.returns...), Breaks: append([]programflow.Break(nil), r.control.breaks...), Labels: append([]programflow.Label(nil), r.control.labels...), Gotos: append([]programflow.Goto(nil), r.control.gotos...), Branches: append([]programflow.Branch(nil), r.control.branches...), Loops: append([]programflow.Loop(nil), r.control.loops...), Cells: cloneTerms(r.control.loopCells)}
	input.Operators = programflow.OperatorsInput{Unaries: append([]programflow.Unary(nil), r.operators.unaries...), Binaries: append([]programflow.Binary(nil), r.operators.binaries...), Selects: append([]programflow.Select(nil), r.operators.selects...)}
	input.Claims = append([]programflow.ValueClaim(nil), r.operands.claims...)
	input.TypeValues = append([]programflow.TypeValue(nil), r.operands.typeValues...)
	keys := preimage.Keys()
	for index := 0; index < r.storage.globalCensus.Len(); index++ {
		row, ok := r.storage.globalCensus.At(index)
		if !ok || index >= len(input.Storage.Cells) || row.Slot() != uint32(index) || input.Storage.Cells[index].Kind != programflow.CellGlobal {
			return programflow.Input{}, fmt.Errorf("program/lower/collector: malformed global Cell prefix at %d", index+1)
		}
		key, ok := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: row.Name()})
		if !ok || key == 0 {
			return programflow.Input{}, fmt.Errorf("program/lower/collector: Source is missing global atom %q", row.Name())
		}
		input.Storage.Cells[index].Key = key
		input.Storage.Cells[index].Body = 0
	}
	for index := r.storage.globalCensus.Len(); index < len(input.Storage.Cells); index++ {
		if input.Storage.Cells[index].Kind == programflow.CellGlobal {
			return programflow.Input{}, fmt.Errorf("program/lower/collector: global Cell outside reserved census prefix at %d", index+1)
		}
	}
	return input, nil
}

// resolveBreakTargets is the lower assembly's one canonical owner/target
// boundary. Break rows retain their lexical Body owner, while the authored
// target is selected from the immutable Source Body nesting and the authored
// Loop→Body rows before Flow is built. No causal projection is consulted.
func (r *Rows) resolveBreakTargets(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) error {
	if r == nil {
		return errors.New("program/lower/collector: nil Flow rows")
	}
	bodyCount := int(counts[keyspace.FamilyBody])
	parents := make([]keyspace.Term, bodyCount+1)
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		length, ok := preimage.Order().BodyLen(body)
		if !ok {
			return fmt.Errorf("program/lower/collector: missing Source order for Body %v", body)
		}
		for index := 0; index < length; index++ {
			child, childOK := preimage.Order().BodyAt(body, index)
			if !childOK || keyspace.TermFamily(child) != keyspace.FamilyBody {
				continue
			}
			childOrdinal := keyspace.TermOrdinal(child)
			if childOrdinal == 0 || int(childOrdinal) > bodyCount || (parents[childOrdinal] != 0 && parents[childOrdinal] != body) {
				return fmt.Errorf("program/lower/collector: invalid Source Body parent for %v", child)
			}
			parents[childOrdinal] = body
		}
	}
	setParent := func(child, parent keyspace.Term) error {
		childOrdinal := keyspace.TermOrdinal(child)
		parentOrdinal := keyspace.TermOrdinal(parent)
		if keyspace.TermFamily(child) != keyspace.FamilyBody || childOrdinal == 0 || int(childOrdinal) > bodyCount ||
			keyspace.TermFamily(parent) != keyspace.FamilyBody || parentOrdinal == 0 || int(parentOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: invalid Body parent relation %v -> %v", parent, child)
		}
		if parents[childOrdinal] != 0 && parents[childOrdinal] != parent {
			return fmt.Errorf("program/lower/collector: conflicting Body parents for %v", child)
		}
		parents[childOrdinal] = parent
		return nil
	}
	for _, row := range r.control.branches {
		if err := setParent(row.WhenTrue, row.Owner); err != nil {
			return err
		}
		if err := setParent(row.WhenFalse, row.Owner); err != nil {
			return err
		}
	}

	loopByBody := make([]keyspace.Term, bodyCount+1)
	for index, row := range r.control.loops {
		bodyOrdinal := keyspace.TermOrdinal(row.Body)
		if bodyOrdinal == 0 || int(bodyOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: Loop %d has invalid Body", index+1)
		}
		if loopByBody[bodyOrdinal] != 0 {
			return fmt.Errorf("program/lower/collector: Body %v has duplicate Loop owners", row.Body)
		}
		loopByBody[bodyOrdinal] = keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		if err := setParent(row.Body, row.Owner); err != nil {
			return err
		}
	}
	functionBody := make([]bool, bodyCount+1)
	for _, row := range r.functions.rows {
		bodyOrdinal := keyspace.TermOrdinal(row.Body)
		if bodyOrdinal == 0 || int(bodyOrdinal) > bodyCount {
			return errors.New("program/lower/collector: Function has invalid Body")
		}
		functionBody[bodyOrdinal] = true
		if err := setParent(row.Body, row.Owner); err != nil {
			return err
		}
	}

	for index := range r.control.breaks {
		owner := r.control.breaks[index].Owner
		ownerOrdinal := keyspace.TermOrdinal(owner)
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || ownerOrdinal == 0 || int(ownerOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: Break %d has invalid Body owner", index+1)
		}
		current := owner
		target := keyspace.Term(0)
		for current != 0 {
			currentOrdinal := keyspace.TermOrdinal(current)
			if functionBody[currentOrdinal] {
				break
			}
			if loop := loopByBody[currentOrdinal]; loop != 0 {
				target = loop
				break
			}
			parent := parents[currentOrdinal]
			if parent == 0 {
				break
			}
			current = parent
		}
		if target == 0 {
			return fmt.Errorf("program/lower/collector: Break %d has no same-function Loop target", index+1)
		}
		if r.control.breaks[index].Target != 0 && r.control.breaks[index].Target != target {
			return fmt.Errorf("program/lower/collector: Break %d target disagrees with Body topology", index+1)
		}
		r.control.breaks[index].Target = target
	}
	return nil
}

func (r *Rows) checkCounts(counts [keyspace.FamilyCount]uint32) error {
	checks := []struct {
		family keyspace.Family
		got    int
		name   string
	}{
		{keyspace.FamilyValues, len(r.values.rows), "Values"}, {keyspace.FamilyLensExact, len(r.access.exact), "LensExact"}, {keyspace.FamilyLensKey, len(r.access.dynamic), "LensKey"}, {keyspace.FamilyCell, len(r.storage.cells), "Cell"}, {keyspace.FamilyRead, len(r.storage.reads), "Read"}, {keyspace.FamilyVararg, len(r.storage.varargs), "Vararg"}, {keyspace.FamilyBind, len(r.storage.binds), "Bind"}, {keyspace.FamilyAssign, len(r.storage.assigns), "Assign"}, {keyspace.FamilyWrite, len(r.storage.writes), "Write"}, {keyspace.FamilyTable, len(r.tables.rows), "Table"}, {keyspace.FamilyTableField, len(r.tables.fields), "TableField"}, {keyspace.FamilyFunction, len(r.functions.rows), "Function"}, {keyspace.FamilyCall, len(r.calls.rows), "Call"}, {keyspace.FamilyReturn, len(r.control.returns), "Return"}, {keyspace.FamilyBreak, len(r.control.breaks), "Break"}, {keyspace.FamilyLabel, len(r.control.labels), "Label"}, {keyspace.FamilyGoto, len(r.control.gotos), "Goto"}, {keyspace.FamilyBranch, len(r.control.branches), "Branch"}, {keyspace.FamilyLoop, len(r.control.loops), "Loop"}, {keyspace.FamilyUnary, len(r.operators.unaries), "Unary"}, {keyspace.FamilyBinary, len(r.operators.binaries), "Binary"}, {keyspace.FamilySelect, len(r.operators.selects), "Select"}, {keyspace.FamilyValueClaim, len(r.operands.claims), "ValueClaim"}, {keyspace.FamilyTypeValue, len(r.operands.typeValues), "TypeValue"},
	}
	for _, check := range checks {
		if check.got < 0 || uint64(check.got) > uint64(keyspace.MaxTermOrdinal) || uint64(check.got) != uint64(counts[check.family]) {
			return fmt.Errorf("program/lower/collector: %s denominator mismatch", check.name)
		}
	}
	globalCount := r.storage.globalCensus.Len()
	if globalCount > len(r.storage.cells) {
		return errors.New("program/lower/collector: global census rows exceed Cell rows")
	}
	for index := 0; index < globalCount; index++ {
		row, ok := r.storage.globalCensus.At(index)
		if !ok || row.Slot() != uint32(index) || row.Ordinal() != uint32(index+1) || r.storage.cells[index].Kind != programflow.CellGlobal {
			return fmt.Errorf("program/lower/collector: invalid global Cell prefix at %d", index+1)
		}
	}
	for index := globalCount; index < len(r.storage.cells); index++ {
		if r.storage.cells[index].Kind == programflow.CellGlobal {
			return fmt.Errorf("program/lower/collector: unreserved global Cell at %d", index+1)
		}
	}
	if len(r.values.rows) == 0 && len(r.values.terms) != 0 {
		return errors.New("program/lower/collector: orphan Values terms")
	}
	if len(r.tables.filled) != len(r.tables.rows) {
		return errors.New("program/lower/collector: Table fill denominator mismatch")
	}
	for index, filled := range r.tables.filled {
		if !filled {
			return fmt.Errorf("program/lower/collector: Table %d was not filled", index+1)
		}
	}
	if len(r.tables.fields) == 0 && len(r.tables.order) != 0 {
		return errors.New("program/lower/collector: orphan Table order")
	}
	if uint64(len(r.functions.captures)) > uint64(keyspace.MaxTermOrdinal) || uint64(len(r.control.loopCells)) > uint64(keyspace.MaxTermOrdinal) {
		return errors.New("program/lower/collector: dense pool overflow")
	}
	return nil
}
