package causal

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Local is the final Program-local recurrence projection. It owns neither a
// graph nor copied causal rows: Region membership refers only to existing
// successor refs and Site handles from Result's sole Causal authority.
type Local struct{ result *Result }

func (r *Result) Local() Local { return Local{result: r} }

func (r *Result) componentPath(head keyspace.Term) (identity.ContentID, bool) {
	if r == nil || !r.componentIssued(head) {
		return identity.ContentID{}, false
	}
	index, ok := r.localIndex(head)
	if !ok || uint64(index) >= uint64(len(r.componentPaths)) {
		return identity.ContentID{}, false
	}
	path := r.componentPaths[index]
	return path, path.Available()
}

// installStructuralPaths installs the parent-issued semantic path view used
// by final route rows. Paths are parallel to the Source family denominators;
// zero entries are rejected when a route later requires that role.
func (r *Result) installStructuralPaths(paths *semanticpath.CausalPaths) bool {
	if r == nil || paths == nil || r.structuralPaths != nil || !paths.Matches(r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return false
	}
	// The certificate already checked the exact Source family denominator.
	// Causal's denominator must agree before it retains the opaque projection.
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if int(r.index.familyCounts[family])+1 == 0 { // keeps the check explicit and overflow-safe
			return false
		}
	}
	r.structuralPaths = paths
	return true
}

type Region struct {
	result *Result
	index  uint32
	id     identity.ContentID
}

type localRegion struct {
	head       keyspace.Term
	id         identity.ContentID
	successors []successorRef
	sites      []uint32
}

// localStore is a flat canonical projection. regions follows the ordered
// recurrence-issued head directory; inverses refer only to existing rows.
type localStore struct {
	regions     []localRegion
	byID        map[identity.ContentID]uint32
	bySuccessor map[successorRef]uint32
	bySite      [][]uint32
}

const (
	regionIDDomain = "wippy/program/flow/local-region"
)

func regionID(path identity.ContentID) identity.ContentID {
	if !path.Available() {
		return identity.ContentID{}
	}
	var encoded [len(regionIDDomain) + 1 + 32]byte
	offset := copy(encoded[:], regionIDDomain)
	encoded[offset] = 0
	offset++
	offset += copy(encoded[offset:], path[:])
	return identity.ContentID(sha256.Sum256(encoded[:offset]))
}

// buildLocal transfers the recurrence-issued ordered head directory into the
// final owner exactly once. It consumes existing successor refs and Site rows
// in O(C+R+S); no endpoint graph or SCC membership is reconstructed.
func (r *Result) buildLocal() bool {
	if r == nil || !r.available() || r.local.regions != nil || r.components == nil ||
		len(r.sites.byTerm) != len(r.sites.rows) {
		return false
	}
	store := localStore{
		regions:     make([]localRegion, len(r.components)),
		byID:        make(map[identity.ContentID]uint32, len(r.components)),
		bySuccessor: make(map[successorRef]uint32),
		bySite:      make([][]uint32, len(r.sites.rows)),
	}
	for index, head := range r.components {
		if !canonicalComponent(head, true) || uint64(index) > uint64(^uint32(0)) {
			return false
		}
		id := identity.ContentID{}
		if path, pathOK := r.componentPath(head); pathOK {
			id = regionID(path)
			if _, duplicate := store.byID[id]; duplicate {
				return false
			}
			store.byID[id] = uint32(index)
		}
		store.regions[index] = localRegion{head: head, id: id}
	}
	// First group existing successor refs by the exact issued head. The
	// transient lookup is discarded below; it is not a retained SCC map.
	for _, ref := range r.index.refs {
		component, _, _, ok := r.localRefComponent(ref)
		if !ok {
			return false
		}
		if component == 0 {
			continue
		}
		regionIndex, found := r.componentIndex[component]
		if !found || uint64(regionIndex) >= uint64(len(store.regions)) || regionIndex == ^uint32(0) {
			return false
		}
		if _, duplicate := store.bySuccessor[ref]; duplicate {
			return false
		}
		store.bySuccessor[ref] = regionIndex
		store.regions[regionIndex].successors = append(store.regions[regionIndex].successors, ref)
	}
	// Then traverse each region once. The stamp plane now cannot suffer the
	// A/B/A interleaving bug: a region's entire route group is contiguous.
	siteMark := make([]uint32, len(r.sites.rows))
	for regionIndex := range store.regions {
		if uint64(regionIndex) >= uint64(^uint32(0)) {
			return false
		}
		row := &store.regions[regionIndex]
		stamp := uint32(regionIndex) + 1
		for _, ref := range row.successors {
			_, from, to, ok := r.localRefComponent(ref)
			if !ok {
				return false
			}
			for _, term := range [...]keyspace.Term{from, to} {
				site, exists := r.sites.byTerm[term]
				if !exists {
					return false
				}
				if siteMark[site] != stamp {
					siteMark[site] = stamp
					row.sites = append(row.sites, site)
				}
			}
		}
		for _, site := range row.sites {
			if uint64(site) >= uint64(len(store.bySite)) {
				return false
			}
			store.bySite[site] = append(store.bySite[site], uint32(regionIndex))
		}
	}
	r.local = store
	return true
}

