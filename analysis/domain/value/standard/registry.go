package standard

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

var registry = mustRegistry()

// Registry returns the frozen singleton registry for the standard value-axis
// bundle.
func Registry() *axis.Registry {
	return registry
}

// RegistryWithAxes returns a fresh frozen registry containing the standard
// sparse-axis bundle plus caller-provided sparse axes.
func RegistryWithAxes(specs ...axis.ErasedSpec) (*axis.Registry, error) {
	base := append(defaultSpecs(), specs...)
	return product.RegistryWithAxes(base...)
}

func mustRegistry() *axis.Registry {
	reg, err := RegistryWithAxes()
	if err != nil {
		panic(err)
	}
	return reg
}

func defaultSpecs() []axis.ErasedSpec {
	return []axis.ErasedSpec{
		variantorigin.Spec().Erase(),
		identity.Spec().Erase(),
		runtimekind.Spec().Erase(),
		escape.Spec().Erase(),
		ownership.Spec().Erase(),
		evidence.Spec().Erase(),
		assertion.Spec().Erase(),
	}
}
