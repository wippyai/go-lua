// Package source owns the mutable Source construction rows used by the Lua
// lowerer.  It intentionally knows nothing about Collector, Flow, Static, or
// Module admission.  The assembly root supplies already-minted terms and
// coordinates the cross-owner checks before calling these row operations.
package source

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

// Rows is the Source-owned construction plane.  Its fields remain private so
// sibling assembly verticals cannot retain or mutate Source rows directly.
// Every slice returned by a read method is a copy suitable for the immediate
// synchronous Source.Build transaction.
type Rows struct {
	nilLiterals []programsource.NilLiteral
	bools       []programsource.BoolLiteral
	integers    []programsource.IntegerLiteral
	floats      []programsource.FloatLiteral
	strings     []programsource.StringLiteral
	bodies      []programsource.BodySource
	binds       []programsource.BindCells
	functions   []programsource.FunctionFormals
	keys        []programsource.KeyInput
	faults      []programsource.ControlFault
	exact       []keyspace.LiteralValue
	entry       keyspace.Term
	filled      []bool
	imports     []bool
}

// New reserves the Source-side fill markers for the pre-censused Import
// family.  The family denominator itself remains owned by assembly core.
func New(importCount int) Rows {
	if importCount < 0 || uint64(importCount) > uint64(keyspace.MaxTermOrdinal) {
		return Rows{}
	}
	return Rows{imports: make([]bool, importCount)}
}

// Reset releases all construction rows. It is used only after the core has
// closed the one-shot construction transaction.
func (r *Rows) Reset() {
	if r != nil {
		*r = Rows{}
	}
}

func (r *Rows) AddNil(owner keyspace.Term) {
	if r != nil {
		r.nilLiterals = append(r.nilLiterals, programsource.NilLiteral{Owner: owner})
	}
}

func (r *Rows) AddBool(owner keyspace.Term, value bool) {
	if r != nil {
		r.bools = append(r.bools, programsource.BoolLiteral{Owner: owner, Value: value})
	}
}

func (r *Rows) AddInteger(owner keyspace.Term, value int64) {
	if r != nil {
		r.integers = append(r.integers, programsource.IntegerLiteral{Owner: owner, Value: value})
	}
}

func (r *Rows) AddFloat(owner keyspace.Term, bits uint64) {
	if r != nil {
		r.floats = append(r.floats, programsource.FloatLiteral{Owner: owner, Bits: bits})
	}
}

func (r *Rows) AddString(owner keyspace.Term, value string) {
	if r != nil {
		r.strings = append(r.strings, programsource.StringLiteral{Owner: owner, Value: string([]byte(value))})
	}
}

// AddBody reserves one authored Body source row and its later fill marker.
func (r *Rows) AddBody(body keyspace.Term) {
	if r != nil {
		r.bodies = append(r.bodies, programsource.BodySource{Body: body})
		r.filled = append(r.filled, false)
	}
}

// SetBody fills an existing Body exactly once. Direct-term admission belongs
// to assembly core because it needs Flow/Static owner facts.
func (r *Rows) SetBody(body keyspace.Term, terms []keyspace.Term) bool {
	if r == nil || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
		return false
	}
	at := int(keyspace.TermOrdinal(body) - 1)
	if at < 0 || at >= len(r.bodies) || at >= len(r.filled) || r.filled[at] {
		return false
	}
	r.bodies[at].Terms = append([]keyspace.Term(nil), terms...)
	r.filled[at] = true
	return true
}

func (r *Rows) SetEntry(body keyspace.Term) bool {
	if r == nil || r.entry != 0 || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
		return false
	}
	r.entry = body
	return true
}

func (r *Rows) Entry() keyspace.Term {
	if r == nil {
		return 0
	}
	return r.entry
}

func (r *Rows) AddBind(bind keyspace.Term, cells []keyspace.Term) {
	if r != nil {
		r.binds = append(r.binds, programsource.BindCells{Bind: bind, Cells: append([]keyspace.Term(nil), cells...)})
	}
}

func (r *Rows) AddFunction(function keyspace.Term, formals []keyspace.Term) {
	if r != nil {
		r.functions = append(r.functions, programsource.FunctionFormals{Function: function, Formals: append([]keyspace.Term(nil), formals...)})
	}
}

func (r *Rows) AddKey(key programsource.KeyInput) {
	if r != nil {
		r.keys = append(r.keys, key)
	}
}

func (r *Rows) AddFault(fault programsource.ControlFault) {
	if r != nil {
		r.faults = append(r.faults, fault)
	}
}

// AddExact records a raw exact-key candidate. Source.Build performs the
// canonical normalization and dense Key assignment only during Publish.
func (r *Rows) AddExact(value keyspace.LiteralValue) error {
	if r == nil || !validExact(value) {
		return errors.New("program/lower/collector: invalid exact-key candidate")
	}
	if value.String != "" {
		value.String = string([]byte(value.String))
	}
	r.exact = append(r.exact, value)
	return nil
}

