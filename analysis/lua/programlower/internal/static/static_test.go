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

func TestPrimitiveAnnotationsFailClosed(t *testing.T) {
	w, _, _ := writerFor(t, "")
	_, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{
		Name:        "number",
		Annotations: []ast.AnnotationExpr{{Name: "min"}},
	})
	if !handled || err == nil {
		t.Fatalf("annotated primitive handled=%v err=%v", handled, err)
	}
}

func TestWriterStructuralContainers(t *testing.T) {
	t.Run("array rejects unsupported annotations", func(t *testing.T) {
		w, _, _ := writerFor(t, "")
		element, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: "number"})
		if err != nil || !handled {
			t.Fatalf("element = %v %v", handled, err)
		}
		for _, expr := range []*ast.ArrayTypeExpr{
			{Element: &ast.PrimitiveTypeExpr{Name: "number"}, ElementAnnotations: []ast.AnnotationExpr{{Name: "element"}}},
			{Element: &ast.PrimitiveTypeExpr{Name: "number"}, ArrayAnnotations: []ast.AnnotationExpr{{Name: "array"}}},
		} {
			if _, err := w.Array(expr, element); err == nil {
				t.Fatal("annotated array was accepted")
			}
		}
	})

	t.Run("map preserves ordered children and readonly", func(t *testing.T) {
		w, builder, _ := writerFor(t, "")
		key, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: "string"})
		if err != nil || !handled {
			t.Fatalf("key = %v %v", handled, err)
		}
		value, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: "number"})
		if err != nil || !handled {
			t.Fatalf("value = %v %v", handled, err)
		}
		mapped, err := w.Map(&ast.MapTypeExpr{Key: &ast.PrimitiveTypeExpr{Name: "string"}, Value: &ast.PrimitiveTypeExpr{Name: "number"}, Readonly: true}, key, value)
		if err != nil {
			t.Fatal(err)
		}
		p := sealStaticTarget(t, builder, mapped)
		gotKey, gotValue, readonly, ok := p.Map(mapped)
		if !ok || gotKey != key || gotValue != value || !readonly {
			t.Fatalf("map = %v %v %v %v", gotKey, gotValue, readonly, ok)
		}
	})

	t.Run("record preserves field metadata and reuses descriptor scratch", func(t *testing.T) {
		w, builder, stmts := writerFor(t, "type Shape = readonly { first: string, second?: number }")
		record, ok := stmts[0].(*ast.TypeDefStmt).Type.(*ast.RecordTypeExpr)
		if !ok {
			t.Fatalf("record = %T", stmts[0].(*ast.TypeDefStmt).Type)
		}
		first, handled, err := w.Leaf(record.Fields[0].Type)
		if err != nil || !handled {
			t.Fatalf("first = %v %v", handled, err)
		}
		second, handled, err := w.Leaf(record.Fields[1].Type)
		if err != nil || !handled {
			t.Fatalf("second = %v %v", handled, err)
		}
		mark := w.Mark()
		if err := w.Append(first); err != nil {
			t.Fatal(err)
		}
		if err := w.Append(second); err != nil {
			t.Fatal(err)
		}
		shape, err := w.Record(record, mark, len(record.Fields))
		if err != nil || !w.Clean() {
			t.Fatalf("record = %v clean=%v", err, w.Clean())
		}

		p := sealStaticTarget(t, builder, shape)
		readonly, count, ok := p.Record(shape)
		if !ok || !readonly || count != 2 {
			t.Fatalf("record = %v %d %v", readonly, count, ok)
		}
		key, typ, span, optional, ok := p.RecordField(shape, 1)
		if !ok || key == 0 || typ != second || !optional {
			t.Fatalf("second field = %v %v %v %v", key, typ, optional, ok)
		}
		position := record.Fields[1].NamePosition
		wantSpan := program.Span{File: position.File, StartLine: position.Line, StartCol: position.Column, EndLine: position.EndLine, EndCol: position.EndColumn}
		if wantSpan.File == "" {
			wantSpan.File = "static.lua"
		}
		if span != wantSpan {
			t.Fatalf("field name span = %#v, want %#v", span, wantSpan)
		}
	})

	t.Run("record reuses writer descriptor scratch", func(t *testing.T) {
		w, _, _ := writerFor(t, "")
		record := &ast.RecordTypeExpr{Fields: []ast.RecordFieldExpr{
			{Name: "first", Type: &ast.PrimitiveTypeExpr{Name: "string"}},
			{Name: "second", Type: &ast.PrimitiveTypeExpr{Name: "number"}},
		}}
		makeRecord := func() {
			types := []string{"string", "number"}
			mark := w.Mark()
			for _, name := range types {
				term, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: name})
				if err != nil || !handled || w.Append(term) != nil {
					t.Fatalf("field %q = handled=%v err=%v", name, handled, err)
				}
			}
			if _, err := w.Record(record, mark, len(record.Fields)); err != nil || !w.Clean() {
				t.Fatalf("record err=%v clean=%v", err, w.Clean())
			}
		}
		makeRecord()
		fieldCapacity := cap(w.fields)
		if fieldCapacity < len(record.Fields) {
			t.Fatalf("field scratch cap = %d", fieldCapacity)
		}
		makeRecord()
		if cap(w.fields) != fieldCapacity {
			t.Fatalf("field scratch cap=%d want=%d", cap(w.fields), fieldCapacity)
		}
	})

	t.Run("record rejects field annotations", func(t *testing.T) {
		w, _, _ := writerFor(t, "")
		term, handled, err := w.Leaf(&ast.PrimitiveTypeExpr{Name: "string"})
		if err != nil || !handled {
			t.Fatalf("term = %v %v", handled, err)
		}
		mark := w.Mark()
		if err := w.Append(term); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Record(&ast.RecordTypeExpr{Fields: []ast.RecordFieldExpr{{Name: "name", Type: &ast.PrimitiveTypeExpr{Name: "string"}, Annotations: []ast.AnnotationExpr{{Name: "field"}}}}}, mark, 1); err == nil {
			t.Fatal("annotated record field was accepted")
		}
	})
}

func sealStaticTarget(t *testing.T, builder *program.Builder, target program.Term) *program.Program {
	t.Helper()
	entry := builder.Body(program.Span{})
	if entry == 0 || !builder.SetEntry(entry) {
		t.Fatal("entry")
	}
	alias := builder.DeclareTypeAlias(program.Span{}, entry, "Target")
	if alias == 0 || !builder.SetTypeAliasGap(alias, 0) || !builder.SetTypeAliasParams(alias, nil) || !builder.FillTypeAlias(alias, target) || !builder.SetBody(entry) {
		t.Fatal("attach target")
	}
	p, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	return p
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
