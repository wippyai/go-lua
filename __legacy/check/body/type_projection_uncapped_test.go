package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestTypeProjectionIsDepthIndependent(t *testing.T) {
	record := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "present", Type: typ.String}}})
	var deep typ.Type = record
	for range 256 {
		deep = &typ.Annotated{Inner: deep}
	}
	if !ClosedRecordLacksField(deep, "missing") {
		t.Fatal("deep transparent wrappers changed the closed-record proof")
	}
	children := StaticTypeChildren(deep)
	if len(children) != 1 || children[0].Segment.Name != "present" {
		t.Fatalf("deep transparent wrappers changed static children: %#v", children)
	}
	if got := TransparentExpectedType(deep); got != record {
		t.Fatalf("deep transparent wrappers resolved to %T, want record", got)
	}
}

func TestSemanticExpressionChildWalkIsDepthIndependent(t *testing.T) {
	leaf := &ast.IdentExpr{Value: "value"}
	var root ast.Expr = leaf
	for range 256 {
		root = &ast.UnaryNotOpExpr{Expr: root}
	}
	stack := []ast.Expr{root}
	visited := 0
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		visited++
		children := adviceClaimChildren(current)
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	if visited != 257 {
		t.Fatalf("semantic expression walk visited %d nodes, want 257", visited)
	}
}

func TestClosedRecordProofHandlesRecursiveCycles(t *testing.T) {
	record := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "present", Type: typ.String}}})
	recursive := typ.NewRecursivePlaceholder("Record")
	recursive.SetBody(&typ.Union{Members: []typ.Type{recursive, record}})
	if !ClosedRecordLacksField(recursive, "missing") {
		t.Fatal("recursive union of closed records must prove the common absence")
	}
	if ClosedRecordLacksField(recursive, "present") {
		t.Fatal("recursive union must not prove absence of an existing field")
	}
}
