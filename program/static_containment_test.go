package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
)

func staticScopeAfter(t *testing.T, b *program.Builder, body program.Term) program.Term {
	t.Helper()
	return b.Cell(program.Span{}, body)
}

func TestTypeOfStaticContainmentExcludesRuntimeEvidence(t *testing.T) {
	b, entry := entryBuilder(t)
	fnBody := b.Body(program.Span{})
	f := b.Cell(program.Span{}, entry)
	fn := b.Function(program.Span{}, entry, fnBody, nil, 0, nil)
	fv := b.Values(program.Span{}, entry, []program.Term{fn}, 0)
	bindFn := b.Bind(program.Span{}, entry, []program.Term{f}, fv)
	scope := staticScopeAfter(t, b, entry)
	callee := b.Read(program.Span{}, entry, f)
	actuals := b.Values(program.Span{}, entry, nil, 0)
	call := b.Call(program.Span{}, entry, callee, 0, actuals)
	typeOf := b.TypeOf(program.Span{}, scope, call)
	if typeOf == 0 {
		t.Fatal("TypeOf")
	}
	if !b.SetBody(fnBody) || !b.SetBody(entry, bindFn, b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if gotScope, gotOperand, ok := p.TypeOf(typeOf); !ok || gotScope != scope || gotOperand != call {
		t.Fatalf("TypeOf = %v, %v, %v", gotScope, gotOperand, ok)
	}
	if _, _, _, _, direct, ok := p.Call(call); !ok || direct != 0 {
		t.Fatalf("static Call direct = %v, %v", direct, ok)
	}
	if !p.Static(typeOf) || !p.Static(call) || !p.Static(callee) || p.Static(f) || p.Static(bindFn) {
		t.Fatalf("static classification type=%v call=%v callee=%v cell=%v bind=%v", p.Static(typeOf), p.Static(call), p.Static(callee), p.Static(f), p.Static(bindFn))
	}
	if head, ok := p.Mu(call); ok || head != 0 {
		t.Fatalf("static Call Mu = %v, %v", head, ok)
	}
}

func TestTypeOfFormalCellScopeSeesAllFormals(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	first, second := b.Cell(program.Span{}, body), b.Cell(program.Span{}, body)
	fn := b.Function(program.Span{}, entry, body, []program.Term{first, second}, 0, nil)
	if b.TypeOf(program.Span{}, first, b.Read(program.Span{}, body, second)) == 0 {
		t.Fatal("formal-cell typeof")
	}
	local := b.Cell(program.Span{}, body)
	if b.TypeOf(program.Span{}, local, b.Read(program.Span{}, body, first)) == 0 {
		t.Fatal("first-root local typeof(formal)")
	}
	localBind := b.Bind(program.Span{}, body, []program.Term{local}, b.Values(program.Span{}, body, []program.Term{b.Nil(program.Span{}, body)}, 0))
	fnCell := b.Cell(program.Span{}, entry)
	fnBind := b.Bind(program.Span{}, entry, []program.Term{fnCell}, b.Values(program.Span{}, entry, []program.Term{fn}, 0))
	if !b.SetBody(body, localBind) || !b.SetBody(entry, fnBind) {
		t.Fatal("SetBody")
	}
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestTypeOfStaticReadVisibilityAndGlobalEvidence(t *testing.T) {
	t.Run("earlier and global are visible", func(t *testing.T) {
		b, entry := entryBuilder(t)
		earlier := b.Cell(program.Span{}, entry)
		earlyValues := b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0)
		earlyBind := b.Bind(program.Span{}, entry, []program.Term{earlier}, earlyValues)
		scope := staticScopeAfter(t, b, entry)
		read := b.Read(program.Span{}, entry, earlier)
		if b.TypeOf(program.Span{}, scope, read) == 0 {
			t.Fatal("TypeOf earlier")
		}
		global := b.Global(program.Span{}, "external")
		globalRead := b.Read(program.Span{}, entry, global)
		if b.TypeOf(program.Span{}, scope, globalRead) == 0 {
			t.Fatal("TypeOf global")
		}
		scopeValue := b.Nil(program.Span{}, entry)
		scopeBind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{scopeValue}, 0))
		if !b.SetBody(entry, earlyBind, scopeBind) {
			t.Fatal("SetBody")
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if p.ImplicitReadCount() != 0 {
			t.Fatalf("static global Read became implicit evidence: %d", p.ImplicitReadCount())
		}
	})
	t.Run("later same Body is rejected", func(t *testing.T) {
		b, entry := entryBuilder(t)
		scope := staticScopeAfter(t, b, entry)
		later := b.Cell(program.Span{}, entry)
		read := b.Read(program.Span{}, entry, later)
		b.TypeOf(program.Span{}, scope, read)
		scopeBind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		laterBind := b.Bind(program.Span{}, entry, []program.Term{later}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		b.SetBody(entry, scopeBind, laterBind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("later same-Body Cell accepted")
		}
	})
	t.Run("local host cannot see itself", func(t *testing.T) {
		b, entry := entryBuilder(t)
		scope := staticScopeAfter(t, b, entry)
		b.TypeOf(program.Span{}, scope, b.Read(program.Span{}, entry, scope))
		bind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		b.SetBody(entry, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("local Cell saw itself before declaration")
		}
	})
}

func TestTypeOfCannotShareExecutableParentAndRejectsFunctionLiteral(t *testing.T) {
	b, entry := entryBuilder(t)
	scope := staticScopeAfter(t, b, entry)
	v := b.Integer(program.Span{}, entry, 1)
	b.TypeOf(program.Span{}, scope, v)
	values := b.Values(program.Span{}, entry, []program.Term{v}, 0)
	b.SetBody(entry, b.Return(program.Span{}, entry, values))
	if _, err := b.Seal(); err == nil {
		t.Fatal("static operand also accepted as executable child")
	}

	b, entry = entryBuilder(t)
	body := b.Body(program.Span{})
	scope = staticScopeAfter(t, b, entry)
	fn := b.Function(program.Span{}, entry, body, nil, 0, nil)
	if got := b.TypeOf(program.Span{}, scope, fn); got != 0 {
		t.Fatalf("typeof Function literal = %v, want rejection", got)
	}
}

func TestTypeOfRejectsUnsupportedStaticExpressionShapes(t *testing.T) {
	t.Run("ambiguous function host", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		fn := b.Function(program.Span{}, entry, body, nil, 0, nil)
		if got := b.TypeOf(program.Span{}, fn, b.Integer(program.Span{}, body, 1)); got != 0 {
			t.Fatalf("Function host = %v, want rejection", got)
		}
	})
	t.Run("implicit global", func(t *testing.T) {
		b, entry := entryBuilder(t)
		scope := staticScopeAfter(t, b, entry)
		read := b.ImplicitRead(program.Span{}, entry, b.Global(program.Span{}, "missing"))
		b.TypeOf(program.Span{}, scope, read)
		bind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		b.SetBody(entry, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("static implicit Read accepted")
		}
	})
	t.Run("vararg", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		vararg := b.Cell(program.Span{}, body)
		fn := b.Function(program.Span{}, entry, body, nil, vararg, nil)
		b.TypeOf(program.Span{}, fn, b.Vararg(program.Span{}, body, vararg))
		if !b.SetBody(body) {
			t.Fatal("SetBody")
		}
		cell := b.Cell(program.Span{}, entry)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, b.Values(program.Span{}, entry, []program.Term{fn}, 0))
		b.SetBody(entry, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("static vararg accepted")
		}
	})
	t.Run("nested function", func(t *testing.T) {
		b, entry := entryBuilder(t)
		scope := staticScopeAfter(t, b, entry)
		body := b.Body(program.Span{})
		fn := b.Function(program.Span{}, entry, body, nil, 0, nil)
		call := b.Call(program.Span{}, entry, fn, 0, b.Values(program.Span{}, entry, nil, 0))
		b.TypeOf(program.Span{}, scope, call)
		if !b.SetBody(body) {
			t.Fatal("SetBody")
		}
		bind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		b.SetBody(entry, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("nested static Function accepted")
		}
	})
}

