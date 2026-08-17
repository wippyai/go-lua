package authored

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Build admits authored Flow structure only. Causal projections await the
// later whole-Flow finalizer and cannot affect source identity here.
func Build(input Input) (*Draft, error) {
	if !validCounts(input.Counts) ||
		!countEquals(input.Counts[keyspace.FamilyValues], len(input.Values.Rows)) ||
		!countEquals(input.Counts[keyspace.FamilyLensExact], len(input.Access.Exact)) ||
		!countEquals(input.Counts[keyspace.FamilyLensKey], len(input.Access.Dynamic)) ||
		!countEquals(input.Counts[keyspace.FamilyCell], len(input.Storage.Cells)) ||
		!countEquals(input.Counts[keyspace.FamilyRead], len(input.Storage.Reads)) ||
		!countEquals(input.Counts[keyspace.FamilyVararg], len(input.Storage.Varargs)) ||
		!countEquals(input.Counts[keyspace.FamilyBind], len(input.Storage.Binds)) ||
		!countEquals(input.Counts[keyspace.FamilyAssign], len(input.Storage.Assigns)) ||
		!countEquals(input.Counts[keyspace.FamilyWrite], len(input.Storage.Writes)) ||
		!countEquals(input.Counts[keyspace.FamilyTable], len(input.Tables.Rows)) ||
		!countEquals(input.Counts[keyspace.FamilyTableField], len(input.Tables.Fields)) ||
		!countEquals(input.Counts[keyspace.FamilyUnary], len(input.Operators.Unaries)) ||
		!countEquals(input.Counts[keyspace.FamilyBinary], len(input.Operators.Binaries)) ||
		!countEquals(input.Counts[keyspace.FamilySelect], len(input.Operators.Selects)) ||
		!countEquals(input.Counts[keyspace.FamilyFunction], len(input.Functions.Rows)) ||
		!countEquals(input.Counts[keyspace.FamilyCall], len(input.Calls)) ||
		!countEquals(input.Counts[keyspace.FamilyReturn], len(input.Control.Returns)) ||
		!countEquals(input.Counts[keyspace.FamilyBreak], len(input.Control.Breaks)) ||
		!countEquals(input.Counts[keyspace.FamilyLabel], len(input.Control.Labels)) ||
		!countEquals(input.Counts[keyspace.FamilyGoto], len(input.Control.Gotos)) ||
		!countEquals(input.Counts[keyspace.FamilyBranch], len(input.Control.Branches)) ||
		!countEquals(input.Counts[keyspace.FamilyLoop], len(input.Control.Loops)) ||
		!countEquals(input.Counts[keyspace.FamilyValueClaim], len(input.Claims)) ||
		!countEquals(input.Counts[keyspace.FamilyTypeValue], len(input.TypeValues)) ||
		!lengthFits(len(input.Values.Terms)) || !lengthFits(len(input.Tables.Order)) ||
		!lengthFits(len(input.Functions.Captures)) || !lengthFits(len(input.Control.Cells)) {
		return nil, errors.New("program/flow: inconsistent authored cardinality")
	}
	component := &component{
		values: valueStore{
			rows:  append([]Value(nil), input.Values.Rows...),
			terms: append([]keyspace.Term(nil), input.Values.Terms...),
		},
		access: accessStore{
			exact:   append([]ExactLens(nil), input.Access.Exact...),
			dynamic: append([]DynamicLens(nil), input.Access.Dynamic...),
		},
		storage: storageStore{
			cells:   append([]Cell(nil), input.Storage.Cells...),
			reads:   append([]Read(nil), input.Storage.Reads...),
			varargs: append([]Vararg(nil), input.Storage.Varargs...),
			binds:   append([]Bind(nil), input.Storage.Binds...),
			assigns: append([]Assign(nil), input.Storage.Assigns...),
			writes:  append([]Write(nil), input.Storage.Writes...),
		},
		tables: tableStore{
			rows:   append([]Table(nil), input.Tables.Rows...),
			fields: append([]Field(nil), input.Tables.Fields...),
			order:  append([]keyspace.Term(nil), input.Tables.Order...),
		},
		functions: functionStore{
			rows:     append([]Function(nil), input.Functions.Rows...),
			captures: append([]Capture(nil), input.Functions.Captures...),
		},
		calls: callStore{rows: append([]Call(nil), input.Calls...)},
		operators: operatorStore{
			unaries:  append([]Unary(nil), input.Operators.Unaries...),
			binaries: append([]Binary(nil), input.Operators.Binaries...),
			selects:  append([]Select(nil), input.Operators.Selects...),
		},
		claims: claimStore{
			claims:     append([]ValueClaim(nil), input.Claims...),
			typeValues: append([]TypeValue(nil), input.TypeValues...),
		},
		authoredControl: authoredControlStore{
			returns:  append([]Return(nil), input.Control.Returns...),
			breaks:   normalizeBreakTargets(input.Control.Breaks, input.Control.Loops),
			labels:   append([]Label(nil), input.Control.Labels...),
			gotos:    append([]Goto(nil), input.Control.Gotos...),
			branches: append([]Branch(nil), input.Control.Branches...),
			loops:    append([]Loop(nil), input.Control.Loops...),
			cells:    append([]keyspace.Term(nil), input.Control.Cells...),
		},
	}
	if err := validateAuthored(component, input.Counts); err != nil {
		return nil, err
	}
	component.contentID = contentID(component)
	if !component.contentID.Available() {
		return nil, errors.New("program/flow: unavailable content identity")
	}
	return &Draft{state: &draftState{component: component, phase: draftOpen}}, nil
}

