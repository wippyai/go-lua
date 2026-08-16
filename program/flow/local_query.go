package flow

import (
	"github.com/wippyai/go-lua/program/flow/internal/causal"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Local is Flow's sole public Program-local cyclic-region facade. It projects
// recurrence-issued Causal rows; it never derives membership from routes,
// Mu/reset witnesses, or authored topology.
type Local struct{ result *causal.Result }

// Local returns the Program-local cyclic-region projection for this exact
// committed Flow. Causal's physical storage remains private.
func (view View) Local() Local {
	if !view.semanticSourceAvailable() {
		return Local{}
	}
	return Local{result: view.component.programStructure.causal}
}

type Regions struct{ local causal.Local }

func (local Local) Regions() Regions {
	if local.result == nil {
		return Regions{}
	}
	return Regions{local: local.result.Local()}
}

// LocalWTO is the complete parent-issued local schedule.  Unlike Regions,
// which is the legacy flat cyclic-component diagnostic projection, WTO also
// contains acyclic singleton leaves and balanced Enter/Point/Exit events.
// Consumers must copy this certificate; they must not reconstruct an order
// from final routes.
type LocalWTO struct{ local causal.LocalWTO }

func (local Local) WTO() LocalWTO {
	if local.result == nil {
		return LocalWTO{}
	}
	return LocalWTO{local: local.result.LocalWTO()}
}

type WTORegion struct{ region causal.WTORegion }

func publicWTORegion(region causal.WTORegion) WTORegion { return WTORegion{region: region} }
func (region WTORegion) Available() bool                { return region.region.Available() }
func (region WTORegion) ID() keyspace.ContentID         { return region.region.ID() }
func (region WTORegion) ParentID() keyspace.ContentID   { return region.region.ParentID() }
func (region WTORegion) Cyclic() bool                   { return region.region.Cyclic() }
func (region WTORegion) Header() (Site, bool) {
	site, ok := region.region.Header()
	return publicSite(site), ok
}
func (region WTORegion) HeaderPoint() (WTOPoint, bool) {
	point, ok := region.region.HeaderPoint()
	return publicWTOPoint(point), ok
}
func (region WTORegion) RouteCount() int { return region.region.RouteCount() }
func (region WTORegion) RouteAt(index int) (Successor, bool) {
	route, ok := region.region.RouteAt(index)
	return publicSuccessor(route), ok
}
func (region WTORegion) SiteCount() int { return region.region.SiteCount() }
func (region WTORegion) SiteAt(index int) (Site, bool) {
	site, ok := region.region.SiteAt(index)
	return publicSite(site), ok
}
func (region WTORegion) PointCount() int { return region.region.PointCount() }
func (region WTORegion) PointAt(index int) (WTOPoint, bool) {
	point, ok := region.region.PointAt(index)
	return publicWTOPoint(point), ok
}

func (wto LocalWTO) Count() int { return wto.local.Count() }
func (wto LocalWTO) At(index int) (WTORegion, bool) {
	region, ok := wto.local.At(index)
	return publicWTORegion(region), ok
}
func (wto LocalWTO) Resolve(id keyspace.ContentID) (WTORegion, bool) {
	region, ok := wto.local.Resolve(id)
	return publicWTORegion(region), ok
}
func (wto LocalWTO) EventCount() int { return wto.local.EventCount() }

type WTOEventKind uint8

const (
	WTOEventInvalid WTOEventKind = iota
	WTOEventEnter
	WTOEventPoint
	WTOEventExit
)

type WTOEvent struct{ event causal.WTOEvent }

// WTOPoint is the parent-issued semantic LocalWTO vertex. A point may carry
// zero, one, or many optional Site attachments; consumers must preserve the
// point even when it has no Site.
type WTOPoint struct{ point causal.WTOPoint }

func publicWTOPoint(point causal.WTOPoint) WTOPoint { return WTOPoint{point: point} }
func (point WTOPoint) Available() bool              { return point.point.Available() }
func (point WTOPoint) PathID() keyspace.ContentID   { return point.point.PathID() }
func (point WTOPoint) SiteCount() int               { return point.point.SiteCount() }
func (point WTOPoint) SiteAt(index int) (Site, bool) {
	site, ok := point.point.SiteAt(index)
	return publicSite(site), ok
}

func publicWTOEvent(event causal.WTOEvent) WTOEvent { return WTOEvent{event: event} }
func (event WTOEvent) Available() bool              { return event.event.Available() }
func (event WTOEvent) Kind() WTOEventKind {
	return WTOEventKind(event.event.Kind())
}
func (event WTOEvent) Region() (WTORegion, bool) {
	region, ok := event.event.Region()
	return publicWTORegion(region), ok
}
func (event WTOEvent) Site() (Site, bool) {
	site, ok := event.event.Site()
	return publicSite(site), ok
}
func (event WTOEvent) Point() (WTOPoint, bool) {
	point, ok := event.event.Point()
	return publicWTOPoint(point), ok
}
func (wto LocalWTO) EventAt(index int) (WTOEvent, bool) {
	event, ok := wto.local.EventAt(index)
	return publicWTOEvent(event), ok
}

// Region is an opaque stable Program-local cyclic component. Its ID commits
// to the exact owner quartet and canonical component head, never a physical
// node, arc, SCC ordinal, or storage index.
type Region struct{ region causal.Region }

func publicRegion(region causal.Region) Region { return Region{region: region} }

func (region Region) Available() bool        { return region.region.Available() }
func (region Region) ID() keyspace.ContentID { return region.region.ID() }
func (region Region) Head() (keyspace.Term, bool) {
	return region.region.Head()
}

// ForHead resolves only an exact recurrence-issued component head.
func (regions Regions) ForHead(head keyspace.Term) (Region, bool) {
	region, ok := regions.local.ForHead(head)
	return publicRegion(region), ok
}

// Count and At enumerate recurrence-issued Regions in canonical head order.
func (regions Regions) Count() int { return regions.local.Count() }
func (regions Regions) At(index int) (Region, bool) {
	region, ok := regions.local.At(index)
	return publicRegion(region), ok
}

// Resolve authenticates a stable exact-quartet Region ID.
func (regions Regions) Resolve(id keyspace.ContentID) (Region, bool) {
	region, ok := regions.local.Resolve(id)
	return publicRegion(region), ok
}

// ForSuccessor is the singular membership inverse for an existing Flow
// Successor. A forged or foreign value has no region.
func (regions Regions) ForSuccessor(successor Successor) (Region, bool) {
	region, ok := regions.local.ForSuccessor(successor.route)
	return publicRegion(region), ok
}

// RegionCountForSite and RegionAtSite are the allocation-free multi-valued
// inverse for a term-level Site which may participate in several Regions.
func (regions Regions) RegionCountForSite(site Site) int {
	return regions.local.RegionCountForSite(site.site)
}
func (regions Regions) RegionAtSite(site Site, index int) (Region, bool) {
	region, ok := regions.local.RegionAtSite(site.site, index)
	return publicRegion(region), ok
}

func (region Region) ContainsSuccessor(successor Successor) bool {
	return region.region.ContainsSuccessor(successor.route)
}

func (region Region) ContainsSite(site Site) bool { return region.region.ContainsSite(site.site) }

// SuccessorCount/At and SiteCount/At traverse only existing Causal rows.
func (region Region) SuccessorCount() int { return region.region.SuccessorCount() }
func (region Region) SuccessorAt(index int) (Successor, bool) {
	successor, ok := region.region.SuccessorAt(index)
	return publicSuccessor(successor), ok
}
func (region Region) SiteCount() int { return region.region.SiteCount() }
func (region Region) SiteAt(index int) (Site, bool) {
	site, ok := region.region.SiteAt(index)
	return publicSite(site), ok
}