// localRefComponent projects only prevalidated final-row fields. It is used
// during sealing after the issued directory has been validated, avoiding query
// predicates or binary searches in the O(R) Local construction pass.
func (r *Result) localRefComponent(ref successorRef) (keyspace.Term, keyspace.Term, keyspace.Term, bool) {
	if r == nil || !isKnownArm(ref.arm) || ref.local != isLocalArm(ref.arm) {
		return 0, 0, 0, false
	}
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return 0, 0, 0, false
		}
		row := r.edges.rows[ref.index]
		if row.From == 0 || row.To == 0 || (row.component != 0 && !canonicalComponent(row.component, true)) {
			return 0, 0, 0, false
		}
		return row.component, row.From, row.To, true
	}
	if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
		return 0, 0, 0, false
	}
	row := r.boundaries.rows[ref.index]
	if !boundaryArmPresent(row, ref.arm) {
		return 0, 0, 0, false
	}
	to, _, _, ok := boundarySuccessor(row.CallBoundary, ref.arm)
	component := row.components[ref.arm]
	if !ok || row.Call == 0 || to == 0 || (component != 0 && !canonicalComponent(component, true)) {
		return 0, 0, 0, false
	}
	return component, row.Call, to, true
}

func (r *Result) localIndex(head keyspace.Term) (uint32, bool) {
	if r == nil || !r.componentIssued(head) {
		return 0, false
	}
	index, found := r.componentIndex[head]
	if !found || uint64(index) > uint64(^uint32(0)) {
		return 0, false
	}
	return index, true
}

func (v Local) regionAt(index int) (Region, bool) {
	if v.result == nil || !v.result.available() || index < 0 || index >= len(v.result.local.regions) ||
		uint64(index) >= uint64(len(v.result.components)) {
		return Region{}, false
	}
	row := v.result.local.regions[index]
	head := v.result.components[index]
	path, pathOK := v.result.componentPath(head)
	if row.head != head || !v.result.componentIssued(head) || !pathOK || row.id != regionID(path) {
		return Region{}, false
	}
	return Region{result: v.result, index: uint32(index), id: row.id}, true
}

func (v Local) region(head keyspace.Term) (Region, bool) {
	index, ok := v.result.localIndex(head)
	if !ok {
		return Region{}, false
	}
	return v.regionAt(int(index))
}

// Count reports all recurrence-issued cyclic components, including an empty
// component that has no final route in this Program projection.
func (v Local) Count() int {
	if v.result == nil || !v.result.available() || len(v.result.local.regions) != len(v.result.components) {
		return 0
	}
	return len(v.result.local.regions)
}

func (v Local) At(index int) (Region, bool) { return v.regionAt(index) }

// Resolve authenticates a stable exact-quartet region identity by canonical
// ordered lookup. It does not search physical rows or reconstruct a region.
func (v Local) Resolve(id identity.ContentID) (Region, bool) {
	if v.result == nil || !v.result.available() || !id.Available() {
		return Region{}, false
	}
	index, ok := v.result.local.byID[id]
	if !ok {
		return Region{}, false
	}
	return v.regionAt(int(index))
}

