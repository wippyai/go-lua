package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// Solver-side summary surface receipt; the compile-side validation of that
// receipt lives in schema_surface_receipt.go.

// SummarySurfaceReceipt is an opaque Factor/form authority for one summary
// surface. The graph-local Surface.Local is supplied by the topology builder;
// the Factor and normalizer identity come only from this receipt.
type SummarySurfaceReceipt[K ~uint32 | ~uint64, V any] struct {
	receipt    factorRuntimeReceipt
	form       factorFormReceipt
	formSchema *Schema
}

func (receipt SummarySurfaceReceipt[K, V]) valid(state *schemaBindingState, authority *schemaBindingAuthority) bool {
	return receipt.receipt.valid() && receipt.receipt.state == state && receipt.receipt.authority == authority && receipt.formSchema == state.schema && receipt.form.kind == SchemaFormReadSummary && receipt.form.ordinal < uint64(len(receipt.receipt.forms)) && receipt.receipt.forms[receipt.form.ordinal] == receipt.form
}

func (receipt SummarySurfaceReceipt[K, V]) boundTopologySummarySurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, composition.Key, bool) {
	if receipt.formSchema == nil || !receipt.valid(receipt.receipt.state, receipt.receipt.authority) {
		return nil, nil, composition.Key{}, composition.Key{}, false
	}
	return receipt.receipt.state, receipt.receipt.authority, receipt.receipt.semantic, receipt.form.semantic, true
}
