package programlower_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/programlower"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
)

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

func TestParseBindLowerLiteralLocalsTableAssignmentAndReturn(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
local t = {name = x, [2] = "two", [x] = false}
x = 3
return x, t
`)

	var bodies, cells, reads, binds, assigns, tables, lenses, returns int
	var rootBody, tableTerm, returnValues program.Term
	var cellTerms, bindTerms []program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, ok := p.TermAt(i)
		if !ok {
			t.Fatalf("TermAt(%d) failed", i)
		}
		if _, ok := p.BodyLen(term); ok {
			bodies++
			rootBody = term
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
	var bodies, cells []program.Term
	var returnValues program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		if _, ok := p.BodyLen(term); ok {
			bodies = append(bodies, term)
		}
		if _, ok := p.Cell(term); ok {
			cells = append(cells, term)
		}
		if values, ok := p.Return(term); ok {
			returnValues = values
		}
	}
	if len(cells) != 2 {
		t.Fatalf("cells = %d", len(cells))
	}
	if len(bodies) != 2 {
		t.Fatalf("bodies = %d", len(bodies))
	}
	outerRoots, ok := p.AppendBody(bodies[0], nil)
	if !ok || len(outerRoots) != 2 || outerRoots[1] != bodies[1] {
		t.Fatalf("outer Body = %v, %v; want Bind then child Body", outerRoots, ok)
	}
	innerRoots, ok := p.AppendBody(bodies[1], nil)
	if !ok || len(innerRoots) != 2 {
		t.Fatalf("inner Body = %v, %v; want Bind then Return", innerRoots, ok)
	}
	if owner, ok := p.Cell(cells[0]); !ok || owner != bodies[0] {
		t.Fatalf("outer Cell owner = %v, %v; want %v", owner, ok, bodies[0])
	}
	if owner, ok := p.Cell(cells[1]); !ok || owner != bodies[1] {
		t.Fatalf("inner Cell owner = %v, %v; want %v", owner, ok, bodies[1])
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
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		values, ok := p.Return(term)
		if !ok {
			continue
		}
		value, ok := p.ValuesAt(values, 0)
		if !ok {
			t.Fatal("Return has no first value")
		}
		return p, value
	}
	t.Fatal("Program has no Return")
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
	var returned program.Term
	for i := 0; i < p.TermCount(); i++ {
		term, _ := p.TermAt(i)
		if values, ok := p.Return(term); ok {
			returned = values
			break
		}
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
	stmts, err := parse.ParseString(`return function() end`, "unsupported.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("unsupported.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil {
		t.Fatal("function expression lowered without a typed Program relation")
	}
	if !strings.Contains(err.Error(), "unsupported expression *ast.FunctionExpr") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := programlower.Lower("unsupported.lua", stmts, nil); err == nil {
		t.Fatal("nil binding result was accepted")
	}
}
