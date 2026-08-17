package project

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SourceViews is the sealed Project-owned source-column set. It contains only
// owner-fenced row identities; Project rows and Programs never cross this API.
type SourceViews struct {
	owner           identity.ContentID
	shardMount      semanticsource.DigestView
	baseApplication semanticsource.DigestView
}

func (views SourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner, views.shardMount, views.baseApplication)
}
func (views SourceViews) Valid() bool {
	return views.owner.Available() && views.valid()
}
func (views SourceViews) OwnerID() identity.ContentID                { return views.owner }
func (views SourceViews) ShardMount() semanticsource.DigestView      { return views.shardMount }
func (views SourceViews) Mount() semanticsource.DigestView           { return views.shardMount }
func (views SourceViews) BaseApplication() semanticsource.DigestView { return views.baseApplication }

func (views SourceViews) viewFor(token semanticsource.Token) (semanticsource.DigestView, bool) {
	switch token.Origin() {
	case semanticsource.OriginLinkProjectShardMount:
		if token.Facet() == 0 {
			return views.shardMount, true
		}
	case semanticsource.OriginLinkProjectBaseApplication:
		if token.Facet() == 0 {
			return views.baseApplication, true
		}
	}
	return semanticsource.DigestView{}, false
}

// Publications projects the Project source columns through the sealed schema.
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
	}, semanticsource.OriginLinkProjectShardMount, semanticsource.OriginLinkProjectBaseApplication)
}

func (c *Component) SourceViews() (SourceViews, bool) {
	if c == nil || c.authority == nil || !c.authority.contentID.Available() || !c.authority.sourceViews.Valid() || c.authority.sourceViews.OwnerID() != c.authority.contentID {
		return SourceViews{}, false
	}
	return c.authority.sourceViews, true
}

func (c *Component) buildSourceViews() (SourceViews, error) {
	if c == nil || c.authority == nil || !c.authority.contentID.Available() {
		return SourceViews{}, errors.New("link/project: unavailable semantic-source owner")
	}
	owner := c.authority.contentID
	views := SourceViews{owner: owner}
	mountToken, mountOK := projectSourceToken(semanticsource.OriginLinkProjectShardMount, 0)
	baseToken, baseOK := projectSourceToken(semanticsource.OriginLinkProjectBaseApplication, 0)
	if !mountOK || !baseOK {
		return SourceViews{}, errors.New("link/project: unavailable semantic-source token")
	}
	views.shardMount, mountOK = projectRows(c, mountToken, true)
	views.baseApplication, baseOK = projectRows(c, baseToken, false)
	if !mountOK || !baseOK {
		return SourceViews{}, errors.New("link/project: unavailable semantic-source rows")
	}
	if !views.Valid() {
		return SourceViews{}, errors.New("link/project: malformed semantic-source rows")
	}
	return views, nil
}

func (v Cold) SourceViews() (SourceViews, bool) {
	if !v.live() || !v.sourceViews.Valid() || v.sourceViews.OwnerID() != v.contentID {
		return SourceViews{}, false
	}
	return v.sourceViews, true
}

func projectSourceToken(origin semanticsource.Origin, facet semanticsource.Facet) (semanticsource.Token, bool) {
	definition, ok := semanticsource.Declare(origin, facet)
	if !ok {
		return semanticsource.Token{}, false
	}
	return definition.Token(), true
}

func projectRows(c *Component, token semanticsource.Token, mounts bool) (semanticsource.DigestView, bool) {
	owner := c.authority.contentID
	var digests []identity.ContentID
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "wippy.link/project/semantic-source", 1) != nil || writer.Record(1) != nil || writer.Uint(uint64(token.Origin())) != nil || writer.Uint(uint64(token.Facet())) != nil || writer.Uint(uint64(token.Revision())) != nil || writer.Uint(token.Digest()) != nil {
		return semanticsource.DigestView{}, false
	}
	appendRow := func(index int, id identity.ContentID) bool {
		if !id.Available() || writer.Record(2) != nil || writer.Uint(uint64(index+1)) != nil || writer.Bytes(id[:]) != nil {
			return false
		}
		digest := identity.ContentID(sha256.Sum256(append([]byte("wippy.link/project/semantic-source-row"), id[:]...)))
		digests = append(digests, digest)
		return true
	}
	if mounts {
		mountsView := c.Mounts()
		for index := 0; index < mountsView.Count(); index++ {
			shard, ok := mountsView.At(index)
			id, idOK := c.ModuleKey(shard)
			if !ok || !idOK || !appendRow(index, id) {
				return semanticsource.DigestView{}, false
			}
		}
	} else {
		bases := c.Applications().Bases()
		for index := 0; index < bases.Count(); index++ {
			application, ok := bases.At(index)
			id, idOK := c.ApplicationID(application)
			if !ok || !idOK || !appendRow(index, id) {
				return semanticsource.DigestView{}, false
			}
		}
	}
	if writer.Finish() != nil {
		return semanticsource.DigestView{}, false
	}
	return semanticsource.SealDigestView(owner, digests)
}
