package host

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is a detached Host-owned typed Count/At receipt. It
// stores only row digests; the Host selector, bootstrap, and endpoint rows do
// not cross the child boundary.
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
	owner                                        keyspace.ContentID
	host, exposure, boot, member, endpointTarget SemanticSourceView
}

func (views SemanticSourceViews) all() []SemanticSourceView {
	return []SemanticSourceView{views.host, views.exposure, views.boot, views.member, views.endpointTarget}
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
func (views SemanticSourceViews) Host() SemanticSourceView           { return views.host }
func (views SemanticSourceViews) Exposure() SemanticSourceView       { return views.exposure }
func (views SemanticSourceViews) Boot() SemanticSourceView           { return views.boot }
func (views SemanticSourceViews) Member() SemanticSourceView         { return views.member }
func (views SemanticSourceViews) EndpointTarget() SemanticSourceView { return views.endpointTarget }
func (views SemanticSourceViews) viewFor(token semanticsource.Token) (SemanticSourceView, bool) {
	if token.Origin() != semanticsource.OriginLinkHost {
		return SemanticSourceView{}, false
	}
	switch token.Facet() {
	case 0:
		return views.host, true
	case semanticsource.FacetLinkHostExposure:
		return views.exposure, true
	case semanticsource.FacetLinkHostBoot:
		return views.boot, true
	case semanticsource.FacetLinkHostMember:
		return views.member, true
	case semanticsource.FacetLinkHostEndpointTarget:
		return views.endpointTarget, true
	default:
		return SemanticSourceView{}, false
	}
}

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
func (receipt SemanticSourceReceipt) Publications() []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	views, ok := receipt.Views()
	if !ok {
		return nil
	}
	schema := semanticsource.CatalogSchema()
	rows := make([]semanticsource.Publication, 0, 5)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined || definition.Token().Origin() != semanticsource.OriginLinkHost {
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

func hostReceiptRows(owner keyspace.ContentID, token semanticsource.Token, count int, at func(int) bool) (SemanticSourceView, bool) {
	if !owner.Available() || count < 0 || at == nil {
		return SemanticSourceView{}, false
	}
	view := SemanticSourceView{owner: owner, digests: make([]keyspace.ContentID, 0, count)}
	for index := 0; index < count; index++ {
		if !at(index) {
			return SemanticSourceView{}, false
		}
		h := sha256.New()
		_, _ = h.Write([]byte("wippy.link/host/semantic-source-row/v1"))
		_, _ = h.Write(owner[:])
		var frame [24]byte
		binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
		binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
		binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
		binary.BigEndian.PutUint64(frame[8:16], token.Digest())
		binary.BigEndian.PutUint64(frame[16:24], uint64(index))
		_, _ = h.Write(frame[:])
		var id keyspace.ContentID
		copy(id[:], h.Sum(nil))
		if !id.Available() {
			return SemanticSourceView{}, false
		}
		view.digests = append(view.digests, id)
	}
	return view, view.valid()
}
func hostToken(facet semanticsource.Facet) (semanticsource.Token, bool) {
	d, ok := semanticsource.Definition(semanticsource.OriginLinkHost, facet)
	if !ok {
		return semanticsource.Token{}, false
	}
	return d.Token(), true
}
func (c *Component) buildSemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if !live(c) || !c.authority.content.Available() || c.authority.boundary == nil {
		return SemanticSourceReceipt{}, false
	}
	owner := c.authority.content
	views := SemanticSourceViews{owner: owner}
	token, ok := hostToken(0)
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	endpointCount := c.authority.boundary.Endpoints().Count()
	views.host, ok = hostReceiptRows(owner, token, endpointCount, func(i int) bool { _, good := c.authority.boundary.Endpoints().At(i); return good })
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostExposure)
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	views.exposure, ok = hostReceiptRows(owner, token, c.Exposures().Count(), func(i int) bool { _, _, _, _, _, good := c.Exposures().At(i); return good })
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostBoot)
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	views.boot, ok = hostReceiptRows(owner, token, c.Globals().Count(), func(i int) bool {
		row, good := c.Globals().At(i)
		if !good {
			return false
		}
		_, _, _, _, _, _, mapped := c.Globals().Mapping(row)
		return mapped
	})
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostMember)
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	views.member, ok = hostReceiptRows(owner, token, c.Members().Count(), func(i int) bool { _, _, _, _, _, _, _, good := c.Members().At(i); return good })
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostEndpointTarget)
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	views.endpointTarget, ok = hostReceiptRows(owner, token, endpointCount, func(i int) bool { _, good := c.authority.boundary.Endpoints().At(i); return good })
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	receipt := SemanticSourceReceipt{owner: owner, views: views}
	return receipt, receipt.Valid()
}
