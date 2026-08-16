package occurrence

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticowner "github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestMalformedNumericLiteralIsParserReachableButPublicIngressRejected(t *testing.T) {
	const input = "local value: 1e9999 = 0"
	statements := parseResidueSource(t, input)
	local, ok := statements[0].(*ast.LocalAssignStmt)
	if !ok || len(local.Types) != 1 {
		t.Fatalf("invalid-number root = %T types=%d", statements[0], len(local.Types))
	}
	literal, ok := local.Types[0].(*ast.LiteralTypeExpr)
	if !ok {
		t.Fatalf("invalid-number type = %T", local.Types[0])
	}
	if literal.Value != nil {
		t.Fatalf("invalid-number parser value = %#v, want nil", literal.Value)
	}
	if _, err := lualower.Lower(lualower.Source{Name: "invalid-number.lua", Text: []byte(input)}); err == nil || !strings.Contains(err.Error(), "malformed numeric literal type") {
		t.Fatalf("invalid-number public ingress error = %v, want malformed numeric literal type", err)
	}
}

// These four public vertical witnesses are the exact successful-source states
// which a reduction-time trace cannot see reliably. They prove source syntax,
// final parser state, and the corresponding sealed Program relation together;
// no lowerer package structure is inspected.
func TestSourceReachableResidueHasExactProgramSemantics(t *testing.T) {
	t.Run("scalar-vararg", witnessScalarVararg)
	t.Run("generic-function-type", witnessGenericFunctionType)
	t.Run("empty-interface", witnessEmptyInterface)
	t.Run("optional-interface-field", witnessOptionalInterfaceField)
}

func witnessScalarVararg(t *testing.T) {
	const source = "local function f(...) return (...) end"
	statements := parseResidueSource(t, source)
	local, ok := statements[0].(*ast.LocalAssignStmt)
	if !ok || len(local.Exprs) != 1 {
		t.Fatalf("source root = %T", statements[0])
	}
	function, ok := local.Exprs[0].(*ast.FunctionExpr)
	if !ok || len(function.Stmts) != 1 {
		t.Fatalf("local initializer = %T", local.Exprs[0])
	}
	returned, ok := function.Stmts[0].(*ast.ReturnStmt)
	if !ok || len(returned.Exprs) != 1 {
		t.Fatalf("function statement = %T", function.Stmts[0])
	}
	vararg, ok := returned.Exprs[0].(*ast.Comma3Expr)
	if !ok || !vararg.AdjustRet {
		t.Fatalf("parenthesized vararg = %T adjust=%v", returned.Exprs[0], ok && vararg.AdjustRet)
	}
	p := lowerResidueSource(t, source)
	varargs := p.Flow().Authored().Storage().Varargs()
	varargTerm := termAtSpan(t, p, varargs.Count, varargs.At, spanOf(vararg))
	returns := p.Flow().Authored().Control().Returns()
	returnTerm := termAtSpan(t, p, returns.Count, returns.At, spanOf(returned))
	_, values, ok := returns.Get(returnTerm)
	if !ok {
		t.Fatal("source Return has no Values relation")
	}
	valueView := p.Flow().Authored().Values()
	fixed, fixedOK := valueView.Len(values)
	_, tail, valuesOK := valueView.Get(values)
	first, firstOK := valueView.Member(values, 0)
	if !fixedOK || !valuesOK || !firstOK || fixed != 1 || tail != 0 || first != varargTerm {
		t.Fatalf("scalar vararg Values = fixed %d tail %v first %v/%v, want exact %v", fixed, tail, first, firstOK, varargTerm)
	}
}

func witnessGenericFunctionType(t *testing.T) {
	const source = "interface Subject\n function map<T: string>(value: T): T\nend"
	statements := parseResidueSource(t, source)
	decl, ok := statements[0].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("source root = %T", statements[0])
	}
	if len(decl.Members) != 1 {
		t.Fatalf("interface members=%d, want one", len(decl.Members))
	}
	signature, ok := decl.Members[0].Type.(*ast.FunctionTypeExpr)
	if !ok || len(signature.TypeParams) != 1 {
		t.Fatalf("method signature = %T generics=%d", decl.Members[0].Type, len(signature.TypeParams))
	}
	p := lowerResidueSource(t, source)
	interfaces := p.Static().Declarations().Interfaces()
	iface, ok := interfaces.At(0)
	if !ok {
		t.Fatal("Program has no interface declaration")
	}
	member, ok := interfaces.MemberAt(iface, 0)
	if !ok || member.Kind != staticowner.InterfaceMethod || member.Signature == 0 {
		t.Fatalf("Program interface member = %#v/%v", member, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().TypeParamCount(member.Signature); !ok || count != 1 {
		t.Fatalf("Program method generic count = %d/%v, want one", count, ok)
	}
}

func witnessEmptyInterface(t *testing.T) {
	const source = "interface Empty end"
	statements := parseResidueSource(t, source)
	decl, ok := statements[0].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("empty interface root = %T", statements[0])
	}
	if len(decl.Members) != 0 {
		t.Fatalf("empty interface members=%d", len(decl.Members))
	}
	p := lowerResidueSource(t, source)
	interfaces := p.Static().Declarations().Interfaces()
	iface, ok := interfaces.At(0)
	if !ok {
		t.Fatal("Program has no empty interface declaration")
	}
	if count, ok := interfaces.MemberCount(iface); !ok || count != 0 {
		t.Fatalf("Program empty interface members = %d/%v", count, ok)
	}
}

func witnessOptionalInterfaceField(t *testing.T) {
	const source = "interface Subject\n field?: string\nend"
	statements := parseResidueSource(t, source)
	decl, ok := statements[0].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("optional interface root = %T", statements[0])
	}
	if len(decl.Members) != 1 || !decl.Members[0].Optional {
		t.Fatalf("optional interface members=%#v", decl.Members)
	}
	p := lowerResidueSource(t, source)
	interfaces := p.Static().Declarations().Interfaces()
	iface, ok := interfaces.At(0)
	if !ok {
		t.Fatal("Program has no interface declaration")
	}
	member, ok := interfaces.MemberAt(iface, 0)
	if !ok || member.Kind != staticowner.InterfaceField || member.Field == 0 {
		t.Fatalf("Program optional field member = %#v/%v", member, ok)
	}
	_, _, optional, ok := p.Static().Types().Fields().Get(member.Field)
	if !ok || !optional {
		t.Fatalf("Program TypeField optional = %v/%v", optional, ok)
	}
}

func parseResidueSource(t *testing.T, source string) []ast.Stmt {
	t.Helper()
	statements, err := parse.ParseString(source, "residue.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) == 0 {
		t.Fatal("parser returned no statements")
	}
	return statements
}

func lowerResidueSource(t *testing.T, source string) *program.Program {
	t.Helper()
	p, err := lualower.Lower(lualower.Source{Name: "residue.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func spanOf(value ast.PositionHolder) source.Span {
	return source.Span{File: "residue.lua", StartLine: uint32(value.Line()), StartCol: uint32(value.Column()), EndLine: uint32(value.LastLine()), EndCol: uint32(value.LastColumn())}
}

func termAtSpan(t *testing.T, p *program.Program, count func() int, at func(int) (keyspace.Term, bool), want source.Span) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			t.Fatalf("Program family enumeration missing %d", index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok || span != want {
			continue
		}
		if found != 0 {
			t.Fatalf("source span %#v names multiple Program Terms", want)
		}
		found = term
	}
	if found == 0 {
		t.Fatalf("source span %#v has no Program Term", want)
	}
	return found
}
