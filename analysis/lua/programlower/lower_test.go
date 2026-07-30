package programlower_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/programlower"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
)

var loweredSink *program.Program

func parseBindLower(t *testing.T, source string) *program.Program {
	t.Helper()
	stmts, err := parse.ParseString(source, "fixture.lua")
	if err != nil {
		t.Fatal(err)
	}
	binding := bind.BindChunk(stmts, bind.Options{})
	lowered, err := programlower.Lower("fixture.lua", stmts, binding)
	if err != nil {
		t.Fatal(err)
	}
	return lowered
}

func TestEmptyChunkHasOneCanonicalEntry(t *testing.T) {
	p := parseBindLower(t, "")
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("empty chunk has no Entry")
	}
	roots, ok := p.AppendBody(entry, nil)
	if !ok || len(roots) != 1 {
		t.Fatalf("empty Entry roots = %v, %v; want one Normal outcome", roots, ok)
	}
	values, ok := p.Normal(roots[0])
	if !ok {
		t.Fatalf("empty Entry root is not Normal: %v", roots[0])
	}
	if count, ok := p.ValuesLen(values); !ok || count != 0 {
		t.Fatalf("empty Entry ValuesLen = %d, %v", count, ok)
	}
	if tail, ok := p.ValuesTail(values); !ok || tail != 0 {
		t.Fatalf("empty Entry tail = %v, %v", tail, ok)
	}
}

func TestParseBindLowerLiteralLocalsTableAssignmentAndReturn(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
local t = {name = x, [2] = "two", [x] = false}
x = 3
return x, t
`)

	var bodies, cells, reads, binds, assigns, tables, lenses, returns int
	rootBody, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	var tableTerm, returnValues program.Term
	var cellTerms, bindTerms []program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, ok := p.TermAt(i)
		if !ok {
			t.Fatalf("TermAt(%d) failed", i)
		}
		if _, ok := p.BodyLen(term); ok {
			bodies++
		}
		if _, ok := p.Cell(term); ok {
			cells++
			cellTerms = append(cellTerms, term)
		}
		if _, ok := p.Read(term); ok {
			reads++
		}
		if _, ok := p.BindLen(term); ok {
			binds++
			bindTerms = append(bindTerms, term)
		}
		if _, ok := p.AssignLen(term); ok {
			assigns++
		}
		if p.Table(term) {
			tables++
			tableTerm = term
		}
		if _, _, dynamic, ok := p.Lens(term); ok {
			_ = dynamic
			lenses++
		}
		if values, ok := p.Return(term); ok {
			returns++
			returnValues = values
		}
		if value, ok := p.Integer(term); ok && value == 1 {
			span, ok := p.Span(term)
			if !ok || span.File != "fixture.lua" || span.StartLine != 2 {
				t.Fatalf("literal span = %#v, %v", span, ok)
			}
		}
	}
	if bodies != 1 || cells != 2 || reads != 4 || binds != 2 || assigns != 1 || tables != 1 {
		t.Fatalf("counts: bodies=%d cells=%d reads=%d binds=%d assigns=%d tables=%d",
			bodies, cells, reads, binds, assigns, tables)
	}
	if lenses != 0 {
		t.Fatalf("constructor minted %d Lens terms; want none", lenses)
	}
	if returns != 1 {
		t.Fatalf("returns = %d", returns)
	}
	roots, ok := p.AppendBody(rootBody, nil)
	if !ok || len(roots) != 4 {
		t.Fatalf("root Body = %v, %v; want four statement roots", roots, ok)
	}
	if _, ok := p.BindLen(roots[0]); !ok {
		t.Fatalf("root 0 is not Bind: %v", roots[0])
	}
	if _, ok := p.BindLen(roots[1]); !ok {
		t.Fatalf("root 1 is not Bind: %v", roots[1])
	}
	if _, ok := p.AssignLen(roots[2]); !ok {
		t.Fatalf("root 2 is not Assign: %v", roots[2])
	}
	if _, ok := p.Return(roots[3]); !ok {
		t.Fatalf("root 3 is not Return: %v", roots[3])
	}
	for i, binding := range bindTerms {
		values, ok := p.BindValues(binding)
		if !ok {
			t.Fatalf("BindValues(%d) failed", i)
		}
		order, ok := p.AppendOrder(binding, nil)
		if !ok || len(order) != 1 || order[0] != values {
			t.Fatalf("Bind %d order = %v, %v; want only RHS Values %v", i, order, ok, values)
		}
	}
	for i, cell := range cellTerms {
		owner, ok := p.Cell(cell)
		if !ok || owner != rootBody {
			t.Fatalf("Cell %d owner = %v, %v; want root Body %v", i, owner, ok, rootBody)
		}
		span, ok := p.Span(cell)
		if !ok || span.File != "fixture.lua" || span.StartLine != i+2 || span.StartCol != 7 {
			t.Fatalf("Cell %d span = %#v, %v", i, span, ok)
		}
	}
	if count, ok := p.TableLen(tableTerm); !ok || count != 3 {
		t.Fatalf("TableLen = %d, %v; want three direct fields", count, ok)
	}
	for i, wantKind := range []program.FieldKind{program.FieldExact, program.FieldExact, program.FieldKey} {
		key, values, kind, ok := p.TableAt(tableTerm, i)
		if !ok || key == 0 || values == 0 || kind != wantKind {
			t.Fatalf("TableAt(%d) = key %v values %v kind %v ok %v", i, key, values, kind, ok)
		}
		if count, ok := p.ValuesLen(values); !ok || count != 1 {
			t.Fatalf("field %d ValuesLen = %d, %v", i, count, ok)
		}
		if i == 0 {
			if got, ok := p.String(key); !ok || got != "name" {
				t.Fatalf("field 0 key = %q, %v", got, ok)
			}
		}
		if i == 1 {
			if got, ok := p.Integer(key); !ok || got != 2 {
				t.Fatalf("field 1 key = %d, %v", got, ok)
			}
			if span, ok := p.Span(key); !ok || span.StartLine != 3 || span.StartCol != 23 {
				t.Fatalf("field 1 key span = %#v, %v; want bracket key token", span, ok)
			}
		}
		if i == 2 {
			if _, ok := p.Read(key); !ok {
				t.Fatalf("dynamic field key is not a bound Read: %v", key)
			}
		}
	}
	seen := map[program.Term]bool{tableTerm: true}
	work := []program.Term{tableTerm}
	for len(work) != 0 {
		current := work[len(work)-1]
		work = work[:len(work)-1]
		count, ok := p.OrderCount(current)
		if !ok {
			continue
		}
		for i := 0; i < count; i++ {
			child, ok := p.OrderAt(current, i)
			if !ok {
				t.Fatalf("OrderAt(%v, %d) failed", current, i)
			}
			if child == tableTerm {
				t.Fatal("Table constructor order reaches its own allocation")
			}
			if !seen[child] {
				seen[child] = true
				work = append(work, child)
			}
		}
	}
	if count, ok := p.ValuesLen(returnValues); !ok || count != 2 {
		t.Fatalf("return Values = %d, %v", count, ok)
	}
	for i := 0; i < 2; i++ {
		value, ok := p.ValuesAt(returnValues, i)
		if !ok {
			t.Fatalf("return ValuesAt(%d) failed", i)
		}
		if _, ok := p.Read(value); !ok {
			t.Fatalf("return value %d is not a bound Read", i)
		}
	}
}

func TestBinderIdentityControlsShadowedRead(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
do
  local x = 2
  return x
end
`)
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	var cells []program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		if _, ok := p.Cell(term); ok {
			cells = append(cells, term)
		}
	}
	if len(cells) != 2 {
		t.Fatalf("cells = %d", len(cells))
	}
	outerRoots, ok := p.AppendBody(entry, nil)
	if !ok || len(outerRoots) != 2 {
		t.Fatalf("outer Body = %v, %v; want Bind then child Body", outerRoots, ok)
	}
	innerBody := outerRoots[1]
	if _, ok := p.BodyLen(innerBody); !ok || innerBody == entry {
		t.Fatalf("nested Body = %v, %v; Entry = %v", innerBody, ok, entry)
	}
	innerRoots, ok := p.AppendBody(innerBody, nil)
	if !ok || len(innerRoots) != 2 {
		t.Fatalf("inner Body = %v, %v; want Bind then Return", innerRoots, ok)
	}
	returnValues, ok := p.Return(innerRoots[1])
	if !ok {
		t.Fatalf("nested root is not Return: %v", innerRoots[1])
	}
	if owner, ok := p.Cell(cells[0]); !ok || owner != entry {
		t.Fatalf("outer Cell owner = %v, %v; want %v", owner, ok, entry)
	}
	if owner, ok := p.Cell(cells[1]); !ok || owner != innerBody {
		t.Fatalf("inner Cell owner = %v, %v; want %v", owner, ok, innerBody)
	}
	value, ok := p.ValuesAt(returnValues, 0)
	if !ok {
		t.Fatal("missing return value")
	}
	cell, ok := p.Read(value)
	if !ok || cell != cells[1] || cell == cells[0] {
		t.Fatalf("shadowed Read cell = %v, want inner %v", cell, cells[1])
	}
}

