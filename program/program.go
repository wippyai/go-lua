// Package program contains the sealed source-level semantic program core.
package program

import (
	"errors"
	"math"
)

// Term is a compact 32-bit identity: a 24-bit typed-family index and an
// 8-bit family tag. Zero is invalid.
type Term uint32

const (
	tagBits  = 8
	tagMask  = uint32(1<<tagBits - 1)
	indexMax = uint32(1<<(32-tagBits) - 1)
)

const (
	tagInvalid uint8 = iota
	tagNil
	tagBool
	tagInteger
	tagFloat
	tagString
	tagValues
	tagLensExact
	tagLensKey
	tagEvaluate
	tagNormal
	tagReturn
	tagThrow
	tagYield
	tagBreak
	tagContinue
	tagMu
	tagBody
	tagCell
	tagRead
	tagAssign
	tagFunction
	tagCapture
	tagCall
	tagBranch
	tagTable
	tagCount
)

func makeTerm(tag uint8, index uint32) Term { return Term(index<<tagBits | uint32(tag)) }
func (t Term) tag() uint8                   { return uint8(uint32(t) & tagMask) }
func (t Term) index() uint32                { return uint32(t) >> tagBits }

// Span identifies a 1-based source line/column extent. Zero end coordinates
// mean that the extent is point-like or unknown. An all-zero coordinate set is
// valid for generated code.
type Span struct {
	File                string
	StartLine, StartCol int
	EndLine, EndCol     int
}

// termRange indexes one contiguous Term pool. It never owns a slice.
type termRange struct{ start, end uint32 }
type storedSpan struct {
	file                uint32
	startLine, startCol uint32
	endLine, endCol     uint32
}
type valuesRow struct {
	fixed termRange
	tail  Term
}
type exactLensRow struct {
	base, source Term
	exact        uint32
}
type keyLensRow struct{ base, key Term }
type outcomeRow struct{ values Term }
type muRow struct{ head, back Term }
type bodyRow struct {
	owned  termRange
	filled bool
}
type cellRow struct{ body Term }
type readRow struct{ cell Term }
type assignRow struct {
	targets termRange
	values  Term
}
type functionRow struct {
	body    Term
	formals termRange
	vararg  bool
}
type captureRow struct{ inner, outer Term }
type callRow struct{ callee, actuals Term }
type branchRow struct{ condition, whenTrue, whenFalse Term }

const (
	exactBool uint8 = iota + 1
	exactInteger
	exactFloat
	exactString
)

type exactKey struct {
	kind uint8
	bool bool
	int  int64
	bits uint64
	text string
}

// Program is the immutable result of Builder.Seal.
type Program struct {
	terms []Term // source mint order only; never used for lookup
	files []string
	spans [tagCount][]storedSpan

	valueTerms  []Term
	formalTerms []Term
	orderTerms  []Term
	orders      [tagCount][]termRange

	nils     uint32
	bools    []bool
	integers []int64
	floats   []uint64
	strings  []string
	values   []valuesRow

	lensExact []exactLensRow
	lensKeys  []keyLensRow
	evaluates uint32
	normals   []outcomeRow
	returns   []outcomeRow
	throws    []outcomeRow
	yields    []outcomeRow
	breaks    []outcomeRow
	continues []outcomeRow
	mus       []muRow
	bodies    []bodyRow
	cells     []cellRow
	reads     []readRow
	assigns   []assignRow
	functions []functionRow
	captures  []captureRow
	calls     []callRow
	branches  []branchRow
	tables    uint32
	exactKeys []exactKey
}

// Builder is the sole mutable construction path for Program.
type Builder struct {
	terms     []Term
	files     []string
	fileIndex map[string]uint32
	spans     [tagCount][]storedSpan
	poison    bool

	valueTerms  []Term
	formalTerms []Term
	orderTerms  []Term
	orders      [tagCount][]termRange

	nils     uint32
	bools    []bool
	integers []int64
	floats   []uint64
	strings  []string
	values   []valuesRow

	lensExact   []exactLensRow
	lensKeys    []keyLensRow
	evaluates   uint32
	normals     []outcomeRow
	returns     []outcomeRow
	throws      []outcomeRow
	yields      []outcomeRow
	breaks      []outcomeRow
	continues   []outcomeRow
	mus         []muRow
	bodies      []bodyRow
	cells       []cellRow
	reads       []readRow
	assigns     []assignRow
	functions   []functionRow
	captures    []captureRow
	calls       []callRow
	branches    []branchRow
	tables      uint32
	exactKeys   []exactKey
	exactLookup map[exactKey]uint32
}

// NewBuilder returns an empty Program builder.
func NewBuilder() *Builder { return &Builder{} }

func validSpan(span Span) bool {
	coords := [...]int{span.StartLine, span.StartCol, span.EndLine, span.EndCol}
	allZero := true
	for _, coord := range coords {
		if coord < 0 || uint64(coord) > math.MaxUint32 {
			return false
		}
		allZero = allZero && coord == 0
	}
	if allZero {
		return true
	}
	if span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	if span.EndLine == 0 || span.EndCol == 0 {
		return span.EndLine == 0 && span.EndCol == 0
	}
	return span.EndLine > span.StartLine || span.EndLine == span.StartLine && span.EndCol >= span.StartCol
}

