package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source/admission"
)

// Build seals only authored Source rows. The explicit typed index batch is
// consumed later, after Flow's position seal has supplied its local geometry.
func Build(input Input) (*Draft, error) {
	a := &authority{}
	if err := buildIdentity(&a.identity, input.Name, input.Families); err != nil {
		return nil, err
	}
	if err := buildLiterals(a, input); err != nil {
		return nil, err
	}
	if err := buildOrder(a, input); err != nil {
		return nil, err
	}
	if err := buildSpellings(a, input); err != nil {
		return nil, err
	}
	if err := buildKeyFault(a, input); err != nil {
		return nil, err
	}
	if err := validateFaultSourceOwnership(a); err != nil {
		return nil, err
	}
	a.content = authoredContentID(a)
	if !a.content.Available() {
		return nil, errors.New("program/source: unavailable authored identity")
	}
	return &Draft{state: &draftState{authority: a}}, nil
}

func buildSpellings(a *authority, input Input) error {
	if a == nil {
		return errors.New("program/source: nil spelling authority")
	}
	cellCount := a.count(keyspace.FamilyCell)
	// Generic Source fixtures that do not carry backend debug metadata use the
	// canonical all-absent dense column. Lua lowering supplies the explicit
	// rows when authored names are available.
	if input.CellSpellings == nil {
		a.spellings.cells = make([]string, cellCount)
	} else {
		if len(input.CellSpellings) != cellCount {
			return errors.New("program/source: Cell spelling cardinality mismatch")
		}
		a.spellings.cells = make([]string, cellCount)
		for index, row := range input.CellSpellings {
			if !a.validFamilyTerm(row.Cell, keyspace.FamilyCell) || keyspace.TermOrdinal(row.Cell) != uint32(index+1) {
				return errors.New("program/source: invalid Cell spelling owner or order")
			}
			a.spellings.cells[index] = row.Name
		}
	}
	if len(input.CallSpellings) == 0 {
		return nil
	}
	a.spellings.calls = make([]CallSpelling, len(input.CallSpellings))
	var previous keyspace.Term
	for index, row := range input.CallSpellings {
		if !a.validFamilyTerm(row.Call, keyspace.FamilyCall) || row.Name == "" ||
			index > 0 && (row.Call <= previous || keyspace.TermOrdinal(row.Call) == 0) {
			return errors.New("program/source: invalid or duplicate Call spelling")
		}
		a.spellings.calls[index] = CallSpelling{Call: row.Call, Name: row.Name}
		previous = row.Call
	}
	return nil
}

// validateFaultSourceOwnership closes the authored side of control-fault
// containment. Every dense fault ordinal occurs once in its declared owner
// Body's direct source sequence; Finalize then requires that same direct term
// to have exactly one sealed source Position.
func validateFaultSourceOwnership(a *authority) error {
	if a == nil || a.count(keyspace.FamilyControlFault) == 0 {
		return nil
	}
	owners := make([]keyspace.Term, a.count(keyspace.FamilyControlFault))
	for bodyOrdinal, sourceRange := range a.order.bodyRanges {
		if !validRange(a.order.sourceTerms, sourceRange) {
			return errors.New("program/source: invalid Body source range")
		}
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1))
		for _, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			if keyspace.TermFamily(term) != keyspace.FamilyControlFault {
				continue
			}
			ordinal := keyspace.TermOrdinal(term)
			if ordinal == 0 || uint64(ordinal) > uint64(len(owners)) || owners[ordinal-1] != 0 {
				return errors.New("program/source: duplicate or invalid direct control fault")
			}
			owners[ordinal-1] = body
		}
	}
	for index, row := range a.keys.faults {
		if owners[index] == 0 || owners[index] != row.Owner {
			return errors.New("program/source: control fault lacks its owner Body source occurrence")
		}
	}
	return nil
}

