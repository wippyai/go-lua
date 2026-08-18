package source

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
)

// CoordinateFromParts validates one compact coordinate. The all-zero value is
// the explicit absence representation used by artifact plumbing; every other
// valid value has a start and either a complete end or no end.
func CoordinateFromParts(startLine, startCol, endLine, endCol uint32) (Coordinate, bool) {
	if startLine == 0 && startCol == 0 && endLine == 0 && endCol == 0 {
		return Coordinate{}, true
	}
	if startLine == 0 || startCol == 0 ||
		(endLine == 0) != (endCol == 0) ||
		(endLine != 0 && (endLine < startLine || endLine == startLine && endCol < startCol)) {
		return Coordinate{}, false
	}
	return Coordinate{startLine: startLine, startCol: startCol, endLine: endLine, endCol: endCol}, true
}

// Parts returns the compact filename-free source coordinate fields.
func (v Coordinate) Parts() (startLine, startCol, endLine, endCol uint32) {
	return v.startLine, v.startCol, v.endLine, v.endCol
}

func (c *Component) View() View {
	if c == nil {
		return View{}
	}
	return View{authority: c.authority}
}

// Cold returns the immutable identity snapshot without retaining the Source
// owner or any finalization/derived-position storage.
func (c *Component) Cold() Cold {
	if c == nil || c.authority == nil {
		return Cold{}
	}
	return Cold{contentID: c.authority.content}
}

// ContentID is the authored Source identity exposed by the cold snapshot.
func (c Cold) ContentID() identity.ContentID { return c.contentID }

func (v View) Identity() Identity { return Identity{authority: v.authority} }
func (v View) Order() Order       { return Order{authority: v.authority} }
func (v View) Binds() BindOrder   { return BindOrder{authority: v.authority} }
func (v View) Formals() FormalOrder {
	return FormalOrder{authority: v.authority}
}
func (v View) Spellings() Spellings {
	return Spellings{authority: v.authority}
}
func (v View) Index() Index       { return Index(v) }
func (v View) Literals() Literals { return Literals{authority: v.authority} }
func (v View) Keys() Keys         { return Keys{authority: v.authority} }
func (v View) Faults() Faults     { return Faults{authority: v.authority} }

// CellRoles returns Source's immutable authored Cell role column. The view
// remains fenced to the exact committed authority pointer; callers cannot
// construct a role or substitute an equal-content Source component.
func (v View) CellRoles() CellRoles {
	if v.authority == nil {
		return CellRoles{}
	}
	return CellRoles{authority: v.authority, roles: v.authority.cellRoles}
}

// liveAuthority validates a lifecycle-bound authored view. Published
// Component views pass a nil state and remain ordinary immutable reads;
// Preimage views carry the Draft state and disappear at Commit or Abort.
func liveAuthority(a *authority, state *draftState) *authority {
	if state == nil {
		return a
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftFinalizerClaimed || state.authority == nil {
		return nil
	}
	return state.authority
}

func (v Identity) Name() string {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return ""
	}
	return a.identity.name
}

func (v Identity) TermCount() uint32 {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0
	}
	return a.identity.termCount
}

func (v Identity) FamilyCount(family keyspace.Family) int {
	a := liveAuthority(v.authority, v.state)
	if a == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount {
		return 0
	}
	return int(a.identity.counts[family])
}

func (v Identity) Span(term keyspace.Term) (Span, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return Span{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family == keyspace.FamilyInvalid || ordinal == 0 || uint64(ordinal) > uint64(len(a.identity.spans[family])) {
		return Span{}, false
	}
	stored := a.identity.spans[family][ordinal-1]
	return Span{File: a.identity.name, StartLine: stored.startLine, StartCol: stored.startCol, EndLine: stored.endLine, EndCol: stored.endCol}, true
}

// Render attaches this Source identity's filename to one nonzero valid
// coordinate. It is the only conversion from a secondary coordinate back to a
// public source Span.
func (v Identity) Render(coordinate Coordinate) (Span, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || a.identity.name == "" {
		return Span{}, false
	}
	startLine, startCol, endLine, endCol := coordinate.Parts()
	if startLine == 0 && startCol == 0 && endLine == 0 && endCol == 0 {
		return Span{}, false
	}
	valid, ok := CoordinateFromParts(startLine, startCol, endLine, endCol)
	if !ok || valid != coordinate {
		return Span{}, false
	}
	return Span{File: a.identity.name, StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol}, true
}

func (v Identity) ContentID() identity.ContentID {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return identity.ContentID{}
	}
	return a.content
}