// normalizeBreakTargets fills the unambiguous direct Loop target for authored
// inputs that provide only the lexical Body owner. Lowered Lua rows are
// resolved against the complete Source Body tree during assembly Freeze; this
// construction normalization keeps the row boundary explicit for direct
// authored inputs without retaining another index.
func normalizeBreakTargets(rows []Break, loops []Loop) []Break {
	breaks := append([]Break(nil), rows...)
	for index := range breaks {
		if breaks[index].Target != 0 {
			continue
		}
		for loopIndex, loop := range loops {
			if loop.Body == breaks[index].Owner {
				breaks[index].Target = keyspace.MakeTerm(keyspace.FamilyLoop, uint32(loopIndex+1))
				break
			}
		}
	}
	return breaks
}

// Finalizer claims the authored Draft exactly once. The returned capability
// exposes only lifecycle-bound typed views until its terminal Commit or Abort.
func (draft *Draft) Finalizer() (Finalizer, error) {
	if draft == nil || draft.state == nil {
		return Finalizer{}, errors.New("program/flow/authored: invalid draft finalizer")
	}
	state := draft.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftOpen || state.component == nil || !state.component.contentID.Available() {
		return Finalizer{}, errors.New("program/flow/authored: finalizer already claimed")
	}
	state.phase = draftClaimed
	return Finalizer{state: state}, nil
}

// View returns the lifecycle-bound authored typed surface while this
// Finalizer is claimed. Captured views expire after Commit or Abort.
func (finalizer Finalizer) View() View {
	if finalizer.state == nil {
		return View{}
	}
	state := finalizer.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftClaimed || state.component == nil {
		return View{}
	}
	return View{viewAccess: viewAccess{component: state.component, state: state}}
}

// Commit terminally consumes the claimed authored Draft and returns a direct
// immutable View. The returned View has no lifecycle fence and remains valid
// after this Finalizer and every copied Finalizer reach their terminal state.
func (finalizer Finalizer) Commit() (View, error) {
	if finalizer.state == nil {
		return View{}, errors.New("program/flow/authored: invalid finalizer commit")
	}
	state := finalizer.state
	state.mu.Lock()
	if state.phase != draftClaimed || state.component == nil {
		state.mu.Unlock()
		return View{}, errors.New("program/flow/authored: finalizer is terminal")
	}
	component := state.component
	state.component = nil
	state.phase = draftCommitted
	state.mu.Unlock()
	return View{viewAccess: viewAccess{component: component}}, nil
}

// Abort terminally discards the claimed authored Draft without publishing a
// View. A later copied Finalizer cannot reopen or consume the Draft.
func (finalizer Finalizer) Abort() error {
	if finalizer.state == nil {
		return errors.New("program/flow/authored: invalid finalizer abort")
	}
	state := finalizer.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftClaimed || state.component == nil {
		return errors.New("program/flow/authored: finalizer is terminal")
	}
	state.component = nil
	state.phase = draftAborted
	return nil
}