func validExact(value keyspace.LiteralValue) bool {
	switch value.Kind {
	case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
		return true
	case keyspace.LiteralFloat:
		return !math.IsNaN(math.Float64frombits(value.FloatBits))
	default:
		return false
	}
}

func (r *Rows) FillImport(ordinal uint32, span programsource.Span) bool {
	if r == nil || ordinal == 0 || uint64(ordinal) > uint64(len(r.imports)) || !validSpan(span) {
		return false
	}
	at := ordinal - 1
	if r.imports[at] {
		return false
	}
	r.imports[at] = true
	return true
}

func validSpan(span programsource.Span) bool {
	allZero := span.StartLine == 0 && span.StartCol == 0 && span.EndLine == 0 && span.EndCol == 0
	if allZero {
		return true
	}
	if span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	if span.EndLine == 0 || span.EndCol == 0 {
		return span.EndLine == 0 && span.EndCol == 0
	}
	return span.EndLine > span.StartLine || (span.EndLine == span.StartLine && span.EndCol >= span.StartCol)
}

func (r *Rows) ImportComplete() bool {
	if r == nil {
		return false
	}
	for _, filled := range r.imports {
		if !filled {
			return false
		}
	}
	return true
}

func (r *Rows) BodyAt(index int) (programsource.BodySource, bool) {
	if r == nil || index < 0 || index >= len(r.bodies) {
		return programsource.BodySource{}, false
	}
	row := r.bodies[index]
	row.Terms = append([]keyspace.Term(nil), row.Terms...)
	return row, true
}

func (r *Rows) FaultAt(index int) (programsource.ControlFault, bool) {
	if r == nil || index < 0 || index >= len(r.faults) {
		return programsource.ControlFault{}, false
	}
	return r.faults[index], true
}

func (r *Rows) BindCount() int {
	if r == nil {
		return 0
	}
	return len(r.binds)
}

func (r *Rows) FunctionCount() int {
	if r == nil {
		return 0
	}
	return len(r.functions)
}

func (r *Rows) BodyTermSeen(term keyspace.Term) bool {
	if r == nil {
		return false
	}
	for index, row := range r.bodies {
		if index >= len(r.filled) || !r.filled[index] {
			continue
		}
		for _, existing := range row.Terms {
			if existing == term {
				return true
			}
		}
	}
	return false
}

func (r *Rows) CellAlreadyOrdered(cell keyspace.Term) bool {
	if r == nil {
		return false
	}
	for _, row := range r.binds {
		for _, existing := range row.Cells {
			if existing == cell {
				return true
			}
		}
	}
	for _, row := range r.functions {
		for _, existing := range row.Formals {
			if existing == cell {
				return true
			}
		}
	}
	return false
}

// ExactLiteral exposes only a copied authored literal payload to Flow
// admission. It never exposes the mutable exact candidate pool.
func (r *Rows) ExactLiteral(term keyspace.Term) (keyspace.LiteralValue, bool) {
	if r == nil || term == 0 {
		return keyspace.LiteralValue{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.LiteralValue{}, false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		if int(ordinal) <= len(r.bools) {
			return keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: r.bools[ordinal-1].Value}, true
		}
	case keyspace.FamilyInteger:
		if int(ordinal) <= len(r.integers) {
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: r.integers[ordinal-1].Value}, true
		}
	case keyspace.FamilyFloat:
		if int(ordinal) <= len(r.floats) {
			value := keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: r.floats[ordinal-1].Bits}
			return value, validExact(value)
		}
	case keyspace.FamilyString:
		if int(ordinal) <= len(r.strings) {
			return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: r.strings[ordinal-1].Value}, true
		}
	}
	return keyspace.LiteralValue{}, false
}

func (r *Rows) cloneRows() (nilRows []programsource.NilLiteral, boolRows []programsource.BoolLiteral, integerRows []programsource.IntegerLiteral, floatRows []programsource.FloatLiteral, stringRows []programsource.StringLiteral, bodyRows []programsource.BodySource, bindRows []programsource.BindCells, functionRows []programsource.FunctionFormals, keyRows []programsource.KeyInput, faultRows []programsource.ControlFault, exactRows []keyspace.LiteralValue) {
	if r == nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil
	}
	return append([]programsource.NilLiteral(nil), r.nilLiterals...),
		append([]programsource.BoolLiteral(nil), r.bools...),
		append([]programsource.IntegerLiteral(nil), r.integers...),
		append([]programsource.FloatLiteral(nil), r.floats...),
		append([]programsource.StringLiteral(nil), r.strings...),
		cloneBodyRows(r.bodies), cloneBindRows(r.binds), cloneFunctionRows(r.functions),
		append([]programsource.KeyInput(nil), r.keys...),
		append([]programsource.ControlFault(nil), r.faults...),
		append([]keyspace.LiteralValue(nil), r.exact...)
}

