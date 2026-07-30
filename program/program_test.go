package program_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/program"
)

var (
	queryTerm      program.Term
	queryTerm2     program.Term
	queryTerm3     program.Term
	queryTerm4     program.Term
	queryTerm5     program.Term
	queryKey       program.Key
	queryFieldKind program.FieldKind
	queryUnary     program.UnaryOp
	queryBinary    program.BinaryOp
	querySelect    program.SelectOp
	querySpan      program.Span
	queryInt       int
	queryInt64     int64
	queryFloat64   float64
	queryString    string
	queryBool      bool
	queryOK        bool
	queryProgram   *program.Program
)

func entryBuilder(t *testing.T) (*program.Builder, program.Term) {
	t.Helper()
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	if !b.SetEntry(entry) {
		t.Fatal("SetEntry")
	}
	return b, entry
}

func TestTypedContainmentAndOwners(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	if !b.SetEntry(entry) {
		t.Fatal("SetEntry")
	}
	value := b.Integer(program.Span{}, entry, 7)
	values := b.Values(program.Span{}, entry, []program.Term{value}, 0)
	result := b.Normal(program.Span{}, entry, values)
	if !b.SetBody(entry, result) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	owner, got, ok := p.Integer(value)
	if !ok || owner != entry || got != 7 {
		t.Fatalf("Integer = %v, %v, %v", owner, got, ok)
	}
	owner, tail, ok := p.Values(values)
	if !ok || owner != entry || tail != 0 {
		t.Fatalf("Values = %v, %v, %v", owner, tail, ok)
	}
	owner, gotValues, ok := p.Normal(result)
	if !ok || owner != entry || gotValues != values {
		t.Fatalf("Normal = %v, %v, %v", owner, gotValues, ok)
	}
}

func TestEntryAndSpanContractsFailClosed(t *testing.T) {
	t.Run("valid spans round trip", func(t *testing.T) {
		b, entry := entryBuilder(t)
		generated := b.Nil(program.Span{}, entry)
		point := b.Bool(program.Span{File: "span.lua", StartLine: 3, StartCol: 7}, entry, true)
		full := b.Integer(program.Span{File: "span.lua", StartLine: 3, StartCol: 8, EndLine: 4, EndCol: 2}, entry, 1)
		values := b.Values(program.Span{}, entry, []program.Term{generated, point, full}, 0)
		b.SetBody(entry, b.Normal(program.Span{}, entry, values))
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := p.Span(generated); !ok || got != (program.Span{}) {
			t.Fatalf("generated Span = %#v, %v", got, ok)
		}
		if got, ok := p.Span(point); !ok || got.File != "span.lua" || got.StartLine != 3 || got.StartCol != 7 || got.EndLine != 0 || got.EndCol != 0 {
			t.Fatalf("point Span = %#v, %v", got, ok)
		}
		if got, ok := p.Span(full); !ok || got.File != "span.lua" || got.StartLine != 3 || got.StartCol != 8 || got.EndLine != 4 || got.EndCol != 2 {
			t.Fatalf("full Span = %#v, %v", got, ok)
		}
	})

	invalidSpans := []struct {
		name string
		span program.Span
	}{
		{name: "negative line", span: program.Span{StartLine: -1}},
		{name: "negative column", span: program.Span{StartCol: -1}},
		{name: "missing start column", span: program.Span{StartLine: 1}},
		{name: "missing start line", span: program.Span{StartCol: 1}},
		{name: "missing end column", span: program.Span{StartLine: 1, StartCol: 1, EndLine: 2}},
		{name: "missing end line", span: program.Span{StartLine: 1, StartCol: 1, EndCol: 2}},
		{name: "reversed lines", span: program.Span{StartLine: 2, StartCol: 1, EndLine: 1, EndCol: 1}},
		{name: "reversed columns", span: program.Span{StartLine: 2, StartCol: 2, EndLine: 2, EndCol: 1}},
	}
	for _, test := range invalidSpans {
		t.Run(test.name, func(t *testing.T) {
			b, entry := entryBuilder(t)
			if term := b.Nil(test.span, entry); term != 0 {
				t.Fatalf("invalid span minted %v", term)
			}
			if _, err := b.Seal(); err == nil {
				t.Fatal("invalid span did not poison Builder")
			}
		})
	}

	t.Run("missing entry", func(t *testing.T) {
		if _, err := program.NewBuilder().Seal(); err == nil {
			t.Fatal("missing Entry was accepted")
		}
	})
	t.Run("entry must be Body", func(t *testing.T) {
		b := program.NewBuilder()
		body := b.Body(program.Span{})
		value := b.Integer(program.Span{}, body, 1)
		if b.SetEntry(value) {
			t.Fatal("SetEntry accepted non-Body")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("invalid Entry did not poison Builder")
		}
	})
	t.Run("entry is one shot", func(t *testing.T) {
		b := program.NewBuilder()
		first, second := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetBody(first)
		b.SetBody(second)
		if !b.SetEntry(first) || b.SetEntry(second) {
			t.Fatal("SetEntry one-shot law failed")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("second Entry did not poison Builder")
		}
	})
	t.Run("entry must be filled once", func(t *testing.T) {
		b, entry := entryBuilder(t)
		if _, err := b.Seal(); err == nil {
			t.Fatal("unfilled Entry was accepted")
		}

		b, entry = entryBuilder(t)
		if !b.SetBody(entry) || b.SetBody(entry) {
			t.Fatal("SetBody one-shot law failed")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("second Body fill did not poison Builder")
		}
	})
	t.Run("entry cannot be nested", func(t *testing.T) {
		b := program.NewBuilder()
		parent, child := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(child)
		b.SetBody(child)
		b.SetBody(parent, child)
		if _, err := b.Seal(); err == nil {
			t.Fatal("nested Entry was accepted")
		}
	})
	t.Run("entry cannot be Function body", func(t *testing.T) {
		b := program.NewBuilder()
		owner, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(body)
		fn := b.Function(program.Span{}, owner, body, nil, 0, nil)
		bodyValues := b.Values(program.Span{}, body, nil, 0)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		fnValues := b.Values(program.Span{}, owner, []program.Term{fn}, 0)
		b.SetBody(owner, b.Normal(program.Span{}, owner, fnValues))
		if _, err := b.Seal(); err == nil {
			t.Fatal("Function Body was accepted as Entry")
		}
	})
	t.Run("empty entry queries do not allocate", func(t *testing.T) {
		b, entry := entryBuilder(t)
		b.SetBody(entry)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := p.Entry(); !ok || got != entry {
			t.Fatalf("Entry = %v, %v", got, ok)
		}
		if count, ok := p.BodyLen(entry); !ok || count != 0 {
			t.Fatalf("BodyLen = %d, %v", count, ok)
		}
		if allocations := testing.AllocsPerRun(1000, func() {
			queryTerm, queryOK = p.Entry()
			queryInt, queryOK = p.BodyLen(entry)
		}); allocations != 0 {
			t.Fatalf("Entry queries allocated: %g", allocations)
		}
	})
}

func TestFieldSyntaxPreservesEvaluationKind(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	name := b.Name(program.Span{}, entry, "x")
	fieldLiteral := b.Integer(program.Span{}, entry, 1)
	fieldValue := b.Values(program.Span{}, entry, []program.Term{fieldLiteral}, 0)
	table := b.Table(program.Span{}, entry, []program.Term{name}, []program.Term{fieldValue}, []program.FieldKind{program.FieldName})
	values := b.Values(program.Span{}, entry, []program.Term{table}, 0)
	result := b.Normal(program.Span{}, entry, values)
	b.SetBody(entry, result)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if p.StringCount() != 0 {
		t.Fatal("static Name leaked into String family")
	}
	owner, ok := p.Table(table)
	if !ok || owner != entry {
		t.Fatalf("Table owner = %v, %v", owner, ok)
	}
	source, _, kind, _, ok := p.Field(table, 0)
	if !ok || source != name || kind != program.FieldName {
		t.Fatalf("Field = %v, %v, %v", source, kind, ok)
	}
}

func TestConstructorRejectsForgedFutureChild(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	if got := b.Values(program.Span{}, entry, []program.Term{program.Term(1<<8 | 3)}, 0); got != 0 {
		t.Fatalf("Values accepted forged child %v", got)
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("forged child did not poison builder")
	}
}