func (b *Builder) compactSpan(span Span) (storedSpan, bool) {
	if !validSpan(span) {
		b.poison = true
		return storedSpan{}, false
	}
	if b.fileIndex == nil {
		b.fileIndex = make(map[string]uint32)
	}
	index, ok := b.fileIndex[span.File]
	if !ok {
		if uint64(len(b.files)) >= math.MaxUint32 {
			b.poison = true
			return storedSpan{}, false
		}
		index = uint32(len(b.files))
		b.files = append(b.files, span.File)
		b.fileIndex[span.File] = index
	}
	return storedSpan{
		file:      index,
		startLine: uint32(span.StartLine),
		startCol:  uint32(span.StartCol),
		endLine:   uint32(span.EndLine),
		endCol:    uint32(span.EndCol),
	}, true
}

func (b *Builder) mint(tag uint8, span Span, index uint32) Term {
	if b.poison || index == 0 || index > indexMax {
		b.poison = true
		return 0
	}
	stored, ok := b.compactSpan(span)
	if !ok {
		return 0
	}
	term := makeTerm(tag, index)
	b.terms = append(b.terms, term)
	b.spans[tag] = append(b.spans[tag], stored)
	return term
}

func (b *Builder) familyIndex(length int) uint32 {
	if length <= 0 || uint64(length) > uint64(indexMax) {
		b.poison = true
		return 0
	}
	return uint32(length)
}

func (b *Builder) appendPool(pool *[]Term, terms []Term) (termRange, bool) {
	start := uint64(len(*pool))
	end := start + uint64(len(terms))
	if end > math.MaxUint32 {
		b.poison = true
		return termRange{}, false
	}
	*pool = append(*pool, terms...)
	return termRange{start: uint32(start), end: uint32(end)}, true
}

func (b *Builder) setOrder(term Term, terms []Term) {
	rows := &b.orders[term.tag()]
	index := term.index()
	for uint32(len(*rows)) < index {
		*rows = append(*rows, termRange{})
	}
	rangeOrder, ok := b.appendPool(&b.orderTerms, terms)
	if !ok {
		return
	}
	(*rows)[index-1] = rangeOrder
}

func (b *Builder) setOrderRange(term Term, order termRange) {
	rows := &b.orders[term.tag()]
	for uint32(len(*rows)) < term.index() {
		*rows = append(*rows, termRange{})
	}
	(*rows)[term.index()-1] = order
}

func (b *Builder) setValuesOrder(term Term, fixed []Term, tail Term) {
	rows := &b.orders[term.tag()]
	index := term.index()
	for uint32(len(*rows)) < index {
		*rows = append(*rows, termRange{})
	}
	start := uint64(len(b.orderTerms))
	end := start + uint64(len(fixed))
	if tail != 0 {
		end++
	}
	if end > math.MaxUint32 {
		b.poison = true
		return
	}
	b.orderTerms = append(b.orderTerms, fixed...)
	if tail != 0 {
		b.orderTerms = append(b.orderTerms, tail)
	}
	(*rows)[index-1] = termRange{start: uint32(start), end: uint32(end)}
}

// Nil, Bool, Integer, Float, and String mint a fresh literal occurrence on
// every call, preserving distinct spans and float bits.
func (b *Builder) Nil(span Span) Term {
	if b.nils == indexMax {
		b.poison = true
		return 0
	}
	b.nils++
	term := b.mint(tagNil, span, b.nils)
	if term == 0 {
		b.nils--
	}
	return term
}
func (b *Builder) Bool(span Span, value bool) Term {
	b.bools = append(b.bools, value)
	term := b.mint(tagBool, span, b.familyIndex(len(b.bools)))
	if term == 0 {
		b.bools = b.bools[:len(b.bools)-1]
	}
	return term
}
func (b *Builder) Integer(span Span, value int64) Term {
	b.integers = append(b.integers, value)
	term := b.mint(tagInteger, span, b.familyIndex(len(b.integers)))
	if term == 0 {
		b.integers = b.integers[:len(b.integers)-1]
	}
	return term
}
func (b *Builder) Float(span Span, value float64) Term {
	b.floats = append(b.floats, math.Float64bits(value))
	term := b.mint(tagFloat, span, b.familyIndex(len(b.floats)))
	if term == 0 {
		b.floats = b.floats[:len(b.floats)-1]
	}
	return term
}
func (b *Builder) String(span Span, value string) Term {
	b.strings = append(b.strings, value)
	term := b.mint(tagString, span, b.familyIndex(len(b.strings)))
	if term == 0 {
		b.strings = b.strings[:len(b.strings)-1]
	}
	return term
}

// Values records an ordered fixed prefix and optional final open tail.
func (b *Builder) Values(span Span, fixed []Term, tail Term) Term {
	rangeFixed, ok := b.appendPool(&b.valueTerms, fixed)
	if !ok {
		return 0
	}
	b.values = append(b.values, valuesRow{fixed: rangeFixed, tail: tail})
	term := b.mint(tagValues, span, b.familyIndex(len(b.values)))
	if term == 0 {
		b.values = b.values[:len(b.values)-1]
		b.valueTerms = b.valueTerms[:rangeFixed.start]
		return 0
	}
	b.setValuesOrder(term, fixed, tail)
	return term
}

