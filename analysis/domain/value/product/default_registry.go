package product

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
)

var defaultRegistry = buildDefaultRegistry()

func DefaultRegistry() *axis.Registry {
	return defaultRegistry
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
}

func buildDefaultRegistry() *axis.Registry {
	reg := axis.NewRegistry()
	axis.Register(reg, variantorigin.Spec())
	axis.Register(reg, identity.Spec())
	axis.Register(reg, escape.Spec())
	axis.Register(reg, ownership.Spec())
	axis.Register(reg, evidence.Spec())
	return reg.Freeze()
}