func TestLiteralOccurrencesAndInputRangesRemainDistinct(t *testing.T) {
	b, entry := entryBuilder(t)
	first := b.Integer(
		program.Span{File: "literal.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2},
		entry,
		7,
	)
	second := b.Integer(
		program.Span{File: "literal.lua", StartLine: 1, StartCol: 9, EndLine: 1, EndCol: 10},
		entry,
		7,
	)
	if first == 0 || second == 0 || first == second {
		t.Fatalf("equal literal occurrences collapsed: %v, %v", first, second)
	}
	fixed := []program.Term{first, second}
	values := b.Values(program.Span{}, entry, fixed, 0)
	fixed[0], fixed[1] = 0, 0
	b.SetBody(entry, b.Normal(program.Span{}, entry, values))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Value(values, 0); !ok || got != first {
		t.Fatalf("first Value = %v, %v", got, ok)
	}
	if got, ok := p.Value(values, 1); !ok || got != second {
		t.Fatalf("second Value = %v, %v", got, ok)
	}
	if _, got, ok := p.Integer(first); !ok || got != 7 {
		t.Fatalf("first Integer = %d, %v", got, ok)
	}
	if _, got, ok := p.Integer(second); !ok || got != 7 {
		t.Fatalf("second Integer = %d, %v", got, ok)
	}
	if span, ok := p.Span(second); !ok || span.StartCol != 9 {
		t.Fatalf("second Span = %#v, %v", span, ok)
	}
}

func TestTypedConstructorsRejectWrongRelationFamilies(t *testing.T) {
	tests := []struct {
		name  string
		build func(*program.Builder, program.Term) program.Term
	}{
		{
			name: "Values fixed Body",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Values(program.Span{}, owner, []program.Term{owner}, 0)
			},
		},
		{
			name: "Values scalar tail",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Values(program.Span{}, owner, nil, b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "exact Lens nonliteral key",
			build: func(b *program.Builder, owner program.Term) program.Term {
				base := b.Table(program.Span{}, owner, nil, nil, nil)
				key := b.Values(program.Span{}, owner, nil, 0)
				return b.LensExact(program.Span{}, owner, base, key, program.FieldExact)
			},
		},
		{
			name: "dynamic Lens nonvalue base",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.Integer(program.Span{}, owner, 1)
				return b.LensKey(program.Span{}, owner, owner, key)
			},
		},
		{
			name: "Call nonvalue callee",
			build: func(b *program.Builder, owner program.Term) program.Term {
				actuals := b.Values(program.Span{}, owner, nil, 0)
				return b.Call(program.Span{}, owner, owner, 0, actuals)
			},
		},
		{
			name: "Call actuals",
			build: func(b *program.Builder, owner program.Term) program.Term {
				callee := b.Table(program.Span{}, owner, nil, nil, nil)
				return b.Call(program.Span{}, owner, callee, 0, b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "Assign requires target",
			build: func(b *program.Builder, owner program.Term) program.Term {
				values := b.Values(program.Span{}, owner, nil, 0)
				return b.Assign(program.Span{}, owner, nil, values)
			},
		},
		{
			name: "Assign target family",
			build: func(b *program.Builder, owner program.Term) program.Term {
				target := b.Integer(program.Span{}, owner, 1)
				values := b.Values(program.Span{}, owner, nil, 0)
				return b.Assign(program.Span{}, owner, []program.Term{target}, values)
			},
		},
		{
			name: "Assign Values",
			build: func(b *program.Builder, owner program.Term) program.Term {
				cell := b.Cell(program.Span{}, owner)
				return b.Assign(program.Span{}, owner, []program.Term{cell}, b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "Branch condition",
			build: func(b *program.Builder, owner program.Term) program.Term {
				values := b.Values(program.Span{}, owner, nil, 0)
				arm := b.Normal(program.Span{}, owner, values)
				return b.Branch(program.Span{}, owner, owner, arm, arm)
			},
		},
		{
			name: "Branch arm",
			build: func(b *program.Builder, owner program.Term) program.Term {
				condition := b.Bool(program.Span{}, owner, true)
				return b.Branch(program.Span{}, owner, condition, b.Integer(program.Span{}, owner, 1), owner)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, entry := entryBuilder(t)
			if term := test.build(b, entry); term != 0 {
				t.Fatalf("invalid relation minted %v", term)
			}
			if _, err := b.Seal(); err == nil {
				t.Fatal("invalid relation did not poison Builder")
			}
		})
	}
}

func TestBranchBodyIsStructuralAuthority(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	whenTrue := b.Body(program.Span{})
	whenFalse := b.Body(program.Span{})
	b.SetEntry(entry)
	condition := b.Bool(program.Span{}, entry, true)
	trueValues := b.Values(program.Span{}, whenTrue, nil, 0)
	falseValues := b.Values(program.Span{}, whenFalse, nil, 0)
	trueResult := b.Normal(program.Span{}, whenTrue, trueValues)
	falseResult := b.Normal(program.Span{}, whenFalse, falseValues)
	b.SetBody(whenTrue, trueResult)
	b.SetBody(whenFalse, falseResult)
	branch := b.Branch(program.Span{}, entry, condition, whenTrue, whenFalse)
	b.SetBody(entry, branch)
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestExactKeysNormalizeWithoutSharingOccurrences(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	leftBase := b.Table(program.Span{}, entry, nil, nil, nil)
	rightBase := b.Table(program.Span{}, entry, nil, nil, nil)
	minusZero := b.Float(program.Span{}, entry, -0.0)
	integerZero := b.Integer(program.Span{}, entry, 0)
	left := b.LensExact(program.Span{}, entry, leftBase, minusZero, program.FieldExact)
	right := b.LensExact(program.Span{}, entry, rightBase, integerZero, program.FieldExact)
	if leftBase == 0 || rightBase == 0 || minusZero == 0 || integerZero == 0 || left == 0 || right == 0 {
		t.Fatalf("mint failed: %v %v %v %v %v %v", leftBase, rightBase, minusZero, integerZero, left, right)
	}
	leftValues := b.Values(program.Span{}, entry, nil, 0)
	rightValues := b.Values(program.Span{}, entry, nil, 0)
	leftAssign := b.Assign(program.Span{}, entry, []program.Term{left}, leftValues)
	rightAssign := b.Assign(program.Span{}, entry, []program.Term{right}, rightValues)
	if !b.SetBody(entry, leftAssign, rightAssign) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, leftKey, _ := p.Lens(left)
	_, _, _, _, rightKey, _ := p.Lens(right)
	if leftKey == 0 || leftKey != rightKey {
		t.Fatal("-0 and integer 0 must normalize to one exact key")
	}
}

func TestExactKeyEqualityClasses(t *testing.T) {
	b, entry := entryBuilder(t)
	keys := []program.Term{
		b.Integer(program.Span{}, entry, 1),
		b.Float(program.Span{}, entry, 1),
		b.Bool(program.Span{}, entry, true),
		b.String(program.Span{}, entry, "true"),
	}
	lenses := make([]program.Term, len(keys))
	roots := make([]program.Term, len(keys))
	for i, key := range keys {
		base := b.Table(program.Span{}, entry, nil, nil, nil)
		lenses[i] = b.LensExact(program.Span{}, entry, base, key, program.FieldExact)
		values := b.Values(program.Span{}, entry, nil, 0)
		roots[i] = b.Assign(program.Span{}, entry, []program.Term{lenses[i]}, values)
	}
	b.SetBody(entry, roots...)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	normalized := make([]program.Key, len(lenses))
	for i, lens := range lenses {
		_, _, _, _, normalized[i], _ = p.Lens(lens)
	}
	if normalized[0] == 0 || normalized[0] != normalized[1] {
		t.Fatalf("integer/float equality class = %v", normalized)
	}
	if normalized[2] == 0 || normalized[3] == 0 || normalized[2] == normalized[3] || normalized[0] == normalized[2] || normalized[0] == normalized[3] {
		t.Fatalf("typed exact-key classes collapsed: %v", normalized)
	}
}

func TestNilExactKeysRemainDistinctSources(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	leftBase, rightBase := b.Table(program.Span{}, entry, nil, nil, nil), b.Table(program.Span{}, entry, nil, nil, nil)
	leftNil, rightNil := b.Nil(program.Span{}, entry), b.Nil(program.Span{}, entry)
	left := b.LensExact(program.Span{}, entry, leftBase, leftNil, program.FieldExact)
	right := b.LensExact(program.Span{}, entry, rightBase, rightNil, program.FieldExact)
	leftValues, rightValues := b.Values(program.Span{}, entry, nil, 0), b.Values(program.Span{}, entry, nil, 0)
	b.SetBody(entry, b.Assign(program.Span{}, entry, []program.Term{left}, leftValues), b.Assign(program.Span{}, entry, []program.Term{right}, rightValues))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, leftKey, _ := p.Lens(left)
	_, _, _, _, rightKey, _ := p.Lens(right)
	if leftKey != 0 || rightKey != 0 {
		t.Fatal("nil exact keys must have no normalized Key")
	}
}

func TestNaNExactKeyDoesNotBecomeComparable(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	leftBase, rightBase := b.Table(program.Span{}, entry, nil, nil, nil), b.Table(program.Span{}, entry, nil, nil, nil)
	leftNaN, rightNaN := b.Float(program.Span{}, entry, math.NaN()), b.Float(program.Span{}, entry, math.NaN())
	left := b.LensExact(program.Span{}, entry, leftBase, leftNaN, program.FieldExact)
	right := b.LensExact(program.Span{}, entry, rightBase, rightNaN, program.FieldExact)
	leftValues, rightValues := b.Values(program.Span{}, entry, nil, 0), b.Values(program.Span{}, entry, nil, 0)
	b.SetBody(entry, b.Assign(program.Span{}, entry, []program.Term{left}, leftValues), b.Assign(program.Span{}, entry, []program.Term{right}, rightValues))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, leftKey, _ := p.Lens(left)
	_, _, _, _, rightKey, _ := p.Lens(right)
	if leftKey != 0 || rightKey != 0 {
		t.Fatal("NaN keys must not be comparable")
	}
}

func TestTypedScalarRelations(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	left := b.Integer(program.Span{}, entry, 2)
	negated := b.Unary(program.Span{}, entry, program.UnaryNeg, left)
	right := b.Integer(program.Span{}, entry, 3)
	sum := b.Binary(program.Span{}, entry, program.BinaryAdd, negated, right)
	condition := b.Bool(program.Span{}, entry, true)
	selected := b.Select(program.Span{}, entry, program.SelectAnd, condition, sum)
	values := b.Values(program.Span{}, entry, []program.Term{selected}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, values))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	owner, op, operand, ok := p.Unary(negated)
	if !ok || owner != entry || op != program.UnaryNeg || operand != left {
		t.Fatalf("Unary = %v, %v, %v, %v", owner, op, operand, ok)
	}
	owner, binary, gotLeft, gotRight, ok := p.Binary(sum)
	if !ok || owner != entry || binary != program.BinaryAdd || gotLeft != negated || gotRight != right {
		t.Fatalf("Binary = %v, %v, %v, %v, %v", owner, binary, gotLeft, gotRight, ok)
	}
	owner, selectOp, gotCondition, gotRight, ok := p.Select(selected)
	if !ok || owner != entry || selectOp != program.SelectAnd || gotCondition != condition || gotRight != sum {
		t.Fatalf("Select = %v, %v, %v, %v, %v", owner, selectOp, gotCondition, gotRight, ok)
	}
}

func TestScalarFamiliesFailClosed(t *testing.T) {
	tests := []struct {
		name  string
		build func(*program.Builder, program.Term) program.Term
	}{
		{
			name: "Read source",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Read(program.Span{}, owner, b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "Vararg source",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Vararg(program.Span{}, owner, b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "Unary zero operator",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Unary(program.Span{}, owner, 0, b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "Unary high operator",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Unary(program.Span{}, owner, program.UnaryOp(255), b.Integer(program.Span{}, owner, 1))
			},
		},
		{
			name: "Unary operand",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Unary(program.Span{}, owner, program.UnaryNeg, owner)
			},
		},
		{
			name: "Binary zero operator",
			build: func(b *program.Builder, owner program.Term) program.Term {
				left, right := b.Integer(program.Span{}, owner, 1), b.Integer(program.Span{}, owner, 2)
				return b.Binary(program.Span{}, owner, 0, left, right)
			},
		},
		{
			name: "Binary high operator",
			build: func(b *program.Builder, owner program.Term) program.Term {
				left, right := b.Integer(program.Span{}, owner, 1), b.Integer(program.Span{}, owner, 2)
				return b.Binary(program.Span{}, owner, program.BinaryOp(255), left, right)
			},
		},
		{
			name: "Binary left operand",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Binary(program.Span{}, owner, program.BinaryAdd, owner, b.Integer(program.Span{}, owner, 2))
			},
		},
		{
			name: "Binary right operand",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Binary(program.Span{}, owner, program.BinaryAdd, b.Integer(program.Span{}, owner, 1), owner)
			},
		},
		{
			name: "Select zero operator",
			build: func(b *program.Builder, owner program.Term) program.Term {
				left, right := b.Bool(program.Span{}, owner, true), b.Integer(program.Span{}, owner, 2)
				return b.Select(program.Span{}, owner, 0, left, right)
			},
		},
		{
			name: "Select high operator",
			build: func(b *program.Builder, owner program.Term) program.Term {
				left, right := b.Bool(program.Span{}, owner, true), b.Integer(program.Span{}, owner, 2)
				return b.Select(program.Span{}, owner, program.SelectOp(255), left, right)
			},
		},
		{
			name: "Select left operand",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Select(program.Span{}, owner, program.SelectAnd, owner, b.Integer(program.Span{}, owner, 2))
			},
		},
		{
			name: "Select right operand",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Select(program.Span{}, owner, program.SelectOr, b.Bool(program.Span{}, owner, true), owner)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, entry := entryBuilder(t)
			if term := test.build(b, entry); term != 0 {
				t.Fatalf("invalid scalar constructor minted %v", term)
			}
			if _, err := b.Seal(); err == nil {
				t.Fatal("invalid scalar constructor did not poison Builder")
			}
		})
	}
}

func TestLensAndTableShareNormalizedKey(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	fieldName := b.Name(program.Span{}, entry, "x")
	lensName := b.Name(program.Span{}, entry, "x")
	fieldLiteral := b.Integer(program.Span{}, entry, 1)
	fieldValues := b.Values(program.Span{}, entry, []program.Term{fieldLiteral}, 0)
	table := b.Table(program.Span{}, entry, []program.Term{fieldName}, []program.Term{fieldValues}, []program.FieldKind{program.FieldName})
	lens := b.LensExact(program.Span{}, entry, table, lensName, program.FieldName)
	rhs := b.Values(program.Span{}, entry, nil, 0)
	b.SetBody(entry, b.Assign(program.Span{}, entry, []program.Term{lens}, rhs))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, lensKey, ok := p.Lens(lens)
	if !ok || lensKey == 0 {
		t.Fatalf("Lens Key = %v, %v", lensKey, ok)
	}
	_, _, _, fieldKey, ok := p.Field(table, 0)
	if !ok || fieldKey != lensKey {
		t.Fatalf("Field Key = %v, %v; Lens Key = %v", fieldKey, ok, lensKey)
	}
}

func TestSealIsAllocationBoundedAndIterative(t *testing.T) {
	build := func(depth int) *program.Builder {
		b := program.NewBuilder()
		bodies := make([]program.Term, depth)
		for i := range bodies {
			bodies[i] = b.Body(program.Span{})
		}
		b.SetEntry(bodies[0])
		leafValue := b.Integer(program.Span{}, bodies[depth-1], 1)
		leafValues := b.Values(program.Span{}, bodies[depth-1], []program.Term{leafValue}, 0)
		b.SetBody(bodies[depth-1], b.Normal(program.Span{}, bodies[depth-1], leafValues))
		for i := depth - 2; i >= 0; i-- {
			b.SetBody(bodies[i], bodies[i+1])
		}
		return b
	}
	small, large := build(128), build(4096)
	if _, err := large.Seal(); err != nil {
		t.Fatalf("deep Seal: %v", err)
	}
	smallAllocs := testing.AllocsPerRun(20, func() {
		if _, err := small.Seal(); err != nil {
			t.Fatal(err)
		}
	})
	largeAllocs := testing.AllocsPerRun(20, func() {
		if _, err := large.Seal(); err != nil {
			t.Fatal(err)
		}
	})
	if largeAllocs > smallAllocs+2 {
		t.Fatalf("Seal allocations grow with depth: small=%g large=%g", smallAllocs, largeAllocs)
	}
}

func TestFunctionBindingAuthorityComesOnlyFromBind(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	binding := b.Cell(program.Span{}, entry)
	fn := b.Function(program.Span{}, entry, body, nil, 0, nil)
	bodyValues := b.Values(program.Span{}, body, nil, 0)
	b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
	wrong := b.Integer(program.Span{}, entry, 1)
	wrongValues := b.Values(program.Span{}, entry, []program.Term{wrong}, 0)
	actuals := b.Values(program.Span{}, entry, nil, 0)
	read := b.Read(program.Span{}, entry, binding)
	call := b.Call(program.Span{}, entry, read, 0, actuals)
	functionValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	b.SetBody(entry,
		b.Bind(program.Span{}, entry, []program.Term{binding}, wrongValues),
		b.Normal(program.Span{}, entry, functionValues),
		call,
	)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, direct, ok := p.Call(call)
	if !ok || direct != 0 {
		t.Fatalf("non-Function Bind invented direct authority: %v, %v", direct, ok)
	}
}

func TestDirectEvidenceIsDerivedFromCaptureChain(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	binding, inner := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, body)
	fn := b.Function(program.Span{}, entry, body, nil, 0, []program.Capture{{Inner: inner, Outer: binding}})
	actuals := b.Values(program.Span{}, body, nil, 0)
	callee := b.Read(program.Span{}, body, inner)
	call := b.Call(program.Span{}, body, callee, 0, actuals)
	returned := b.Values(program.Span{}, body, nil, call)
	b.SetBody(body, b.Return(program.Span{}, body, returned))
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{binding}, bound))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := p.FunctionCaptureCount(fn); !ok || count != 1 {
		t.Fatalf("FunctionCaptureCount = %d, %v", count, ok)
	}
	gotInner, gotOuter, ok := p.FunctionCapture(fn, 0)
	if !ok || gotInner != inner || gotOuter != binding {
		t.Fatalf("FunctionCapture = %v, %v, %v", gotInner, gotOuter, ok)
	}
	if _, _, ok := p.FunctionCapture(fn, 1); ok {
		t.Fatal("FunctionCapture accepted out-of-range index")
	}
	_, _, _, _, direct, ok := p.Call(call)
	if !ok || direct != fn {
		t.Fatalf("derived direct = %v, %v", direct, ok)
	}
}

