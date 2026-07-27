package effectlowering

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestMaterializeStaticAllocationMatchesCanonicalProvider(t *testing.T) {
	reg := standard.Registry()
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, _ := StaticSignatureAllocationTemplate(sig)
	ks := keyspace.New()
	point := cfg.Point(4101)
	materialized, ok := MaterializeStaticAllocation(reg, nil, ks, point, template, nil)
	if !ok {
		t.Fatal("static allocation materialization rejected")
	}
	provider := testSignatureOutcomeProvider(t, SignatureOutcomeProviderConfig{
		Signatures: signatureMap{"table.create": sig}, NameFor: staticName("table.create"), KeySpace: ks,
	})

	oracle := testEvaluateCallOutcome(t, provider, transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), testCallOutcomeInput(t, provider, transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil))
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

func TestMaterializeStaticAllocationSubstitutesExactIdentitiesBeforeHeapConstruction(t *testing.T) {
	reg := standard.Registry()
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, _ := StaticSignatureAllocationTemplate(sig)
	exact := make(map[signature.AllocationTemplateID]identity.Term, len(template.Objects))
	want := make(map[signature.AllocationTemplateID]identity.ID, len(template.Objects))
	for index, object := range template.Objects {
		id := identity.ID{Kind: "test.exact-allocation", Site: "sealed-relation", Index: uint64(index + 1)}
		exact[object.ID] = identity.ConcreteTerm(id)
		want[object.ID] = id
	}
	materialized, ok := MaterializeStaticAllocation(reg, nil, keyspace.New(), 4101, template, exact)
	if !ok {
		t.Fatal("exact static allocation materialization rejected")
	}
	root, exactRoot := product.Get(reg, materialized.Result, identity.Key).ID()
	if !exactRoot || root != want[template.Root] {
		t.Fatalf("result identity = %#v/%v, want %#v", root, exactRoot, want[template.Root])
	}
	for _, id := range want {
		if _, ok := materialized.Objects[id]; !ok {
			t.Fatalf("exact heap identity %v was not substituted before construction", id)
		}
		if _, ok := materialized.Placements[id]; !ok {
			t.Fatalf("exact placement identity %v was not substituted before construction", id)
		}
	}
}