func cloneBodyRows(rows []programsource.BodySource) []programsource.BodySource {
	copyRows := append([]programsource.BodySource(nil), rows...)
	for index := range copyRows {
		copyRows[index].Terms = append([]keyspace.Term(nil), rows[index].Terms...)
	}
	return copyRows
}

func cloneBindRows(rows []programsource.BindCells) []programsource.BindCells {
	copyRows := append([]programsource.BindCells(nil), rows...)
	for index := range copyRows {
		copyRows[index].Cells = append([]keyspace.Term(nil), rows[index].Cells...)
	}
	return copyRows
}

func cloneFunctionRows(rows []programsource.FunctionFormals) []programsource.FunctionFormals {
	copyRows := append([]programsource.FunctionFormals(nil), rows...)
	for index := range copyRows {
		copyRows[index].Formals = append([]keyspace.Term(nil), rows[index].Formals...)
	}
	return copyRows
}

// Materialize validates the Source-owned denominator and returns an owned
// input. All row slices are copied before returning, so a later Reset cannot
// affect it.
func (r *Rows) Materialize(name string, spans [keyspace.FamilyCount][]programsource.Span, counts [keyspace.FamilyCount]uint32, moduleComplete bool) (programsource.Input, keyspace.Term, error) {
	if r == nil || name == "" || r.entry == 0 || len(r.bodies) == 0 {
		return programsource.Input{}, 0, errors.New("program/lower/collector: incomplete Source construction")
	}
	if keyspace.TermFamily(r.entry) != keyspace.FamilyBody || keyspace.TermOrdinal(r.entry) == 0 || keyspace.TermOrdinal(r.entry) > counts[keyspace.FamilyBody] {
		return programsource.Input{}, 0, errors.New("program/lower/collector: Entry is not a known Body")
	}
	if !moduleComplete {
		return programsource.Input{}, 0, errors.New("program/lower/collector: incomplete Module census")
	}
	if counts[keyspace.FamilyInvalid] != 0 {
		return programsource.Input{}, 0, errors.New("program/lower/collector: invalid Source family denominator")
	}
	if len(r.imports) != int(counts[keyspace.FamilyImport]) || !r.ImportComplete() {
		return programsource.Input{}, 0, errors.New("program/lower/collector: incomplete reserved Import")
	}
	pairs := []struct {
		family keyspace.Family
		got    int
		name   string
	}{
		{keyspace.FamilyNil, len(r.nilLiterals), "Nil"},
		{keyspace.FamilyBool, len(r.bools), "Bool"},
		{keyspace.FamilyInteger, len(r.integers), "Integer"},
		{keyspace.FamilyFloat, len(r.floats), "Float"},
		{keyspace.FamilyString, len(r.strings), "String"},
		{keyspace.FamilyBody, len(r.bodies), "Body"},
		{keyspace.FamilyBind, len(r.binds), "Bind"},
		{keyspace.FamilyFunction, len(r.functions), "Function"},
		{keyspace.FamilyKey, len(r.keys), "Key"},
		{keyspace.FamilyControlFault, len(r.faults), "ControlFault"},
	}
	for _, pair := range pairs {
		if pair.got != int(counts[pair.family]) {
			return programsource.Input{}, 0, fmt.Errorf("program/lower/collector: Source %s row count %d disagrees with census %d", pair.name, pair.got, counts[pair.family])
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if len(spans[family]) != int(counts[family]) {
			return programsource.Input{}, 0, fmt.Errorf("program/lower/collector: Source %v span count %d disagrees with census %d", family, len(spans[family]), counts[family])
		}
	}
	for _, filled := range r.filled {
		if !filled {
			return programsource.Input{}, 0, errors.New("program/lower/collector: unfilled Body")
		}
	}
	nilRows, boolRows, integerRows, floatRows, stringRows, bodyRows, bindRows, functionRows, keyRows, faultRows, exactRows := r.cloneRows()
	families := make([]programsource.FamilySpans, int(keyspace.FamilyCount-1))
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		families[family-1] = programsource.FamilySpans{Family: family, Spans: append([]programsource.Span(nil), spans[family]...)}
	}
	return programsource.Input{
		Name: name, Families: families, Nil: nilRows, Bool: boolRows,
		Integer: integerRows, Float: floatRows, String: stringRows,
		Bodies: bodyRows, Binds: bindRows, Functions: functionRows,
		Keys: keyRows, Faults: faultRows, ExactAtoms: exactRows,
	}, r.entry, nil
}
