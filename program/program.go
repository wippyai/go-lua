// Package program contains the sealed source-level semantic program core.
package program

import (
	"errors"
	"math"
)

// Term is a compact 32-bit identity: a 24-bit typed-family index and an
// 8-bit family tag. Zero is invalid.
type Term uint32

// Key is a Program-scoped normalized exact-key atom. Zero denotes a dynamic,
// nil, or NaN key with no storable equality identity.
type Key uint32

// FieldKind distinguishes Lua constructor field evaluation without introducing
// a field Term family.
type FieldKind uint8

type Capture struct{ Inner, Outer Term }

const (
	FieldList FieldKind = iota + 1
	// FieldName is dot/name syntax. Its spelling is metadata, never an
	// evaluated key occurrence.
	FieldName
	// FieldExact is bracket syntax whose key is a statically known scalar
	// occurrence and is therefore evaluated.
	FieldExact
	// FieldKey is bracket syntax whose key is an arbitrary value occurrence.
	FieldKey
)

// UnaryOp is the closed source-language unary operator vocabulary.
type UnaryOp uint8

const (
	UnaryNeg UnaryOp = iota + 1
	UnaryNot
	UnaryLen
	UnaryBitNot
)

func (op UnaryOp) valid() bool {
	return op >= UnaryNeg && op <= UnaryBitNot
}

// BinaryOp is the closed source-language binary operator vocabulary.
type BinaryOp uint8

const (
	BinaryAdd BinaryOp = iota + 1
	BinarySub
	BinaryMul
	BinaryDiv
	BinaryIDiv
	BinaryMod
	BinaryPow
	BinaryConcat
	BinaryBitAnd
	BinaryBitOr
	BinaryBitXor
	BinaryShiftLeft
	BinaryShiftRight
	BinaryEqual
	BinaryNotEqual
	BinaryLess
	BinaryLessEqual
	BinaryGreater
	BinaryGreaterEqual
)

func (op BinaryOp) valid() bool {
	return op >= BinaryAdd && op <= BinaryGreaterEqual
}

// SelectOp is Lua's short-circuit value-selection vocabulary.
type SelectOp uint8

const (
	SelectAnd SelectOp = iota + 1
	SelectOr
)

func (op SelectOp) valid() bool {
	return op == SelectAnd || op == SelectOr
}

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
	tagNormal
	tagReturn
	tagThrow
	tagYield
	tagBreak
	tagContinue
	tagBody
	tagCell
	tagRead
	tagVararg
	tagUnary
	tagBinary
	tagSelect
	tagBind
	tagAssign
	tagFunction
	tagCall
	tagBranch
	tagTable
	tagKey
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
type captureRange struct{ start, end uint32 }
type storedSpan struct {
	file                uint32
	startLine, startCol uint32
	endLine, endCol     uint32
}
type boolRow struct {
	owner Term
	value bool
}
type integerRow struct {
	owner Term
	value int64
}
type floatRow struct {
	owner Term
	bits  uint64
}
type stringRow struct {
	owner Term
	value string
}
type keyRow struct {
	owner Term
	kind  FieldKind
	exact Key
}
type valuesRow struct {
	owner Term
	fixed termRange
	tail  Term
}
type exactLensRow struct {
	owner        Term
	kind         FieldKind
	base, source Term
	exact        Key
}
type keyLensRow struct{ owner, base, key Term }
type outcomeRow struct{ owner, values Term }
type bodyRow struct {
	roots  termRange
	filled bool
}
type cellRow struct{ body Term }
type readRow struct{ owner, source Term }
type varargRow struct{ owner, cell Term }
type unaryRow struct {
	owner   Term
	op      UnaryOp
	operand Term
}
type binaryRow struct {
	owner       Term
	op          BinaryOp
	left, right Term
}
type selectRow struct {
	owner       Term
	op          SelectOp
	left, right Term
}
type bindRow struct {
	owner  Term
	cells  termRange
	values Term
}
type assignRow struct {
	owner   Term
	targets termRange
	values  Term
}
type functionRow struct {
	owner, body, vararg Term
	formals             termRange
	captures            captureRange
}
type captureRow struct{ inner, outer Term }
type callRow struct{ owner, callee, receiver, actuals Term }
type branchRow struct{ owner, condition, whenTrue, whenFalse Term }
type tableRow struct {
	owner  Term
	fields termRange
}
type tableFieldRow struct {
	key, values Term
	kind        FieldKind
	normalized  Key
}

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
type sealClaimSlot struct {
	owner Term
	count uint8
}

// Program is the immutable result of Builder.Seal.
type Program struct {
	termCount int // size metric only; no generic Term stream is retained
	files     []string
	spans     [tagCount][]storedSpan

	valueTerms  []Term
	bodyTerms   []Term
	bindTerms   []Term
	assignTerms []Term
	formalTerms []Term

	nils     []Term
	bools    []boolRow
	integers []integerRow
	floats   []floatRow
	strings  []stringRow
	values   []valuesRow

	lensExact   []exactLensRow
	lensKeys    []keyLensRow
	normals     []outcomeRow
	returns     []outcomeRow
	throws      []outcomeRow
	yields      []outcomeRow
	breaks      []outcomeRow
	continues   []outcomeRow
	muFunctions []Term // dense Seal-derived canonical heads for cyclic Functions
	directCalls []Term // Seal-derived direct Function evidence by Call index
	entry       Term   // the one canonical non-Function shard entry Body
	bodies      []bodyRow
	cells       []cellRow
	reads       []readRow
	varargs     []varargRow
	unaries     []unaryRow
	binaries    []binaryRow
	selects     []selectRow
	binds       []bindRow
	assigns     []assignRow
	functions   []functionRow
	captures    []captureRow
	calls       []callRow
	branches    []branchRow
	tables      []tableRow
	tableFields []tableFieldRow
	keys        []keyRow
	exactKeys   []exactKey
}

// Builder is the sole mutable construction path for Program.
type Builder struct {
	termCount int
	files     []string
	fileIndex map[string]uint32
	spans     [tagCount][]storedSpan
	poison    bool

	valueTerms  []Term
	bodyTerms   []Term
	bindTerms   []Term
	assignTerms []Term
	formalTerms []Term

	nils     []Term
	bools    []boolRow
	integers []integerRow
	floats   []floatRow
	strings  []stringRow
	values   []valuesRow

	lensExact   []exactLensRow
	lensKeys    []keyLensRow
	normals     []outcomeRow
	returns     []outcomeRow
	throws      []outcomeRow
	yields      []outcomeRow
	breaks      []outcomeRow
	continues   []outcomeRow
	entry       Term
	bodies      []bodyRow
	cells       []cellRow
	reads       []readRow
	varargs     []varargRow
	unaries     []unaryRow
	binaries    []binaryRow
	selects     []selectRow
	binds       []bindRow
	assigns     []assignRow
	functions   []functionRow
	captures    []captureRow
	calls       []callRow
	branches    []branchRow
	tables      []tableRow
	tableFields []tableFieldRow
	keys        []keyRow
	exactKeys   []exactKey
	exactLookup map[exactKey]Key
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
	b.termCount++
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

// boundedRange converts a slice offset and length to the persisted uint32
// half-open range. The exclusive end is allowed to equal MaxUint32; anything
// beyond it could wrap a persisted range and is rejected before mutation.
func boundedRange(start, length int) (uint32, uint32, bool) {
	if start < 0 || length < 0 {
		return 0, 0, false
	}
	first := uint64(start)
	end := first + uint64(length)
	if first > math.MaxUint32 || end > math.MaxUint32 {
		return 0, 0, false
	}
	return uint32(first), uint32(end), true
}

func (b *Builder) appendPool(pool *[]Term, terms []Term) (termRange, bool) {
	start, end, ok := boundedRange(len(*pool), len(terms))
	if !ok {
		b.poison = true
		return termRange{}, false
	}
	*pool = append(*pool, terms...)
	return termRange{start: start, end: end}, true
}

func (b *Builder) appendCaptures(captures []Capture) (captureRange, bool) {
	start, end, ok := boundedRange(len(b.captures), len(captures))
	if !ok {
		b.poison = true
		return captureRange{}, false
	}
	for _, capture := range captures {
		b.captures = append(b.captures, captureRow{inner: capture.Inner, outer: capture.Outer})
	}
	return captureRange{start: start, end: end}, true
}

func (b *Builder) require(ok bool) bool {
	if !ok {
		b.poison = true
	}
	return ok
}

// Nil, Bool, Integer, Float, and String mint a fresh literal occurrence on
// every call, preserving distinct spans and float bits.
func (b *Builder) Nil(span Span, owner Term) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	b.nils = append(b.nils, owner)
	term := b.mint(tagNil, span, b.familyIndex(len(b.nils)))
	if term == 0 {
		b.nils = b.nils[:len(b.nils)-1]
	}
	return term
}
func (b *Builder) Bool(span Span, owner Term, value bool) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	b.bools = append(b.bools, boolRow{owner: owner, value: value})
	term := b.mint(tagBool, span, b.familyIndex(len(b.bools)))
	if term == 0 {
		b.bools = b.bools[:len(b.bools)-1]
	}
	return term
}
func (b *Builder) Integer(span Span, owner Term, value int64) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	b.integers = append(b.integers, integerRow{owner: owner, value: value})
	term := b.mint(tagInteger, span, b.familyIndex(len(b.integers)))
	if term == 0 {
		b.integers = b.integers[:len(b.integers)-1]
	}
	return term
}
func (b *Builder) Float(span Span, owner Term, value float64) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	b.floats = append(b.floats, floatRow{owner: owner, bits: math.Float64bits(value)})
	term := b.mint(tagFloat, span, b.familyIndex(len(b.floats)))
	if term == 0 {
		b.floats = b.floats[:len(b.floats)-1]
	}
	return term
}
func (b *Builder) String(span Span, owner Term, value string) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	b.strings = append(b.strings, stringRow{owner: owner, value: value})
	term := b.mint(tagString, span, b.familyIndex(len(b.strings)))
	if term == 0 {
		b.strings = b.strings[:len(b.strings)-1]
	}
	return term
}

