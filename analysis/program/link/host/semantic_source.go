package host

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SourceViews is Host's sealed source-column set. It retains only detached
// row identities; host selectors and endpoint tables remain private.
type SourceViews struct {
	owner                                        identity.ContentID
	host, exposure, boot, member, endpointTarget semanticsource.DigestView
}

func (views SourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner, views.host, views.exposure, views.boot, views.member, views.endpointTarget)
}
func (views SourceViews) Valid() bool                               { return views.owner.Available() && views.valid() }
func (views SourceViews) OwnerID() identity.ContentID               { return views.owner }
func (views SourceViews) Host() semanticsource.DigestView           { return views.host }
func (views SourceViews) Exposure() semanticsource.DigestView       { return views.exposure }
func (views SourceViews) Boot() semanticsource.DigestView           { return views.boot }
func (views SourceViews) Member() semanticsource.DigestView         { return views.member }
func (views SourceViews) EndpointTarget() semanticsource.DigestView { return views.endpointTarget }

func (views SourceViews) viewFor(token semanticsource.Token) (semanticsource.DigestView, bool) {
	if token.Origin() != semanticsource.OriginLinkHost {
		return semanticsource.DigestView{}, false
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
	}, semanticsource.OriginLinkHost)
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

func sourceRows(owner identity.ContentID, token semanticsource.Token, count int, at func(int) bool) (semanticsource.DigestView, bool) {
	if !owner.Available() || count < 0 || at == nil {
		return semanticsource.DigestView{}, false
	}
	digests := make([]identity.ContentID, 0, count)
	for index := 0; index < count; index++ {
		if !at(index) {
			return semanticsource.DigestView{}, false
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
			return semanticsource.DigestView{}, false
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
func (c *Component) buildSourceViews() (SourceViews, bool) {
	if !live(c) || !c.authority.content.Available() || c.authority.boundary == nil {
		return SourceViews{}, false
	}
	owner := c.authority.content
	views := SourceViews{owner: owner}
	token, ok := hostToken(0)
	if !ok {
		return SourceViews{}, false
	}
	endpointCount := c.authority.boundary.Endpoints().Count()
	views.host, ok = sourceRows(owner, token, endpointCount, func(i int) bool { _, good := c.authority.boundary.Endpoints().At(i); return good })
	if !ok {
		return SourceViews{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostExposure)
	if !ok {
		return SourceViews{}, false
	}
	views.exposure, ok = sourceRows(owner, token, c.Exposures().Count(), func(i int) bool { _, _, _, _, _, good := c.Exposures().At(i); return good })
	if !ok {
		return SourceViews{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostBoot)
	if !ok {
		return SourceViews{}, false
	}
	views.boot, ok = sourceRows(owner, token, c.Globals().Count(), func(i int) bool {
		row, good := c.Globals().At(i)
		if !good {
			return false
		}
		_, _, _, _, _, _, mapped := c.Globals().Mapping(row)
		return mapped
	})
	if !ok {
		return SourceViews{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostMember)
	if !ok {
		return SourceViews{}, false
	}
	views.member, ok = sourceRows(owner, token, c.Members().Count(), func(i int) bool { _, _, _, _, _, _, _, good := c.Members().At(i); return good })
	if !ok {
		return SourceViews{}, false
	}
	token, ok = hostToken(semanticsource.FacetLinkHostEndpointTarget)
	if !ok {
		return SourceViews{}, false
	}
	views.endpointTarget, ok = sourceRows(owner, token, endpointCount, func(i int) bool { _, good := c.authority.boundary.Endpoints().At(i); return good })
	if !ok {
		return SourceViews{}, false
	}
	return views, views.Valid()
}
