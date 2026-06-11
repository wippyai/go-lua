package product

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/defaults"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

var defaultRegistry = buildDefaultRegistry()

func DefaultRegistry() *axis.Registry {
	return defaultRegistry
}

// DefaultRegistryWithAxes returns a fresh frozen registry containing the
// product default sparse axes plus caller-provided sparse axes.
func DefaultRegistryWithAxes(specs ...axis.ErasedSpec) (*axis.Registry, error) {
	reg := axis.NewRegistry()
	registerDefaultSparseAxes(reg)
	for _, spec := range specs {
		if err := registerProductSparseAxis(reg, spec); err != nil {
			return nil, err
		}
	}
	return reg.Freeze(), nil
}

func registryOrDefault(reg *axis.Registry) *axis.Registry {
	if reg == nil {
		return defaultRegistry
	}
	requireProductRegistry(reg)
	return reg
}

func requireProductRegistry(reg *axis.Registry) {
	if reg == nil {
		panic("product: nil registry")
	}
	if !reg.Frozen() {
		panic("product: registry must be frozen before use")
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		panic("product: presence is a core lane and must not be registered as a sparse axis")
	}
	for _, spec := range reg.Specs() {
		if err := validateProductSparseAxis(spec); err != nil {
			panic(err)
		}
	}
}

func buildDefaultRegistry() *axis.Registry {
	reg := axis.NewRegistry()
	registerDefaultSparseAxes(reg)
	return reg.Freeze()
}

func registerDefaultSparseAxes(reg *axis.Registry) {
	for _, spec := range defaults.SparseSpecs() {
		if err := registerProductSparseAxis(reg, spec); err != nil {
			panic(err)
		}
	}
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