func validCounts(counts [keyspace.FamilyCount]uint32) bool {
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return false
	}
	for _, count := range counts {
		if count > keyspace.MaxTermOrdinal {
			return false
		}
	}
	return true
}

func lengthFits(length int) bool {
	return length >= 0 && uint64(length) <= uint64(keyspace.MaxTermOrdinal)
}

func countEquals(count uint32, length int) bool {
	return lengthFits(length) && uint64(count) == uint64(length)
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}

func validateAuthored(component *component, counts [keyspace.FamilyCount]uint32) error {
	if err := validateOperators(component, counts); err != nil {
		return err
	}
	if err := validateAccessStorage(component, counts); err != nil {
		return err
	}
	if err := validateFunctionsCalls(component, counts); err != nil {
		return err
	}
	if err := validateAuthoredControl(component, counts); err != nil {
		return err
	}
	if err := validateClaims(component, counts); err != nil {
		return err
	}
	if err := validateChildBodies(component, counts); err != nil {
		return err
	}
	for _, term := range component.values.terms {
		if !flowrole.ValueOccurrence(counts, term) {
			return errors.New("program/flow: invalid Values member")
		}
	}
	valueCursor := uint32(0)
	for _, row := range component.values.rows {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			row.Fixed.Start != valueCursor || row.Fixed.Start > row.Fixed.End ||
			uint64(row.Fixed.End) > uint64(len(component.values.terms)) ||
			(row.Tail != 0 && !flowrole.OpenOccurrence(counts, row.Tail)) {
			return errors.New("program/flow: invalid Values row")
		}
		valueCursor = row.Fixed.End
	}
	if uint64(valueCursor) != uint64(len(component.values.terms)) {
		return errors.New("program/flow: Values ranges do not partition members")
	}

	tableSlots := make([]bool, len(component.tables.order))
	fieldSeen := make([]bool, len(component.tables.fields))
	for _, row := range component.tables.rows {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			row.Fields.Start > row.Fields.End ||
			uint64(row.Fields.End) > uint64(len(component.tables.order)) {
			return errors.New("program/flow: invalid Table row")
		}
		for at := row.Fields.Start; at < row.Fields.End; at++ {
			if tableSlots[at] {
				return errors.New("program/flow: overlapping Table field ranges")
			}
			tableSlots[at] = true
		}
	}
	for index, row := range component.tables.fields {
		if !fieldAuthored(component, counts, row) || !ordinalFits(index, len(component.tables.fields)) {
			return errors.New("program/flow: invalid TableField row")
		}
	}
	for _, field := range component.tables.order {
		if !hasFamily(counts, field, keyspace.FamilyTableField) {
			return errors.New("program/flow: invalid TableField order")
		}
	}
	for tableIndex, row := range component.tables.rows {
		table, ok := termAt(keyspace.FamilyTable, tableIndex, len(component.tables.rows))
		if !ok {
			return errors.New("program/flow: unrepresentable Table ordinal")
		}
		for at := row.Fields.Start; at < row.Fields.End; at++ {
			field := component.tables.order[at]
			ordinal := keyspace.TermOrdinal(field)
			if ordinal == 0 || uint64(ordinal) > uint64(len(component.tables.fields)) ||
				component.tables.fields[ordinal-1].Table != table || fieldSeen[ordinal-1] {
				return errors.New("program/flow: TableField order crosses Table owner")
			}
			fieldSeen[ordinal-1] = true
		}
	}
	for _, used := range tableSlots {
		if !used {
			return errors.New("program/flow: Table ranges do not partition field order")
		}
	}
	for _, used := range fieldSeen {
		if !used {
			return errors.New("program/flow: orphan TableField")
		}
	}
	if err := validateTableValueAdjustment(component); err != nil {
		return err
	}
	return nil
}

