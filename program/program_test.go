package program_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestSealRejectsInvalidReferencesAndExactKeyTypes(t *testing.T) {
	span := program.Span{File: "x.lua", Start: 1, End: 2}

	b := program.NewBuilder()
	b.Values(span, []program.Term{program.Term(0x0101)}, 0)
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted an invalid Values reference")
	}

	b = program.NewBuilder()
	base := b.String(span, "base")
	b.LensExact(span, base, b.Nil(span))
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted nil as an exact Lens key")
	}

	b = program.NewBuilder()
	base = b.String(span, "base")
	b.LensExact(span, base, b.Float(span, math.NaN()))
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted NaN as an exact Lens key")
	}

	b = program.NewBuilder()
	base = b.String(span, "base")
	value := b.Integer(span, 1)
	b.LensExact(span, base, b.Values(span, []program.Term{value}, 0))
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted a non-literal exact Lens key")
	}

	b = program.NewBuilder()
	if term := b.Integer(program.Span{Start: -1, End: 0}, 1); term != 0 {
		t.Fatal("invalid span minted a term")
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("invalid construction did not poison Builder")
	}
}

func TestLiteralsAreDistinctOccurrences(t *testing.T) {
	b := program.NewBuilder()
	first := b.Integer(program.Span{File: "x.lua", Start: 1, End: 2}, 7)
	second := b.Integer(program.Span{File: "x.lua", Start: 9, End: 10}, 7)
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
	if !ok || span.Start != 9 {
		t.Fatalf("distinct literal span lost: %#v %v", span, ok)
	}
}

func TestLensExactNormalizesKeys(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "x.lua", Start: 0, End: 1}
	base := b.String(span, "base")
	oneInt := b.Integer(span, 1)
	oneFloat := b.Float(span, 1)
	minusZero := b.Float(span, math.Copysign(0, -1))
	zero := b.Integer(span, 0)
	truth := b.Bool(span, true)
	text := b.String(span, "1")

	integerLens := b.LensExact(span, base, oneInt)
	floatLens := b.LensExact(span, base, oneFloat)
	minusZeroLens := b.LensExact(span, base, minusZero)
	zeroLens := b.LensExact(span, base, zero)
	boolLens := b.LensExact(span, base, truth)
	stringLens := b.LensExact(span, base, text)
	dynamicLens := b.LensKey(span, base, oneInt)
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
	if got, ok := p.Float(minusZero); !ok || !math.Signbit(got) {
		t.Fatal("literal float bits were normalized instead of preserved")
	}
}

var (
	hotTerm  program.Term
	hotCount int
	hotOK    bool
	hotProg  *program.Program
)

