package lower_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// valuesSourceCases is the values vertical's complete atomic source witness
// set. Each case is consumed directly by the exact source-to-Program law.
var valuesSourceCases = []sourceCase{
	{ID: "values.case.nil", Form: "NilExpr", Source: "local x = nil", Line: 1},
	{ID: "values.case.false", Form: "FalseExpr", Source: "local x = false", Line: 1},
	{ID: "values.case.true", Form: "TrueExpr", Source: "local x = true", Line: 1},
	{ID: "values.case.number.integer", Form: "NumberExpr", Source: "local x = 1", Line: 1},
	{ID: "values.case.number.float", Form: "NumberExpr", Source: "local x = 1.5", Line: 1},
	{ID: "values.case.string", Form: "StringExpr", Source: "local x = 's'", Line: 1},
	{ID: "values.case.vararg.open", Form: "Comma3Expr", Source: "local function f(...)\n  return ...\nend", Line: 2},
	{ID: "values.case.vararg.scalar", Form: "Comma3Expr", Source: "local function f(...)\n  return (...)\nend", Line: 2},
	{ID: "values.case.identifier.read", Form: "IdentExpr", Source: "local x = 1\nlocal y = x", Line: 2},
	{ID: "values.case.attr.dot", Form: "AttrGetExpr", Source: "local t = { x = 1 }\nlocal y = t.x", Line: 2},
	{ID: "values.case.attr.index-exact", Form: "AttrGetExpr", Source: "local t = {}\nlocal y = t[1]", Line: 2},
	{ID: "values.case.attr.index-dynamic", Form: "AttrGetExpr", Source: "local t = {}\nlocal k = 1\nlocal y = t[k]", Line: 3},
	{ID: "values.case.assignment", Form: "AssignStmt", Source: "local t = {}\nlocal first = 1\nlocal second = 2\nt[first], t[second] = 10, 20", Line: 4},
	{ID: "values.case.values.return-list", Form: "ReturnStmt", Source: "return 1, 2", Line: 1},
	{ID: "values.case.table", Form: "TableExpr", Source: "local t = {}", Line: 1},
	{ID: "values.case.table-field.name", Form: "Field", Source: "local t = {\n  x = 1,\n}", Line: 2},
	{ID: "values.case.table-field.exact", Form: "Field", Source: "local t = {\n  [1] = 2,\n}", Line: 2},
	{ID: "values.case.table-field.dynamic", Form: "Field", Source: "local k = 1\nlocal t = {\n  [k] = 2,\n}", Line: 3},
	{ID: "values.case.table-field.list-scalar-final", Form: "Field", Source: "local t = {\n  1,\n}", Line: 2},
	{ID: "values.case.table-field.list", Form: "Field", Source: "local function f(...)\n  local t = {\n    ...,\n  }\nend", Line: 3},
	{ID: "values.case.table-field.list-prefix", Form: "Field", Source: "local function f(...)\n  local t = {\n    ...,\n    1,\n  }\nend", Line: 3},
}

// TestValuesSourceCasesHaveExactProgramWitnesses is the source-to-Program
// witness for every atomic values case.  Each arm starts with the parsed AST
// occurrence, derives its expected semantic discriminant from that occurrence,
// and then follows only typed Program relations from the matching source span.
func TestValuesSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range valuesSourceCases {
		sourceCase := sourceCase
		t.Run(sourceCase.ID, func(t *testing.T) {
			statements, err := parse.ParseString(sourceCase.Source, "values-witness.lua")
			if err != nil {
				t.Fatalf("parse %s: %v", sourceCase.ID, err)
			}
			target := valuesASTTarget(t, statements, sourceCase.Form, sourceCase.Line)
			binding := bind.BindChunk(statements)
			if binding == nil {
				t.Fatal("binder returned nil result")
			}
			p := parseBindLower(t, sourceCase.Source)
			assertValuesCase(t, p, binding, statements, sourceCase, target)
		})
	}
}

