package readmodel

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestValueTypeWitnessPresentProjectsConcreteType(t *testing.T) {
	reg := standard.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = typevalue.WithWitness(reg, value, typ.NewOptional(typ.String))

	got, ok := New(&body.Result{}).ValueType(value)
	if ok {
		t.Fatalf("ValueType with nil registry result returned %v, want false", got)
	}

	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	got, ok = New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestValueTypeAbsentProjectsNil(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	got, ok := New(result).ValueType(product.Absent(reg))
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.Nil)
}

func TestRuntimeKindProjection(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	reader := New(result)
	for _, tc := range []struct {
		name string
		kind runtimekind.Tag
		want typ.Type
	}{
		{name: "nil", kind: runtimekind.Nil, want: typ.Nil},
		{name: "boolean", kind: runtimekind.Boolean, want: typ.Boolean},
		{name: "number", kind: runtimekind.Number, want: typ.Number},
		{name: "string", kind: runtimekind.String, want: typ.String},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(tc.kind))
			got, ok := reader.ValueType(value)
			if !ok {
				t.Fatalf("ValueType returned false")
			}
			assertSameType(t, got, tc.want)
		})
	}

	for _, tc := range []struct {
		name string
		kind runtimekind.Tag
		want kind.Kind
	}{
		{name: "table", kind: runtimekind.Table, want: kind.Map},
		{name: "function", kind: runtimekind.Function, want: kind.Function},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(tc.kind))
			got, ok := reader.RefineDeclaredType(typ.Unknown, value)
			if !ok {
				t.Fatalf("RefineDeclaredType returned false")
			}
			if got.Kind() != tc.want {
				t.Fatalf("RefineDeclaredType kind = %s, want %s (%v)", got.Kind(), tc.want, got)
			}
		})
	}
}

func TestRefineDeclaredTypeOptionalByPresentEvidence(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)

	got, ok := New(result).RefineDeclaredType(typ.NewOptional(typ.String), value)
	if !ok {
		t.Fatalf("RefineDeclaredType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestSourceTypeReadsCallSourceThroughBoundary(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local v = Point(data)
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	assign := stmts[2].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	if fact.Source.Kind != sourceprovenance.SourceCall || !fact.Source.HasCallPoint {
		t.Fatalf("local source = %#v, want call source with call point", fact.Source)
	}
	if !fact.Source.Final || !fact.Source.Expanded || fact.Source.Adjusted || fact.Source.OpenTail {
		t.Fatalf("call source shape = final:%v expanded:%v adjusted:%v openTail:%v, want expanded final call source",
			fact.Source.Final, fact.Source.Expanded, fact.Source.Adjusted, fact.Source.OpenTail)
	}

	got, ok := New(result).SourceType(point, fact.Source)
	if !ok {
		t.Fatalf("SourceType returned false")
	}
	if got.Kind() != kind.Record {
		t.Fatalf("SourceType kind = %s, want record (%v)", got.Kind(), got)
	}
}

func assertSameType(t *testing.T, got, want typ.Type) {
	t.Helper()
	if !typ.SameNodeOrAcyclicEqual(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}

func requireLocalAssignment(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) (cfg.Point, semantics.LocalAssignmentFact) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point, fact
		}
	}
	t.Fatalf("missing local assignment for stmt %p index %d", stmt, index)
	return 0, semantics.LocalAssignmentFact{}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(strings.TrimSpace(src), "readmodel_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}