func TestIndexedQueriesAndPooledSealAllocations(t *testing.T) {
	build := func(rows int) (*program.Program, program.Term, program.Term) {
		b := program.NewBuilder()
		span := program.Span{File: "shared.lua", Start: 1, End: 2}
		base := b.String(span, "base")
		key := b.Integer(span, 1)
		var values, order program.Term
		for i := 0; i < rows; i++ {
			values = b.Values(span, []program.Term{base, key}, 0)
			order = b.Evaluate(span, base, key)
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
		span := program.Span{File: "shared.lua", Start: 1, End: 2}
		base := b.String(span, "base")
		key := b.Integer(span, 1)
		for i := 0; i < rows; i++ {
			b.Values(span, []program.Term{base, key}, 0)
			b.Evaluate(span, base, key)
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
}

func TestValuesLensMuOrderAndCopies(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "x.lua", Start: 3, End: 5}
	base := b.String(span, "base")
	key := b.Integer(span, 3)
	tail := b.String(span, "tail")
	fixed := []program.Term{base, key}
	values := b.Values(span, fixed, tail)
	fixed[0] = 0
	dynamic := b.LensKey(span, base, key)
	exact := b.LensExact(span, base, key)
	returnTerm := b.Return(span, values)
	mu := b.Mu(span, returnTerm, dynamic)
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
	orderBacking := make([]program.Term, 2)
	order, ok := p.AppendOrder(mu, orderBacking[:0])
	if !ok || &order[0] != &orderBacking[0] || order[0] != returnTerm {
		t.Fatal("AppendOrder did not reuse destination capacity")
	}
	head, back, ok := p.Mu(mu)
	if !ok || head != returnTerm || back != dynamic {
		t.Fatalf("Mu = %v, %v, %v", head, back, ok)
	}
}

func TestDeterministicMinting(t *testing.T) {
	build := func() []program.Term {
		b := program.NewBuilder()
		span := program.Span{File: "same.lua", Start: 0, End: 0}
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
	span := program.Span{File: "closure.lua", Start: 1, End: 20}
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
	if !b.SetBody(innerBody, capture, read, assign) {
		t.Fatal("SetBody(inner) failed")
	}
	if !b.SetBody(outerBody, outerCell, table, lens, function) {
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

func TestDirectRecursiveCallMuAndBodyFill(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "recursive.lua", Start: 0, End: 12}
	body := b.Body(span)
	formal := b.Cell(span, body)
	function := b.Function(span, body, []program.Term{formal}, false)
	argument := b.Integer(span, 1)
	actuals := b.Values(span, []program.Term{argument}, 0)
	call := b.Call(span, function, actuals)
	mu := b.Mu(span, function, call)
	if !b.SetBody(body, formal, call, mu) {
		t.Fatal("SetBody failed")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if callee, gotActuals, ok := p.Call(call); !ok || callee != function || gotActuals != actuals {
		t.Fatalf("Call = %v, %v, %v", callee, gotActuals, ok)
	}
	if head, back, ok := p.Mu(mu); !ok || head != function || back != call {
		t.Fatalf("Mu = %v, %v, %v", head, back, ok)
	}
	if got, ok := p.AppendBody(body, nil); !ok ||
		!reflect.DeepEqual(got, []program.Term{formal, call, mu}) {
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

func TestBranchAndTableAssignmentKeys(t *testing.T) {
	b := program.NewBuilder()
	span := program.Span{File: "flow.lua", Start: 0, End: 9}
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
	stringLens := b.LensExact(span, table, stringKey)
	numericLens := b.LensExact(span, table, numericKey)
	value := b.String(span, "value")
	rhs := b.Values(span, []program.Term{value}, 0)
	assign := b.Assign(span, []program.Term{stringLens, numericLens}, rhs)
	body := b.Body(span)
	if !b.SetBody(body, table, stringLens, numericLens, assign, branch) {
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
	if count, ok := p.AssignLen(assign); !ok || count != 2 {
		t.Fatalf("AssignLen = %d, %v", count, ok)
	}
}

func TestNewFamiliesFailClosed(t *testing.T) {
	span := program.Span{File: "bad.lua", Start: 0, End: 1}
	tests := []func(*program.Builder){
		func(b *program.Builder) { b.Cell(span, b.Integer(span, 1)) },
		func(b *program.Builder) { b.Read(span, b.Integer(span, 1)) },
		func(b *program.Builder) {
			value := b.Integer(span, 1)
			b.Assign(span, []program.Term{value}, b.Values(span, []program.Term{value}, 0))
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
			b.Call(span, callee, b.Integer(span, 1))
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

func TestBodyOwnershipAndClosureIdentityFailClosed(t *testing.T) {
	span := program.Span{File: "ownership.lua", Start: 0, End: 1}

	duplicate := program.NewBuilder()
	duplicateBody := duplicate.Body(span)
	duplicateTerm := duplicate.Integer(span, 1)
	duplicate.SetBody(duplicateBody, duplicateTerm, duplicateTerm)
	if _, err := duplicate.Seal(); err == nil {
		t.Fatal("Seal accepted duplicate ownership within one Body")
	}

	shared := program.NewBuilder()
	firstBody := shared.Body(span)
	secondBody := shared.Body(span)
	sharedTerm := shared.Integer(span, 1)
	shared.SetBody(firstBody, sharedTerm)
	shared.SetBody(secondBody, sharedTerm)
	if _, err := shared.Seal(); err == nil {
		t.Fatal("Seal accepted one top-level term in two Bodies")
	}

	ambiguousFunction := program.NewBuilder()
	functionBody := ambiguousFunction.Body(span)
	firstFunction := ambiguousFunction.Function(span, functionBody, nil, false)
	secondFunction := ambiguousFunction.Function(span, functionBody, nil, true)
	ambiguousFunction.SetBody(functionBody, firstFunction, secondFunction)
	if _, err := ambiguousFunction.Seal(); err == nil {
		t.Fatal("Seal accepted two Functions for one Body")
	}

	localCapture := program.NewBuilder()
	localBody := localCapture.Body(span)
	firstCell := localCapture.Cell(span, localBody)
	secondCell := localCapture.Cell(span, localBody)
	capture := localCapture.Capture(span, firstCell, secondCell)
	localCapture.SetBody(localBody, firstCell, secondCell, capture)
	if _, err := localCapture.Seal(); err == nil {
		t.Fatal("Seal accepted a Capture within one Body")
	}
}