func assertValuesCase(t *testing.T, p *program.Program, binding *bind.Result, statements []ast.Stmt, sourceCase sourceCase, target ast.PositionHolder) {
	t.Helper()
	switch sourceCase.ID {
	case "values.case.nil":
		if _, ok := target.(*ast.NilExpr); !ok {
			t.Fatalf("parsed target = %T, want *ast.NilExpr", target)
		}
		term := valuesNilAt(t, p, target)
		if literal, owner, ok := p.Source().Literals().Nils().At(int(keyspace.TermOrdinal(term)) - 1); !ok || literal != term || owner == 0 {
			t.Fatalf("exact Nil owner = %v/%v", owner, ok)
		}
	case "values.case.false", "values.case.true":
		_, ok := target.(*ast.FalseExpr)
		want := false
		if !ok {
			if _, trueOK := target.(*ast.TrueExpr); !trueOK {
				t.Fatalf("parsed target = %T, want boolean literal", target)
			}
			want = true
		}
		term := valuesBoolAt(t, p, target)
		literal, owner, got, boolOK := p.Source().Literals().Bools().At(int(keyspace.TermOrdinal(term)) - 1)
		if !boolOK || literal != term || owner == 0 || got != want {
			t.Fatalf("Bool = owner %v payload %v/%v, want owned %v", owner, got, boolOK, want)
		}
	case "values.case.number.integer", "values.case.number.float":
		expr, ok := target.(*ast.NumberExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.NumberExpr", target)
		}
		integer, integerOK := numparse.ParseIntegerLiteral(expr.Value)
		if integerOK {
			term := valuesIntegerAt(t, p, target)
			literal, owner, got, ok := p.Source().Literals().Integers().At(int(keyspace.TermOrdinal(term)) - 1)
			if !ok || literal != term || owner == 0 || got != integer {
				t.Fatalf("Integer = owner %v payload %d/%v, want owned %d", owner, got, ok, integer)
			}
			return
		}
		_, floating, parsed := numparse.ParseNumberLiteral(expr.Value)
		if !parsed {
			t.Fatalf("parser accepted unparseable numeric spelling %q", expr.Value)
		}
		term := valuesFloatAt(t, p, target)
		literal, owner, gotBits, ok := p.Source().Literals().Floats().At(int(keyspace.TermOrdinal(term)) - 1)
		got := math.Float64frombits(gotBits)
		if !ok || literal != term || owner == 0 || got != floating {
			t.Fatalf("Float = owner %v payload %g/%v, want owned %g", owner, got, ok, floating)
		}
	case "values.case.string":
		expr, ok := target.(*ast.StringExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.StringExpr", target)
		}
		term := valuesStringAt(t, p, target)
		literal, owner, got, ok := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(term)) - 1)
		if !ok || literal != term || owner == 0 || got != expr.Value {
			t.Fatalf("String = owner %v payload %q/%v, want owned %q", owner, got, ok, expr.Value)
		}
	case "values.case.vararg.open", "values.case.vararg.scalar":
		expr, ok := target.(*ast.Comma3Expr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.Comma3Expr", target)
		}
		vararg := valuesVarargAt(t, p, target)
		if _, cell, ok := p.Flow().Authored().Storage().Varargs().Get(vararg); !ok || cell == 0 {
			t.Fatalf("Vararg(%v) has no function vararg Cell", vararg)
		}
		if expr.AdjustRet != (sourceCase.ID == "values.case.vararg.scalar") {
			t.Fatalf("parsed AdjustRet = %v contradicts source-case %s", expr.AdjustRet, sourceCase.ID)
		}
		ret := valuesReturnAt(t, p, valuesASTTarget(t, statements, "ReturnStmt", target.Line()))
		_, values, ok := p.Flow().Authored().Control().Returns().Get(ret)
		if !ok {
			t.Fatal("vararg return lacks Values")
		}
		fixed, fixedOK := p.Flow().Authored().Values().Len(values)
		_, tail, valuesOK := p.Flow().Authored().Values().Get(values)
		if !fixedOK || !valuesOK {
			t.Fatal("vararg return Values relation is missing")
		}
		if expr.AdjustRet {
			if fixed != 1 || tail != 0 || valueAt(t, p, values, 0) != vararg {
				t.Fatalf("scalar vararg Values = fixed %d tail %v, want one fixed exact occurrence", fixed, tail)
			}
		} else if fixed != 0 || tail != vararg {
			t.Fatalf("open vararg Values = fixed %d tail %v, want exact open occurrence %v", fixed, tail, vararg)
		}
	case "values.case.identifier.read":
		expr, ok := target.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.IdentExpr", target)
		}
		if symbol, ok := binding.SymbolOf(expr); !ok || symbol == 0 {
			t.Fatal("binder did not select a declaration for the authored identifier")
		}
		if owner, source, _, ok := p.Flow().Authored().Storage().Reads().Get(valuesReadAt(t, p, target)); !ok || owner == 0 || source == 0 {
			t.Fatalf("exact identifier Read = owner %v source %v ok %v", owner, source, ok)
		}
	case "values.case.attr.dot", "values.case.attr.index-exact", "values.case.attr.index-dynamic":
		expr, ok := target.(*ast.AttrGetExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.AttrGetExpr", target)
		}
		read := valuesReadAt(t, p, target)
		readOwner, lens, _, ok := p.Flow().Authored().Storage().Reads().Get(read)
		if !ok || readOwner == 0 {
			t.Fatalf("exact attribute read %v is not Read", read)
		}
		want := fieldKindForAttr(expr)
		var lensOwner, base, keySource keyspace.Term
		var gotKind flowkind.FieldKind
		if want == flowkind.FieldKey {
			lensOwner, base, keySource, ok = p.Flow().Authored().Access().Dynamic().Get(lens)
			gotKind = flowkind.FieldKey
		} else {
			lensOwner, base, keySource, gotKind, ok = p.Flow().Authored().Access().Exact().Get(lens)
		}
		if !ok || lensOwner == 0 || base == 0 {
			t.Fatalf("attribute Read source = Lens(%v) = owner %v base %v kind %v ok %v", lens, lensOwner, base, gotKind, ok)
		}
		if gotKind != want {
			t.Fatalf("Lens kind = %v, want AST-derived %v", gotKind, want)
		}
		if want == flowkind.FieldName {
			name := ast.KeyName(expr.Key)
			if name == "" {
				t.Fatalf("dot key = %T, has no authored spelling", expr.Key)
			}
			_, got, _, nameOK := p.Source().Keys().Name(keySource)
			if !nameOK || got != name {
				t.Fatalf("dot Lens key = Name(%v) = %q/%v, want static %q", keySource, got, nameOK, name)
			}
		}
		if want != flowkind.FieldName && keySource == 0 {
			t.Fatal("bracket Lens lacks its evaluated key term")
		}
	case "values.case.assignment":
		stmt, ok := target.(*ast.AssignStmt)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.AssignStmt", target)
		}
		assign := valuesAssignAt(t, p, target)
		assigns := p.Flow().Authored().Storage().Assigns()
		if owner, _, ok := assigns.Get(assign); !ok || owner == 0 {
			t.Fatalf("Assign owner = %v/%v", owner, ok)
		}
		if fixed, ok := assigns.WriteCount(assign); !ok || fixed != len(stmt.Lhs) {
			t.Fatalf("Assign write width = %d/%v, want %d", fixed, ok, len(stmt.Lhs))
		}
		_, values, ok := assigns.Get(assign)
		fixed, fixedOK := assigns.WriteCount(assign)
		if !ok || !fixedOK || values == 0 || fixed != len(stmt.Lhs) {
			t.Fatalf("AssignValues = %v/%d/%v, want one scalarized slot per target", values, fixed, ok)
		}
		for index := range stmt.Lhs {
			write, ok := assigns.WriteAt(assign, index)
			if !ok {
				t.Fatalf("missing Write %d", index)
			}
			if parent, target, ok := p.Flow().Authored().Storage().Writes().Get(write); !ok || parent != assign || target == 0 {
				t.Fatalf("Write %d = parent %v target %v ok %v", index, parent, target, ok)
			}
		}
	case "values.case.values.return-list":
		stmt, ok := target.(*ast.ReturnStmt)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.ReturnStmt", target)
		}
		ret := valuesReturnAt(t, p, target)
		owner, values, returnOK := p.Flow().Authored().Control().Returns().Get(ret)
		if !returnOK || owner == 0 {
			t.Fatalf("Return owner = %v/%v", owner, returnOK)
		}
		if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != len(stmt.Exprs) {
			t.Fatalf("Return Values fixed width = %d/%v, want %d", fixed, ok, len(stmt.Exprs))
		}
	case "values.case.table":
		expr, ok := target.(*ast.TableExpr)
		if !ok {
			t.Fatalf("parsed target = %T, want *ast.TableExpr", target)
		}
		table := valuesTableAt(t, p, target)
		if owner, ok := p.Flow().Authored().Tables().Get(table); !ok || owner == 0 {
			t.Fatalf("exact Table allocation owner = %v/%v", owner, ok)
		}
		if len(expr.Fields) == 0 {
			if _, ok := p.Flow().Authored().Tables().FieldAt(table, 0); ok {
				t.Fatal("empty source table retained a TableField")
			}
			if finish, ok := p.Flow().Ports().Finish(table); !ok || finish == 0 {
				t.Fatal("empty Table lacks its completion frontier")
			}
		}
	case "values.case.table-field.name", "values.case.table-field.exact", "values.case.table-field.dynamic", "values.case.table-field.list-scalar-final", "values.case.table-field.list", "values.case.table-field.list-prefix":
		field, final, ok := valuesFieldTarget(target, statements)
		if !ok {
			t.Fatal("target Field did not belong to an authored table constructor")
		}
		fieldTerm := valuesTableFieldAt(t, p, target)
		parent, key, values, fieldKind, ok := p.Flow().Authored().Fields().Get(fieldTerm)
		if !ok || parent == 0 || values == 0 {
			t.Fatalf("TableField = parent %v key %v values %v kind %v ok %v", parent, key, values, fieldKind, ok)
		}
		wantKind := fieldKindForField(field)
		if fieldKind != wantKind {
			t.Fatalf("TableField kind = %v, want AST-derived %v", fieldKind, wantKind)
		}
		if wantKind == flowkind.FieldName && key == 0 {
			t.Fatal("named TableField lacks exact name key")
		}
		if wantKind != flowkind.FieldName && wantKind != flowkind.FieldList && key == 0 {
			t.Fatal("bracket TableField lacks evaluated key")
		}
		_, finalOpen, ok := p.Flow().Authored().Fields().Values(fieldTerm)
		wantOpen := final && ast.CanProduceMultipleValues(field.Value)
		if !ok || finalOpen != wantOpen {
			t.Fatalf("TableField final-open = %v/%v, want %v", finalOpen, ok, wantOpen)
		}
	default:
		t.Fatalf("values SourceCase %s has no direct semantic witness", sourceCase.ID)
	}
}

