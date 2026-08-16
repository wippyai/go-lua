package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// SummarySurfaceReceipt is an opaque Factor/form authority for one summary
// surface. The graph-local Surface.Local is supplied by the topology builder;
// the Factor and normalizer identity come only from this receipt.
type SummarySurfaceReceipt[K ~uint32 | ~uint64, V any] struct {
	receipt    factorRuntimeReceipt
	form       factorFormReceipt
	formSchema *Schema
}

// WeakSurfaceReceipt is the corresponding authority for a weak exact-write
// surface. Weak coverage is graph-local, but the Factor/form owner remains
// sealed and cannot be replaced by an equal SchemaBinding.
type WeakSurfaceReceipt[K ~uint32 | ~uint64, V any] struct {
	receipt    factorRuntimeReceipt
	form       factorFormReceipt
	formSchema *Schema
}

func (receipt SummarySurfaceReceipt[K, V]) valid(state *schemaBindingState, authority *schemaBindingAuthority) bool {
	return receipt.receipt.valid() && receipt.receipt.state == state && receipt.receipt.authority == authority && receipt.formSchema == state.schema && receipt.form.kind == SchemaFormReadSummary && receipt.form.ordinal < uint64(len(receipt.receipt.forms)) && receipt.receipt.forms[receipt.form.ordinal] == receipt.form
}

func (receipt WeakSurfaceReceipt[K, V]) valid(state *schemaBindingState, authority *schemaBindingAuthority) bool {
	return receipt.receipt.valid() && receipt.receipt.state == state && receipt.receipt.authority == authority && receipt.formSchema == state.schema && receipt.form.kind == SchemaFormWriteExact && receipt.form.ordinal == receipt.receipt.ordinal
}

type bindingSummarySurfaceReceipt interface {
	boundTopologySummarySurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, composition.Key, bool)
}

type bindingWeakSurfaceReceipt interface {
	boundTopologyWeakSurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool)
}

func (receipt SummarySurfaceReceipt[K, V]) boundTopologySummarySurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, composition.Key, bool) {
	if receipt.formSchema == nil || !receipt.valid(receipt.receipt.state, receipt.receipt.authority) {
		return nil, nil, composition.Key{}, composition.Key{}, false
	}
	return receipt.receipt.state, receipt.receipt.authority, receipt.receipt.semantic, receipt.form.semantic, true
}

func (receipt WeakSurfaceReceipt[K, V]) boundTopologyWeakSurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if receipt.formSchema == nil || !receipt.valid(receipt.receipt.state, receipt.receipt.authority) {
		return nil, nil, composition.Key{}, false
	}
	return receipt.receipt.state, receipt.receipt.authority, receipt.receipt.semantic, true
}

// SummarySurface issues an exact form receipt after Binding.Seal.
func (implementation *FactorImplementation[K, V]) SummarySurface(form SchemaReadForm[V]) (SummarySurfaceReceipt[K, V], bool) {
	if implementation == nil || !implementation.receipt.validForms() || form.cell == nil || form.Schema() != implementation.receipt.schema || form.cell.kind != SchemaFormReadSummary {
		return SummarySurfaceReceipt[K, V]{}, false
	}
	formFactor, formOrdinal := form.cell.ordinal>>32, uint64(uint32(form.cell.ordinal))
	if formFactor != implementation.receipt.ordinal || formOrdinal >= uint64(len(implementation.receipt.forms)) {
		return SummarySurfaceReceipt[K, V]{}, false
	}
	formReceipt := implementation.receipt.forms[formOrdinal]
	if formReceipt.kind != SchemaFormReadSummary || !formReceipt.semantic.Available() {
		return SummarySurfaceReceipt[K, V]{}, false
	}
	return SummarySurfaceReceipt[K, V]{receipt: implementation.receipt, form: formReceipt, formSchema: form.Schema()}, true
}

// WeakExactWriteSurface issues the Factor-owned exact-write authority used by
// graph weak-target coverage.
func (implementation *FactorImplementation[K, V]) WeakExactWriteSurface() (WeakSurfaceReceipt[K, V], bool) {
	if implementation == nil || !implementation.receipt.validForms() {
		return WeakSurfaceReceipt[K, V]{}, false
	}
	// Exact read/write forms are canonical Factor forms rather than entries in
	// the optional summary/selector form vector.
	formReceipt := factorFormReceipt{ordinal: implementation.receipt.ordinal, kind: SchemaFormWriteExact}
	return WeakSurfaceReceipt[K, V]{receipt: implementation.receipt, form: formReceipt, formSchema: implementation.receipt.schema}, true
}

func validateSummarySurfaceReceipt(receipt bindingSummarySurfaceReceipt, state *schemaBindingState, authority *schemaBindingAuthority, surface equation.Surface) bool {
	receiptState, receiptAuthority, factor, normalizer, ok := receipt.boundTopologySummarySurfaceReceipt()
	return ok && receiptState == state && receiptAuthority == authority && surface.Available() && surface.Factor == factor && surface.Form == equation.SurfaceReadSummary && surface.Semantic == normalizer && surface.Normalizer == normalizer && surface.Mode == equation.TargetModeNone
}

func validateWeakSurfaceReceipt(receipt bindingWeakSurfaceReceipt, state *schemaBindingState, authority *schemaBindingAuthority, surface equation.Surface) bool {
	receiptState, receiptAuthority, factor, ok := receipt.boundTopologyWeakSurfaceReceipt()
	return ok && receiptState == state && receiptAuthority == authority && surface.Available() && surface.Factor == factor && surface.Form == equation.SurfaceWriteExact && surface.Mode == equation.TargetModeWeak && !surface.Semantic.Available() && !surface.Normalizer.Available()
}

func duplicateSummaryMapping(rows []equation.SummaryMapping, value equation.SummaryMapping) bool {
	for _, row := range rows {
		if row.Surface == value.Surface {
			return true
		}
	}
	return false
}

func duplicateWeakTargetMapping(rows []equation.WeakTargetMapping, value equation.WeakTargetMapping) bool {
	for _, row := range rows {
		if row.Surface == value.Surface {
			return true
		}
	}
	return false
}
