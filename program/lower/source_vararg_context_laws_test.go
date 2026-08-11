package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// These laws cover the source contexts that cannot be exercised by a call
// producer alone: Lua's vararg expression is open only where the grammar
// permits a final multi-result producer.  The expected shape is derived from
// one concrete parsed function and then checked on its sealed Program body.
// They deliberately do not use schema catalog context tags.
func TestSourceVarargContextExpansionLaws(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		t.Run("non-final is scalar", func(t *testing.T) {
			p, _, body, _ := varargContextProgram(t, "return ..., 0")
			returned := contextBodySource(t, p, body, 0)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("Return has no Values")
			}
			contextFixedVararg(t, p, values, 0, 2)
		})
		t.Run("final is open", func(t *testing.T) {
			p, _, body, vararg := varargContextProgram(t, "return 0, ...")
			returned := contextBodySource(t, p, body, 0)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("Return has no Values")
			}
			contextOpenVararg(t, p, values, vararg, 1)
		})
		t.Run("parenthesized final is scalar", func(t *testing.T) {
			p, _, body, _ := varargContextProgram(t, "return (...)")
			returned := contextBodySource(t, p, body, 0)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("Return has no Values")
			}
			contextFixedVararg(t, p, values, 0, 1)
		})
	})

	t.Run("local binding and assignment", func(t *testing.T) {
		p, _, body, vararg := varargContextProgram(t, "local a, b = ..., ...\na, b = ..., ...")
		bind := contextBodySource(t, p, body, 0)
		_, bindValues, ok := p.Flow().Authored().Storage().Binds().Get(bind)
		width, widthOK := p.Source().Binds().Len(bind)
		if !ok || !widthOK || width != 2 {
			t.Fatalf("BindValues = %v/%d/%v, want width 2", bindValues, width, ok)
		}
		contextOpenVararg(t, p, bindValues, vararg, 1)

		assign := contextBodySource(t, p, body, 1)
		_, assignValues, ok := p.Flow().Authored().Storage().Assigns().Get(assign)
		width, widthOK = p.Flow().Authored().Storage().Assigns().WriteCount(assign)
		if !ok || !widthOK || width != 2 {
			t.Fatalf("AssignValues = %v/%d/%v, want width 2", assignValues, width, ok)
		}
		contextOpenVararg(t, p, assignValues, vararg, 1)

		fixed, _, fixedBody, _ := varargContextProgram(t, "local one = (...)\nlocal a, b\na, b = (...), (...)")
		one := contextBodySource(t, fixed, fixedBody, 0)
		_, oneValues, ok := fixed.Flow().Authored().Storage().Binds().Get(one)
		if !ok {
			t.Fatal("parenthesized Bind has no Values")
		}
		contextFixedVararg(t, fixed, oneValues, 0, 1)
		parenthesizedAssign := contextBodySource(t, fixed, fixedBody, 2)
		_, parenthesizedValues, ok := fixed.Flow().Authored().Storage().Assigns().Get(parenthesizedAssign)
		if !ok {
			t.Fatal("parenthesized Assign has no Values")
		}
		contextFixedVararg(t, fixed, parenthesizedValues, 0, 2)
	})

	t.Run("call actuals", func(t *testing.T) {
		p, _, body, vararg := varargContextProgram(t, "sink(..., ...)")
		call := contextBodySource(t, p, body, 0)
		_, _, _, actuals, ok := p.Flow().Authored().Calls().Get(call)
		if !ok {
			t.Fatal("authored sink call is absent")
		}
		contextOpenVararg(t, p, actuals, vararg, 1)

		fixed, _, fixedBody, _ := varargContextProgram(t, "sink((...))")
		call = contextBodySource(t, fixed, fixedBody, 0)
		_, _, _, actuals, ok = fixed.Flow().Authored().Calls().Get(call)
		if !ok {
			t.Fatal("parenthesized sink call is absent")
		}
		contextFixedVararg(t, fixed, actuals, 0, 1)
	})

	t.Run("table list fields", func(t *testing.T) {
		p, _, body, vararg := varargContextProgram(t, "return {..., ...}")
		returned := contextBodySource(t, p, body, 0)
		_, returnValues, ok := p.Flow().Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatal("Return has no Values")
		}
		table := valueAt(t, p, returnValues, 0)
		first, ok := p.Flow().Authored().Tables().FieldAt(table, 0)
		if !ok {
			t.Fatal("first list field is absent")
		}
		firstValues, firstOpen, ok := p.Flow().Authored().Fields().Values(first)
		if !ok || firstOpen {
			t.Fatalf("first list field = Values %v finalOpen %v/%v, want scalar", firstValues, firstOpen, ok)
		}
		contextFixedVararg(t, p, firstValues, 0, 1)

		last, ok := p.Flow().Authored().Tables().FieldAt(table, 1)
		if !ok {
			t.Fatal("final list field is absent")
		}
		lastValues, lastOpen, ok := p.Flow().Authored().Fields().Values(last)
		if !ok || !lastOpen {
			t.Fatalf("final list field = Values %v finalOpen %v/%v, want open", lastValues, lastOpen, ok)
		}
		contextOpenVararg(t, p, lastValues, vararg, 0)

		fixed, _, fixedBody, _ := varargContextProgram(t, "return {(...)}")
		returned = contextBodySource(t, fixed, fixedBody, 0)
		_, returnValues, _ = fixed.Flow().Authored().Control().Returns().Get(returned)
		table = valueAt(t, fixed, returnValues, 0)
		field, _ := fixed.Flow().Authored().Tables().FieldAt(table, 0)
		fieldValues, finalOpen, ok := fixed.Flow().Authored().Fields().Values(field)
		if !ok || finalOpen {
			t.Fatalf("parenthesized final field = Values %v finalOpen %v/%v, want scalar", fieldValues, finalOpen, ok)
		}
		contextFixedVararg(t, fixed, fieldValues, 0, 1)
	})

	t.Run("loop headers", func(t *testing.T) {
		generic, _, body, vararg := varargContextProgram(t, "for key in ..., ... do end")
		loop := contextBodySource(t, generic, body, 0)
		_, _, genericKind, header, ok := generic.Flow().Authored().Control().Loops().Get(loop)
		if !ok || genericKind != kind.LoopGenericFor {
			t.Fatal("generic-for header has no Values")
		}
		contextOpenVararg(t, generic, header, vararg, 1)

		genericNonFinal, _, nonFinalBody, _ := varargContextProgram(t, "for key in ..., 0 do end")
		loop = contextBodySource(t, genericNonFinal, nonFinalBody, 0)
		_, _, genericKind, header, ok = genericNonFinal.Flow().Authored().Control().Loops().Get(loop)
		if !ok || genericKind != kind.LoopGenericFor {
			t.Fatal("non-final generic-for header has no Values")
		}
		contextFixedVararg(t, genericNonFinal, header, 0, 2)

		genericFixed, _, fixedBody, _ := varargContextProgram(t, "for key in (...) do end")
		loop = contextBodySource(t, genericFixed, fixedBody, 0)
		_, _, genericKind, header, ok = genericFixed.Flow().Authored().Control().Loops().Get(loop)
		if !ok || genericKind != kind.LoopGenericFor {
			t.Fatal("parenthesized generic-for header has no Values")
		}
		contextFixedVararg(t, genericFixed, header, 0, 1)

		numeric, _, numericBody, _ := varargContextProgram(t, "for i = ..., ..., (...) do end")
		loop = contextBodySource(t, numeric, numericBody, 0)
		_, _, numericKind, header, ok := numeric.Flow().Authored().Control().Loops().Get(loop)
		width, widthOK := numeric.Flow().Authored().Values().Len(header)
		if !ok || !widthOK || numericKind != kind.LoopNumericFor || width != 3 {
			t.Fatalf("numeric-for header = %v/%d/%v, want three scalar operands", header, width, ok)
		}
		if tail := valuesTail(t, numeric, header); tail != 0 {
			t.Fatalf("numeric-for header retained open tail %v", tail)
		}
		if fixed, ok := numeric.Flow().Authored().Values().Len(header); !ok || fixed != 3 {
			t.Fatalf("numeric-for fixed operands = %d/%v, want 3", fixed, ok)
		}
		for index := 0; index < 3; index++ {
			operand := valueAt(t, numeric, header, index)
			if _, cell, ok := numeric.Flow().Authored().Storage().Varargs().Get(operand); !ok || cell == 0 {
				t.Fatalf("numeric-for operand %d = %v, want scalar Vararg", index, operand)
			}
		}
	})
}

