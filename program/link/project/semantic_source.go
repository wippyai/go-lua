package project

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is one detached Project-owned source-family receipt.
// It stores only owner-fenced typed-row identities; Project rows and Programs
// never cross this boundary.
type SemanticSourceView struct {
	owner   keyspace.ContentID
	digests []keyspace.ContentID
}

func (view SemanticSourceView) valid() bool {
	if !view.owner.Available() {
		return false
	}
	for _, digest := range view.digests {
		if !digest.Available() {
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

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	return SemanticSourceCursor{view: view}
}

func (cursor *SemanticSourceCursor) Next() (keyspace.ContentID, bool) {
	if cursor == nil || !cursor.view.valid() || cursor.index < 0 || cursor.index >= len(cursor.view.digests) {
		return keyspace.ContentID{}, false
	}
	digest := cursor.view.digests[cursor.index]
	cursor.index++
	return digest, digest.Available()
}

// SemanticSourceViews names Project's two generated source families.
type SemanticSourceViews struct {
	owner           keyspace.ContentID
	shardMount      SemanticSourceView
	baseApplication SemanticSourceView
}

func (views SemanticSourceViews) valid() bool {
	return views.owner.Available() && views.shardMount.valid() && views.baseApplication.valid() &&
		views.shardMount.owner == views.owner && views.baseApplication.owner == views.owner
}

func (views SemanticSourceViews) OwnerID() keyspace.ContentID         { return views.owner }
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
	owner keyspace.ContentID
	views SemanticSourceViews
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
	rows := make([]semanticsource.Publication, 0, 2)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined {
			return nil
		}
		if definition.Token().Origin() != semanticsource.OriginLinkProjectShardMount && definition.Token().Origin() != semanticsource.OriginLinkProjectBaseApplication {
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
	definition, ok := semanticsource.Definition(origin, facet)
	if !ok {
		return semanticsource.Token{}, false
	}
	return definition.Token(), true
}

func projectRows(c *Component, token semanticsource.Token, mounts bool) (SemanticSourceView, bool) {
	view := SemanticSourceView{owner: c.authority.contentID}
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "wippy.link/project/semantic-source", 1) != nil || writer.Record(1) != nil || writer.Uint(uint64(token.Origin())) != nil || writer.Uint(uint64(token.Facet())) != nil || writer.Uint(uint64(token.Revision())) != nil || writer.Uint(token.Digest()) != nil {
		return SemanticSourceView{}, false
	}
	appendRow := func(index int, id keyspace.ContentID) bool {
		if !id.Available() || writer.Record(2) != nil || writer.Uint(uint64(index+1)) != nil || writer.Bytes(id[:]) != nil {
			return false
		}
		digest := keyspace.ContentID(sha256.Sum256(append([]byte("wippy.link/project/semantic-source-row"), id[:]...)))
		view.digests = append(view.digests, digest)
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
	if writer.Finish() != nil || !view.valid() {
		return SemanticSourceView{}, false
	}
	return view, true
}