// LensExact records an evaluated base plus an exact bool, integer, float, or
// string literal key. Seal rejects nil, NaN, invalid, and non-literal keys.
func (b *Builder) LensExact(span Span, base, key Term) Term {
	exact, _ := b.normalizedExactKey(key)
	b.lensExact = append(b.lensExact, exactLensRow{base: base, source: key, exact: exact})
	term := b.mint(tagLensExact, span, b.familyIndex(len(b.lensExact)))
	if term == 0 {
		b.lensExact = b.lensExact[:len(b.lensExact)-1]
		return 0
	}
	b.setOrder(term, []Term{base})
	return term
}

// LensKey records base then dynamic key evaluation.
func (b *Builder) LensKey(span Span, base, key Term) Term {
	b.lensKeys = append(b.lensKeys, keyLensRow{base: base, key: key})
	term := b.mint(tagLensKey, span, b.familyIndex(len(b.lensKeys)))
	if term == 0 {
		b.lensKeys = b.lensKeys[:len(b.lensKeys)-1]
		return 0
	}
	b.setOrder(term, []Term{base, key})
	return term
}

// Evaluate retains exact left-to-right evaluation without a generic node.
func (b *Builder) Evaluate(span Span, terms ...Term) Term {
	if b.evaluates == indexMax {
		b.poison = true
		return 0
	}
	b.evaluates++
	term := b.mint(tagEvaluate, span, b.evaluates)
	if term != 0 {
		b.setOrder(term, terms)
	} else {
		b.evaluates--
	}
	return term
}

func (b *Builder) Outcome(span Span, values Term) Term  { return b.Normal(span, values) }
func (b *Builder) Normal(span Span, values Term) Term   { return b.outcome(tagNormal, span, values) }
func (b *Builder) Return(span Span, values Term) Term   { return b.outcome(tagReturn, span, values) }
func (b *Builder) Throw(span Span, values Term) Term    { return b.outcome(tagThrow, span, values) }
func (b *Builder) Yield(span Span, values Term) Term    { return b.outcome(tagYield, span, values) }
func (b *Builder) Break(span Span, values Term) Term    { return b.outcome(tagBreak, span, values) }
func (b *Builder) Continue(span Span, values Term) Term { return b.outcome(tagContinue, span, values) }
func (b *Builder) outcome(tag uint8, span Span, value Term) Term {
	var rows *[]outcomeRow
	switch tag {
	case tagNormal:
		rows = &b.normals
	case tagReturn:
		rows = &b.returns
	case tagThrow:
		rows = &b.throws
	case tagYield:
		rows = &b.yields
	case tagBreak:
		rows = &b.breaks
	default:
		rows = &b.continues
	}
	*rows = append(*rows, outcomeRow{values: value})
	term := b.mint(tag, span, b.familyIndex(len(*rows)))
	if term == 0 {
		*rows = (*rows)[:len(*rows)-1]
		return 0
	}
	b.setOrder(term, []Term{value})
	return term
}

// Mu records an explicit recurrence head and exact backedge.
func (b *Builder) Mu(span Span, head, back Term) Term {
	b.mus = append(b.mus, muRow{head: head, back: back})
	term := b.mint(tagMu, span, b.familyIndex(len(b.mus)))
	if term == 0 {
		b.mus = b.mus[:len(b.mus)-1]
		return 0
	}
	b.setOrder(term, []Term{head, back})
	return term
}

// Body mints an identity. SetBody must later fill its owned execution order
// exactly once, after Cells, Functions, recursive Calls, and outcomes exist.
func (b *Builder) Body(span Span) Term {
	b.bodies = append(b.bodies, bodyRow{})
	term := b.mint(tagBody, span, b.familyIndex(len(b.bodies)))
	if term == 0 {
		b.bodies = b.bodies[:len(b.bodies)-1]
	}
	return term
}

func (b *Builder) SetBody(body Term, owned ...Term) bool {
	if !b.has(body, tagBody) {
		b.poison = true
		return false
	}
	row := &b.bodies[body.index()-1]
	if row.filled {
		b.poison = true
		return false
	}
	order, ok := b.appendPool(&b.orderTerms, owned)
	if !ok {
		return false
	}
	row.owned = order
	row.filled = true
	b.setOrderRange(body, order)
	return true
}

// Cell is lexical storage owned by one Body. Read observes that storage.
func (b *Builder) Cell(span Span, body Term) Term {
	b.cells = append(b.cells, cellRow{body: body})
	term := b.mint(tagCell, span, b.familyIndex(len(b.cells)))
	if term == 0 {
		b.cells = b.cells[:len(b.cells)-1]
	}
	return term
}

func (b *Builder) Read(span Span, cell Term) Term {
	b.reads = append(b.reads, readRow{cell: cell})
	term := b.mint(tagRead, span, b.familyIndex(len(b.reads)))
	if term == 0 {
		b.reads = b.reads[:len(b.reads)-1]
	}
	return term
}