func TestCapturedAliasWriteInvalidatesDirectEvidence(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	binding, inner := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, body)
	fn := b.Function(program.Span{}, entry, body, nil, 0, []program.Capture{{Inner: inner, Outer: binding}})
	actuals := b.Values(program.Span{}, body, nil, 0)
	callee := b.Read(program.Span{}, body, inner)
	call := b.Call(program.Span{}, body, callee, 0, actuals)
	returned := b.Values(program.Span{}, body, nil, call)
	writeLiteral := b.Integer(program.Span{}, body, 1)
	writeValues := b.Values(program.Span{}, body, []program.Term{writeLiteral}, 0)
	write := b.Assign(program.Span{}, body, []program.Term{inner}, writeValues)
	b.SetBody(body, write, b.Return(program.Span{}, body, returned))
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{binding}, bound))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, direct, ok := p.Call(call)
	if !ok || direct != 0 {
		t.Fatalf("captured binding write retained direct evidence: %v, %v", direct, ok)
	}
}

func TestSealedProgramDoesNotAliasBuilderPools(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	value := b.Integer(program.Span{}, entry, 1)
	values := b.Values(program.Span{}, entry, []program.Term{value}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, values))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	before := p.IntegerCount()
	if extra := b.Integer(program.Span{}, entry, 2); extra == 0 {
		t.Fatal("post-seal builder mint failed")
	}
	if after := p.IntegerCount(); after != before {
		t.Fatalf("sealed IntegerCount changed: %d -> %d", before, after)
	}
}