func buildIdentity(store *identityStore, name string, rows []FamilySpans) error {
	if store == nil || name == "" || len(rows) != int(keyspace.FamilyCount-1) {
		return errors.New("program/source: incomplete family spans")
	}
	store.name = name
	var termCount uint64
	for index, row := range rows {
		family := keyspace.Family(index + 1)
		if row.Family != family || !keyspace.TermOrdinalFits(len(row.Spans)) {
			return errors.New("program/source: invalid family spans")
		}
		// Outcome is the sole derived Term family. Its identity is assigned by
		// Flow's canonical control topology during Finalize, never by authored
		// Source input. Keep the explicit family row in the denominator so a
		// caller cannot silently omit the derived family, but require it empty.
		if family == keyspace.FamilyOutcome && len(row.Spans) != 0 {
			return errors.New("program/source: authored Outcome spans are forbidden")
		}
		spans := make([]storedSpan, len(row.Spans))
		for at, span := range row.Spans {
			stored, ok := compactSpan(name, span)
			if !ok {
				return errors.New("program/source: invalid source span")
			}
			spans[at] = stored
		}
		store.spans[family] = spans
		termCount += uint64(len(spans))
		if termCount > uint64(^uint32(0)) {
			return errors.New("program/source: Term cardinality overflow")
		}
	}
	if termCount == 0 {
		return errors.New("program/source: empty Term cardinality")
	}
	return nil
}

func buildLiterals(a *authority, input Input) error {
	if a == nil || len(input.Nil) != a.count(keyspace.FamilyNil) ||
		len(input.Bool) != a.count(keyspace.FamilyBool) ||
		len(input.Integer) != a.count(keyspace.FamilyInteger) ||
		len(input.Float) != a.count(keyspace.FamilyFloat) ||
		len(input.String) != a.count(keyspace.FamilyString) {
		return errors.New("program/source: literal family cardinality mismatch")
	}
	a.literals.nil = append([]NilLiteral(nil), input.Nil...)
	a.literals.bool = append([]BoolLiteral(nil), input.Bool...)
	a.literals.integer = append([]IntegerLiteral(nil), input.Integer...)
	a.literals.float = append([]FloatLiteral(nil), input.Float...)
	a.literals.string = append([]StringLiteral(nil), input.String...)
	for _, owner := range a.literals.nil {
		if !a.validFamilyTerm(owner.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid nil owner")
		}
	}
	for _, row := range a.literals.bool {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid bool owner")
		}
	}
	for _, row := range a.literals.integer {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid integer owner")
		}
	}
	for _, row := range a.literals.float {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid float owner")
		}
	}
	for _, row := range a.literals.string {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid string owner")
		}
	}
	return nil
}

func buildOrder(a *authority, input Input) error {
	if a == nil {
		return errors.New("program/source: nil authority")
	}
	if err := buildBodyOrder(a, input.Bodies); err != nil {
		return err
	}
	cells := make([]bool, a.count(keyspace.FamilyCell))
	if err := buildBindOrder(a, input.Binds, cells); err != nil {
		return err
	}
	return buildFormalOrder(a, input.Functions, cells)
}

func buildBodyOrder(a *authority, rows []BodySource) error {
	count := a.count(keyspace.FamilyBody)
	if len(rows) != count {
		return errors.New("program/source: Body source cardinality mismatch")
	}
	a.order.bodyRanges = make([]termRange, count)
	seen := newTermMarks(&a.identity)
	for index, row := range rows {
		if !a.validFamilyTerm(row.Body, keyspace.FamilyBody) || keyspace.TermOrdinal(row.Body) != uint32(index+1) {
			return errors.New("program/source: invalid Body source owner")
		}
		start := len(a.order.sourceTerms)
		for _, term := range row.Terms {
			if !a.validDirectBodyTerm(term) || seen.take(term) {
				return errors.New("program/source: duplicate or invalid direct source Term")
			}
			a.order.sourceTerms = append(a.order.sourceTerms, term)
		}
		r, ok := makeRange(start, len(row.Terms))
		if !ok {
			return errors.New("program/source: Body source range overflow")
		}
		a.order.bodyRanges[index] = r
	}
	return nil
}

