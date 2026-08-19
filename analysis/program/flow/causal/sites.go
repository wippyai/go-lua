package causal

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Sites is the allocation-free canonical causal endpoint projection. Its
// indexes borrow the one sealed Causal owner and retain no syntax/IR copy.
type Sites struct{ result *Result }

// Sites opens the endpoint projection over this sealed owner.
func (r *Result) Sites() Sites { return Sites{result: r} }

// Owns accepts only a Site issued by this exact hot Causal owner. Unlike
// Equal, it deliberately rejects equivalent artifact replay handles.
func (v Sites) Owns(site Site) bool { return v.result.OwnsSite(site) }

// Count reports the deduped route-endpoint denominator.
func (v Sites) Count() int { return v.result.SiteCount() }

// At returns one endpoint in canonical Term order.
func (v Sites) At(index int) (Site, bool) { return v.result.SiteAt(index) }

// ForTerm resolves only Terms that occur as existing route endpoints or sealed
// body-terminal Outcome coordinates.
func (v Sites) ForTerm(term keyspace.Term) (Site, bool) { return v.result.SiteForTerm(term) }

// ResolveContextID performs an exact-quartet-fenced contextual lookup.
func (v Sites) ResolveContextID(id identity.ContentID) (Site, bool) {
	return v.result.ResolveContextID(id)
}