func TestAcyclicDirectCallHasNoMu(t *testing.T) {
	b := program.NewBuilder()
	entry, callerBody, calleeBody := b.Body(program.Span{}), b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	callerBinding, calleeBinding := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, entry)
	captureCell := b.Cell(program.Span{}, callerBody)
	caller := b.Function(program.Span{}, entry, callerBody, nil, 0, []program.Capture{{Inner: captureCell, Outer: calleeBinding}})
	callee := b.Function(program.Span{}, entry, calleeBody, nil, 0, nil)
	actuals := b.Values(program.Span{}, callerBody, nil, 0)
	read := b.Read(program.Span{}, callerBody, captureCell)
	call := b.Call(program.Span{}, callerBody, read, 0, actuals)
	callerValues := b.Values(program.Span{}, callerBody, nil, call)
	b.SetBody(callerBody, b.Return(program.Span{}, callerBody, callerValues))
	calleeValues := b.Values(program.Span{}, calleeBody, nil, 0)
	b.SetBody(calleeBody, b.Return(program.Span{}, calleeBody, calleeValues))
	callerValue := b.Values(program.Span{}, entry, []program.Term{caller}, 0)
	calleeValue := b.Values(program.Span{}, entry, []program.Term{callee}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{callerBinding}, callerValue), b.Bind(program.Span{}, entry, []program.Term{calleeBinding}, calleeValue))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(caller); ok || head != 0 {
		t.Fatalf("caller Mu = %v, %v", head, ok)
	}
	if head, ok := p.Mu(callee); ok || head != 0 {
		t.Fatalf("callee Mu = %v, %v", head, ok)
	}
}

func TestOpenCallTailAndDirectMu(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	functionBody := b.Body(program.Span{})
	b.SetEntry(entry)
	binding := b.Cell(program.Span{}, entry)
	inner := b.Cell(program.Span{}, functionBody)
	fn := b.Function(program.Span{}, entry, functionBody, nil, 0, []program.Capture{{Inner: inner, Outer: binding}})
	actuals := b.Values(program.Span{}, functionBody, nil, 0)
	callee := b.Read(program.Span{}, functionBody, inner)
	call := b.Call(program.Span{}, functionBody, callee, 0, actuals)
	callValues := b.Values(program.Span{}, functionBody, nil, call)
	callResult := b.Return(program.Span{}, functionBody, callValues)
	b.SetBody(functionBody, callResult)
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	bind := b.Bind(program.Span{}, entry, []program.Term{binding}, bound)
	b.SetBody(entry, bind)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Mu(fn); !ok || got != fn {
		t.Fatalf("Mu = %v, %v", got, ok)
	}
}

func TestMethodReceiverRequiresNameSyntax(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	receiver := b.Table(program.Span{}, entry, nil, nil, nil)
	name := b.Name(program.Span{}, entry, "m")
	lens := b.LensExact(program.Span{}, entry, receiver, name, program.FieldName)
	callee := b.Read(program.Span{}, entry, lens)
	actuals := b.Values(program.Span{}, entry, nil, 0)
	call := b.Call(program.Span{}, entry, callee, receiver, actuals)
	values := b.Values(program.Span{}, entry, []program.Term{call}, 0)
	result := b.Normal(program.Span{}, entry, values)
	b.SetBody(entry, result)
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestCellMustHaveExactlyOneRole(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	_ = b.Cell(program.Span{}, entry)
	values := b.Values(program.Span{}, entry, nil, 0)
	result := b.Normal(program.Span{}, entry, values)
	b.SetBody(entry, result)
	if _, err := b.Seal(); err == nil {
		t.Fatal("unbound Cell was accepted")
	}
}

func TestListOrdinalSkipsNamedFields(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	listOne := b.List(program.Span{}, entry, 1)
	name := b.Name(program.Span{}, entry, "x")
	listTwo := b.List(program.Span{}, entry, 2)
	firstValue := b.Integer(program.Span{}, entry, 10)
	secondValue := b.Integer(program.Span{}, entry, 20)
	thirdValue := b.Integer(program.Span{}, entry, 30)
	first := b.Values(program.Span{}, entry, []program.Term{firstValue}, 0)
	second := b.Values(program.Span{}, entry, []program.Term{secondValue}, 0)
	third := b.Values(program.Span{}, entry, []program.Term{thirdValue}, 0)
	table := b.Table(program.Span{}, entry, []program.Term{listOne, name, listTwo}, []program.Term{first, second, third}, []program.FieldKind{program.FieldList, program.FieldName, program.FieldList})
	values := b.Values(program.Span{}, entry, []program.Term{table}, 0)
	result := b.Normal(program.Span{}, entry, values)
	b.SetBody(entry, result)
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestTypedFamilyEnumeration(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	value := b.Integer(program.Span{}, entry, 1)
	values := b.Values(program.Span{}, entry, []program.Term{value}, 0)
	result := b.Normal(program.Span{}, entry, values)
	b.SetBody(entry, result)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if count := p.IntegerCount(); count != 1 {
		t.Fatalf("IntegerCount = %d", count)
	}
	if term, ok := p.IntegerAt(0); !ok || term != value {
		t.Fatalf("IntegerAt = %v, %v", term, ok)
	}
	if count := p.CallCount(); count != 0 {
		t.Fatalf("CallCount = %d", count)
	}
}

func TestSealRejectsSharedOccurrence(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	value := b.Integer(program.Span{}, entry, 1)
	first := b.Values(program.Span{}, entry, []program.Term{value}, 0)
	second := b.Values(program.Span{}, entry, []program.Term{value}, 0)
	firstResult := b.Normal(program.Span{}, entry, first)
	secondResult := b.Normal(program.Span{}, entry, second)
	b.SetBody(entry, firstResult, secondResult)
	if _, err := b.Seal(); err == nil {
		t.Fatal("shared occurrence was accepted")
	}
}

func TestReadCannotCrossFunctionActivation(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	functionBody := b.Body(program.Span{})
	b.SetEntry(entry)
	outer := b.Cell(program.Span{}, entry)
	fn := b.Function(program.Span{}, entry, functionBody, nil, 0, nil)
	read := b.Read(program.Span{}, functionBody, outer)
	functionValues := b.Values(program.Span{}, functionBody, []program.Term{read}, 0)
	functionResult := b.Return(program.Span{}, functionBody, functionValues)
	b.SetBody(functionBody, functionResult)
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	bind := b.Bind(program.Span{}, entry, []program.Term{outer}, bound)
	b.SetBody(entry, bind)
	if _, err := b.Seal(); err == nil {
		t.Fatal("cross-activation Cell Read was accepted")
	}
}

func TestMutualDirectMuUsesCanonicalHead(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	leftBody, rightBody := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	leftBinding, rightBinding := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, entry)
	leftCapture, rightCapture := b.Cell(program.Span{}, leftBody), b.Cell(program.Span{}, rightBody)
	left := b.Function(program.Span{}, entry, leftBody, nil, 0, []program.Capture{{Inner: leftCapture, Outer: rightBinding}})
	right := b.Function(program.Span{}, entry, rightBody, nil, 0, []program.Capture{{Inner: rightCapture, Outer: leftBinding}})
	leftActuals := b.Values(program.Span{}, leftBody, nil, 0)
	leftRead := b.Read(program.Span{}, leftBody, leftCapture)
	leftCall := b.Call(program.Span{}, leftBody, leftRead, 0, leftActuals)
	leftResultValues := b.Values(program.Span{}, leftBody, nil, leftCall)
	b.SetBody(leftBody, b.Return(program.Span{}, leftBody, leftResultValues))
	rightActuals := b.Values(program.Span{}, rightBody, nil, 0)
	rightRead := b.Read(program.Span{}, rightBody, rightCapture)
	rightCall := b.Call(program.Span{}, rightBody, rightRead, 0, rightActuals)
	rightResultValues := b.Values(program.Span{}, rightBody, nil, rightCall)
	b.SetBody(rightBody, b.Return(program.Span{}, rightBody, rightResultValues))
	leftValue := b.Values(program.Span{}, entry, []program.Term{left}, 0)
	rightValue := b.Values(program.Span{}, entry, []program.Term{right}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{leftBinding}, leftValue), b.Bind(program.Span{}, entry, []program.Term{rightBinding}, rightValue))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range []program.Term{left, right} {
		if head, ok := p.Mu(function); !ok || head != left {
			t.Fatalf("Mu(%v) = %v, %v", function, head, ok)
		}
	}
}

