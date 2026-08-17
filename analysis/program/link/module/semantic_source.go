package module

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SourceViews is Module's sealed source-column set. Rows retain only
// owner-fenced identities; module hot tables do not cross this API.
type SourceViews struct {
	owner                                                                                             identity.ContentID
	module, cache, representative, transport, analysisRoot, initGeneration, initOutcome, initTerminal semanticsource.DigestView
}

func (views SourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner, views.module, views.cache, views.representative, views.transport,
		views.analysisRoot, views.initGeneration, views.initOutcome, views.initTerminal)
}
func (views SourceViews) Valid() bool                               { return views.owner.Available() && views.valid() }
func (views SourceViews) OwnerID() identity.ContentID               { return views.owner }
func (views SourceViews) Module() semanticsource.DigestView         { return views.module }
func (views SourceViews) Cache() semanticsource.DigestView          { return views.cache }
func (views SourceViews) Representative() semanticsource.DigestView { return views.representative }
func (views SourceViews) Transport() semanticsource.DigestView      { return views.transport }
func (views SourceViews) AnalysisRoot() semanticsource.DigestView   { return views.analysisRoot }
func (views SourceViews) InitGeneration() semanticsource.DigestView { return views.initGeneration }
func (views SourceViews) InitOutcome() semanticsource.DigestView    { return views.initOutcome }
func (views SourceViews) InitTerminal() semanticsource.DigestView   { return views.initTerminal }

func (views SourceViews) viewFor(token semanticsource.Token) (semanticsource.DigestView, bool) {
	if token.Origin() != semanticsource.OriginLinkModule {
		return semanticsource.DigestView{}, false
	}
	switch token.Facet() {
	case 0:
		return views.module, true
	case semanticsource.FacetLinkModuleCache:
		return views.cache, true
	case semanticsource.FacetLinkModuleRepresentative:
		return views.representative, true
	case semanticsource.FacetLinkModuleTransport:
		return views.transport, true
	case semanticsource.FacetLinkModuleAnalysisRoot:
		return views.analysisRoot, true
	case semanticsource.FacetLinkModuleInitGeneration:
		return views.initGeneration, true
	case semanticsource.FacetLinkModuleInitOutcome:
		return views.initOutcome, true
	case semanticsource.FacetLinkModuleInitTerminal:
		return views.initTerminal, true
	default:
		return semanticsource.DigestView{}, false
	}
}

func (views SourceViews) Publications(schema semanticsource.ProgramSchema) []semanticsource.Publication {
	if !views.Valid() {
		return nil
	}
	return semanticsource.OriginPublications(schema, func(token semanticsource.Token) (int, bool) {
		row, found := views.viewFor(token)
		if !found {
			return 0, false
		}
		return row.Count(), true
	}, semanticsource.OriginLinkModule)
}

func (c *Component) SourceViews() (SourceViews, bool) {
	if !live(c) || !c.authority.sourceViews.Valid() || c.authority.sourceViews.OwnerID() != c.authority.content {
		return SourceViews{}, false
	}
	return c.authority.sourceViews, true
}
func (v Cold) SourceViews() (SourceViews, bool) {
	if v.fence == nil || !v.fence.sealed || !v.content.Available() || !v.sourceViews.Valid() || v.sourceViews.OwnerID() != v.content {
		return SourceViews{}, false
	}
	return v.sourceViews, true
}

