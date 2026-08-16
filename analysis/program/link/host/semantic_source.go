package host

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SemanticSourceView is a detached Host-owned typed Count/At receipt. It
// stores only row digests; the Host selector, bootstrap, and endpoint rows do
// not cross the child boundary.
type SemanticSourceView = semanticsource.DigestView

// SemanticSourceCursor walks one exact Host-owned digest interval.
type SemanticSourceCursor = semanticsource.DigestCursor

type SemanticSourceViews struct {
	owner                                        identity.ContentID
	host, exposure, boot, member, endpointTarget SemanticSourceView
}

func (views SemanticSourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner, views.host, views.exposure, views.boot, views.member, views.endpointTarget)
}
func (views SemanticSourceViews) OwnerID() identity.ContentID        { return views.owner }
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

// Publications projects this owner through the injected sealed ProgramSchema.
// Host contributes only detached row cardinalities; relation membership and
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
	}, semanticsource.OriginLinkHost)
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

func hostReceiptRows(owner identity.ContentID, token semanticsource.Token, count int, at func(int) bool) (SemanticSourceView, bool) {
	if !owner.Available() || count < 0 || at == nil {
		return SemanticSourceView{}, false
	}
	digests := make([]identity.ContentID, 0, count)
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
		var id identity.ContentID
		copy(id[:], h.Sum(nil))
		if !id.Available() {
			return SemanticSourceView{}, false
		}
		digests = append(digests, id)
	}
	return semanticsource.SealDigestView(owner, digests)
}
func hostToken(facet semanticsource.Facet) (semanticsource.Token, bool) {
	d, ok := semanticsource.Declare(semanticsource.OriginLinkHost, facet)
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
