package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLowerResolvedAllocationPreservesOneSharedIdentity(t *testing.T) {
	reg := standard.Registry()
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, _ := effectlowering.StaticSignatureAllocationTemplate(sig)
	op, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 41, Template: template.Root, Ordinal: 7,
	}, template)
	id := identity.ManifestAllocation("stdlib.table.create:return:0", 7)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, template.Objects[0].Type), template.Objects[0].Type)
	value = product.Set(reg, value, identity.Key, identity.Singleton(id))
	resolution, ok := LowerResolvedEffects(reg, []transformer.ResolvedEffect{{
		Kind:       transformer.EffectAllocationTemplate,
		Allocation: transformer.ResolvedAllocationTemplate{Site: op.Site(), Template: op, Result: value},
	}})
	got := resolution.Summary
	if !ok || len(got.HeapTableObjects) != 1 || len(got.FreshHeapAllocations) != 1 || got.HeapKeySpace == nil {
		t.Fatalf("allocation Summary = %#v/%v", got, ok)
	}
	if got.FreshHeapAllocations[0].ID != id {
		t.Fatalf("fresh identity = %v, want %v", got.FreshHeapAllocations[0].ID, id)
	}
	object, exists := got.HeapTableObjects[id]
	if !exists || !product.Equal(reg, object.Root(), value) {
		t.Fatalf("heap object/result identity diverged: %#v/%v", object, exists)
	}
}