func TestSourceVarargCaptureBoundaryLaw(t *testing.T) {
	p, _, body, _ := varargContextProgram(t, "local snapshot = ...\nreturn function() return snapshot end")
	bind := contextBodySource(t, p, body, 0)
	snapshot := boundCell(t, p, bind, 0)
	returned := contextBodySource(t, p, body, 1)
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatal("closure Return has no Values")
	}
	inner := valueAt(t, p, values, 0)
	_, child, _, ok := p.Flow().Authored().Functions().Get(inner)
	if !ok || child == 0 {
		t.Fatalf("returned closure = Function body %v ok %v", child, ok)
	}
	capture, captured, ok := p.Flow().Authored().Functions().CaptureAt(inner, 0)
	if !ok || captured != snapshot {
		t.Fatalf("closure capture = inner %v outer %v ok %v, want snapshot Cell %v", capture, captured, ok, snapshot)
	}
	if cellKind, cellBody, _, ok := p.Flow().Authored().Storage().Cells().Get(capture); !ok || cellKind != flow.CellLocal || cellBody != child {
		t.Fatalf("closure capture Cell = kind %v body %v ok %v, want local child Cell", cellKind, cellBody, ok)
	}
	if parent, ok := p.Source().Index().BodyParent(child); !ok || parent != body {
		t.Fatalf("closure Body parent = %v/%v, want lexical function Body %v", parent, ok, body)
	}
}

