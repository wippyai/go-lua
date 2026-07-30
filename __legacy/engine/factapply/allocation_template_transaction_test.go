package factapply

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestAllocationTemplateTransactionUsesOneClosedIdentityGraph(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	sig, ok := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	if !ok {
		t.Fatal("table.create signature")
	}
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create allocation template")
	}
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-transaction")), 1)
	exact := make(map[signature.AllocationTemplateID]identity.Term, len(template.Objects))
	for index, object := range template.Objects {
		templateIdentity := identity.ManifestAllocationTemplate(body, 1, uint32(index+1))
		exact[object.ID] = identity.ConcreteTerm(identity.RootBoundaryAllocation(templateIdentity))
	}
	materialized, ok := effectlowering.MaterializeStaticAllocation(reg, nil, keys, cfg.Point(7), template, exact)
	if !ok {
		t.Fatal("materialize allocation")
	}
	transaction, err := NewAllocationTemplateTransaction(reg, allocationMaterializationForTest(materialized))
	if err != nil {
		t.Fatal(err)
	}
	if !transaction.Valid(reg) || transaction.Len() != len(template.Objects) {
		t.Fatalf("allocation transaction = %#v", transaction)
	}
	resultID, ok := identityvalue.ExactID(reg, transaction.Result())
	if !ok {
		t.Fatal("transaction result has no exact identity")
	}

	out, err := ApplyAllocationTemplateTransaction(context.Background(), reg, transaction, state.Domain(reg).Bottom())
	if err != nil {
		t.Fatal(err)
	}
	seenRoot := false
	for index := 0; index < transaction.Len(); index++ {
		fresh, ok := transaction.Fresh(index)
		if !ok {
			t.Fatalf("fresh[%d]", index)
		}
		object, ok := transaction.Object(fresh.ID)
		if !ok {
			t.Fatalf("object[%v]", fresh.ID)
		}
		objectID, exact := identityvalue.ExactID(reg, object.Root())
		if !exact || objectID != fresh.ID {
			t.Fatalf("object root identity = %v/%v, want %v", objectID, exact, fresh.ID)
		}
		if !product.Equal(reg, out.ReadHeapTableObject(reg, fresh.ID).Root(), object.Root()) || out.ReadPlacement(fresh.ID) != fresh.Placement {
			t.Fatalf("executed object/placement diverged for %v", fresh.ID)
		}
		seenRoot = seenRoot || fresh.ID == resultID
	}
	if !seenRoot {
		t.Fatal("return identity is absent from the fresh graph")
	}
}

func TestAllocationTemplateTransactionCancellationRollsBack(t *testing.T) {
	reg := standard.Registry()
	transaction := allocationTransactionForTest(t, reg)
	input := state.Domain(reg).Bottom()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := ApplyAllocationTemplateTransaction(ctx, reg, transaction, input)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if !state.Domain(reg).Equal(got, input) {
		t.Fatal("canceled allocation transaction published a prefix")
	}
}

func TestAllocationTemplateFactorLawMatchesCanonicalPointwiseJoin(t *testing.T) {
	reg := standard.Registry()
	transaction := allocationTransactionForTest(t, reg)
	input := state.Domain(reg).Bottom()
	legacyKey, ok := heapidentity.StaticMemberSuffixKey(transaction.keys, []segment.Segment{{Kind: segment.SegmentField, Name: "legacy"}})
	if !ok {
		t.Fatal("legacy static key")
	}
	for index := 0; index < transaction.Len(); index++ {
		fresh, _ := transaction.Fresh(index)
		object, _ := transaction.Object(fresh.ID)
		prior := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: object.Root(), StaticMembers: map[keyspace.Key]product.Value{legacyKey: product.Top()},
		})
		input = input.WriteHeapTableObject(reg, fresh.ID, prior).WritePlacement(fresh.ID, placement.Stack)
	}

	want := input
	objectDomain := heapidentity.ObjectDomain(reg)
	for index := 0; index < transaction.Len(); index++ {
		fresh, _ := transaction.Fresh(index)
		object, _ := transaction.Object(fresh.ID)
		want = want.WriteHeapTableObject(reg, fresh.ID, objectDomain.Join(want.ReadHeapTableObject(reg, fresh.ID), object))
		want = want.WritePlacement(fresh.ID, placement.Join(want.ReadPlacement(fresh.ID), fresh.Placement))
	}
	got, err := ApplyAllocationTemplateTransaction(context.Background(), reg, transaction, input)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Domain(reg).Equal(got, want) {
		t.Fatal("factor-native allocation diverged from the canonical pointwise object/placement join")
	}
}

func allocationTransactionForTest(t *testing.T, registry *axis.Registry) AllocationTemplateTransaction {
	t.Helper()
	keys := keyspace.New()
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, _ := effectlowering.StaticSignatureAllocationTemplate(sig)
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-transaction-cancel")), 1)
	exact := map[signature.AllocationTemplateID]identity.Term{}
	for index, object := range template.Objects {
		templateIdentity := identity.ManifestAllocationTemplate(body, 1, uint32(index+1))
		exact[object.ID] = identity.ConcreteTerm(identity.RootBoundaryAllocation(templateIdentity))
	}
	materialized, ok := effectlowering.MaterializeStaticAllocation(registry, nil, keys, cfg.Point(7), template, exact)
	if !ok {
		t.Fatal("materialize allocation")
	}
	transaction, err := NewAllocationTemplateTransaction(registry, allocationMaterializationForTest(materialized))
	if err != nil {
		t.Fatal(err)
	}
	return transaction
}

func allocationMaterializationForTest(materialized effectlowering.MaterializedStaticAllocation) AllocationTemplateMaterialization {
	return AllocationTemplateMaterialization{
		Result: materialized.Result, Objects: materialized.Objects, Placements: materialized.Placements, KeySpace: materialized.KeySpace,
	}
}