func TestDirectMuFindsConditionalSelectRight(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	binding, captureCell := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, body)
	fn := b.Function(program.Span{}, entry, body, nil, 0, []program.Capture{{Inner: captureCell, Outer: binding}})
	actuals := b.Values(program.Span{}, body, nil, 0)
	callee := b.Read(program.Span{}, body, captureCell)
	call := b.Call(program.Span{}, body, callee, 0, actuals)
	left := b.Bool(program.Span{}, body, true)
	selectTerm := b.Select(program.Span{}, body, program.SelectAnd, left, call)
	values := b.Values(program.Span{}, body, []program.Term{selectTerm}, 0)
	b.SetBody(body, b.Return(program.Span{}, body, values))
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{binding}, bound))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(fn); !ok || head != fn {
		t.Fatalf("Mu = %v, %v", head, ok)
	}
}

func TestVarargHasFunctionLocalRole(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	binding, varargCell := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, body)
	fn := b.Function(program.Span{}, entry, body, nil, varargCell, nil)
	vararg := b.Vararg(program.Span{}, body, varargCell)
	values := b.Values(program.Span{}, body, nil, vararg)
	b.SetBody(body, b.Return(program.Span{}, body, values))
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{binding}, bound))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	owner, cell, ok := p.Vararg(vararg)
	if !ok || owner != body || cell != varargCell {
		t.Fatalf("Vararg = %v, %v, %v", owner, cell, ok)
	}
}

func TestBindFormalAndVarargRolePermutations(t *testing.T) {
	t.Run("valid distinct roles", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		binding, local := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, entry)
		formal, varargCell, captured := b.Cell(program.Span{}, body), b.Cell(program.Span{}, body), b.Cell(program.Span{}, body)
		fn := b.Function(
			program.Span{},
			entry,
			body,
			[]program.Term{formal},
			varargCell,
			[]program.Capture{{Inner: captured, Outer: binding}},
		)
		formalRead := b.Read(program.Span{}, body, formal)
		formalValues := b.Values(program.Span{}, body, []program.Term{formalRead}, 0)
		vararg := b.Vararg(program.Span{}, body, varargCell)
		varargValues := b.Values(program.Span{}, body, nil, vararg)
		b.SetBody(body,
			b.Normal(program.Span{}, body, formalValues),
			b.Return(program.Span{}, body, varargValues),
		)
		functionValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		localValue := b.Integer(program.Span{}, entry, 1)
		localValues := b.Values(program.Span{}, entry, []program.Term{localValue}, 0)
		bindFunction := b.Bind(program.Span{}, entry, []program.Term{binding}, functionValues)
		bindLocal := b.Bind(program.Span{}, entry, []program.Term{local}, localValues)
		b.SetBody(entry, bindFunction, bindLocal)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if count, ok := p.BindLen(bindLocal); !ok || count != 1 {
			t.Fatalf("BindLen = %d, %v", count, ok)
		}
		if got, ok := p.BoundCell(bindLocal, 0); !ok || got != local {
			t.Fatalf("BoundCell = %v, %v", got, ok)
		}
		if owner, values, ok := p.Bind(bindLocal); !ok || owner != entry || values != localValues {
			t.Fatalf("Bind = %v, %v, %v", owner, values, ok)
		}
		if count, ok := p.FormalLen(fn); !ok || count != 1 {
			t.Fatalf("FormalLen = %d, %v", count, ok)
		}
		if got, ok := p.FormalAt(fn, 0); !ok || got != formal {
			t.Fatalf("FormalAt = %v, %v", got, ok)
		}
		if owner, cell, ok := p.Vararg(vararg); !ok || owner != body || cell != varargCell {
			t.Fatalf("Vararg = %v, %v, %v", owner, cell, ok)
		}
	})

	t.Run("Bind requires cells", func(t *testing.T) {
		b, entry := entryBuilder(t)
		values := b.Values(program.Span{}, entry, nil, 0)
		if got := b.Bind(program.Span{}, entry, nil, values); got != 0 {
			t.Fatalf("empty Bind minted %v", got)
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("empty Bind was accepted")
		}
	})
	t.Run("Bind requires Values", func(t *testing.T) {
		b, entry := entryBuilder(t)
		cell := b.Cell(program.Span{}, entry)
		value := b.Integer(program.Span{}, entry, 1)
		if got := b.Bind(program.Span{}, entry, []program.Term{cell}, value); got != 0 {
			t.Fatalf("Bind without Values minted %v", got)
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("Bind without Values was accepted")
		}
	})
	t.Run("Bind Cell belongs to owner", func(t *testing.T) {
		b, entry := entryBuilder(t)
		child := b.Body(program.Span{})
		cell := b.Cell(program.Span{}, child)
		values := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, values)
		b.SetBody(child)
		b.SetBody(entry, child, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("cross-Body Bind was accepted")
		}
	})
	t.Run("Bind Cell is unique within row", func(t *testing.T) {
		b, entry := entryBuilder(t)
		cell := b.Cell(program.Span{}, entry)
		values := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell, cell}, values)
		b.SetBody(entry, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("duplicate Cell in Bind was accepted")
		}
	})
	t.Run("Bind Cell is unique across rows", func(t *testing.T) {
		b, entry := entryBuilder(t)
		cell := b.Cell(program.Span{}, entry)
		firstValues := b.Values(program.Span{}, entry, nil, 0)
		secondValues := b.Values(program.Span{}, entry, nil, 0)
		first := b.Bind(program.Span{}, entry, []program.Term{cell}, firstValues)
		second := b.Bind(program.Span{}, entry, []program.Term{cell}, secondValues)
		b.SetBody(entry, first, second)
		if _, err := b.Seal(); err == nil {
			t.Fatal("Cell rebound by two Binds")
		}
	})
	t.Run("formal belongs to Function Body", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body, other := b.Body(program.Span{}), b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		formal := b.Cell(program.Span{}, other)
		fn := b.Function(program.Span{}, entry, body, []program.Term{formal}, 0, nil)
		bodyValues := b.Values(program.Span{}, body, nil, 0)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		b.SetBody(other)
		fnValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		b.SetBody(entry, other, b.Normal(program.Span{}, entry, fnValues))
		if _, err := b.Seal(); err == nil {
			t.Fatal("foreign formal Cell was accepted")
		}
	})
	t.Run("formal is unique", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		formal := b.Cell(program.Span{}, body)
		fn := b.Function(program.Span{}, entry, body, []program.Term{formal, formal}, 0, nil)
		bodyValues := b.Values(program.Span{}, body, nil, 0)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		fnValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		b.SetBody(entry, b.Normal(program.Span{}, entry, fnValues))
		if _, err := b.Seal(); err == nil {
			t.Fatal("duplicate formal Cell was accepted")
		}
	})
	t.Run("formal and vararg are distinct", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		cell := b.Cell(program.Span{}, body)
		fn := b.Function(program.Span{}, entry, body, []program.Term{cell}, cell, nil)
		bodyValues := b.Values(program.Span{}, body, nil, 0)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		fnValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		b.SetBody(entry, b.Normal(program.Span{}, entry, fnValues))
		if _, err := b.Seal(); err == nil {
			t.Fatal("formal/vararg role collision was accepted")
		}
	})
	t.Run("vararg belongs to Function Body", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		cell := b.Cell(program.Span{}, entry)
		fn := b.Function(program.Span{}, entry, body, nil, cell, nil)
		bodyValues := b.Values(program.Span{}, body, nil, 0)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		fnValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{cell}, fnValues))
		if _, err := b.Seal(); err == nil {
			t.Fatal("foreign vararg Cell was accepted")
		}
	})
	t.Run("Vararg occurrence requires declared vararg role", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		formal := b.Cell(program.Span{}, body)
		fn := b.Function(program.Span{}, entry, body, []program.Term{formal}, 0, nil)
		vararg := b.Vararg(program.Span{}, body, formal)
		bodyValues := b.Values(program.Span{}, body, nil, vararg)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		fnValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		b.SetBody(entry, b.Normal(program.Span{}, entry, fnValues))
		if _, err := b.Seal(); err == nil {
			t.Fatal("Vararg occurrence on formal Cell was accepted")
		}
	})
	t.Run("Vararg occurrence stays in Function activation", func(t *testing.T) {
		b := program.NewBuilder()
		entry, body := b.Body(program.Span{}), b.Body(program.Span{})
		b.SetEntry(entry)
		varargCell := b.Cell(program.Span{}, body)
		fn := b.Function(program.Span{}, entry, body, nil, varargCell, nil)
		bodyValues := b.Values(program.Span{}, body, nil, 0)
		b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
		vararg := b.Vararg(program.Span{}, entry, varargCell)
		entryVarargs := b.Values(program.Span{}, entry, nil, vararg)
		fnValues := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
		b.SetBody(entry,
			b.Normal(program.Span{}, entry, entryVarargs),
			b.Normal(program.Span{}, entry, fnValues),
		)
		if _, err := b.Seal(); err == nil {
			t.Fatal("cross-activation Vararg was accepted")
		}
	})
}

func TestMethodReceiverRejectsBracketKey(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	receiver := b.Table(program.Span{}, entry, nil, nil, nil)
	key := b.String(program.Span{}, entry, "m")
	lens := b.LensExact(program.Span{}, entry, receiver, key, program.FieldExact)
	callee := b.Read(program.Span{}, entry, lens)
	actuals := b.Values(program.Span{}, entry, nil, 0)
	call := b.Call(program.Span{}, entry, callee, receiver, actuals)
	values := b.Values(program.Span{}, entry, []program.Term{call}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, values))
	if _, err := b.Seal(); err == nil {
		t.Fatal("bracket method key was accepted")
	}
}