func TestSyntheticListKeyHasGeneratedCoordinates(t *testing.T) {
	p := parseBindLower(t, `return {"first"}`)
	var table program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		if p.Table(term) {
			table = term
			break
		}
	}
	key, _, kind, ok := p.TableAt(table, 0)
	if !ok {
		t.Fatal("missing table field")
	}
	if kind != program.FieldList {
		t.Fatalf("synthetic list kind = %v, want FieldList", kind)
	}
	span, ok := p.Span(key)
	if !ok || span.File != "fixture.lua" ||
		span.StartLine != 0 || span.StartCol != 0 || span.EndLine != 0 || span.EndCol != 0 {
		t.Fatalf("synthetic key span = %#v, %v", span, ok)
	}
}

func TestTypedLocalFailsUntilAuthoredTypesLand(t *testing.T) {
	stmts, err := parse.ParseString(`local x: number = 1`, "typed.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("typed.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unsupported declared type for local slot 0") {
		t.Fatalf("typed local error = %v", err)
	}
}

func TestUnknownTableKeySyntaxFailsExplicitly(t *testing.T) {
	key := &ast.StringExpr{Value: "key"}
	value := &ast.NumberExpr{Value: "1"}
	table := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       key,
		KeySyntax: ast.AttrKeyUnknown,
		Value:     value,
	}}}
	stmts := []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{table}}}
	_, err := programlower.Lower("manual.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unknown key syntax") {
		t.Fatalf("unknown table key syntax error = %v", err)
	}
}

