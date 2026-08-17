package causal

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Site is an opaque exact-quartet-fenced handle for one existing causal route
// endpoint or sealed body-terminal Outcome coordinate. Its physical row index
// is private and never contributes to its contextual identity.
type Site struct {
	result *Result
	index  uint32
}

type siteRow struct {
	term    keyspace.Term
	context identity.ContentID
	path    identity.ContentID // copied while the structural lease is live
}

type siteLookup struct {
	context identity.ContentID
	index   uint32
}

type siteStore struct {
	rows    []siteRow
	lookups []siteLookup
	byTerm  map[keyspace.Term]uint32
}

const (
	siteContextDomain  = "wippy/program/flow/causal-context-site"
	siteContextVersion = uint64(2)
)

func hashSiteContext(sourceID, flowID, staticID, moduleID identity.ContentID, term keyspace.Term) identity.ContentID {
	var encoded [256]byte
	offset := copy(encoded[:], siteContextDomain)
	encoded[offset] = 0
	offset++
	binary.BigEndian.PutUint64(encoded[offset:], siteContextVersion)
	offset += 8
	for _, owner := range [...]identity.ContentID{sourceID, flowID, staticID, moduleID} {
		offset += copy(encoded[offset:], owner[:])
	}
	var scalar [4]byte
	binary.BigEndian.PutUint32(scalar[:], uint32(term))
	offset += copy(encoded[offset:], scalar[:])
	return identity.ContentID(sha256.Sum256(encoded[:offset]))
}

func (s Site) available() bool {
	if s.result == nil || !s.result.available() || uint64(s.index) >= uint64(len(s.result.sites.rows)) {
		return false
	}
	row := s.result.sites.rows[s.index]
	return row.context == hashSiteContext(s.result.sourceID, s.result.flowID, s.result.staticID, s.result.moduleID, row.term)
}

// Available reports whether this handle belongs to the exact sealed Causal
// quartet and still authenticates its contextual endpoint preimage.
func (s Site) Available() bool { return s.available() }

// Equal compares contextual identity and exact owner provenance, never the
// private row index. Equivalent replay of the same quartet compares equal.
func (s Site) Equal(other Site) bool {
	if !s.available() || !other.available() {
		return false
	}
	left, right := s.result.sites.rows[s.index], other.result.sites.rows[other.index]
	return left.context == right.context && s.result.sourceID == other.result.sourceID &&
		s.result.flowID == other.result.flowID && s.result.staticID == other.result.staticID &&
		s.result.moduleID == other.result.moduleID
}

// ContextID returns the exact-quartet-fenced contextual endpoint identity. It
// is stable across equivalent seal/artifact replay but has no cross-mutation
// portability guarantee.
func (s Site) ContextID() identity.ContentID {
	if !s.available() {
		return identity.ContentID{}
	}
	return s.result.sites.rows[s.index].context
}

// PathID returns the parent-issued structural semantic identity of this Site.
// Unlike ContextID it contains no owner quartet or raw coordinate and is the
// portable identity consumed by reusable Program artifacts.
func (s Site) PathID() identity.ContentID {
	if !s.available() {
		return identity.ContentID{}
	}
	return s.result.sites.rows[s.index].path
}

// Term returns the existing causal endpoint represented by this site.
func (s Site) Term() (keyspace.Term, bool) {
	if !s.available() {
		return 0, false
	}
	return s.result.sites.rows[s.index].term, true
}

func (r *Result) siteAt(index int) (Site, bool) {
	if !r.available() || index < 0 || index >= len(r.sites.rows) {
		return Site{}, false
	}
	site := Site{result: r, index: uint32(index)}
	return site, site.available()
}

// SiteCount reports the sparse Causal site denominator: the union of From/To
// Terms over existing sealed routes plus sealed body-terminal Outcome Terms.
func (r *Result) SiteCount() int {
	if r == nil || !r.available() {
		return 0
	}
	return len(r.sites.rows)
}

// SiteAt returns one site in canonical Term order.
func (r *Result) SiteAt(index int) (Site, bool) { return r.siteAt(index) }

// OwnsSite accepts only a handle issued by this exact hot Causal Result.
// Equivalent replay sites are intentionally not owner-identical.
func (r *Result) OwnsSite(site Site) bool { return r != nil && site.result == r && site.available() }

func (r *Result) siteForTerm(term keyspace.Term) (Site, bool) {
	if !r.available() || keyspace.TermFamily(term) == keyspace.FamilyInvalid {
		return Site{}, false
	}
	index, found := r.sites.byTerm[term]
	if !found || uint64(index) >= uint64(len(r.sites.rows)) || r.sites.rows[index].term != term {
		return Site{}, false
	}
	return r.siteAt(int(index))
}

// SiteForTerm resolves only an actual endpoint of an existing sealed route or
// a sealed body-terminal Outcome Term. It does not invent sites for rootless
// metadata or for terms that merely share a Source root/cursor with another
// occurrence.
func (r *Result) SiteForTerm(term keyspace.Term) (Site, bool) { return r.siteForTerm(term) }

func (r *Result) resolveSite(id identity.ContentID) (Site, bool) {
	if !r.available() || !id.Available() {
		return Site{}, false
	}
	left := sort.Search(len(r.sites.lookups), func(index int) bool {
		return bytes.Compare(r.sites.lookups[index].context[:], id[:]) >= 0
	})
	found := -1
	for index := left; index < len(r.sites.lookups) && r.sites.lookups[index].context == id; index++ {
		if found != -1 {
			return Site{}, false
		}
		found = index
	}
	if found == -1 {
		return Site{}, false
	}
	return r.siteAt(int(r.sites.lookups[found].index))
}

