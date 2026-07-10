package diagnostics

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestWalkExprChildrenVisitsDirectChildrenInStableOrder(t *testing.T) {
	callee := &ast.IdentExpr{Value: "call"}
	receiver := &ast.IdentExpr{Value: "recv"}
	firstArg := &ast.StringExpr{Value: "a"}
	secondArg := &ast.StringExpr{Value: "b"}
	var got []string

	walkExprChildren(&ast.FuncCallExpr{
		Func:     callee,
		Receiver: receiver,
		Args:     []ast.Expr{firstArg, secondArg},
	}, func(expr ast.Expr) {
		got = append(got, exprName(expr))
	})

	want := []string{"call", "recv", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("walkExprChildren order = %#v, want %#v", got, want)
	}
}

func TestWalkExprChildrenVisitsBracketKeysButNotDotKeys(t *testing.T) {
	object := &ast.IdentExpr{Value: "object"}
	dotKey := &ast.IdentExpr{Value: "dot"}
	bracketKey := &ast.StringExpr{Value: "bracket"}
	var dotChildren []string
	var bracketChildren []string

	walkExprChildren(&ast.AttrGetExpr{
		Object:    object,
		Key:       dotKey,
		KeySyntax: ast.AttrKeyDot,
	}, func(expr ast.Expr) {
		dotChildren = append(dotChildren, exprName(expr))
	})
	walkExprChildren(&ast.AttrGetExpr{
		Object:    object,
		Key:       bracketKey,
		KeySyntax: ast.AttrKeyIndex,
	}, func(expr ast.Expr) {
		bracketChildren = append(bracketChildren, exprName(expr))
	})

	if want := []string{"object"}; !reflect.DeepEqual(dotChildren, want) {
		t.Fatalf("dot children = %#v, want %#v", dotChildren, want)
	}
	if want := []string{"object", "bracket"}; !reflect.DeepEqual(bracketChildren, want) {
		t.Fatalf("bracket children = %#v, want %#v", bracketChildren, want)
	}
}

func TestWalkExprChildrenSkipsNilTableFields(t *testing.T) {
	key := &ast.StringExpr{Value: "k"}
	value := &ast.StringExpr{Value: "v"}
	var got []string

	walkExprChildren(&ast.TableExpr{
		Fields: []*ast.Field{
			nil,
			{Key: key, KeySyntax: ast.AttrKeyIndex, Value: value},
		},
	}, func(expr ast.Expr) {
		got = append(got, exprName(expr))
	})

	want := []string{"k", "v"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("table children = %#v, want %#v", got, want)
	}
}

func exprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.StringExpr:
		return e.Value
	default:
		return ""
	}
}
