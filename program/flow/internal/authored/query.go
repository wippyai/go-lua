package authored

import (
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Position is the complete Lua adjustment for one Values position. Exactly
// one of Fixed, Tail, and NilFill is populated for a valid position.
type Position struct {
	Fixed      keyspace.Term
	Tail       keyspace.Term
	TailOffset int
	NilFill    bool
}

func (view Values) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.values.rows)
}

func (view Values) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyValues, index, len(view.component.values.rows))
}

func (view Values) Get(term keyspace.Term) (owner, tail keyspace.Term, ok bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, 0, false
	}
	return row.Owner, row.Tail, true
}

func (view Values) Len(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	return int(row.Fixed.End - row.Fixed.Start), ok
}

func (view Values) Member(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok || index < 0 || uint64(index) >= uint64(row.Fixed.End-row.Fixed.Start) {
		return 0, false
	}
	return view.component.values.terms[row.Fixed.Start+uint32(index)], true
}

func (view Values) Position(term keyspace.Term, index int) (Position, bool) {
	if index < 0 {
		return Position{}, false
	}
	row, ok := view.row(term)
	if !ok {
		return Position{}, false
	}
	fixed := int(row.Fixed.End - row.Fixed.Start)
	if index < fixed {
		return Position{Fixed: view.component.values.terms[row.Fixed.Start+uint32(index)]}, true
	}
	if row.Tail != 0 {
		return Position{Tail: row.Tail, TailOffset: index - fixed}, true
	}
	return Position{NilFill: true}, true
}

func (view Values) row(term keyspace.Term) (Value, bool) {
	if !view.active() ||
		!keyspace.ValidTerm(term, keyspace.FamilyValues, len(view.component.values.rows)) {
		return Value{}, false
	}
	return view.component.values.rows[keyspace.TermOrdinal(term)-1], true
}

func (view Tables) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.tables.rows)
}

func (view Tables) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyTable, index, len(view.component.tables.rows))
}

func (view Tables) Get(term keyspace.Term) (owner keyspace.Term, ok bool) {
	row, ok := view.row(term)
	return row.Owner, ok
}

func (view Tables) FieldCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	return int(row.Fields.End - row.Fields.Start), ok
}

func (view Tables) FieldAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok || index < 0 || uint64(index) >= uint64(row.Fields.End-row.Fields.Start) {
		return 0, false
	}
	return view.component.tables.order[row.Fields.Start+uint32(index)], true
}

func (view Tables) row(term keyspace.Term) (Table, bool) {
	if !view.active() ||
		!keyspace.ValidTerm(term, keyspace.FamilyTable, len(view.component.tables.rows)) {
		return Table{}, false
	}
	return view.component.tables.rows[keyspace.TermOrdinal(term)-1], true
}

func (view Fields) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.tables.fields)
}

func (view Fields) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyTableField, index, len(view.component.tables.fields))
}

func (view Fields) Get(term keyspace.Term) (table, key, values keyspace.Term, fieldKind kind.FieldKind, ok bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.Table, row.Key, row.Values, row.Kind, true
}

func (view Fields) Values(term keyspace.Term) (values keyspace.Term, finalOpen bool, ok bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false, false
	}
	valueRow, valuesOK := Values{viewAccess: view.viewAccess}.row(row.Values)
	if !valuesOK {
		return 0, false, false
	}
	// Build proved this shape can occur only at the final field of its owning
	// table, so no order-pool scan belongs on this hot query path.
	finalOpen = row.Kind == kind.FieldList && valueRow.Fixed.Start == valueRow.Fixed.End && valueRow.Tail != 0
	return row.Values, finalOpen, true
}

func (view Fields) row(term keyspace.Term) (Field, bool) {
	if !view.active() ||
		!keyspace.ValidTerm(term, keyspace.FamilyTableField, len(view.component.tables.fields)) {
		return Field{}, false
	}
	return view.component.tables.fields[keyspace.TermOrdinal(term)-1], true
}

func (view Access) Exact() ExactLenses {
	return ExactLenses{viewAccess: view.viewAccess}
}
func (view Access) Dynamic() DynamicLenses {
	return DynamicLenses{viewAccess: view.viewAccess}
}

func (view ExactLenses) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.access.exact)
}

func (view ExactLenses) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyLensExact, index, len(view.component.access.exact))
}

func (view ExactLenses) Get(term keyspace.Term) (owner, base, source keyspace.Term, fieldKind kind.FieldKind, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyLensExact, len(view.component.access.exact)) {
		return 0, 0, 0, 0, false
	}
	row := view.component.access.exact[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Base, row.Source, row.Kind, true
}

func (view DynamicLenses) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.access.dynamic)
}