// Name and List mint static source-key identities. They retain source span and
// exact-key identity for diagnostics, but are never runtime value occurrences.
func (b *Builder) Name(span Span, owner Term, text string) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	exact := b.internExact(exactKey{kind: exactString, text: text})
	b.keys = append(b.keys, keyRow{owner: owner, kind: FieldName, exact: exact})
	term := b.mint(tagKey, span, b.familyIndex(len(b.keys)))
	if term == 0 {
		b.keys = b.keys[:len(b.keys)-1]
	}
	return term
}

func (b *Builder) List(span Span, owner Term, ordinal int64) Term {
	if !b.require(b.has(owner, tagBody) && ordinal > 0) {
		return 0
	}
	exact := b.internExact(exactKey{kind: exactInteger, int: ordinal})
	b.keys = append(b.keys, keyRow{owner: owner, kind: FieldList, exact: exact})
	term := b.mint(tagKey, span, b.familyIndex(len(b.keys)))
	if term == 0 {
		b.keys = b.keys[:len(b.keys)-1]
	}
	return term
}

// Values records an ordered fixed prefix and optional final open tail.
func (b *Builder) Values(span Span, owner Term, fixed []Term, tail Term) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	for _, value := range fixed {
		if !b.require(b.valueOccurrence(value)) {
			return 0
		}
	}
	if tail != 0 && !b.require(b.openOccurrence(tail)) {
		return 0
	}
	rangeFixed, ok := b.appendPool(&b.valueTerms, fixed)
	if !ok {
		return 0
	}
	b.values = append(b.values, valuesRow{owner: owner, fixed: rangeFixed, tail: tail})
	term := b.mint(tagValues, span, b.familyIndex(len(b.values)))
	if term == 0 {
		b.values = b.values[:len(b.values)-1]
		b.valueTerms = b.valueTerms[:rangeFixed.start]
		return 0
	}
	return term
}

// LensExact records an evaluated base plus a static nil, bool, integer, float,
// or string key occurrence. Nil and NaN remain exact source evidence but do
// not receive a storable normalized-key atom.
func (b *Builder) LensExact(span Span, owner, base, key Term, kind FieldKind) Term {
	if !b.require(b.has(owner, tagBody) && b.valueOccurrence(base)) || kind != FieldName && kind != FieldExact {
		b.poison = true
		return 0
	}
	var exact Key
	if kind == FieldName {
		if !b.require(b.has(key, tagKey) && b.keys[key.index()-1].kind == FieldName && b.keys[key.index()-1].owner == owner) {
			return 0
		}
		exact = b.keys[key.index()-1].exact
	} else {
		if !b.require(b.staticExactKey(key)) {
			return 0
		}
		exact, _ = b.normalizedExactKey(key)
	}
	b.lensExact = append(b.lensExact, exactLensRow{owner: owner, kind: kind, base: base, source: key, exact: exact})
	term := b.mint(tagLensExact, span, b.familyIndex(len(b.lensExact)))
	if term == 0 {
		b.lensExact = b.lensExact[:len(b.lensExact)-1]
		return 0
	}
	return term
}

// LensKey records base then dynamic key evaluation.
func (b *Builder) LensKey(span Span, owner, base, key Term) Term {
	if !b.require(b.has(owner, tagBody) && b.valueOccurrence(base) && b.valueOccurrence(key)) {
		return 0
	}
	b.lensKeys = append(b.lensKeys, keyLensRow{owner: owner, base: base, key: key})
	term := b.mint(tagLensKey, span, b.familyIndex(len(b.lensKeys)))
	if term == 0 {
		b.lensKeys = b.lensKeys[:len(b.lensKeys)-1]
		return 0
	}
	return term
}

func (b *Builder) Normal(span Span, owner, values Term) Term {
	return b.outcome(tagNormal, span, owner, values)
}
func (b *Builder) Return(span Span, owner, values Term) Term {
	return b.outcome(tagReturn, span, owner, values)
}
func (b *Builder) Throw(span Span, owner, values Term) Term {
	return b.outcome(tagThrow, span, owner, values)
}
func (b *Builder) Yield(span Span, owner, values Term) Term {
	return b.outcome(tagYield, span, owner, values)
}
func (b *Builder) Break(span Span, owner, values Term) Term {
	return b.outcome(tagBreak, span, owner, values)
}
func (b *Builder) Continue(span Span, owner, values Term) Term {
	return b.outcome(tagContinue, span, owner, values)
}
func (b *Builder) outcome(tag uint8, span Span, owner, value Term) Term {
	if !b.require(b.has(owner, tagBody) && b.has(value, tagValues)) {
		return 0
	}
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
	*rows = append(*rows, outcomeRow{owner: owner, values: value})
	term := b.mint(tag, span, b.familyIndex(len(*rows)))
	if term == 0 {
		*rows = (*rows)[:len(*rows)-1]
		return 0
	}
	return term
}

// Body mints an identity. SetBody must later fill its typed root set
// exactly once, after Cells, Functions, recursive Calls, and outcomes exist.
func (b *Builder) Body(span Span) Term {
	b.bodies = append(b.bodies, bodyRow{})
	term := b.mint(tagBody, span, b.familyIndex(len(b.bodies)))
	if term == 0 {
		b.bodies = b.bodies[:len(b.bodies)-1]
	}
	return term
}

func (b *Builder) SetBody(body Term, roots ...Term) bool {
	if !b.has(body, tagBody) {
		b.poison = true
		return false
	}
	row := &b.bodies[body.index()-1]
	if row.filled {
		b.poison = true
		return false
	}
	for _, root := range roots {
		if !b.require(b.statementRoot(root)) {
			return false
		}
	}
	terms, ok := b.appendPool(&b.bodyTerms, roots)
	if !ok {
		return false
	}
	row.roots = terms
	row.filled = true
	return true
}

// SetEntry fixes the shard's one canonical top-level Body. It may be called
// exactly once; Seal verifies that the selected Body is neither nested nor a
// Function body.
func (b *Builder) SetEntry(body Term) bool {
	if !b.has(body, tagBody) || b.entry != 0 {
		b.poison = true
		return false
	}
	b.entry = body
	return true
}

// Cell is lexical storage owned by one Body. Read observes a Cell or a Lens.
func (b *Builder) Cell(span Span, body Term) Term {
	if !b.require(b.has(body, tagBody)) {
		return 0
	}
	b.cells = append(b.cells, cellRow{body: body})
	term := b.mint(tagCell, span, b.familyIndex(len(b.cells)))
	if term == 0 {
		b.cells = b.cells[:len(b.cells)-1]
	}
	return term
}

// Read observes a lexical Cell or an evaluated Lens. Reading a Cell has no
// evaluation predecessor; reading a Lens evaluates that Lens exactly once.
func (b *Builder) Read(span Span, owner, source Term) Term {
	if !b.require(b.has(owner, tagBody) && (b.has(source, tagCell) || b.has(source, tagLensExact) || b.has(source, tagLensKey))) {
		return 0
	}
	b.reads = append(b.reads, readRow{owner: owner, source: source})
	term := b.mint(tagRead, span, b.familyIndex(len(b.reads)))
	if term == 0 {
		b.reads = b.reads[:len(b.reads)-1]
		return 0
	}
	return term
}

// Vararg records one open source occurrence anchored to a Function's vararg
// Cell. It is an occurrence, not a scalar Cell read, so Values may retain it
// as its final open tail.
func (b *Builder) Vararg(span Span, owner, cell Term) Term {
	if !b.require(b.has(owner, tagBody) && b.has(cell, tagCell)) {
		return 0
	}
	b.varargs = append(b.varargs, varargRow{owner: owner, cell: cell})
	term := b.mint(tagVararg, span, b.familyIndex(len(b.varargs)))
	if term == 0 {
		b.varargs = b.varargs[:len(b.varargs)-1]
		return 0
	}
	return term
}

// Unary records one closed unary scalar operation.
func (b *Builder) Unary(span Span, owner Term, op UnaryOp, operand Term) Term {
	if !b.require(b.has(owner, tagBody) && op.valid() && b.valueOccurrence(operand)) {
		return 0
	}
	b.unaries = append(b.unaries, unaryRow{owner: owner, op: op, operand: operand})
	term := b.mint(tagUnary, span, b.familyIndex(len(b.unaries)))
	if term == 0 {
		b.unaries = b.unaries[:len(b.unaries)-1]
		return 0
	}
	return term
}

// Binary records one closed left-to-right binary scalar operation.
func (b *Builder) Binary(span Span, owner Term, op BinaryOp, left, right Term) Term {
	if !b.require(b.has(owner, tagBody) && op.valid() && b.valueOccurrence(left) && b.valueOccurrence(right)) {
		return 0
	}
	b.binaries = append(b.binaries, binaryRow{owner: owner, op: op, left: left, right: right})
	term := b.mint(tagBinary, span, b.familyIndex(len(b.binaries)))
	if term == 0 {
		b.binaries = b.binaries[:len(b.binaries)-1]
		return 0
	}
	return term
}

// Select records Lua's lazy and/or value selection. The left operand always
// evaluates first; the right operand is retained in the row and evaluates only
// when the selected operator's truthiness rule demands it.
func (b *Builder) Select(span Span, owner Term, op SelectOp, left, right Term) Term {
	if !b.require(b.has(owner, tagBody) && op.valid() && b.valueOccurrence(left) && b.valueOccurrence(right)) {
		return 0
	}
	b.selects = append(b.selects, selectRow{owner: owner, op: op, left: left, right: right})
	term := b.mint(tagSelect, span, b.familyIndex(len(b.selects)))
	if term == 0 {
		b.selects = b.selects[:len(b.selects)-1]
		return 0
	}
	return term
}

// Bind initializes lexical Cells. Cell identities are static; its sole
// evaluated child is the RHS Values relation.
func (b *Builder) Bind(span Span, owner Term, cells []Term, values Term) Term {
	if !b.require(b.has(owner, tagBody) && len(cells) != 0 && b.has(values, tagValues)) {
		return 0
	}
	for _, cell := range cells {
		if !b.require(b.has(cell, tagCell)) {
			return 0
		}
	}
	r, ok := b.appendPool(&b.bindTerms, cells)
	if !ok {
		return 0
	}
	b.binds = append(b.binds, bindRow{owner: owner, cells: r, values: values})
	term := b.mint(tagBind, span, b.familyIndex(len(b.binds)))
	if term == 0 {
		b.binds = b.binds[:len(b.binds)-1]
		b.bindTerms = b.bindTerms[:r.start]
		return 0
	}
	return term
}