func (v Order) BodyLen(body keyspace.Term) (int, bool) {
	r, ok := v.bodyRange(body)
	return int(r.end - r.start), ok
}

func (v Order) BodyAt(body keyspace.Term, index int) (keyspace.Term, bool) {
	r, ok := v.bodyRange(body)
	if !ok || index < 0 || uint64(index) >= uint64(r.end-r.start) {
		return 0, false
	}
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0, false
	}
	return a.order.sourceTerms[r.start+uint32(index)], true
}

func (v Order) bodyRange(body keyspace.Term) (termRange, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(body, keyspace.FamilyBody) {
		return termRange{}, false
	}
	r := a.order.bodyRanges[keyspace.TermOrdinal(body)-1]
	return r, validRange(a.order.sourceTerms, r)
}

func (v BindOrder) Len(bind keyspace.Term) (int, bool) {
	r, ok := v.rangeFor(bind)
	return int(r.end - r.start), ok
}

func (v BindOrder) At(bind keyspace.Term, index int) (keyspace.Term, bool) {
	r, ok := v.rangeFor(bind)
	if !ok || index < 0 || uint64(index) >= uint64(r.end-r.start) {
		return 0, false
	}
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0, false
	}
	return a.order.bindTerms[r.start+uint32(index)], true
}

func (v BindOrder) rangeFor(bind keyspace.Term) (termRange, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(bind, keyspace.FamilyBind) {
		return termRange{}, false
	}
	r := a.order.bindRanges[keyspace.TermOrdinal(bind)-1]
	return r, validRange(a.order.bindTerms, r)
}

func (v FormalOrder) Len(function keyspace.Term) (int, bool) {
	r, ok := v.rangeFor(function)
	return int(r.end - r.start), ok
}

func (v FormalOrder) At(function keyspace.Term, index int) (keyspace.Term, bool) {
	r, ok := v.rangeFor(function)
	if !ok || index < 0 || uint64(index) >= uint64(r.end-r.start) {
		return 0, false
	}
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0, false
	}
	return a.order.formalTerms[r.start+uint32(index)], true
}

func (v FormalOrder) rangeFor(function keyspace.Term) (termRange, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(function, keyspace.FamilyFunction) {
		return termRange{}, false
	}
	r := a.order.formalRanges[keyspace.TermOrdinal(function)-1]
	return r, validRange(a.order.formalTerms, r)
}

// CellName returns an authored debug spelling for one Cell. Empty dense
// entries denote an unavailable/anonymous spelling and therefore return
// false; the Cell identity and its coordinate remain available through the
// independent Identity and Order views.
func (v Spellings) CellName(cell keyspace.Term) (string, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(cell, keyspace.FamilyCell) {
		return "", false
	}
	name := a.spellings.cells[keyspace.TermOrdinal(cell)-1]
	return name, name != ""
}

// CallName returns an optional authored debug spelling for one statically
// named Call. Dynamic or unknown calls have no row and return false.
func (v Spellings) CallName(call keyspace.Term) (string, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(call, keyspace.FamilyCall) {
		return "", false
	}
	rows := a.spellings.calls
	at := sort.Search(len(rows), func(index int) bool { return rows[index].Call >= call })
	if at == len(rows) || rows[at].Call != call {
		return "", false
	}
	return rows[at].Name, true
}

func (v Index) Root(term keyspace.Term) (keyspace.Term, bool) {
	row, ok := v.position(term)
	return row.root, ok
}

// Entry returns the unique root Body of the sealed lexical forest. Entry is
// Source-owned derived index state: callers must not infer it from Body
// ordinals or reconstruct it by scanning parents.
func (v Index) Entry() (keyspace.Term, bool) {
	if v.authority == nil || !v.authority.validFamilyTerm(v.authority.index.entry, keyspace.FamilyBody) {
		return 0, false
	}
	return v.authority.index.entry, true
}

// Position is the exact containing direct source coordinate. It never implies
// Flow activation; callers compose that separate Flow relation explicitly.
func (v Index) Position(term keyspace.Term) (keyspace.Term, int, int, bool) {
	row, ok := v.position(term)
	if !ok {
		return 0, 0, 0, false
	}
	return row.body, int(row.offset), int(row.cursor), true
}