func (view DynamicLenses) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyLensKey, index, len(view.component.access.dynamic))
}

func (view DynamicLenses) Get(term keyspace.Term) (owner, base, key keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyLensKey, len(view.component.access.dynamic)) {
		return 0, 0, 0, false
	}
	row := view.component.access.dynamic[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Base, row.Key, true
}

func (view Storage) Cells() Cells {
	return Cells{viewAccess: view.viewAccess}
}
func (view Storage) Reads() Reads {
	return Reads{viewAccess: view.viewAccess}
}
func (view Storage) Varargs() Varargs {
	return Varargs{viewAccess: view.viewAccess}
}
func (view Storage) Binds() Binds {
	return Binds{viewAccess: view.viewAccess}
}
func (view Storage) Assigns() Assigns {
	return Assigns{viewAccess: view.viewAccess}
}
func (view Storage) Writes() Writes {
	return Writes{viewAccess: view.viewAccess}
}

func (view Cells) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.cells)
}

func (view Cells) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyCell, index, len(view.component.storage.cells))
}

func (view Cells) Get(term keyspace.Term) (kind CellKind, body keyspace.Term, key keyspace.Key, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyCell, len(view.component.storage.cells)) {
		return 0, 0, 0, false
	}
	row := view.component.storage.cells[keyspace.TermOrdinal(term)-1]
	return row.Kind, row.Body, row.Key, true
}

func (view Reads) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.reads)
}

func (view Reads) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyRead, index, len(view.component.storage.reads))
}

func (view Reads) Get(term keyspace.Term) (owner, source keyspace.Term, implicit, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyRead, len(view.component.storage.reads)) {
		return 0, 0, false, false
	}
	row := view.component.storage.reads[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Source, row.Implicit, true
}

// ImplicitCount and ImplicitAt expose the derived sparse implicit Read index.
func (view Reads) ImplicitCount() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.implicit)
}

func (view Reads) ImplicitAt(index int) (keyspace.Term, bool) {
	if !view.active() || index < 0 || index >= len(view.component.storage.implicit) {
		return 0, false
	}
	return view.component.storage.implicit[index], true
}

func (view Varargs) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.varargs)
}

func (view Varargs) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyVararg, index, len(view.component.storage.varargs))
}

func (view Varargs) Get(term keyspace.Term) (owner, cell keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyVararg, len(view.component.storage.varargs)) {
		return 0, 0, false
	}
	row := view.component.storage.varargs[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Cell, true
}

func (view Binds) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.binds)
}

func (view Binds) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyBind, index, len(view.component.storage.binds))
}

func (view Binds) Get(term keyspace.Term) (owner, values keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyBind, len(view.component.storage.binds)) {
		return 0, 0, false
	}
	row := view.component.storage.binds[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Values, true
}

func (view Assigns) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.assigns)
}

func (view Assigns) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyAssign, index, len(view.component.storage.assigns))
}

func (view Assigns) Get(term keyspace.Term) (owner, values keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyAssign, len(view.component.storage.assigns)) {
		return 0, 0, false
	}
	row := view.component.storage.assigns[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Values, true
}

func (view Assigns) WriteCount(term keyspace.Term) (int, bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyAssign, len(view.component.storage.assigns)) {
		return 0, false
	}
	span := view.component.storage.assignWrite[keyspace.TermOrdinal(term)-1]
	return int(span.End - span.Start), true
}

func (view Assigns) WriteAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.active() || index < 0 || !keyspace.ValidTerm(term, keyspace.FamilyAssign, len(view.component.storage.assigns)) {
		return 0, false
	}
	span := view.component.storage.assignWrite[keyspace.TermOrdinal(term)-1]
	if uint64(index) >= uint64(span.End-span.Start) {
		return 0, false
	}
	return termAt(keyspace.FamilyWrite, int(span.Start)+index, len(view.component.storage.writes))
}

func (view Writes) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.storage.writes)
}

func (view Writes) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyWrite, index, len(view.component.storage.writes))
}

func (view Writes) Get(term keyspace.Term) (assign, target keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyWrite, len(view.component.storage.writes)) {
		return 0, 0, false
	}
	row := view.component.storage.writes[keyspace.TermOrdinal(term)-1]
	return row.Assign, row.Target, true
}

func (view Functions) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.functions.rows)
}

func (view Functions) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyFunction, index, len(view.component.functions.rows))
}

func (view Functions) Get(term keyspace.Term) (owner, body, vararg keyspace.Term, ok bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, 0, 0, false
	}
	return row.Owner, row.Body, row.Vararg, true
}

func (view Functions) CaptureCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	return int(row.Captures.End - row.Captures.Start), ok
}