func TestNilKeysRemainExactAuthoredOccurrences(t *testing.T) {
	p := parseBindLower(t, `return {[nil] = 1}`)
	var table program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		if p.Table(term) {
			table = term
			break
		}
	}
	key, _, kind, ok := p.TableAt(table, 0)
	if !ok || kind != program.FieldExact || !p.Nil(key) {
		t.Fatalf("nil table field = key %v kind %v ok %v", key, kind, ok)
	}

	p = parseBindLower(t, `
local t = {}
t[nil] = 1
return t
`)
	var lens program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		if _, _, _, ok := p.Lens(term); ok {
			if lens != 0 {
				t.Fatal("nil target minted more than one Lens")
			}
			lens = term
		}
	}
	_, key, dynamic, ok := p.Lens(lens)
	if !ok || dynamic || !p.Nil(key) {
		t.Fatalf("nil assignment Lens = key %v dynamic %v ok %v", key, dynamic, ok)
	}
}

func loweredReturnValue(t *testing.T, source string) (*program.Program, program.Term) {
	t.Helper()
	p := parseBindLower(t, source)
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	bodies := []program.Term{entry}
	for len(bodies) != 0 {
		body := bodies[len(bodies)-1]
		bodies = bodies[:len(bodies)-1]
		roots, ok := p.AppendBody(body, nil)
		if !ok {
			t.Fatalf("Entry-reachable term is not Body: %v", body)
		}
		for _, root := range roots {
			if _, ok := p.BodyLen(root); ok {
				bodies = append(bodies, root)
				continue
			}
			values, ok := p.Return(root)
			if !ok {
				continue
			}
			value, ok := p.ValuesAt(values, 0)
			if !ok {
				t.Fatal("Return has no first value")
			}
			return p, value
		}
	}
	t.Fatal("Entry-reachable Bodies have no Return")
	return nil, 0
}

func TestEveryClosedUnaryOperatorLowers(t *testing.T) {
	tests := []struct {
		source string
		want   program.UnaryOp
	}{
		{`return -1`, program.UnaryNeg},
		{`return not true`, program.UnaryNot},
		{`return #"x"`, program.UnaryLen},
		{`return ~1`, program.UnaryBitNot},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			p, term := loweredReturnValue(t, test.source)
			op, operand, ok := p.Unary(term)
			if !ok || op != test.want || operand == 0 {
				t.Fatalf("Unary = op %v operand %v ok %v", op, operand, ok)
			}
			order, ok := p.AppendOrder(term, nil)
			if !ok || !reflect.DeepEqual(order, []program.Term{operand}) {
				t.Fatalf("Unary order = %v, %v", order, ok)
			}
		})
	}
}

func TestEveryClosedBinaryOperatorLowers(t *testing.T) {
	tests := []struct {
		source string
		want   program.BinaryOp
	}{
		{`return 1 + 2`, program.BinaryAdd},
		{`return 1 - 2`, program.BinarySub},
		{`return 1 * 2`, program.BinaryMul},
		{`return 1 / 2`, program.BinaryDiv},
		{`return 1 // 2`, program.BinaryIDiv},
		{`return 1 % 2`, program.BinaryMod},
		{`return 1 ^ 2`, program.BinaryPow},
		{`return "a" .. "b"`, program.BinaryConcat},
		{`return 1 & 2`, program.BinaryBitAnd},
		{`return 1 | 2`, program.BinaryBitOr},
		{`return 1 ~ 2`, program.BinaryBitXor},
		{`return 1 << 2`, program.BinaryShiftLeft},
		{`return 1 >> 2`, program.BinaryShiftRight},
		{`return 1 == 2`, program.BinaryEqual},
		{`return 1 ~= 2`, program.BinaryNotEqual},
		{`return 1 < 2`, program.BinaryLess},
		{`return 1 <= 2`, program.BinaryLessEqual},
		{`return 1 > 2`, program.BinaryGreater},
		{`return 1 >= 2`, program.BinaryGreaterEqual},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			p, term := loweredReturnValue(t, test.source)
			op, left, right, ok := p.Binary(term)
			if !ok || op != test.want || left == 0 || right == 0 {
				t.Fatalf("Binary = op %v left %v right %v ok %v", op, left, right, ok)
			}
			order, ok := p.AppendOrder(term, nil)
			if !ok || !reflect.DeepEqual(order, []program.Term{left, right}) {
				t.Fatalf("Binary order = %v, %v", order, ok)
			}
			span, ok := p.Span(term)
			if !ok || span.File != "fixture.lua" || span.StartLine != 1 ||
				span.StartCol != 8 || span.EndLine != 1 || span.EndCol == 0 {
				t.Fatalf("Binary span = %#v, %v", span, ok)
			}
		})
	}
}

func TestLogicalSelectionKeepsRightSideConditional(t *testing.T) {
	for _, test := range []struct {
		source string
		want   program.SelectOp
	}{
		{`return false and (1 + 2)`, program.SelectAnd},
		{`return true or (1 + 2)`, program.SelectOr},
	} {
		t.Run(test.source, func(t *testing.T) {
			p, term := loweredReturnValue(t, test.source)
			op, left, right, ok := p.Select(term)
			if !ok || op != test.want || left == 0 || right == 0 {
				t.Fatalf("Select = op %v left %v right %v ok %v", op, left, right, ok)
			}
			if _, _, _, ok := p.Binary(right); !ok {
				t.Fatalf("conditional right operand is not retained Binary: %v", right)
			}
			order, ok := p.AppendOrder(term, nil)
			if !ok || !reflect.DeepEqual(order, []program.Term{left}) {
				t.Fatalf("Select eager order = %v, %v; want only left %v", order, ok, left)
			}
		})
	}
}