// Assign evaluates its Cells/Lenses from left to right, then evaluates RHS
// Values. The row represents one delayed commit after that complete order.
func (b *Builder) Assign(span Span, targets []Term, values Term) Term {
	start := uint32(len(b.orderTerms))
	targetRange, ok := b.appendPool(&b.orderTerms, targets)
	if !ok {
		return 0
	}
	if _, ok = b.appendPool(&b.orderTerms, []Term{values}); !ok {
		b.orderTerms = b.orderTerms[:start]
		return 0
	}
	b.assigns = append(b.assigns, assignRow{targets: targetRange, values: values})
	term := b.mint(tagAssign, span, b.familyIndex(len(b.assigns)))
	if term == 0 {
		b.assigns = b.assigns[:len(b.assigns)-1]
		b.orderTerms = b.orderTerms[:start]
		return 0
	}
	b.setOrderRange(term, termRange{start: start, end: uint32(len(b.orderTerms))})
	return term
}

// Function binds a Body to its ordered formal Cells.
func (b *Builder) Function(span Span, body Term, formals []Term, vararg bool) Term {
	r, ok := b.appendPool(&b.formalTerms, formals)
	if !ok {
		return 0
	}
	b.functions = append(b.functions, functionRow{body: body, formals: r, vararg: vararg})
	term := b.mint(tagFunction, span, b.familyIndex(len(b.functions)))
	if term == 0 {
		b.functions = b.functions[:len(b.functions)-1]
		b.formalTerms = b.formalTerms[:r.start]
	}
	return term
}

// Capture records one exact inner-to-outer lexical Cell correspondence.
func (b *Builder) Capture(span Span, inner, outer Term) Term {
	b.captures = append(b.captures, captureRow{inner: inner, outer: outer})
	term := b.mint(tagCapture, span, b.familyIndex(len(b.captures)))
	if term == 0 {
		b.captures = b.captures[:len(b.captures)-1]
	}
	return term
}

// Call is an open result producer. Evaluation is callee followed by actual Values.
func (b *Builder) Call(span Span, callee, actuals Term) Term {
	b.calls = append(b.calls, callRow{callee: callee, actuals: actuals})
	term := b.mint(tagCall, span, b.familyIndex(len(b.calls)))
	if term == 0 {
		b.calls = b.calls[:len(b.calls)-1]
		return 0
	}
	b.setOrder(term, []Term{callee, actuals})
	return term
}

// Branch evaluates condition and then transfers to exactly one owned Body or Outcome.
func (b *Builder) Branch(span Span, condition, whenTrue, whenFalse Term) Term {
	b.branches = append(b.branches, branchRow{condition: condition, whenTrue: whenTrue, whenFalse: whenFalse})
	term := b.mint(tagBranch, span, b.familyIndex(len(b.branches)))
	if term == 0 {
		b.branches = b.branches[:len(b.branches)-1]
		return 0
	}
	b.setOrder(term, []Term{condition})
	return term
}

// Table mints allocation identity only. Field initialization is represented
// once by Lens and delayed Assign terms owned by a Body.
func (b *Builder) Table(span Span) Term {
	if b.tables == indexMax {
		b.poison = true
		return 0
	}
	b.tables++
	term := b.mint(tagTable, span, b.tables)
	if term == 0 {
		b.tables--
	}
	return term
}

func (b *Builder) normalizedExactKey(term Term) (uint32, bool) {
	key, ok := b.exactKey(term)
	if !ok {
		return 0, false
	}
	if b.exactLookup == nil {
		b.exactLookup = make(map[exactKey]uint32)
	}
	if index := b.exactLookup[key]; index != 0 {
		return index, true
	}
	if uint64(len(b.exactKeys)) >= uint64(indexMax) {
		b.poison = true
		return 0, false
	}
	index := uint32(len(b.exactKeys) + 1)
	b.exactKeys = append(b.exactKeys, key)
	b.exactLookup[key] = index
	return index, true
}
func (b *Builder) exactKey(term Term) (exactKey, bool) {
	if !b.valid(term) {
		return exactKey{}, false
	}
	switch term.tag() {
	case tagBool:
		return exactKey{kind: exactBool, bool: b.bools[term.index()-1]}, true
	case tagInteger:
		return exactKey{kind: exactInteger, int: b.integers[term.index()-1]}, true
	case tagFloat:
		return normalizeFloat(b.floats[term.index()-1])
	case tagString:
		return exactKey{kind: exactString, text: b.strings[term.index()-1]}, true
	}
	return exactKey{}, false
}
func normalizeFloat(bits uint64) (exactKey, bool) {
	value := math.Float64frombits(bits)
	if math.IsNaN(value) {
		return exactKey{}, false
	}
	if value == 0 {
		return exactKey{kind: exactInteger}, true
	}
	const minInt64Float = -9223372036854775808.0
	const maxInt64Exclusive = 9223372036854775808.0
	if value >= minInt64Float && value < maxInt64Exclusive && math.Trunc(value) == value {
		integer := int64(value)
		if float64(integer) == value {
			return exactKey{kind: exactInteger, int: integer}, true
		}
	}
	return exactKey{kind: exactFloat, bits: bits}, true
}