func validateOperators(component *component, counts [keyspace.FamilyCount]uint32) error {
	for _, row := range component.operators.unaries {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !validUnaryOp(row.Op) || !flowrole.ValueOccurrence(counts, row.Operand) {
			return errors.New("program/flow: invalid Unary row")
		}
	}
	for _, row := range component.operators.binaries {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !validBinaryOp(row.Op) ||
			!flowrole.ValueOccurrence(counts, row.Left) || !flowrole.ValueOccurrence(counts, row.Right) {
			return errors.New("program/flow: invalid Binary row")
		}
	}
	for _, row := range component.operators.selects {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !validSelectOp(row.Op) ||
			!flowrole.ValueOccurrence(counts, row.Left) || !flowrole.ValueOccurrence(counts, row.Right) {
			return errors.New("program/flow: invalid Select row")
		}
	}
	return nil
}

func validateClaims(component *component, counts [keyspace.FamilyCount]uint32) error {
	for _, row := range component.claims.claims {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !validValueClaimKind(row.Kind) ||
			!flowrole.ValueOccurrence(counts, row.Operand) {
			return errors.New("program/flow: invalid ValueClaim row")
		}
	}
	for _, row := range component.claims.typeValues {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) {
			return errors.New("program/flow: invalid TypeValue row")
		}
	}
	return nil
}

func validateFunctionsCalls(component *component, counts [keyspace.FamilyCount]uint32) error {
	innerSeen := make(map[keyspace.Term]struct{}, len(component.functions.captures))
	captureCursor := uint32(0)
	for _, row := range component.functions.rows {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			!hasFamily(counts, row.Body, keyspace.FamilyBody) || row.Owner == row.Body ||
			row.Captures.Start != captureCursor || row.Captures.Start > row.Captures.End ||
			uint64(row.Captures.End) > uint64(len(component.functions.captures)) {
			return errors.New("program/flow: invalid Function row")
		}
		if row.Vararg != 0 && !localCellInBody(counts, component.storage.cells, row.Vararg, row.Body) {
			return errors.New("program/flow: invalid Function Vararg")
		}
		outerSeen := make(map[keyspace.Term]struct{}, row.Captures.End-row.Captures.Start)
		for at := row.Captures.Start; at < row.Captures.End; at++ {
			capture := component.functions.captures[at]
			if !localCellInBody(counts, component.storage.cells, capture.Inner, row.Body) ||
				!localCell(counts, component.storage.cells, capture.Outer) ||
				component.storage.cells[keyspace.TermOrdinal(capture.Outer)-1].Body == row.Body {
				return errors.New("program/flow: invalid Function Capture")
			}
			if _, duplicate := innerSeen[capture.Inner]; duplicate {
				return errors.New("program/flow: duplicate Capture Inner")
			}
			if _, duplicate := outerSeen[capture.Outer]; duplicate {
				return errors.New("program/flow: duplicate Function Capture Outer")
			}
			innerSeen[capture.Inner] = struct{}{}
			outerSeen[capture.Outer] = struct{}{}
		}
		captureCursor = row.Captures.End
	}
	if uint64(captureCursor) != uint64(len(component.functions.captures)) {
		return errors.New("program/flow: Function Capture ranges do not partition captures")
	}
	for _, row := range component.calls.rows {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			!flowrole.ValueOccurrence(counts, row.Callee) ||
			!hasFamily(counts, row.Actuals, keyspace.FamilyValues) ||
			(row.Receiver != 0 && !methodCallee(counts, component, row)) {
			return errors.New("program/flow: invalid Call row")
		}
	}
	return nil
}

