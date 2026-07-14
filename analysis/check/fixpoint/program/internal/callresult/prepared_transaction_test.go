package callresult

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPreparedSummaryTransactionMatchesSnapshotProvider(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(8801)
	key := summary.DefaultSummaryKey(ref.FromSymbol(callee))
	sum := summary.Normalize(reg, summary.Summary{Returns: []product.Value{typevalue.LiteralString(reg, "ok")}})
	site := factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: callee}).View()
	config := ProviderConfig{}
	directConfig := config
	directConfig.Summaries = summary.NewSnapshotOwnedNormalized(reg, summary.EntrySummary{Key: key, Summary: sum})
	directConfig.KeyFor = ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key})
	ctx := transfer.NodeContext{Registry: reg}
	want := OutcomeProvider(directConfig)(ctx, site, state.State{}, nil)
	got := NewPreparedSummaryTransaction(config).Apply(ctx, site, state.State{}, nil, sum, nil, false)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("prepared transaction differs\nwant=%#v\n got=%#v", want, got)
	}
}

func TestPreparedSummaryTransactionRekeysProducerHeapIntoCallerKeySpace(t *testing.T) {
	reg := standard.Registry()
	producerKeys := keyspace.New()
	callerKeys := keyspace.New()
	if _, ok := callerKeys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "padding"}}); !ok {
		t.Fatal("failed to build caller padding key")
	}
	producerMember, ok := producerKeys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("failed to build producer member key")
	}
	callerMember, ok := callerKeys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("failed to build caller member key")
	}
	if producerMember.Segs == callerMember.Segs {
		t.Fatalf("test setup did not produce conflicting dense ids: producer=%v caller=%v", producerMember.Segs, callerMember.Segs)
	}

	tableID := identity.ID{Kind: "test.table", Site: "prepared-transaction-rekey", Index: 1}
	memberValue := typevalue.LiteralString(reg, "value")
	sum := summary.Summary{
		HeapKeySpace: producerKeys,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			tableID: heapidentity.NewOwnedStaticTableObject(product.Top(), map[keyspace.Key]product.Value{
				producerMember: memberValue,
			}),
		},
	}
	tx := NewPreparedSummaryTransaction(ProviderConfig{KeySpace: callerKeys})
	out := tx.Apply(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil, sum, nil, false)

	object, ok := out.HeapTableObjects[tableID]
	if !ok {
		t.Fatalf("outcome dropped heap object %v", tableID)
	}
	got, ok := object.StaticMember(callerMember)
	if !ok || !product.Equal(reg, got, memberValue) {
		t.Fatalf("caller-owned .member = (%#v, %v), want %#v", got, ok, memberValue)
	}
	for key := range object.StaticMembers() {
		if got := callerKeys.FormatReadOnly(key); got != ".member" {
			t.Fatalf("outcome retained foreign heap key %v formatted as %q", key, got)
		}
	}
}

func TestPreparedSummaryTransactionFailsStopOnCorruptHeapProvenance(t *testing.T) {
	reg := standard.Registry()
	ownerKeys, foreignKeys, callerKeys := keyspace.New(), keyspace.New(), keyspace.New()
	foreignMember, ok := foreignKeys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("foreign member key failed")
	}
	tableID := identity.ID{Kind: "table", Site: "corrupt-transaction", Index: 1}
	corrupt := summary.Summary{
		HeapKeySpace: ownerKeys,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			tableID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{foreignMember: product.Top()},
			}),
		},
	}
	tx := NewPreparedSummaryTransaction(ProviderConfig{KeySpace: callerKeys})
	published := false
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = tx.Apply(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil, corrupt, nil, false)
		published = true
	}()
	if recovered == nil {
		t.Fatal("corrupt transaction did not fail stop")
	}
	if published {
		t.Fatal("corrupt transaction published an outcome")
	}
}

func BenchmarkPreparedSummaryTransactionApply(b *testing.B) {
	reg := standard.Registry()
	callee := symbol.ID(8802)
	sum := summary.Normalize(reg, summary.Summary{Returns: []product.Value{typevalue.LiteralString(reg, "ok")}})
	site := factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: callee}).View()
	tx := NewPreparedSummaryTransaction(ProviderConfig{})
	ctx := transfer.NodeContext{Registry: reg}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := tx.Apply(ctx, site, state.State{}, nil, sum, nil, false)
		if len(out.Results) != 1 {
			b.Fatal("missing result")
		}
	}
}
