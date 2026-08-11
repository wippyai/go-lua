package boundary

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is the detached count receipt for Boundary's virtual
// Application×Operation predicate. The predicate is intentionally factorized;
// there is no row identity or copied product relation to detach.
type SemanticSourceView struct {
	owner keyspace.ContentID
	count int
}

func (view SemanticSourceView) valid() bool {
	return view.owner.Available() && view.count >= 0
}
func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.owner }
func (view SemanticSourceView) Count() int {
	if !view.valid() {
		return 0
	}
	return view.count
}

// DigestAt is deliberately unavailable for a virtual predicate: Boundary
// owns a cardinality claim, not a materialized Application×Operation table.
func (view SemanticSourceView) DigestAt(index int) (keyspace.ContentID, bool) {
	return keyspace.ContentID{}, false
}
func (view SemanticSourceView) Digests() []keyspace.ContentID { return nil }

type SemanticSourceViews struct {
	owner    keyspace.ContentID
	boundary SemanticSourceView
}

func (views SemanticSourceViews) valid() bool {
	return views.owner.Available() && views.boundary.valid() && views.boundary.owner == views.owner
}
func (views SemanticSourceViews) OwnerID() keyspace.ContentID  { return views.owner }
func (views SemanticSourceViews) Boundary() SemanticSourceView { return views.boundary }

type SemanticSourceReceipt struct {
	owner keyspace.ContentID
	views SemanticSourceViews
}

func (receipt SemanticSourceReceipt) Publications() []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	definition, ok := semanticsource.Definition(semanticsource.OriginLinkBoundary, 0)
	if !ok {
		return nil
	}
	publication, err := semanticsource.SealPublication(definition, receipt.views.boundary.Count())
	if err != nil {
		return nil
	}
	return []semanticsource.Publication{publication}
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
	if c == nil || c.authority == nil || c.authority.component != c || !c.authority.semanticReceipt.Valid() || c.authority.semanticReceipt.OwnerID() != c.authority.content {
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

func (c *Component) buildSemanticSourceReceipt() (SemanticSourceReceipt, error) {
	if c == nil || c.authority == nil || c.authority.component != c || !c.authority.content.Available() {
		return SemanticSourceReceipt{}, errors.New("link/boundary: unavailable semantic-source owner")
	}
	count, ok := c.Cardinality()
	if !ok || count < 0 {
		return SemanticSourceReceipt{}, errors.New("link/boundary: malformed semantic-source cardinality")
	}
	owner := c.authority.content
	views := SemanticSourceViews{owner: owner, boundary: SemanticSourceView{owner: owner, count: count}}
	receipt := SemanticSourceReceipt{owner: owner, views: views}
	if !receipt.Valid() {
		return SemanticSourceReceipt{}, errors.New("link/boundary: malformed semantic-source receipt")
	}
	return receipt, nil
}