func TestMethodReceiverMustMatchNameLensBase(t *testing.T) {
	b, entry := entryBuilder(t)
	receiver := b.Table(program.Span{}, entry, nil, nil, nil)
	other := b.Table(program.Span{}, entry, nil, nil, nil)
	name := b.Name(program.Span{}, entry, "m")
	lens := b.LensExact(program.Span{}, entry, other, name, program.FieldName)
	callee := b.Read(program.Span{}, entry, lens)
	actuals := b.Values(program.Span{}, entry, nil, 0)
	call := b.Call(program.Span{}, entry, callee, receiver, actuals)
	receiverValues := b.Values(program.Span{}, entry, []program.Term{receiver}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, receiverValues), call)
	if _, err := b.Seal(); err == nil {
		t.Fatal("method receiver disagreed with name Lens base")
	}
}

func TestTypedQueriesAllocateZero(t *testing.T) {
	b := program.NewBuilder()
	entry, functionBody := b.Body(program.Span{}), b.Body(program.Span{})
	whenTrue, whenFalse := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)

	binding, local := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, entry)
	formal := b.Cell(program.Span{}, functionBody)
	varargCell := b.Cell(program.Span{}, functionBody)
	captured := b.Cell(program.Span{}, functionBody)
	function := b.Function(
		program.Span{},
		entry,
		functionBody,
		[]program.Term{formal},
		varargCell,
		[]program.Capture{{Inner: captured, Outer: binding}},
	)
	capturedRead := b.Read(program.Span{}, functionBody, captured)
	actuals := b.Values(program.Span{}, functionBody, nil, 0)
	call := b.Call(program.Span{}, functionBody, capturedRead, 0, actuals)
	vararg := b.Vararg(program.Span{}, functionBody, varargCell)
	returnValues := b.Values(program.Span{}, functionBody, nil, vararg)
	returned := b.Return(program.Span{}, functionBody, returnValues)
	b.SetBody(functionBody, call, returned)

	functionValues := b.Values(program.Span{}, entry, []program.Term{function}, 0)
	bindFunction := b.Bind(program.Span{}, entry, []program.Term{binding}, functionValues)
	localLiteral := b.Integer(program.Span{}, entry, 41)
	localValues := b.Values(program.Span{}, entry, []program.Term{localLiteral}, 0)
	bindLocal := b.Bind(program.Span{}, entry, []program.Term{local}, localValues)

	left := b.Integer(program.Span{}, entry, 1)
	unary := b.Unary(program.Span{}, entry, program.UnaryNeg, left)
	right := b.Integer(program.Span{}, entry, 2)
	binary := b.Binary(program.Span{}, entry, program.BinaryAdd, unary, right)
	condition := b.Bool(program.Span{}, entry, true)
	selected := b.Select(program.Span{}, entry, program.SelectAnd, condition, binary)
	localRead := b.Read(program.Span{}, entry, local)
	nilValue := b.Nil(program.Span{}, entry)
	floatValue := b.Float(program.Span{}, entry, -0.0)
	stringValue := b.String(program.Span{}, entry, "value")
	scalarValues := b.Values(
		program.Span{},
		entry,
		[]program.Term{selected, localRead, nilValue, floatValue, stringValue},
		0,
	)
	normal := b.Normal(program.Span{}, entry, scalarValues)

	exactBase := b.Table(program.Span{}, entry, nil, nil, nil)
	name := b.Name(program.Span{}, entry, "field")
	exactLens := b.LensExact(program.Span{}, entry, exactBase, name, program.FieldName)
	lensRead := b.Read(program.Span{}, entry, exactLens)
	lensValues := b.Values(program.Span{}, entry, []program.Term{lensRead}, 0)
	lensResult := b.Normal(program.Span{}, entry, lensValues)

	dynamicBase := b.Table(program.Span{}, entry, nil, nil, nil)
	dynamicKey := b.String(program.Span{}, entry, "dynamic")
	dynamicLens := b.LensKey(program.Span{}, entry, dynamicBase, dynamicKey)
	assignedValue := b.Integer(program.Span{}, entry, 9)
	assignedValues := b.Values(program.Span{}, entry, []program.Term{assignedValue}, 0)
	assign := b.Assign(program.Span{}, entry, []program.Term{dynamicLens}, assignedValues)

	list := b.List(program.Span{}, entry, 1)
	fieldLiteral := b.String(program.Span{}, entry, "field value")
	fieldValues := b.Values(program.Span{}, entry, []program.Term{fieldLiteral}, 0)
	table := b.Table(
		program.Span{},
		entry,
		[]program.Term{list},
		[]program.Term{fieldValues},
		[]program.FieldKind{program.FieldList},
	)
	tableValues := b.Values(program.Span{}, entry, []program.Term{table}, 0)
	tableResult := b.Normal(program.Span{}, entry, tableValues)

	trueValues := b.Values(program.Span{}, whenTrue, nil, 0)
	falseValues := b.Values(program.Span{}, whenFalse, nil, 0)
	b.SetBody(whenTrue, b.Normal(program.Span{}, whenTrue, trueValues))
	b.SetBody(whenFalse, b.Normal(program.Span{}, whenFalse, falseValues))
	branchCondition := b.Bool(program.Span{}, entry, false)
	branch := b.Branch(program.Span{}, entry, branchCondition, whenTrue, whenFalse)

	throwValues := b.Values(program.Span{}, entry, nil, 0)
	yieldValues := b.Values(program.Span{}, entry, nil, 0)
	breakValues := b.Values(program.Span{}, entry, nil, 0)
	continueValues := b.Values(program.Span{}, entry, nil, 0)
	thrown := b.Throw(program.Span{}, entry, throwValues)
	yielded := b.Yield(program.Span{}, entry, yieldValues)
	broken := b.Break(program.Span{}, entry, breakValues)
	continued := b.Continue(program.Span{}, entry, continueValues)

	b.SetBody(
		entry,
		bindFunction,
		bindLocal,
		normal,
		lensResult,
		assign,
		tableResult,
		branch,
		thrown,
		yielded,
		broken,
		continued,
	)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}

	allocations := testing.AllocsPerRun(1000, func() {
		queryInt = p.TermCount()
		querySpan, queryOK = p.Span(stringValue)
		queryTerm, queryOK = p.Entry()
		queryTerm, queryOK = p.Nil(nilValue)
		queryTerm, queryBool, queryOK = p.Bool(condition)
		queryTerm, queryInt64, queryOK = p.Integer(left)
		queryTerm, queryFloat64, queryOK = p.Float(floatValue)
		queryTerm, queryString, queryOK = p.String(stringValue)
		queryInt, queryOK = p.ValuesLen(scalarValues)
		queryTerm, queryOK = p.Value(scalarValues, 0)
		queryTerm, queryTerm2, queryOK = p.Values(scalarValues)
		queryTerm, queryTerm2, queryTerm3, queryFieldKind, queryKey, queryOK = p.Lens(exactLens)
		queryTerm, queryTerm2, queryOK = p.Outcome(normal)
		queryTerm, queryTerm2, queryOK = p.Normal(normal)
		queryTerm, queryTerm2, queryOK = p.Return(returned)
		queryTerm, queryTerm2, queryOK = p.Throw(thrown)
		queryTerm, queryTerm2, queryOK = p.Yield(yielded)
		queryTerm, queryTerm2, queryOK = p.Break(broken)
		queryTerm, queryTerm2, queryOK = p.Continue(continued)
		queryTerm, queryOK = p.Mu(function)
		queryInt, queryOK = p.BodyLen(entry)
		queryTerm, queryOK = p.Root(entry, 0)
		queryTerm, queryOK = p.Cell(local)
		queryTerm, queryTerm2, queryOK = p.Read(localRead)
		queryTerm, queryTerm2, queryOK = p.Vararg(vararg)
		queryTerm, queryUnary, queryTerm2, queryOK = p.Unary(unary)
		queryTerm, queryBinary, queryTerm2, queryTerm3, queryOK = p.Binary(binary)
		queryTerm, querySelect, queryTerm2, queryTerm3, queryOK = p.Select(selected)
		queryInt, queryOK = p.BindLen(bindLocal)
		queryTerm, queryOK = p.BoundCell(bindLocal, 0)
		queryTerm, queryTerm2, queryOK = p.Bind(bindLocal)
		queryInt, queryOK = p.AssignLen(assign)
		queryTerm, queryOK = p.Target(assign, 0)
		queryTerm, queryTerm2, queryOK = p.Assign(assign)
		queryTerm, queryTerm2, queryTerm3, queryOK = p.Function(function)
		queryInt, queryOK = p.FormalLen(function)
		queryTerm, queryOK = p.FormalAt(function, 0)
		queryInt, queryOK = p.FunctionCaptureCount(function)
		queryTerm, queryTerm2, queryOK = p.FunctionCapture(function, 0)
		queryTerm, queryTerm2, queryTerm3, queryTerm4, queryTerm5, queryOK = p.Call(call)
		queryTerm, queryTerm2, queryTerm3, queryTerm4, queryOK = p.Branch(branch)
		queryTerm, queryOK = p.Table(table)
		queryTerm, queryString, queryKey, queryOK = p.Name(name)
		queryTerm, queryInt64, queryKey, queryOK = p.List(list)
		queryInt, queryOK = p.TableLen(table)
		queryTerm, queryTerm2, queryFieldKind, queryKey, queryOK = p.Field(table, 0)
		queryInt = p.IntegerCount()
		queryTerm, queryOK = p.IntegerAt(0)
		queryInt = p.ValuesCount()
		queryTerm, queryOK = p.ValuesAt(0)
		queryInt = p.BodyCount()
		queryTerm, queryOK = p.BodyAt(0)
		queryInt = p.FunctionCount()
		queryTerm, queryOK = p.FunctionAt(0)
		queryInt = p.CallCount()
		queryTerm, queryOK = p.CallAt(0)
	})
	if allocations != 0 {
		t.Fatalf("typed queries allocated: %g", allocations)
	}
}