func validateAuthoredControl(component *component, counts [keyspace.FamilyCount]uint32) error {
	for _, row := range component.authoredControl.returns {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !hasFamily(counts, row.Values, keyspace.FamilyValues) {
			return errors.New("program/flow: invalid Return row")
		}
	}
	for _, row := range component.authoredControl.breaks {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			(row.Target != 0 && !hasFamily(counts, row.Target, keyspace.FamilyLoop)) {
			return errors.New("program/flow: invalid Break row")
		}
	}
	for _, row := range component.authoredControl.labels {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) {
			return errors.New("program/flow: invalid Label row")
		}
	}
	for _, row := range component.authoredControl.gotos {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !hasFamily(counts, row.Target, keyspace.FamilyLabel) {
			return errors.New("program/flow: invalid Goto row")
		}
	}
	for _, row := range component.authoredControl.branches {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !flowrole.ValueOccurrence(counts, row.Condition) ||
			!hasFamily(counts, row.WhenTrue, keyspace.FamilyBody) || !hasFamily(counts, row.WhenFalse, keyspace.FamilyBody) ||
			row.Owner == row.WhenTrue || row.Owner == row.WhenFalse || row.WhenTrue == row.WhenFalse {
			return errors.New("program/flow: invalid Branch row")
		}
	}
	cursor := uint32(0)
	for _, row := range component.authoredControl.loops {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !hasFamily(counts, row.Body, keyspace.FamilyBody) ||
			row.Owner == row.Body || !validLoopKind(row.Kind) || row.Cells.Start != cursor || row.Cells.Start > row.Cells.End ||
			uint64(row.Cells.End) > uint64(len(component.authoredControl.cells)) {
			return errors.New("program/flow: invalid Loop row")
		}
		cellCount := row.Cells.End - row.Cells.Start
		switch row.Kind {
		case kind.LoopWhile, kind.LoopRepeat:
			if cellCount != 0 || !flowrole.LoopControlFamily(counts, row.Control, row.Kind) {
				return errors.New("program/flow: invalid scalar Loop")
			}
		case kind.LoopNumericFor:
			if cellCount != 1 || !numericForControl(component, counts, row.Control) {
				return errors.New("program/flow: invalid numeric Loop")
			}
		case kind.LoopGenericFor:
			if cellCount == 0 || !genericForControl(component, counts, row.Control) {
				return errors.New("program/flow: invalid generic Loop")
			}
		}
		seen := make(map[keyspace.Term]struct{}, cellCount)
		for at := row.Cells.Start; at < row.Cells.End; at++ {
			cell := component.authoredControl.cells[at]
			if !localCellInBody(counts, component.storage.cells, cell, row.Body) {
				return errors.New("program/flow: invalid Loop Cell")
			}
			if _, duplicate := seen[cell]; duplicate {
				return errors.New("program/flow: duplicate Loop Cell")
			}
			seen[cell] = struct{}{}
		}
		cursor = row.Cells.End
	}
	if uint64(cursor) != uint64(len(component.authoredControl.cells)) {
		return errors.New("program/flow: Loop Cell ranges do not partition cells")
	}
	return nil
}

func numericForControl(component *component, counts [keyspace.FamilyCount]uint32, control keyspace.Term) bool {
	if !flowrole.LoopControlFamily(counts, control, kind.LoopNumericFor) {
		return false
	}
	values := component.values.rows[keyspace.TermOrdinal(control)-1]
	width := values.Fixed.End - values.Fixed.Start
	return (width == 2 || width == 3) && values.Tail == 0
}

func genericForControl(component *component, counts [keyspace.FamilyCount]uint32, control keyspace.Term) bool {
	if !flowrole.LoopControlFamily(counts, control, kind.LoopGenericFor) {
		return false
	}
	values := component.values.rows[keyspace.TermOrdinal(control)-1]
	return values.Fixed.Start != values.Fixed.End || values.Tail != 0
}

// Child executable Bodies are unique across the authored constructs that
// declare one. Source later validates their lexical parent forest and order.
func validateChildBodies(component *component, counts [keyspace.FamilyCount]uint32) error {
	children := make(map[keyspace.Term]struct{}, len(component.functions.rows)+len(component.authoredControl.branches)*2+len(component.authoredControl.loops))
	claim := func(body keyspace.Term) bool {
		if !hasFamily(counts, body, keyspace.FamilyBody) {
			return false
		}
		if _, duplicate := children[body]; duplicate {
			return false
		}
		children[body] = struct{}{}
		return true
	}
	for _, row := range component.functions.rows {
		if !claim(row.Body) {
			return errors.New("program/flow: duplicate child Body")
		}
	}
	for _, row := range component.authoredControl.branches {
		if !claim(row.WhenTrue) || !claim(row.WhenFalse) {
			return errors.New("program/flow: duplicate child Body")
		}
	}
	for _, row := range component.authoredControl.loops {
		if !claim(row.Body) {
			return errors.New("program/flow: duplicate child Body")
		}
	}
	return nil
}

