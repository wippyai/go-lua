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

// TestZZSagaIdxProbe pins the array-index-in-bounds presence over a backward
// `for i = #saga.compensations, 1, -1` loop reading a field-path container: the
// element local `comp = saga.compensations[i]` is provably in range, so it reads
// the NON-optional element union at every reachable point (and discriminant
// narrowing then refines it on each branch arm). The element local is a `local`
// declaration, so its per-iteration binding overwrites the loop-carried prior
// value rather than LUB-joining it back to its optional form.
func TestZZSagaIdxProbe(t *testing.T) {
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
	tr := transfer.New(in, nil, nil, nil, nil, nil, nil, nil)
	fs := equation.NewBuilder(in.Graph, in.Scope.NumParams(), tr).Solve()

	// At no reachable point may the element local carry nil: a provably in-bounds
	// sequence read is present. A surviving nil here is the array-index-presence
	// regression (or a monotonic re-assignment join re-admitting the optional).
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
		t.Fatal("element local never observed; probe did not exercise the read")
	}
}

// TestZZSagaIdxOutOfBoundsStaysOptional pins the SOUNDNESS half: an arbitrary
// literal index read `arr[3]` with no proven length floor (no in-range loop, no
// guard) keeps its optional element. The array-index-presence refinement fires
// ONLY when the index is provably in bounds; an out-of-bounds-capable read must
// still observe nil.
func TestZZSagaIdxOutOfBoundsStaysOptional(t *testing.T) {
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
	tr := transfer.New(in, nil, nil, nil, nil, nil, nil, nil)
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
		t.Fatal("element local never observed; probe did not exercise the read")
	}
}

// typeMentionsKind reports whether t is a union (optionally with nil) whose
// non-nil members include a record with a kind: "<lit>" field, the shape the
// element union carries.
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
