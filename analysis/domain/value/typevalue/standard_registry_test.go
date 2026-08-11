package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// registry is test-local construction only. Production Value ownership must
// supply its private registry explicitly rather than consult a singleton.
var registry testRegistryFactory

type testRegistryFactory struct{}

func (testRegistryFactory) Registry() *axis.Registry {
	registry, err := product.RegistryWithAxes(
		variantorigin.Spec().Erase(),
		identity.Spec().Erase(),
		runtimekind.Spec().Erase(),
		typewitness.Spec().Erase(),
		escape.Spec().Erase(),
		evidence.Spec().Erase(),
		assertion.Spec().Erase(),
	)
	if err != nil {
		panic(err)
	}
	return registry
}