func (view Functions) CaptureAt(term keyspace.Term, index int) (inner, outer keyspace.Term, ok bool) {
	row, ok := view.row(term)
	if !ok || index < 0 || uint64(index) >= uint64(row.Captures.End-row.Captures.Start) {
		return 0, 0, false
	}
	capture := view.component.functions.captures[row.Captures.Start+uint32(index)]
	return capture.Inner, capture.Outer, true
}

func (view Functions) row(term keyspace.Term) (Function, bool) {
	if !view.active() ||
		!keyspace.ValidTerm(term, keyspace.FamilyFunction, len(view.component.functions.rows)) {
		return Function{}, false
	}
	return view.component.functions.rows[keyspace.TermOrdinal(term)-1], true
}

func (view Calls) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.calls.rows)
}

func (view Calls) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyCall, index, len(view.component.calls.rows))
}

func (view Calls) Get(term keyspace.Term) (owner, callee, receiver, actuals keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyCall, len(view.component.calls.rows)) {
		return 0, 0, 0, 0, false
	}
	row := view.component.calls.rows[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Callee, row.Receiver, row.Actuals, true
}

// Returns through Loops expose authored control evidence only. They never
// expose Source-owned order or derived execution relations.
func (view Control) Returns() Returns {
	return Returns{viewAccess: view.viewAccess}
}
func (view Control) Breaks() Breaks {
	return Breaks{viewAccess: view.viewAccess}
}
func (view Control) Labels() Labels {
	return Labels{viewAccess: view.viewAccess}
}
func (view Control) Gotos() Gotos {
	return Gotos{viewAccess: view.viewAccess}
}
func (view Control) Branches() Branches {
	return Branches{viewAccess: view.viewAccess}
}
func (view Control) Loops() Loops {
	return Loops{viewAccess: view.viewAccess}
}

func (view Returns) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.authoredControl.returns)
}
func (view Returns) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyReturn, index, len(view.component.authoredControl.returns))
}
func (view Returns) Get(term keyspace.Term) (owner, values keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyReturn, len(view.component.authoredControl.returns)) {
		return 0, 0, false
	}
	row := view.component.authoredControl.returns[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Values, true
}

func (view Breaks) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.authoredControl.breaks)
}
func (view Breaks) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyBreak, index, len(view.component.authoredControl.breaks))
}
func (view Breaks) Get(term keyspace.Term) (owner keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyBreak, len(view.component.authoredControl.breaks)) {
		return 0, false
	}
	return view.component.authoredControl.breaks[keyspace.TermOrdinal(term)-1].Owner, true
}

func (view Labels) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.authoredControl.labels)
}
func (view Labels) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyLabel, index, len(view.component.authoredControl.labels))
}
func (view Labels) Get(term keyspace.Term) (owner keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyLabel, len(view.component.authoredControl.labels)) {
		return 0, false
	}
	return view.component.authoredControl.labels[keyspace.TermOrdinal(term)-1].Owner, true
}

func (view Gotos) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.authoredControl.gotos)
}
func (view Gotos) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyGoto, index, len(view.component.authoredControl.gotos))
}
func (view Gotos) Get(term keyspace.Term) (owner, target keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyGoto, len(view.component.authoredControl.gotos)) {
		return 0, 0, false
	}
	row := view.component.authoredControl.gotos[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Target, true
}

func (view Branches) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.authoredControl.branches)
}
func (view Branches) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyBranch, index, len(view.component.authoredControl.branches))
}
func (view Branches) Get(term keyspace.Term) (owner, condition, whenTrue, whenFalse keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyBranch, len(view.component.authoredControl.branches)) {
		return 0, 0, 0, 0, false
	}
	row := view.component.authoredControl.branches[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Condition, row.WhenTrue, row.WhenFalse, true
}

func (view Loops) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.authoredControl.loops)
}
func (view Loops) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyLoop, index, len(view.component.authoredControl.loops))
}
func (view Loops) Get(term keyspace.Term) (owner, body keyspace.Term, loopKind kind.LoopKind, control keyspace.Term, ok bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.Owner, row.Body, row.Kind, row.Control, true
}
func (view Loops) CellCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	return int(row.Cells.End - row.Cells.Start), ok
}
func (view Loops) CellAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok || index < 0 || uint64(index) >= uint64(row.Cells.End-row.Cells.Start) {
		return 0, false
	}
	return view.component.authoredControl.cells[row.Cells.Start+uint32(index)], true
}
func (view Loops) row(term keyspace.Term) (Loop, bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyLoop, len(view.component.authoredControl.loops)) {
		return Loop{}, false
	}
	return view.component.authoredControl.loops[keyspace.TermOrdinal(term)-1], true
}
