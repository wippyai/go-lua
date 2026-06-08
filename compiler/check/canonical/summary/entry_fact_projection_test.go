package summary_test

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDirectCallEntryFactsProjectsLengthBoundsToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(10), "graph")
	nodeOrder := source.Field("node_order")
	num := numeric.NewState()
	num.ApplyLenGeConst(flow.SymbolPathKey(nodeOrder.Symbol, nodeOrder.Segments), 1)

	callee := summary.FuncRef{GraphID: 7}
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
	}
	got := projection.DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:      callee,
		Call:        call,
		KeyPresence: flow.KeyPresenceFacts{},
		Num:         num,
		IndexWrites: flow.IndexWriteAdmissionFacts{},
	}).Facts

	bounds := got.LengthLowerBounds()
	if len(bounds) != 1 {
		t.Fatalf("length bounds = %#v, want one projected bound", bounds)
	}
	want := flow.BoundaryPath{
		Kind:     flow.BoundaryPathParam,
		Index:    0,
		Segments: nodeOrder.Segments,
	}
	if !boundaryPathEqualForTest(bounds[0].Target, want) || bounds[0].Lower != 1 {
		t.Fatalf("length bound = %#v, want %#v >= 1", bounds[0], want)
	}
}

func TestDirectCallEntryFactsProjectsIndexWritesToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(11), "graph")
	table := source.Field("nodes")
	key := source.Field("last_node_id")
	value := product.FromType(typ.String)

	callee := summary.FuncRef{GraphID: 8}
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
	}
	indexWrites := flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
		Target:  flow.StablePathKey(table),
		KeyPath: flow.StablePathKey(key),
		Key:     product.FromType(typ.String),
		Value:   value,
	})
	got := projection.DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:      callee,
		Call:        call,
		KeyPresence: flow.KeyPresenceFacts{},
		IndexWrites: indexWrites,
	}).Facts

	writes := got.IndexWrites()
	if len(writes) != 1 {
		t.Fatalf("index writes = %#v, want one projected write", writes)
	}
	wantTable := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: table.Segments}
	wantKey := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: key.Segments}
	if !boundaryPathEqualForTest(writes[0].Table, wantTable) ||
		!boundaryPathEqualForTest(writes[0].Key, wantKey) ||
		!product.Domain.Equal(writes[0].Value, value) {
		t.Fatalf("index write = %#v, want table %#v key %#v value %s", writes[0], wantTable, wantKey, value.ProjectValue())
	}
}

func TestDirectCallEntryFactsProjectsKeyArrayValuesToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(14), "graph")
	array := source.Field("node_order")
	table := source.Field("edges")
	value := product.FromType(typ.NewRecord().
		Field("targets", typ.NewArray(typ.String)).
		Field("error_targets", typ.NewArray(typ.String)).
		Build())

	callee := summary.FuncRef{GraphID: 11}
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
	}
	got := projection.DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee: callee,
		Call:   call,
		KeyPresence: flow.KeyPresenceFacts{}.
			WithKeyArrayValueAddresses(
				entryFactStableAddress(t, array),
				entryFactStableAddress(t, table),
				value,
			),
	}).Facts

	facts := got.KeyArrayValues()
	if len(facts) != 1 {
		t.Fatalf("key-array values = %#v, want one projected proof", facts)
	}
	wantArray := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: array.Segments}
	wantTable := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: table.Segments}
	if !boundaryPathEqualForTest(facts[0].Array, wantArray) ||
		!boundaryPathEqualForTest(facts[0].Table, wantTable) ||
		!product.Domain.Equal(facts[0].Value, value) {
		t.Fatalf("key-array value = %#v, want array %#v table %#v value %s", facts[0], wantArray, wantTable, value.ProjectValue())
	}
}

func entryFactStableAddress(t *testing.T, path constraint.Path) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		t.Fatalf("stable address for path %s", path.Key())
	}
	return addr
}

func TestDirectCallEntryFactsProjectsStaticMembersToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(12), "graph")
	nodeOrder := source.Field("node_order")
	value := product.FromType(typ.NewArray(typ.String))
	addr, ok := flow.StableAddressOfPath(nodeOrder)
	if !ok {
		t.Fatalf("stable address for %s", nodeOrder.Key())
	}

	callee := summary.FuncRef{GraphID: 9}
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
	}
	got := projection.DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:        callee,
		Call:          call,
		StaticMembers: flow.StaticMemberFactsDomain.Top().WithAddress(addr, value),
	}).Facts

	members := got.StaticMembers()
	if len(members) != 1 {
		t.Fatalf("static members = %#v, want one projected member", members)
	}
	want := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: nodeOrder.Segments}
	if !boundaryPathEqualForTest(members[0].Target, want) || !product.Domain.Equal(members[0].Value, value) {
		t.Fatalf("static member = %#v, want target %#v value %s", members[0], want, value.ProjectValue())
	}
}

func TestDirectCallEntryFactsNormalizesStaticMembersAgainstRuntimeArgValue(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(13), "graph")
	edges := source.Field("edges")
	addr, ok := flow.StableAddressOfPath(edges)
	if !ok {
		t.Fatalf("stable address for %s", edges.Key())
	}
	stale := product.FromType(typ.NewRecord().Build())
	liveType := typ.NewMap(typ.String, typ.NewRecord().
		Field("targets", typ.NewFreshArray()).
		Field("error_targets", typ.NewFreshArray()).
		Build())
	liveRoot := product.FromType(typ.NewRecord().Field("edges", liveType).Build())

	callee := summary.FuncRef{GraphID: 10}
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
	}
	got := projection.DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:        callee,
		Call:          call,
		RuntimeValues: []product.AbstractValue{liveRoot},
		StaticMembers: flow.StaticMemberFactsDomain.Top().WithAddress(addr, stale),
	}).Facts

	members := got.StaticMembers()
	if len(members) != 1 {
		t.Fatalf("static members = %#v, want one projected member", members)
	}
	want := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: edges.Segments}
	if !boundaryPathEqualForTest(members[0].Target, want) || !typ.TypeEquals(members[0].Value.ProjectValue(), liveType) {
		t.Fatalf("static member = %#v, want normalized live product value %s", members[0], liveType)
	}
}

func TestAggregateEntryFactsKeepsOnlyFactsProvenByAllCallers(t *testing.T) {
	table := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	key := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "last_node_id"}}}
	proof := flow.BoundaryFactsOf(
		[]flow.BoundaryKeyPresenceFact{{Table: table, Key: key}},
		nil, nil, nil, nil, nil,
	)

	got := summary.AggregateEntryFacts(func(yield func(flow.BoundaryFacts)) {
		yield(proof)
		yield(proof)
	})
	if !got.HasKeyPresence(flow.BoundaryKeyPresenceFact{Table: table, Key: key}) {
		t.Fatalf("AggregateEntryFacts = %#v, want shared proof", got.KeyPresence())
	}

	weakened := summary.AggregateEntryFacts(func(yield func(flow.BoundaryFacts)) {
		yield(proof)
		yield(flow.BoundaryFactsDomain.Top())
	})
	if weakened.HasProof() {
		t.Fatalf("AggregateEntryFacts = %#v, want proof dropped when one caller is weak", weakened)
	}
}

func boundaryPathEqualForTest(a, b flow.BoundaryPath) bool {
	return a.Kind == b.Kind &&
		a.Index == b.Index &&
		slices.Equal(a.Segments, b.Segments)
}
