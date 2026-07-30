package program_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program"
)

// newProgramBuilder supplies the canonical empty shard entry for tests that
// exercise a relation in isolation. Tests of entry construction use the raw
// builder directly so missing and invalid entry laws remain observable.
func newProgramBuilder(t *testing.T) *program.Builder {
	t.Helper()
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	if !b.SetBody(entry) || !b.SetEntry(entry) {
		t.Fatal("failed to establish test Entry Body")
	}
	return b
}

func newClosureBuilder(t *testing.T, span program.Span) (*program.Builder, program.Term) {
	t.Helper()
	b := program.NewBuilder()
	entry := b.Body(span)
	if !b.SetEntry(entry) {
		t.Fatal("failed to establish closure test Entry Body")
	}
	return b, entry
}

func fillEmptyEntry(t *testing.T, b *program.Builder, entry program.Term) {
	t.Helper()
	if !b.SetBody(entry) {
		t.Fatal("failed to fill closure test Entry Body")
	}
}

func TestSealRejectsInvalidReferencesAndExactKeyTypes(t *testing.T) {
	span := program.Span{File: "x.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}

	b := newProgramBuilder(t)
	b.Values(span, []program.Term{program.Term(0x0101)}, 0)
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted an invalid Values reference")
	}

	b = newProgramBuilder(t)
	base := b.String(span, "base")
	value := b.Integer(span, 1)
	b.LensExact(span, base, b.Values(span, []program.Term{value}, 0))
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted a non-literal exact Lens key")
	}

	b = newProgramBuilder(t)
	b.Values(span, nil, b.Integer(span, 1))
	if _, err := b.Seal(); err == nil || err.Error() != "program: invalid Values tail" {
		t.Fatalf("Seal accepted scalar Values tail: %v", err)
	}

	b = newProgramBuilder(t)
	body := b.Body(span)
	b.Values(span, []program.Term{body}, 0)
	if _, err := b.Seal(); err == nil || err.Error() != "program: invalid Values reference" {
		t.Fatalf("Seal accepted Body in Values: %v", err)
	}

	b = newProgramBuilder(t)
	if term := b.Integer(program.Span{StartLine: -1}, 1); term != 0 {
		t.Fatal("invalid span minted a term")
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("invalid construction did not poison Builder")
	}
}

