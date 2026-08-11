package identityvalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// registry is test-local construction only; it is not analyzer composition
// authority.
var registry testRegistryFactory

type testRegistryFactory struct{}

func (testRegistryFactory) Registry() *axis.Registry {
	registry, err := product.RegistryWithAxes(identity.Spec().Erase())
	if err != nil {
		panic(err)
	}
	return registry
}