func (b *Builder) valid(term Term) bool {
	if b == nil || term == 0 || term.index() == 0 {
		return false
	}
	index := term.index()
	switch term.tag() {
	case tagNil:
		return index <= b.nils
	case tagBool:
		return index <= uint32(len(b.bools))
	case tagInteger:
		return index <= uint32(len(b.integers))
	case tagFloat:
		return index <= uint32(len(b.floats))
	case tagString:
		return index <= uint32(len(b.strings))
	case tagValues:
		return index <= uint32(len(b.values))
	case tagLensExact:
		return index <= uint32(len(b.lensExact))
	case tagLensKey:
		return index <= uint32(len(b.lensKeys))
	case tagEvaluate:
		return index <= b.evaluates
	case tagNormal:
		return index <= uint32(len(b.normals))
	case tagReturn:
		return index <= uint32(len(b.returns))
	case tagThrow:
		return index <= uint32(len(b.throws))
	case tagYield:
		return index <= uint32(len(b.yields))
	case tagBreak:
		return index <= uint32(len(b.breaks))
	case tagContinue:
		return index <= uint32(len(b.continues))
	case tagMu:
		return index <= uint32(len(b.mus))
	case tagBody:
		return index <= uint32(len(b.bodies))
	case tagCell:
		return index <= uint32(len(b.cells))
	case tagRead:
		return index <= uint32(len(b.reads))
	case tagAssign:
		return index <= uint32(len(b.assigns))
	case tagFunction:
		return index <= uint32(len(b.functions))
	case tagCapture:
		return index <= uint32(len(b.captures))
	case tagCall:
		return index <= uint32(len(b.calls))
	case tagBranch:
		return index <= uint32(len(b.branches))
	case tagTable:
		return index <= b.tables
	}
	return false
}
func (b *Builder) has(term Term, tag uint8) bool { return term.tag() == tag && b.valid(term) }

// Seal validates every family iteratively, then returns an immutable snapshot.
func (b *Builder) Seal() (*Program, error) {
	if b == nil {
		return nil, errors.New("program: nil builder")
	}
	if b.poison {
		return nil, errors.New("program: poisoned builder")
	}
	for _, row := range b.values {
		for i := row.fixed.start; i < row.fixed.end; i++ {
			if !b.valid(b.valueTerms[i]) {
				return nil, errors.New("program: invalid Values reference")
			}
		}
		if row.tail != 0 && !b.valid(row.tail) {
			return nil, errors.New("program: invalid Values tail")
		}
	}
	for _, row := range b.lensExact {
		if !b.valid(row.base) || !b.valid(row.source) || row.exact == 0 || row.exact > uint32(len(b.exactKeys)) {
			return nil, errors.New("program: invalid exact Lens reference")
		}
		key, ok := b.exactKey(row.source)
		if !ok || b.exactKeys[row.exact-1] != key {
			return nil, errors.New("program: invalid exact Lens key")
		}
	}
	for _, row := range b.lensKeys {
		if !b.valid(row.base) || !b.valid(row.key) {
			return nil, errors.New("program: invalid dynamic Lens reference")
		}
	}
	for tag := uint8(1); tag < tagCount; tag++ {
		for _, row := range b.orders[tag] {
			for i := row.start; i < row.end; i++ {
				if !b.valid(b.orderTerms[i]) {
					return nil, errors.New("program: invalid evaluation-order reference")
				}
			}
		}
	}
	for _, rows := range [][]outcomeRow{b.normals, b.returns, b.throws, b.yields, b.breaks, b.continues} {
		for _, row := range rows {
			if !b.has(row.values, tagValues) {
				return nil, errors.New("program: Outcome requires Values")
			}
		}
	}
	for _, row := range b.mus {
		if !b.valid(row.head) || !b.valid(row.back) {
			return nil, errors.New("program: invalid Mu reference")
		}
	}
	ownedBy := make(map[Term]Term)
	for bodyIndex, row := range b.bodies {
		if !row.filled {
			return nil, errors.New("program: Body was not filled")
		}
		body := makeTerm(tagBody, uint32(bodyIndex+1))
		for i := row.owned.start; i < row.owned.end; i++ {
			owned := b.orderTerms[i]
			if !b.valid(owned) {
				return nil, errors.New("program: invalid Body-owned term")
			}
			if previous := ownedBy[owned]; previous != 0 {
				return nil, errors.New("program: term has duplicate Body ownership")
			}
			ownedBy[owned] = body
		}
	}
	for _, row := range b.cells {
		if !b.has(row.body, tagBody) {
			return nil, errors.New("program: Cell requires Body owner")
		}
	}
	for _, row := range b.reads {
		if !b.has(row.cell, tagCell) {
			return nil, errors.New("program: Read requires Cell")
		}
	}
	for _, row := range b.assigns {
		if !b.has(row.values, tagValues) {
			return nil, errors.New("program: Assign requires RHS Values")
		}
		for i := row.targets.start; i < row.targets.end; i++ {
			target := b.orderTerms[i]
			if !b.has(target, tagCell) && !b.has(target, tagLensExact) && !b.has(target, tagLensKey) {
				return nil, errors.New("program: Assign target requires Cell or Lens")
			}
		}
	}
	functionByBody := make(map[Term]Term)
	for functionIndex, row := range b.functions {
		if !b.has(row.body, tagBody) {
			return nil, errors.New("program: Function requires Body")
		}
		if functionByBody[row.body] != 0 {
			return nil, errors.New("program: Body has more than one Function")
		}
		functionByBody[row.body] = makeTerm(tagFunction, uint32(functionIndex+1))
		for i := row.formals.start; i < row.formals.end; i++ {
			formal := b.formalTerms[i]
			if !b.has(formal, tagCell) || b.cells[formal.index()-1].body != row.body {
				return nil, errors.New("program: Function formal requires Cell owned by its Body")
			}
		}
	}
	for _, row := range b.captures {
		if !b.has(row.inner, tagCell) || !b.has(row.outer, tagCell) {
			return nil, errors.New("program: Capture requires inner and outer Cells")
		}
		if b.cells[row.inner.index()-1].body == b.cells[row.outer.index()-1].body {
			return nil, errors.New("program: Capture must cross Body ownership")
		}
	}
	for _, row := range b.calls {
		if !b.valid(row.callee) || !b.has(row.actuals, tagValues) {
			return nil, errors.New("program: Call requires callee and actual Values")
		}
	}
	for _, row := range b.branches {
		if !b.valid(row.condition) || !b.branchArm(row.whenTrue) || !b.branchArm(row.whenFalse) {
			return nil, errors.New("program: Branch requires condition and Body/Outcome arms")
		}
	}
	return b.snapshot(), nil
}