func fieldKindForAttr(expr *ast.AttrGetExpr) flowkind.FieldKind {
	if expr.KeySyntax == ast.AttrKeyDot {
		return flowkind.FieldName
	}
	if scalarKeyExpr(expr.Key) {
		return flowkind.FieldExact
	}
	return flowkind.FieldKey
}

func fieldKindForField(field *ast.Field) flowkind.FieldKind {
	if field.Key == nil {
		return flowkind.FieldList
	}
	if field.KeySyntax == ast.AttrKeyDot {
		return flowkind.FieldName
	}
	if scalarKeyExpr(field.Key) {
		return flowkind.FieldExact
	}
	return flowkind.FieldKey
}

func scalarKeyExpr(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.NilExpr, *ast.FalseExpr, *ast.TrueExpr, *ast.NumberExpr, *ast.StringExpr:
		return true
	default:
		return false
	}
}

func valuesNilAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	nils := p.Source().Literals().Nils()
	for index := 0; index < nils.Count(); index++ {
		term, _, ok := nils.At(index)
		if !ok {
			t.Fatalf("NilAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Nil terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Nil term at exact AST span")
	}
	return found
}

func valuesBoolAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	bools := p.Source().Literals().Bools()
	for index := 0; index < bools.Count(); index++ {
		term, _, _, ok := bools.At(index)
		if !ok {
			t.Fatalf("BoolAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Bool terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Bool term at exact AST span")
	}
	return found
}

func valuesIntegerAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	integers := p.Source().Literals().Integers()
	for index := 0; index < integers.Count(); index++ {
		term, _, _, ok := integers.At(index)
		if !ok {
			t.Fatalf("IntegerAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Integer terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Integer term at exact AST span")
	}
	return found
}

func valuesFloatAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	floats := p.Source().Literals().Floats()
	for index := 0; index < floats.Count(); index++ {
		term, _, _, ok := floats.At(index)
		if !ok {
			t.Fatalf("FloatAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Float terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Float term at exact AST span")
	}
	return found
}

func valuesStringAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	strings := p.Source().Literals().Strings()
	for index := 0; index < strings.Count(); index++ {
		term, _, _, ok := strings.At(index)
		if !ok {
			t.Fatalf("StringAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact String terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no String term at exact AST span")
	}
	return found
}

func valuesVarargAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	varargs := p.Flow().Authored().Storage().Varargs()
	for index := 0; index < varargs.Count(); index++ {
		term, ok := varargs.At(index)
		if !ok {
			t.Fatalf("VarargAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Vararg terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Vararg term at exact AST span")
	}
	return found
}

func valuesReadAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	reads := p.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		term, ok := reads.At(index)
		if !ok {
			t.Fatalf("ReadAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Read terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Read term at exact AST span")
	}
	return found
}

func valuesAssignAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	assigns := p.Flow().Authored().Storage().Assigns()
	for index := 0; index < assigns.Count(); index++ {
		term, ok := assigns.At(index)
		if !ok {
			t.Fatalf("AssignAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Assign terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Assign term at exact AST span")
	}
	return found
}

func valuesReturnAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	returns := p.Flow().Authored().Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		term, ok := returns.At(index)
		if !ok {
			t.Fatalf("ReturnAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Return terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Return term at exact AST span")
	}
	return found
}

func valuesTableAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	tables := p.Flow().Authored().Tables()
	for index := 0; index < tables.Count(); index++ {
		term, ok := tables.At(index)
		if !ok {
			t.Fatalf("TableAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact Table terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no Table term at exact AST span")
	}
	return found
}

func valuesTableFieldAt(t *testing.T, p *program.Program, target ast.PositionHolder) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	fields := p.Flow().Authored().Fields()
	for index := 0; index < fields.Count(); index++ {
		term, ok := fields.At(index)
		if !ok {
			t.Fatalf("TableFieldAt(%d) failed", index)
		}
		if valuesSameSpan(p, term, target) {
			if found != 0 {
				t.Fatal("multiple exact TableField terms")
			}
			found = term
		}
	}
	if found == 0 {
		t.Fatal("no TableField term at exact AST span")
	}
	return found
}

func valuesSameSpan(p *program.Program, term keyspace.Term, target ast.PositionHolder) bool {
	span, ok := p.Source().Identity().Span(term)
	want := ast.SpanOf(target)
	return ok && span.StartLine == uint32(want.StartLine) && span.StartCol == uint32(want.StartCol) && span.EndLine == uint32(want.EndLine) && span.EndCol == uint32(want.EndCol)
}