func TestAttributeReadsPreserveExactAndDynamicLenses(t *testing.T) {
	p := parseBindLower(t, "local t = {}\nlocal k = \"name\"\nreturn t.name, t[nil], t[k]")
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	roots, ok := p.AppendBody(entry, nil)
	if !ok || len(roots) != 3 {
		t.Fatalf("Entry roots = %v, %v", roots, ok)
	}
	returned, ok := p.Return(roots[2])
	if !ok {
		t.Fatalf("Entry root 2 is not Return: %v", roots[2])
	}
	for i, wantDynamic := range []bool{false, false, true} {
		read, ok := p.ValuesAt(returned, i)
		if !ok {
			t.Fatalf("return ValuesAt(%d) failed", i)
		}
		lens, ok := p.Read(read)
		if !ok {
			t.Fatalf("return value %d is not Read", i)
		}
		base, key, dynamic, ok := p.Lens(lens)
		if !ok || base == 0 || key == 0 || dynamic != wantDynamic {
			t.Fatalf("Lens %d = base %v key %v dynamic %v ok %v", i, base, key, dynamic, ok)
		}
		if i == 0 {
			if value, ok := p.String(key); !ok || value != "name" {
				t.Fatalf("dot key = %q, %v", value, ok)
			}
		}
		if i == 1 && !p.Nil(key) {
			t.Fatalf("nil index key = %v", key)
		}
		order, ok := p.AppendOrder(read, nil)
		if !ok || !reflect.DeepEqual(order, []program.Term{lens}) {
			t.Fatalf("Read %d order = %v, %v", i, order, ok)
		}
		span, ok := p.Span(read)
		if !ok || span.StartLine != 3 || span.StartCol == 0 || span.EndCol == 0 {
			t.Fatalf("Read %d span = %#v, %v", i, span, ok)
		}
	}
}

func TestInvalidOperatorAndUnknownAttributeSyntaxFail(t *testing.T) {
	badOp := &ast.ArithmeticOpExpr{
		Operator: "??",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}
	stmts := []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{badOp}}}
	_, err := programlower.Lower("manual.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), `unsupported arithmetic operator "??"`) {
		t.Fatalf("invalid operator error = %v", err)
	}

	attr := &ast.AttrGetExpr{
		Object:    &ast.StringExpr{Value: "base"},
		Key:       &ast.StringExpr{Value: "key"},
		KeySyntax: ast.AttrKeyUnknown,
	}
	stmts = []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{attr}}}
	_, err = programlower.Lower("manual.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unknown key syntax") {
		t.Fatalf("unknown attribute syntax error = %v", err)
	}
}

func TestUnsupportedSyntaxIsHonest(t *testing.T) {
	stmts, err := parse.ParseString(`return 1 :: number`, "unsupported.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("unsupported.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil {
		t.Fatal("cast expression lowered without a typed Program relation")
	}
	if !strings.Contains(err.Error(), "unsupported expression *ast.CastExpr") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := programlower.Lower("unsupported.lua", stmts, nil); err == nil {
		t.Fatal("nil binding result was accepted")
	}
}

func entryRoots(t *testing.T, p *program.Program) []program.Term {
	t.Helper()
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	roots, ok := p.AppendBody(entry, nil)
	if !ok {
		t.Fatalf("Entry %v is not a Body", entry)
	}
	return roots
}

func TestAnonymousFunctionFormalsVarargAndOpenValues(t *testing.T) {
	p, function := loweredReturnValue(t, `return function(a, ...) return a, ... end`)
	entry, _ := p.Entry()
	owner, body, bindingCell, varargCell, ok := p.Function(function)
	if !ok || owner != entry || body == 0 || bindingCell != 0 || varargCell == 0 {
		t.Fatalf("Function = owner %v body %v binding %v vararg %v ok %v", owner, body, bindingCell, varargCell, ok)
	}
	if count, ok := p.FormalLen(function); !ok || count != 1 {
		t.Fatalf("FormalLen = %d, %v", count, ok)
	}
	formal, ok := p.FormalAt(function, 0)
	if !ok {
		t.Fatal("missing formal Cell")
	}
	if formalOwner, ok := p.Cell(formal); !ok || formalOwner != body {
		t.Fatalf("formal owner = %v, %v; want %v", formalOwner, ok, body)
	}
	if varargOwner, ok := p.Cell(varargCell); !ok || varargOwner != body {
		t.Fatalf("vararg owner = %v, %v; want %v", varargOwner, ok, body)
	}
	if count, ok := p.FunctionCaptureLen(function); !ok || count != 0 {
		t.Fatalf("FunctionCaptureLen = %d, %v", count, ok)
	}
	bodyRoots, ok := p.AppendBody(body, nil)
	if !ok || len(bodyRoots) != 1 {
		t.Fatalf("Function Body roots = %v, %v", bodyRoots, ok)
	}
	returned, ok := p.Return(bodyRoots[0])
	if !ok {
		t.Fatalf("Function root is not Return: %v", bodyRoots[0])
	}
	if count, ok := p.ValuesLen(returned); !ok || count != 1 {
		t.Fatalf("function return fixed prefix = %d, %v", count, ok)
	}
	tail, ok := p.ValuesTail(returned)
	if !ok || tail == 0 {
		t.Fatalf("function return tail = %v, %v", tail, ok)
	}
	if cell, ok := p.Vararg(tail); !ok || cell != varargCell {
		t.Fatalf("vararg occurrence = Cell %v, %v; want %v", cell, ok, varargCell)
	}
}

func TestDirectCaptureFinalizationAndLexicalIsolation(t *testing.T) {
	p := parseBindLower(t, `local x = 1
return function(a) return x, a end, function(a) return x, a end`)
	roots := entryRoots(t, p)
	if len(roots) != 2 {
		t.Fatalf("Entry roots = %v", roots)
	}
	boundOuter, ok := p.BindAt(roots[0], 0)
	if !ok {
		t.Fatal("missing outer x Cell")
	}
	returned, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("Entry root 1 is not Return")
	}
	functions := make([]program.Term, 2)
	for i := range functions {
		functions[i], ok = p.ValuesAt(returned, i)
		if !ok {
			t.Fatalf("missing returned Function %d", i)
		}
	}
	var priorFormal, priorInner program.Term
	for i, function := range functions {
		_, body, _, _, ok := p.Function(function)
		if !ok {
			t.Fatalf("value %d is not Function", i)
		}
		formal, ok := p.FormalAt(function, 0)
		if !ok {
			t.Fatalf("Function %d has no formal", i)
		}
		if count, ok := p.FunctionCaptureLen(function); !ok || count != 1 {
			t.Fatalf("Function %d capture count = %d, %v", i, count, ok)
		}
		capture, ok := p.FunctionCaptureAt(function, 0)
		if !ok {
			t.Fatalf("Function %d capture missing", i)
		}
		captureFunction, inner, outer, ok := p.Capture(capture)
		if !ok || captureFunction != function || outer != boundOuter {
			t.Fatalf("Capture %d = function %v inner %v outer %v ok %v", i, captureFunction, inner, outer, ok)
		}
		if innerOwner, ok := p.Cell(inner); !ok || innerOwner != body {
			t.Fatalf("Capture %d inner owner = %v, %v; want %v", i, innerOwner, ok, body)
		}
		bodyRoots, _ := p.AppendBody(body, nil)
		functionValues, ok := p.Return(bodyRoots[0])
		if !ok {
			t.Fatalf("Function %d Body root is not Return", i)
		}
		capturedRead, _ := p.ValuesAt(functionValues, 0)
		if source, ok := p.Read(capturedRead); !ok || source != inner {
			t.Fatalf("Function %d captured Read source = %v, %v; want inner %v", i, source, ok, inner)
		}
		if i != 0 && (formal == priorFormal || inner == priorInner) {
			t.Fatalf("sibling Functions share lexical Cells: formal %v/%v inner %v/%v", formal, priorFormal, inner, priorInner)
		}
		priorFormal, priorInner = formal, inner
	}
}