func fieldAuthored(component *component, counts [keyspace.FamilyCount]uint32, row Field) bool {
	if !hasFamily(counts, row.Table, keyspace.FamilyTable) ||
		!hasFamily(counts, row.Values, keyspace.FamilyValues) || !validFieldKind(row.Kind) {
		return false
	}
	switch row.Kind {
	case kind.FieldList, kind.FieldName:
		return flowrole.FieldSourceFamily(counts, row.Key, row.Kind)
	case kind.FieldExact:
		return flowrole.FieldSourceFamily(counts, row.Key, row.Kind) && staticExactCandidate(component, counts, row.Key)
	case kind.FieldKey:
		return flowrole.FieldSourceFamily(counts, row.Key, row.Kind)
	default:
		return false
	}
}

func validateAccessStorage(component *component, counts [keyspace.FamilyCount]uint32) error {
	for _, row := range component.access.exact {
		if !exactLensAuthored(component, counts, row) {
			return errors.New("program/flow: invalid ExactLens row")
		}
	}
	for _, row := range component.access.dynamic {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			!flowrole.ValueOccurrence(counts, row.Base) || !flowrole.ValueOccurrence(counts, row.Key) {
			return errors.New("program/flow: invalid DynamicLens row")
		}
	}
	globals := make(map[keyspace.Key]struct{}, len(component.storage.cells))
	for _, row := range component.storage.cells {
		if !cellAuthored(counts, row) {
			return errors.New("program/flow: invalid Cell row")
		}
		if row.Kind == CellGlobal {
			if _, duplicate := globals[row.Key]; duplicate {
				return errors.New("program/flow: duplicate global Cell")
			}
			globals[row.Key] = struct{}{}
		}
	}
	for _, row := range component.storage.reads {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !flowrole.Addressable(counts, row.Source) {
			return errors.New("program/flow: invalid Read row")
		}
		if row.Implicit && !globalCell(counts, component.storage.cells, row.Source) {
			return errors.New("program/flow: implicit Read is not global")
		}
	}
	for index, row := range component.storage.varargs {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) ||
			!localCell(counts, component.storage.cells, row.Cell) || !ordinalFits(index, len(component.storage.varargs)) {
			return errors.New("program/flow: invalid Vararg row")
		}
	}
	for _, row := range component.storage.binds {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !hasFamily(counts, row.Values, keyspace.FamilyValues) {
			return errors.New("program/flow: invalid Bind row")
		}
	}
	for _, row := range component.storage.assigns {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !hasFamily(counts, row.Values, keyspace.FamilyValues) {
			return errors.New("program/flow: invalid Assign row")
		}
	}
	if !deriveAssignWrites(component, counts) {
		return errors.New("program/flow: invalid Assign Write ranges")
	}
	component.storage.implicit = component.storage.implicit[:0]
	for index, row := range component.storage.reads {
		if row.Implicit {
			term, ok := termAt(keyspace.FamilyRead, index, len(component.storage.reads))
			if !ok {
				return errors.New("program/flow: unrepresentable implicit Read")
			}
			component.storage.implicit = append(component.storage.implicit, term)
		}
	}
	return nil
}

func exactLensAuthored(component *component, counts [keyspace.FamilyCount]uint32, row ExactLens) bool {
	if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !flowrole.ValueOccurrence(counts, row.Base) {
		return false
	}
	switch row.Kind {
	case kind.FieldName:
		return flowrole.FieldSourceFamily(counts, row.Source, row.Kind)
	case kind.FieldExact:
		return flowrole.FieldSourceFamily(counts, row.Source, row.Kind) && staticExactCandidate(component, counts, row.Source)
	default:
		return false
	}
}

func cellAuthored(counts [keyspace.FamilyCount]uint32, row Cell) bool {
	if !row.Kind.valid() {
		return false
	}
	switch row.Kind {
	case CellLocal:
		return hasFamily(counts, row.Body, keyspace.FamilyBody) && row.Key == 0
	case CellGlobal:
		return row.Body == 0 && row.Key != 0
	default:
		return false
	}
}

func localCell(counts [keyspace.FamilyCount]uint32, rows []Cell, cell keyspace.Term) bool {
	return hasFamily(counts, cell, keyspace.FamilyCell) &&
		rows[keyspace.TermOrdinal(cell)-1].Kind == CellLocal
}