func (b *Builder) branchArm(term Term) bool {
	switch term.tag() {
	case tagBody, tagNormal, tagReturn, tagThrow, tagYield, tagBreak, tagContinue:
		return b.valid(term)
	default:
		return false
	}
}

func (b *Builder) snapshot() *Program {
	p := &Program{
		terms:       copyTerms(b.terms),
		files:       append([]string(nil), b.files...),
		valueTerms:  copyTerms(b.valueTerms),
		formalTerms: copyTerms(b.formalTerms),
		orderTerms:  copyTerms(b.orderTerms),
		nils:        b.nils,
		bools:       append([]bool(nil), b.bools...),
		integers:    append([]int64(nil), b.integers...),
		floats:      append([]uint64(nil), b.floats...),
		strings:     append([]string(nil), b.strings...),
		values:      append([]valuesRow(nil), b.values...),
		lensExact:   append([]exactLensRow(nil), b.lensExact...),
		lensKeys:    append([]keyLensRow(nil), b.lensKeys...),
		evaluates:   b.evaluates,
		normals:     append([]outcomeRow(nil), b.normals...),
		returns:     append([]outcomeRow(nil), b.returns...),
		throws:      append([]outcomeRow(nil), b.throws...),
		yields:      append([]outcomeRow(nil), b.yields...),
		breaks:      append([]outcomeRow(nil), b.breaks...),
		continues:   append([]outcomeRow(nil), b.continues...),
		mus:         append([]muRow(nil), b.mus...),
		bodies:      append([]bodyRow(nil), b.bodies...),
		cells:       append([]cellRow(nil), b.cells...),
		reads:       append([]readRow(nil), b.reads...),
		assigns:     append([]assignRow(nil), b.assigns...),
		functions:   append([]functionRow(nil), b.functions...),
		captures:    append([]captureRow(nil), b.captures...),
		calls:       append([]callRow(nil), b.calls...),
		branches:    append([]branchRow(nil), b.branches...),
		tables:      b.tables,
		exactKeys:   append([]exactKey(nil), b.exactKeys...),
	}
	for tag := uint8(1); tag < tagCount; tag++ {
		p.spans[tag] = append([]storedSpan(nil), b.spans[tag]...)
		p.orders[tag] = append([]termRange(nil), b.orders[tag]...)
	}
	return p
}

// Valid reports whether term selects a row in p.
func (p *Program) Valid(term Term) bool {
	if p == nil || term == 0 || term.index() == 0 {
		return false
	}
	index := term.index()
	switch term.tag() {
	case tagNil:
		return index <= p.nils
	case tagBool:
		return index <= uint32(len(p.bools))
	case tagInteger:
		return index <= uint32(len(p.integers))
	case tagFloat:
		return index <= uint32(len(p.floats))
	case tagString:
		return index <= uint32(len(p.strings))
	case tagValues:
		return index <= uint32(len(p.values))
	case tagLensExact:
		return index <= uint32(len(p.lensExact))
	case tagLensKey:
		return index <= uint32(len(p.lensKeys))
	case tagEvaluate:
		return index <= p.evaluates
	case tagNormal:
		return index <= uint32(len(p.normals))
	case tagReturn:
		return index <= uint32(len(p.returns))
	case tagThrow:
		return index <= uint32(len(p.throws))
	case tagYield:
		return index <= uint32(len(p.yields))
	case tagBreak:
		return index <= uint32(len(p.breaks))
	case tagContinue:
		return index <= uint32(len(p.continues))
	case tagMu:
		return index <= uint32(len(p.mus))
	case tagBody:
		return index <= uint32(len(p.bodies))
	case tagCell:
		return index <= uint32(len(p.cells))
	case tagRead:
		return index <= uint32(len(p.reads))
	case tagAssign:
		return index <= uint32(len(p.assigns))
	case tagFunction:
		return index <= uint32(len(p.functions))
	case tagCapture:
		return index <= uint32(len(p.captures))
	case tagCall:
		return index <= uint32(len(p.calls))
	case tagBranch:
		return index <= uint32(len(p.branches))
	case tagTable:
		return index <= p.tables
	}
	return false
}
func (p *Program) has(term Term, tag uint8) bool { return term.tag() == tag && p.Valid(term) }

