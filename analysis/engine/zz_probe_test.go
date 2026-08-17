package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func probeCompileReceiptFactors(t testing.TB, binding *SchemaBinding, graph *equation.Graph) {
	runtime, ok := newReceiptRuntimeBinding(binding, graph)
	t.Logf("newReceiptRuntimeBinding ok=%v nil=%v", ok, runtime == nil)
	if !ok || runtime == nil {
		return
	}
	state := bindingState(binding)
	t.Logf("state nil=%v mode=%v stateEq=%v authNil=%v valid=%v", state == nil, runtime.mode, runtime.state == state, runtime.authority == nil, runtime.valid())
	if state != nil {
		state.mu.Lock()
		t.Logf("phase=%v authEq=%v schemaEq=%v cells=%d want=%d", state.phase, state.authority == runtime.authority, state.schema == runtime.schema, len(state.factors), schemaFactorCount(state.schema))
		for ordinal, cell := range state.factors {
			if cell == nil {
				t.Logf("cell %d nil", ordinal)
				continue
			}
			t.Logf("cell %d ord=%d schemaEq=%v complete=%v", ordinal, cell.schemaFactorOrdinal(), cell.schemaFactorSchema() == state.schema, cell.schemaFactorComplete())
			f, bound := cell.schemaFactorRuntimeBinding(runtime)
			key := state.schema.factorSemanticAt(uint64(ordinal))
			t.Logf("cell %d bound=%v fnil=%v keyAvail=%v keyEq=%v", ordinal, bound, f == nil, key.Available(), f != nil && compositionKeyOf(f.semantic()) == key)
		}
		state.mu.Unlock()
	}
	factors, _, ok := bindReceiptFactors(binding, runtime)
	t.Logf("bindReceiptFactors ok=%v", ok)
	frozen := runtime.freezeCatalog()
	t.Logf("freezeCatalog=%v", frozen)
	if !ok || !frozen {
		return
	}
	prepared, ordered, ok := prepareRuntimeComposition(factors, runtime.guards)
	t.Logf("prepareRuntimeComposition ok=%v nil=%v", ok, prepared == nil)
	if !ok || prepared == nil {
		return
	}
	attached, ok := prepared.Attach()
	t.Logf("Attach ok=%v nil=%v", ok, attached == nil)
	for _, factor := range ordered {
		preparer, preparable := factor.(interface{ prepareRouteTransformClosure() bool })
		t.Logf("route preparable=%v ok=%v", preparable, preparable && preparer.prepareRouteTransformClosure())
	}
}