func TestSourceVarargNestedOrdinaryBodyLaws(t *testing.T) {
	t.Run("chunk occurrence keeps entry Cell across nested block", func(t *testing.T) {
		p, err := lowerSource("do\nreturn ...\nend\n")
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := p.Source().Index().Entry()
		if !ok || entry == 0 {
			t.Fatal("missing chunk entry Body")
		}
		nested, ok := p.Source().Order().BodyAt(entry, 0)
		if !ok || nested == 0 {
			t.Fatal("missing nested ordinary Body")
		}
		if parent, ok := p.Source().Index().BodyParent(nested); !ok || parent != entry {
			t.Fatalf("nested Body parent = %v/%v, want entry %v", parent, ok, entry)
		}
		returned := contextBodySource(t, p, nested, 0)
		_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatal("nested Return has no Values")
		}
		tail := valuesTail(t, p, values)
		owner, cell, ok := p.Flow().Authored().Storage().Varargs().Get(tail)
		if !ok || owner != nested || cell == 0 {
			t.Fatalf("nested chunk Vararg = owner %v Cell %v/%v, want owner %v", owner, cell, ok, nested)
		}
		if cellKind, host, _, ok := p.Flow().Authored().Storage().Cells().Get(cell); !ok || cellKind != flow.CellLocal || host != entry {
			t.Fatalf("chunk Vararg Cell = kind %v host %v/%v, want local entry Cell %v", cellKind, host, ok, entry)
		}
	})

	t.Run("function occurrence keeps function Cell across nested block", func(t *testing.T) {
		p, _, functionBody, functionVararg := varargContextProgram(t, "do\nreturn ...\nend")
		nested, ok := p.Source().Order().BodyAt(functionBody, 0)
		if !ok || nested == 0 {
			t.Fatal("missing nested function Body")
		}
		if parent, ok := p.Source().Index().BodyParent(nested); !ok || parent != functionBody {
			t.Fatalf("nested function Body parent = %v/%v, want function Body %v", parent, ok, functionBody)
		}
		returned := contextBodySource(t, p, nested, 0)
		_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatal("nested function Return has no Values")
		}
		tail := valuesTail(t, p, values)
		owner, cell, ok := p.Flow().Authored().Storage().Varargs().Get(tail)
		if !ok || owner != nested || cell != functionVararg {
			t.Fatalf("nested function Vararg = owner %v Cell %v/%v, want owner %v Cell %v", owner, cell, ok, nested, functionVararg)
		}
		if cellKind, host, _, ok := p.Flow().Authored().Storage().Cells().Get(cell); !ok || cellKind != flow.CellLocal || host != functionBody {
			t.Fatalf("function Vararg Cell = kind %v host %v/%v, want local function Cell %v", cellKind, host, ok, functionBody)
		}
	})
}