// TermCount and TermAt provide non-allocating mint-order traversal.
func (p *Program) TermCount() int {
	if p == nil {
		return 0
	}
	return len(p.terms)
}
func (p *Program) TermAt(index int) (Term, bool) {
	if p == nil || index < 0 || index >= len(p.terms) {
		return 0, false
	}
	return p.terms[index], true
}

// AppendTerms appends mint-order terms to dst, reusing its capacity when able.
func (p *Program) AppendTerms(dst []Term) []Term {
	if p == nil {
		return dst
	}
	return append(dst, p.terms...)
}

// Span returns a Term's source span in O(1).
func (p *Program) Span(term Term) (Span, bool) {
	if !p.Valid(term) {
		return Span{}, false
	}
	row := p.spans[term.tag()][term.index()-1]
	return Span{
		File:      p.files[row.file],
		StartLine: int(row.startLine),
		StartCol:  int(row.startCol),
		EndLine:   int(row.endLine),
		EndCol:    int(row.endCol),
	}, true
}
func (p *Program) Nil(term Term) bool { return p.has(term, tagNil) }
func (p *Program) Bool(term Term) (bool, bool) {
	if !p.has(term, tagBool) {
		return false, false
	}
	return p.bools[term.index()-1], true
}
func (p *Program) Integer(term Term) (int64, bool) {
	if !p.has(term, tagInteger) {
		return 0, false
	}
	return p.integers[term.index()-1], true
}
func (p *Program) Float(term Term) (float64, bool) {
	if !p.has(term, tagFloat) {
		return 0, false
	}
	return math.Float64frombits(p.floats[term.index()-1]), true
}
func (p *Program) String(term Term) (string, bool) {
	if !p.has(term, tagString) {
		return "", false
	}
	return p.strings[term.index()-1], true
}

func (p *Program) valueRange(term Term) (termRange, bool) {
	if !p.has(term, tagValues) {
		return termRange{}, false
	}
	return p.values[term.index()-1].fixed, true
}

// ValuesLen, ValuesAt, and ValuesTail are non-allocating indexed accessors
// for a Values relation.
func (p *Program) ValuesLen(term Term) (int, bool) {
	r, ok := p.valueRange(term)
	return int(r.end - r.start), ok
}
func (p *Program) ValuesAt(term Term, index int) (Term, bool) {
	r, ok := p.valueRange(term)
	if !ok || index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.valueTerms[r.start+uint32(index)], true
}
func (p *Program) ValuesTail(term Term) (Term, bool) {
	if !p.has(term, tagValues) {
		return 0, false
	}
	return p.values[term.index()-1].tail, true
}

// AppendValues appends the fixed Values prefix to dst. tail is zero for a
// closed relation.
func (p *Program) AppendValues(term Term, dst []Term) (out []Term, tail Term, ok bool) {
	r, ok := p.valueRange(term)
	if !ok {
		return dst, 0, false
	}
	row := p.values[term.index()-1]
	return append(dst, p.valueTerms[r.start:r.end]...), row.tail, true
}

func (p *Program) Lens(term Term) (base, key Term, dynamic, ok bool) {
	if p.has(term, tagLensExact) {
		r := p.lensExact[term.index()-1]
		return r.base, r.source, false, true
	}
	if p.has(term, tagLensKey) {
		r := p.lensKeys[term.index()-1]
		return r.base, r.key, true, true
	}
	return 0, 0, false, false
}
func (p *Program) SameKey(left, right Term) bool {
	if !p.has(left, tagLensExact) || !p.has(right, tagLensExact) {
		return false
	}
	return p.lensExact[left.index()-1].exact == p.lensExact[right.index()-1].exact
}

func (p *Program) orderRange(term Term) (termRange, bool) {
	if !p.Valid(term) {
		return termRange{}, false
	}
	rows := p.orders[term.tag()]
	if term.index() > uint32(len(rows)) {
		return termRange{}, false
	}
	return rows[term.index()-1], true
}

// OrderCount and OrderAt are non-allocating indexed accessors for exact
// left-to-right evaluation order.
func (p *Program) OrderCount(term Term) (int, bool) {
	r, ok := p.orderRange(term)
	return int(r.end - r.start), ok
}
func (p *Program) OrderAt(term Term, index int) (Term, bool) {
	r, ok := p.orderRange(term)
	if !ok || index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.orderTerms[r.start+uint32(index)], true
}

// AppendOrder appends exact left-to-right evaluation order to dst.
func (p *Program) AppendOrder(term Term, dst []Term) ([]Term, bool) {
	r, ok := p.orderRange(term)
	if !ok {
		return dst, false
	}
	return append(dst, p.orderTerms[r.start:r.end]...), true
}