func TestTypeOfStaticMethodCallAndWrongBody(t *testing.T) {
	b, entry := entryBuilder(t)
	receiverCell := b.Cell(program.Span{}, entry)
	receiverBind := b.Bind(program.Span{}, entry, []program.Term{receiverCell}, b.Values(program.Span{}, entry, []program.Term{b.Table(program.Span{}, entry, nil, nil, nil)}, 0))
	scope := staticScopeAfter(t, b, entry)
	receiver := b.Read(program.Span{}, entry, receiverCell)
	name := b.Name(program.Span{}, entry, "m")
	lens := b.LensExact(program.Span{}, entry, receiver, name, program.FieldName)
	callee := b.Read(program.Span{}, entry, lens)
	call := b.Call(program.Span{}, entry, callee, receiver, b.Values(program.Span{}, entry, nil, 0))
	b.TypeOf(program.Span{}, scope, call)
	scopeBind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
	if !b.SetBody(entry, receiverBind, scopeBind) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if !p.Static(call) || !p.Static(lens) {
		t.Fatalf("method static classification missing")
	}
	if _, _, _, _, direct, _ := p.Call(call); direct != 0 {
		t.Fatalf("static method direct = %v", direct)
	}

	b, entry = entryBuilder(t)
	scope = staticScopeAfter(t, b, entry)
	other := b.Body(program.Span{})
	v := b.Integer(program.Span{}, other, 1)
	b.TypeOf(program.Span{}, scope, v)
	bind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
	b.SetBody(other)
	b.SetBody(entry, bind)
	if _, err := b.Seal(); err == nil {
		t.Fatal("wrong Body static operand accepted")
	}
}

