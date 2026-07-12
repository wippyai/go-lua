package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestAllocationResultAndEffectShareOneTemplateTerm(t *testing.T) {
	reg := standard.Registry()
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create template rejected")
	}
	op, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 41, Template: template.Root, Ordinal: 7,
	}, template)
	arena := NewArena(reg)
	allocation := arena.AllocationTemplate(op)
	result := arena.AllocationResultValue(allocation, 0)
	effects := NewEffectArena(arena)
	effect, err := effects.AllocationTemplate(allocation)
	if err != nil {
		t.Fatal(err)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	value, exact := arena.evalValue(result, cursor, SpecializationContext{})
	resolved, resolvedExact := effects.resolve(effect, cursor, SpecializationContext{})
	if !exact || !resolvedExact || resolved.Kind != EffectAllocationTemplate || !product.Equal(reg, value, resolved.Allocation.Result) {
		t.Fatalf("shared allocation value/effect exact=%v/%v resolved=%#v", exact, resolvedExact, resolved)
	}

	callerTerms := NewArena(reg)
	callerEffects := NewEffectArena(callerTerms)
	bindings, _ := NewTermRootBindings(Shape{}, Shape{}, nil, nil)
	rebased, err := RebaseEffectDAGs(callerEffects, effects, bindings, []EffectTerm{effect})
	if err != nil || len(rebased.Effects) != 1 {
		t.Fatalf("allocation effect rebase = %#v/%v", rebased, err)
	}
	again, ok := callerEffects.resolve(rebased.Effects[0], cursor, SpecializationContext{})
	if !ok || again.Allocation.Site != op.Site() || !product.Equal(reg, again.Allocation.Result, value) {
		t.Fatalf("rebased allocation = %#v/%v", again, ok)
	}
}