func (p *Program) Outcome(term Term) (Term, bool) {
	switch term.tag() {
	case tagNormal:
		return p.Normal(term)
	case tagReturn:
		return p.Return(term)
	case tagThrow:
		return p.Throw(term)
	case tagYield:
		return p.Yield(term)
	case tagBreak:
		return p.Break(term)
	case tagContinue:
		return p.Continue(term)
	}
	return 0, false
}
func (p *Program) Normal(term Term) (Term, bool)   { return p.outcome(term, tagNormal) }
func (p *Program) Return(term Term) (Term, bool)   { return p.outcome(term, tagReturn) }
func (p *Program) Throw(term Term) (Term, bool)    { return p.outcome(term, tagThrow) }
func (p *Program) Yield(term Term) (Term, bool)    { return p.outcome(term, tagYield) }
func (p *Program) Break(term Term) (Term, bool)    { return p.outcome(term, tagBreak) }
func (p *Program) Continue(term Term) (Term, bool) { return p.outcome(term, tagContinue) }
func (p *Program) outcome(term Term, tag uint8) (Term, bool) {
	if !p.has(term, tag) {
		return 0, false
	}
	index := term.index() - 1
	switch tag {
	case tagNormal:
		return p.normals[index].values, true
	case tagReturn:
		return p.returns[index].values, true
	case tagThrow:
		return p.throws[index].values, true
	case tagYield:
		return p.yields[index].values, true
	case tagBreak:
		return p.breaks[index].values, true
	}
	return p.continues[index].values, true
}
func (p *Program) Mu(term Term) (head, back Term, ok bool) {
	if !p.has(term, tagMu) {
		return 0, 0, false
	}
	r := p.mus[term.index()-1]
	return r.head, r.back, true
}

func (p *Program) BodyLen(term Term) (int, bool) {
	if !p.has(term, tagBody) {
		return 0, false
	}
	r := p.bodies[term.index()-1].owned
	return int(r.end - r.start), true
}

func (p *Program) BodyAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagBody) {
		return 0, false
	}
	r := p.bodies[term.index()-1].owned
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.orderTerms[r.start+uint32(index)], true
}

func (p *Program) AppendBody(term Term, dst []Term) ([]Term, bool) {
	if !p.has(term, tagBody) {
		return dst, false
	}
	r := p.bodies[term.index()-1].owned
	return append(dst, p.orderTerms[r.start:r.end]...), true
}

func (p *Program) Cell(term Term) (Term, bool) {
	if !p.has(term, tagCell) {
		return 0, false
	}
	return p.cells[term.index()-1].body, true
}

func (p *Program) Read(term Term) (Term, bool) {
	if !p.has(term, tagRead) {
		return 0, false
	}
	return p.reads[term.index()-1].cell, true
}

func (p *Program) AssignLen(term Term) (int, bool) {
	if !p.has(term, tagAssign) {
		return 0, false
	}
	r := p.assigns[term.index()-1].targets
	return int(r.end - r.start), true
}

func (p *Program) AssignAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagAssign) {
		return 0, false
	}
	r := p.assigns[term.index()-1].targets
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.orderTerms[r.start+uint32(index)], true
}

func (p *Program) AssignValues(term Term) (Term, bool) {
	if !p.has(term, tagAssign) {
		return 0, false
	}
	return p.assigns[term.index()-1].values, true
}

func (p *Program) AppendAssign(term Term, dst []Term) (targets []Term, values Term, ok bool) {
	if !p.has(term, tagAssign) {
		return dst, 0, false
	}
	row := p.assigns[term.index()-1]
	return append(dst, p.orderTerms[row.targets.start:row.targets.end]...), row.values, true
}

func (p *Program) Function(term Term) (body Term, vararg, ok bool) {
	if !p.has(term, tagFunction) {
		return 0, false, false
	}
	row := p.functions[term.index()-1]
	return row.body, row.vararg, true
}

func (p *Program) FormalLen(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].formals
	return int(r.end - r.start), true
}

func (p *Program) FormalAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].formals
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.formalTerms[r.start+uint32(index)], true
}

func (p *Program) AppendFormals(term Term, dst []Term) ([]Term, bool) {
	if !p.has(term, tagFunction) {
		return dst, false
	}
	r := p.functions[term.index()-1].formals
	return append(dst, p.formalTerms[r.start:r.end]...), true
}

func (p *Program) Capture(term Term) (inner, outer Term, ok bool) {
	if !p.has(term, tagCapture) {
		return 0, 0, false
	}
	row := p.captures[term.index()-1]
	return row.inner, row.outer, true
}

func (p *Program) Call(term Term) (callee, actuals Term, ok bool) {
	if !p.has(term, tagCall) {
		return 0, 0, false
	}
	row := p.calls[term.index()-1]
	return row.callee, row.actuals, true
}

func (p *Program) Branch(term Term) (condition, whenTrue, whenFalse Term, ok bool) {
	if !p.has(term, tagBranch) {
		return 0, 0, 0, false
	}
	row := p.branches[term.index()-1]
	return row.condition, row.whenTrue, row.whenFalse, true
}

func (p *Program) Table(term Term) bool { return p.has(term, tagTable) }

func copyTerms(terms []Term) []Term {
	if len(terms) == 0 {
		return nil
	}
	return append([]Term(nil), terms...)
}