func TestTypeOfNestedContainmentIsIterative(t *testing.T) {
	b, entry := entryBuilder(t)
	base := b.Cell(program.Span{}, entry)
	baseBind := b.Bind(program.Span{}, entry, []program.Term{base}, b.Values(program.Span{}, entry, []program.Term{b.Integer(program.Span{}, entry, 1)}, 0))
	scope := staticScopeAfter(t, b, entry)
	term := b.Read(program.Span{}, entry, base)
	for i := 0; i < 4096; i++ {
		term = b.Unary(program.Span{}, entry, program.UnaryNot, term)
	}
	if b.TypeOf(program.Span{}, scope, term) == 0 {
		t.Fatal("TypeOf")
	}
	scopeBind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
	if !b.SetBody(entry, baseBind, scopeBind) {
		t.Fatal("SetBody")
	}
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestTypeOfStaticForestOwnershipAndDescendants(t *testing.T) {
	t.Run("one operand one root", func(t *testing.T) {
		b, entry := entryBuilder(t)
		left, right := staticScopeAfter(t, b, entry), staticScopeAfter(t, b, entry)
		value := b.Integer(program.Span{}, entry, 1)
		b.TypeOf(program.Span{}, left, value)
		b.TypeOf(program.Span{}, right, value)
		leftBind := b.Bind(program.Span{}, entry, []program.Term{left}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		rightBind := b.Bind(program.Span{}, entry, []program.Term{right}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		b.SetBody(entry, leftBind, rightBind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("one operand was accepted by two typeof roots")
		}
	})
	t.Run("table values lens and key", func(t *testing.T) {
		b, entry := entryBuilder(t)
		base := b.Cell(program.Span{}, entry)
		baseBind := b.Bind(program.Span{}, entry, []program.Term{base}, b.Values(program.Span{}, entry, []program.Term{b.Table(program.Span{}, entry, nil, nil, nil)}, 0))
		scope := staticScopeAfter(t, b, entry)
		baseRead := b.Read(program.Span{}, entry, base)
		lensKey := b.Name(program.Span{}, entry, "field")
		lens := b.LensExact(program.Span{}, entry, baseRead, lensKey, program.FieldName)
		fieldRead := b.Read(program.Span{}, entry, lens)
		fieldValues := b.Values(program.Span{}, entry, []program.Term{fieldRead}, 0)
		tableKey := b.Name(program.Span{}, entry, "answer")
		table := b.Table(program.Span{}, entry, []program.Term{tableKey}, []program.Term{fieldValues}, []program.FieldKind{program.FieldName})
		typeOf := b.TypeOf(program.Span{}, scope, table)
		scopeBind := b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
		if !b.SetBody(entry, baseBind, scopeBind) {
			t.Fatal("SetBody")
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		for _, term := range []program.Term{typeOf, table, tableKey, fieldValues, fieldRead, lens, lensKey, baseRead} {
			if !p.Static(term) {
				t.Fatalf("descendant %v is not static", term)
			}
		}
	})
}

func TestTypeOfManyRootsAndReadsRemainLinear(t *testing.T) {
	b, entry := entryBuilder(t)
	base := b.Cell(program.Span{}, entry)
	roots := []program.Term{b.Bind(program.Span{}, entry, []program.Term{base}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))}
	for i := 0; i < 1024; i++ {
		scope := staticScopeAfter(t, b, entry)
		read := b.Read(program.Span{}, entry, base)
		if b.TypeOf(program.Span{}, scope, read) == 0 {
			t.Fatal("TypeOf")
		}
		roots = append(roots, b.Bind(program.Span{}, entry, []program.Term{scope}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0)))
	}
	if !b.SetBody(entry, roots...) {
		t.Fatal("SetBody")
	}
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestNoTypeOfManyBindsKeepsStaticClassificationEmpty(t *testing.T) {
	b, entry := entryBuilder(t)
	roots := make([]program.Term, 0, 2048)
	for i := 0; i < cap(roots); i++ {
		cell := b.Cell(program.Span{}, entry)
		roots = append(roots, b.Bind(program.Span{}, entry, []program.Term{cell}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0)))
	}
	if !b.SetBody(entry, roots...) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < p.TermCount(); i += 127 {
		if term, ok := p.CellAt(i % p.CellCount()); ok && p.Static(term) {
			t.Fatalf("runtime Cell %v classified static", term)
		}
	}
}

func TestTypeOfFirstLoopBodyRootSeesLoopCell(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	loopCell := b.Cell(program.Span{}, body)
	control := b.Values(program.Span{}, entry, []program.Term{b.Integer(program.Span{}, entry, 1), b.Integer(program.Span{}, entry, 2)}, 0)
	loop := b.Loop(program.Span{}, entry, body, control, []program.Term{loopCell}, program.LoopNumericFor)
	scope := b.Cell(program.Span{}, body)
	if b.TypeOf(program.Span{}, scope, b.Read(program.Span{}, body, loopCell)) == 0 {
		t.Fatal("first-root typeof(loopCell)")
	}
	bind := b.Bind(program.Span{}, body, []program.Term{scope}, b.Values(program.Span{}, body, []program.Term{b.Nil(program.Span{}, body)}, 0))
	if !b.SetBody(body, bind) || !b.SetBody(entry, loop) {
		t.Fatal("SetBody")
	}
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestTypeOfAliasGapUsesDeclarationFrontier(t *testing.T) {
	b, entry := entryBuilder(t)
	cell := b.Cell(program.Span{}, entry)
	bind := b.Bind(program.Span{}, entry, []program.Term{cell}, b.Values(program.Span{}, entry, []program.Term{b.Nil(program.Span{}, entry)}, 0))
	alias := b.DeclareTypeAlias(program.Span{}, entry, "T")
	if alias == 0 || !b.SetTypeAliasGap(alias, 1) {
		t.Fatal("alias declare/place")
	}
	target := b.TypeOf(program.Span{}, alias, b.Read(program.Span{}, entry, cell))
	if target == 0 || !b.SetTypeAliasParams(alias, nil) || !b.FillTypeAlias(alias, target) {
		t.Fatal("alias predeclare/fill")
	}
	if !b.SetBody(entry, bind) {
		t.Fatal("SetBody")
	}
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}
