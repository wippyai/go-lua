package boundary

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SourceRow is Boundary's factorized cardinality column for the virtual
// Application×Operation predicate. Boundary does not materialize that product.
type SourceRow struct {
	owner identity.ContentID
	count int
}

func (row SourceRow) valid() bool                 { return row.owner.Available() && row.count >= 0 }
func (row SourceRow) OwnerID() identity.ContentID { return row.owner }
func (row SourceRow) Count() int {
	if !row.valid() {
		return 0
	}
	return row.count
}
func (row SourceRow) DigestAt(int) (identity.ContentID, bool) { return identity.ContentID{}, false }
func (row SourceRow) Digests() []identity.ContentID           { return nil }

type SourceViews struct {
	owner    identity.ContentID
	boundary SourceRow
}

func (views SourceViews) valid() bool {
	return views.owner.Available() && views.boundary.valid() && views.boundary.owner == views.owner
}
func (views SourceViews) Valid() bool                 { return views.owner.Available() && views.valid() }
func (views SourceViews) OwnerID() identity.ContentID { return views.owner }
func (views SourceViews) Boundary() SourceRow         { return views.boundary }

// Publications projects Boundary's factorized source column through the
// sealed schema; the schema owns membership and canonical relation order.
func (views SourceViews) Publications(schema semanticsource.ProgramSchema) []semanticsource.Publication {
	if schema == nil || !views.Valid() {
		return nil
	}
	definition, ok := schema.Definition(semanticsource.OriginLinkBoundary, 0)
	if !ok {
		return nil
	}
	publication, err := semanticsource.SealPublication(definition, views.boundary.Count())
	if err != nil {
		return nil
	}
	return []semanticsource.Publication{publication}
}

func (c *Component) SourceViews() (SourceViews, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || !c.authority.sourceViews.Valid() || c.authority.sourceViews.OwnerID() != c.authority.content {
		return SourceViews{}, false
	}
	return c.authority.sourceViews, true
}

func (c *Component) buildSourceViews() (SourceViews, error) {
	if c == nil || c.authority == nil || c.authority.component != c || !c.authority.content.Available() {
		return SourceViews{}, errors.New("link/boundary: unavailable semantic-source owner")
	}
	count, ok := c.Cardinality()
	if !ok || count < 0 {
		return SourceViews{}, errors.New("link/boundary: malformed semantic-source cardinality")
	}
	owner := c.authority.content
	views := SourceViews{owner: owner, boundary: SourceRow{owner: owner, count: count}}
	if !views.Valid() {
		return SourceViews{}, errors.New("link/boundary: malformed semantic-source rows")
	}
	return views, nil
}
