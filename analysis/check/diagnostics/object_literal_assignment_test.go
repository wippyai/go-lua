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

func TestDirectCallRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		function take(p: Point)
		end
		local raw: any = nil
		take({id = raw})
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeDirectCallArgType || !strings.Contains(d.Message, "any") || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want any-to-string call argument member mismatch", d)
	}
}

func TestReturnContractRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		function make(raw: any): Point
			return {id = raw}
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType || !strings.Contains(d.Message, "any") || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want any-to-string return member mismatch", d)
	}
}

func TestReturnContractAcceptsGuardedObjectLiteralAnyMember(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {id: string}
		function make(raw: any): Point?
			if type(raw) ~= "string" then
				return nil
			end
			return {id = raw}
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded return member witness", diags)
	}
}

func TestReturnContractAcceptsGuardedObjectLiteralAnyPathMember(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {id: string}
		function make(raw: any): Point?
			if type(raw.id) ~= "string" then
				return nil
			end
			return {id = raw.id}
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded return path member witness", diags)
	}
}

func TestReturnContractAcceptsGuardedPathMemberThroughNestedUnionReturn(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Task = {kind: "task", id: string}
		type Err = {code: string, message: string}
		type Result = {ok: true, value: Task} | {ok: false, error: Err}
		function make(raw: any): Result
			if type(raw.kind) ~= "string" then
				return {ok = false, error = {code = "kind", message = "bad"}}
			end
			if type(raw.id) ~= "string" then
				return {ok = false, error = {code = "id", message = "bad"}}
			end
			if raw.kind == "task" then
				return {
					ok = true,
					value = {
						kind = "task",
						id = raw.id,
					},
				}
			end
			return {ok = false, error = {code = "unknown", message = raw.kind}}
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded nested-union return members", diags)
	}
}

func TestCallParamObligationRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		type Sink = {send: (p: Point) -> ()}
		function wrap(sink: Sink, payload)
			sink.send(payload)
		end
		local sink: Sink = {send = function(p: Point) end}
		local raw: any = nil
		wrap(sink, {id = raw})
	`)
	for _, d := range diags {
		if d.Code == CodeDirectCallArgType &&
			strings.Contains(d.Message, "argument 2 expected string, got any") {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want call-site obligation member mismatch", diags)
}

func TestOrdinaryAssignmentRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		type Box = {p: Point}
		local raw: any = nil
		local box: Box = {p = {id = "ok"}}
		box.p = {id = raw}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "any") || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want any-to-string ordinary assignment member mismatch", d)
	}
}

func TestDirectCallAcceptsGuardedObjectLiteralAnyMember(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {id: string}
		function take(p: Point)
		end
		function validate(raw: any)
			if type(raw) == "string" then
				take({id = raw})
			end
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded concrete witness", diags)
	}
}