// ResolveContextID performs exact-quartet-fenced O(log n) lookup by
// contextual endpoint identity.
func (r *Result) ResolveContextID(id identity.ContentID) (Site, bool) {
	return r.resolveSite(id)
}

// captureOutcomePhasePaths seals Outcome-to-phase paths before recurrence.
// The later Site/WTO phases consume this map and never
// reopen SourceControl or Outcome to reconstruct an attachment.
func (r *Result) captureOutcomePhasePaths(control *sourcecontrol.Result, outcomes *outcome.Result) error {
	if r == nil || control == nil || outcomes == nil || !sourcecontrol.Matches(control, r.sourceID, r.flowID, r.staticID, r.moduleID) || !outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return errors.New("program/flow/causal: terminal outcome row owners disagree")
	}
	paths := make(map[keyspace.Term]identity.ContentID)
	for index := 0; index < outcomes.Count(); index++ {
		term, ok := outcomes.At(index)
		if !ok {
			return errors.New("program/flow/causal: outcome row disappeared while building tail paths")
		}
		owner, outcomeKind, _, ok := outcomes.Get(term)
		if !ok {
			continue
		}
		switch outcomeKind {
		case kind.OutcomeNormal:
			path, pathOK := control.BodyTailPath(owner)
			if !pathOK {
				return errors.New("program/flow/causal: terminal Outcome Body tail path is unavailable")
			}
			paths[term] = path
		default:
			phase, phaseOK := control.OutcomePhase(term)
			path, pathOK := control.ResolvePhaseRef(phase)
			if !phaseOK || !pathOK {
				// Static/unreachable non-Normal Outcomes remain valid Sites but do
				// not have a parent-issued schedule point.
				continue
			}
			paths[term] = path
		}
	}
	r.outcomePhasePaths = paths
	return nil
}

func (r *Result) buildSites(sourceView source.View, control *sourcecontrol.Result, outcomes *outcome.Result) error {
	if r == nil || !r.available() || control == nil || outcomes == nil || !sourceView.Identity().ContentID().Available() {
		return errors.New("program/flow/causal: site owner is unavailable")
	}
	if !sourcecontrol.Matches(control, r.sourceID, r.flowID, r.staticID, r.moduleID) || sourceView.Identity().ContentID() != r.sourceID || !outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return errors.New("program/flow/causal: site prerequisite provenance disagrees")
	}

	// The existing successor index is the sole route authority. Gather its
	// endpoint Terms, then add the already-sealed body terminal coordinates
	// from Outcome. A terminal outcome is a valid causal site even when no
	// successor has it as a From/To endpoint; adding the existing coordinate
	// does not fabricate an edge or create another graph/index authority.
	if len(r.index.refs) > int(^uint(0)>>1)/2 {
		return errors.New("program/flow/causal: endpoint denominator overflows host index")
	}
	terms := make([]keyspace.Term, 0, len(r.index.refs)*2)
	if r.outcomePhasePaths == nil {
		return errors.New("program/flow/causal: terminal Outcome paths were not captured")
	}
	for _, routeRef := range r.index.refs {
		route, ok := r.successorForRef(routeRef)
		if !ok || keyspace.TermFamily(route.From) == keyspace.FamilyInvalid || keyspace.TermFamily(route.To) == keyspace.FamilyInvalid {
			return errors.New("program/flow/causal: successor endpoint is malformed")
		}
		terms = append(terms, route.From, route.To)
	}
	for outcomeTerm := range r.outcomePhasePaths {
		terms = append(terms, outcomeTerm)
	}
	seenTerms := make(map[keyspace.Term]struct{}, len(terms))
	unique := make([]keyspace.Term, 0, len(terms))
	for _, term := range terms {
		if _, exists := seenTerms[term]; !exists {
			seenTerms[term] = struct{}{}
			unique = append(unique, term)
		}
	}
	store := siteStore{rows: make([]siteRow, len(unique)), byTerm: make(map[keyspace.Term]uint32, len(unique))}
	for index, term := range unique {
		path, pathOK := r.semanticTermPath(term)
		if !pathOK {
			return errors.New("program/flow/causal: Site structural path is unavailable")
		}
		store.rows[index] = siteRow{
			term:    term,
			context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, term),
			path:    path,
		}
	}
	identity.SortByContentID(store.rows, siteRowPath)
	for index, row := range store.rows {
		if _, duplicate := store.byTerm[row.term]; duplicate {
			return errors.New("program/flow/causal: duplicate Site term")
		}
		store.byTerm[row.term] = uint32(index)
	}
	store.lookups = make([]siteLookup, len(store.rows))
	for index, row := range store.rows {
		store.lookups[index] = siteLookup{context: row.context, index: uint32(index)}
	}
	identity.SortByContentID(store.lookups, siteLookupContext)
	for index := 1; index < len(store.lookups); index++ {
		if store.lookups[index-1].context == store.lookups[index].context {
			return errors.New("program/flow/causal: contextual site digest collision")
		}
	}
	r.sites = store
	return nil
}

func siteRowPath(row siteRow) identity.ContentID { return row.path }

func siteLookupContext(row siteLookup) identity.ContentID { return row.context }
