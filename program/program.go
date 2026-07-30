// Package program contains the sealed source-level semantic program core.
package program

import (
	"errors"
	"math"
)

// Term is a compact 32-bit identity: a 24-bit typed-family index and an
// 8-bit family tag. Zero is invalid.
type Term uint32

// FieldKind distinguishes Lua constructor field evaluation without introducing
// a field Term family.
type FieldKind uint8

const (
	FieldList FieldKind = iota + 1
	FieldExact
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
type bodyRow struct {
	owned  termRange
	filled bool
}
type cellRow struct{ body Term }
type readRow struct{ source Term }
type varargRow struct{ cell Term }
type unaryRow struct {
	op      UnaryOp
	operand Term
}
type binaryRow struct {
	op          BinaryOp
	left, right Term
}
type selectRow struct {
	op          SelectOp
	left, right Term
}
type bindRow struct {
	cells  termRange
	values Term
}
type assignRow struct {
	targets termRange
	values  Term
}
type functionRow struct {
	owner, body, binding, vararg Term
	formals                      termRange
	captures                     termRange
	capturesFilled               bool
}
type captureRow struct{ function, inner, outer Term }
type callRow struct{ callee, receiver, actuals, direct Term }
type branchRow struct{ condition, whenTrue, whenFalse Term }
type tableRow struct {
	fields termRange
	filled bool
}
type tableFieldRow struct {
	key, values Term
	kind        FieldKind
	normalized  uint32
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

// Program is the immutable result of Builder.Seal.
type Program struct {
	terms []Term // source mint order only; never used for lookup
	files []string
	spans [tagCount][]storedSpan

	valueTerms   []Term
	bindTerms    []Term
	formalTerms  []Term
	captureTerms []Term
	orderTerms   []Term
	orders       [tagCount][]termRange

	nils     uint32
	bools    []bool
	integers []int64
	floats   []uint64
	strings  []string
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
	exactKeys   []exactKey
}

// Builder is the sole mutable construction path for Program.
type Builder struct {
	terms     []Term
	files     []string
	fileIndex map[string]uint32
	spans     [tagCount][]storedSpan
	poison    bool

	valueTerms   []Term
	bindTerms    []Term
	formalTerms  []Term
	captureTerms []Term
	orderTerms   []Term
	orders       [tagCount][]termRange

	nils     uint32
	bools    []bool
	integers []int64
	floats   []uint64
	strings  []string
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

// LensExact records an evaluated base plus a static nil, bool, integer, float,
// or string key occurrence. Nil and NaN remain exact source evidence but do
// not receive a storable normalized-key atom.
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
	b.cells = append(b.cells, cellRow{body: body})
	term := b.mint(tagCell, span, b.familyIndex(len(b.cells)))
	if term == 0 {
		b.cells = b.cells[:len(b.cells)-1]
	}
	return term
}

// Read observes a lexical Cell or an evaluated Lens. Reading a Cell has no
// evaluation predecessor; reading a Lens evaluates that Lens exactly once.
func (b *Builder) Read(span Span, source Term) Term {
	b.reads = append(b.reads, readRow{source: source})
	term := b.mint(tagRead, span, b.familyIndex(len(b.reads)))
	if term == 0 {
		b.reads = b.reads[:len(b.reads)-1]
		return 0
	}
	if b.has(source, tagLensExact) || b.has(source, tagLensKey) {
		b.setOrder(term, []Term{source})
	} else {
		b.setOrder(term, nil)
	}
	return term
}

// Vararg records one open source occurrence anchored to a Function's vararg
// Cell. It is an occurrence, not a scalar Cell read, so Values may retain it
// as its final open tail.
func (b *Builder) Vararg(span Span, cell Term) Term {
	b.varargs = append(b.varargs, varargRow{cell: cell})
	term := b.mint(tagVararg, span, b.familyIndex(len(b.varargs)))
	if term == 0 {
		b.varargs = b.varargs[:len(b.varargs)-1]
		return 0
	}
	b.setOrder(term, nil)
	return term
}

// Unary records one closed unary scalar operation.
func (b *Builder) Unary(span Span, op UnaryOp, operand Term) Term {
	b.unaries = append(b.unaries, unaryRow{op: op, operand: operand})
	term := b.mint(tagUnary, span, b.familyIndex(len(b.unaries)))
	if term == 0 {
		b.unaries = b.unaries[:len(b.unaries)-1]
		return 0
	}
	b.setOrder(term, []Term{operand})
	return term
}

// Binary records one closed left-to-right binary scalar operation.
func (b *Builder) Binary(span Span, op BinaryOp, left, right Term) Term {
	b.binaries = append(b.binaries, binaryRow{op: op, left: left, right: right})
	term := b.mint(tagBinary, span, b.familyIndex(len(b.binaries)))
	if term == 0 {
		b.binaries = b.binaries[:len(b.binaries)-1]
		return 0
	}
	b.setOrder(term, []Term{left, right})
	return term
}

// Select records Lua's lazy and/or value selection. The left operand always
// evaluates first; the right operand is retained in the row and evaluates only
// when the selected operator's truthiness rule demands it.
func (b *Builder) Select(span Span, op SelectOp, left, right Term) Term {
	b.selects = append(b.selects, selectRow{op: op, left: left, right: right})
	term := b.mint(tagSelect, span, b.familyIndex(len(b.selects)))
	if term == 0 {
		b.selects = b.selects[:len(b.selects)-1]
		return 0
	}
	b.setOrder(term, []Term{left})
	return term
}

// Bind initializes lexical Cells. Cell identities are static; evaluation
// order contains only the RHS Values relation.
func (b *Builder) Bind(span Span, cells []Term, values Term) Term {
	r, ok := b.appendPool(&b.bindTerms, cells)
	if !ok {
		return 0
	}
	b.binds = append(b.binds, bindRow{cells: r, values: values})
	term := b.mint(tagBind, span, b.familyIndex(len(b.binds)))
	if term == 0 {
		b.binds = b.binds[:len(b.binds)-1]
		b.bindTerms = b.bindTerms[:r.start]
		return 0
	}
	b.setOrder(term, []Term{values})
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

// Function records a closure with its lexical owner, execution Body, optional
// enclosing binding Cell, ordered formal Cells, and optional vararg Cell.
// SetFunctionCaptures must fill its capture set exactly once before Seal.
func (b *Builder) Function(span Span, owner, body, binding Term, formals []Term, vararg Term) Term {
	r, ok := b.appendPool(&b.formalTerms, formals)
	if !ok {
		return 0
	}
	b.functions = append(b.functions, functionRow{owner: owner, body: body, binding: binding, vararg: vararg, formals: r})
	term := b.mint(tagFunction, span, b.familyIndex(len(b.functions)))
	if term == 0 {
		b.functions = b.functions[:len(b.functions)-1]
		b.formalTerms = b.formalTerms[:r.start]
	}
	return term
}

// SetFunctionCaptures fixes one Function's capture rows exactly once.
func (b *Builder) SetFunctionCaptures(function Term, captures []Term) bool {
	if !b.has(function, tagFunction) {
		b.poison = true
		return false
	}
	row := &b.functions[function.index()-1]
	if row.capturesFilled {
		b.poison = true
		return false
	}
	r, ok := b.appendPool(&b.captureTerms, captures)
	if !ok {
		return false
	}
	row.captures = r
	row.capturesFilled = true
	return true
}

// Capture records one exact lexical alias owned by function's execution Body.
func (b *Builder) Capture(span Span, function, inner, outer Term) Term {
	b.captures = append(b.captures, captureRow{function: function, inner: inner, outer: outer})
	term := b.mint(tagCapture, span, b.familyIndex(len(b.captures)))
	if term == 0 {
		b.captures = b.captures[:len(b.captures)-1]
	}
	return term
}

// Call is an open result producer. Evaluation is callee then actual Values.
// receiver is an optional semantic correspondence for method syntax, not an
// extra evaluation child: a method callee is Read(Lens(receiver, key)), which
// already evaluates receiver exactly once. direct is zero or coherent
// binder-proven Function evidence; it never replaces the callee occurrence.
func (b *Builder) Call(span Span, callee, receiver, actuals, direct Term) Term {
	b.calls = append(b.calls, callRow{callee: callee, receiver: receiver, actuals: actuals, direct: direct})
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

// Table mints allocation identity. SetTable later fills its direct constructor
// fields exactly once without routing construction through mutation relations.
func (b *Builder) Table(span Span) Term {
	b.tables = append(b.tables, tableRow{})
	term := b.mint(tagTable, span, b.familyIndex(len(b.tables)))
	if term == 0 {
		b.tables = b.tables[:len(b.tables)-1]
	}
	return term
}

func (b *Builder) SetTable(table Term, keys, values []Term, kinds []FieldKind) bool {
	if !b.has(table, tagTable) {
		b.poison = true
		return false
	}
	row := &b.tables[table.index()-1]
	if row.filled || len(keys) != len(values) || len(keys) != len(kinds) {
		b.poison = true
		return false
	}
	startFields := uint64(len(b.tableFields))
	endFields := startFields + uint64(len(keys))
	if endFields > math.MaxUint32 {
		b.poison = true
		return false
	}
	orderStart := uint32(len(b.orderTerms))
	for i := range keys {
		field := tableFieldRow{key: keys[i], values: values[i], kind: kinds[i]}
		switch kinds[i] {
		case FieldList, FieldExact:
			field.normalized, _ = b.normalizedExactKey(keys[i])
		case FieldKey:
			if _, ok := b.appendPool(&b.orderTerms, []Term{keys[i]}); !ok {
				return false
			}
		}
		if _, ok := b.appendPool(&b.orderTerms, []Term{values[i]}); !ok {
			return false
		}
		b.tableFields = append(b.tableFields, field)
	}
	row.fields = termRange{start: uint32(startFields), end: uint32(endFields)}
	row.filled = true
	b.setOrderRange(table, termRange{start: orderStart, end: uint32(len(b.orderTerms))})
	return true
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

func (b *Builder) absentNormalizedKey(term Term) bool {
	if !b.valid(term) {
		return false
	}
	if term.tag() == tagNil {
		return true
	}
	return term.tag() == tagFloat && math.IsNaN(math.Float64frombits(b.floats[term.index()-1]))
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
	case tagCapture:
		return index <= uint32(len(b.captures))
	case tagCall:
		return index <= uint32(len(b.calls))
	case tagBranch:
		return index <= uint32(len(b.branches))
	case tagTable:
		return index <= uint32(len(b.tables))
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
			if !b.valueOccurrence(b.valueTerms[i]) {
				return nil, errors.New("program: invalid Values reference")
			}
		}
		if row.tail != 0 && !b.openOccurrence(row.tail) {
			return nil, errors.New("program: invalid Values tail")
		}
	}
	for _, row := range b.lensExact {
		if !b.valueOccurrence(row.base) || !b.staticExactKey(row.source) {
			return nil, errors.New("program: invalid exact Lens reference")
		}
		if row.exact == 0 {
			if !b.absentNormalizedKey(row.source) {
				return nil, errors.New("program: invalid exact Lens key")
			}
			continue
		}
		if row.exact > uint32(len(b.exactKeys)) {
			return nil, errors.New("program: invalid exact Lens key")
		}
		key, ok := b.exactKey(row.source)
		if !ok || b.exactKeys[row.exact-1] != key {
			return nil, errors.New("program: invalid exact Lens key")
		}
	}
	for _, row := range b.lensKeys {
		if !b.valueOccurrence(row.base) || !b.valueOccurrence(row.key) {
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
	if b.entry == 0 {
		return nil, errors.New("program: missing Entry Body")
	}
	if !b.has(b.entry, tagBody) {
		return nil, errors.New("program: invalid Entry Body")
	}
	var ownerOffset [tagCount]int
	ownerCount := 0
	for tag := uint8(1); tag < tagCount; tag++ {
		if b.statementTag(tag) {
			ownerOffset[tag] = ownerCount
			ownerCount += len(b.spans[tag])
		}
	}
	ownedBy := make([]Term, ownerCount)
	bodyParent := make([]uint32, len(b.bodies)+1)
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
			if !b.statementRoot(owned) {
				return nil, errors.New("program: Body requires statement roots")
			}
			ownerSlot := ownerOffset[owned.tag()] + int(owned.index()-1)
			if previous := ownedBy[ownerSlot]; previous != 0 {
				return nil, errors.New("program: term has duplicate Body ownership")
			}
			ownedBy[ownerSlot] = body
			if owned.tag() == tagBody {
				bodyParent[owned.index()] = uint32(bodyIndex + 1)
			}
		}
	}
	bodyState := make([]uint8, len(b.bodies)+1)
	for start := uint32(1); start <= uint32(len(b.bodies)); start++ {
		body := start
		for body != 0 && bodyState[body] == 0 {
			bodyState[body] = 1
			body = bodyParent[body]
		}
		if body != 0 && bodyState[body] == 1 {
			return nil, errors.New("program: Body ownership cycle")
		}
		body = start
		for body != 0 && bodyState[body] == 1 {
			bodyState[body] = 2
			body = bodyParent[body]
		}
	}
	if bodyParent[b.entry.index()] != 0 {
		return nil, errors.New("program: Entry Body cannot be nested")
	}
	for _, row := range b.cells {
		if !b.has(row.body, tagBody) {
			return nil, errors.New("program: Cell requires Body owner")
		}
	}
	for readIndex, row := range b.reads {
		if !b.has(row.source, tagCell) && !b.has(row.source, tagLensExact) && !b.has(row.source, tagLensKey) {
			return nil, errors.New("program: Read requires Cell or Lens")
		}
		orders := b.orders[tagRead]
		if readIndex >= len(orders) {
			return nil, errors.New("program: Read has no evaluation order")
		}
		order := orders[readIndex]
		if b.has(row.source, tagCell) {
			if order.start != order.end {
				return nil, errors.New("program: Cell Read must have empty evaluation order")
			}
			continue
		}
		if order.end-order.start != 1 || b.orderTerms[order.start] != row.source {
			return nil, errors.New("program: Lens Read must evaluate its Lens exactly once")
		}
	}
	for _, row := range b.unaries {
		if !row.op.valid() || !b.valueOccurrence(row.operand) {
			return nil, errors.New("program: invalid Unary relation")
		}
	}
	for _, row := range b.binaries {
		if !row.op.valid() || !b.valueOccurrence(row.left) || !b.valueOccurrence(row.right) {
			return nil, errors.New("program: invalid Binary relation")
		}
	}
	for _, row := range b.selects {
		if !row.op.valid() || !b.valueOccurrence(row.left) || !b.valueOccurrence(row.right) {
			return nil, errors.New("program: invalid Select relation")
		}
	}
	boundCell := make([]Term, len(b.cells)+1)
	for bindIndex, row := range b.binds {
		bind := makeTerm(tagBind, uint32(bindIndex+1))
		owner := ownedBy[ownerOffset[tagBind]+int(bind.index()-1)]
		if owner == 0 {
			return nil, errors.New("program: Bind requires Body ownership")
		}
		if !b.has(row.values, tagValues) {
			return nil, errors.New("program: Bind requires RHS Values")
		}
		if row.cells.start == row.cells.end {
			return nil, errors.New("program: Bind requires Cells")
		}
		for i := row.cells.start; i < row.cells.end; i++ {
			cell := b.bindTerms[i]
			if !b.has(cell, tagCell) || b.cells[cell.index()-1].body != owner {
				return nil, errors.New("program: Bind Cell must belong to its Body")
			}
			if boundCell[cell.index()] != 0 {
				return nil, errors.New("program: Cell bound more than once")
			}
			boundCell[cell.index()] = bind
		}
	}
	for _, row := range b.assigns {
		if !b.has(row.values, tagValues) {
			return nil, errors.New("program: Assign requires RHS Values")
		}
		if row.targets.start == row.targets.end {
			return nil, errors.New("program: Assign requires targets")
		}
		for i := row.targets.start; i < row.targets.end; i++ {
			target := b.orderTerms[i]
			if !b.has(target, tagCell) && !b.has(target, tagLensExact) && !b.has(target, tagLensKey) {
				return nil, errors.New("program: Assign target requires Cell or Lens")
			}
		}
	}
	functionByBody := make([]Term, len(b.bodies)+1)
	functionOwnerByBody := make([]uint32, len(b.bodies)+1)
	for functionIndex, row := range b.functions {
		if !b.has(row.owner, tagBody) || !b.has(row.body, tagBody) || row.owner == row.body {
			return nil, errors.New("program: Function requires distinct owner and Body")
		}
		if functionByBody[row.body.index()] != 0 {
			return nil, errors.New("program: Body has more than one Function")
		}
		functionByBody[row.body.index()] = makeTerm(tagFunction, uint32(functionIndex+1))
		functionOwnerByBody[row.body.index()] = row.owner.index()
	}
	if functionByBody[b.entry.index()] != 0 {
		return nil, errors.New("program: Entry Body cannot be a Function body")
	}
	lexicalParent := make([]uint32, len(b.bodies)+1)
	for bodyIndex := uint32(1); bodyIndex <= uint32(len(b.bodies)); bodyIndex++ {
		if bodyIndex == b.entry.index() {
			if bodyParent[bodyIndex] != 0 {
				return nil, errors.New("program: Entry Body cannot be nested")
			}
			continue
		}
		nested := bodyParent[bodyIndex] != 0
		functionBody := functionOwnerByBody[bodyIndex] != 0
		if nested == functionBody {
			return nil, errors.New("program: non-Entry Body requires exactly one structural parent")
		}
		if nested {
			lexicalParent[bodyIndex] = bodyParent[bodyIndex]
		} else {
			lexicalParent[bodyIndex] = functionOwnerByBody[bodyIndex]
		}
	}
	for _, row := range b.captures {
		if !b.has(row.function, tagFunction) {
			return nil, errors.New("program: Capture requires Function")
		}
	}

	var captureTerminal []Term
	var varargFunction []Term
	var functionCellRole []Term
	var bindingFunction []Term
	if len(b.functions) != 0 {
		functionCellRole = make([]Term, len(b.cells)+1)
		varargFunction = make([]Term, len(b.cells)+1)
		bindingFunction = make([]Term, len(b.cells)+1)
		for functionIndex, row := range b.functions {
			function := makeTerm(tagFunction, uint32(functionIndex+1))
			if row.binding != 0 && (!b.has(row.binding, tagCell) || b.cells[row.binding.index()-1].body != row.owner) {
				return nil, errors.New("program: Function binding requires Cell in owner Body")
			}
			if row.binding != 0 {
				if bindingFunction[row.binding.index()] != 0 {
					return nil, errors.New("program: Function binding Cell is not unique")
				}
				bindingFunction[row.binding.index()] = function
			}
			if !row.capturesFilled {
				return nil, errors.New("program: Function captures were not filled")
			}
			for i := row.formals.start; i < row.formals.end; i++ {
				formal := b.formalTerms[i]
				if !b.has(formal, tagCell) || b.cells[formal.index()-1].body != row.body {
					return nil, errors.New("program: Function formal requires Cell owned by its Body")
				}
				if functionCellRole[formal.index()] != 0 {
					return nil, errors.New("program: Function formal and vararg Cells must be distinct")
				}
				functionCellRole[formal.index()] = function
			}
			if row.vararg != 0 {
				if !b.has(row.vararg, tagCell) || b.cells[row.vararg.index()-1].body != row.body {
					return nil, errors.New("program: Function vararg requires Cell owned by its Body")
				}
				if functionCellRole[row.vararg.index()] != 0 {
					return nil, errors.New("program: Function formal and vararg Cells must be distinct")
				}
				functionCellRole[row.vararg.index()] = function
				varargFunction[row.vararg.index()] = function
			}
		}
		if !entryBodyForest(lexicalParent, b.entry.index()) {
			return nil, errors.New("program: lexical Body forest is disconnected or cyclic")
		}
		pre, post := bodyIntervals(lexicalParent)
		captureLinked := make([]bool, len(b.captures)+1)
		captureOuter := make([]Term, len(b.cells)+1)
		for functionIndex, row := range b.functions {
			function := makeTerm(tagFunction, uint32(functionIndex+1))
			for i := row.captures.start; i < row.captures.end; i++ {
				capture := b.captureTerms[i]
				if !b.has(capture, tagCapture) {
					return nil, errors.New("program: Function capture requires Capture")
				}
				captureIndex := capture.index()
				if captureLinked[captureIndex] || b.captures[captureIndex-1].function != function {
					return nil, errors.New("program: Function capture ownership is not exact")
				}
				captureLinked[captureIndex] = true
				captureRow := b.captures[captureIndex-1]
				if !b.has(captureRow.inner, tagCell) || !b.has(captureRow.outer, tagCell) {
					return nil, errors.New("program: Capture requires inner and outer Cells")
				}
				if b.cells[captureRow.inner.index()-1].body != row.body {
					return nil, errors.New("program: Capture inner Cell must belong to Function Body")
				}
				outerBody := b.cells[captureRow.outer.index()-1].body
				if outerBody == row.body || !(pre[outerBody.index()] <= pre[row.body.index()] && post[row.body.index()] <= post[outerBody.index()]) {
					return nil, errors.New("program: Capture outer Cell must belong to strict lexical ancestor")
				}
				if captureOuter[captureRow.inner.index()] != 0 {
					return nil, errors.New("program: Capture inner Cell has more than one outer alias")
				}
				if functionCellRole[captureRow.inner.index()] != 0 {
					return nil, errors.New("program: Capture inner Cell conflicts with Function local role")
				}
				captureOuter[captureRow.inner.index()] = captureRow.outer
				functionCellRole[captureRow.inner.index()] = function
			}
		}
		for captureIndex := range b.captures {
			if !captureLinked[captureIndex+1] {
				return nil, errors.New("program: Capture was not set on its Function")
			}
		}
		captureTerminal = terminalCaptureCells(captureOuter)
		for _, row := range b.functions {
			if row.binding != 0 && captureTerminal[row.binding.index()] != row.binding {
				return nil, errors.New("program: Function binding must be terminal lexical Cell")
			}
		}
	}
	if len(b.functions) == 0 && !entryBodyForest(lexicalParent, b.entry.index()) {
		return nil, errors.New("program: lexical Body forest is disconnected or cyclic")
	}
	if captureTerminal == nil {
		captureTerminal = terminalCaptureCells(make([]Term, len(b.cells)+1))
	}
	for _, row := range b.varargs {
		if !b.has(row.cell, tagCell) || len(varargFunction) == 0 || varargFunction[row.cell.index()] == 0 {
			return nil, errors.New("program: Vararg requires Function vararg Cell")
		}
	}
	for _, row := range b.calls {
		if !b.valueOccurrence(row.callee) || row.receiver != 0 && !b.valueOccurrence(row.receiver) || !b.has(row.actuals, tagValues) {
			return nil, errors.New("program: Call requires callee, optional receiver, and actual Values")
		}
		if row.receiver != 0 {
			if !b.has(row.callee, tagRead) {
				return nil, errors.New("program: Call receiver requires method Read callee")
			}
			lens := b.reads[row.callee.index()-1].source
			if !b.has(lens, tagLensExact) {
				return nil, errors.New("program: Call receiver requires exact method Lens")
			}
			method := b.lensExact[lens.index()-1]
			if method.base != row.receiver || !b.has(method.source, tagString) {
				return nil, errors.New("program: Call receiver disagrees with method callee")
			}
		}
		if b.has(row.callee, tagFunction) {
			if row.direct != row.callee {
				return nil, errors.New("program: Function callee requires matching direct target")
			}
			continue
		}
		if b.has(row.callee, tagRead) {
			source := b.reads[row.callee.index()-1].source
			if b.has(source, tagCell) {
				terminal := captureTerminal[source.index()]
				named := Term(0)
				if terminal != 0 && len(bindingFunction) != 0 {
					named = bindingFunction[terminal.index()]
				}
				if named == 0 {
					if row.direct != 0 {
						return nil, errors.New("program: Call Cell binding has no direct Function")
					}
					continue
				}
				if row.direct != named {
					return nil, errors.New("program: Call Cell binding requires matching direct target")
				}
				continue
			}
		}
		if row.direct != 0 {
			return nil, errors.New("program: Call direct target requires Function or bound Cell Read callee")
		}
	}
	for _, row := range b.branches {
		if !b.valueOccurrence(row.condition) || !b.branchArm(row.whenTrue) || !b.branchArm(row.whenFalse) {
			return nil, errors.New("program: Branch requires condition and Body/Outcome arms")
		}
	}
	for _, row := range b.tables {
		if !row.filled {
			return nil, errors.New("program: Table was not filled")
		}
		for i := row.fields.start; i < row.fields.end; i++ {
			field := b.tableFields[i]
			if !b.has(field.values, tagValues) {
				return nil, errors.New("program: Table field requires Values")
			}
			switch field.kind {
			case FieldList, FieldExact:
				if !b.staticExactKey(field.key) {
					return nil, errors.New("program: invalid exact Table key")
				}
				if field.normalized == 0 {
					if !b.absentNormalizedKey(field.key) {
						return nil, errors.New("program: invalid exact Table key")
					}
					continue
				}
				if field.normalized > uint32(len(b.exactKeys)) {
					return nil, errors.New("program: invalid exact Table key")
				}
				key, ok := b.exactKey(field.key)
				if !ok || b.exactKeys[field.normalized-1] != key {
					return nil, errors.New("program: invalid exact Table key")
				}
			case FieldKey:
				if !b.valueOccurrence(field.key) {
					return nil, errors.New("program: invalid dynamic Table key")
				}
			default:
				return nil, errors.New("program: invalid Table field kind")
			}
		}
	}
	muFunctions, err := b.directCallMu()
	if err != nil {
		return nil, err
	}
	return b.snapshot(muFunctions), nil
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
type directCallWork struct {
	term  Term
	owner uint32
}

// directCallMu derives canonical Function heads for static direct-call SCCs.
// It walks each executable occurrence once under its unique owning Function,
// never descends through a nested Function value, and stores no second
// execution graph.
func (b *Builder) directCallMu() ([]Term, error) {
	functionCount := len(b.functions)
	mu := make([]Term, functionCount+1)
	if functionCount == 0 {
		return mu, nil
	}

	var ownerOffset [tagCount]int
	ownerCount := 0
	for tag := uint8(1); tag < tagCount; tag++ {
		ownerOffset[tag] = ownerCount
		ownerCount += len(b.spans[tag])
	}
	owner := make([]uint32, ownerCount)
	edges := make([]directCallEdge, 0, len(b.calls))
	stack := make([]directCallWork, 0, len(b.terms)+len(b.functions))
	for functionIndex, row := range b.functions {
		function := uint32(functionIndex + 1)
		stack = append(stack, directCallWork{term: row.body, owner: function})
	}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		term := current.term
		if term == 0 || term.tag() >= tagCount || term.index() > uint32(len(b.spans[term.tag()])) {
			continue
		}

		// A Function is a static value at this point. Its body is seeded as a
		// separate execution root above, so common callee references are lawful.
		if term.tag() == tagFunction {
			continue
		}
		ownerSlot := ownerOffset[term.tag()] + int(term.index()-1)
		if previous := owner[ownerSlot]; previous != 0 {
			if previous != current.owner {
				return nil, errors.New("program: executable occurrence belongs to multiple Functions")
			}
			continue
		}
		owner[ownerSlot] = current.owner
		if term.tag() == tagCall {
			direct := b.calls[term.index()-1].direct
			if direct != 0 {
				edges = append(edges, directCallEdge{from: current.owner, to: direct.index()})
			}
		}
		if term.tag() == tagBranch {
			branch := b.branches[term.index()-1]
			stack = append(stack,
				directCallWork{term: branch.whenTrue, owner: current.owner},
				directCallWork{term: branch.whenFalse, owner: current.owner},
			)
		}
		if term.tag() == tagSelect {
			// Select's RHS is conditional, not part of its unconditional
			// evaluation order. It is nevertheless executable under this
			// Function and therefore participates in ownership and direct-call
			// recurrence discovery.
			stack = append(stack, directCallWork{term: b.selects[term.index()-1].right, owner: current.owner})
		}
		rows := b.orders[term.tag()]
		if term.index() <= uint32(len(rows)) {
			order := rows[term.index()-1]
			for i := order.start; i < order.end; i++ {
				stack = append(stack, directCallWork{term: b.orderTerms[i], owner: current.owner})
			}
		}
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

func (b *Builder) snapshot(muFunctions []Term) *Program {
	p := &Program{
		terms:        copyTerms(b.terms),
		files:        append([]string(nil), b.files...),
		valueTerms:   copyTerms(b.valueTerms),
		bindTerms:    copyTerms(b.bindTerms),
		formalTerms:  copyTerms(b.formalTerms),
		captureTerms: copyTerms(b.captureTerms),
		orderTerms:   copyTerms(b.orderTerms),
		nils:         b.nils,
		bools:        append([]bool(nil), b.bools...),
		integers:     append([]int64(nil), b.integers...),
		floats:       append([]uint64(nil), b.floats...),
		strings:      append([]string(nil), b.strings...),
		values:       append([]valuesRow(nil), b.values...),
		lensExact:    append([]exactLensRow(nil), b.lensExact...),
		lensKeys:     append([]keyLensRow(nil), b.lensKeys...),
		normals:      append([]outcomeRow(nil), b.normals...),
		returns:      append([]outcomeRow(nil), b.returns...),
		throws:       append([]outcomeRow(nil), b.throws...),
		yields:       append([]outcomeRow(nil), b.yields...),
		breaks:       append([]outcomeRow(nil), b.breaks...),
		continues:    append([]outcomeRow(nil), b.continues...),
		muFunctions:  copyTerms(muFunctions),
		entry:        b.entry,
		bodies:       append([]bodyRow(nil), b.bodies...),
		cells:        append([]cellRow(nil), b.cells...),
		reads:        append([]readRow(nil), b.reads...),
		varargs:      append([]varargRow(nil), b.varargs...),
		unaries:      append([]unaryRow(nil), b.unaries...),
		binaries:     append([]binaryRow(nil), b.binaries...),
		selects:      append([]selectRow(nil), b.selects...),
		binds:        append([]bindRow(nil), b.binds...),
		assigns:      append([]assignRow(nil), b.assigns...),
		functions:    append([]functionRow(nil), b.functions...),
		captures:     append([]captureRow(nil), b.captures...),
		calls:        append([]callRow(nil), b.calls...),
		branches:     append([]branchRow(nil), b.branches...),
		tables:       append([]tableRow(nil), b.tables...),
		tableFields:  append([]tableFieldRow(nil), b.tableFields...),
		exactKeys:    append([]exactKey(nil), b.exactKeys...),
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
	case tagCapture:
		return index <= uint32(len(p.captures))
	case tagCall:
		return index <= uint32(len(p.calls))
	case tagBranch:
		return index <= uint32(len(p.branches))
	case tagTable:
		return index <= uint32(len(p.tables))
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
	leftRow := p.lensExact[left.index()-1]
	rightRow := p.lensExact[right.index()-1]
	if leftRow.exact != 0 || rightRow.exact != 0 {
		return leftRow.exact != 0 && leftRow.exact == rightRow.exact
	}
	return leftRow.source.tag() == tagNil && rightRow.source.tag() == tagNil
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
	return p.reads[term.index()-1].source, true
}

// Vararg returns the Function vararg Cell anchoring an open source occurrence.
func (p *Program) Vararg(term Term) (Term, bool) {
	if !p.has(term, tagVararg) {
		return 0, false
	}
	return p.varargs[term.index()-1].cell, true
}

// Unary returns a closed unary scalar relation in O(1).
func (p *Program) Unary(term Term) (UnaryOp, Term, bool) {
	if !p.has(term, tagUnary) {
		return 0, 0, false
	}
	row := p.unaries[term.index()-1]
	return row.op, row.operand, true
}

// Binary returns a closed binary scalar relation in O(1).
func (p *Program) Binary(term Term) (BinaryOp, Term, Term, bool) {
	if !p.has(term, tagBinary) {
		return 0, 0, 0, false
	}
	row := p.binaries[term.index()-1]
	return row.op, row.left, row.right, true
}

// Select returns a lazy and/or relation in O(1). Its right operand is absent
// from Order because execution branches on the evaluated left operand.
func (p *Program) Select(term Term) (SelectOp, Term, Term, bool) {
	if !p.has(term, tagSelect) {
		return 0, 0, 0, false
	}
	row := p.selects[term.index()-1]
	return row.op, row.left, row.right, true
}

func (p *Program) BindLen(term Term) (int, bool) {
	if !p.has(term, tagBind) {
		return 0, false
	}
	r := p.binds[term.index()-1].cells
	return int(r.end - r.start), true
}

func (p *Program) BindAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagBind) {
		return 0, false
	}
	r := p.binds[term.index()-1].cells
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.bindTerms[r.start+uint32(index)], true
}

func (p *Program) BindValues(term Term) (Term, bool) {
	if !p.has(term, tagBind) {
		return 0, false
	}
	return p.binds[term.index()-1].values, true
}

func (p *Program) AppendBind(term Term, dst []Term) (cells []Term, values Term, ok bool) {
	if !p.has(term, tagBind) {
		return dst, 0, false
	}
	row := p.binds[term.index()-1]
	return append(dst, p.bindTerms[row.cells.start:row.cells.end]...), row.values, true
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

func (p *Program) Function(term Term) (owner, body, binding, vararg Term, ok bool) {
	if !p.has(term, tagFunction) {
		return 0, 0, 0, 0, false
	}
	row := p.functions[term.index()-1]
	return row.owner, row.body, row.binding, row.vararg, true
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

func (p *Program) FunctionCaptureLen(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].captures
	return int(r.end - r.start), true
}

func (p *Program) FunctionCaptureAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].captures
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, false
	}
	return p.captureTerms[r.start+uint32(index)], true
}

func (p *Program) AppendFunctionCaptures(term Term, dst []Term) ([]Term, bool) {
	if !p.has(term, tagFunction) {
		return dst, false
	}
	r := p.functions[term.index()-1].captures
	return append(dst, p.captureTerms[r.start:r.end]...), true
}

func (p *Program) Capture(term Term) (function, inner, outer Term, ok bool) {
	if !p.has(term, tagCapture) {
		return 0, 0, 0, false
	}
	row := p.captures[term.index()-1]
	return row.function, row.inner, row.outer, true
}

func (p *Program) Call(term Term) (callee, receiver, actuals, direct Term, ok bool) {
	if !p.has(term, tagCall) {
		return 0, 0, 0, 0, false
	}
	row := p.calls[term.index()-1]
	return row.callee, row.receiver, row.actuals, row.direct, true
}

func (p *Program) Branch(term Term) (condition, whenTrue, whenFalse Term, ok bool) {
	if !p.has(term, tagBranch) {
		return 0, 0, 0, false
	}
	row := p.branches[term.index()-1]
	return row.condition, row.whenTrue, row.whenFalse, true
}

func (p *Program) Table(term Term) bool { return p.has(term, tagTable) }

func (p *Program) TableLen(term Term) (int, bool) {
	if !p.has(term, tagTable) {
		return 0, false
	}
	r := p.tables[term.index()-1].fields
	return int(r.end - r.start), true
}

func (p *Program) TableAt(term Term, index int) (key, values Term, kind FieldKind, ok bool) {
	if !p.has(term, tagTable) {
		return 0, 0, 0, false
	}
	r := p.tables[term.index()-1].fields
	if index < 0 || uint32(index) >= r.end-r.start {
		return 0, 0, 0, false
	}
	field := p.tableFields[r.start+uint32(index)]
	return field.key, field.values, field.kind, true
}

func copyTerms(terms []Term) []Term {
	if len(terms) == 0 {
		return nil
	}
	return append([]Term(nil), terms...)
}
