package body

import (
	"os"
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
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func resolveReferenceFixture(t testing.TB) *Static {
	t.Helper()
	src, err := os.ReadFile("../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts := parseChunk(t, string(src))
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"uuid"}})
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func.Line() != 289 {
			continue
		}
		prepared, err := PrepareBoundFunction(origin.Func, bindings, Config{
			Registry: standard.Registry(), Globals: []string{"uuid"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		})
		if err != nil {
			t.Fatalf("PrepareBoundFunction(resolve_reference): %v", err)
		}
		return prepared
	}
	t.Fatal("FlowGraph.resolve_reference at line 289 is missing")
	return nil
}

func TestCallOutcomeContextDynamicReadUsesCanonicalBodyVisibility(t *testing.T) {
	prepared := resolveReferenceFixture(t)
	cache := typevalue.NewCache()
	ctx := prepared.callOutcomeContext(cache)
	if ctx.DynamicRead == nil || ctx.DynamicTableRead == nil {
		t.Fatal("call outcome dynamic-read seams missing")
	}
	params := prepared.operationPlan.BoundaryParams()
	if len(params) != 2 {
		t.Fatalf("resolve_reference params = %v", params)
	}
	reg := prepared.registry
	ks := prepared.visibility.KeySpace()
	selfID := identity.ID{Kind: "table", Site: "call-context-self", Index: 1}
	referencesID := identity.ID{Kind: "table", Site: "call-context-references", Index: 2}
	selfValue := identityvalue.Present(reg, selfID)
	referencesValue := identityvalue.Present(reg, referencesID)
	referencesKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "references"}})
	valueKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "node"}})
	wantValue := typevalue.LiteralString(reg, "node-17")
	in := state.State{}.
		WriteHeapTableObject(reg, selfID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: selfValue, StaticMembers: map[keyspace.Key]product.Value{referencesKey: referencesValue}, StableShape: true,
		})).
		WriteHeapTableObject(reg, referencesID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: referencesValue, StaticMembers: map[keyspace.Key]product.Value{valueKey: wantValue}, StableShape: true,
		}))
	tablePath := pathdom.NewPath(params[0], "self").Field("references")
	key := typevalue.LiteralString(reg, "node")
	node := transfer.NodeContext{Registry: reg, Graph: prepared.cfg.Graph, Point: prepared.cfg.Graph.Entry()}
	got, ok := ctx.DynamicRead(node, tablePath, selfValue, key, in)
	want, wantOK := sourcevalue.ReadBoundDynamicValue(sourcevalue.BoundDynamicRead{
		Registry: reg, TypeValues: cache, KeySpace: ks, Visibility: prepared.visibility, Point: node.Point,
		TablePath: tablePath, TableValue: selfValue, KeyValue: key, ValueInput: in, ProjectPath: true,
	})
	if ok != wantOK || !ok || !product.Equal(reg, got, want) || !product.Equal(reg, got, wantValue) {
		t.Fatalf("body dynamic read = %#v/%v, canonical = %#v/%v", got, ok, want, wantOK)
	}
	direct, directOK := ctx.DynamicTableRead(node, tablePath, referencesValue, key, in)
	directWant, directWantOK := sourcevalue.ReadBoundDynamicValue(sourcevalue.BoundDynamicRead{
		Registry: reg, TypeValues: cache, KeySpace: ks, Visibility: prepared.visibility, Point: node.Point,
		TablePath: tablePath, TableValue: referencesValue, KeyValue: key, ValueInput: in,
	})
	if directOK != directWantOK || !directOK || !product.Equal(reg, direct, directWant) {
		t.Fatalf("body direct-table read = %#v/%v, canonical = %#v/%v", direct, directOK, directWant, directWantOK)
	}
}
