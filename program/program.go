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
	tagCount
)

func makeTerm(tag uint8, index uint32) Term { return Term(index<<tagBits | uint32(tag)) }
func (t Term) tag() uint8                   { return uint8(uint32(t) & tagMask) }
func (t Term) index() uint32                { return uint32(t) >> tagBits }

// Span identifies a source byte extent. File may be empty for generated code.
type Span struct {
	File       string
	Start, End int
}

// termRange indexes one contiguous Term pool. It never owns a slice.
type termRange struct{ start, end uint32 }
type storedSpan struct{ file, start, end uint32 }
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

	valueTerms []Term
	orderTerms []Term
	orders     [tagCount][]termRange

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
	exactKeys []exactKey
}

// Builder is the sole mutable construction path for Program.
type Builder struct {
	terms     []Term
	files     []string
	fileIndex map[string]uint32
	spans     [tagCount][]storedSpan
	poison    bool

	valueTerms []Term
	orderTerms []Term
	orders     [tagCount][]termRange

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
	exactKeys   []exactKey
	exactLookup map[exactKey]uint32
}

// NewBuilder returns an empty Program builder.
func NewBuilder() *Builder { return &Builder{} }

func validSpan(span Span) bool {
	return span.Start >= 0 && span.End >= span.Start && uint64(span.Start) <= math.MaxUint32 && uint64(span.End) <= math.MaxUint32
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
	return storedSpan{file: index, start: uint32(span.Start), end: uint32(span.End)}, true
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
	return b.snapshot(), nil
}

func (b *Builder) snapshot() *Program {
	p := &Program{
		terms:      copyTerms(b.terms),
		files:      append([]string(nil), b.files...),
		valueTerms: copyTerms(b.valueTerms),
		orderTerms: copyTerms(b.orderTerms),
		nils:       b.nils,
		bools:      append([]bool(nil), b.bools...),
		integers:   append([]int64(nil), b.integers...),
		floats:     append([]uint64(nil), b.floats...),
		strings:    append([]string(nil), b.strings...),
		values:     append([]valuesRow(nil), b.values...),
		lensExact:  append([]exactLensRow(nil), b.lensExact...),
		lensKeys:   append([]keyLensRow(nil), b.lensKeys...),
		evaluates:  b.evaluates,
		normals:    append([]outcomeRow(nil), b.normals...),
		returns:    append([]outcomeRow(nil), b.returns...),
		throws:     append([]outcomeRow(nil), b.throws...),
		yields:     append([]outcomeRow(nil), b.yields...),
		breaks:     append([]outcomeRow(nil), b.breaks...),
		continues:  append([]outcomeRow(nil), b.continues...),
		mus:        append([]muRow(nil), b.mus...),
		exactKeys:  append([]exactKey(nil), b.exactKeys...),
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
	return Span{File: p.files[row.file], Start: int(row.start), End: int(row.end)}, true
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

func copyTerms(terms []Term) []Term {
	if len(terms) == 0 {
		return nil
	}
	return append([]Term(nil), terms...)
}