func localCellInBody(counts [keyspace.FamilyCount]uint32, rows []Cell, cell, body keyspace.Term) bool {
	return localCell(counts, rows, cell) && rows[keyspace.TermOrdinal(cell)-1].Body == body
}

func globalCell(counts [keyspace.FamilyCount]uint32, rows []Cell, cell keyspace.Term) bool {
	return hasFamily(counts, cell, keyspace.FamilyCell) &&
		rows[keyspace.TermOrdinal(cell)-1].Kind == CellGlobal
}

// methodCallee proves only Flow-local method-call syntax. Source later proves
// that the named exact lens has a valid source Name key; lexical visibility and
// call/control roles likewise remain for whole-Flow sealing.
func methodCallee(counts [keyspace.FamilyCount]uint32, component *component, call Call) bool {
	if !flowrole.ValueOccurrence(counts, call.Receiver) ||
		!hasFamily(counts, call.Callee, keyspace.FamilyRead) {
		return false
	}
	read := component.storage.reads[keyspace.TermOrdinal(call.Callee)-1]
	if read.Owner != call.Owner || !hasFamily(counts, read.Source, keyspace.FamilyLensExact) {
		return false
	}
	lens := component.access.exact[keyspace.TermOrdinal(read.Source)-1]
	return lens.Owner == call.Owner && lens.Base == call.Receiver && lens.Kind == kind.FieldName
}

// deriveAssignWrites proves that every Assign has one nonempty contiguous
// increasing range in the authored Write sequence. The ranges are private
// projections: parent rows remain the sole authored authority.
func deriveAssignWrites(component *component, counts [keyspace.FamilyCount]uint32) bool {
	ranges := make([]Range, len(component.storage.assigns))
	cursor := uint32(0)
	for assignIndex := range component.storage.assigns {
		start := cursor
		assign, ok := termAt(keyspace.FamilyAssign, assignIndex, len(component.storage.assigns))
		if !ok {
			return false
		}
		for cursor < uint32(len(component.storage.writes)) && component.storage.writes[cursor].Assign == assign {
			write := component.storage.writes[cursor]
			if !flowrole.Addressable(counts, write.Target) {
				return false
			}
			cursor++
		}
		if start == cursor {
			return false
		}
		ranges[assignIndex] = Range{Start: start, End: cursor}
	}
	if cursor != uint32(len(component.storage.writes)) {
		return false
	}
	component.storage.assignWrite = ranges
	return true
}

// staticExactCandidate admits exactly the authored static-key vocabulary.
// Negative numeric keys retain their Unary source row until Source owns the
// future normalization into its canonical Key handle.
func staticExactCandidate(component *component, counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	if !flowrole.FieldSourceFamily(counts, term, kind.FieldExact) {
		return false
	}
	if keyspace.TermFamily(term) != keyspace.FamilyUnary {
		return true
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) > uint64(len(component.operators.unaries)) {
		return false
	}
	row := component.operators.unaries[ordinal-1]
	return row.Op == kind.UnaryNeg && (hasFamily(counts, row.Operand, keyspace.FamilyInteger) ||
		hasFamily(counts, row.Operand, keyspace.FamilyFloat))
}

func validateTableValueAdjustment(component *component) error {
	for _, row := range component.tables.rows {
		for at := row.Fields.Start; at < row.Fields.End; at++ {
			field := component.tables.order[at]
			fieldRow := component.tables.fields[keyspace.TermOrdinal(field)-1]
			values := component.values.rows[keyspace.TermOrdinal(fieldRow.Values)-1]
			fixed := values.Fixed.End - values.Fixed.Start
			open := values.Tail != 0
			finalOpen := at+1 == row.Fields.End && fieldRow.Kind == kind.FieldList && fixed == 0 && open
			if !(fixed == 1 && !open) && !finalOpen {
				return errors.New("program/flow: invalid TableField value adjustment")
			}
		}
	}
	return nil
}

func ordinalFits(index, length int) bool {
	return index >= 0 && length >= 0 && uint64(index) < uint64(length) && uint64(index) < uint64(keyspace.MaxTermOrdinal)
}

func termAt(family keyspace.Family, index, length int) (keyspace.Term, bool) {
	if !ordinalFits(index, length) {
		return 0, false
	}
	return keyspace.MakeTerm(family, uint32(index+1)), true
}
