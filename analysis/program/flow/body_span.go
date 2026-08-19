package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// BodyContextIDs returns the existing owner-local Body path and exact Body
// boundary context. Both coordinates are issued by Flow's sealed
// FunctionBoundaries and semantic-path projections; callers must not rebuild
// the join at Program altitude.
func (view View) BodyContextIDs(body keyspace.Term) (path, context identity.ContentID, ok bool) {
	if !view.available() || body == 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	boundary, boundaryOK := view.FunctionBoundaries().ForBody(body)
	path, pathOK := view.BodyPath(body)
	context = boundary.ContextID()
	return path, context, boundaryOK && boundary.Available() && pathOK && path.Available() && context.Available()
}

// FinishSite resolves an authored Finish chain to its first existing causal
// Site. Positionless contextual terms remain internal Flow ports and never
// escape through this query. Flow owns both the port plane and causal Site
// directory, so no Program-level geometry projection is needed.
//
// The sealed Source denominator bounds malformed cyclic chains without a
// retained visited index or an allocation.
func (view View) FinishSite(term keyspace.Term) (causal.Site, bool) {
	if !view.available() || term == 0 {
		return causal.Site{}, false
	}
	ports, sites := view.Ports(), view.Causal().Sites()
	limit := uint64(ports.termCount())
	for step := uint64(0); step <= limit; step++ {
		finish, finishOK := ports.Finish(term)
		if !finishOK || finish == 0 {
			return causal.Site{}, false
		}
		if site, siteOK := sites.ForTerm(finish); siteOK && site.Available() {
			return site, true
		}
		if finish == term {
			return causal.Site{}, false
		}
		term = finish
	}
	return causal.Site{}, false
}
