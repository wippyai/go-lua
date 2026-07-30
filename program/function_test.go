package program

import "testing"

func TestFunctionTwoPhaseHeaderRetainsExactRelations(t *testing.T) {
	b := NewBuilder()
	entry := b.Body(Span{})
	if !b.SetEntry(entry) {
		t.Fatal("SetEntry")
	}
	outer := b.Cell(Span{}, entry)
	outerBind := b.Bind(Span{}, entry, []Term{outer}, b.Values(Span{}, entry, []Term{b.Nil(Span{}, entry)}, 0))

	function := b.DeclareFunction(Span{}, entry)
	if function == 0 || !b.SetFunctionOuterGap(function, 1) {
		t.Fatal("DeclareFunction")
	}
	param := b.DeclareTypeParam(Span{}, function, "T")
	if param == 0 || !b.SetFunctionGenerics(function, []Term{param}) {
		t.Fatal("SetFunctionGenerics")
	}
	constraint := b.TypeOf(Span{}, param, b.Read(Span{}, entry, outer))
	if constraint == 0 || !b.FillTypeParam(param, constraint) {
		t.Fatal("generic constraint")
	}

	body := b.Body(Span{})
	formal := b.Cell(Span{}, body)
	if !b.FillFunction(function, body, []Term{formal}, 0, nil) {
		t.Fatal("FillFunction")
	}
	formalType := b.TypeOf(Span{}, formal, b.Read(Span{}, body, formal))
	if formalType == 0 || b.DeclareCellType(Span{}, formal, formalType) == 0 {
		t.Fatal("formal declared type")
	}
	assertNarrow := b.TypeOf(Span{}, function, b.Read(Span{}, body, formal))
	assertion := b.Assertion(Span{}, "value", 0, assertNarrow)
	if assertion == 0 || !b.SetFunctionReturns(function, true, []Term{assertion}) {
		t.Fatal("SetFunctionReturns")
	}

	functionCell := b.Cell(Span{}, entry)
	functionBind := b.Bind(Span{}, entry, []Term{functionCell}, b.Values(Span{}, entry, []Term{function}, 0))
	if !b.SetBody(body) || !b.SetBody(entry, outerBind, functionBind) {
		t.Fatal("SetBody")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.FunctionTypeParamAt(function, 0); !ok || got != param {
		t.Fatalf("FunctionTypeParamAt = %v/%v, want %v/true", got, ok, param)
	}
	if owner, _, gotConstraint, ok := p.TypeParam(param); !ok || owner != function || gotConstraint != constraint {
		t.Fatalf("TypeParam = owner %v constraint %v ok %v", owner, gotConstraint, ok)
	}
	if known, ok := p.FunctionReturnsKnown(function); !ok || !known {
		t.Fatalf("FunctionReturnsKnown = %v/%v", known, ok)
	}
	if got, ok := p.FunctionReturnAt(function, 0); !ok || got != assertion {
		t.Fatalf("FunctionReturnAt = %v/%v, want %v/true", got, ok, assertion)
	}
	if got, ok := p.CellDeclaredType(formal); !ok || got == 0 {
		t.Fatalf("CellDeclaredType = %v/%v", got, ok)
	}
}

func TestFunctionReturnsPreserveOmittedAndExplicitEmpty(t *testing.T) {
	for _, known := range []bool{false, true} {
		b := NewBuilder()
		entry := b.Body(Span{})
		if !b.SetEntry(entry) {
			t.Fatal("SetEntry")
		}
		function := b.DeclareFunction(Span{}, entry)
		body := b.Body(Span{})
		if function == 0 || !b.FillFunction(function, body, nil, 0, nil) || !b.SetFunctionReturns(function, known, nil) {
			t.Fatal("function header")
		}
		cell := b.Cell(Span{}, entry)
		bind := b.Bind(Span{}, entry, []Term{cell}, b.Values(Span{}, entry, []Term{function}, 0))
		if !b.SetBody(body) || !b.SetBody(entry, bind) {
			t.Fatal("SetBody")
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := p.FunctionReturnsKnown(function); !ok || got != known {
			t.Fatalf("FunctionReturnsKnown = %v/%v, want %v/true", got, ok, known)
		}
		if count, ok := p.FunctionReturnCount(function); !ok || count != 0 {
			t.Fatalf("FunctionReturnCount = %d/%v", count, ok)
		}
	}
}

func TestFunctionHeaderPhasesRejectOutOfOrderMutation(t *testing.T) {
	newFunction := func(t *testing.T) (*Builder, Term, Term) {
		t.Helper()
		b := NewBuilder()
		entry := b.Body(Span{})
		if !b.SetEntry(entry) {
			t.Fatal("SetEntry")
		}
		return b, entry, b.DeclareFunction(Span{}, entry)
	}
	t.Run("generic phase requires outer frontier", func(t *testing.T) {
		b, _, function := newFunction(t)
		if b.SetFunctionGenerics(function, nil) {
			t.Fatal("SetFunctionGenerics without outer gap succeeded")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("poisoned out-of-order generic phase sealed")
		}
	})
	t.Run("Fill rejects incomplete header phase", func(t *testing.T) {
		b, _, function := newFunction(t)
		body := b.Body(Span{})
		if !b.SetFunctionOuterGap(function, 0) {
			t.Fatal("SetFunctionOuterGap")
		}
		if b.FillFunction(function, body, nil, 0, nil) {
			t.Fatal("FillFunction with unfinalized generic phase succeeded")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("incomplete header phase sealed")
		}
	})
	t.Run("Seal rejects unfilled incomplete header phase", func(t *testing.T) {
		b, entry, function := newFunction(t)
		if !b.SetFunctionOuterGap(function, 0) {
			t.Fatal("SetFunctionOuterGap")
		}
		cell := b.Cell(Span{}, entry)
		bind := b.Bind(Span{}, entry, []Term{cell}, b.Values(Span{}, entry, []Term{function}, 0))
		if !b.SetBody(entry, bind) {
			t.Fatal("SetBody")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("unfilled incomplete header phase sealed")
		}
	})
	t.Run("generics cannot follow Fill", func(t *testing.T) {
		b, _, function := newFunction(t)
		body := b.Body(Span{})
		if !b.FillFunction(function, body, nil, 0, nil) {
			t.Fatal("untyped FillFunction")
		}
		if b.SetFunctionGenerics(function, nil) {
			t.Fatal("SetFunctionGenerics after Fill succeeded")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("post-Fill generic mutation sealed")
		}
	})
	t.Run("returns require filled function", func(t *testing.T) {
		b, _, function := newFunction(t)
		if b.SetFunctionReturns(function, false, nil) {
			t.Fatal("SetFunctionReturns before Fill succeeded")
		}
		if _, err := b.Seal(); err == nil {
			t.Fatal("pre-Fill return mutation sealed")
		}
	})
}
