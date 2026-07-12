package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDynamicReadValueIsCanonicalAndRebasesAcrossArenas(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	calleeShape, callerShape := Shape{Params: 2}, Shape{Params: 3}
	table := callee.Root(Root{Kind: RootParam, Index: 0})
	key := callee.Root(Root{Kind: RootParam, Index: 1})
	path := callee.Path(Root{Kind: RootParam, Index: 0}, segment.Segment{Kind: segment.SegmentField, Name: "references"})
	read := callee.DynamicReadValue(table, path, key)
	if read == 0 || read != callee.DynamicReadValue(table, path, key) {
		t.Fatal("DynamicRead was not hash-consed")
	}
	callerTable := caller.Root(Root{Kind: RootParam, Index: 1})
	callerKey := caller.Root(Root{Kind: RootParam, Index: 2})
	callerPath := caller.Path(Root{Kind: RootParam, Index: 1}, segment.Segment{Kind: segment.SegmentField, Name: "owner"})
	bindings, err := NewTermRootBindings(calleeShape, callerShape, []ValueTerm{callerTable, callerKey}, []PathTerm{callerPath, caller.Path(Root{Kind: RootParam, Index: 2})})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{read}, Guards: []Guard{callee.Falsy(read)}})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := caller.Path(Root{Kind: RootParam, Index: 1},
		segment.Segment{Kind: segment.SegmentField, Name: "owner"},
		segment.Segment{Kind: segment.SegmentField, Name: "references"})
	want := caller.DynamicReadValue(callerTable, wantPath, callerKey)
	if len(rebased.Values) != 1 || rebased.Values[0] != want || len(rebased.Guards) != 1 || rebased.Guards[0] != caller.Falsy(want) {
		t.Fatalf("rebased DynamicRead = %#v, want canonical value %d", rebased, want)
	}
}

func TestDynamicReadTableValueIsDistinctRebasesAndUsesStaticFallback(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 2}
	table := callee.Root(Root{Kind: RootParam, Index: 0})
	key := callee.Root(Root{Kind: RootParam, Index: 1})
	path := callee.Path(Root{Kind: RootParam, Index: 0})
	fallback := callee.Constant(typevalue.FromType(reg, typ.MaterializeOptional(typ.String)))
	direct := callee.DynamicReadTableValueOr(table, path, key, fallback)
	if direct == 0 || direct != callee.DynamicReadTableValueOr(table, path, key, fallback) {
		t.Fatal("direct DynamicRead was not hash-consed")
	}
	if direct == callee.DynamicReadValue(table, path, key) {
		t.Fatal("direct-table and owner-path DynamicRead terms collapsed")
	}
	callerTable := caller.Root(Root{Kind: RootParam, Index: 0})
	callerKey := caller.Root(Root{Kind: RootParam, Index: 1})
	callerPath := caller.Path(Root{Kind: RootParam, Index: 0})
	bindings, err := NewTermRootBindings(shape, shape, []ValueTerm{callerTable, callerKey}, []PathTerm{callerPath, caller.Path(Root{Kind: RootParam, Index: 1})})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{direct}})
	if err != nil {
		t.Fatal(err)
	}
	want := caller.DynamicReadTableValueOr(callerTable, callerPath, callerKey, caller.Constant(typevalue.FromType(reg, typ.MaterializeOptional(typ.String))))
	if len(rebased.Values) != 1 || rebased.Values[0] != want {
		t.Fatalf("rebased direct DynamicRead = %#v, want %d", rebased.Values, want)
	}
	cursor, _ := NewBindingCursor(shape,
		[]product.Value{product.Top(), typevalue.LiteralString(reg, "node")},
		[]pathdom.Path{{Root: "graph.references"}, {Root: "name"}})
	value, ok := caller.evalValue(want, cursor, SpecializationContext{
		DynamicTableRead: func(pathdom.Path, product.Value, product.Value) (product.Value, bool) {
			return product.Value{}, false
		},
	})
	if !ok || !product.Equal(reg, value, typevalue.FromType(reg, typ.MaterializeOptional(typ.String))) {
		t.Fatalf("direct DynamicRead fallback = %#v/%v", value, ok)
	}
}

