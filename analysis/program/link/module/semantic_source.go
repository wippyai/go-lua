package module

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SemanticSourceView is one detached Module-owned typed relation receipt.
// Digests authenticate the exact typed At/ID traversal without copying rows.
type SemanticSourceView = semanticsource.DigestView

// SemanticSourceCursor walks one exact Module-owned digest interval.
type SemanticSourceCursor = semanticsource.DigestCursor

type SemanticSourceViews struct {
	owner                                                                                             identity.ContentID
	module, cache, representative, transport, analysisRoot, initGeneration, initOutcome, initTerminal SemanticSourceView
}

func (views SemanticSourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner, views.module, views.cache, views.representative, views.transport,
		views.analysisRoot, views.initGeneration, views.initOutcome, views.initTerminal)
}
func (views SemanticSourceViews) OwnerID() identity.ContentID        { return views.owner }
func (views SemanticSourceViews) Module() SemanticSourceView         { return views.module }
func (views SemanticSourceViews) Cache() SemanticSourceView          { return views.cache }
func (views SemanticSourceViews) Representative() SemanticSourceView { return views.representative }
func (views SemanticSourceViews) Transport() SemanticSourceView      { return views.transport }
func (views SemanticSourceViews) AnalysisRoot() SemanticSourceView   { return views.analysisRoot }
func (views SemanticSourceViews) InitGeneration() SemanticSourceView { return views.initGeneration }
func (views SemanticSourceViews) InitOutcome() SemanticSourceView    { return views.initOutcome }
func (views SemanticSourceViews) InitTerminal() SemanticSourceView   { return views.initTerminal }

type SemanticSourceReceipt struct {
	owner identity.ContentID
	views SemanticSourceViews
}

func (receipt SemanticSourceReceipt) Valid() bool {
	return receipt.owner.Available() && receipt.views.valid() && receipt.views.owner == receipt.owner
}
func (receipt SemanticSourceReceipt) OwnerID() identity.ContentID { return receipt.owner }
func (receipt SemanticSourceReceipt) Views() (SemanticSourceViews, bool) {
	if !receipt.Valid() {
		return SemanticSourceViews{}, false
	}
	return receipt.views, true
}
func (views SemanticSourceViews) viewFor(token semanticsource.Token) (SemanticSourceView, bool) {
	if token.Origin() != semanticsource.OriginLinkModule {
		return SemanticSourceView{}, false
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
		return SemanticSourceView{}, false
	}
}

// Publications projects this owner through the injected sealed ProgramSchema.
// Module contributes only detached row cardinalities; relation membership and
// order remain owned by the schema.
func (receipt SemanticSourceReceipt) Publications(schema semanticsource.ProgramSchema) []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	views, ok := receipt.Views()
	if !ok {
		return nil
	}
	return semanticsource.OriginPublications(schema, func(token semanticsource.Token) (int, bool) {
		view, found := views.viewFor(token)
		if !found {
			return 0, false
		}
		return view.Count(), true
	}, semanticsource.OriginLinkModule)
}
func (c *Component) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if !live(c) || !c.authority.semanticReceipt.Valid() || c.authority.semanticReceipt.OwnerID() != c.authority.content {
		return SemanticSourceReceipt{}, false
	}
	return c.authority.semanticReceipt, true
}
func (c *Component) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := c.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}
func (v Cold) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if v.fence == nil || !v.fence.sealed || !v.content.Available() || !v.semanticReceipt.Valid() || v.semanticReceipt.OwnerID() != v.content {
		return SemanticSourceReceipt{}, false
	}
	return v.semanticReceipt, true
}
func (v Cold) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := v.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}

func moduleReceiptDigest(owner identity.ContentID, token semanticsource.Token, index int, id identity.ContentID) (identity.ContentID, bool) {
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
func moduleReceiptRows(owner identity.ContentID, token semanticsource.Token, ids []identity.ContentID) (SemanticSourceView, bool) {
	digests := make([]identity.ContentID, 0, len(ids))
	for index, id := range ids {
		digest, ok := moduleReceiptDigest(owner, token, index, id)
		if !ok {
			return SemanticSourceView{}, false
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
func (c *Component) buildSemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if !live(c) || !c.authority.content.Available() {
		return SemanticSourceReceipt{}, false
	}
	owner := c.authority.content
	views := SemanticSourceViews{owner: owner}
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
	set := func(origin semanticsource.Origin, facet semanticsource.Facet, ids []identity.ContentID, dst *SemanticSourceView) bool {
		token, ok := moduleToken(origin, facet)
		if !ok {
			return false
		}
		view, ok := moduleReceiptRows(owner, token, ids)
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
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Cache().InstanceCount(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Cache().InstanceAt(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Cache().InstanceID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, ids, &views.cache) {
		return SemanticSourceReceipt{}, false
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
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Coordinates().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Coordinates().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Coordinates().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, ids, &views.transport) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Roots().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Roots().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Roots().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot, ids, &views.analysisRoot) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Generations().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Generations().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Generations().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, ids, &views.initGeneration) {
		return SemanticSourceReceipt{}, false
	}
	var outcomes []identity.ContentID
	for i := 0; i < c.Generations().Count(); i++ {
		g, good := c.Generations().At(i)
		if !good {
			return SemanticSourceReceipt{}, false
		}
		for j := 0; j < c.Outcomes().Count(g); j++ {
			o, good := c.Outcomes().At(g, j)
			if !good {
				return SemanticSourceReceipt{}, false
			}
			id, good := c.Outcomes().ID(o)
			if !good {
				return SemanticSourceReceipt{}, false
			}
			outcomes = append(outcomes, id)
		}
	}
	if !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome, outcomes, &views.initOutcome) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Terminals().Count(), func(i int) (identity.ContentID, bool) {
		x, ok := c.Terminals().At(i)
		if !ok {
			return identity.ContentID{}, false
		}
		return c.Terminals().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, ids, &views.initTerminal) {
		return SemanticSourceReceipt{}, false
	}
	receipt := SemanticSourceReceipt{owner: owner, views: views}
	return receipt, receipt.Valid()
}