func buildBindOrder(a *authority, rows []BindCells, cells []bool) error {
	count := a.count(keyspace.FamilyBind)
	if len(rows) != count {
		return errors.New("program/source: Bind order cardinality mismatch")
	}
	a.order.bindRanges = make([]termRange, count)
	for index, row := range rows {
		if !a.validFamilyTerm(row.Bind, keyspace.FamilyBind) || keyspace.TermOrdinal(row.Bind) != uint32(index+1) {
			return errors.New("program/source: invalid Bind order owner")
		}
		start := len(a.order.bindTerms)
		for _, cell := range row.Cells {
			if !a.validFamilyTerm(cell, keyspace.FamilyCell) || cells[keyspace.TermOrdinal(cell)-1] {
				return errors.New("program/source: duplicate or invalid Bind Cell")
			}
			cells[keyspace.TermOrdinal(cell)-1] = true
			a.order.bindTerms = append(a.order.bindTerms, cell)
		}
		r, ok := makeRange(start, len(row.Cells))
		if !ok {
			return errors.New("program/source: Bind range overflow")
		}
		a.order.bindRanges[index] = r
	}
	return nil
}

func buildFormalOrder(a *authority, rows []FunctionFormals, cells []bool) error {
	count := a.count(keyspace.FamilyFunction)
	if len(rows) != count {
		return errors.New("program/source: Function formal cardinality mismatch")
	}
	a.order.formalRanges = make([]termRange, count)
	for index, row := range rows {
		if !a.validFamilyTerm(row.Function, keyspace.FamilyFunction) || keyspace.TermOrdinal(row.Function) != uint32(index+1) {
			return errors.New("program/source: invalid Function formal owner")
		}
		start := len(a.order.formalTerms)
		for _, formal := range row.Formals {
			if !a.validFamilyTerm(formal, keyspace.FamilyCell) || cells[keyspace.TermOrdinal(formal)-1] {
				return errors.New("program/source: duplicate or invalid Function formal")
			}
			cells[keyspace.TermOrdinal(formal)-1] = true
			a.order.formalTerms = append(a.order.formalTerms, formal)
		}
		r, ok := makeRange(start, len(row.Formals))
		if !ok {
			return errors.New("program/source: Function formal range overflow")
		}
		a.order.formalRanges[index] = r
	}
	return nil
}

func (a *authority) count(family keyspace.Family) int {
	if a == nil {
		return 0
	}
	return a.identity.familyCount(family)
}

func (a *authority) validTerm(term keyspace.Term) bool {
	return a != nil && keyspace.ValidTerm(term, keyspace.TermFamily(term), a.count(keyspace.TermFamily(term)))
}

// validDirectBodyTerm is the exact Source Body-order denominator, gated by
// the canonical AdmitsDirectBodyFamily admission primitive that Lua lowering
// also consumes.
func (a *authority) validDirectBodyTerm(term keyspace.Term) bool {
	return a != nil && admission.AdmitsDirectBodyFamily(keyspace.TermFamily(term)) && a.validTerm(term)
}

func (a *authority) validFamilyTerm(term keyspace.Term, family keyspace.Family) bool {
	return a != nil && keyspace.ValidTerm(term, family, a.count(family))
}

type termMarks struct{ rows [keyspace.FamilyCount][]bool }

func newTermMarks(identity *identityStore) termMarks {
	var result termMarks
	if identity == nil {
		return result
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		result.rows[family] = make([]bool, identity.familyCount(family))
	}
	return result
}

func (s *termMarks) take(term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family == keyspace.FamilyInvalid || ordinal == 0 || uint64(ordinal) > uint64(len(s.rows[family])) {
		return true
	}
	at := ordinal - 1
	if s.rows[family][at] {
		return true
	}
	s.rows[family][at] = true
	return false
}

func makeRange(start, count int) (termRange, bool) {
	if start < 0 || count < 0 || uint64(start)+uint64(count) > uint64(^uint32(0)) {
		return termRange{}, false
	}
	return termRange{start: uint32(start), end: uint32(start + count)}, true
}

func compactSpan(name string, span Span) (storedSpan, bool) {
	allZero := span.StartLine == 0 && span.StartCol == 0 && span.EndLine == 0 && span.EndCol == 0
	if name == "" || span.File != name && (span.File != "" || !allZero) {
		return storedSpan{}, false
	}
	if _, ok := CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol); !ok {
		return storedSpan{}, false
	}
	return storedSpan{startLine: span.StartLine, startCol: span.StartCol, endLine: span.EndLine, endCol: span.EndCol}, true
}
