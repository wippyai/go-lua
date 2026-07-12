package body

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
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

func TestCallOutcomeContextDynamicReadUsesCanonicalBodyVisibility(t *testing.T) {
	prepared := resolveReferenceTransformerFixture(t)
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
	want, wantOK := sourcevalue.ReadBoundDynamicIndexValue(reg, cache, ks, prepared.visibility, node.Point, tablePath, selfValue, key, in)
	if ok != wantOK || !ok || !product.Equal(reg, got, want) || !product.Equal(reg, got, wantValue) {
		t.Fatalf("body dynamic read = %#v/%v, canonical = %#v/%v", got, ok, want, wantOK)
	}
	direct, directOK := ctx.DynamicTableRead(node, tablePath, referencesValue, key, in)
	directWant, directWantOK := sourcevalue.ReadBoundDynamicTableValue(reg, cache, ks, prepared.visibility, node.Point, tablePath, referencesValue, key, in)
	if directOK != directWantOK || !directOK || !product.Equal(reg, direct, directWant) {
		t.Fatalf("body direct-table read = %#v/%v, canonical = %#v/%v", direct, directOK, directWant, directWantOK)
	}
}
