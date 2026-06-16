package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestObjectLiteralDiagnosticTypeSeparatesDotFieldAndBracketStringMember(t *testing.T) {
	got := objectLiteralType(nil, semantics.ObjectLiteralFact{
		Entries: []semantics.ObjectEntryFact{
			{
				Suffix: path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "id"}}},
				Value:  &ast.StringExpr{Value: "field"},
			},
			{
				Suffix: path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: "id"}}},
				Value:  &ast.TrueExpr{},
			},
		},
	})
	want := typetable.NewRecord().
		Field("id", typ.LiteralString("field")).
		StaticStringIndex("id", typ.LiteralBool(true)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal diagnostic type = %v, want %v", got, want)
	}
}

func TestDirectObjectLiteralArgumentTypeSeparatesDotFieldAndBracketStringMember(t *testing.T) {
	table := &ast.TableExpr{Fields: []*ast.Field{
		{
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyDot,
			Value:     &ast.StringExpr{Value: "field"},
		},
		{
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyIndex,
			Value:     &ast.TrueExpr{},
		},
	}}

	got, ok := directObjectLiteralArgumentType(nil, nil, 0, table, nil)
	if !ok {
		t.Fatal("directObjectLiteralArgumentType returned false")
	}
	want := typetable.NewRecord().
		Field("id", typ.LiteralString("field")).
		StaticStringIndex("id", typ.LiteralBool(true)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("direct object literal argument type = %v, want %v", got, want)
	}
}

func TestAnnotationAssignabilityAcceptsBracketStringMapLiteralEntries(t *testing.T) {
	diags := runDiagnostics(t, `
		local routes: {[string]: string} = { ["/ok"] = "page:ok" }
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityBracketStringDoesNotSatisfyRequiredField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local p: Point = {["x"] = 10, y = 20}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, `"x"`) {
		t.Fatalf("diagnostic = %#v, want missing required field x", d)
	}
}

func TestAnnotationAssignabilityRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		local raw: any = nil
		local p: Point = {id = raw}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "any") || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want any-to-string object member mismatch", d)
	}
}

func TestAnnotationAssignabilityRejectsObjectLiteralTopOriginInsideClosedUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		type A = {kind: "a", id: string}
		type B = {kind: "b", id: number}
		type Item = A | B
		local raw: any = nil
		local item: Item = {kind = "a", id = raw}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
		t.Fatalf("diagnostic = %#v, want union object member mismatch", d)
	}
}
