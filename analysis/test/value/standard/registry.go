package standard

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Registry returns the analyzer-owned canonical value-axis registry. This
// compatibility façade intentionally retains no second registry or axis list.
func Registry() *axis.Registry {
	return product.CanonicalRegistry()
}

// RegistryWithAxes returns a fresh frozen registry containing the standard
// sparse-axis bundle plus caller-provided sparse axes.
func RegistryWithAxes(specs ...axis.ErasedSpec) (*axis.Registry, error) {
	return product.RegistryWithCanonicalAxes(specs...)
}
