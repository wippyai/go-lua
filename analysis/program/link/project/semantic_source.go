package project

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SemanticSourceView is one detached Project-owned source-family receipt.
// It stores only owner-fenced typed-row identities; Project rows and Programs
// never cross this boundary.
type SemanticSourceView = semanticsource.DigestView

// SemanticSourceCursor walks one exact Project-owned digest interval.
type SemanticSourceCursor = semanticsource.DigestCursor

// SemanticSourceViews names Project's two generated source families.
type SemanticSourceViews struct {
	owner           identity.ContentID
	shardMount      SemanticSourceView
	baseApplication SemanticSourceView
}

func (views SemanticSourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner, views.shardMount, views.baseApplication)
}

func (views SemanticSourceViews) OwnerID() identity.ContentID         { return views.owner }
func (views SemanticSourceViews) ShardMount() SemanticSourceView      { return views.shardMount }
func (views SemanticSourceViews) Mount() SemanticSourceView           { return views.shardMount }
func (views SemanticSourceViews) BaseApplication() SemanticSourceView { return views.baseApplication }

func (views SemanticSourceViews) viewFor(token semanticsource.Token) (SemanticSourceView, bool) {
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
	return SemanticSourceView{}, false
}

type SemanticSourceReceipt struct {
	owner identity.ContentID
	views SemanticSourceViews
}

// Publications projects this owner through the injected sealed ProgramSchema.
// The child owns row cardinalities; the schema owns relation membership and
// canonical enumeration.
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
	}, semanticsource.OriginLinkProjectShardMount, semanticsource.OriginLinkProjectBaseApplication)
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

func (c *Component) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if c == nil || c.authority == nil || !c.authority.contentID.Available() || c.authority.semanticReceipt.Valid() == false {
		return SemanticSourceReceipt{}, false
	}
	return c.authority.semanticReceipt, true
}

func (c *Component) buildSemanticSourceReceipt() (SemanticSourceReceipt, error) {
	if c == nil || c.authority == nil || !c.authority.contentID.Available() {
		return SemanticSourceReceipt{}, errors.New("link/project: unavailable semantic-source owner")
	}
	owner := c.authority.contentID
	views := SemanticSourceViews{owner: owner}
	mountToken, mountOK := projectSourceToken(semanticsource.OriginLinkProjectShardMount, 0)
	baseToken, baseOK := projectSourceToken(semanticsource.OriginLinkProjectBaseApplication, 0)
	if !mountOK || !baseOK {
		return SemanticSourceReceipt{}, errors.New("link/project: unavailable semantic-source token")
	}
	views.shardMount, mountOK = projectRows(c, mountToken, true)
	views.baseApplication, baseOK = projectRows(c, baseToken, false)
	if !mountOK || !baseOK {
		return SemanticSourceReceipt{}, errors.New("link/project: unavailable semantic-source rows")
	}
	receipt := SemanticSourceReceipt{owner: owner, views: views}
	if !receipt.Valid() {
		return SemanticSourceReceipt{}, errors.New("link/project: malformed semantic-source receipt")
	}
	return receipt, nil
}

func (c *Component) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := c.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}

func (v Cold) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if !v.live() || !v.semanticReceipt.Valid() || v.semanticReceipt.OwnerID() != v.contentID {
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

func projectSourceToken(origin semanticsource.Origin, facet semanticsource.Facet) (semanticsource.Token, bool) {
	definition, ok := semanticsource.Declare(origin, facet)
	if !ok {
		return semanticsource.Token{}, false
	}
	return definition.Token(), true
}

func projectRows(c *Component, token semanticsource.Token, mounts bool) (SemanticSourceView, bool) {
	owner := c.authority.contentID
	var digests []identity.ContentID
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "wippy.link/project/semantic-source", 1) != nil || writer.Record(1) != nil || writer.Uint(uint64(token.Origin())) != nil || writer.Uint(uint64(token.Facet())) != nil || writer.Uint(uint64(token.Revision())) != nil || writer.Uint(token.Digest()) != nil {
		return SemanticSourceView{}, false
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
				return SemanticSourceView{}, false
			}
		}
	} else {
		bases := c.Applications().Bases()
		for index := 0; index < bases.Count(); index++ {
			application, ok := bases.At(index)
			id, idOK := c.ApplicationID(application)
			if !ok || !idOK || !appendRow(index, id) {
				return SemanticSourceView{}, false
			}
		}
	}
	if writer.Finish() != nil {
		return SemanticSourceView{}, false
	}
	return semanticsource.SealDigestView(owner, digests)
}
