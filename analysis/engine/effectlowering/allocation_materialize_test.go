package effectlowering

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestMaterializeStaticAllocationMatchesCanonicalProvider(t *testing.T) {
	reg := standard.Registry()
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, _ := StaticSignatureAllocationTemplate(sig)
	ks := keyspace.New()
	point := cfg.Point(4101)
	materialized, ok := MaterializeStaticAllocation(reg, nil, ks, point, template)
	if !ok {
		t.Fatal("static allocation materialization rejected")
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{"table.create": sig}, NameFor: staticName("table.create"), KeySpace: ks,
	})
	oracle := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	if len(oracle.Results) != 1 || !product.Equal(reg, oracle.Results[0].Value, materialized.Result) {
		t.Fatalf("provider/materialized result differ: %#v / %#v", oracle.Results, materialized.Result)
	}
	if len(oracle.HeapTableObjects) != len(materialized.Objects) || len(oracle.Placements) != len(materialized.Placements) {
		t.Fatalf("provider/materialized heap cardinality differs: %d/%d %d/%d", len(oracle.HeapTableObjects), len(materialized.Objects), len(oracle.Placements), len(materialized.Placements))
	}
	for id, object := range oracle.HeapTableObjects {
		got, exists := materialized.Objects[id]
		if !exists || !product.Equal(reg, got.Root(), object.Root()) || materialized.Placements[id] != oracle.Placements[id] {
			t.Fatalf("provider/materialized allocation %v differs", id)
		}
	}
}
