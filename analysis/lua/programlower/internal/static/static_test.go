package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
)

func writerFor(t *testing.T, source string) (*Writer, *program.Builder, []ast.Stmt) {
	t.Helper()
	stmts, err := parse.ParseString(source, "static.lua")
	if err != nil {
		t.Fatal(err)
	}
	builder := program.NewBuilder()
	return New(builder, bind.BindChunk(stmts, bind.Options{}), "static.lua"), builder, stmts
}

func TestWriterPredeclarePlaceAndLexicalIdentity(t *testing.T) {
	w, builder, stmts := writerFor(t, `
type Box<T> = T
type Uses = Box<number>
`)
	entry := builder.Body(program.Span{})
	if entry == 0 || !builder.SetEntry(entry) {
		t.Fatal("entry")
	}
	if err := w.Predeclare(entry, stmts); err != nil {
		t.Fatal("predeclare", err)
	}
	box, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("first statement = %T", stmts[0])
	}
	uses, ok := stmts[1].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("second statement = %T", stmts[1])
	}
	if err := w.Place(box, 0); err != nil {
		t.Fatal(err)
	}
	params := w.binding.TypeDefParams(box)
	if len(params) != 1 {
		t.Fatalf("params = %d", len(params))
	}
	param, ok := w.Host(params[0])
	if !ok || param == 0 {
		t.Fatal("missing parameter host")
	}
	boxTarget, handled, err := w.Leaf(box.Type)
	if err != nil || !handled || boxTarget == 0 {
		t.Fatalf("box target = %v %v %v", boxTarget, handled, err)
	}
	if err := w.FinishParam(params[0], 0); err != nil {
		t.Fatal(err)
	}
	if err := w.FinishAlias(box, boxTarget); err != nil {
		t.Fatal(err)
	}
	if err := w.Place(uses, 0); err != nil {
		t.Fatal(err)
	}
	generic, ok := uses.Type.(*ast.GenericTypeExpr)
	if !ok {
		t.Fatalf("uses type = %T", uses.Type)
	}
	base, handled, err := w.Leaf(generic.Base)
	if err != nil || !handled {
		t.Fatalf("generic base = %v %v %v", base, handled, err)
	}
	mark := w.Mark()
	arg, handled, err := w.Leaf(generic.Args[0])
	if err != nil || !handled || w.Append(arg) != nil {
		t.Fatalf("generic arg = %v %v %v", arg, handled, err)
	}
	usesTarget, err := w.Generic(generic, base, mark, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.FinishAlias(uses, usesTarget); err != nil || !w.Clean() || !builder.SetBody(entry) {
		t.Fatalf("finish uses = %v clean=%v", err, w.Clean())
	}
	p, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	state, target, _, _, ok := p.TypeRef(boxTarget)
	if !ok || state != program.TypeRefDeclaration || target != param {
		t.Fatalf("Box<T> reference = %v %v %v", state, target, ok)
	}
	state, target, _, _, ok = p.TypeRef(base)
	boxHost, _ := w.Alias(box)
	if !ok || state != program.TypeRefDeclaration || target != boxHost {
		t.Fatalf("Box<number> base = %v %v %v", state, target, ok)
	}
}

func TestWriterLeafAndOrderedFinishers(t *testing.T) {
	w, _, _ := writerFor(t, "")
	if _, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: "number"}); err != nil || !handled {
		t.Fatalf("primitive leaf handled=%v err=%v", handled, err)
	}
	if _, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: "Custom"}); err != nil || !handled {
		t.Fatalf("unresolved primitive leaf handled=%v err=%v", handled, err)
	}
	if _, err := w.Literal(&ast.LiteralTypeExpr{}); err == nil {
		t.Fatal("malformed numeric literal was accepted")
	}
	if _, err := w.TypeRef(&ast.TypeRefExpr{Path: []string{"a", "b", "c"}}); err == nil {
		t.Fatal("three-part parser-unreachable reference was accepted")
	}
	base, _, err := w.Leaf(&ast.TypeRefExpr{Path: []string{"Box"}})
	if err != nil {
		t.Fatal(err)
	}
	mark := w.Mark()
	member, _, err := w.Leaf(&ast.LiteralTypeExpr{Value: int64(1)})
	if err != nil || w.Append(member) != nil {
		t.Fatalf("member = %v", err)
	}
	if _, err := w.Generic(&ast.GenericTypeExpr{Base: &ast.TypeRefExpr{Path: []string{"Box"}}}, base, mark, 1); err != nil {
		t.Fatal(err)
	}
	if !w.Clean() {
		t.Fatal("ordered static scratch leaked")
	}
}

func TestWriterQualifiedReferenceDecisions(t *testing.T) {
	t.Run("qualified source resolves to lexical alias", func(t *testing.T) {
		w, builder, stmts := writerFor(t, `return function()
type Local = string
local M = {}
M.Remote = Local
type Use = M.Remote
end`)
		fn := stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FunctionExpr)
		stmts = fn.Stmts
		entry := builder.Body(program.Span{})
		if !builder.SetEntry(entry) || w.Predeclare(entry, stmts) != nil {
			t.Fatal("entry/predeclare")
		}
		local := stmts[0].(*ast.TypeDefStmt)
		use := stmts[3].(*ast.TypeDefStmt)
		if err := w.Place(local, 0); err != nil {
			t.Fatal(err)
		}
		localTarget, handled, err := w.Leaf(local.Type)
		if err != nil || !handled || w.FinishAlias(local, localTarget) != nil {
			t.Fatal("finish Local")
		}
		if err := w.Place(use, 0); err != nil {
			t.Fatal(err)
		}
		ref, handled, err := w.Leaf(use.Type)
		if err != nil || !handled || w.FinishAlias(use, ref) != nil || !builder.SetBody(entry) {
			t.Fatal("finish Use")
		}
		p, err := builder.Seal()
		if err != nil {
			t.Fatal(err)
		}
		state, target, pkg, _, ok := p.TypeRef(ref)
		localHost, _ := w.Alias(local)
		if !ok || state != program.TypeRefDeclaration || target != localHost || pkg == 0 {
			t.Fatalf("qualified lexical ref = %v %v %v %v", state, target, pkg, ok)
		}
	})
	t.Run("qualified source preserves canonical path", func(t *testing.T) {
		w, builder, stmts := writerFor(t, `return function()
local M = {}
M.Remote = external.Type
type Use = M.Remote
end`)
		fn := stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FunctionExpr)
		stmts = fn.Stmts
		entry := builder.Body(program.Span{})
		if !builder.SetEntry(entry) || w.Predeclare(entry, stmts) != nil {
			t.Fatal("entry/predeclare")
		}
		use := stmts[2].(*ast.TypeDefStmt)
		if err := w.Place(use, 0); err != nil {
			t.Fatal(err)
		}
		ref, handled, err := w.Leaf(use.Type)
		if err != nil || !handled || w.FinishAlias(use, ref) != nil || !builder.SetBody(entry) {
			t.Fatal("finish Use")
		}
		p, err := builder.Seal()
		if err != nil {
			t.Fatal(err)
		}
		state, target, pkg, _, ok := p.TypeRef(ref)
		length, lengthOK := p.TypeRefPathLen(ref)
		if !ok || !lengthOK || state != program.TypeRefCanonicalPath || target != 0 || pkg == 0 || length != 2 {
			t.Fatalf("canonical ref = %v %v %v %d %v", state, target, pkg, length, ok)
		}
	})
}