func valuesASTTarget(t *testing.T, statements []ast.Stmt, form string, line int) ast.PositionHolder {
	t.Helper()
	var matches []ast.PositionHolder
	valuesWalkStatements(statements, func(node ast.PositionHolder, nodeForm string) {
		if nodeForm == form && node.Line() == line {
			matches = append(matches, node)
		}
	})
	if len(matches) != 1 {
		t.Fatalf("AST target %s at line %d has %d matches, want exactly one", form, line, len(matches))
	}
	return matches[0]
}

func valuesFieldTarget(target ast.PositionHolder, statements []ast.Stmt) (*ast.Field, bool, bool) {
	want, ok := target.(*ast.Field)
	if !ok {
		return nil, false, false
	}
	var final bool
	var seen int
	valuesWalkTableFields(statements, func(field *ast.Field, isFinal bool) {
		if field == want {
			seen++
			final = isFinal
		}
	})
	return want, final, seen == 1
}

func valuesWalkStatements(statements []ast.Stmt, visit func(ast.PositionHolder, string)) {
	for _, stmt := range statements {
		valuesWalkStmt(stmt, visit)
	}
}

func valuesWalkStmt(stmt ast.Stmt, visit func(ast.PositionHolder, string)) {
	switch current := stmt.(type) {
	case *ast.AssignStmt:
		visit(current, "AssignStmt")
		for _, expr := range current.Lhs {
			valuesWalkExpr(expr, visit)
		}
		for _, expr := range current.Rhs {
			valuesWalkExpr(expr, visit)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range current.Exprs {
			valuesWalkExpr(expr, visit)
		}
	case *ast.FuncCallStmt:
		valuesWalkExpr(current.Expr, visit)
	case *ast.DoBlockStmt:
		valuesWalkStatements(current.Stmts, visit)
	case *ast.WhileStmt:
		valuesWalkExpr(current.Condition, visit)
		valuesWalkStatements(current.Stmts, visit)
	case *ast.RepeatStmt:
		valuesWalkStatements(current.Stmts, visit)
		valuesWalkExpr(current.Condition, visit)
	case *ast.IfStmt:
		valuesWalkExpr(current.Condition, visit)
		valuesWalkStatements(current.Then, visit)
		valuesWalkStatements(current.Else, visit)
	case *ast.NumberForStmt:
		valuesWalkExpr(current.Init, visit)
		valuesWalkExpr(current.Limit, visit)
		valuesWalkExpr(current.Step, visit)
		valuesWalkStatements(current.Stmts, visit)
	case *ast.GenericForStmt:
		for _, expr := range current.Exprs {
			valuesWalkExpr(expr, visit)
		}
		valuesWalkStatements(current.Stmts, visit)
	case *ast.FuncDefStmt:
		valuesWalkExpr(current.Name.Func, visit)
		valuesWalkExpr(current.Name.Receiver, visit)
		valuesWalkExpr(current.Func, visit)
	case *ast.ReturnStmt:
		visit(current, "ReturnStmt")
		for _, expr := range current.Exprs {
			valuesWalkExpr(expr, visit)
		}
	}
}

func valuesWalkExpr(expr ast.Expr, visit func(ast.PositionHolder, string)) {
	if expr == nil {
		return
	}
	switch current := expr.(type) {
	case *ast.NilExpr:
		visit(current, "NilExpr")
	case *ast.FalseExpr:
		visit(current, "FalseExpr")
	case *ast.TrueExpr:
		visit(current, "TrueExpr")
	case *ast.NumberExpr:
		visit(current, "NumberExpr")
	case *ast.StringExpr:
		visit(current, "StringExpr")
	case *ast.Comma3Expr:
		visit(current, "Comma3Expr")
	case *ast.IdentExpr:
		visit(current, "IdentExpr")
	case *ast.AttrGetExpr:
		visit(current, "AttrGetExpr")
		valuesWalkExpr(current.Object, visit)
		valuesWalkExpr(current.Key, visit)
	case *ast.TableExpr:
		visit(current, "TableExpr")
		for _, field := range current.Fields {
			valuesWalkField(field, visit)
		}
	case *ast.FuncCallExpr:
		valuesWalkExpr(current.Func, visit)
		valuesWalkExpr(current.Receiver, visit)
		for _, arg := range current.Args {
			valuesWalkExpr(arg, visit)
		}
	case *ast.LogicalOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.RelationalOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.StringConcatOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.ArithmeticOpExpr:
		valuesWalkExpr(current.Lhs, visit)
		valuesWalkExpr(current.Rhs, visit)
	case *ast.UnaryMinusOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.UnaryNotOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.UnaryLenOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.UnaryBNotOpExpr:
		valuesWalkExpr(current.Expr, visit)
	case *ast.FunctionExpr:
		valuesWalkStatements(current.Stmts, visit)
	case *ast.CastExpr:
		valuesWalkExpr(current.Expr, visit)
	}
}

func valuesWalkField(field *ast.Field, visit func(ast.PositionHolder, string)) {
	if field == nil {
		return
	}
	visit(field, "Field")
	valuesWalkExpr(field.Key, visit)
	valuesWalkExpr(field.Value, visit)
}

func valuesWalkTableFields(statements []ast.Stmt, visit func(*ast.Field, bool)) {
	var walkStmt func(ast.Stmt)
	var walkExpr func(ast.Expr)
	walkStmt = func(stmt ast.Stmt) {
		switch current := stmt.(type) {
		case *ast.AssignStmt:
			for _, expr := range current.Lhs {
				walkExpr(expr)
			}
			for _, expr := range current.Rhs {
				walkExpr(expr)
			}
		case *ast.LocalAssignStmt:
			for _, expr := range current.Exprs {
				walkExpr(expr)
			}
		case *ast.FuncCallStmt:
			walkExpr(current.Expr)
		case *ast.DoBlockStmt:
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.WhileStmt:
			walkExpr(current.Condition)
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.RepeatStmt:
			for _, child := range current.Stmts {
				walkStmt(child)
			}
			walkExpr(current.Condition)
		case *ast.IfStmt:
			walkExpr(current.Condition)
			for _, child := range current.Then {
				walkStmt(child)
			}
			for _, child := range current.Else {
				walkStmt(child)
			}
		case *ast.NumberForStmt:
			walkExpr(current.Init)
			walkExpr(current.Limit)
			walkExpr(current.Step)
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.GenericForStmt:
			for _, expr := range current.Exprs {
				walkExpr(expr)
			}
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.FuncDefStmt:
			walkExpr(current.Name.Func)
			walkExpr(current.Name.Receiver)
			walkExpr(current.Func)
		case *ast.ReturnStmt:
			for _, expr := range current.Exprs {
				walkExpr(expr)
			}
		}
	}
	walkExpr = func(expr ast.Expr) {
		if expr == nil {
			return
		}
		switch current := expr.(type) {
		case *ast.AttrGetExpr:
			walkExpr(current.Object)
			walkExpr(current.Key)
		case *ast.TableExpr:
			for index, field := range current.Fields {
				visit(field, index == len(current.Fields)-1)
				walkExpr(field.Key)
				walkExpr(field.Value)
			}
		case *ast.FuncCallExpr:
			walkExpr(current.Func)
			walkExpr(current.Receiver)
			for _, arg := range current.Args {
				walkExpr(arg)
			}
		case *ast.LogicalOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(current.Lhs)
			walkExpr(current.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(current.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(current.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(current.Expr)
		case *ast.UnaryBNotOpExpr:
			walkExpr(current.Expr)
		case *ast.FunctionExpr:
			for _, child := range current.Stmts {
				walkStmt(child)
			}
		case *ast.CastExpr:
			walkExpr(current.Expr)
		}
	}
	for _, stmt := range statements {
		walkStmt(stmt)
	}
}
