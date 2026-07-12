package callresult

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
