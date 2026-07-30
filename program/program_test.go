package program_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestSealRejectsInvalidReferencesAndExactKeyTypes(t *testing.T) {
	span := program.Span{File: "x.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}

	b := program.NewBuilder()
	b.Values(span, []program.Term{program.Term(0x0101)}, 0)
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted an invalid Values reference")
	}

	b = program.NewBuilder()
	base := b.String(span, "base")
	value := b.Integer(span, 1)
	b.LensExact(span, base, b.Values(span, []program.Term{value}, 0))
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted a non-literal exact Lens key")
	}

	b = program.NewBuilder()
	if term := b.Integer(program.Span{StartLine: -1}, 1); term != 0 {
		t.Fatal("invalid span minted a term")
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("invalid construction did not poison Builder")
	}
}

func TestLiteralsAreDistinctOccurrences(t *testing.T) {
	b := program.NewBuilder()
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

func TestSpanLineColumnContract(t *testing.T) {
	b := program.NewBuilder()
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
		bad := program.NewBuilder()
		if term := bad.Nil(span); term != 0 {
			t.Fatalf("invalid span minted term: %#v", span)
		}
		if _, err := bad.Seal(); err == nil {
			t.Fatalf("invalid span did not poison Builder: %#v", span)
		}
	}
}

func TestLensExactNormalizesKeys(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "x.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
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
		b := program.NewBuilder()
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
		b := program.NewBuilder()
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
		b := program.NewBuilder()
		span := program.Span{File: "rich.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
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
	b := program.NewBuilder()
	span := program.Span{File: "scalar.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
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
		b := program.NewBuilder()
		build(b)
		if _, err := b.Seal(); err == nil {
			t.Fatalf("invalid scalar relation %d sealed", i)
		}
	}

	sealedAllocs := func(rows int) float64 {
		b := program.NewBuilder()
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
	b := program.NewBuilder()
	span := program.Span{File: "x.lua", StartLine: 1, StartCol: 3, EndLine: 1, EndCol: 5}
	base := b.String(span, "base")
	key := b.Integer(span, 3)
	tail := b.String(span, "tail")
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
		b := program.NewBuilder()
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

func TestLexicalCaptureReadAndDelayedMutation(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "closure.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 20}
	outerBody := b.Body(span)
	outerCell := b.Cell(span, outerBody)
	innerBody := b.Body(span)
	innerCell := b.Cell(span, innerBody)
	capture := b.Capture(span, innerCell, outerCell)
	read := b.Read(span, innerCell)
	table := b.Table(span)
	key := b.String(span, "field")
	lens := b.LensExact(span, table, key)
	rhs := b.Values(span, []program.Term{read}, 0)
	assign := b.Assign(span, []program.Term{innerCell, lens}, rhs)
	function := b.Function(span, innerBody, []program.Term{innerCell}, true)
	if !b.SetTable(table, nil, nil, nil) {
		t.Fatal("SetTable failed")
	}
	if !b.SetBody(innerBody, assign) {
		t.Fatal("SetBody(inner) failed")
	}
	if !b.SetBody(outerBody) {
		t.Fatal("SetBody(outer) failed")
	}

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if owner, ok := p.Cell(innerCell); !ok || owner != innerBody {
		t.Fatalf("Cell = %v, %v", owner, ok)
	}
	if cell, ok := p.Read(read); !ok || cell != innerCell {
		t.Fatalf("Read = %v, %v", cell, ok)
	}
	if inner, outer, ok := p.Capture(capture); !ok || inner != innerCell || outer != outerCell {
		t.Fatalf("Capture = %v, %v, %v", inner, outer, ok)
	}
	if body, vararg, ok := p.Function(function); !ok || body != innerBody || !vararg {
		t.Fatalf("Function = %v, %v, %v", body, vararg, ok)
	}
	if formal, ok := p.FormalAt(function, 0); !ok || formal != innerCell {
		t.Fatalf("FormalAt = %v, %v", formal, ok)
	}
	if got, ok := p.AppendOrder(assign, nil); !ok || !reflect.DeepEqual(got, []program.Term{innerCell, lens, rhs}) {
		t.Fatalf("delayed Assign order = %v, %v", got, ok)
	}
	if targets, values, ok := p.AppendAssign(assign, nil); !ok ||
		!reflect.DeepEqual(targets, []program.Term{innerCell, lens}) || values != rhs {
		t.Fatalf("Assign = %v, %v, %v", targets, values, ok)
	}
}

func TestBindRetainsDeclarationVisibilityAndRHSOrder(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "bind.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10}
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

	empty := program.NewBuilder()
	emptyBody := empty.Body(span)
	emptyValues := empty.Values(span, nil, 0)
	emptyBind := empty.Bind(span, nil, emptyValues)
	empty.SetBody(emptyBody, emptyBind)
	if _, err := empty.Seal(); err == nil {
		t.Fatal("Seal accepted Bind without Cells")
	}

	unowned := program.NewBuilder()
	unownedBody := unowned.Body(span)
	unownedCell := unowned.Cell(span, unownedBody)
	unownedValues := unowned.Values(span, nil, 0)
	unowned.Bind(span, []program.Term{unownedCell}, unownedValues)
	unowned.SetBody(unownedBody)
	if _, err := unowned.Seal(); err == nil {
		t.Fatal("Seal accepted Bind without Body ownership")
	}

	crossBody := program.NewBuilder()
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

	rebound := program.NewBuilder()
	reboundBody := rebound.Body(span)
	reboundCell := rebound.Cell(span, reboundBody)
	reboundValues := rebound.Values(span, nil, 0)
	firstBind := rebound.Bind(span, []program.Term{reboundCell}, reboundValues)
	secondBind := rebound.Bind(span, []program.Term{reboundCell}, reboundValues)
	rebound.SetBody(reboundBody, firstBind, secondBind)
	if _, err := rebound.Seal(); err == nil {
		t.Fatal("Seal accepted a Cell bound more than once")
	}

	wrongValues := program.NewBuilder()
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
	b := program.NewBuilder()
	span := program.Span{File: "recursive.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 12}
	body := b.Body(span)
	formal := b.Cell(span, body)
	function := b.Function(span, body, []program.Term{formal}, false)
	argument := b.Integer(span, 1)
	actuals := b.Values(span, []program.Term{argument}, 0)
	callee := b.Read(span, formal)
	call := b.Call(span, callee, actuals, function)
	if !b.SetBody(body, call) {
		t.Fatal("SetBody failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if gotCallee, gotActuals, direct, ok := p.Call(call); !ok ||
		gotCallee != callee || gotActuals != actuals || direct != function {
		t.Fatalf("Call = %v, %v, %v, %v", gotCallee, gotActuals, direct, ok)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("Mu head = %v, %v; want %v, true", head, ok, function)
	}
	if _, ok := p.Mu(call); ok {
		t.Fatal("non-Function term has a Mu annotation")
	}
	if p.TermCount() != 7 {
		t.Fatalf("Seal minted a recurrence Term: TermCount=%d, want 7", p.TermCount())
	}
	if got, ok := p.AppendBody(body, nil); !ok ||
		!reflect.DeepEqual(got, []program.Term{call}) {
		t.Fatalf("Body = %v, %v", got, ok)
	}

	unfilled := program.NewBuilder()
	unfilled.Body(span)
	if _, err := unfilled.Seal(); err == nil {
		t.Fatal("Seal accepted an unfilled Body")
	}

	doubleFill := program.NewBuilder()
	doubleBody := doubleFill.Body(span)
	if !doubleFill.SetBody(doubleBody) || doubleFill.SetBody(doubleBody) {
		t.Fatal("SetBody did not enforce exactly-once fill")
	}
	if _, err := doubleFill.Seal(); err == nil {
		t.Fatal("Seal accepted a multiply-filled Body")
	}
}

func TestMutualDirectCallMuUsesCanonicalExistingHead(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "mutual.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	firstBody := b.Body(span)
	secondBody := b.Body(span)
	firstCalleeCell := b.Cell(span, firstBody)
	secondCalleeCell := b.Cell(span, secondBody)
	first := b.Function(span, firstBody, nil, false)
	second := b.Function(span, secondBody, nil, false)
	firstActuals := b.Values(span, nil, 0)
	secondActuals := b.Values(span, nil, 0)
	firstCallee := b.Read(span, firstCalleeCell)
	secondCallee := b.Read(span, secondCalleeCell)
	firstCall := b.Call(span, firstCallee, firstActuals, second)
	secondCall := b.Call(span, secondCallee, secondActuals, first)
	if !b.SetBody(firstBody, firstCall) || !b.SetBody(secondBody, secondCall) {
		t.Fatal("SetBody failed")
	}
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
	b := program.NewBuilder()
	span := program.Span{File: "nested-call.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	body := b.Body(span)
	calleeCell := b.Cell(span, body)
	function := b.Function(span, body, nil, false)
	actuals := b.Values(span, nil, 0)
	callee := b.Read(span, calleeCell)
	call := b.Call(span, callee, actuals, function)
	values := b.Values(span, []program.Term{call}, 0)
	result := b.Return(span, values)
	if !b.SetBody(body, result) {
		t.Fatal("SetBody failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("nested direct call Mu = %v, %v", head, ok)
	}
}

func TestDirectCallMuFindsCallOnlyInSelectRHS(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "select-recursive.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	body := b.Body(span)
	calleeCell := b.Cell(span, body)
	function := b.Function(span, body, nil, false)
	callee := b.Read(span, calleeCell)
	call := b.Call(span, callee, b.Values(span, nil, 0), function)
	condition := b.Bool(span, false)
	selectTerm := b.Select(span, program.SelectOr, condition, call)
	result := b.Return(span, b.Values(span, []program.Term{selectTerm}, 0))
	if !b.SetBody(body, result) {
		t.Fatal("SetBody failed")
	}
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
	b := program.NewBuilder()
	span := program.Span{File: "acyclic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	callerBody := b.Body(span)
	calleeBody := b.Body(span)
	callerCalleeCell := b.Cell(span, callerBody)
	caller := b.Function(span, callerBody, nil, false)
	callee := b.Function(span, calleeBody, nil, false)
	actuals := b.Values(span, nil, 0)
	calleeRead := b.Read(span, callerCalleeCell)
	call := b.Call(span, calleeRead, actuals, callee)
	if !b.SetBody(callerBody, call) || !b.SetBody(calleeBody) {
		t.Fatal("SetBody failed")
	}
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
		b := program.NewBuilder()
		span := program.Span{File: "chain.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
		bodies := make([]program.Term, functionCount)
		cells := make([]program.Term, functionCount)
		functions := make([]program.Term, functionCount)
		for i := 0; i < functionCount; i++ {
			bodies[i] = b.Body(span)
			cells[i] = b.Cell(span, bodies[i])
			functions[i] = b.Function(span, bodies[i], nil, false)
		}
		for i := 0; i+1 < functionCount; i++ {
			callee := b.Read(span, cells[i])
			actuals := b.Values(span, nil, 0)
			call := b.Call(span, callee, actuals, functions[i+1])
			if !b.SetBody(bodies[i], call) {
				t.Fatal("SetBody failed")
			}
		}
		if !b.SetBody(bodies[functionCount-1]) {
			t.Fatal("SetBody failed")
		}
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
		b := program.NewBuilder()
		span := program.Span{File: "chain-alloc.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
		bodies := make([]program.Term, functionCount)
		cells := make([]program.Term, functionCount)
		functions := make([]program.Term, functionCount)
		for i := 0; i < functionCount; i++ {
			bodies[i] = b.Body(span)
			cells[i] = b.Cell(span, bodies[i])
			functions[i] = b.Function(span, bodies[i], nil, false)
		}
		for i := 0; i+1 < functionCount; i++ {
			call := b.Call(span, b.Read(span, cells[i]), b.Values(span, nil, 0), functions[i+1])
			b.SetBody(bodies[i], call)
		}
		b.SetBody(bodies[functionCount-1])
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

func TestBranchAndTableAssignmentKeys(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "flow.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 9}
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
	numericRHS := b.Values(span, nil, numericValue)
	if !b.SetTable(
		table,
		[]program.Term{stringKey, numericKey},
		[]program.Term{stringRHS, numericRHS},
		[]program.FieldKind{program.FieldExact, program.FieldList},
	) {
		t.Fatal("SetTable failed")
	}
	body := b.Body(span)
	if !b.SetBody(body, branch) {
		t.Fatal("SetBody failed")
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
			body := b.Body(span)
			other := b.Body(span)
			formal := b.Cell(span, other)
			b.Function(span, body, []program.Term{formal}, false)
			b.SetBody(body)
			b.SetBody(other)
		},
		func(b *program.Builder) {
			callee := b.String(span, "f")
			b.Call(span, callee, b.Integer(span, 1), 0)
		},
		func(b *program.Builder) {
			condition := b.Bool(span, true)
			b.Branch(span, condition, b.Integer(span, 1), b.Integer(span, 2))
		},
	}
	for i, build := range tests {
		b := program.NewBuilder()
		build(b)
		if _, err := b.Seal(); err == nil {
			t.Fatalf("invalid family case %d sealed", i)
		}
	}
}

func TestTableConstructionLaws(t *testing.T) {
	span := program.Span{File: "table.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}

	unfilled := program.NewBuilder()
	unfilled.Table(span)
	if _, err := unfilled.Seal(); err == nil {
		t.Fatal("Seal accepted an unfilled Table")
	}

	doubleFill := program.NewBuilder()
	doubleTable := doubleFill.Table(span)
	if !doubleFill.SetTable(doubleTable, nil, nil, nil) ||
		doubleFill.SetTable(doubleTable, nil, nil, nil) {
		t.Fatal("SetTable did not enforce exactly-once fill")
	}
	if _, err := doubleFill.Seal(); err == nil {
		t.Fatal("Seal accepted a multiply-filled Table")
	}

	nilExact := program.NewBuilder()
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

	nanExact := program.NewBuilder()
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

	nonliteralExact := program.NewBuilder()
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

	wrongValues := program.NewBuilder()
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

	invalidKind := program.NewBuilder()
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

	misaligned := program.NewBuilder()
	misalignedTable := misaligned.Table(span)
	if misaligned.SetTable(misalignedTable, []program.Term{1}, nil, nil) {
		t.Fatal("SetTable accepted misaligned fields")
	}
	if _, err := misaligned.Seal(); err == nil {
		t.Fatal("Seal accepted misaligned Table construction")
	}

	dynamic := program.NewBuilder()
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

	nonStatement := program.NewBuilder()
	nonStatementBody := nonStatement.Body(span)
	nestedValue := nonStatement.Integer(span, 1)
	nonStatement.SetBody(nonStatementBody, nestedValue)
	if _, err := nonStatement.Seal(); err == nil {
		t.Fatal("Seal accepted nested evaluation as a Body statement root")
	}
}

func TestNestedBodyOwnershipAndCycles(t *testing.T) {
	span := program.Span{File: "block.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}
	b := program.NewBuilder()
	parent := b.Body(span)
	child := b.Body(span)
	if !b.SetBody(child) || !b.SetBody(parent, child) {
		t.Fatal("nested Body fill failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if owned, ok := p.BodyAt(parent, 0); !ok || owned != child {
		t.Fatalf("nested Body = %v, %v", owned, ok)
	}

	selfCycle := program.NewBuilder()
	self := selfCycle.Body(span)
	selfCycle.SetBody(self, self)
	if _, err := selfCycle.Seal(); err == nil {
		t.Fatal("Seal accepted self-owned Body")
	}

	multiCycle := program.NewBuilder()
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

	duplicate := program.NewBuilder()
	duplicateBody := duplicate.Body(span)
	duplicateValue := duplicate.Integer(span, 1)
	duplicateActuals := duplicate.Values(span, []program.Term{duplicateValue}, 0)
	duplicateTerm := duplicate.Call(span, duplicateValue, duplicateActuals, 0)
	duplicate.SetBody(duplicateBody, duplicateTerm, duplicateTerm)
	if _, err := duplicate.Seal(); err == nil {
		t.Fatal("Seal accepted duplicate ownership within one Body")
	}

	shared := program.NewBuilder()
	firstBody := shared.Body(span)
	secondBody := shared.Body(span)
	sharedValue := shared.Integer(span, 1)
	sharedActuals := shared.Values(span, []program.Term{sharedValue}, 0)
	sharedTerm := shared.Call(span, sharedValue, sharedActuals, 0)
	shared.SetBody(firstBody, sharedTerm)
	shared.SetBody(secondBody, sharedTerm)
	if _, err := shared.Seal(); err == nil {
		t.Fatal("Seal accepted one top-level term in two Bodies")
	}

	ambiguousFunction := program.NewBuilder()
	functionBody := ambiguousFunction.Body(span)
	ambiguousFunction.Function(span, functionBody, nil, false)
	ambiguousFunction.Function(span, functionBody, nil, true)
	ambiguousFunction.SetBody(functionBody)
	if _, err := ambiguousFunction.Seal(); err == nil {
		t.Fatal("Seal accepted two Functions for one Body")
	}

	localCapture := program.NewBuilder()
	localBody := localCapture.Body(span)
	firstCell := localCapture.Cell(span, localBody)
	secondCell := localCapture.Cell(span, localBody)
	localCapture.Capture(span, firstCell, secondCell)
	localCapture.SetBody(localBody)
	if _, err := localCapture.Seal(); err == nil {
		t.Fatal("Seal accepted a Capture within one Body")
	}
}
