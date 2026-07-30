package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
)

// instantiateFormalInputProduct applies one stabilized symbolic input
// constraint to its caller actual. Bottom is the symbolic-entry identity: it
// contributes no constraint. Every non-Bottom product is interpreted by the
// registered product refinement kernel, so this law is independent of the
// enabled axis inventory.
//
// Executable Apply uses this function as the single product-instantiation law.
// Lexical publication does not replay caller values: it publishes the body's
// own stabilized relation independently of its incoming call sites.
func instantiateFormalInputProduct(
	reg *axis.Registry,
	actual product.Value,
	constraint product.Value,
) (product.Value, error) {
	if reg == nil || !product.BelongsToRegistry(reg, actual) || !product.BelongsToRegistry(reg, constraint) {
		return product.Value{}, fmt.Errorf("transformer: formal input product belongs to a foreign registry")
	}
	if product.Equal(reg, constraint, product.Bottom(reg)) {
		return actual, nil
	}
	return factapply.RefineProductValueConstraint(reg, actual, constraint), nil
}
