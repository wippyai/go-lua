package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSagaIndexPresenceRegression(t *testing.T) {
	a1 := typ.NewRecord().Field("kind", typ.LiteralString("release")).Field("token", typ.String).Build()
	a2 := typ.NewRecord().Field("kind", typ.LiteralString("refund")).Field("pay", typ.String).Build()
	elem := typ.NewUnion(a1, a2)
	arrT := typ.NewArray(elem)
	sagaT := typ.NewRecord().Field("compensations", arrT).Build()

	resolve := func(_ ast.TypeExpr, _ *scope.State) typ.Type { return sagaT }

	body := `
for i = #saga.compensations, 1, -1 do
	local comp = saga.compensations[i]
	local n: string
	if comp.kind == "release" then
		n = comp.token
	else
		n = comp.pay
	end
end
`
	stmts, err := parse.ParseString(body, "saga.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	paramType := &ast.PrimitiveTypeExpr{Name: "Saga"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"saga"}, Types: []ast.TypeExpr{paramType}},
		Stmts:   stmts,
	}
	in := input.BuildFromFunction(fn, resolve, nil)
	if in.Graph == nil {
		t.Fatal("no graph")
	}
	tr := transfer.New(in, transfer.Config{})
	fs := equation.NewBuilder(in.Graph, in.Scope.NumParams(), tr).Solve()

	sawElement := false
	for _, ps := range fs.InPoints {
		for _, av := range ps.Env {
			if av.IsZero() {
				continue
			}
			pv := av.ProjectValue()
			if !typeMentionsKind(pv, "release") || !typeMentionsKind(pv, "refund") {
				continue
			}
			sawElement = true
			if _, optional := typ.SplitNilableFieldType(pv); optional {
				t.Fatalf("in-bounds array element read carries nil: %v", pv)
			}
		}
	}
	if !sawElement {
		t.Fatal("element local never observed; regression did not exercise the read")
	}
}

func TestSagaIndexOutOfBoundsStaysOptional(t *testing.T) {
	a1 := typ.NewRecord().Field("kind", typ.LiteralString("release")).Field("token", typ.String).Build()
	a2 := typ.NewRecord().Field("kind", typ.LiteralString("refund")).Field("pay", typ.String).Build()
	elem := typ.NewUnion(a1, a2)
	arrT := typ.NewArray(elem)
	sagaT := typ.NewRecord().Field("compensations", arrT).Build()
	resolve := func(_ ast.TypeExpr, _ *scope.State) typ.Type { return sagaT }

	body := `
local comp = saga.compensations[3]
local k = comp.kind
`
	stmts, err := parse.ParseString(body, "saga.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	paramType := &ast.PrimitiveTypeExpr{Name: "Saga"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"saga"}, Types: []ast.TypeExpr{paramType}},
		Stmts:   stmts,
	}
	in := input.BuildFromFunction(fn, resolve, nil)
	if in.Graph == nil {
		t.Fatal("no graph")
	}
	tr := transfer.New(in, transfer.Config{})
	fs := equation.NewBuilder(in.Graph, in.Scope.NumParams(), tr).Solve()

	saw := false
	for _, ps := range fs.InPoints {
		for _, av := range ps.Env {
			if av.IsZero() {
				continue
			}
			pv := av.ProjectValue()
			if !typeMentionsKind(pv, "release") || !typeMentionsKind(pv, "refund") {
				continue
			}
			saw = true
			if _, optional := typ.SplitNilableFieldType(pv); !optional {
				t.Fatalf("out-of-bounds-capable index read must stay optional, got %v", pv)
			}
		}
	}
	if !saw {
		t.Fatal("element local never observed; regression did not exercise the read")
	}
}

func typeMentionsKind(t typ.Type, lit string) bool {
	u, ok := t.(*typ.Union)
	if !ok {
		return false
	}
	for _, m := range u.Members {
		rec, ok := m.(*typ.Record)
		if !ok {
			continue
		}
		f := rec.GetField("kind")
		if f == nil {
			continue
		}
		l, ok := f.Type.(*typ.Literal)
		if ok && l.Value == lit {
			return true
		}
	}
	return false
}