func TestLongDirectCycleIsIterative(t *testing.T) {
	const count = 128
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	bodies := make([]program.Term, count)
	bindings := make([]program.Term, count)
	inners := make([]program.Term, count)
	functions := make([]program.Term, count)
	for i := range functions {
		bodies[i] = b.Body(program.Span{})
		bindings[i] = b.Cell(program.Span{}, entry)
		inners[i] = b.Cell(program.Span{}, bodies[i])
	}
	for i := range functions {
		next := (i + 1) % count
		functions[i] = b.Function(program.Span{}, entry, bodies[i], nil, 0, []program.Capture{{Inner: inners[i], Outer: bindings[next]}})
	}
	for i := range functions {
		actuals := b.Values(program.Span{}, bodies[i], nil, 0)
		callee := b.Read(program.Span{}, bodies[i], inners[i])
		call := b.Call(program.Span{}, bodies[i], callee, 0, actuals)
		result := b.Values(program.Span{}, bodies[i], nil, call)
		if !b.SetBody(bodies[i], b.Return(program.Span{}, bodies[i], result)) {
			t.Fatal("SetBody")
		}
	}
	roots := make([]program.Term, count)
	for i := range functions {
		bound := b.Values(program.Span{}, entry, []program.Term{functions[i]}, 0)
		roots[i] = b.Bind(program.Span{}, entry, []program.Term{bindings[i]}, bound)
	}
	if !b.SetBody(entry, roots...) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range functions {
		if head, ok := p.Mu(function); !ok || head != functions[0] {
			t.Fatalf("Mu(%v) = %v, %v", function, head, ok)
		}
	}
}

func TestLongAcyclicDirectCallChainIsIterativeAndAllocationBounded(t *testing.T) {
	build := func(count int) (*program.Builder, []program.Term) {
		b := program.NewBuilder()
		entry := b.Body(program.Span{})
		b.SetEntry(entry)
		bodies := make([]program.Term, count)
		bindings := make([]program.Term, count)
		inners := make([]program.Term, count-1)
		functions := make([]program.Term, count)
		for i := range bodies {
			bodies[i] = b.Body(program.Span{})
			bindings[i] = b.Cell(program.Span{}, entry)
			if i+1 < count {
				inners[i] = b.Cell(program.Span{}, bodies[i])
			}
		}
		for i := range functions {
			var captures []program.Capture
			if i+1 < count {
				captures = []program.Capture{{Inner: inners[i], Outer: bindings[i+1]}}
			}
			functions[i] = b.Function(program.Span{}, entry, bodies[i], nil, 0, captures)
		}
		for i := 0; i+1 < count; i++ {
			callee := b.Read(program.Span{}, bodies[i], inners[i])
			actuals := b.Values(program.Span{}, bodies[i], nil, 0)
			b.SetBody(bodies[i], b.Call(program.Span{}, bodies[i], callee, 0, actuals))
		}
		lastValues := b.Values(program.Span{}, bodies[count-1], nil, 0)
		b.SetBody(bodies[count-1], b.Return(program.Span{}, bodies[count-1], lastValues))
		roots := make([]program.Term, count)
		for i, function := range functions {
			values := b.Values(program.Span{}, entry, []program.Term{function}, 0)
			roots[i] = b.Bind(program.Span{}, entry, []program.Term{bindings[i]}, values)
		}
		b.SetBody(entry, roots...)
		return b, functions
	}

	deep, functions := build(4096)
	p, err := deep.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range functions {
		if head, ok := p.Mu(function); ok || head != 0 {
			t.Fatalf("acyclic Function received Mu head %v", head)
		}
	}

	sealAllocations := func(count int) float64 {
		b, _ := build(count)
		return testing.AllocsPerRun(20, func() {
			var err error
			queryProgram, err = b.Seal()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := sealAllocations(64), sealAllocations(512)
	if large > small+2 {
		t.Fatalf("acyclic direct-call Seal allocations grew per relation: small=%g large=%g", small, large)
	}
}

func TestSpanAndTypedMintingAreDeterministic(t *testing.T) {
	build := func() (*program.Program, program.Term, error) {
		b := program.NewBuilder()
		entry := b.Body(program.Span{File: "x.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2})
		b.SetEntry(entry)
		value := b.String(program.Span{File: "x.lua", StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 6}, entry, "abc")
		values := b.Values(program.Span{}, entry, []program.Term{value}, 0)
		b.SetBody(entry, b.Normal(program.Span{}, entry, values))
		p, err := b.Seal()
		return p, value, err
	}
	left, leftValue, err := build()
	if err != nil {
		t.Fatal(err)
	}
	right, rightValue, err := build()
	if err != nil {
		t.Fatal(err)
	}
	if leftValue != rightValue || left.TermCount() != right.TermCount() {
		t.Fatal("typed minting is not deterministic")
	}
	span, ok := left.Span(leftValue)
	if !ok || span.File != "x.lua" || span.StartLine != 2 || span.StartCol != 3 || span.EndLine != 2 || span.EndCol != 6 {
		t.Fatalf("Span = %#v, %v", span, ok)
	}
}

func TestInvalidValuePositionAndTableSyntaxFailClosed(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	if got := b.Values(program.Span{}, entry, []program.Term{entry}, 0); got != 0 {
		t.Fatalf("Values accepted Body %v", got)
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("invalid value position did not poison builder")
	}

	b = program.NewBuilder()
	entry = b.Body(program.Span{})
	b.SetEntry(entry)
	badName := b.Integer(program.Span{}, entry, 1)
	fieldValues := b.Values(program.Span{}, entry, nil, 0)
	if got := b.Table(program.Span{}, entry, []program.Term{badName}, []program.Term{fieldValues}, []program.FieldKind{program.FieldName}); got != 0 {
		t.Fatalf("Table accepted non-string FieldName %v", got)
	}
}

func TestTableExactAndDynamicFieldLaws(t *testing.T) {
	t.Run("typed syntax and normalized keys", func(t *testing.T) {
		b, entry := entryBuilder(t)
		list := b.List(program.Span{}, entry, 1)
		name := b.Name(program.Span{}, entry, "named")
		exactInteger := b.Integer(program.Span{}, entry, 7)
		exactNil := b.Nil(program.Span{}, entry)
		exactNaN := b.Float(program.Span{}, entry, math.NaN())
		dynamic := b.Integer(program.Span{}, entry, 7)
		keys := []program.Term{list, name, exactInteger, exactNil, exactNaN, dynamic}
		kinds := []program.FieldKind{
			program.FieldList,
			program.FieldName,
			program.FieldExact,
			program.FieldExact,
			program.FieldExact,
			program.FieldKey,
		}
		fieldValues := make([]program.Term, len(keys))
		for i := range fieldValues {
			value := b.String(program.Span{}, entry, "value")
			fieldValues[i] = b.Values(program.Span{}, entry, []program.Term{value}, 0)
		}
		table := b.Table(program.Span{}, entry, keys, fieldValues, kinds)
		values := b.Values(program.Span{}, entry, []program.Term{table}, 0)
		b.SetBody(entry, b.Normal(program.Span{}, entry, values))
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if count, ok := p.TableLen(table); !ok || count != len(keys) {
			t.Fatalf("TableLen = %d, %v", count, ok)
		}
		var normalized []program.Key
		for i := range keys {
			source, gotValues, kind, key, ok := p.Field(table, i)
			if !ok || source != keys[i] || gotValues != fieldValues[i] || kind != kinds[i] {
				t.Fatalf("Field(%d) = %v, %v, %v, %v, %v", i, source, gotValues, kind, key, ok)
			}
			normalized = append(normalized, key)
		}
		if normalized[0] == 0 || normalized[1] == 0 || normalized[2] == 0 {
			t.Fatalf("storable static keys were not normalized: %v", normalized)
		}
		if normalized[3] != 0 || normalized[4] != 0 || normalized[5] != 0 {
			t.Fatalf("nil, NaN, or dynamic syntax gained equality identity: %v", normalized)
		}
		if owner, ordinal, key, ok := p.List(list); !ok || owner != entry || ordinal != 1 || key != normalized[0] {
			t.Fatalf("List = %v, %d, %v, %v", owner, ordinal, key, ok)
		}
		if owner, text, key, ok := p.Name(name); !ok || owner != entry || text != "named" || key != normalized[1] {
			t.Fatalf("Name = %v, %q, %v, %v", owner, text, key, ok)
		}
	})

	tests := []struct {
		name  string
		build func(*program.Builder, program.Term) program.Term
	}{
		{
			name: "misaligned columns",
			build: func(b *program.Builder, owner program.Term) program.Term {
				return b.Table(program.Span{}, owner, []program.Term{b.Name(program.Span{}, owner, "x")}, nil, nil)
			},
		},
		{
			name: "invalid field kind",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.Integer(program.Span{}, owner, 1)
				value := b.String(program.Span{}, owner, "value")
				values := b.Values(program.Span{}, owner, []program.Term{value}, 0)
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{values}, []program.FieldKind{0})
			},
		},
		{
			name: "field requires Values",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.Integer(program.Span{}, owner, 1)
				value := b.String(program.Span{}, owner, "value")
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{value}, []program.FieldKind{program.FieldExact})
			},
		},
		{
			name: "exact key is scalar literal",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.Values(program.Span{}, owner, nil, 0)
				value := b.String(program.Span{}, owner, "value")
				values := b.Values(program.Span{}, owner, []program.Term{value}, 0)
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{values}, []program.FieldKind{program.FieldExact})
			},
		},
		{
			name: "dynamic key is value occurrence",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.Name(program.Span{}, owner, "x")
				value := b.String(program.Span{}, owner, "value")
				values := b.Values(program.Span{}, owner, []program.Term{value}, 0)
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{values}, []program.FieldKind{program.FieldKey})
			},
		},
		{
			name: "name key uses name syntax",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.String(program.Span{}, owner, "x")
				value := b.String(program.Span{}, owner, "value")
				values := b.Values(program.Span{}, owner, []program.Term{value}, 0)
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{values}, []program.FieldKind{program.FieldName})
			},
		},
		{
			name: "list ordinal follows list fields",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.List(program.Span{}, owner, 2)
				value := b.String(program.Span{}, owner, "value")
				values := b.Values(program.Span{}, owner, []program.Term{value}, 0)
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{values}, []program.FieldKind{program.FieldList})
			},
		},
		{
			name: "non-list field has one result",
			build: func(b *program.Builder, owner program.Term) program.Term {
				key := b.Integer(program.Span{}, owner, 1)
				left := b.String(program.Span{}, owner, "left")
				right := b.String(program.Span{}, owner, "right")
				values := b.Values(program.Span{}, owner, []program.Term{left, right}, 0)
				return b.Table(program.Span{}, owner, []program.Term{key}, []program.Term{values}, []program.FieldKind{program.FieldExact})
			},
		},
		{
			name: "only final list field may stay open",
			build: func(b *program.Builder, owner program.Term) program.Term {
				callee := b.Table(program.Span{}, owner, nil, nil, nil)
				actuals := b.Values(program.Span{}, owner, nil, 0)
				open := b.Call(program.Span{}, owner, callee, 0, actuals)
				firstValues := b.Values(program.Span{}, owner, nil, open)
				secondLiteral := b.String(program.Span{}, owner, "second")
				secondValues := b.Values(program.Span{}, owner, []program.Term{secondLiteral}, 0)
				return b.Table(
					program.Span{},
					owner,
					[]program.Term{b.List(program.Span{}, owner, 1), b.List(program.Span{}, owner, 2)},
					[]program.Term{firstValues, secondValues},
					[]program.FieldKind{program.FieldList, program.FieldList},
				)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, entry := entryBuilder(t)
			if table := test.build(b, entry); table != 0 {
				t.Fatalf("invalid Table minted %v", table)
			}
			if _, err := b.Seal(); err == nil {
				t.Fatal("invalid Table did not poison Builder")
			}
		})
	}
}

