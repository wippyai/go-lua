package product

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// RegistryWithAxes returns a fresh frozen registry containing the caller's
// sparse axes in registration order.
func RegistryWithAxes(specs ...axis.ErasedSpec) (*axis.Registry, error) {
	reg := axis.NewRegistry()
	axis.RegisterCanonicalCore(reg, presence.Spec())
	for _, spec := range specs {
		if err := registerProductSparseAxis(reg, spec); err != nil {
			return nil, err
		}
	}
	return reg.Freeze(), nil
}

func registerProductSparseAxis(reg *axis.Registry, spec axis.ErasedSpec) error {
	if spec == nil {
		return reg.RegisterErased(spec)
	}
	if err := validateProductSparseAxis(spec); err != nil {
		return err
	}
	return reg.RegisterErased(spec)
}

func validateProductSparseAxis(spec axis.ErasedSpec) error {
	if spec.ID() == presence.Key.ID() {
		return fmt.Errorf("product: presence is a core lane and must not be registered as a sparse axis")
	}
	if !spec.HasMeet() {
		return fmt.Errorf("product: sparse axis %q must define Meet", spec.ID())
	}
	return nil
}
