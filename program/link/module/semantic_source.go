package module

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is one detached Module-owned typed relation receipt.
// Digests authenticate the exact typed At/ID traversal without copying rows.
type SemanticSourceView struct {
	owner   keyspace.ContentID
	digests []keyspace.ContentID
}

func (view SemanticSourceView) valid() bool {
	if !view.owner.Available() {
		return false
	}
	for _, id := range view.digests {
		if !id.Available() {
			return false
		}
	}
	return true
}
func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.owner }
func (view SemanticSourceView) Count() int {
	if !view.valid() {
		return 0
	}
	return len(view.digests)
}
func (view SemanticSourceView) DigestAt(index int) (keyspace.ContentID, bool) {
	if !view.valid() || index < 0 || index >= len(view.digests) {
		return keyspace.ContentID{}, false
	}
	return view.digests[index], true
}
func (view SemanticSourceView) Digests() []keyspace.ContentID {
	if !view.valid() {
		return nil
	}
	return append([]keyspace.ContentID(nil), view.digests...)
}

type SemanticSourceCursor struct {
	view  SemanticSourceView
	index int
}

func (view SemanticSourceView) Cursor() SemanticSourceCursor { return SemanticSourceCursor{view: view} }
func (cursor *SemanticSourceCursor) Next() (keyspace.ContentID, bool) {
	if cursor == nil || !cursor.view.valid() || cursor.index < 0 || cursor.index >= len(cursor.view.digests) {
		return keyspace.ContentID{}, false
	}
	id := cursor.view.digests[cursor.index]
	cursor.index++
	return id, true
}

type SemanticSourceViews struct {
	owner                                                                                             keyspace.ContentID
	module, cache, representative, transport, analysisRoot, initGeneration, initOutcome, initTerminal SemanticSourceView
}

func (views SemanticSourceViews) all() []SemanticSourceView {
	return []SemanticSourceView{views.module, views.cache, views.representative, views.transport, views.analysisRoot, views.initGeneration, views.initOutcome, views.initTerminal}
}
func (views SemanticSourceViews) valid() bool {
	if !views.owner.Available() {
		return false
	}
	for _, view := range views.all() {
		if !view.valid() || view.owner != views.owner {
			return false
		}
	}
	return true
}
func (views SemanticSourceViews) OwnerID() keyspace.ContentID        { return views.owner }
func (views SemanticSourceViews) Module() SemanticSourceView         { return views.module }
func (views SemanticSourceViews) Cache() SemanticSourceView          { return views.cache }
func (views SemanticSourceViews) Representative() SemanticSourceView { return views.representative }
func (views SemanticSourceViews) Transport() SemanticSourceView      { return views.transport }
func (views SemanticSourceViews) AnalysisRoot() SemanticSourceView   { return views.analysisRoot }
func (views SemanticSourceViews) InitGeneration() SemanticSourceView { return views.initGeneration }
func (views SemanticSourceViews) InitOutcome() SemanticSourceView    { return views.initOutcome }
func (views SemanticSourceViews) InitTerminal() SemanticSourceView   { return views.initTerminal }

type SemanticSourceReceipt struct {
	owner keyspace.ContentID
	views SemanticSourceViews
}

func (receipt SemanticSourceReceipt) Valid() bool {
	return receipt.owner.Available() && receipt.views.valid() && receipt.views.owner == receipt.owner
}
func (receipt SemanticSourceReceipt) OwnerID() keyspace.ContentID { return receipt.owner }
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
func (receipt SemanticSourceReceipt) Publications() []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	views, ok := receipt.Views()
	if !ok {
		return nil
	}
	schema := semanticsource.CatalogSchema()
	rows := make([]semanticsource.Publication, 0, 8)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined || definition.Token().Origin() != semanticsource.OriginLinkModule {
			continue
		}
		view, found := views.viewFor(definition.Token())
		if !found {
			return nil
		}
		publication, err := semanticsource.SealPublication(definition, view.Count())
		if err != nil {
			return nil
		}
		rows = append(rows, publication)
	}
	return rows
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

func moduleReceiptDigest(owner keyspace.ContentID, token semanticsource.Token, index int, id keyspace.ContentID) (keyspace.ContentID, bool) {
	if !owner.Available() || index < 0 || !id.Available() {
		return keyspace.ContentID{}, false
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
	var digest keyspace.ContentID
	copy(digest[:], h.Sum(nil))
	return digest, digest.Available()
}
func moduleReceiptRows(owner keyspace.ContentID, token semanticsource.Token, ids []keyspace.ContentID) (SemanticSourceView, bool) {
	view := SemanticSourceView{owner: owner, digests: make([]keyspace.ContentID, 0, len(ids))}
	for index, id := range ids {
		digest, ok := moduleReceiptDigest(owner, token, index, id)
		if !ok {
			return SemanticSourceView{}, false
		}
		view.digests = append(view.digests, digest)
	}
	return view, view.valid()
}
func moduleToken(origin semanticsource.Origin, facet semanticsource.Facet) (semanticsource.Token, bool) {
	d, ok := semanticsource.Definition(origin, facet)
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
	makeIDs := func(count int, at func(int) (keyspace.ContentID, bool)) ([]keyspace.ContentID, bool) {
		ids := make([]keyspace.ContentID, 0, count)
		for index := 0; index < count; index++ {
			id, ok := at(index)
			if !ok {
				return nil, false
			}
			ids = append(ids, id)
		}
		return ids, true
	}
	set := func(origin semanticsource.Origin, facet semanticsource.Facet, ids []keyspace.ContentID, dst *SemanticSourceView) bool {
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
	ids, ok := makeIDs(c.Cache().EntryCount(), func(i int) (keyspace.ContentID, bool) {
		e, ok := c.Cache().EntryAt(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Cache().EntryID(e)
	})
	if !ok || !set(semanticsource.OriginLinkModule, 0, ids, &views.module) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Cache().InstanceCount(), func(i int) (keyspace.ContentID, bool) {
		x, ok := c.Cache().InstanceAt(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Cache().InstanceID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, ids, &views.cache) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Cache().InstanceCount(), func(i int) (keyspace.ContentID, bool) {
		x, ok := c.Cache().InstanceAt(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		rep, ok := c.Cache().Representative(x)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Cache().InstanceID(rep)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative, ids, &views.representative) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Coordinates().Count(), func(i int) (keyspace.ContentID, bool) {
		x, ok := c.Coordinates().At(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Coordinates().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport, ids, &views.transport) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Roots().Count(), func(i int) (keyspace.ContentID, bool) {
		x, ok := c.Roots().At(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Roots().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot, ids, &views.analysisRoot) {
		return SemanticSourceReceipt{}, false
	}
	ids, ok = makeIDs(c.Generations().Count(), func(i int) (keyspace.ContentID, bool) {
		x, ok := c.Generations().At(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Generations().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration, ids, &views.initGeneration) {
		return SemanticSourceReceipt{}, false
	}
	var outcomes []keyspace.ContentID
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
	ids, ok = makeIDs(c.Terminals().Count(), func(i int) (keyspace.ContentID, bool) {
		x, ok := c.Terminals().At(i)
		if !ok {
			return keyspace.ContentID{}, false
		}
		return c.Terminals().ID(x)
	})
	if !ok || !set(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal, ids, &views.initTerminal) {
		return SemanticSourceReceipt{}, false
	}
	receipt := SemanticSourceReceipt{owner: owner, views: views}
	return receipt, receipt.Valid()
}