// Assign evaluates its Cells/Lenses from left to right, then RHS Values. The
// typed row represents the delayed commit after those operands.
func (b *Builder) Assign(span Span, owner Term, targets []Term, values Term) Term {
	if !b.require(b.has(owner, tagBody) && len(targets) != 0 && b.has(values, tagValues)) {
		return 0
	}
	for _, target := range targets {
		if !b.require(b.has(target, tagCell) || b.has(target, tagLensExact) || b.has(target, tagLensKey)) {
			return 0
		}
	}
	targetRange, ok := b.appendPool(&b.assignTerms, targets)
	if !ok {
		return 0
	}
	b.assigns = append(b.assigns, assignRow{owner: owner, targets: targetRange, values: values})
	term := b.mint(tagAssign, span, b.familyIndex(len(b.assigns)))
	if term == 0 {
		b.assigns = b.assigns[:len(b.assigns)-1]
		b.assignTerms = b.assignTerms[:targetRange.start]
		return 0
	}
	return term
}

// Function records a closure with its lexical owner, execution Body, ordered
// formal Cells, optional vararg Cell, and complete lexical capture pairs.
func (b *Builder) Function(span Span, owner, body Term, formals []Term, vararg Term, captures []Capture) Term {
	if !b.require(b.has(owner, tagBody) && b.has(body, tagBody) && owner != body) {
		return 0
	}
	if vararg != 0 && !b.require(b.has(vararg, tagCell)) {
		return 0
	}
	for _, formal := range formals {
		if !b.require(b.has(formal, tagCell)) {
			return 0
		}
	}
	for _, capture := range captures {
		if !b.require(b.has(capture.Inner, tagCell) && b.has(capture.Outer, tagCell)) {
			return 0
		}
	}
	r, ok := b.appendPool(&b.formalTerms, formals)
	if !ok {
		return 0
	}
	c, ok := b.appendCaptures(captures)
	if !ok {
		b.formalTerms = b.formalTerms[:r.start]
		return 0
	}
	b.functions = append(b.functions, functionRow{owner: owner, body: body, vararg: vararg, formals: r, captures: c})
	term := b.mint(tagFunction, span, b.familyIndex(len(b.functions)))
	if term == 0 {
		b.functions = b.functions[:len(b.functions)-1]
		b.formalTerms = b.formalTerms[:r.start]
		b.captures = b.captures[:c.start]
	}
	return term
}

// Call is an open result producer. Evaluation is callee then actual Values.
// receiver is an optional semantic correspondence for method syntax, not an
// extra evaluation child: a method callee is Read(Lens(receiver, key)), which
// already evaluates receiver exactly once. Seal derives any coherent direct
// Function evidence from lexical bindings; it never replaces the callee
// occurrence.
func (b *Builder) Call(span Span, owner, callee, receiver, actuals Term) Term {
	if !b.require(b.has(owner, tagBody) && b.valueOccurrence(callee) && b.has(actuals, tagValues)) {
		return 0
	}
	if receiver != 0 && !b.require(b.valueOccurrence(receiver)) {
		return 0
	}
	b.calls = append(b.calls, callRow{owner: owner, callee: callee, receiver: receiver, actuals: actuals})
	term := b.mint(tagCall, span, b.familyIndex(len(b.calls)))
	if term == 0 {
		b.calls = b.calls[:len(b.calls)-1]
		return 0
	}
	return term
}

// Branch evaluates condition and then transfers to exactly one owned Body or Outcome.
func (b *Builder) Branch(span Span, owner, condition, whenTrue, whenFalse Term) Term {
	if !b.require(b.has(owner, tagBody) && b.valueOccurrence(condition) && b.branchArm(whenTrue) && b.branchArm(whenFalse)) {
		return 0
	}
	b.branches = append(b.branches, branchRow{owner: owner, condition: condition, whenTrue: whenTrue, whenFalse: whenFalse})
	term := b.mint(tagBranch, span, b.familyIndex(len(b.branches)))
	if term == 0 {
		b.branches = b.branches[:len(b.branches)-1]
		return 0
	}
	return term
}

// Table mints a complete constructor in one operation. Fields are supplied
// before the allocation identity exists, which makes occurrence containment
// acyclic without a global graph or a second construction phase.
func (b *Builder) Table(span Span, owner Term, keys, values []Term, kinds []FieldKind) Term {
	if !b.require(b.has(owner, tagBody)) || len(keys) != len(values) || len(keys) != len(kinds) {
		b.poison = true
		return 0
	}
	fieldStart := len(b.tableFields)
	start, end, ok := boundedRange(fieldStart, len(keys))
	if !ok {
		b.poison = true
		return 0
	}
	listOrdinal := int64(0)
	for i := range keys {
		if !b.require(b.has(values[i], tagValues)) {
			b.tableFields = b.tableFields[:fieldStart]
			return 0
		}
		fieldValues := b.values[values[i].index()-1]
		fixed := fieldValues.fixed.end - fieldValues.fixed.start
		open := fieldValues.tail != 0
		lastList := i == len(keys)-1 && kinds[i] == FieldList
		if !(fixed == 1 && !open) && !(lastList && fixed == 0 && open) {
			b.poison = true
			b.tableFields = b.tableFields[:fieldStart]
			return 0
		}
		field := tableFieldRow{key: keys[i], values: values[i], kind: kinds[i]}
		switch kinds[i] {
		case FieldList:
			listOrdinal++
			if !b.require(b.has(keys[i], tagKey) && b.keys[keys[i].index()-1].kind == FieldList && b.keys[keys[i].index()-1].owner == owner && b.keys[keys[i].index()-1].exact != 0 && b.exactKeys[b.keys[keys[i].index()-1].exact-1] == (exactKey{kind: exactInteger, int: listOrdinal})) {
				b.tableFields = b.tableFields[:fieldStart]
				return 0
			}
			field.normalized = b.keys[keys[i].index()-1].exact
		case FieldName:
			if !b.require(b.has(keys[i], tagKey) && b.keys[keys[i].index()-1].kind == FieldName && b.keys[keys[i].index()-1].owner == owner) {
				b.tableFields = b.tableFields[:fieldStart]
				return 0
			}
			field.normalized = b.keys[keys[i].index()-1].exact
		case FieldExact:
			if !b.require(b.staticExactKey(keys[i])) {
				b.tableFields = b.tableFields[:fieldStart]
				return 0
			}
			field.normalized, _ = b.normalizedExactKey(keys[i])
		case FieldKey:
			if !b.require(b.valueOccurrence(keys[i])) {
				b.tableFields = b.tableFields[:fieldStart]
				return 0
			}
		default:
			b.poison = true
			b.tableFields = b.tableFields[:fieldStart]
			return 0
		}
		b.tableFields = append(b.tableFields, field)
	}
	fields := termRange{start: start, end: end}
	b.tables = append(b.tables, tableRow{owner: owner, fields: fields})
	term := b.mint(tagTable, span, b.familyIndex(len(b.tables)))
	if term == 0 {
		b.tables = b.tables[:len(b.tables)-1]
		b.tableFields = b.tableFields[:fieldStart]
	}
	return term
}

func (b *Builder) normalizedExactKey(term Term) (Key, bool) {
	key, ok := b.exactKey(term)
	if !ok {
		return 0, false
	}
	return b.internExact(key), true
}

func (b *Builder) internExact(key exactKey) Key {
	if index := b.exactLookup[key]; index != 0 {
		return index
	}
	index, ok := exactKeyIndex(len(b.exactKeys))
	if !ok {
		b.poison = true
		return 0
	}
	if b.exactLookup == nil {
		b.exactLookup = make(map[exactKey]Key)
	}
	b.exactKeys = append(b.exactKeys, key)
	b.exactLookup[key] = index
	return index
}

// exactKeyIndex reserves Key(0) for dynamic, nil, and NaN keys. Unlike a
// Term family index, an interned exact Key has the full nonzero uint32 space.
func exactKeyIndex(length int) (Key, bool) {
	if length < 0 || uint64(length) >= uint64(math.MaxUint32) {
		return 0, false
	}
	return Key(uint64(length) + 1), true
}
func (b *Builder) exactKey(term Term) (exactKey, bool) {
	if !b.valid(term) {
		return exactKey{}, false
	}
	switch term.tag() {
	case tagBool:
		return exactKey{kind: exactBool, bool: b.bools[term.index()-1].value}, true
	case tagInteger:
		return exactKey{kind: exactInteger, int: b.integers[term.index()-1].value}, true
	case tagFloat:
		return normalizeFloat(b.floats[term.index()-1].bits)
	case tagString:
		return exactKey{kind: exactString, text: b.strings[term.index()-1].value}, true
	}
	return exactKey{}, false
}

func (b *Builder) staticExactKey(term Term) bool {
	if !b.valid(term) {
		return false
	}
	switch term.tag() {
	case tagNil, tagBool, tagInteger, tagFloat, tagString:
		return true
	default:
		return false
	}
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
		return index <= uint32(len(b.nils))
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
	case tagBody:
		return index <= uint32(len(b.bodies))
	case tagCell:
		return index <= uint32(len(b.cells))
	case tagRead:
		return index <= uint32(len(b.reads))
	case tagVararg:
		return index <= uint32(len(b.varargs))
	case tagUnary:
		return index <= uint32(len(b.unaries))
	case tagBinary:
		return index <= uint32(len(b.binaries))
	case tagSelect:
		return index <= uint32(len(b.selects))
	case tagBind:
		return index <= uint32(len(b.binds))
	case tagAssign:
		return index <= uint32(len(b.assigns))
	case tagFunction:
		return index <= uint32(len(b.functions))
	case tagCall:
		return index <= uint32(len(b.calls))
	case tagBranch:
		return index <= uint32(len(b.branches))
	case tagTable:
		return index <= uint32(len(b.tables))
	case tagKey:
		return index <= uint32(len(b.keys))
	}
	return false
}
func (b *Builder) has(term Term, tag uint8) bool { return term.tag() == tag && b.valid(term) }

