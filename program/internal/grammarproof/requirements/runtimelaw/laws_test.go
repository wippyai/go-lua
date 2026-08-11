package runtimelaw

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/provenance"
	"github.com/wippyai/go-lua/program/keyspace"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/source"
)

const runtimeLawFile = "runtime.lua"

// TestExactScalarSourceLaw anchors an authored numeric literal and identifier
// Read through their existing typed Program relations.
func TestExactScalarSourceLaw(t *testing.T) {
	const source = "local value = 42\nreturn value"
	statements, p := parseBindLower(t, source)
	local := statements[0].(*ast.LocalAssignStmt)
	returned := statements[1].(*ast.ReturnStmt)
	number := local.Exprs[0].(*ast.NumberExpr)
	ident := returned.Exprs[0].(*ast.IdentExpr)

	integers := p.Source().Literals().Integers()
	integerAt := func(index int) (keyspace.Term, bool) {
		term, _, _, ok := integers.At(index)
		return term, ok
	}
	integer, err := anchored(p, number, integers.Count, integerAt)
	if err != nil {
		t.Fatal(err)
	}
	_, owner, value, ok := integers.At(int(keyspace.TermOrdinal(integer)) - 1)
	if !ok || owner == 0 || value != 42 {
		t.Fatalf("Integer = owner %v value %d ok %v", owner, value, ok)
	}
	if err := provenance.Exact(p.Source().Identity(), integer, number, runtimeLawFile); err != nil {
		t.Fatal(err)
	}
	reads := p.Flow().Authored().Storage().Reads()
	read, err := anchored(p, ident, reads.Count, reads.At)
	if err != nil {
		t.Fatal(err)
	}
	readOwner, sourceCell, _, ok := reads.Get(read)
	if !ok || readOwner == 0 || sourceCell == 0 {
		t.Fatalf("Read = owner %v source %v ok %v", readOwner, sourceCell, ok)
	}
	if err := provenance.Exact(p.Source().Identity(), read, ident, runtimeLawFile); err != nil {
		t.Fatal(err)
	}
}

// TestExactValueClaimSourceLaw covers the two source spellings that must not
// collapse: a target-bearing cast and targetless non-nil claim.
func TestExactValueClaimSourceLaw(t *testing.T) {
	const source = "local value = 1\nreturn value as number, value!"
	statements, p := parseBindLower(t, source)
	returned := statements[1].(*ast.ReturnStmt)
	cast := returned.Exprs[0].(*ast.CastExpr)
	nonNil := returned.Exprs[1].(*ast.NonNilAssertExpr)

	claims := p.Flow().Authored().Claims()
	castTerm, err := anchored(p, cast, claims.Count, claims.At)
	if err != nil {
		t.Fatal(err)
	}
	owner, operand, claimKind, ok := claims.Get(castTerm)
	target, targetOK := p.Static().Operands().Claims().Target(castTerm)
	if !ok || owner == 0 || operand == 0 || !targetOK || target == 0 || claimKind != flowkind.ValueClaimTypeAs {
		t.Fatalf("cast ValueClaim = owner %v operand %v target %v kind %d ok %v", owner, operand, target, claimKind, ok)
	}
	if cast.Syntax != ast.CastSyntaxAs {
		t.Fatalf("cast syntax = %d, want as", cast.Syntax)
	}
	if err := provenance.Exact(p.Source().Identity(), castTerm, cast, runtimeLawFile); err != nil {
		t.Fatal(err)
	}
	if err := provenance.Exact(p.Source().Identity(), operand, cast.Expr, runtimeLawFile); err != nil {
		t.Fatalf("cast operand: %v", err)
	}
	if err := provenance.Exact(p.Source().Identity(), target, cast.Type, runtimeLawFile); err != nil {
		t.Fatalf("cast target: %v", err)
	}

	nonNilTerm, err := anchored(p, nonNil, claims.Count, claims.At)
	if err != nil {
		t.Fatal(err)
	}
	owner, operand, claimKind, ok = claims.Get(nonNilTerm)
	target, targetOK = p.Static().Operands().Claims().Target(nonNilTerm)
	if !ok || owner == 0 || operand == 0 || targetOK || target != 0 || claimKind != flowkind.ValueClaimNonNil {
		t.Fatalf("non-nil ValueClaim = owner %v operand %v target %v kind %d ok %v", owner, operand, target, claimKind, ok)
	}
	if err := provenance.Exact(p.Source().Identity(), nonNilTerm, nonNil, runtimeLawFile); err != nil {
		t.Fatal(err)
	}
	if err := provenance.Exact(p.Source().Identity(), operand, nonNil.Expr, runtimeLawFile); err != nil {
		t.Fatalf("non-nil operand: %v", err)
	}
}

// TestExactVarargAdjustmentSourceLaw proves that parenthesized vararg syntax
// retains one authored occurrence while forcing the parser's scalar-result
// marker through Program Values.
func TestExactVarargAdjustmentSourceLaw(t *testing.T) {
	const source = "local function pack(...) return (...) end"
	statements, p := parseBindLower(t, source)
	function := statements[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	returned := function.Stmts[0].(*ast.ReturnStmt)
	vararg := returned.Exprs[0].(*ast.Comma3Expr)
	if !vararg.AdjustRet {
		t.Fatal("parenthesized vararg has no parser adjustment marker")
	}
	varargs := p.Flow().Authored().Storage().Varargs()
	term, err := anchored(p, vararg, varargs.Count, varargs.At)
	if err != nil {
		t.Fatal(err)
	}
	owner, cell, ok := varargs.Get(term)
	if !ok || owner == 0 || cell == 0 {
		t.Fatalf("Vararg = owner %v cell %v ok %v", owner, cell, ok)
	}
	if err := provenance.Exact(p.Source().Identity(), term, vararg, runtimeLawFile); err != nil {
		t.Fatal(err)
	}
	returns := p.Flow().Authored().Control().Returns()
	returnTerm, err := anchored(p, returned, returns.Count, returns.At)
	if err != nil {
		t.Fatal(err)
	}
	_, values, ok := returns.Get(returnTerm)
	if !ok {
		t.Fatal("Return lacks Values")
	}
	valueView := p.Flow().Authored().Values()
	if fixed, ok := valueView.Len(values); !ok || fixed != 1 {
		t.Fatalf("return Values length = %d/%v, want one scalar slot", fixed, ok)
	}
	if value, ok := valueView.Member(values, 0); !ok || value != term {
		t.Fatalf("return Values[0] = %v/%v, want Vararg %v", value, ok, term)
	}
}

func parseBindLower(t testing.TB, source string) ([]ast.Stmt, *program.Program) {
	t.Helper()
	statements, err := parse.ParseString(source, runtimeLawFile)
	if err != nil {
		t.Fatal(err)
	}
	if bound := bind.BindChunk(statements); bound == nil {
		t.Fatal("public binder returned nil result")
	}
	p, err := programlower.Lower(programlower.Source{Name: runtimeLawFile, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return statements, p
}

func anchored(p *program.Program, node ast.PositionHolder, count func() int, at func(int) (keyspace.Term, bool)) (keyspace.Term, error) {
	want := source.Span{File: runtimeLawFile, StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			return 0, fmt.Errorf("typed enumeration missing term %d", index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok || span != want {
			continue
		}
		if found != 0 {
			return 0, fmt.Errorf("source span maps to multiple typed terms")
		}
		found = term
	}
	if found == 0 {
		return 0, fmt.Errorf("no typed term at source span %d:%d-%d:%d", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
	}
	return found, nil
}