func TestNestedFunctionCapturesCurrentLexicalAliases(t *testing.T) {
	p := parseBindLower(t, `local x = 1
return function(a)
  return function(b) return x, a, b end
end`)
	roots := entryRoots(t, p)
	chunkX, ok := p.BindAt(roots[0], 0)
	if !ok {
		t.Fatal("chunk x Cell missing")
	}
	outerValues, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("outer Return missing")
	}
	outerFunction, _ := p.ValuesAt(outerValues, 0)
	_, outerBody, _, _, ok := p.Function(outerFunction)
	if !ok {
		t.Fatal("outer value is not Function")
	}
	outerFormal, _ := p.FormalAt(outerFunction, 0)
	if count, ok := p.FunctionCaptureLen(outerFunction); !ok || count != 1 {
		t.Fatalf("outer capture count = %d, %v; want propagated chunk x", count, ok)
	}
	outerCapture, _ := p.FunctionCaptureAt(outerFunction, 0)
	_, outerX, outerSource, ok := p.Capture(outerCapture)
	if !ok || outerSource != chunkX {
		t.Fatalf("outer x Capture = inner %v outer %v ok %v; want chunk Cell %v", outerX, outerSource, ok, chunkX)
	}
	outerBodyRoots, _ := p.AppendBody(outerBody, nil)
	innerValues, ok := p.Return(outerBodyRoots[0])
	if !ok {
		t.Fatal("outer Function does not return inner Function")
	}
	innerFunction, _ := p.ValuesAt(innerValues, 0)
	if count, ok := p.FunctionCaptureLen(innerFunction); !ok || count != 2 {
		t.Fatalf("inner capture count = %d, %v", count, ok)
	}
	seenOuter := map[program.Term]bool{}
	for i := 0; i < 2; i++ {
		capture, _ := p.FunctionCaptureAt(innerFunction, i)
		_, _, outer, ok := p.Capture(capture)
		if !ok {
			t.Fatalf("inner Capture %d invalid", i)
		}
		seenOuter[outer] = true
	}
	if !seenOuter[outerX] || !seenOuter[outerFormal] || seenOuter[chunkX] {
		t.Fatalf("inner captures = %v; want parent aliases %v and %v, not chunk Cell %v",
			seenOuter, outerX, outerFormal, chunkX)
	}
}