// varargContextProgram binds the unique function expression to its precise
// parsed span before following its Program Body. It is intentionally specific
// to these concrete vararg laws, not a reusable source-context registry.
func varargContextProgram(t *testing.T, bodySource string) (*program.Program, *ast.FunctionExpr, keyspace.Term, keyspace.Term) {
	t.Helper()
	input := "local function run(...)\n" + bodySource + "\nend\n"
	statements, err := parse.ParseString(input, "fixture.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 1 {
		t.Fatalf("parsed top-level statements = %d, want one local function", len(statements))
	}
	decl, ok := statements[0].(*ast.LocalAssignStmt)
	if !ok || !decl.LocalFunction || len(decl.Exprs) != 1 {
		t.Fatalf("parsed declaration = %#v, want normalized local function", statements[0])
	}
	function, ok := decl.Exprs[0].(*ast.FunctionExpr)
	if !ok || function.ParList == nil || !function.ParList.HasVargs {
		t.Fatalf("parsed function = %#v, want vararg function", decl.Exprs[0])
	}
	p := parseBindLower(t, input)
	want := source.Span{File: "fixture.lua", StartLine: uint32(function.Line()), StartCol: uint32(function.Column()), EndLine: uint32(function.LastLine()), EndCol: uint32(function.LastColumn())}
	var term keyspace.Term
	functions := p.Flow().Authored().Functions()
	for index := 0; index < functions.Count(); index++ {
		candidate, ok := functions.At(index)
		if !ok {
			t.Fatalf("FunctionAt(%d) is absent", index)
		}
		span, ok := p.Source().Identity().Span(candidate)
		if !ok || span != want {
			continue
		}
		if term != 0 {
			t.Fatalf("parsed run function span %#v has multiple Program Functions", want)
		}
		term = candidate
	}
	if term == 0 {
		t.Fatalf("no Program Function has parsed run span %#v", want)
	}
	_, body, vararg, ok := functions.Get(term)
	if !ok || body == 0 || vararg == 0 {
		t.Fatalf("Function = body %v vararg %v ok %v", body, vararg, ok)
	}
	return p, function, body, vararg
}

func contextBodySource(t *testing.T, p *program.Program, body keyspace.Term, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Source().Order().BodyAt(body, index)
	if !ok || term == 0 {
		t.Fatalf("BodySourceAt(%v, %d) = %v/%v", body, index, term, ok)
	}
	return term
}

func contextOpenVararg(t *testing.T, p *program.Program, values, vararg keyspace.Term, fixedWant int) {
	t.Helper()
	fixed, ok := p.Flow().Authored().Values().Len(values)
	tail := valuesTail(t, p, values)
	_, cell, varargOK := p.Flow().Authored().Storage().Varargs().Get(tail)
	if !ok || fixed != fixedWant || !varargOK || cell != vararg {
		t.Fatalf("Values(%v) = fixed %d/%v tail %v→Cell %v/%v, want fixed %d and function vararg Cell %v", values, fixed, ok, tail, cell, varargOK, fixedWant, vararg)
	}
}

func contextFixedVararg(t *testing.T, p *program.Program, values keyspace.Term, index, fixedWant int) {
	t.Helper()
	fixed, ok := p.Flow().Authored().Values().Len(values)
	if !ok || fixed != fixedWant || valuesTail(t, p, values) != 0 {
		t.Fatalf("Values(%v) = fixed %d/%v tail %v, want %d fixed scalars", values, fixed, ok, valuesTail(t, p, values), fixedWant)
	}
	vararg := valueAt(t, p, values, index)
	if _, cell, ok := p.Flow().Authored().Storage().Varargs().Get(vararg); !ok || cell == 0 {
		t.Fatalf("Values(%v)[%d] = %v, want scalar Vararg", values, index, vararg)
	}
}