// ForHead resolves only a recurrence-issued component head. It never accepts
// a shaped-but-unissued Label/Loop term.
func (v Local) ForHead(head keyspace.Term) (Region, bool) { return v.region(head) }

// ForSuccessor is the singular owner-issued inverse for an existing final
// route. A foreign or manufactured Successor fails closed.
func (v Local) ForSuccessor(successor Successor) (Region, bool) {
	if v.result == nil || successor.result != v.result || !successor.refValid {
		return Region{}, false
	}
	index, ok := v.result.local.bySuccessor[successor.ref]
	if !ok || uint64(index) >= uint64(len(v.result.local.regions)) || successor.component != v.result.local.regions[index].head {
		return Region{}, false
	}
	return v.regionAt(int(index))
}

// RegionCountForSite and RegionAtSite are the allocation-free multi-valued
// Site inverse. A term-level Site can legitimately appear in several Regions.
func (v Local) RegionCountForSite(site Site) int {
	if v.result == nil || site.result != v.result || !site.available() || uint64(site.index) >= uint64(len(v.result.local.bySite)) {
		return 0
	}
	return len(v.result.local.bySite[site.index])
}

func (v Local) RegionAtSite(site Site, index int) (Region, bool) {
	if index < 0 || v.result == nil || site.result != v.result || !site.available() || uint64(site.index) >= uint64(len(v.result.local.bySite)) {
		return Region{}, false
	}
	regions := v.result.local.bySite[site.index]
	if index >= len(regions) {
		return Region{}, false
	}
	return v.regionAt(int(regions[index]))
}

func (region Region) row() (localRegion, bool) {
	if region.result == nil || uint64(region.index) >= uint64(len(region.result.local.regions)) {
		return localRegion{}, false
	}
	row := region.result.local.regions[region.index]
	if !region.Available() || row.id != region.id {
		return localRegion{}, false
	}
	return row, true
}

func (region Region) Available() bool {
	if region.result == nil || uint64(region.index) >= uint64(len(region.result.local.regions)) {
		return false
	}
	resolved, ok := Local{result: region.result}.regionAt(int(region.index))
	return ok && resolved.id == region.id
}

func (region Region) Head() (keyspace.Term, bool) {
	row, ok := region.row()
	return row.head, ok
}

func (region Region) ID() identity.ContentID {
	if !region.Available() {
		return identity.ContentID{}
	}
	return region.id
}

func (region Region) SuccessorCount() int {
	row, ok := region.row()
	if !ok {
		return 0
	}
	return len(row.successors)
}

// SuccessorAt returns an existing Causal Successor; no route data is copied
// into Region storage.
func (region Region) SuccessorAt(index int) (Successor, bool) {
	row, ok := region.row()
	if !ok || index < 0 || index >= len(row.successors) {
		return Successor{}, false
	}
	successor, ok := region.result.successorForRef(row.successors[index])
	if !ok || !region.ContainsSuccessor(successor) {
		return Successor{}, false
	}
	return successor, true
}

func (region Region) SiteCount() int {
	row, ok := region.row()
	if !ok {
		return 0
	}
	return len(row.sites)
}

// SiteAt returns an existing Causal Site; the Region owns only its reference.
func (region Region) SiteAt(index int) (Site, bool) {
	row, ok := region.row()
	if !ok || index < 0 || index >= len(row.sites) {
		return Site{}, false
	}
	return region.result.siteAt(int(row.sites[index]))
}

func (region Region) ContainsSuccessor(successor Successor) bool {
	member, ok := (Local{result: region.result}).ForSuccessor(successor)
	return ok && member.index == region.index && member.id == region.id
}

func (region Region) ContainsSite(site Site) bool {
	local := Local{result: region.result}
	for index := 0; index < local.RegionCountForSite(site); index++ {
		member, ok := local.RegionAtSite(site, index)
		if ok && member.index == region.index && member.id == region.id {
			return true
		}
	}
	return false
}