func TestPlainStatementImmediateAndMethodCalls(t *testing.T) {
	t.Run("plain and Values adjustment", func(t *testing.T) {
		p := parseBindLower(t, `local f
f(1, 2)
return f(3), 4, f(5)`)
		roots := entryRoots(t, p)
		if len(roots) != 3 {
			t.Fatalf("Entry roots = %v", roots)
		}
		callee, receiver, actuals, direct, ok := p.Call(roots[1])
		if !ok || callee == 0 || receiver != 0 || actuals == 0 || direct != 0 {
			t.Fatalf("statement Call = callee %v receiver %v actuals %v direct %v ok %v", callee, receiver, actuals, direct, ok)
		}
		if count, ok := p.ValuesLen(actuals); !ok || count != 2 {
			t.Fatalf("statement actuals = %d, %v", count, ok)
		}
		returned, _ := p.Return(roots[2])
		if count, ok := p.ValuesLen(returned); !ok || count != 2 {
			t.Fatalf("return fixed prefix = %d, %v", count, ok)
		}
		tail, ok := p.ValuesTail(returned)
		if !ok || tail == 0 {
			t.Fatalf("return open tail = %v, %v", tail, ok)
		}
		if _, _, _, _, ok := p.Call(tail); !ok {
			t.Fatalf("return tail is not Call: %v", tail)
		}
	})

	t.Run("call arguments and table list tail stay open", func(t *testing.T) {
		p := parseBindLower(t, `local f
return f(1, f(2)), {f(3), f(4)}`)
		roots := entryRoots(t, p)
		returned, _ := p.Return(roots[1])
		outerCall, _ := p.ValuesAt(returned, 0)
		_, _, actuals, _, ok := p.Call(outerCall)
		if !ok {
			t.Fatalf("first return value is not Call: %v", outerCall)
		}
		if count, ok := p.ValuesLen(actuals); !ok || count != 1 {
			t.Fatalf("outer actual fixed prefix = %d, %v", count, ok)
		}
		if tail, ok := p.ValuesTail(actuals); !ok || tail == 0 {
			t.Fatalf("outer actual tail = %v, %v", tail, ok)
		}
		table, _ := p.ValuesAt(returned, 1)
		if count, ok := p.TableLen(table); !ok || count != 2 {
			t.Fatalf("TableLen = %d, %v", count, ok)
		}
		_, firstValues, firstKind, _ := p.TableAt(table, 0)
		_, lastValues, lastKind, _ := p.TableAt(table, 1)
		if firstKind != program.FieldList || lastKind != program.FieldList {
			t.Fatalf("table field kinds = %v, %v", firstKind, lastKind)
		}
		if count, ok := p.ValuesLen(firstValues); !ok || count != 1 {
			t.Fatalf("non-final list field fixed prefix = %d, %v", count, ok)
		}
		if tail, ok := p.ValuesTail(firstValues); !ok || tail != 0 {
			t.Fatalf("non-final list field tail = %v, %v", tail, ok)
		}
		if count, ok := p.ValuesLen(lastValues); !ok || count != 0 {
			t.Fatalf("final list field fixed prefix = %d, %v", count, ok)
		}
		if tail, ok := p.ValuesTail(lastValues); !ok || tail == 0 {
			t.Fatalf("final list field tail = %v, %v", tail, ok)
		}
	})

	t.Run("parenthesized call adjusts to scalar", func(t *testing.T) {
		p := parseBindLower(t, `local f
return (f())`)
		roots := entryRoots(t, p)
		returned, _ := p.Return(roots[1])
		if count, ok := p.ValuesLen(returned); !ok || count != 1 {
			t.Fatalf("adjusted return fixed prefix = %d, %v", count, ok)
		}
		if tail, ok := p.ValuesTail(returned); !ok || tail != 0 {
			t.Fatalf("adjusted return tail = %v, %v", tail, ok)
		}
	})

	t.Run("immediate", func(t *testing.T) {
		p := parseBindLower(t, `return (function(a) return a end)(1)`)
		roots := entryRoots(t, p)
		returned, _ := p.Return(roots[0])
		call, _ := p.ValuesTail(returned)
		callee, receiver, actuals, direct, ok := p.Call(call)
		if !ok || receiver != 0 || direct != callee {
			t.Fatalf("immediate Call = callee %v receiver %v direct %v ok %v", callee, receiver, direct, ok)
		}
		if _, _, _, _, ok := p.Function(callee); !ok {
			t.Fatalf("immediate callee is not Function: %v", callee)
		}
		if count, ok := p.ValuesLen(actuals); !ok || count != 1 {
			t.Fatalf("immediate actuals = %d, %v", count, ok)
		}
	})

	t.Run("method needs exact upstream token evidence", func(t *testing.T) {
		stmts, err := parse.ParseString(`local t = {}
return t:m(1)`, "method.lua")
		if err != nil {
			t.Fatal(err)
		}
		_, err = programlower.Lower("method.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
		if err == nil || !strings.Contains(err.Error(), "AST has no MethodPosition evidence") {
			t.Fatalf("method call error = %v", err)
		}
	})
}

func TestFunctionAndCallEvidenceLossFailsClosed(t *testing.T) {
	for _, source := range []string{
		`local function f() end`,
		`local f = function() end`,
	} {
		stmts, err := parse.ParseString(source, "ambiguous.lua")
		if err != nil {
			t.Fatal(err)
		}
		_, err = programlower.Lower("ambiguous.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
		if err == nil || !strings.Contains(err.Error(), "recursive-local syntax was erased") {
			t.Fatalf("%q error = %v", source, err)
		}
	}
	for _, source := range []string{
		`function f() end`,
		`function t.f() end`,
		`function t:f() end`,
	} {
		stmts, err := parse.ParseString(source, "definition.lua")
		if err != nil {
			t.Fatal(err)
		}
		_, err = programlower.Lower("definition.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
		if err == nil || !strings.Contains(err.Error(), "global or qualified function definition") {
			t.Fatalf("%q error = %v", source, err)
		}
	}
	stmts, err := parse.ParseString(`return function(a: number) return a end`, "typed.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("typed.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "typed function parameter") {
		t.Fatalf("typed Function error = %v", err)
	}

	typedCall := &ast.FuncCallExpr{
		Func:     &ast.NilExpr{},
		TypeArgs: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
	}
	stmts = []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{typedCall}}}
	_, err = programlower.Lower("typed-call.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unsupported typed call") {
		t.Fatalf("typed Call error = %v", err)
	}
}

func wideClosureSource(bindings, siblings int) string {
	var source strings.Builder
	if bindings > 0 {
		source.WriteString("local ")
		for i := 0; i < bindings; i++ {
			if i != 0 {
				source.WriteByte(',')
			}
			source.WriteByte('x')
			source.WriteString(strconv.Itoa(i))
		}
		source.WriteString(" = ")
		for i := 0; i < bindings; i++ {
			if i != 0 {
				source.WriteByte(',')
			}
			source.WriteString(strconv.Itoa(i))
		}
		source.WriteByte('\n')
	}
	source.WriteString("return ")
	if siblings == 0 {
		source.WriteByte('0')
		return source.String()
	}
	for i := 0; i < siblings; i++ {
		if i != 0 {
			source.WriteByte(',')
		}
		source.WriteString("function() return x0 end")
	}
	return source.String()
}

func TestWideSiblingCaptureAliasesStayIsolated(t *testing.T) {
	const siblings = 96
	p := parseBindLower(t, wideClosureSource(32, siblings))
	roots := entryRoots(t, p)
	outer, ok := p.BindAt(roots[0], 0)
	if !ok {
		t.Fatal("outer declaration Cell missing")
	}
	returned, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("wide Return missing")
	}
	seenInner := make(map[program.Term]struct{}, siblings)
	for i := 0; i < siblings; i++ {
		function, ok := p.ValuesAt(returned, i)
		if !ok {
			t.Fatalf("Function %d missing", i)
		}
		if count, ok := p.FunctionCaptureLen(function); !ok || count != 1 {
			t.Fatalf("Function %d capture count = %d, %v", i, count, ok)
		}
		capture, _ := p.FunctionCaptureAt(function, 0)
		_, inner, gotOuter, ok := p.Capture(capture)
		if !ok || gotOuter != outer {
			t.Fatalf("Function %d Capture = inner %v outer %v ok %v; want outer %v", i, inner, gotOuter, ok, outer)
		}
		if _, duplicate := seenInner[inner]; duplicate {
			t.Fatalf("Function %d reused capture-inner Cell %v", i, inner)
		}
		seenInner[inner] = struct{}{}
	}
}