// Frontier returns the precomputed final frontier including Repeat adjustment.
func (v Index) Frontier(term keyspace.Term) (keyspace.Term, int, bool) {
	row, ok := v.position(term)
	if !ok {
		return 0, 0, false
	}
	return row.frontierBody, int(row.frontierCursor), true
}

func (v Index) BodyParent(body keyspace.Term) (keyspace.Term, bool) {
	if v.authority == nil || !v.authority.validFamilyTerm(body, keyspace.FamilyBody) {
		return 0, false
	}
	parent := v.authority.index.parents[keyspace.TermOrdinal(body)-1]
	return parent, parent != 0
}

func (v Index) BodyRootLen(body keyspace.Term) (int, bool) {
	r, ok := v.rootRange(body)
	return int(r.end - r.start), ok
}

func (v Index) BodyRootAt(body keyspace.Term, offset int) (keyspace.Term, bool) {
	r, ok := v.rootRange(body)
	if !ok || offset < 0 || uint64(offset) >= uint64(r.end-r.start) {
		return 0, false
	}
	return v.authority.index.rootTerms[r.start+uint32(offset)], true
}

func (v Index) position(term keyspace.Term) (positionSlot, bool) {
	if v.authority == nil {
		return positionSlot{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family == keyspace.FamilyInvalid || ordinal == 0 {
		return positionSlot{}, false
	}
	return v.authority.index.positions[family].lookup(ordinal)
}

func (v Index) rootRange(body keyspace.Term) (termRange, bool) {
	if v.authority == nil || !v.authority.validFamilyTerm(body, keyspace.FamilyBody) {
		return termRange{}, false
	}
	r := v.authority.index.rootRanges[keyspace.TermOrdinal(body)-1]
	return r, validRange(v.authority.index.rootTerms, r)
}

func (v Literals) Nils() Nils {
	return Nils(v)
}
func (v Literals) Bools() Bools {
	return Bools(v)
}
func (v Literals) Integers() Integers {
	return Integers(v)
}
func (v Literals) Floats() Floats {
	return Floats(v)
}
func (v Literals) Strings() Strings {
	return Strings(v)
}

func (v Nils) Count() int {
	return literalCount(liveAuthority(v.authority, v.state), keyspace.FamilyNil)
}
func (v Bools) Count() int {
	return literalCount(liveAuthority(v.authority, v.state), keyspace.FamilyBool)
}
func (v Integers) Count() int {
	return literalCount(liveAuthority(v.authority, v.state), keyspace.FamilyInteger)
}
func (v Floats) Count() int {
	return literalCount(liveAuthority(v.authority, v.state), keyspace.FamilyFloat)
}
func (v Strings) Count() int {
	return literalCount(liveAuthority(v.authority, v.state), keyspace.FamilyString)
}

func (v Nils) At(index int) (keyspace.Term, keyspace.Term, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || index < 0 || index >= len(a.literals.nil) {
		return 0, 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyNil, uint32(index+1)), a.literals.nil[index].Owner, true
}
func (v Bools) At(index int) (keyspace.Term, keyspace.Term, bool, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || index < 0 || index >= len(a.literals.bool) {
		return 0, 0, false, false
	}
	row := a.literals.bool[index]
	return keyspace.MakeTerm(keyspace.FamilyBool, uint32(index+1)), row.Owner, row.Value, true
}
func (v Integers) At(index int) (keyspace.Term, keyspace.Term, int64, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || index < 0 || index >= len(a.literals.integer) {
		return 0, 0, 0, false
	}
	row := a.literals.integer[index]
	return keyspace.MakeTerm(keyspace.FamilyInteger, uint32(index+1)), row.Owner, row.Value, true
}
func (v Floats) At(index int) (keyspace.Term, keyspace.Term, uint64, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || index < 0 || index >= len(a.literals.float) {
		return 0, 0, 0, false
	}
	row := a.literals.float[index]
	return keyspace.MakeTerm(keyspace.FamilyFloat, uint32(index+1)), row.Owner, row.Bits, true
}
func (v Strings) At(index int) (keyspace.Term, keyspace.Term, string, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || index < 0 || index >= len(a.literals.string) {
		return 0, 0, "", false
	}
	row := a.literals.string[index]
	return keyspace.MakeTerm(keyspace.FamilyString, uint32(index+1)), row.Owner, row.Value, true
}

// Count is the exact number of authored Source Key rows.
func (v Keys) Count() int {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0
	}
	return len(a.keys.keys)
}

// Name returns only a name-spelled key. The returned Key resolves through the
// atom methods below; Flow deliberately has no dependency on this form.
func (v Keys) Name(term keyspace.Term) (keyspace.Term, string, keyspace.Key, bool) {
	row, value, ok := v.row(term, keyFormName)
	if !ok || value.Kind != keyspace.LiteralString {
		return 0, "", 0, false
	}
	return row.owner, value.String, row.exact, true
}

// List returns only a positional-list key. Ordinal is positive by Source law.
func (v Keys) List(term keyspace.Term) (keyspace.Term, int64, keyspace.Key, bool) {
	row, value, ok := v.row(term, keyFormList)
	if !ok || value.Kind != keyspace.LiteralInteger {
		return 0, 0, 0, false
	}
	return row.owner, value.Integer, row.exact, true
}

func (v Keys) row(term keyspace.Term, form keyForm) (familyKey, keyspace.LiteralValue, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(term, keyspace.FamilyKey) {
		return familyKey{}, keyspace.LiteralValue{}, false
	}
	row := a.keys.keys[keyspace.TermOrdinal(term)-1]
	if row.form != form {
		return familyKey{}, keyspace.LiteralValue{}, false
	}
	value, ok := exactValue(a, row.exact)
	return row, value, ok
}

// ExactCount is the complete Source-owned exact-atom denominator size.
func (v Keys) ExactCount() int {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0
	}
	return len(a.keys.exact.atoms)
}