func TestDynamicReadResolveReferenceDifferential(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 2, Results: 1}
	builder, certificate := emptyBuilder(t, reg, shape, nil)
	a := builder.Arena()
	tableRoot := Root{Kind: RootParam, Index: 0}
	keyRoot := Root{Kind: RootParam, Index: 1}
	table := a.Root(tableRoot)
	key := a.Root(keyRoot)
	tablePath := a.Path(tableRoot)
	read := a.DynamicReadValue(table, tablePath, key)
	badName := a.Constant(typevalue.LiteralString(reg, "bad-name"))
	missing := a.Constant(typevalue.LiteralString(reg, "missing"))
	rows := []Row{
		{Guard: a.Falsy(key), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: badName}}},
		{Guard: a.And(a.Truthy(key), a.Falsy(read)), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: missing}}},
		{Guard: a.And(a.Truthy(key), a.Truthy(read)), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: read}}},
	}
	relation, err := builder.Build(certificate, rows)
	if err != nil {
		t.Fatal(err)
	}

	ks := keyspace.New()
	id := identity.ID{Kind: "table", Site: "resolve-reference", Index: 3}
	tableValue := identityvalue.Present(reg, id)
	static := func(name string) keyspace.Key {
		key, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: name}})
		if !ok {
			t.Fatalf("static key %q", name)
		}
		return key
	}
	trueNode := typevalue.LiteralString(reg, "node")
	falseNode := typevalue.LiteralBool(reg, false)
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: tableValue,
		StaticMembers: map[keyspace.Key]product.Value{
			static("true-node"):  trueNode,
			static("false-node"): falseNode,
		},
		StableShape: true,
	})
	in := state.State{}.WriteHeapTableObject(reg, id, object)
	before := in.Snapshot()
	basePath := pathdom.Path{Root: "self.references"}
	context := SpecializationContext{DynamicRead: func(tablePath pathdom.Path, table, key product.Value) (product.Value, bool) {
		return sourcevalue.ReadBoundDynamicIndexValue(reg, typevalue.NewCache(), ks, nil, 0, tablePath, table, key, in)
	}}

	tests := []struct {
		name string
		key  product.Value
		want product.Value
	}{
		{name: "falsy name", key: typevalue.Nil(reg), want: typevalue.LiteralString(reg, "bad-name")},
		{name: "truthy lookup", key: typevalue.LiteralString(reg, "true-node"), want: trueNode},
		{name: "false is lookup failure", key: typevalue.LiteralString(reg, "false-node"), want: typevalue.LiteralString(reg, "missing")},
		{name: "absent lookup", key: typevalue.LiteralString(reg, "absent-node"), want: typevalue.LiteralString(reg, "missing")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor, err := NewBindingCursor(shape,
				[]product.Value{tableValue, tc.key, product.Bottom(reg)},
				[]pathdom.Path{basePath, {Root: "name"}, {Root: "result"}})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := relation.SpecializeWithContext(cursor, nil, context)
			if !ok || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], tc.want) {
				t.Fatalf("specialized return = %#v/%v, want %#v", got.Returns, ok, tc.want)
			}
		})
	}
	if !state.Domain(reg).Equal(in, before) {
		t.Fatal("DynamicRead specialization mutated one of the 17 caller State lanes")
	}

	// An abstract string query must retain both feasible lookup rows, exactly as
	// the concrete may-semantics does.
	abstractString := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	cursor, _ := NewBindingCursor(shape,
		[]product.Value{tableValue, abstractString, product.Bottom(reg)},
		[]pathdom.Path{basePath, {Root: "name"}, {Root: "result"}})
	if _, ok := relation.Specialize(cursor, nil, nil); ok {
		t.Fatal("legacy specialization did not fail closed without DynamicRead resolver")
	}
}