func TestGrandchildOnlyCaptureCrossesEveryFunctionBoundary(t *testing.T) {
	p := parseBindLower(t, `local x = 1
return function()
  return function() return x end
end`)
	roots := entryRoots(t, p)
	chunkX, _ := p.BindAt(roots[0], 0)
	outerValues, _ := p.Return(roots[1])
	outerFunction, _ := p.ValuesAt(outerValues, 0)
	if count, ok := p.FunctionCaptureLen(outerFunction); !ok || count != 1 {
		t.Fatalf("outer Function captures = %d, %v; want propagated x", count, ok)
	}
	outerCapture, _ := p.FunctionCaptureAt(outerFunction, 0)
	_, outerInner, gotOuter, ok := p.Capture(outerCapture)
	if !ok || gotOuter != chunkX {
		t.Fatalf("outer Capture = inner %v outer %v ok %v; want chunk declaration %v", outerInner, gotOuter, ok, chunkX)
	}
	_, outerBody, _, _, _ := p.Function(outerFunction)
	outerRoots, _ := p.AppendBody(outerBody, nil)
	innerValues, _ := p.Return(outerRoots[0])
	innerFunction, _ := p.ValuesAt(innerValues, 0)
	capture, ok := p.FunctionCaptureAt(innerFunction, 0)
	if !ok {
		t.Fatal("inner Function capture missing")
	}
	_, _, gotOuter, ok = p.Capture(capture)
	if !ok || gotOuter != outerInner || gotOuter == chunkX {
		t.Fatalf("grandchild Capture outer = %v, %v; want parent capture Cell %v", gotOuter, ok, outerInner)
	}
}

func TestEntryCaptureOrderAndDedupAcrossDescendants(t *testing.T) {
	p := parseBindLower(t, `local x, y, z = 1, 2, 3
return function()
  return x, function() return y, x end, function() return z, y end
end`)
	roots := entryRoots(t, p)
	chunk := make([]program.Term, 3)
	for i := range chunk {
		var ok bool
		chunk[i], ok = p.BindAt(roots[0], i)
		if !ok {
			t.Fatalf("chunk Cell %d missing", i)
		}
	}
	values, _ := p.Return(roots[1])
	outerFunction, _ := p.ValuesAt(values, 0)
	if count, ok := p.FunctionCaptureLen(outerFunction); !ok || count != 3 {
		t.Fatalf("outer capture count = %d, %v; want x,y,z once", count, ok)
	}
	outerInner := make([]program.Term, 3)
	for i := range outerInner {
		capture, _ := p.FunctionCaptureAt(outerFunction, i)
		_, inner, outer, ok := p.Capture(capture)
		if !ok || outer != chunk[i] {
			t.Fatalf("outer Capture %d = inner %v outer %v ok %v; want chunk Cell %v",
				i, inner, outer, ok, chunk[i])
		}
		outerInner[i] = inner
	}

	_, body, _, _, _ := p.Function(outerFunction)
	bodyRoots, _ := p.AppendBody(body, nil)
	returned, _ := p.Return(bodyRoots[0])
	for childIndex, want := range [][]program.Term{
		{outerInner[1], outerInner[0]},
		{outerInner[2], outerInner[1]},
	} {
		child, _ := p.ValuesAt(returned, childIndex+1)
		if count, ok := p.FunctionCaptureLen(child); !ok || count != len(want) {
			t.Fatalf("child %d capture count = %d, %v; want %d", childIndex, count, ok, len(want))
		}
		for i, wantOuter := range want {
			capture, _ := p.FunctionCaptureAt(child, i)
			_, _, outer, ok := p.Capture(capture)
			if !ok || outer != wantOuter {
				t.Fatalf("child %d Capture %d outer = %v, %v; want parent Cell %v",
					childIndex, i, outer, ok, wantOuter)
			}
		}
	}
}

