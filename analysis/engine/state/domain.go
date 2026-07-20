package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

// LaneID names one State product-lattice axis.
type LaneID string

var productDomainCache registrycache.Cache[ProductDomain]

// DomainOptions are per-solve lattice knobs that must not be cached globally.
type DomainOptions struct {
	// WidenThresholds is the finite syntactic threshold set used by threshold
	// widening for numeric interval-like lanes.
	WidenThresholds []int64
}

// Domain builds the default State lattice with every state axis enabled.
func Domain(reg *axis.Registry) lattice.Lattice[State] {
	return RegisteredProductDomain(reg).Lattice()
}

// RegisteredProductDomain returns the canonical all-lane product capability
// required by coordinate-factorized solvers.
func RegisteredProductDomain(reg *axis.Registry) ProductDomain {
	return productDomainCache.GetFor(reg, func(reg *axis.Registry) ProductDomain {
		return defaultLaneCatalog.ProductDomain(reg)
	})
}

func TryRegisteredProductDomainWithOptionalLanesAndOptions(reg *axis.Registry, lanes []LaneID, options DomainOptions) (ProductDomain, error) {
	if lanes == nil {
		return defaultLaneCatalog.TryProductDomainWithLaneSetAndOptions(reg, defaultLaneCatalog.LaneSet(), options)
	}
	return defaultLaneCatalog.TryProductDomainWithLaneSetAndOptions(reg, NewLaneSet(lanes...), options)
}

func TryRegisteredProductDomainWithLanes(reg *axis.Registry, lanes []LaneID) (ProductDomain, error) {
	return defaultLaneCatalog.TryProductDomainWithLaneSet(reg, NewLaneSet(lanes...))
}

// IsBottom reports whether st is the unreachable bottom element of the default
// state product lattice for reg. Consumers outside the transfer engine should
// use this instead of inferring reachability from any individual lane.
func IsBottom(reg *axis.Registry, st State) bool {
	domain := Domain(reg)
	return domain.Equal(st, domain.Bottom())
}

// NormalizeForDomain returns st in the canonical shape owned by domain. For a
// lane-selected domain, disabled lanes are dropped and enabled lanes are
// canonicalized as though st had just been joined from bottom.
func NormalizeForDomain(domain lattice.Lattice[State], st State) State {
	return domain.Join(domain.Bottom(), st)
}

// DomainWithLaneSet builds a State lattice from a sealed lane selection. The
// catalog canonicalizes selected lanes to registry order, so callers only
// choose membership.
func DomainWithLaneSet(reg *axis.Registry, lanes LaneSet) lattice.Lattice[State] {
	domain, err := TryDomainWithLaneSet(reg, lanes)
	if err != nil {
		panic(err)
	}
	return domain
}

// TryDomainWithLaneSet builds a State lattice from a sealed lane selection,
// returning configuration errors instead of panicking.
func TryDomainWithLaneSet(reg *axis.Registry, lanes LaneSet) (lattice.Lattice[State], error) {
	return defaultLaneCatalog.TryDomainWithLaneSet(reg, lanes)
}

// DomainWithOptionalLanes builds a State lattice from config-style lane IDs.
// Nil uses the default lane set; a non-nil slice is the exact enabled set, so
// an empty non-nil slice disables every State axis.
func DomainWithOptionalLanes(reg *axis.Registry, lanes []LaneID) lattice.Lattice[State] {
	domain, err := TryDomainWithOptionalLanes(reg, lanes)
	if err != nil {
		panic(err)
	}
	return domain
}

// TryDomainWithOptionalLanes is the non-panicking form of
// DomainWithOptionalLanes.
func TryDomainWithOptionalLanes(reg *axis.Registry, lanes []LaneID) (lattice.Lattice[State], error) {
	if lanes == nil {
		return Domain(reg), nil
	}
	return TryDomainWithLanes(reg, lanes)
}

// TryDomainWithOptionalLanesAndOptions is the non-cached form used when a solve
// needs per-body domain options such as threshold widening.
func TryDomainWithOptionalLanesAndOptions(reg *axis.Registry, lanes []LaneID, options DomainOptions) (lattice.Lattice[State], error) {
	if lanes == nil {
		return defaultLaneCatalog.TryDomainWithLaneSetAndOptions(reg, defaultLaneCatalog.LaneSet(), options)
	}
	return defaultLaneCatalog.TryDomainWithLaneSetAndOptions(reg, NewLaneSet(lanes...), options)
}

// DomainWithLanes builds a State lattice from a slice of enabled lanes.
// Disabled lanes are ignored by Equal/LessOrEq and dropped by Join/Widen. The
// input slice is copied before validation, so callers can pass config/UI
// storage directly; lane order is canonicalized by the catalog.
func DomainWithLanes(reg *axis.Registry, lanes []LaneID) lattice.Lattice[State] {
	return DomainWithLaneSet(reg, NewLaneSet(lanes...))
}

// TryDomainWithLanes is the non-panicking form of DomainWithLanes.
func TryDomainWithLanes(reg *axis.Registry, lanes []LaneID) (lattice.Lattice[State], error) {
	return TryDomainWithLaneSet(reg, NewLaneSet(lanes...))
}