// Exact resolves one dense atom handle.
func (v Keys) Exact(key keyspace.Key) (keyspace.LiteralValue, bool) {
	return exactValue(liveAuthority(v.authority, v.state), key)
}

// Find resolves one literal through Source's sealed denominator. It uses the
// canonical enumeration directly; no construction map survives publication.
func (v Keys) Find(raw keyspace.LiteralValue) (keyspace.Key, bool) {
	return findExact(liveAuthority(v.authority, v.state), raw)
}

func findExact(a *authority, raw keyspace.LiteralValue) (keyspace.Key, bool) {
	if a == nil {
		return 0, false
	}
	value, ok := scalar.Normalize(raw)
	if !ok {
		return 0, false
	}
	atoms := a.keys.exact.atoms
	at := sort.Search(len(atoms), func(index int) bool {
		return scalar.CompareCanonical(scalar.FromLiteral(atoms[index]), scalar.FromLiteral(value)) >= 0
	})
	if at == len(atoms) {
		return 0, false
	}
	candidate := atoms[at]
	return keyspace.Key(at + 1), scalar.CompareCanonical(scalar.FromLiteral(candidate), scalar.FromLiteral(value)) == 0
}

// ExactAt enumerates atom handles in canonical value order, independent of their
// source allocation order. It is for deterministic artifacts and diagnostics.
func (v Keys) ExactAt(index int) (keyspace.Key, keyspace.LiteralValue, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || index < 0 || index >= len(a.keys.exact.atoms) {
		return 0, keyspace.LiteralValue{}, false
	}
	return keyspace.Key(index + 1), a.keys.exact.atoms[index], true
}

func exactValue(a *authority, key keyspace.Key) (keyspace.LiteralValue, bool) {
	if a == nil || key == 0 || uint64(key) > uint64(len(a.keys.exact.atoms)) {
		return keyspace.LiteralValue{}, false
	}
	return a.keys.exact.atoms[key-1], true
}

// Count is the exact number of binder-rejected source control rows.
func (v Faults) Count() int {
	a := liveAuthority(v.authority, v.state)
	if a == nil {
		return 0
	}
	return len(a.keys.faults)
}

// At returns one typed control-fault row by its canonical term.
func (v Faults) At(term keyspace.Term) (ControlFault, bool) {
	a := liveAuthority(v.authority, v.state)
	if a == nil || !a.validFamilyTerm(term, keyspace.FamilyControlFault) {
		return ControlFault{}, false
	}
	return a.keys.faults[keyspace.TermOrdinal(term)-1], true
}

func literalCount(a *authority, family keyspace.Family) int {
	if a == nil {
		return 0
	}
	return a.count(family)
}