// valueOccurrence is the closed source-value family. It deliberately excludes
// storage, address, tuple, control, and declaration relations.
func (b *Builder) valueOccurrence(term Term) bool {
	if !b.valid(term) {
		return false
	}
	switch term.tag() {
	case tagNil, tagBool, tagInteger, tagFloat, tagString, tagRead, tagVararg,
		tagUnary, tagBinary, tagSelect, tagFunction, tagCall, tagTable:
		return true
	default:
		return false
	}
}

func (b *Builder) openOccurrence(term Term) bool {
	return b.has(term, tagCall) || b.has(term, tagVararg)
}

// Seal proves exactly-one typed containment, lexical Body authority, Cell
// roles, and direct-call recurrence. Its dense proof ledgers are transient;
// the returned Program contains immutable typed rows.
func (b *Builder) Seal() (*Program, error) {
	if b == nil {
		return nil, errors.New("program: nil builder")
	}
	if b.poison {
		return nil, errors.New("program: poisoned builder")
	}
	if !b.has(b.entry, tagBody) {
		return nil, errors.New("program: missing Entry Body")
	}
	for _, row := range b.bodies {
		if !row.filled {
			return nil, errors.New("program: Body was not filled")
		}
	}

	// A Body has exactly one lexical authority: its parent Body root, enclosing
	// Function, or Branch arm. Branch arms are never roots as well.
	parent := make([]uint32, len(b.bodies)+1)
	setParent := func(child, owner Term) error {
		if !b.has(child, tagBody) || !b.has(owner, tagBody) || child == owner || parent[child.index()] != 0 {
			return errors.New("program: Body has ambiguous structural authority")
		}
		parent[child.index()] = owner.index()
		return nil
	}
	for i, row := range b.bodies {
		body := makeTerm(tagBody, uint32(i+1))
		for j := row.roots.start; j < row.roots.end; j++ {
			root := b.bodyTerms[j]
			if !b.statementRoot(root) {
				return nil, errors.New("program: Body requires statement roots")
			}
			if b.has(root, tagBody) {
				if err := setParent(root, body); err != nil {
					return nil, err
				}
			}
		}
	}
	functionAtBody := make([]Term, len(b.bodies)+1)
	for i, row := range b.functions {
		function := makeTerm(tagFunction, uint32(i+1))
		if !b.has(row.owner, tagBody) || !b.has(row.body, tagBody) || row.owner == row.body || functionAtBody[row.body.index()] != 0 {
			return nil, errors.New("program: invalid Function Body authority")
		}
		functionAtBody[row.body.index()] = function
		if err := setParent(row.body, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.branches {
		for _, arm := range [...]Term{row.whenTrue, row.whenFalse} {
			if b.has(arm, tagBody) {
				if err := setParent(arm, row.owner); err != nil {
					return nil, err
				}
			}
		}
	}
	if parent[b.entry.index()] != 0 || functionAtBody[b.entry.index()] != 0 || !entryBodyForest(parent, b.entry.index()) {
		return nil, errors.New("program: invalid Body forest")
	}
	pre, post := bodyIntervals(parent)
	activation := b.bodyActivations(parent, functionAtBody, b.entry)
	if activation == nil {
		return nil, errors.New("program: invalid Body activation")
	}

	claims := [tagCount][]sealClaimSlot{}
	claims[tagNil] = make([]sealClaimSlot, len(b.nils))
	for i, owner := range b.nils {
		claims[tagNil][i].owner = owner
	}
	claims[tagBool] = make([]sealClaimSlot, len(b.bools))
	for i, row := range b.bools {
		claims[tagBool][i].owner = row.owner
	}
	claims[tagInteger] = make([]sealClaimSlot, len(b.integers))
	for i, row := range b.integers {
		claims[tagInteger][i].owner = row.owner
	}
	claims[tagFloat] = make([]sealClaimSlot, len(b.floats))
	for i, row := range b.floats {
		claims[tagFloat][i].owner = row.owner
	}
	claims[tagString] = make([]sealClaimSlot, len(b.strings))
	for i, row := range b.strings {
		claims[tagString][i].owner = row.owner
	}
	claims[tagValues] = make([]sealClaimSlot, len(b.values))
	for i, row := range b.values {
		claims[tagValues][i].owner = row.owner
	}
	claims[tagLensExact] = make([]sealClaimSlot, len(b.lensExact))
	for i, row := range b.lensExact {
		claims[tagLensExact][i].owner = row.owner
	}
	claims[tagLensKey] = make([]sealClaimSlot, len(b.lensKeys))
	for i, row := range b.lensKeys {
		claims[tagLensKey][i].owner = row.owner
	}
	claims[tagNormal] = make([]sealClaimSlot, len(b.normals))
	for i, row := range b.normals {
		claims[tagNormal][i].owner = row.owner
	}
	claims[tagReturn] = make([]sealClaimSlot, len(b.returns))
	for i, row := range b.returns {
		claims[tagReturn][i].owner = row.owner
	}
	claims[tagThrow] = make([]sealClaimSlot, len(b.throws))
	for i, row := range b.throws {
		claims[tagThrow][i].owner = row.owner
	}
	claims[tagYield] = make([]sealClaimSlot, len(b.yields))
	for i, row := range b.yields {
		claims[tagYield][i].owner = row.owner
	}
	claims[tagBreak] = make([]sealClaimSlot, len(b.breaks))
	for i, row := range b.breaks {
		claims[tagBreak][i].owner = row.owner
	}
	claims[tagContinue] = make([]sealClaimSlot, len(b.continues))
	for i, row := range b.continues {
		claims[tagContinue][i].owner = row.owner
	}
	claims[tagRead] = make([]sealClaimSlot, len(b.reads))
	for i, row := range b.reads {
		claims[tagRead][i].owner = row.owner
	}
	claims[tagVararg] = make([]sealClaimSlot, len(b.varargs))
	for i, row := range b.varargs {
		claims[tagVararg][i].owner = row.owner
	}
	claims[tagUnary] = make([]sealClaimSlot, len(b.unaries))
	for i, row := range b.unaries {
		claims[tagUnary][i].owner = row.owner
	}
	claims[tagBinary] = make([]sealClaimSlot, len(b.binaries))
	for i, row := range b.binaries {
		claims[tagBinary][i].owner = row.owner
	}
	claims[tagSelect] = make([]sealClaimSlot, len(b.selects))
	for i, row := range b.selects {
		claims[tagSelect][i].owner = row.owner
	}
	claims[tagBind] = make([]sealClaimSlot, len(b.binds))
	for i, row := range b.binds {
		claims[tagBind][i].owner = row.owner
	}
	claims[tagAssign] = make([]sealClaimSlot, len(b.assigns))
	for i, row := range b.assigns {
		claims[tagAssign][i].owner = row.owner
	}
	claims[tagFunction] = make([]sealClaimSlot, len(b.functions))
	for i, row := range b.functions {
		claims[tagFunction][i].owner = row.owner
	}
	claims[tagCall] = make([]sealClaimSlot, len(b.calls))
	for i, row := range b.calls {
		claims[tagCall][i].owner = row.owner
	}
	claims[tagBranch] = make([]sealClaimSlot, len(b.branches))
	for i, row := range b.branches {
		claims[tagBranch][i].owner = row.owner
	}
	claims[tagTable] = make([]sealClaimSlot, len(b.tables))
	for i, row := range b.tables {
		claims[tagTable][i].owner = row.owner
	}
	claims[tagKey] = make([]sealClaimSlot, len(b.keys))
	for i, row := range b.keys {
		claims[tagKey][i].owner = row.owner
	}
	claim := func(child, owner Term) error {
		if !b.valid(child) || !b.has(owner, tagBody) || child.tag() >= tagCount || int(child.index()) > len(claims[child.tag()]) {
			return errors.New("program: invalid typed containment")
		}
		at := &claims[child.tag()][child.index()-1]
		if at.owner != owner || at.count != 0 {
			return errors.New("program: invalid typed containment")
		}
		at.count = 1
		return nil
	}
	claimValues := func(owner Term, row valuesRow) error {
		for i := row.fixed.start; i < row.fixed.end; i++ {
			if !b.valueOccurrence(b.valueTerms[i]) {
				return errors.New("program: invalid Values value")
			}
			if err := claim(b.valueTerms[i], owner); err != nil {
				return err
			}
		}
		if row.tail != 0 {
			if !b.openOccurrence(row.tail) {
				return errors.New("program: invalid Values tail")
			}
			return claim(row.tail, owner)
		}
		return nil
	}
	for i, row := range b.bodies {
		owner := makeTerm(tagBody, uint32(i+1))
		for j := row.roots.start; j < row.roots.end; j++ {
			root := b.bodyTerms[j]
			if b.has(root, tagBody) {
				continue
			}
			if err := claim(root, owner); err != nil {
				return nil, err
			}
		}
	}
	for _, row := range b.values {
		if err := claimValues(row.owner, row); err != nil {
			return nil, err
		}
	}
	for _, row := range b.lensExact {
		if !b.valueOccurrence(row.base) || !b.staticExactKey(row.source) {
			if row.kind != FieldName || !b.has(row.source, tagKey) || b.keys[row.source.index()-1].kind != FieldName || b.keys[row.source.index()-1].owner != row.owner {
				return nil, errors.New("program: invalid exact Lens")
			}
		}
		if err := claim(row.base, row.owner); err != nil {
			return nil, err
		}
		if row.kind == FieldExact || row.kind == FieldName {
			if err := claim(row.source, row.owner); err != nil {
				return nil, err
			}
		}
	}
	for _, row := range b.lensKeys {
		if err := claim(row.base, row.owner); err != nil {
			return nil, err
		}
		if err := claim(row.key, row.owner); err != nil {
			return nil, err
		}
	}
	for _, rows := range [][]outcomeRow{b.normals, b.returns, b.throws, b.yields, b.breaks, b.continues} {
		for _, row := range rows {
			if !b.has(row.values, tagValues) {
				return nil, errors.New("program: Outcome requires Values")
			}
			if err := claim(row.values, row.owner); err != nil {
				return nil, err
			}
		}
	}
	for _, row := range b.reads {
		if !b.has(row.source, tagCell) && !b.has(row.source, tagLensExact) && !b.has(row.source, tagLensKey) {
			return nil, errors.New("program: invalid Read")
		}
		if !b.has(row.source, tagCell) {
			if err := claim(row.source, row.owner); err != nil {
				return nil, err
			}
		}
	}
	for _, row := range b.unaries {
		if !row.op.valid() {
			return nil, errors.New("program: invalid Unary")
		}
		if err := claim(row.operand, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.binaries {
		if !row.op.valid() {
			return nil, errors.New("program: invalid Binary")
		}
		if err := claim(row.left, row.owner); err != nil {
			return nil, err
		}
		if err := claim(row.right, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.selects {
		if !row.op.valid() {
			return nil, errors.New("program: invalid Select")
		}
		if err := claim(row.left, row.owner); err != nil {
			return nil, err
		}
		if err := claim(row.right, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.binds {
		if !b.has(row.values, tagValues) {
			return nil, errors.New("program: invalid Bind")
		}
		if err := claim(row.values, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.assigns {
		if !b.has(row.values, tagValues) {
			return nil, errors.New("program: invalid Assign")
		}
		for i := row.targets.start; i < row.targets.end; i++ {
			target := b.assignTerms[i]
			if b.has(target, tagLensExact) || b.has(target, tagLensKey) {
				if err := claim(target, row.owner); err != nil {
					return nil, err
				}
			}
		}
		if err := claim(row.values, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.calls {
		if err := claim(row.callee, row.owner); err != nil {
			return nil, err
		}
		if err := claim(row.actuals, row.owner); err != nil {
			return nil, err
		}
	}
	for _, row := range b.branches {
		if err := claim(row.condition, row.owner); err != nil {
			return nil, err
		}
		for _, arm := range [...]Term{row.whenTrue, row.whenFalse} {
			if !b.branchArm(arm) {
				return nil, errors.New("program: invalid Branch arm")
			}
			if !b.has(arm, tagBody) {
				if err := claim(arm, row.owner); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, row := range b.tables {
		for i := row.fields.start; i < row.fields.end; i++ {
			field := b.tableFields[i]
			if field.kind == FieldName || field.kind == FieldList {
				if !b.has(field.key, tagKey) || b.keys[field.key.index()-1].kind != field.kind || b.keys[field.key.index()-1].owner != row.owner {
					return nil, errors.New("program: invalid static Table key")
				}
				if err := claim(field.key, row.owner); err != nil {
					return nil, err
				}
			} else if err := claim(field.key, row.owner); err != nil {
				return nil, err
			}
			if err := claim(field.values, row.owner); err != nil {
				return nil, err
			}
		}
	}
	allClaimed := func(slots []sealClaimSlot) bool {
		for _, slot := range slots {
			if slot.count != 1 {
				return false
			}
		}
		return true
	}
	for _, slots := range claims {
		if !allClaimed(slots) {
			return nil, errors.New("program: unclaimed source term")
		}
	}

	direct := make([]Term, len(b.calls))
	if err := b.validateCells(pre, post, activation, direct); err != nil {
		return nil, err
	}
	mu, err := b.directCallMu(activation, direct)
	if err != nil {
		return nil, err
	}
	return b.snapshot(mu, direct), nil
}

func (b *Builder) bodyActivations(parent []uint32, functionAtBody []Term, entry Term) []Term {
	count := len(parent) - 1
	start := make([]uint32, count+2)
	for child := uint32(1); int(child) < len(parent); child++ {
		if parent[child] != 0 {
			start[parent[child]+1]++
		}
	}
	for i := 1; i < len(start); i++ {
		start[i] += start[i-1]
	}
	next := append([]uint32(nil), start[:count+1]...)
	children := make([]uint32, count-1)
	for child := uint32(1); int(child) < len(parent); child++ {
		at := parent[child]
		if at != 0 {
			children[next[at]] = child
			next[at]++
		}
	}
	activation := make([]Term, len(parent))
	stack := make([]uint32, 1, len(parent)-1)
	stack[0] = entry.index()
	seen := 0
	for len(stack) != 0 {
		body := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		seen++
		for i := start[body]; i < start[body+1]; i++ {
			child := children[i]
			activation[child] = activation[body]
			if functionAtBody[child] != 0 {
				activation[child] = functionAtBody[child]
			}
			stack = append(stack, child)
		}
	}
	if seen != len(parent)-1 {
		return nil
	}
	return activation
}

func (b *Builder) validateCells(pre, post []uint32, activation []Term, direct []Term) error {
	visible := func(owner, cellBody Term) bool {
		return b.has(owner, tagBody) && b.has(cellBody, tagBody) && activation[owner.index()] == activation[cellBody.index()] && pre[cellBody.index()] <= pre[owner.index()] && post[owner.index()] <= post[cellBody.index()]
	}
	for _, cell := range b.cells {
		if !b.has(cell.body, tagBody) {
			return errors.New("program: Cell requires Body")
		}
	}
	roles := make([]uint8, len(b.cells)+1)
	for _, row := range b.binds {
		if row.cells.start == row.cells.end {
			return errors.New("program: Bind requires Cells")
		}
		for i := row.cells.start; i < row.cells.end; i++ {
			cell := b.bindTerms[i]
			if !b.has(cell, tagCell) || b.cells[cell.index()-1].body != row.owner || roles[cell.index()] != 0 {
				return errors.New("program: invalid Bind Cell role")
			}
			roles[cell.index()] = 1
		}
	}
	for _, row := range b.reads {
		if b.has(row.source, tagCell) && !visible(row.owner, b.cells[row.source.index()-1].body) {
			return errors.New("program: Read Cell is not lexically visible")
		}
	}
	for _, row := range b.assigns {
		for i := row.targets.start; i < row.targets.end; i++ {
			target := b.assignTerms[i]
			if b.has(target, tagCell) && !visible(row.owner, b.cells[target.index()-1].body) {
				return errors.New("program: Assign Cell is not lexically visible")
			}
		}
	}

	bindingFunction := make([]Term, len(b.cells)+1)
	unstableBinding := make([]bool, len(b.cells)+1)
	varargFunction := make([]Term, len(b.cells)+1)
	captureOuter := make([]Term, len(b.cells)+1)
	captureOuterSeen := make([]uint32, len(b.cells)+1)
	for functionIndex, row := range b.functions {
		function := makeTerm(tagFunction, uint32(functionIndex+1))
		for i := row.formals.start; i < row.formals.end; i++ {
			cell := b.formalTerms[i]
			if !b.has(cell, tagCell) || b.cells[cell.index()-1].body != row.body || roles[cell.index()] != 0 {
				return errors.New("program: invalid formal Cell role")
			}
			roles[cell.index()] = 1
		}
		if row.vararg != 0 {
			cell := row.vararg
			if !b.has(cell, tagCell) || b.cells[cell.index()-1].body != row.body || roles[cell.index()] != 0 {
				return errors.New("program: invalid vararg Cell role")
			}
			roles[cell.index()] = 1
			varargFunction[cell.index()] = function
		}
		for i := row.captures.start; i < row.captures.end; i++ {
			rowCapture := b.captures[i]
			if !b.has(rowCapture.inner, tagCell) || !b.has(rowCapture.outer, tagCell) || b.cells[rowCapture.inner.index()-1].body != row.body || !visible(row.owner, b.cells[rowCapture.outer.index()-1].body) || roles[rowCapture.inner.index()] != 0 {
				return errors.New("program: invalid lexical Capture")
			}
			if captureOuterSeen[rowCapture.outer.index()] == uint32(functionIndex+1) {
				return errors.New("program: duplicate Function capture outer")
			}
			captureOuterSeen[rowCapture.outer.index()] = uint32(functionIndex + 1)
			roles[rowCapture.inner.index()] = 1
			captureOuter[rowCapture.inner.index()] = rowCapture.outer
		}
	}
	for _, row := range b.binds {
		values := b.values[row.values.index()-1]
		limit := row.cells.end - row.cells.start
		if fixed := values.fixed.end - values.fixed.start; fixed < limit {
			limit = fixed
		}
		for i := uint32(0); i < limit; i++ {
			function := b.valueTerms[values.fixed.start+i]
			if !b.has(function, tagFunction) {
				continue
			}
			cell := b.bindTerms[row.cells.start+i]
			if bindingFunction[cell.index()] != 0 {
				return errors.New("program: Function has duplicate binding")
			}
			bindingFunction[cell.index()] = function
		}
	}
	for cell := 1; cell <= len(b.cells); cell++ {
		if roles[cell] != 1 {
			return errors.New("program: Cell needs exactly one definition role")
		}
	}
	terminal := terminalCaptureCells(captureOuter)
	for _, row := range b.assigns {
		for i := row.targets.start; i < row.targets.end; i++ {
			target := b.assignTerms[i]
			if !b.has(target, tagCell) {
				continue
			}
			base := terminal[target.index()]
			if base != 0 && bindingFunction[base.index()] != 0 {
				unstableBinding[base.index()] = true
			}
		}
	}
	for _, row := range b.varargs {
		if !b.has(row.cell, tagCell) || varargFunction[row.cell.index()] == 0 || !visible(row.owner, b.cells[row.cell.index()-1].body) {
			return errors.New("program: invalid Vararg")
		}
	}
	for callIndex, row := range b.calls {
		if !b.valueOccurrence(row.callee) || !b.has(row.actuals, tagValues) {
			return errors.New("program: invalid Call")
		}
		if row.receiver != 0 {
			if !b.has(row.callee, tagRead) {
				return errors.New("program: method Call requires Read")
			}
			lens := b.reads[row.callee.index()-1].source
			if !b.has(lens, tagLensExact) {
				return errors.New("program: method Call requires name Lens")
			}
			lr := b.lensExact[lens.index()-1]
			if lr.kind != FieldName || lr.base != row.receiver || !b.has(lr.source, tagKey) || b.keys[lr.source.index()-1].kind != FieldName {
				return errors.New("program: method receiver mismatch")
			}
		}
		expected := Term(0)
		if b.has(row.callee, tagFunction) {
			expected = row.callee
		} else if b.has(row.callee, tagRead) && b.has(b.reads[row.callee.index()-1].source, tagCell) {
			cell := b.reads[row.callee.index()-1].source
			base := terminal[cell.index()]
			if base != 0 && !unstableBinding[base.index()] {
				expected = bindingFunction[base.index()]
			}
		}
		direct[callIndex] = expected
	}
	return nil
}

// entryBodyForest verifies the one-root lexical Body forest iteratively. Every
// non-entry Body must reach entry through exactly one already-chosen parent.
func entryBodyForest(parent []uint32, entry uint32) bool {
	if entry == 0 || int(entry) >= len(parent) || parent[entry] != 0 {
		return false
	}
	state := make([]uint8, len(parent))
	state[entry] = 2
	for start := uint32(1); int(start) < len(parent); start++ {
		if state[start] == 2 {
			continue
		}
		node := start
		for node != entry && node != 0 && int(node) < len(parent) && state[node] == 0 {
			state[node] = 1
			node = parent[node]
		}
		if node != entry && (node == 0 || int(node) >= len(parent) || state[node] != 2) {
			return false
		}
		node = start
		for node != entry && state[node] == 1 {
			state[node] = 2
			node = parent[node]
		}
	}
	return true
}

// bodyIntervals returns iterative DFS intervals for strict lexical-ancestor
// checks in a previously verified one-root Body forest.
func bodyIntervals(parent []uint32) (pre, post []uint32) {
	count := len(parent) - 1
	start := make([]uint32, count+2)
	for child := 1; child <= count; child++ {
		if parent[child] != 0 {
			start[parent[child]+1]++
		}
	}
	for i := 1; i < len(start); i++ {
		start[i] += start[i-1]
	}
	next := append([]uint32(nil), start[:count+1]...)
	children := make([]uint32, count-1)
	for child := 1; child <= count; child++ {
		parent := parent[child]
		if parent == 0 {
			continue
		}
		children[next[parent]] = uint32(child)
		next[parent]++
	}
	pre = make([]uint32, count+1)
	post = make([]uint32, count+1)
	root := uint32(1)
	for root <= uint32(count) && parent[root] != 0 {
		root++
	}
	type frame struct{ body, next uint32 }
	clock := uint32(0)
	stack := make([]frame, 0, count)
	clock++
	pre[root] = clock
	stack = append(stack, frame{body: root, next: start[root]})
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		if top.next < start[top.body+1] {
			child := children[top.next]
			top.next++
			clock++
			pre[child] = clock
			stack = append(stack, frame{body: child, next: start[child]})
			continue
		}
		post[top.body] = clock
		stack = stack[:len(stack)-1]
	}
	return pre, post
}

// terminalCaptureCells collapses the validated inner-to-outer capture forest
// once, so direct-call coherence is O(1) per Read after O(number of Cells).
func terminalCaptureCells(outer []Term) []Term {
	terminal := make([]Term, len(outer))
	stack := make([]uint32, 0, len(outer))
	for start := uint32(1); int(start) < len(outer); start++ {
		if terminal[start] != 0 {
			continue
		}
		node := start
		for terminal[node] == 0 && outer[node] != 0 {
			stack = append(stack, node)
			node = outer[node].index()
		}
		base := terminal[node]
		if base == 0 {
			base = makeTerm(tagCell, node)
			terminal[node] = base
		}
		for len(stack) != 0 {
			node = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			terminal[node] = base
		}
	}
	return terminal
}

func (b *Builder) statementRoot(term Term) bool {
	return b.statementTag(term.tag()) && b.valid(term)
}

func (b *Builder) statementTag(tag uint8) bool {
	switch tag {
	case tagBind, tagAssign, tagCall, tagBranch, tagBody, tagNormal, tagReturn,
		tagThrow, tagYield, tagBreak, tagContinue:
		return true
	default:
		return false
	}
}

func (b *Builder) branchArm(term Term) bool {
	switch term.tag() {
	case tagBody, tagNormal, tagReturn, tagThrow, tagYield, tagBreak, tagContinue:
		return b.valid(term)
	default:
		return false
	}
}

type directCallEdge struct{ from, to uint32 }

// directCallMu derives canonical Function heads for static direct-call SCCs.
// Edges are read directly from typed Call rows: call.owner determines the
// enclosing activation, and Seal-derived direct evidence is the target. No
// operand walk, generic operation stream, or reconstructed execution graph exists.
func (b *Builder) directCallMu(activation, direct []Term) ([]Term, error) {
	functionCount := len(b.functions)
	mu := make([]Term, functionCount+1)
	if functionCount == 0 {
		return mu, nil
	}
	edges := make([]directCallEdge, 0, len(b.calls))
	for callIndex, call := range b.calls {
		if direct[callIndex] == 0 {
			continue
		}
		if !b.has(call.owner, tagBody) || int(call.owner.index()) >= len(activation) {
			return nil, errors.New("program: Call has no activation")
		}
		from := activation[call.owner.index()]
		if from == 0 { // an Entry activation call is not a recursive edge.
			continue
		}
		if !b.has(from, tagFunction) || !b.has(direct[callIndex], tagFunction) {
			return nil, errors.New("program: invalid direct Call edge")
		}
		edges = append(edges, directCallEdge{from: from.index(), to: direct[callIndex].index()})
	}

	forwardStart, forward := directCallAdjacency(functionCount, edges, false)
	reverseStart, reverse := directCallAdjacency(functionCount, edges, true)
	finished := directCallFinishOrder(functionCount, forwardStart, forward)
	component := make([]uint32, functionCount+1)
	componentHead := make([]uint32, functionCount+1)
	members := make([]uint32, 0, functionCount)
	componentID := uint32(0)
	for i := len(finished) - 1; i >= 0; i-- {
		start := finished[i]
		if component[start] != 0 {
			continue
		}
		componentID++
		members = append(members[:0], start)
		component[start] = componentID
		head := start
		for len(members) != 0 {
			node := members[len(members)-1]
			members = members[:len(members)-1]
			if node < head {
				head = node
			}
			for edge := reverseStart[node]; edge < reverseStart[node+1]; edge++ {
				previous := reverse[edge]
				if component[previous] == 0 {
					component[previous] = componentID
					members = append(members, previous)
				}
			}
		}
		componentHead[componentID] = head
	}

	componentSize := make([]uint32, componentID+1)
	for function := uint32(1); function <= uint32(functionCount); function++ {
		componentSize[component[function]]++
	}
	for function := uint32(1); function <= uint32(functionCount); function++ {
		componentID := component[function]
		if componentSize[componentID] > 1 {
			mu[function] = makeTerm(tagFunction, componentHead[componentID])
			continue
		}
		for edge := forwardStart[function]; edge < forwardStart[function+1]; edge++ {
			if forward[edge] == function {
				mu[function] = makeTerm(tagFunction, function)
				break
			}
		}
	}
	return mu, nil
}

func directCallAdjacency(functionCount int, edges []directCallEdge, reverse bool) ([]uint32, []uint32) {
	start := make([]uint32, functionCount+2)
	for _, edge := range edges {
		at := edge.from
		if reverse {
			at = edge.to
		}
		start[at+1]++
	}
	for index := 1; index < len(start); index++ {
		start[index] += start[index-1]
	}
	next := append([]uint32(nil), start[:functionCount+1]...)
	adjacent := make([]uint32, len(edges))
	for _, edge := range edges {
		from, to := edge.from, edge.to
		if reverse {
			from, to = to, from
		}
		adjacent[next[from]] = to
		next[from]++
	}
	return start, adjacent
}

func directCallFinishOrder(functionCount int, start, adjacent []uint32) []uint32 {
	type frame struct{ node, next uint32 }
	seen := make([]bool, functionCount+1)
	order := make([]uint32, 0, functionCount)
	stack := make([]frame, 0, functionCount)
	for root := uint32(1); root <= uint32(functionCount); root++ {
		if seen[root] {
			continue
		}
		seen[root] = true
		stack = append(stack, frame{node: root, next: start[root]})
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < start[top.node+1] {
				next := adjacent[top.next]
				top.next++
				if !seen[next] {
					seen[next] = true
					stack = append(stack, frame{node: next, next: start[next]})
				}
				continue
			}
			order = append(order, top.node)
			stack = stack[:len(stack)-1]
		}
	}
	return order
}

func (b *Builder) snapshot(muFunctions, directCalls []Term) *Program {
	p := &Program{
		termCount:   b.termCount,
		files:       append([]string(nil), b.files...),
		valueTerms:  copyTerms(b.valueTerms),
		bodyTerms:   copyTerms(b.bodyTerms),
		bindTerms:   copyTerms(b.bindTerms),
		assignTerms: copyTerms(b.assignTerms),
		formalTerms: copyTerms(b.formalTerms),
		nils:        copyTerms(b.nils),
		bools:       append([]boolRow(nil), b.bools...),
		integers:    append([]integerRow(nil), b.integers...),
		floats:      append([]floatRow(nil), b.floats...),
		strings:     append([]stringRow(nil), b.strings...),
		values:      append([]valuesRow(nil), b.values...),
		lensExact:   append([]exactLensRow(nil), b.lensExact...),
		lensKeys:    append([]keyLensRow(nil), b.lensKeys...),
		normals:     append([]outcomeRow(nil), b.normals...),
		returns:     append([]outcomeRow(nil), b.returns...),
		throws:      append([]outcomeRow(nil), b.throws...),
		yields:      append([]outcomeRow(nil), b.yields...),
		breaks:      append([]outcomeRow(nil), b.breaks...),
		continues:   append([]outcomeRow(nil), b.continues...),
		muFunctions: copyTerms(muFunctions),
		directCalls: copyTerms(directCalls),
		entry:       b.entry,
		bodies:      append([]bodyRow(nil), b.bodies...),
		cells:       append([]cellRow(nil), b.cells...),
		reads:       append([]readRow(nil), b.reads...),
		varargs:     append([]varargRow(nil), b.varargs...),
		unaries:     append([]unaryRow(nil), b.unaries...),
		binaries:    append([]binaryRow(nil), b.binaries...),
		selects:     append([]selectRow(nil), b.selects...),
		binds:       append([]bindRow(nil), b.binds...),
		assigns:     append([]assignRow(nil), b.assigns...),
		functions:   append([]functionRow(nil), b.functions...),
		captures:    append([]captureRow(nil), b.captures...),
		calls:       append([]callRow(nil), b.calls...),
		branches:    append([]branchRow(nil), b.branches...),
		tables:      append([]tableRow(nil), b.tables...),
		tableFields: append([]tableFieldRow(nil), b.tableFields...),
		keys:        append([]keyRow(nil), b.keys...),
		exactKeys:   append([]exactKey(nil), b.exactKeys...),
	}
	for tag := uint8(1); tag < tagCount; tag++ {
		p.spans[tag] = append([]storedSpan(nil), b.spans[tag]...)
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
		return index <= uint32(len(p.nils))
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
	case tagBody:
		return index <= uint32(len(p.bodies))
	case tagCell:
		return index <= uint32(len(p.cells))
	case tagRead:
		return index <= uint32(len(p.reads))
	case tagVararg:
		return index <= uint32(len(p.varargs))
	case tagUnary:
		return index <= uint32(len(p.unaries))
	case tagBinary:
		return index <= uint32(len(p.binaries))
	case tagSelect:
		return index <= uint32(len(p.selects))
	case tagBind:
		return index <= uint32(len(p.binds))
	case tagAssign:
		return index <= uint32(len(p.assigns))
	case tagFunction:
		return index <= uint32(len(p.functions))
	case tagCall:
		return index <= uint32(len(p.calls))
	case tagBranch:
		return index <= uint32(len(p.branches))
	case tagTable:
		return index <= uint32(len(p.tables))
	case tagKey:
		return index <= uint32(len(p.keys))
	}
	return false
}
func (p *Program) has(term Term, tag uint8) bool { return term.tag() == tag && p.Valid(term) }

// TermCount is a size metric only. Program intentionally exposes no generic
// term stream: consumers must select a typed relation.
func (p *Program) TermCount() int {
	if p == nil {
		return 0
	}
	return p.termCount
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
func (p *Program) Nil(term Term) (owner Term, ok bool) {
	if !p.has(term, tagNil) {
		return 0, false
	}
	return p.nils[term.index()-1], true
}
func (p *Program) Bool(term Term) (owner Term, value bool, ok bool) {
	if !p.has(term, tagBool) {
		return 0, false, false
	}
	row := p.bools[term.index()-1]
	return row.owner, row.value, true
}
func (p *Program) Integer(term Term) (owner Term, value int64, ok bool) {
	if !p.has(term, tagInteger) {
		return 0, 0, false
	}
	row := p.integers[term.index()-1]
	return row.owner, row.value, true
}
func (p *Program) Float(term Term) (owner Term, value float64, ok bool) {
	if !p.has(term, tagFloat) {
		return 0, 0, false
	}
	row := p.floats[term.index()-1]
	return row.owner, math.Float64frombits(row.bits), true
}
func (p *Program) String(term Term) (owner Term, value string, ok bool) {
	if !p.has(term, tagString) {
		return 0, "", false
	}
	row := p.strings[term.index()-1]
	return row.owner, row.value, true
}

func (p *Program) valueRange(term Term) (termRange, bool) {
	if !p.has(term, tagValues) {
		return termRange{}, false
	}
	return p.values[term.index()-1].fixed, true
}

// ValuesLen and Value are non-allocating indexed accessors for a Values relation.
func (p *Program) ValuesLen(term Term) (int, bool) {
	r, ok := p.valueRange(term)
	return int(r.end - r.start), ok
}
func (p *Program) Value(term Term, index int) (Term, bool) {
	r, ok := p.valueRange(term)
	if !ok || index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.valueTerms[r.start+uint32(index)], true
}
func (p *Program) Values(term Term) (owner Term, tail Term, ok bool) {
	if !p.has(term, tagValues) {
		return 0, 0, false
	}
	row := p.values[term.index()-1]
	return row.owner, row.tail, true
}

func (p *Program) Lens(term Term) (owner, base, source Term, kind FieldKind, key Key, ok bool) {
	if p.has(term, tagLensExact) {
		r := p.lensExact[term.index()-1]
		return r.owner, r.base, r.source, r.kind, Key(r.exact), true
	}
	if p.has(term, tagLensKey) {
		r := p.lensKeys[term.index()-1]
		return r.owner, r.base, r.key, FieldKey, 0, true
	}
	return 0, 0, 0, 0, 0, false
}

func (p *Program) Outcome(term Term) (owner, values Term, ok bool) {
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
	return 0, 0, false
}
func (p *Program) Normal(term Term) (owner, values Term, ok bool) { return p.outcome(term, tagNormal) }
func (p *Program) Return(term Term) (owner, values Term, ok bool) { return p.outcome(term, tagReturn) }
func (p *Program) Throw(term Term) (owner, values Term, ok bool)  { return p.outcome(term, tagThrow) }
func (p *Program) Yield(term Term) (owner, values Term, ok bool)  { return p.outcome(term, tagYield) }
func (p *Program) Break(term Term) (owner, values Term, ok bool)  { return p.outcome(term, tagBreak) }
func (p *Program) Continue(term Term) (owner, values Term, ok bool) {
	return p.outcome(term, tagContinue)
}
func (p *Program) outcome(term Term, tag uint8) (Term, Term, bool) {
	if !p.has(term, tag) {
		return 0, 0, false
	}
	index := term.index() - 1
	switch tag {
	case tagNormal:
		row := p.normals[index]
		return row.owner, row.values, true
	case tagReturn:
		row := p.returns[index]
		return row.owner, row.values, true
	case tagThrow:
		row := p.throws[index]
		return row.owner, row.values, true
	case tagYield:
		row := p.yields[index]
		return row.owner, row.values, true
	case tagBreak:
		row := p.breaks[index]
		return row.owner, row.values, true
	}
	row := p.continues[index]
	return row.owner, row.values, true
}

// Mu returns the canonical existing Function head for term's Seal-derived
// direct-call recurrence SCC. Mu is an annotation, never a Term.
func (p *Program) Mu(term Term) (Term, bool) {
	if !p.has(term, tagFunction) || int(term.index()) >= len(p.muFunctions) {
		return 0, false
	}
	head := p.muFunctions[term.index()]
	return head, head != 0
}

// Entry returns the shard's one canonical top-level Body in O(1).
func (p *Program) Entry() (Term, bool) {
	if p == nil || !p.has(p.entry, tagBody) {
		return 0, false
	}
	return p.entry, true
}

func (p *Program) BodyLen(term Term) (int, bool) {
	if !p.has(term, tagBody) {
		return 0, false
	}
	r := p.bodies[term.index()-1].roots
	return int(r.end - r.start), true
}

func (p *Program) Root(term Term, index int) (Term, bool) {
	if !p.has(term, tagBody) {
		return 0, false
	}
	r := p.bodies[term.index()-1].roots
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.bodyTerms[r.start+uint32(index)], true
}

func (p *Program) Cell(term Term) (Term, bool) {
	if !p.has(term, tagCell) {
		return 0, false
	}
	return p.cells[term.index()-1].body, true
}

func (p *Program) Read(term Term) (owner, source Term, ok bool) {
	if !p.has(term, tagRead) {
		return 0, 0, false
	}
	row := p.reads[term.index()-1]
	return row.owner, row.source, true
}

// Vararg returns the Function vararg Cell anchoring an open source occurrence.
func (p *Program) Vararg(term Term) (owner, cell Term, ok bool) {
	if !p.has(term, tagVararg) {
		return 0, 0, false
	}
	row := p.varargs[term.index()-1]
	return row.owner, row.cell, true
}

// Unary returns a closed unary scalar relation in O(1).
func (p *Program) Unary(term Term) (owner Term, op UnaryOp, operand Term, ok bool) {
	if !p.has(term, tagUnary) {
		return 0, 0, 0, false
	}
	row := p.unaries[term.index()-1]
	return row.owner, row.op, row.operand, true
}

// Binary returns a closed binary scalar relation in O(1).
func (p *Program) Binary(term Term) (owner Term, op BinaryOp, left, right Term, ok bool) {
	if !p.has(term, tagBinary) {
		return 0, 0, 0, 0, false
	}
	row := p.binaries[term.index()-1]
	return row.owner, row.op, row.left, row.right, true
}

// Select returns a lazy and/or relation in O(1). Its left operand always
// evaluates; its right operand evaluates only along the operator-selected arm.
func (p *Program) Select(term Term) (owner Term, op SelectOp, left, right Term, ok bool) {
	if !p.has(term, tagSelect) {
		return 0, 0, 0, 0, false
	}
	row := p.selects[term.index()-1]
	return row.owner, row.op, row.left, row.right, true
}

func (p *Program) BindLen(term Term) (int, bool) {
	if !p.has(term, tagBind) {
		return 0, false
	}
	r := p.binds[term.index()-1].cells
	return int(r.end - r.start), true
}

func (p *Program) BoundCell(term Term, index int) (Term, bool) {
	if !p.has(term, tagBind) {
		return 0, false
	}
	r := p.binds[term.index()-1].cells
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.bindTerms[r.start+uint32(index)], true
}

func (p *Program) Bind(term Term) (owner, values Term, ok bool) {
	if !p.has(term, tagBind) {
		return 0, 0, false
	}
	row := p.binds[term.index()-1]
	return row.owner, row.values, true
}

func (p *Program) AssignLen(term Term) (int, bool) {
	if !p.has(term, tagAssign) {
		return 0, false
	}
	r := p.assigns[term.index()-1].targets
	return int(r.end - r.start), true
}

func (p *Program) Target(term Term, index int) (Term, bool) {
	if !p.has(term, tagAssign) {
		return 0, false
	}
	r := p.assigns[term.index()-1].targets
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.assignTerms[r.start+uint32(index)], true
}

func (p *Program) Assign(term Term) (owner, values Term, ok bool) {
	if !p.has(term, tagAssign) {
		return 0, 0, false
	}
	row := p.assigns[term.index()-1]
	return row.owner, row.values, true
}

func (p *Program) Function(term Term) (owner, body, vararg Term, ok bool) {
	if !p.has(term, tagFunction) {
		return 0, 0, 0, false
	}
	row := p.functions[term.index()-1]
	return row.owner, row.body, row.vararg, true
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

func (p *Program) FunctionCaptureCount(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].captures
	return int(r.end - r.start), true
}

func (p *Program) FunctionCapture(term Term, index int) (inner, outer Term, ok bool) {
	if !p.has(term, tagFunction) {
		return 0, 0, false
	}
	r := p.functions[term.index()-1].captures
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, 0, false
	}
	row := p.captures[r.start+uint32(index)]
	return row.inner, row.outer, true
}

func (p *Program) Call(term Term) (owner, callee, receiver, actuals, direct Term, ok bool) {
	if !p.has(term, tagCall) {
		return 0, 0, 0, 0, 0, false
	}
	row := p.calls[term.index()-1]
	return row.owner, row.callee, row.receiver, row.actuals, p.directCalls[term.index()-1], true
}

func (p *Program) Branch(term Term) (owner, condition, whenTrue, whenFalse Term, ok bool) {
	if !p.has(term, tagBranch) {
		return 0, 0, 0, 0, false
	}
	row := p.branches[term.index()-1]
	return row.owner, row.condition, row.whenTrue, row.whenFalse, true
}

func (p *Program) Table(term Term) (owner Term, ok bool) {
	if !p.has(term, tagTable) {
		return 0, false
	}
	return p.tables[term.index()-1].owner, true
}

func (p *Program) Name(term Term) (owner Term, text string, key Key, ok bool) {
	if !p.has(term, tagKey) {
		return 0, "", 0, false
	}
	row := p.keys[term.index()-1]
	if row.kind != FieldName || row.exact == 0 || int(row.exact) > len(p.exactKeys) {
		return 0, "", 0, false
	}
	atom := p.exactKeys[row.exact-1]
	return row.owner, atom.text, Key(row.exact), true
}

func (p *Program) List(term Term) (owner Term, ordinal int64, key Key, ok bool) {
	if !p.has(term, tagKey) {
		return 0, 0, 0, false
	}
	row := p.keys[term.index()-1]
	if row.kind != FieldList || row.exact == 0 || int(row.exact) > len(p.exactKeys) {
		return 0, 0, 0, false
	}
	return row.owner, p.exactKeys[row.exact-1].int, Key(row.exact), true
}

func (p *Program) TableLen(term Term) (int, bool) {
	if !p.has(term, tagTable) {
		return 0, false
	}
	r := p.tables[term.index()-1].fields
	return int(r.end - r.start), true
}

func (p *Program) Field(term Term, index int) (source, values Term, kind FieldKind, key Key, ok bool) {
	if !p.has(term, tagTable) {
		return 0, 0, 0, 0, false
	}
	r := p.tables[term.index()-1].fields
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, 0, 0, 0, false
	}
	field := p.tableFields[r.start+uint32(index)]
	return field.key, field.values, field.kind, Key(field.normalized), true
}

// Typed family enumeration keeps relational consumers allocation-free without
// reintroducing a generic operation stream or tag-dispatch API.
func familyTerm(tag uint8, count, index int) (Term, bool) {
	if index < 0 || index >= count {
		return 0, false
	}
	return makeTerm(tag, uint32(index+1)), true
}
func (p *Program) NilCount() int {
	if p == nil {
		return 0
	}
	return len(p.nils)
}
func (p *Program) NilAt(index int) (Term, bool) { return familyTerm(tagNil, p.NilCount(), index) }
func (p *Program) BoolCount() int {
	if p == nil {
		return 0
	}
	return len(p.bools)
}
func (p *Program) BoolAt(index int) (Term, bool) { return familyTerm(tagBool, p.BoolCount(), index) }
func (p *Program) IntegerCount() int {
	if p == nil {
		return 0
	}
	return len(p.integers)
}
func (p *Program) IntegerAt(index int) (Term, bool) {
	return familyTerm(tagInteger, p.IntegerCount(), index)
}
func (p *Program) FloatCount() int {
	if p == nil {
		return 0
	}
	return len(p.floats)
}
func (p *Program) FloatAt(index int) (Term, bool) { return familyTerm(tagFloat, p.FloatCount(), index) }
func (p *Program) StringCount() int {
	if p == nil {
		return 0
	}
	return len(p.strings)
}
func (p *Program) StringAt(index int) (Term, bool) {
	return familyTerm(tagString, p.StringCount(), index)
}
func (p *Program) ValuesCount() int {
	if p == nil {
		return 0
	}
	return len(p.values)
}
func (p *Program) ValuesAt(index int) (Term, bool) {
	return familyTerm(tagValues, p.ValuesCount(), index)
}
func (p *Program) LensExactCount() int {
	if p == nil {
		return 0
	}
	return len(p.lensExact)
}
func (p *Program) LensExactAt(index int) (Term, bool) {
	return familyTerm(tagLensExact, p.LensExactCount(), index)
}
func (p *Program) LensKeyCount() int {
	if p == nil {
		return 0
	}
	return len(p.lensKeys)
}
func (p *Program) LensKeyAt(index int) (Term, bool) {
	return familyTerm(tagLensKey, p.LensKeyCount(), index)
}
func (p *Program) NormalCount() int {
	if p == nil {
		return 0
	}
	return len(p.normals)
}
func (p *Program) NormalAt(index int) (Term, bool) {
	return familyTerm(tagNormal, p.NormalCount(), index)
}
func (p *Program) ReturnCount() int {
	if p == nil {
		return 0
	}
	return len(p.returns)
}
func (p *Program) ReturnAt(index int) (Term, bool) {
	return familyTerm(tagReturn, p.ReturnCount(), index)
}
func (p *Program) ThrowCount() int {
	if p == nil {
		return 0
	}
	return len(p.throws)
}
func (p *Program) ThrowAt(index int) (Term, bool) { return familyTerm(tagThrow, p.ThrowCount(), index) }
func (p *Program) YieldCount() int {
	if p == nil {
		return 0
	}
	return len(p.yields)
}
func (p *Program) YieldAt(index int) (Term, bool) { return familyTerm(tagYield, p.YieldCount(), index) }
func (p *Program) BreakCount() int {
	if p == nil {
		return 0
	}
	return len(p.breaks)
}
func (p *Program) BreakAt(index int) (Term, bool) { return familyTerm(tagBreak, p.BreakCount(), index) }
func (p *Program) ContinueCount() int {
	if p == nil {
		return 0
	}
	return len(p.continues)
}
func (p *Program) ContinueAt(index int) (Term, bool) {
	return familyTerm(tagContinue, p.ContinueCount(), index)
}
func (p *Program) BodyCount() int {
	if p == nil {
		return 0
	}
	return len(p.bodies)
}
func (p *Program) BodyAt(index int) (Term, bool) {
	return familyTerm(tagBody, p.BodyCount(), index)
}
func (p *Program) CellCount() int {
	if p == nil {
		return 0
	}
	return len(p.cells)
}
func (p *Program) CellAt(index int) (Term, bool) { return familyTerm(tagCell, p.CellCount(), index) }
func (p *Program) ReadCount() int {
	if p == nil {
		return 0
	}
	return len(p.reads)
}
func (p *Program) ReadAt(index int) (Term, bool) { return familyTerm(tagRead, p.ReadCount(), index) }
func (p *Program) VarargCount() int {
	if p == nil {
		return 0
	}
	return len(p.varargs)
}
func (p *Program) VarargAt(index int) (Term, bool) {
	return familyTerm(tagVararg, p.VarargCount(), index)
}
func (p *Program) UnaryCount() int {
	if p == nil {
		return 0
	}
	return len(p.unaries)
}
func (p *Program) UnaryAt(index int) (Term, bool) { return familyTerm(tagUnary, p.UnaryCount(), index) }
func (p *Program) BinaryCount() int {
	if p == nil {
		return 0
	}
	return len(p.binaries)
}
func (p *Program) BinaryAt(index int) (Term, bool) {
	return familyTerm(tagBinary, p.BinaryCount(), index)
}
func (p *Program) SelectCount() int {
	if p == nil {
		return 0
	}
	return len(p.selects)
}
func (p *Program) SelectAt(index int) (Term, bool) {
	return familyTerm(tagSelect, p.SelectCount(), index)
}
func (p *Program) BindCount() int {
	if p == nil {
		return 0
	}
	return len(p.binds)
}
func (p *Program) BindAt(index int) (Term, bool) {
	return familyTerm(tagBind, p.BindCount(), index)
}
func (p *Program) AssignCount() int {
	if p == nil {
		return 0
	}
	return len(p.assigns)
}
func (p *Program) AssignAt(index int) (Term, bool) {
	return familyTerm(tagAssign, p.AssignCount(), index)
}
func (p *Program) FunctionCount() int {
	if p == nil {
		return 0
	}
	return len(p.functions)
}
func (p *Program) FunctionAt(index int) (Term, bool) {
	return familyTerm(tagFunction, p.FunctionCount(), index)
}
func (p *Program) CallCount() int {
	if p == nil {
		return 0
	}
	return len(p.calls)
}
func (p *Program) CallAt(index int) (Term, bool) { return familyTerm(tagCall, p.CallCount(), index) }
func (p *Program) BranchCount() int {
	if p == nil {
		return 0
	}
	return len(p.branches)
}
func (p *Program) BranchAt(index int) (Term, bool) {
	return familyTerm(tagBranch, p.BranchCount(), index)
}
func (p *Program) TableCount() int {
	if p == nil {
		return 0
	}
	return len(p.tables)
}
func (p *Program) TableAt(index int) (Term, bool) {
	return familyTerm(tagTable, p.TableCount(), index)
}
func copyTerms(terms []Term) []Term {
	if len(terms) == 0 {
		return nil
	}
	return append([]Term(nil), terms...)
}
