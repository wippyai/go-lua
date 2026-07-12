package summary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func TestHeapTableObjectsCompareAndDigestStructurallyAcrossKeySpaces(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "keyspace-determinism", Index: 1}
	leftKS, rightKS := keyspace.New(), keyspace.New()
	if _, ok := rightKS.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "padding"}}); !ok {
		t.Fatal("right padding suffix failed")
	}
	leftKey := mustRootlessSuffix(t, leftKS, "name")
	rightKey := mustRootlessSuffix(t, rightKS, "name")
	if leftKey.Segs == rightKey.Segs {
		t.Fatal("test setup did not shift dense segment ids")
	}
	makeSummary := func(ks *keyspace.KeySpace, key keyspace.Key) Summary {
		return Summary{
			HeapKeySpace: ks,
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{key: product.Top()},
				}),
			},
		}
	}
	left, right := makeSummary(leftKS, leftKey), makeSummary(rightKS, rightKey)
	if !EqualNormalized(reg, left, right) || !LessOrEq(reg, left, right) || !LessOrEq(reg, right, left) {
		t.Fatal("structurally equal heap objects differed across keyspaces")
	}
	if leftDigest, rightDigest := NormalizedPayloadDigest(reg, left), NormalizedPayloadDigest(reg, right); leftDigest != rightDigest {
		t.Fatalf("structurally equal heap digests differ: left=%d right=%d", leftDigest, rightDigest)
	}
}

func TestRekeyHeapTableObjectsImportsDynamicTableKeys(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "dynamic-rekey", Index: 1}
	from, to := keyspace.New(), keyspace.New()
	if _, ok := to.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "padding"}}); !ok {
		t.Fatal("target padding suffix failed")
	}
	fromTable := mustRootlessSuffix(t, from, "items")
	fact := dynamicindex.Fact{
		KeyPresence: presence.Present(), KeyValue: product.Top(), Value: product.Top(), Admission: dynamicindex.AdmissionAdmitted,
	}
	s := Summary{
		HeapKeySpace: from,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: product.Top(),
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
					{Table: fromTable, Site: "write"}: fact,
				},
			}),
		},
	}
	rekeyed := s.RekeyHeapTableObjects(to)
	object := rekeyed.HeapTableObjects[id]
	facts := object.DynamicIndexFacts()
	if len(facts) != 1 {
		t.Fatalf("dynamic facts after rekey = %#v", facts)
	}
	for key := range facts {
		if got := to.Format(key.Table); got != ".items" {
			t.Fatalf("dynamic table key after rekey = %q, want .items", got)
		}
	}
}

func TestSummaryDigestRejectsDenseHeapKeyWithoutProducingKeySpace(t *testing.T) {
	reg := mustRegistry(t)
	ks := keyspace.New()
	key := mustRootlessSuffix(t, ks, "name")
	s := Summary{HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		{Kind: "table", Site: "missing-keyspace", Index: 1}: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{key: product.Top()},
		}),
	}}
	defer func() {
		if recover() == nil {
			t.Fatal("digest accepted solve-local dense heap key without producing keyspace")
		}
	}()
	_ = NormalizedPayloadDigest(reg, s)
}
