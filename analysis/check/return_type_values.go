package check

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

// ReturnTypeValues materializes declared return-type evidence for summary
// projection at call boundaries.
func (r *Result) ReturnTypeValues() []product.Value {
	if r == nil || r.registry == nil || r.bindings == nil || r.Function() == nil {
		return nil
	}
	returnTypes := r.Function().ReturnTypes
	if len(returnTypes) == 0 {
		return nil
	}
	resolver := newEntryTypeResolver(r.bindings)
	out := make([]product.Value, 0, len(returnTypes))
	for _, expr := range returnTypes {
		t, ok := resolver.Type(expr)
		if !ok {
			out = append(out, product.Top())
			continue
		}
		out = append(out, typevalue.WithWitness(r.registry, typevalue.FromType(r.registry, t), t))
	}
	return out
}