func deepClosureSource(depth int) string {
	var source strings.Builder
	source.WriteString("local x = 1\nreturn ")
	for i := 0; i < depth; i++ {
		source.WriteString("function() return ")
	}
	source.WriteByte('x')
	for i := 0; i < depth; i++ {
		source.WriteString(" end")
	}
	return source.String()
}

func TestDeepDescendantCaptureLoweringScalesWithBoundaryEdges(t *testing.T) {
	type measurement struct {
		terms  int
		allocs float64
	}
	measure := func(depth int) measurement {
		t.Helper()
		stmts, err := parse.ParseString(deepClosureSource(depth), "deep.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower("deep.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		roots := entryRoots(t, p)
		declaration, ok := p.BindAt(roots[0], 0)
		if !ok {
			t.Fatalf("depth %d chunk declaration missing", depth)
		}
		values, ok := p.Return(roots[1])
		if !ok {
			t.Fatalf("depth %d Entry return missing", depth)
		}
		function, ok := p.ValuesAt(values, 0)
		if !ok {
			t.Fatalf("depth %d outer Function missing", depth)
		}
		wantOuter := declaration
		for level := 0; level < depth; level++ {
			_, body, _, _, ok := p.Function(function)
			if !ok {
				t.Fatalf("depth %d level %d is not a Function", depth, level)
			}
			if count, ok := p.FunctionCaptureLen(function); !ok || count != 1 {
				t.Fatalf("depth %d level %d capture count = %d, %v; want one boundary edge",
					depth, level, count, ok)
			}
			capture, _ := p.FunctionCaptureAt(function, 0)
			_, inner, outer, ok := p.Capture(capture)
			if !ok || outer != wantOuter {
				t.Fatalf("depth %d level %d Capture = inner %v outer %v ok %v; want outer %v",
					depth, level, inner, outer, ok, wantOuter)
			}
			bodyRoots, ok := p.AppendBody(body, nil)
			if !ok || len(bodyRoots) != 1 {
				t.Fatalf("depth %d level %d Body roots = %v, %v", depth, level, bodyRoots, ok)
			}
			returned, ok := p.Return(bodyRoots[0])
			if !ok {
				t.Fatalf("depth %d level %d Body return missing", depth, level)
			}
			value, ok := p.ValuesAt(returned, 0)
			if !ok {
				t.Fatalf("depth %d level %d return value missing", depth, level)
			}
			if level == depth-1 {
				if read, ok := p.Read(value); !ok || read != inner {
					t.Fatalf("depth %d terminal Read = %v, %v; want terminal capture Cell %v",
						depth, read, ok, inner)
				}
				continue
			}
			function = value
			wantOuter = inner
		}
		allocs := testing.AllocsPerRun(5, func() {
			loweredSink, err = programlower.Lower("deep.lua", stmts, binding)
			if err != nil {
				t.Fatal(err)
			}
		})
		return measurement{terms: p.TermCount(), allocs: allocs}
	}

	small := measure(40)
	large := measure(80)
	if large.terms > small.terms*3 {
		t.Fatalf("deep term growth is not linear: depth40=%d depth80=%d", small.terms, large.terms)
	}
	if large.allocs > small.allocs*3 {
		t.Fatalf("deep allocation growth is not linear: depth40=%.0f depth80=%.0f", small.allocs, large.allocs)
	}
}

func TestFunctionLoweringAllocationGrowthHasNoBindingTimesClosureTerm(t *testing.T) {
	type measurement struct {
		bytes int64
		terms int
	}
	measure := func(bindings, siblings int) measurement {
		t.Helper()
		source := wideClosureSource(bindings, siblings)
		stmts, err := parse.ParseString(source, "growth.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower("growth.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				loweredSink, err = programlower.Lower("growth.lua", stmts, binding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		return measurement{bytes: result.AllocedBytesPerOp(), terms: p.TermCount()}
	}

	bindingsOnly := measure(384, 0)
	closuresOnly := measure(1, 48)
	combined := measure(384, 48)
	if combined.terms > bindingsOnly.terms+closuresOnly.terms+16 {
		t.Fatalf("Term growth is non-additive: bindings=%d closures=%d combined=%d",
			bindingsOnly.terms, closuresOnly.terms, combined.terms)
	}
	additiveBytes := bindingsOnly.bytes + closuresOnly.bytes
	if combined.bytes > additiveBytes*2 {
		t.Fatalf("allocation bytes expose binding×closure growth: bindings=%d closures=%d combined=%d",
			bindingsOnly.bytes, closuresOnly.bytes, combined.bytes)
	}
}