func TestLiteralsAreDistinctOccurrences(t *testing.T) {
	b := newProgramBuilder(t)
	first := b.Integer(program.Span{File: "x.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, 7)
	second := b.Integer(program.Span{File: "x.lua", StartLine: 1, StartCol: 9, EndLine: 1, EndCol: 10}, 7)
	if first == second || first == 0 || second == 0 {
		t.Fatalf("equal literal occurrences were not distinct: %v %v", first, second)
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Integer(first); !ok || got != 7 {
		t.Fatalf("first literal = %v, %v", got, ok)
	}
	if got, ok := p.Integer(second); !ok || got != 7 {
		t.Fatalf("second literal = %v, %v", got, ok)
	}
	span, ok := p.Span(second)
	if !ok || span.StartCol != 9 {
		t.Fatalf("distinct literal span lost: %#v %v", span, ok)
	}
}

func TestValuePositionsRejectNonValueRelations(t *testing.T) {
	span := program.Span{File: "value-position.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}

	lens := newProgramBuilder(t)
	lens.LensKey(span, lens.Body(span), lens.Integer(span, 1))
	if _, err := lens.Seal(); err == nil || err.Error() != "program: invalid dynamic Lens reference" {
		t.Fatalf("Lens value-position error = %v", err)
	}

	b := program.NewBuilder()
	entryBody := b.Body(span)
	if !b.SetBody(entryBody) || !b.SetEntry(entryBody) {
		t.Fatal("Entry setup failed")
	}
	values := b.Values(span, nil, 0)
	b.Branch(span, entryBody, b.Normal(span, values), b.Normal(span, values))
	if _, err := b.Seal(); err == nil || err.Error() != "program: Branch requires condition and Body/Outcome arms" {
		t.Fatalf("Branch value-position error = %v", err)
	}

	table := program.NewBuilder()
	tableEntry := table.Body(span)
	if !table.SetBody(tableEntry) || !table.SetEntry(tableEntry) {
		t.Fatal("Entry setup failed")
	}
	term := table.Table(span)
	fieldValues := table.Values(span, nil, 0)
	if !table.SetTable(term, []program.Term{tableEntry}, []program.Term{fieldValues}, []program.FieldKind{program.FieldKey}) {
		t.Fatal("SetTable failed")
	}
	if _, err := table.Seal(); err == nil || err.Error() != "program: invalid dynamic Table key" {
		t.Fatalf("Table value-position error = %v", err)
	}
}

func TestSpanLineColumnContract(t *testing.T) {
	b := newProgramBuilder(t)
	generated := b.Nil(program.Span{})
	point := b.Bool(program.Span{File: "x.lua", StartLine: 3, StartCol: 7}, true)
	full := b.Integer(program.Span{File: "x.lua", StartLine: 3, StartCol: 8, EndLine: 4, EndCol: 2}, 1)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Span(generated); !ok || got != (program.Span{}) {
		t.Fatalf("generated span = %#v, %v", got, ok)
	}
	if got, ok := p.Span(point); !ok || got.EndLine != 0 || got.EndCol != 0 {
		t.Fatalf("point span = %#v, %v", got, ok)
	}
	if got, ok := p.Span(full); !ok || got.StartLine != 3 || got.EndLine != 4 || got.EndCol != 2 {
		t.Fatalf("full span = %#v, %v", got, ok)
	}

	invalid := []program.Span{
		{StartLine: 1},
		{StartLine: 1, StartCol: 1, EndLine: 2},
		{StartLine: 2, StartCol: 1, EndLine: 1, EndCol: 1},
	}
	for _, span := range invalid {
		bad := newProgramBuilder(t)
		if term := bad.Nil(span); term != 0 {
			t.Fatalf("invalid span minted term: %#v", span)
		}
		if _, err := bad.Seal(); err == nil {
			t.Fatalf("invalid span did not poison Builder: %#v", span)
		}
	}
}

func TestEntryBodyLaws(t *testing.T) {
	span := program.Span{File: "entry.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}

	missing := program.NewBuilder()
	if _, err := missing.Seal(); err == nil || err.Error() != "program: missing Entry Body" {
		t.Fatalf("missing Entry error = %v", err)
	}

	invalid := program.NewBuilder()
	notBody := invalid.Integer(span, 1)
	if invalid.SetEntry(notBody) {
		t.Fatal("SetEntry accepted a non-Body term")
	}
	if _, err := invalid.Seal(); err == nil || err.Error() != "program: poisoned builder" {
		t.Fatalf("invalid Entry error = %v", err)
	}

	double := program.NewBuilder()
	first := double.Body(span)
	second := double.Body(span)
	if !double.SetBody(first) || !double.SetBody(second) || !double.SetEntry(first) || double.SetEntry(second) {
		t.Fatal("SetEntry did not enforce exactly-once construction")
	}
	if _, err := double.Seal(); err == nil || err.Error() != "program: poisoned builder" {
		t.Fatalf("double Entry error = %v", err)
	}

	nested := program.NewBuilder()
	parent := nested.Body(span)
	child := nested.Body(span)
	if !nested.SetBody(child) || !nested.SetBody(parent, child) || !nested.SetEntry(child) {
		t.Fatal("nested Entry setup failed")
	}
	if _, err := nested.Seal(); err == nil || err.Error() != "program: Entry Body cannot be nested" {
		t.Fatalf("nested Entry error = %v", err)
	}

	function := program.NewBuilder()
	owner := function.Body(span)
	functionBody := function.Body(span)
	fn := function.Function(span, owner, functionBody, 0, nil, 0)
	if !function.SetFunctionCaptures(fn, nil) {
		t.Fatal("Function capture setup failed")
	}
	if !function.SetBody(owner) || !function.SetBody(functionBody) || !function.SetEntry(functionBody) {
		t.Fatal("Function Entry setup failed")
	}
	if _, err := function.Seal(); err == nil || err.Error() != "program: Entry Body cannot be a Function body" {
		t.Fatalf("Function Entry error = %v", err)
	}

	empty := program.NewBuilder()
	entry := empty.Body(span)
	if !empty.SetBody(entry) || !empty.SetEntry(entry) {
		t.Fatal("empty Entry setup failed")
	}
	p, err := empty.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Entry(); !ok || got != entry {
		t.Fatalf("Entry = %v, %v", got, ok)
	}
	if length, ok := p.BodyLen(entry); !ok || length != 0 {
		t.Fatalf("empty Entry length = %d, %v", length, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		hotTerm, hotOK = p.Entry()
	}); allocations != 0 {
		t.Fatalf("Entry query allocated: %.2f", allocations)
	}
}

func TestEntryRejectsFunctionFreeOrphanBody(t *testing.T) {
	span := program.Span{File: "orphan.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b := program.NewBuilder()
	entry := b.Body(span)
	orphan := b.Body(span)
	if !b.SetBody(entry) || !b.SetBody(orphan) || !b.SetEntry(entry) {
		t.Fatal("orphan Body setup failed")
	}
	if _, err := b.Seal(); err == nil || err.Error() != "program: non-Entry Body requires exactly one structural parent" {
		t.Fatalf("function-free orphan Body error = %v", err)
	}
}

func TestLensExactNormalizesKeys(t *testing.T) {
	span := program.Span{File: "x.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	body := b.Body(span)
	base := b.String(span, "base")
	oneInt := b.Integer(span, 1)
	oneFloat := b.Float(span, 1)
	minusZero := b.Float(span, math.Copysign(0, -1))
	zero := b.Integer(span, 0)
	truth := b.Bool(span, true)
	text := b.String(span, "1")
	firstNil := b.Nil(span)
	secondNil := b.Nil(span)
	firstNaN := b.Float(span, math.NaN())
	secondNaN := b.Float(span, math.NaN())

	integerLens := b.LensExact(span, base, oneInt)
	floatLens := b.LensExact(span, base, oneFloat)
	minusZeroLens := b.LensExact(span, base, minusZero)
	zeroLens := b.LensExact(span, base, zero)
	boolLens := b.LensExact(span, base, truth)
	stringLens := b.LensExact(span, base, text)
	firstNilLens := b.LensExact(span, base, firstNil)
	secondNilLens := b.LensExact(span, base, secondNil)
	firstNaNLens := b.LensExact(span, base, firstNaN)
	secondNaNLens := b.LensExact(span, base, secondNaN)
	dynamicLens := b.LensKey(span, base, oneInt)
	writeValues := b.Values(span, nil, 0)
	nilWrite := b.Assign(span, []program.Term{firstNilLens}, writeValues)
	if !b.SetBody(body, nilWrite) {
		t.Fatal("SetBody failed")
	}
	if !b.SetBody(entry, body) {
		t.Fatal("SetBody(entry) failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}

	if !p.SameKey(integerLens, floatLens) {
		t.Fatal("integer and equal float keys were not normalized")
	}
	if !p.SameKey(minusZeroLens, zeroLens) {
		t.Fatal("signed zero keys were not normalized")
	}
	if p.SameKey(integerLens, boolLens) || p.SameKey(integerLens, stringLens) {
		t.Fatal("unequal exact keys compared equal")
	}
	if p.SameKey(integerLens, dynamicLens) {
		t.Fatal("dynamic Lens key compared as exact")
	}
	if !p.SameKey(firstNilLens, secondNilLens) {
		t.Fatal("nil exact key sources did not retain their literal identity class")
	}
	if p.SameKey(firstNaNLens, secondNaNLens) || p.SameKey(firstNaNLens, firstNaNLens) {
		t.Fatal("NaN exact key sources compared equal")
	}
	if _, key, dynamic, ok := p.Lens(firstNilLens); !ok || dynamic || key != firstNil {
		t.Fatalf("nil exact Lens = %v, %v, %v", key, dynamic, ok)
	}
	if got, ok := p.Float(minusZero); !ok || !math.Signbit(got) {
		t.Fatal("literal float bits were normalized instead of preserved")
	}
}

var (
	hotTerm   program.Term
	hotCount  int
	hotOK     bool
	hotProg   *program.Program
	hotUnary  program.UnaryOp
	hotBinary program.BinaryOp
	hotSelect program.SelectOp
)

func TestIndexedQueriesAndPooledSealAllocations(t *testing.T) {
	build := func(rows int) (*program.Program, program.Term, program.Term) {
		b := newProgramBuilder(t)
		span := program.Span{File: "shared.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
		base := b.String(span, "base")
		key := b.Integer(span, 1)
		var values, order program.Term
		for i := 0; i < rows; i++ {
			values = b.Values(span, []program.Term{base, key}, 0)
			order = b.LensKey(span, base, key)
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		return p, values, order
	}
	p, values, order := build(256)
	if got, ok := p.TermAt(p.TermCount() - 1); !ok || got != order {
		t.Fatalf("TermAt = %v, %v", got, ok)
	}
	termBacking := make([]program.Term, p.TermCount())
	terms := p.AppendTerms(termBacking[:0])
	if len(terms) != p.TermCount() || &terms[0] != &termBacking[0] {
		t.Fatal("AppendTerms did not reuse destination capacity")
	}
	if count, ok := p.ValuesLen(values); !ok || count != 2 {
		t.Fatalf("ValuesLen = %d, %v", count, ok)
	}
	if got, ok := p.ValuesAt(values, 1); !ok || got == 0 {
		t.Fatalf("ValuesAt = %v, %v", got, ok)
	}
	if tail, ok := p.ValuesTail(values); !ok || tail != 0 {
		t.Fatalf("ValuesTail = %v, %v", tail, ok)
	}
	if count, ok := p.OrderCount(order); !ok || count != 2 {
		t.Fatalf("OrderCount = %d, %v", count, ok)
	}
	if got, ok := p.OrderAt(order, 0); !ok || got == 0 {
		t.Fatalf("OrderAt = %v, %v", got, ok)
	}
	if got := testing.AllocsPerRun(1000, func() {
		hotTerm, hotOK = p.TermAt(0)
		hotCount, hotOK = p.ValuesLen(values)
		hotTerm, hotOK = p.ValuesAt(values, 0)
		hotTerm, hotOK = p.ValuesTail(values)
		hotCount, hotOK = p.OrderCount(order)
		hotTerm, hotOK = p.OrderAt(order, 0)
	}); got != 0 {
		t.Fatalf("indexed hot-path queries allocated: %.2f", got)
	}
	_ = hotTerm
	_ = hotCount
	_ = hotOK

	// The ratio guards against one retained allocation per Values/order row.
	sealedAllocs := func(rows int) float64 {
		b := newProgramBuilder(t)
		span := program.Span{File: "shared.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
		base := b.String(span, "base")
		key := b.Integer(span, 1)
		for i := 0; i < rows; i++ {
			b.Values(span, []program.Term{base, key}, 0)
			b.LensKey(span, base, key)
		}
		return testing.AllocsPerRun(30, func() {
			var err error
			hotProg, err = b.Seal()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := sealedAllocs(1), sealedAllocs(512)
	if large > small+4 {
		t.Fatalf("Seal allocations grew per relation: small=%.2f large=%.2f", small, large)
	}

	richSealAllocs := func(rows int) float64 {
		span := program.Span{File: "rich.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
		b, entry := newClosureBuilder(t, span)
		body := b.Body(span)
		roots := make([]program.Term, 0, rows*2)
		for i := 0; i < rows; i++ {
			cell := b.Cell(span, body)
			value := b.Integer(span, int64(i))
			values := b.Values(span, []program.Term{value}, 0)
			roots = append(roots, b.Bind(span, []program.Term{cell}, values))
			roots = append(roots, b.Assign(span, []program.Term{cell}, values))
		}
		if !b.SetBody(body, roots...) {
			t.Fatal("SetBody failed")
		}
		if !b.SetBody(entry, body) {
			t.Fatal("SetBody(entry) failed")
		}
		return testing.AllocsPerRun(30, func() {
			var err error
			hotProg, err = b.Seal()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	richSmall, richLarge := richSealAllocs(1), richSealAllocs(512)
	if richLarge > richSmall+4 {
		t.Fatalf("Bind/Body-rich Seal allocations grew per statement: small=%.2f large=%.2f", richSmall, richLarge)
	}
}

func TestScalarRelationsAreTypedAndPreserveEvaluationOrder(t *testing.T) {
	span := program.Span{File: "scalar.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	body := b.Body(span)
	cell := b.Cell(span, body)
	base := b.String(span, "table")
	key := b.String(span, "field")
	lens := b.LensKey(span, base, key)
	cellRead := b.Read(span, cell)
	lensRead := b.Read(span, lens)
	unary := b.Unary(span, program.UnaryBitNot, lensRead)
	binary := b.Binary(span, program.BinaryIDiv, cellRead, unary)
	selectTerm := b.Select(span, program.SelectAnd, binary, lensRead)
	values := b.Values(span, []program.Term{selectTerm}, 0)
	result := b.Return(span, values)
	if !b.SetBody(body, result) {
		t.Fatal("SetBody failed")
	}
	if !b.SetBody(entry, body) {
		t.Fatal("SetBody(entry) failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}

	if source, ok := p.Read(cellRead); !ok || source != cell {
		t.Fatalf("Cell Read = %v, %v", source, ok)
	}
	if source, ok := p.Read(lensRead); !ok || source != lens {
		t.Fatalf("Lens Read = %v, %v", source, ok)
	}
	if count, ok := p.OrderCount(cellRead); !ok || count != 0 {
		t.Fatalf("Cell Read order count = %d, %v", count, ok)
	}
	if order, ok := p.AppendOrder(cellRead, nil); !ok || len(order) != 0 {
		t.Fatalf("Cell Read order = %v, %v", order, ok)
	}
	if order, ok := p.AppendOrder(lensRead, nil); !ok || !reflect.DeepEqual(order, []program.Term{lens}) {
		t.Fatalf("Lens Read order = %v, %v", order, ok)
	}
	if op, operand, ok := p.Unary(unary); !ok || op != program.UnaryBitNot || operand != lensRead {
		t.Fatalf("Unary = %v, %v, %v", op, operand, ok)
	}
	if order, ok := p.AppendOrder(unary, nil); !ok || !reflect.DeepEqual(order, []program.Term{lensRead}) {
		t.Fatalf("Unary order = %v, %v", order, ok)
	}
	if op, left, right, ok := p.Binary(binary); !ok || op != program.BinaryIDiv || left != cellRead || right != unary {
		t.Fatalf("Binary = %v, %v, %v, %v", op, left, right, ok)
	}
	if order, ok := p.AppendOrder(binary, nil); !ok || !reflect.DeepEqual(order, []program.Term{cellRead, unary}) {
		t.Fatalf("Binary order = %v, %v", order, ok)
	}
	if op, left, right, ok := p.Select(selectTerm); !ok || op != program.SelectAnd || left != binary || right != lensRead {
		t.Fatalf("Select = %v, %v, %v, %v", op, left, right, ok)
	}
	if order, ok := p.AppendOrder(selectTerm, nil); !ok || !reflect.DeepEqual(order, []program.Term{binary}) {
		t.Fatalf("Select order = %v, %v", order, ok)
	}

	if allocations := testing.AllocsPerRun(1000, func() {
		hotTerm, hotOK = p.Read(lensRead)
		hotUnary, hotTerm, hotOK = p.Unary(unary)
		hotBinary, hotTerm, _, hotOK = p.Binary(binary)
		hotSelect, hotTerm, _, hotOK = p.Select(selectTerm)
		hotCount, hotOK = p.OrderCount(selectTerm)
		hotTerm, hotOK = p.OrderAt(selectTerm, 0)
	}); allocations != 0 {
		t.Fatalf("scalar queries allocated: %.2f", allocations)
	}
}

func TestScalarRelationsFailClosedAndSealWithoutPerRowAllocation(t *testing.T) {
	span := program.Span{File: "bad-scalar.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	invalid := []func(*program.Builder){
		func(b *program.Builder) { b.Read(span, b.Integer(span, 1)) },
		func(b *program.Builder) { b.Unary(span, program.UnaryOp(255), b.Integer(span, 1)) },
		func(b *program.Builder) {
			b.Binary(span, program.BinaryOp(255), b.Integer(span, 1), b.Integer(span, 2))
		},
		func(b *program.Builder) {
			b.Select(span, program.SelectOp(255), b.Bool(span, true), b.Integer(span, 1))
		},
		func(b *program.Builder) { b.Unary(span, program.UnaryNeg, program.Term(0x0101)) },
		func(b *program.Builder) { b.Binary(span, program.BinaryAdd, b.Integer(span, 1), program.Term(0x0101)) },
		func(b *program.Builder) { b.Select(span, program.SelectOr, program.Term(0x0101), b.Integer(span, 1)) },
	}
	for i, build := range invalid {
		b := newProgramBuilder(t)
		build(b)
		if _, err := b.Seal(); err == nil {
			t.Fatalf("invalid scalar relation %d sealed", i)
		}
	}

	sealedAllocs := func(rows int) float64 {
		b := newProgramBuilder(t)
		span := program.Span{File: "scalar-alloc.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
		left := b.Integer(span, 1)
		right := b.Integer(span, 2)
		for i := 0; i < rows; i++ {
			unary := b.Unary(span, program.UnaryNeg, left)
			binary := b.Binary(span, program.BinaryAdd, unary, right)
			b.Select(span, program.SelectOr, binary, left)
		}
		return testing.AllocsPerRun(30, func() {
			var err error
			hotProg, err = b.Seal()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if small, large := sealedAllocs(64), sealedAllocs(512); large > small+2 {
		t.Fatalf("scalar Seal allocations grew per relation: small=%.2f large=%.2f", small, large)
	}
}

func TestValuesLensOrderAndCopies(t *testing.T) {
	b := newProgramBuilder(t)
	span := program.Span{File: "x.lua", StartLine: 1, StartCol: 3, EndLine: 1, EndCol: 5}
	base := b.String(span, "base")
	key := b.Integer(span, 3)
	tail := b.Call(span, b.String(span, "open"), 0, b.Values(span, nil, 0), 0)
	fixed := []program.Term{base, key}
	values := b.Values(span, fixed, tail)
	fixed[0] = 0
	dynamic := b.LensKey(span, base, key)
	exact := b.LensExact(span, base, key)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}

	gotFixed, gotTail, ok := p.AppendValues(values, make([]program.Term, 0, 2))
	if !ok || !reflect.DeepEqual(gotFixed, []program.Term{base, key}) || gotTail != tail {
		t.Fatalf("Values = %v, %v, %v", gotFixed, gotTail, ok)
	}
	valueBacking := make([]program.Term, 2)
	valueDst, _, ok := p.AppendValues(values, valueBacking[:0])
	if !ok || &valueDst[0] != &valueBacking[0] {
		t.Fatal("AppendValues did not reuse destination capacity")
	}
	if got, ok := p.AppendOrder(values, make([]program.Term, 0, 3)); !ok || !reflect.DeepEqual(got, []program.Term{base, key, tail}) {
		t.Fatalf("Values order = %v", got)
	}
	if got, ok := p.AppendOrder(dynamic, make([]program.Term, 0, 2)); !ok || !reflect.DeepEqual(got, []program.Term{base, key}) {
		t.Fatalf("dynamic Lens order = %v", got)
	}
	if got, ok := p.AppendOrder(exact, make([]program.Term, 0, 1)); !ok || !reflect.DeepEqual(got, []program.Term{base}) {
		t.Fatalf("exact Lens order = %v", got)
	}
}

func TestDeterministicMinting(t *testing.T) {
	build := func() []program.Term {
		b := newProgramBuilder(t)
		span := program.Span{File: "same.lua"}
		base := b.String(span, "base")
		key := b.Integer(span, 1)
		values := b.Values(span, []program.Term{base}, 0)
		b.LensExact(span, base, key)
		b.Return(span, values)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		return p.AppendTerms(nil)
	}
	if !reflect.DeepEqual(build(), build()) {
		t.Fatal("equivalent builders did not mint deterministically")
	}
}

func TestFunctionCaptureVarargAndDirectCall(t *testing.T) {
	span := program.Span{File: "closure.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 20}
	b, entry := newClosureBuilder(t, span)
	binding := b.Cell(span, entry)
	body := b.Body(span)
	formal := b.Cell(span, body)
	varargCell := b.Cell(span, body)
	captured := b.Cell(span, body)
	function := b.Function(span, entry, body, binding, []program.Term{formal}, varargCell)
	capture := b.Capture(span, function, captured, binding)
	if !b.SetFunctionCaptures(function, []program.Term{capture}) {
		t.Fatal("SetFunctionCaptures failed")
	}
	callee := b.Read(span, captured)
	actuals := b.Values(span, nil, 0)
	call := b.Call(span, callee, 0, actuals, function)
	vararg := b.Vararg(span, varargCell)
	result := b.Return(span, b.Values(span, []program.Term{call}, vararg))
	if !b.SetBody(body, result) {
		t.Fatal("SetBody(function) failed")
	}
	fillEmptyEntry(t, b, entry)

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if owner, gotBody, gotBinding, gotVararg, ok := p.Function(function); !ok ||
		owner != entry || gotBody != body || gotBinding != binding || gotVararg != varargCell {
		t.Fatalf("Function = %v, %v, %v, %v, %v", owner, gotBody, gotBinding, gotVararg, ok)
	}
	if got, ok := p.FormalAt(function, 0); !ok || got != formal {
		t.Fatalf("FormalAt = %v, %v", got, ok)
	}
	if count, ok := p.FunctionCaptureLen(function); !ok || count != 1 {
		t.Fatalf("FunctionCaptureLen = %d, %v", count, ok)
	}
	if got, ok := p.FunctionCaptureAt(function, 0); !ok || got != capture {
		t.Fatalf("FunctionCaptureAt = %v, %v", got, ok)
	}
	if gotFunction, inner, outer, ok := p.Capture(capture); !ok || gotFunction != function || inner != captured || outer != binding {
		t.Fatalf("Capture = %v, %v, %v, %v", gotFunction, inner, outer, ok)
	}
	if got, ok := p.Vararg(vararg); !ok || got != varargCell {
		t.Fatalf("Vararg = %v, %v", got, ok)
	}
	if callee, receiver, gotActuals, direct, ok := p.Call(call); !ok ||
		callee == 0 || receiver != 0 || gotActuals != actuals || direct != function {
		t.Fatalf("Call = %v, %v, %v, %v, %v", callee, receiver, gotActuals, direct, ok)
	}
	if order, ok := p.AppendOrder(call, nil); !ok || !reflect.DeepEqual(order, []program.Term{callee, actuals}) {
		t.Fatalf("Call order = %v, %v", order, ok)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("self direct-call Mu = %v, %v", head, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, hotTerm, hotTerm, hotTerm, hotOK = p.Function(function)
		hotCount, hotOK = p.FunctionCaptureLen(function)
		hotTerm, hotOK = p.FunctionCaptureAt(function, 0)
		_, hotTerm, hotTerm, hotOK = p.Capture(capture)
		hotTerm, hotOK = p.Vararg(vararg)
		_, hotTerm, hotTerm, hotTerm, hotOK = p.Call(call)
	}); allocations != 0 {
		t.Fatalf("Function/Capture/Call queries allocated: %.2f", allocations)
	}
}

func TestFunctionCaptureCallLawsFailClosed(t *testing.T) {
	span := program.Span{File: "bad-closure.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	sealError := func(t *testing.T, b *program.Builder, want string) {
		t.Helper()
		if _, err := b.Seal(); err == nil || err.Error() != want {
			t.Fatalf("Seal error = %v, want %q", err, want)
		}
	}

	t.Run("captures must be filled", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		body := b.Body(span)
		b.Function(span, entry, body, 0, nil, 0)
		b.SetBody(body)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Function captures were not filled")
	})
	t.Run("captures fill exactly once", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		body := b.Body(span)
		function := b.Function(span, entry, body, 0, nil, 0)
		if !b.SetFunctionCaptures(function, nil) || b.SetFunctionCaptures(function, nil) {
			t.Fatal("SetFunctionCaptures did not enforce exactly-once fill")
		}
		b.SetBody(body)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: poisoned builder")
	})
	t.Run("function body has one parent", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		body := b.Body(span)
		function := b.Function(span, entry, body, 0, nil, 0)
		b.SetFunctionCaptures(function, nil)
		orphan := b.Body(span)
		b.SetBody(body)
		b.SetBody(orphan)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: non-Entry Body requires exactly one structural parent")
	})
	t.Run("capture inner is not a formal", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		binding := b.Cell(span, entry)
		body := b.Body(span)
		formal := b.Cell(span, body)
		function := b.Function(span, entry, body, 0, []program.Term{formal}, 0)
		capture := b.Capture(span, function, formal, binding)
		b.SetFunctionCaptures(function, []program.Term{capture})
		b.SetBody(body)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Capture inner Cell conflicts with Function local role")
	})
	t.Run("vararg is distinct from formals", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		body := b.Body(span)
		cell := b.Cell(span, body)
		function := b.Function(span, entry, body, 0, []program.Term{cell}, cell)
		b.SetFunctionCaptures(function, nil)
		b.SetBody(body)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Function formal and vararg Cells must be distinct")
	})
	t.Run("capture outer is strict ancestor", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		leftBody := b.Body(span)
		rightBody := b.Body(span)
		leftInner := b.Cell(span, leftBody)
		rightOuter := b.Cell(span, rightBody)
		left := b.Function(span, entry, leftBody, 0, nil, 0)
		right := b.Function(span, entry, rightBody, 0, nil, 0)
		capture := b.Capture(span, left, leftInner, rightOuter)
		b.SetFunctionCaptures(left, []program.Term{capture})
		b.SetFunctionCaptures(right, nil)
		b.SetBody(leftBody)
		b.SetBody(rightBody)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Capture outer Cell must belong to strict lexical ancestor")
	})
	t.Run("binding is unique", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		binding := b.Cell(span, entry)
		firstBody := b.Body(span)
		secondBody := b.Body(span)
		first := b.Function(span, entry, firstBody, binding, nil, 0)
		second := b.Function(span, entry, secondBody, binding, nil, 0)
		b.SetFunctionCaptures(first, nil)
		b.SetFunctionCaptures(second, nil)
		b.SetBody(firstBody)
		b.SetBody(secondBody)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Function binding Cell is not unique")
	})
	t.Run("capture inner has one outer alias", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		firstOuter := b.Cell(span, entry)
		secondOuter := b.Cell(span, entry)
		body := b.Body(span)
		inner := b.Cell(span, body)
		function := b.Function(span, entry, body, 0, nil, 0)
		first := b.Capture(span, function, inner, firstOuter)
		second := b.Capture(span, function, inner, secondOuter)
		b.SetFunctionCaptures(function, []program.Term{first, second})
		b.SetBody(body)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Capture inner Cell has more than one outer alias")
	})
	t.Run("direct Function evidence is mandatory", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		callerBody := b.Body(span)
		calleeBody := b.Body(span)
		caller := b.Function(span, entry, callerBody, 0, nil, 0)
		callee := b.Function(span, entry, calleeBody, 0, nil, 0)
		b.SetFunctionCaptures(caller, nil)
		b.SetFunctionCaptures(callee, nil)
		call := b.Call(span, callee, 0, b.Values(span, nil, 0), 0)
		b.SetBody(callerBody, call)
		b.SetBody(calleeBody)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Function callee requires matching direct target")
	})
	t.Run("Read direct evidence follows capture to binding", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		callerBinding := b.Cell(span, entry)
		calleeBinding := b.Cell(span, entry)
		callerBody := b.Body(span)
		calleeBody := b.Body(span)
		inner := b.Cell(span, callerBody)
		caller := b.Function(span, entry, callerBody, callerBinding, nil, 0)
		callee := b.Function(span, entry, calleeBody, calleeBinding, nil, 0)
		capture := b.Capture(span, caller, inner, callerBinding)
		b.SetFunctionCaptures(caller, []program.Term{capture})
		b.SetFunctionCaptures(callee, nil)
		call := b.Call(span, b.Read(span, inner), 0, b.Values(span, nil, 0), callee)
		b.SetBody(callerBody, call)
		b.SetBody(calleeBody)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Call Cell binding requires matching direct target")
	})
	t.Run("bound Cell Read cannot omit direct evidence", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		binding := b.Cell(span, entry)
		body := b.Body(span)
		inner := b.Cell(span, body)
		function := b.Function(span, entry, body, binding, nil, 0)
		capture := b.Capture(span, function, inner, binding)
		b.SetFunctionCaptures(function, []program.Term{capture})
		b.SetBody(body, b.Call(span, b.Read(span, inner), 0, b.Values(span, nil, 0), 0))
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Call Cell binding requires matching direct target")
	})
	t.Run("unbound Cell Read cannot claim direct evidence", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		binding := b.Cell(span, entry)
		unbound := b.Cell(span, entry)
		callerBody := b.Body(span)
		targetBody := b.Body(span)
		inner := b.Cell(span, callerBody)
		caller := b.Function(span, entry, callerBody, 0, nil, 0)
		target := b.Function(span, entry, targetBody, binding, nil, 0)
		capture := b.Capture(span, caller, inner, unbound)
		b.SetFunctionCaptures(caller, []program.Term{capture})
		b.SetFunctionCaptures(target, nil)
		b.SetBody(callerBody, b.Call(span, b.Read(span, inner), 0, b.Values(span, nil, 0), target))
		b.SetBody(targetBody)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Call Cell binding has no direct Function")
	})
	t.Run("unbound Cell Read remains dynamic without direct evidence", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		unbound := b.Cell(span, entry)
		body := b.Body(span)
		function := b.Function(span, entry, body, 0, nil, 0)
		b.SetFunctionCaptures(function, nil)
		call := b.Call(span, b.Read(span, unbound), 0, b.Values(span, nil, 0), 0)
		b.SetBody(body, call)
		fillEmptyEntry(t, b, entry)
		if _, err := b.Seal(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("Vararg anchors only Function vararg Cell", func(t *testing.T) {
		b, entry := newClosureBuilder(t, span)
		body := b.Body(span)
		cell := b.Cell(span, body)
		function := b.Function(span, entry, body, 0, nil, 0)
		b.SetFunctionCaptures(function, nil)
		b.Vararg(span, cell)
		b.SetBody(body)
		fillEmptyEntry(t, b, entry)
		sealError(t, b, "program: Vararg requires Function vararg Cell")
	})
}

func TestBindRetainsDeclarationVisibilityAndRHSOrder(t *testing.T) {
	span := program.Span{File: "bind.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10}
	b, entry := newClosureBuilder(t, span)
	body := b.Body(span)
	first := b.Cell(span, body)
	second := b.Cell(span, body)
	left := b.Integer(span, 1)
	right := b.Integer(span, 2)
	values := b.Values(span, []program.Term{left, right}, 0)
	bind := b.Bind(span, []program.Term{first, second}, values)
	if !b.SetBody(body, bind) {
		t.Fatal("SetBody failed")
	}
	if !b.SetBody(entry, body) {
		t.Fatal("SetBody(entry) failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := p.BindLen(bind); !ok || count != 2 {
		t.Fatalf("BindLen = %d, %v", count, ok)
	}
	if cell, ok := p.BindAt(bind, 1); !ok || cell != second {
		t.Fatalf("BindAt = %v, %v", cell, ok)
	}
	if cells, gotValues, ok := p.AppendBind(bind, nil); !ok ||
		!reflect.DeepEqual(cells, []program.Term{first, second}) || gotValues != values {
		t.Fatalf("Bind = %v, %v, %v", cells, gotValues, ok)
	}
	if gotValues, ok := p.BindValues(bind); !ok || gotValues != values {
		t.Fatalf("BindValues = %v, %v", gotValues, ok)
	}
	if order, ok := p.AppendOrder(bind, nil); !ok ||
		!reflect.DeepEqual(order, []program.Term{values}) {
		t.Fatalf("Bind order = %v, %v", order, ok)
	}
}

func TestBindLawsFailClosed(t *testing.T) {
	span := program.Span{File: "bad-bind.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}

	empty := newProgramBuilder(t)
	emptyBody := empty.Body(span)
	emptyValues := empty.Values(span, nil, 0)
	emptyBind := empty.Bind(span, nil, emptyValues)
	empty.SetBody(emptyBody, emptyBind)
	if _, err := empty.Seal(); err == nil {
		t.Fatal("Seal accepted Bind without Cells")
	}

	unowned := newProgramBuilder(t)
	unownedBody := unowned.Body(span)
	unownedCell := unowned.Cell(span, unownedBody)
	unownedValues := unowned.Values(span, nil, 0)
	unowned.Bind(span, []program.Term{unownedCell}, unownedValues)
	unowned.SetBody(unownedBody)
	if _, err := unowned.Seal(); err == nil {
		t.Fatal("Seal accepted Bind without Body ownership")
	}

	crossBody := newProgramBuilder(t)
	owner := crossBody.Body(span)
	other := crossBody.Body(span)
	otherCell := crossBody.Cell(span, other)
	crossValues := crossBody.Values(span, nil, 0)
	crossBind := crossBody.Bind(span, []program.Term{otherCell}, crossValues)
	crossBody.SetBody(owner, crossBind)
	crossBody.SetBody(other)
	if _, err := crossBody.Seal(); err == nil {
		t.Fatal("Seal accepted Bind for another Body's Cell")
	}

	rebound := newProgramBuilder(t)
	reboundBody := rebound.Body(span)
	reboundCell := rebound.Cell(span, reboundBody)
	reboundValues := rebound.Values(span, nil, 0)
	firstBind := rebound.Bind(span, []program.Term{reboundCell}, reboundValues)
	secondBind := rebound.Bind(span, []program.Term{reboundCell}, reboundValues)
	rebound.SetBody(reboundBody, firstBind, secondBind)
	if _, err := rebound.Seal(); err == nil {
		t.Fatal("Seal accepted a Cell bound more than once")
	}

	wrongValues := newProgramBuilder(t)
	wrongBody := wrongValues.Body(span)
	wrongCell := wrongValues.Cell(span, wrongBody)
	notValues := wrongValues.Integer(span, 1)
	wrongBind := wrongValues.Bind(span, []program.Term{wrongCell}, notValues)
	wrongValues.SetBody(wrongBody, wrongBind)
	if _, err := wrongValues.Seal(); err == nil {
		t.Fatal("Seal accepted Bind without RHS Values")
	}
}

func TestDirectRecursiveCallMuAndBodyFill(t *testing.T) {
	span := program.Span{File: "recursive.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 12}
	b, entry := newClosureBuilder(t, span)
	body := b.Body(span)
	binding := b.Cell(span, entry)
	inner := b.Cell(span, body)
	function := b.Function(span, entry, body, binding, nil, 0)
	capture := b.Capture(span, function, inner, binding)
	if !b.SetFunctionCaptures(function, []program.Term{capture}) {
		t.Fatal("SetFunctionCaptures failed")
	}
	argument := b.Integer(span, 1)
	actuals := b.Values(span, []program.Term{argument}, 0)
	callee := b.Read(span, inner)
	call := b.Call(span, callee, 0, actuals, function)
	if !b.SetBody(body, call) {
		t.Fatal("SetBody failed")
	}
	fillEmptyEntry(t, b, entry)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if gotCallee, receiver, gotActuals, direct, ok := p.Call(call); !ok ||
		gotCallee != callee || receiver != 0 || gotActuals != actuals || direct != function {
		t.Fatalf("Call = %v, %v, %v, %v, %v", gotCallee, receiver, gotActuals, direct, ok)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("Mu head = %v, %v; want %v, true", head, ok, function)
	}
	if _, ok := p.Mu(call); ok {
		t.Fatal("non-Function term has a Mu annotation")
	}
	if got, ok := p.AppendBody(body, nil); !ok ||
		!reflect.DeepEqual(got, []program.Term{call}) {
		t.Fatalf("Body = %v, %v", got, ok)
	}

	unfilled := newProgramBuilder(t)
	unfilled.Body(span)
	if _, err := unfilled.Seal(); err == nil {
		t.Fatal("Seal accepted an unfilled Body")
	}

	doubleFill := newProgramBuilder(t)
	doubleBody := doubleFill.Body(span)
	if !doubleFill.SetBody(doubleBody) || doubleFill.SetBody(doubleBody) {
		t.Fatal("SetBody did not enforce exactly-once fill")
	}
	if _, err := doubleFill.Seal(); err == nil {
		t.Fatal("Seal accepted a multiply-filled Body")
	}
}

func TestMutualDirectCallMuUsesCanonicalExistingHead(t *testing.T) {
	span := program.Span{File: "mutual.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	firstBody := b.Body(span)
	secondBody := b.Body(span)
	firstBinding := b.Cell(span, entry)
	secondBinding := b.Cell(span, entry)
	firstCalleeCell := b.Cell(span, firstBody)
	secondCalleeCell := b.Cell(span, secondBody)
	first := b.Function(span, entry, firstBody, firstBinding, nil, 0)
	second := b.Function(span, entry, secondBody, secondBinding, nil, 0)
	firstCapture := b.Capture(span, first, firstCalleeCell, secondBinding)
	secondCapture := b.Capture(span, second, secondCalleeCell, firstBinding)
	if !b.SetFunctionCaptures(first, []program.Term{firstCapture}) || !b.SetFunctionCaptures(second, []program.Term{secondCapture}) {
		t.Fatal("SetFunctionCaptures failed")
	}
	firstActuals := b.Values(span, nil, 0)
	secondActuals := b.Values(span, nil, 0)
	firstCallee := b.Read(span, firstCalleeCell)
	secondCallee := b.Read(span, secondCalleeCell)
	firstCall := b.Call(span, firstCallee, 0, firstActuals, second)
	secondCall := b.Call(span, secondCallee, 0, secondActuals, first)
	if !b.SetBody(firstBody, firstCall) || !b.SetBody(secondBody, secondCall) {
		t.Fatal("SetBody failed")
	}
	fillEmptyEntry(t, b, entry)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(first); !ok || head != first {
		t.Fatalf("first Mu head = %v, %v; want %v, true", head, ok, first)
	}
	if head, ok := p.Mu(second); !ok || head != first {
		t.Fatalf("second Mu head = %v, %v; want %v, true", head, ok, first)
	}
}

func TestDirectCallMuFindsCallNestedInValues(t *testing.T) {
	span := program.Span{File: "nested-call.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	body := b.Body(span)
	binding := b.Cell(span, entry)
	calleeCell := b.Cell(span, body)
	function := b.Function(span, entry, body, binding, nil, 0)
	capture := b.Capture(span, function, calleeCell, binding)
	if !b.SetFunctionCaptures(function, []program.Term{capture}) {
		t.Fatal("SetFunctionCaptures failed")
	}
	actuals := b.Values(span, nil, 0)
	callee := b.Read(span, calleeCell)
	call := b.Call(span, callee, 0, actuals, function)
	values := b.Values(span, []program.Term{call}, 0)
	result := b.Return(span, values)
	if !b.SetBody(body, result) {
		t.Fatal("SetBody failed")
	}
	fillEmptyEntry(t, b, entry)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("nested direct call Mu = %v, %v", head, ok)
	}
}

func TestDirectCallMuFindsCallOnlyInSelectRHS(t *testing.T) {
	span := program.Span{File: "select-recursive.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	body := b.Body(span)
	binding := b.Cell(span, entry)
	calleeCell := b.Cell(span, body)
	function := b.Function(span, entry, body, binding, nil, 0)
	capture := b.Capture(span, function, calleeCell, binding)
	if !b.SetFunctionCaptures(function, []program.Term{capture}) {
		t.Fatal("SetFunctionCaptures failed")
	}
	callee := b.Read(span, calleeCell)
	call := b.Call(span, callee, 0, b.Values(span, nil, 0), function)
	condition := b.Bool(span, false)
	selectTerm := b.Select(span, program.SelectOr, condition, call)
	result := b.Return(span, b.Values(span, []program.Term{selectTerm}, 0))
	if !b.SetBody(body, result) {
		t.Fatal("SetBody failed")
	}
	fillEmptyEntry(t, b, entry)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("Select RHS direct-call Mu = %v, %v", head, ok)
	}
	if order, ok := p.AppendOrder(selectTerm, nil); !ok || !reflect.DeepEqual(order, []program.Term{condition}) {
		t.Fatalf("Select order = %v, %v", order, ok)
	}
}

func TestAcyclicDirectCallHasNoMu(t *testing.T) {
	span := program.Span{File: "acyclic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	callerBody := b.Body(span)
	calleeBody := b.Body(span)
	callerBinding := b.Cell(span, entry)
	calleeBinding := b.Cell(span, entry)
	callerCalleeCell := b.Cell(span, callerBody)
	caller := b.Function(span, entry, callerBody, callerBinding, nil, 0)
	callee := b.Function(span, entry, calleeBody, calleeBinding, nil, 0)
	callerCapture := b.Capture(span, caller, callerCalleeCell, calleeBinding)
	if !b.SetFunctionCaptures(caller, []program.Term{callerCapture}) || !b.SetFunctionCaptures(callee, nil) {
		t.Fatal("SetFunctionCaptures failed")
	}
	actuals := b.Values(span, nil, 0)
	calleeRead := b.Read(span, callerCalleeCell)
	call := b.Call(span, calleeRead, 0, actuals, callee)
	if !b.SetBody(callerBody, call) || !b.SetBody(calleeBody) {
		t.Fatal("SetBody failed")
	}
	fillEmptyEntry(t, b, entry)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(caller); ok || head != 0 {
		t.Fatalf("acyclic caller Mu = %v, %v", head, ok)
	}
	if head, ok := p.Mu(callee); ok || head != 0 {
		t.Fatalf("acyclic callee Mu = %v, %v", head, ok)
	}
}

func TestDirectCallMuLongChainStaysIterativeAndAllocationBounded(t *testing.T) {
	build := func(functionCount int) *program.Program {
		span := program.Span{File: "chain.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
		b, entry := newClosureBuilder(t, span)
		bodies := make([]program.Term, functionCount)
		functions := make([]program.Term, functionCount)
		for i := 0; i < functionCount; i++ {
			bodies[i] = b.Body(span)
			functions[i] = b.Function(span, entry, bodies[i], 0, nil, 0)
			if !b.SetFunctionCaptures(functions[i], nil) {
				t.Fatal("SetFunctionCaptures failed")
			}
		}
		for i := 0; i+1 < functionCount; i++ {
			actuals := b.Values(span, nil, 0)
			call := b.Call(span, functions[i+1], 0, actuals, functions[i+1])
			if !b.SetBody(bodies[i], call) {
				t.Fatal("SetBody failed")
			}
		}
		if !b.SetBody(bodies[functionCount-1]) {
			t.Fatal("SetBody failed")
		}
		fillEmptyEntry(t, b, entry)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		for _, function := range functions {
			if head, ok := p.Mu(function); ok || head != 0 {
				t.Fatal("acyclic chain received Mu")
			}
		}
		return p
	}
	_ = build(4096)

	sealedAllocs := func(functionCount int) float64 {
		span := program.Span{File: "chain-alloc.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
		b, entry := newClosureBuilder(t, span)
		bodies := make([]program.Term, functionCount)
		functions := make([]program.Term, functionCount)
		for i := 0; i < functionCount; i++ {
			bodies[i] = b.Body(span)
			functions[i] = b.Function(span, entry, bodies[i], 0, nil, 0)
			b.SetFunctionCaptures(functions[i], nil)
		}
		for i := 0; i+1 < functionCount; i++ {
			call := b.Call(span, functions[i+1], 0, b.Values(span, nil, 0), functions[i+1])
			b.SetBody(bodies[i], call)
		}
		b.SetBody(bodies[functionCount-1])
		fillEmptyEntry(t, b, entry)
		return testing.AllocsPerRun(20, func() {
			var err error
			hotProg, err = b.Seal()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	// Both samples activate the same relation families and prewarm their small
	// backing slices; the 16x comparison catches retained per-row allocation.
	if small, large := sealedAllocs(64), sealedAllocs(512); large > small+2 {
		t.Fatalf("direct-call Seal allocations grew per relation: small=%.2f large=%.2f", small, large)
	}
}

func TestMethodCallRetainsReceiverWithoutDuplicateEvaluation(t *testing.T) {
	span := program.Span{File: "method.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	receiver := b.Table(span)
	if !b.SetTable(receiver, nil, nil, nil) {
		t.Fatal("SetTable failed")
	}
	key := b.String(span, "method")
	lens := b.LensExact(span, receiver, key)
	callee := b.Read(span, lens)
	actuals := b.Values(span, nil, 0)
	call := b.Call(span, callee, receiver, actuals, 0)
	body := b.Body(span)
	if !b.SetBody(body, call) {
		t.Fatal("SetBody failed")
	}
	if !b.SetBody(entry, body) {
		t.Fatal("SetBody(entry) failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if gotCallee, gotReceiver, gotActuals, direct, ok := p.Call(call); !ok ||
		gotCallee != callee || gotReceiver != receiver || gotActuals != actuals || direct != 0 {
		t.Fatalf("Call = %v, %v, %v, %v, %v", gotCallee, gotReceiver, gotActuals, direct, ok)
	}
	if order, ok := p.AppendOrder(call, nil); !ok || !reflect.DeepEqual(order, []program.Term{callee, actuals}) {
		t.Fatalf("Call order = %v, %v", order, ok)
	}
	if order, ok := p.AppendOrder(callee, nil); !ok || !reflect.DeepEqual(order, []program.Term{lens}) {
		t.Fatalf("callee order = %v, %v", order, ok)
	}
	if order, ok := p.AppendOrder(lens, nil); !ok || !reflect.DeepEqual(order, []program.Term{receiver}) {
		t.Fatalf("Lens order = %v, %v", order, ok)
	}
}

func TestMethodCallReceiverMustMatchExactMethodCallee(t *testing.T) {
	span := program.Span{File: "bad-method.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	receiver := b.Table(span)
	other := b.Table(span)
	if !b.SetTable(receiver, nil, nil, nil) || !b.SetTable(other, nil, nil, nil) {
		t.Fatal("SetTable failed")
	}
	method := b.Read(span, b.LensExact(span, other, b.String(span, "method")))
	body := b.Body(span)
	if !b.SetBody(body, b.Call(span, method, receiver, b.Values(span, nil, 0), 0)) || !b.SetBody(entry, body) {
		t.Fatal("SetBody failed")
	}
	if _, err := b.Seal(); err == nil || err.Error() != "program: Call receiver disagrees with method callee" {
		t.Fatalf("method receiver error = %v", err)
	}
}

func TestBranchAndTableAssignmentKeys(t *testing.T) {
	span := program.Span{File: "flow.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 9}
	b, entry := newClosureBuilder(t, span)
	whenTrue := b.Body(span)
	whenFalse := b.Body(span)
	if !b.SetBody(whenTrue) || !b.SetBody(whenFalse) {
		t.Fatal("empty branch Body fill failed")
	}
	condition := b.Bool(span, true)
	branch := b.Branch(span, condition, whenTrue, whenFalse)
	table := b.Table(span)
	stringKey := b.String(span, "name")
	numericKey := b.Integer(span, 1)
	stringValue := b.String(span, "string value")
	numericValue := b.String(span, "numeric value")
	stringRHS := b.Values(span, []program.Term{stringValue}, 0)
	numericRHS := b.Values(span, []program.Term{numericValue}, 0)
	if !b.SetTable(
		table,
		[]program.Term{stringKey, numericKey},
		[]program.Term{stringRHS, numericRHS},
		[]program.FieldKind{program.FieldExact, program.FieldList},
	) {
		t.Fatal("SetTable failed")
	}
	body := b.Body(span)
	if !b.SetBody(body, branch, whenTrue, whenFalse) {
		t.Fatal("SetBody failed")
	}
	if !b.SetBody(entry, body) {
		t.Fatal("SetBody(entry) failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if !p.Table(table) {
		t.Fatal("Table identity was not retained")
	}
	if gotCondition, gotTrue, gotFalse, ok := p.Branch(branch); !ok ||
		gotCondition != condition || gotTrue != whenTrue || gotFalse != whenFalse {
		t.Fatalf("Branch = %v, %v, %v, %v", gotCondition, gotTrue, gotFalse, ok)
	}
	if count, ok := p.TableLen(table); !ok || count != 2 {
		t.Fatalf("TableLen = %d, %v", count, ok)
	}
	if key, values, kind, ok := p.TableAt(table, 1); !ok ||
		key != numericKey || values != numericRHS || kind != program.FieldList {
		t.Fatalf("TableAt = %v, %v, %v, %v", key, values, kind, ok)
	}
	if order, ok := p.AppendOrder(table, nil); !ok ||
		!reflect.DeepEqual(order, []program.Term{stringRHS, numericRHS}) {
		t.Fatalf("Table order = %v, %v", order, ok)
	}
}

func TestNewFamiliesFailClosed(t *testing.T) {
	span := program.Span{File: "bad.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	tests := []func(*program.Builder){
		func(b *program.Builder) { b.Cell(span, b.Integer(span, 1)) },
		func(b *program.Builder) { b.Read(span, b.Integer(span, 1)) },
		func(b *program.Builder) {
			value := b.Integer(span, 1)
			b.Assign(span, []program.Term{value}, b.Values(span, []program.Term{value}, 0))
		},
		func(b *program.Builder) {
			value := b.Integer(span, 1)
			b.Assign(span, nil, b.Values(span, []program.Term{value}, 0))
		},
		func(b *program.Builder) {
			owner := b.Body(span)
			body := b.Body(span)
			other := b.Body(span)
			formal := b.Cell(span, other)
			function := b.Function(span, owner, body, 0, []program.Term{formal}, 0)
			b.SetFunctionCaptures(function, nil)
			b.SetBody(owner)
			b.SetBody(body)
			b.SetBody(other)
		},
		func(b *program.Builder) {
			callee := b.String(span, "f")
			b.Call(span, callee, 0, b.Integer(span, 1), 0)
		},
		func(b *program.Builder) {
			condition := b.Bool(span, true)
			b.Branch(span, condition, b.Integer(span, 1), b.Integer(span, 2))
		},
	}
	for i, build := range tests {
		b := newProgramBuilder(t)
		build(b)
		if _, err := b.Seal(); err == nil {
			t.Fatalf("invalid family case %d sealed", i)
		}
	}
}

func TestTableConstructionLaws(t *testing.T) {
	span := program.Span{File: "table.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}

	unfilled := newProgramBuilder(t)
	unfilled.Table(span)
	if _, err := unfilled.Seal(); err == nil {
		t.Fatal("Seal accepted an unfilled Table")
	}

	doubleFill := newProgramBuilder(t)
	doubleTable := doubleFill.Table(span)
	if !doubleFill.SetTable(doubleTable, nil, nil, nil) ||
		doubleFill.SetTable(doubleTable, nil, nil, nil) {
		t.Fatal("SetTable did not enforce exactly-once fill")
	}
	if _, err := doubleFill.Seal(); err == nil {
		t.Fatal("Seal accepted a multiply-filled Table")
	}

	nilExact := newProgramBuilder(t)
	exactTable := nilExact.Table(span)
	nilKey := nilExact.Nil(span)
	exactValues := nilExact.Values(span, nil, 0)
	nilExact.SetTable(
		exactTable,
		[]program.Term{nilKey},
		[]program.Term{exactValues},
		[]program.FieldKind{program.FieldExact},
	)
	nilProgram, err := nilExact.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if key, values, kind, ok := nilProgram.TableAt(exactTable, 0); !ok ||
		key != nilKey || values != exactValues || kind != program.FieldExact {
		t.Fatalf("nil exact Table field = %v, %v, %v, %v", key, values, kind, ok)
	}

	nanExact := newProgramBuilder(t)
	nanTable := nanExact.Table(span)
	nanKey := nanExact.Float(span, math.NaN())
	nanValues := nanExact.Values(span, nil, 0)
	nanExact.SetTable(
		nanTable,
		[]program.Term{nanKey},
		[]program.Term{nanValues},
		[]program.FieldKind{program.FieldExact},
	)
	nanProgram, err := nanExact.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if key, _, kind, ok := nanProgram.TableAt(nanTable, 0); !ok ||
		key != nanKey || kind != program.FieldExact {
		t.Fatalf("NaN exact Table field = %v, %v, %v", key, kind, ok)
	}

	nonliteralExact := newProgramBuilder(t)
	nonliteralTable := nonliteralExact.Table(span)
	nonliteralKey := nonliteralExact.Values(span, nil, 0)
	nonliteralValues := nonliteralExact.Values(span, nil, 0)
	nonliteralExact.SetTable(
		nonliteralTable,
		[]program.Term{nonliteralKey},
		[]program.Term{nonliteralValues},
		[]program.FieldKind{program.FieldExact},
	)
	if _, err := nonliteralExact.Seal(); err == nil {
		t.Fatal("Seal accepted non-literal exact Table key")
	}

	wrongValues := newProgramBuilder(t)
	wrongTable := wrongValues.Table(span)
	wrongKey := wrongValues.String(span, "key")
	notValues := wrongValues.Integer(span, 1)
	wrongValues.SetTable(
		wrongTable,
		[]program.Term{wrongKey},
		[]program.Term{notValues},
		[]program.FieldKind{program.FieldExact},
	)
	if _, err := wrongValues.Seal(); err == nil {
		t.Fatal("Seal accepted non-Values Table field")
	}

	invalidKind := newProgramBuilder(t)
	kindTable := invalidKind.Table(span)
	kindKey := invalidKind.String(span, "key")
	kindValues := invalidKind.Values(span, nil, 0)
	invalidKind.SetTable(
		kindTable,
		[]program.Term{kindKey},
		[]program.Term{kindValues},
		[]program.FieldKind{0},
	)
	if _, err := invalidKind.Seal(); err == nil {
		t.Fatal("Seal accepted invalid Table field kind")
	}

	misaligned := newProgramBuilder(t)
	misalignedTable := misaligned.Table(span)
	if misaligned.SetTable(misalignedTable, []program.Term{1}, nil, nil) {
		t.Fatal("SetTable accepted misaligned fields")
	}
	if _, err := misaligned.Seal(); err == nil {
		t.Fatal("Seal accepted misaligned Table construction")
	}

	dynamic := newProgramBuilder(t)
	dynamicTable := dynamic.Table(span)
	dynamicKey := dynamic.Integer(span, 1)
	dynamicValue := dynamic.String(span, "value")
	dynamicValues := dynamic.Values(span, []program.Term{dynamicValue}, 0)
	if !dynamic.SetTable(
		dynamicTable,
		[]program.Term{dynamicKey},
		[]program.Term{dynamicValues},
		[]program.FieldKind{program.FieldKey},
	) {
		t.Fatal("dynamic SetTable failed")
	}
	p, err := dynamic.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if order, ok := p.AppendOrder(dynamicTable, nil); !ok ||
		!reflect.DeepEqual(order, []program.Term{dynamicKey, dynamicValues}) {
		t.Fatalf("dynamic Table order = %v, %v", order, ok)
	}

	nonStatement := newProgramBuilder(t)
	nonStatementBody := nonStatement.Body(span)
	nestedValue := nonStatement.Integer(span, 1)
	nonStatement.SetBody(nonStatementBody, nestedValue)
	if _, err := nonStatement.Seal(); err == nil {
		t.Fatal("Seal accepted nested evaluation as a Body statement root")
	}
}

func TestNestedBodyOwnershipAndCycles(t *testing.T) {
	span := program.Span{File: "block.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b, entry := newClosureBuilder(t, span)
	parent := b.Body(span)
	child := b.Body(span)
	if !b.SetBody(child) || !b.SetBody(parent, child) || !b.SetBody(entry, parent) {
		t.Fatal("nested Body fill failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if owned, ok := p.BodyAt(parent, 0); !ok || owned != child {
		t.Fatalf("nested Body = %v, %v", owned, ok)
	}

	selfCycle := newProgramBuilder(t)
	self := selfCycle.Body(span)
	selfCycle.SetBody(self, self)
	if _, err := selfCycle.Seal(); err == nil {
		t.Fatal("Seal accepted self-owned Body")
	}

	multiCycle := newProgramBuilder(t)
	first := multiCycle.Body(span)
	second := multiCycle.Body(span)
	multiCycle.SetBody(first, second)
	multiCycle.SetBody(second, first)
	if _, err := multiCycle.Seal(); err == nil {
		t.Fatal("Seal accepted Body ownership cycle")
	}
}

func TestBodyOwnershipAndClosureIdentityFailClosed(t *testing.T) {
	span := program.Span{File: "ownership.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}

	duplicate := newProgramBuilder(t)
	duplicateBody := duplicate.Body(span)
	duplicateValue := duplicate.Integer(span, 1)
	duplicateActuals := duplicate.Values(span, []program.Term{duplicateValue}, 0)
	duplicateTerm := duplicate.Call(span, duplicateValue, 0, duplicateActuals, 0)
	duplicate.SetBody(duplicateBody, duplicateTerm, duplicateTerm)
	if _, err := duplicate.Seal(); err == nil {
		t.Fatal("Seal accepted duplicate ownership within one Body")
	}

	shared := newProgramBuilder(t)
	firstBody := shared.Body(span)
	secondBody := shared.Body(span)
	sharedValue := shared.Integer(span, 1)
	sharedActuals := shared.Values(span, []program.Term{sharedValue}, 0)
	sharedTerm := shared.Call(span, sharedValue, 0, sharedActuals, 0)
	shared.SetBody(firstBody, sharedTerm)
	shared.SetBody(secondBody, sharedTerm)
	if _, err := shared.Seal(); err == nil {
		t.Fatal("Seal accepted one top-level term in two Bodies")
	}

	ambiguousFunction := newProgramBuilder(t)
	functionOwner := ambiguousFunction.Body(span)
	functionBody := ambiguousFunction.Body(span)
	ambiguousFunction.Function(span, functionOwner, functionBody, 0, nil, 0)
	ambiguousFunction.Function(span, functionOwner, functionBody, 0, nil, 0)
	ambiguousFunction.SetBody(functionOwner)
	ambiguousFunction.SetBody(functionBody)
	if _, err := ambiguousFunction.Seal(); err == nil {
		t.Fatal("Seal accepted two Functions for one Body")
	}

	localCapture, captureEntry := newClosureBuilder(t, span)
	localBody := localCapture.Body(span)
	firstCell := localCapture.Cell(span, localBody)
	secondCell := localCapture.Cell(span, localBody)
	function := localCapture.Function(span, captureEntry, localBody, 0, nil, 0)
	capture := localCapture.Capture(span, function, firstCell, secondCell)
	localCapture.SetFunctionCaptures(function, []program.Term{capture})
	localCapture.SetBody(localBody)
	fillEmptyEntry(t, localCapture, captureEntry)
	if _, err := localCapture.Seal(); err == nil {
		t.Fatal("Seal accepted a Capture within one Body")
	}
}