func TestBodyForestRejectsOrphanAndCycle(t *testing.T) {
	b := program.NewBuilder()
	entry, orphan := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	entryValues := b.Values(program.Span{}, entry, nil, 0)
	orphanValues := b.Values(program.Span{}, orphan, nil, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, entryValues))
	b.SetBody(orphan, b.Normal(program.Span{}, orphan, orphanValues))
	if _, err := b.Seal(); err == nil {
		t.Fatal("orphan Body was accepted")
	}

	b = program.NewBuilder()
	left, right := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(left)
	b.SetBody(left, right)
	b.SetBody(right, left)
	if _, err := b.Seal(); err == nil {
		t.Fatal("Body cycle was accepted")
	}
}

func TestFunctionRejectsDuplicateCaptureOuter(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	outer := b.Cell(program.Span{}, entry)
	innerOne, innerTwo := b.Cell(program.Span{}, body), b.Cell(program.Span{}, body)
	fn := b.Function(program.Span{}, entry, body, nil, 0, []program.Capture{
		{Inner: innerOne, Outer: outer},
		{Inner: innerTwo, Outer: outer},
	})
	bodyValues := b.Values(program.Span{}, body, nil, 0)
	b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
	bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))
	if _, err := b.Seal(); err == nil {
		t.Fatal("duplicate outer capture was accepted")
	}
}

func TestFunctionRejectsCaptureOutsideLexicalAncestry(t *testing.T) {
	tests := []struct {
		name  string
		build func(*program.Builder, program.Term, program.Term, program.Term) program.Term
	}{
		{
			name: "inner belongs to owner",
			build: func(b *program.Builder, entry, body, outer program.Term) program.Term {
				return b.Function(program.Span{}, entry, body, nil, 0, []program.Capture{{Inner: outer, Outer: outer}})
			},
		},
		{
			name: "outer belongs to function body",
			build: func(b *program.Builder, entry, body, _ program.Term) program.Term {
				inner := b.Cell(program.Span{}, body)
				localOuter := b.Cell(program.Span{}, body)
				return b.Function(program.Span{}, entry, body, nil, 0, []program.Capture{{Inner: inner, Outer: localOuter}})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := program.NewBuilder()
			entry, body := b.Body(program.Span{}), b.Body(program.Span{})
			b.SetEntry(entry)
			outer := b.Cell(program.Span{}, entry)
			fn := test.build(b, entry, body, outer)
			bodyValues := b.Values(program.Span{}, body, nil, 0)
			b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
			bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
			b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))
			if _, err := b.Seal(); err == nil {
				t.Fatal("non-ancestral Capture was accepted")
			}
		})
	}
}

func TestFunctionRejectsCaptureDefinitionRoleCollisions(t *testing.T) {
	tests := []struct {
		name     string
		function func(*program.Builder, program.Term, program.Term, program.Term, program.Term) program.Term
	}{
		{
			name: "formal and capture",
			function: func(b *program.Builder, entry, body, inner, outer program.Term) program.Term {
				return b.Function(program.Span{}, entry, body, []program.Term{inner}, 0, []program.Capture{{Inner: inner, Outer: outer}})
			},
		},
		{
			name: "vararg and capture",
			function: func(b *program.Builder, entry, body, inner, outer program.Term) program.Term {
				return b.Function(program.Span{}, entry, body, nil, inner, []program.Capture{{Inner: inner, Outer: outer}})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := program.NewBuilder()
			entry, body := b.Body(program.Span{}), b.Body(program.Span{})
			b.SetEntry(entry)
			outer, inner := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, body)
			fn := test.function(b, entry, body, inner, outer)
			bodyValues := b.Values(program.Span{}, body, nil, 0)
			b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
			bound := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
			b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))
			if _, err := b.Seal(); err == nil {
				t.Fatal("Capture role collision was accepted")
			}
		})
	}
}

func TestFunctionBodyHasExactlyOneStructuralAuthority(t *testing.T) {
	b := program.NewBuilder()
	entry, body := b.Body(program.Span{}), b.Body(program.Span{})
	b.SetEntry(entry)
	left := b.Function(program.Span{}, entry, body, nil, 0, nil)
	right := b.Function(program.Span{}, entry, body, nil, 0, nil)
	bodyValues := b.Values(program.Span{}, body, nil, 0)
	b.SetBody(body, b.Return(program.Span{}, body, bodyValues))
	functions := b.Values(program.Span{}, entry, []program.Term{left, right}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, functions))
	if _, err := b.Seal(); err == nil {
		t.Fatal("two Functions claimed one Body")
	}
}

func TestTableFieldRequiresOneExpressionExceptFinalListTail(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	callee := b.Table(program.Span{}, entry, nil, nil, nil)
	actuals := b.Values(program.Span{}, entry, nil, 0)
	open := b.Call(program.Span{}, entry, callee, 0, actuals)
	field := b.Values(program.Span{}, entry, nil, open)
	key := b.Name(program.Span{}, entry, "x")
	if table := b.Table(program.Span{}, entry, []program.Term{key}, []program.Term{field}, []program.FieldKind{program.FieldName}); table != 0 {
		t.Fatalf("named field accepted open tail %v", table)
	}

	b = program.NewBuilder()
	entry = b.Body(program.Span{})
	b.SetEntry(entry)
	callee = b.Table(program.Span{}, entry, nil, nil, nil)
	actuals = b.Values(program.Span{}, entry, nil, 0)
	open = b.Call(program.Span{}, entry, callee, 0, actuals)
	field = b.Values(program.Span{}, entry, nil, open)
	listKey := b.List(program.Span{}, entry, 1)
	table := b.Table(program.Span{}, entry, []program.Term{listKey}, []program.Term{field}, []program.FieldKind{program.FieldList})
	if table == 0 {
		t.Fatal("final list field rejected open tail")
	}
	values := b.Values(program.Span{}, entry, []program.Term{table}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, values))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	source, gotValues, kind, _, ok := p.Field(table, 0)
	if !ok || source != listKey || gotValues != field || kind != program.FieldList {
		t.Fatalf("open final list Field = %v, %v, %v, %v", source, gotValues, kind, ok)
	}
}
