package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

// LaneID names one State product-lattice axis.
type LaneID string

var domainCache registrycache.Cache[lattice.Lattice[State]]

// Domain builds the default State lattice with every state axis enabled.
func Domain(reg *axis.Registry) lattice.Lattice[State] {
	return domainCache.Get(reg, func() lattice.Lattice[State] {
		return defaultDomainLaneCatalog.Domain(reg)
	})
}

// NormalizeForDomain returns st in the canonical shape owned by domain. For a
// lane-selected domain, disabled lanes are dropped and enabled lanes are
// canonicalized as though st had just been joined from bottom.
func NormalizeForDomain(domain lattice.Lattice[State], st State) State {
	return domain.Join(domain.Bottom(), st)
}

// DomainWithLaneSet builds a State lattice from a sealed ordered lane
// selection. Use DomainWithLanes when the caller already has a plain slice from
// configuration or UI controls.
func DomainWithLaneSet(reg *axis.Registry, lanes LaneSet) lattice.Lattice[State] {
	domain, err := TryDomainWithLaneSet(reg, lanes)
	if err != nil {
		panic(err)
	}
	return domain
}

// TryDomainWithLaneSet builds a State lattice from a sealed ordered lane
// selection, returning configuration errors instead of panicking.
func TryDomainWithLaneSet(reg *axis.Registry, lanes LaneSet) (lattice.Lattice[State], error) {
	return defaultDomainLaneCatalog.TryDomainWithLaneSet(reg, lanes)
}

// DomainWithLanes builds a State lattice from the exact ordered slice of enabled
// lanes. Disabled lanes are ignored by Equal/LessOrEq and dropped by Join/Widen.
// The input slice is copied before validation, so callers can pass config/UI
// storage directly.
func DomainWithLanes(reg *axis.Registry, lanes []LaneID) lattice.Lattice[State] {
	return DomainWithLaneSet(reg, NewLaneSet(lanes...))
}

// TryDomainWithLanes is the non-panicking form of DomainWithLanes.
func TryDomainWithLanes(reg *axis.Registry, lanes []LaneID) (lattice.Lattice[State], error) {
	return TryDomainWithLaneSet(reg, NewLaneSet(lanes...))
}