func sourceRowDigest(owner identity.ContentID, token semanticsource.Token, index int, id identity.ContentID) (identity.ContentID, bool) {
	if !owner.Available() || index < 0 || !id.Available() {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.link/module/semantic-source-row/v1"))
	_, _ = h.Write(owner[:])
	var frame [24]byte
	binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
	binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
	binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
	binary.BigEndian.PutUint64(frame[8:16], token.Digest())
	binary.BigEndian.PutUint64(frame[16:24], uint64(index))
	_, _ = h.Write(frame[:])
	_, _ = h.Write(id[:])
	var digest identity.ContentID
	copy(digest[:], h.Sum(nil))
	return digest, digest.Available()
}
func sourceRows(owner identity.ContentID, token semanticsource.Token, ids []identity.ContentID) (semanticsource.DigestView, bool) {
	digests := make([]identity.ContentID, 0, len(ids))
	for index, id := range ids {
		digest, ok := sourceRowDigest(owner, token, index, id)
		if !ok {
			return semanticsource.DigestView{}, false
		}
		digests = append(digests, digest)
	}
	return semanticsource.SealDigestView(owner, digests)
}
func moduleToken(origin semanticsource.Origin, facet semanticsource.Facet) (semanticsource.Token, bool) {
	d, ok := semanticsource.Declare(origin, facet)
	if !ok {
		return semanticsource.Token{}, false
	}
	return d.Token(), true
}
func (c *Component) buildSourceViews() (SourceViews, bool) {
	if !live(c) || !c.authority.content.Available() {
		return SourceViews{}, false
	}
	owner := c.authority.content
	views := SourceViews{owner: owner}
	makeIDs := func(count int, at func(int) (identity.ContentID, bool)) ([]identity.ContentID, bool) {
		ids := make([]identity.ContentID, 0, count)
		for index := 0; index < count; index++ {
			id, ok := at(index)
			if !ok {
				return nil, false
			}
			ids = append(ids, id)
		}
		return ids, true
	}
	set := func(origin semanticsource.Origin, facet semanticsource.Facet, ids []identity.ContentID, dst *semanticsource.DigestView) bool {
		token, ok := moduleToken(origin, facet)
		if !ok {
			return false
		}
		view, ok := sourceRows(owner, token, ids)
		if !ok {
			return false
		}
		*dst = view
		return true
	}
	ids, ok := makeIDs(c.Cache().EntryCount(), func(i int) (identity.ContentID, bool) {
		e, ok := c.Cache().EntryAt(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Cache().EntryID(e)
	})
	if !ok || !set(semanticsource.OriginLinkModule, 0, ids, &views.module) {
		return SourceViews{}, false
	}
	ids, ok = makeIDs(c.Cache().InstanceCount(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Cache().InstanceAt(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Cache().InstanceID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, ids, &views.cache) {
		return SourceViews{}, false
	}
	ids, ok = makeIDs(c.Cache().InstanceCount(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Cache().InstanceAt(i)
		if !ok {
			return identity.ContentID{}, false
		}
		rep, ok := c.Cache().Representative(x)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Cache().InstanceID(rep)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative, ids, &views.representative) {
		return SourceViews{}, false
	}
	ids, ok = makeIDs(c.Coordinates().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Coordinates().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Coordinates().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, ids, &views.transport) {
		return SourceViews{}, false
	}
	ids, ok = makeIDs(c.Roots().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Roots().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Roots().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot, ids, &views.analysisRoot) {
		return SourceViews{}, false
	}
	ids, ok = makeIDs(c.Generations().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Generations().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Generations().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, ids, &views.initGeneration) {
		return SourceViews{}, false
	}
	var outcomes []identity.ContentID
	for i := 0; i < c.Generations().Count(); i++ {
		g, good := c.Generations().At(i)
		if !good {
			return SourceViews{}, false
		}
		for j := 0; j < c.Outcomes().Count(g); j++ {
			o, good := c.Outcomes().At(g, j)
			if !good {
				return SourceViews{}, false
			}
			id, good := c.Outcomes().ID(o)
			if !good {
				return SourceViews{}, false
			}
			outcomes = append(outcomes, id)
		}
	}
	if !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, outcomes, &views.initOutcome) {
		return SourceViews{}, false
	}
	ids, ok = makeIDs(c.Terminals().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Terminals().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Terminals().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, ids, &views.initTerminal) {
		return SourceViews{}, false
	}
	return views, views.Valid()
}
