package controllaw

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

const controlLawFile = "control.lua"

// TestExactBindingAndForSourceLaw verifies source names and their lexical
// storage identities without relying on a lowerer type switch or a generic
// symbol registry.
func TestExactBindingAndForSourceLaw(t *testing.T) {
	const source = "local value = 1\nfor index = 1, 2 do end\nfor key, item in pairs({}) do end"
	statements, p := parseBindLower(t, source)
	local := statements[0].(*ast.LocalAssignStmt)
	number := statements[1].(*ast.NumberForStmt)
	generic := statements[2].(*ast.GenericForStmt)

	binds := p.Flow().Authored().Storage().Binds()
	bind, err := anchored(p, local, binds.Count, binds.At)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := p.Source().Binds().Len(bind); !ok || count != len(local.Names) {
		t.Fatalf("Bind cells = %d/%v, want %d", count, ok, len(local.Names))
	}
	cell, ok := p.Source().Binds().At(bind, 0)
	if !ok {
		t.Fatal("local binding has no Cell")
	}
	if err := exactToken(p, cell, local.NamePositions[0]); err != nil {
		t.Fatalf("local name: %v", err)
	}

	loops := p.Flow().Authored().Control().Loops()
	numberLoop, err := anchored(p, number, loops.Count, loops.At)
	if err != nil {
		t.Fatal(err)
	}
	_, _, loopKind, _, ok := loops.Get(numberLoop)
	if !ok || loopKind != flowkind.LoopNumericFor {
		t.Fatalf("numeric Loop kind = %d/%v", loopKind, ok)
	}
	if count, ok := loops.CellCount(numberLoop); !ok || count != 1 {
		t.Fatalf("numeric Loop cells = %d/%v", count, ok)
	}
	index, ok := loops.CellAt(numberLoop, 0)
	if !ok {
		t.Fatal("numeric Loop index cell missing")
	}
	if err := exactToken(p, index, number.NamePosition); err != nil {
		t.Fatalf("numeric index: %v", err)
	}

	genericLoop, err := anchored(p, generic, loops.Count, loops.At)
	if err != nil {
		t.Fatal(err)
	}
	_, _, loopKind, values, ok := loops.Get(genericLoop)
	if !ok || loopKind != flowkind.LoopGenericFor {
		t.Fatalf("generic Loop kind = %d/%v", loopKind, ok)
	}
	if count, ok := loops.CellCount(genericLoop); !ok || count != len(generic.Names) {
		t.Fatalf("generic Loop cells = %d/%v, want %d", count, ok, len(generic.Names))
	}
	for position := range generic.Names {
		cell, ok := loops.CellAt(genericLoop, position)
		if !ok {
			t.Fatalf("generic Loop cell %d missing", position)
		}
		if err := exactToken(p, cell, generic.NamePositions[position]); err != nil {
			t.Fatalf("generic name %d: %v", position, err)
		}
	}
	if values == 0 {
		t.Fatal("generic Loop has no header Values")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 0 {
		t.Fatalf("generic header Values fixed length = %d/%v, want no fixed values", fixed, ok)
	}
	call, ok := generic.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("generic iterator source = %T, want call", generic.Exprs[0])
	}
	calls := p.Flow().Authored().Calls()
	callTerm, err := anchored(p, call, calls.Count, calls.At)
	if err != nil {
		t.Fatal(err)
	}
	if _, tail, ok := p.Flow().Authored().Values().Get(values); !ok || tail != callTerm {
		t.Fatalf("generic header Values tail = %v/%v, want Call %v", tail, ok, callTerm)
	}
	if err := provenance.Exact(p.Source().Identity(), callTerm, call, controlLawFile); err != nil {
		t.Fatalf("generic header Call: %v", err)
	}
}

// TestExactLabelGotoSourceLaw proves both distinct label and goto source
// occurrences and their one typed target relation.
func TestExactLabelGotoSourceLaw(t *testing.T) {
	const source = "::again::\ngoto again"
	statements, p := parseBindLower(t, source)
	label := statements[0].(*ast.LabelStmt)
	jump := statements[1].(*ast.GotoStmt)
	if label.Name == "" || jump.Label == "" {
		t.Fatal("parser accepted empty label spelling")
	}
	labels := p.Flow().Authored().Control().Labels()
	labelTerm, err := anchored(p, label, labels.Count, labels.At)
	if err != nil {
		t.Fatal(err)
	}
	gotos := p.Flow().Authored().Control().Gotos()
	jumpTerm, err := anchored(p, jump, gotos.Count, gotos.At)
	if err != nil {
		t.Fatal(err)
	}
	labelOwner, ok := labels.Get(labelTerm)
	if !ok || labelOwner == 0 {
		t.Fatalf("Label = owner %v ok %v", labelOwner, ok)
	}
	jumpOwner, target, ok := gotos.Get(jumpTerm)
	if !ok || jumpOwner != labelOwner || target != labelTerm {
		t.Fatalf("Goto = owner %v target %v ok %v, want target %v", jumpOwner, target, ok, labelTerm)
	}
	if err := provenance.Exact(p.Source().Identity(), labelTerm, label, controlLawFile); err != nil {
		t.Fatal(err)
	}
	if err := provenance.Exact(p.Source().Identity(), jumpTerm, jump, controlLawFile); err != nil {
		t.Fatal(err)
	}
}

func parseBindLower(t testing.TB, source string) ([]ast.Stmt, *program.Program) {
	t.Helper()
	statements, err := parse.ParseString(source, controlLawFile)
	if err != nil {
		t.Fatal(err)
	}
	if bound := bind.BindChunk(statements); bound == nil {
		t.Fatal("public binder returned nil result")
	}
	p, err := programlower.Lower(programlower.Source{Name: controlLawFile, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return statements, p
}

func anchored(p *program.Program, node ast.PositionHolder, count func() int, at func(int) (keyspace.Term, bool)) (keyspace.Term, error) {
	want := source.Span{File: controlLawFile, StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			return 0, fmt.Errorf("typed enumeration missing %d", index)
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

func exactToken(p *program.Program, term keyspace.Term, position ast.Position) error {
	want := source.Span{File: controlLawFile, StartLine: uint32(position.Line), StartCol: uint32(position.Column), EndLine: uint32(position.EndLine), EndCol: uint32(position.EndColumn)}
	got, ok := p.Source().Identity().Span(term)
	if !ok || got != want {
		return fmt.Errorf("Source span = %#v/%v, want %#v", got, ok, want)
	}
	return nil
}
