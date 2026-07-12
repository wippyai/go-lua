package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestPlanInternsAllocationTemplateAndKeepsLexicalOrdinals(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeCall)
	second := graph.AddNode(cfg.NodeCall)
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template := sig.OperationalEffects.ReturnAllocationTemplates[0]
	one, _ := NewSignatureAllocationOperation(SignatureAllocationSite{Template: template.Root, Ordinal: 1}, template)
	two, _ := NewSignatureAllocationOperation(SignatureAllocationSite{Template: template.Root, Ordinal: 2}, template)
	plan := New(graph, factflow.FactsInput{}).WithSignatureAllocations(map[cfg.Point]SignatureAllocationOperation{first: one, second: two})
	if len(plan.signatureAllocationTemplates) != 1 {
		t.Fatalf("template pool = %d, want 1", len(plan.signatureAllocationTemplates))
	}
	for point, ordinal := range map[cfg.Point]uint32{first: 1, second: 2} {
		op, ok := plan.SignatureAllocationOperation(point)
		if !ok || op.Site().Ordinal != ordinal || op.Site().Template != template.Root {
			t.Fatalf("point %d site = %#v/%v", point, op.Site(), ok)
		}
	}
	got, _ := plan.SignatureAllocationOperation(first)
	owned := got.Template()
	owned.Objects[0].ID = "mutated"
	again, _ := plan.SignatureAllocationOperation(first)
	if again.Template().Objects[0].ID != template.Root {
		t.Fatal("allocation template accessor exposed Plan storage")
	}
}
