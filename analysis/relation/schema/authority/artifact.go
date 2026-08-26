package authority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Relation is one sealed immutable relation row. Its local labels remain
// available for a later Registry projection, while ID is the exact owner-
// issued relation identity.
type Relation struct {
	name        schema.Key
	id          model.RelationID
	scope       schema.Key
	columns     []schema.Key
	keys        []schema.Key
	addressing  []Address
	publication schema.Key
}

func (relation Relation) Available() bool {
	return relation.name.Available() && relation.id.Available() && relation.scope.Available()
}

// Name returns the authored local relation label.
func (relation Relation) Name() schema.Key { return relation.name }

// ID returns the owner-issued relation identity.
func (relation Relation) ID() model.RelationID { return relation.id }

// Token returns the relation's owner-issued content token.
func (relation Relation) Token() identity.ContentID { return relation.id.Content() }

// Scope returns the local scope label attached to the relation.
func (relation Relation) Scope() schema.Key { return relation.scope }

// Columns returns the relation's authored ordered column labels.
func (relation Relation) Columns() []schema.Key { return cloneLabels(relation.columns) }

// Keys returns the relation's authored ordered key labels.
func (relation Relation) Keys() []schema.Key { return cloneLabels(relation.keys) }

// Addressing returns the relation's owner-declared coordinate rows.
func (relation Relation) Addressing() []Address { return cloneAddresses(relation.addressing) }

// PublicationKey returns the optional local key label under which this
// relation publishes rows.
func (relation Relation) PublicationKey() (schema.Key, bool) {
	return relation.publication, relation.publication.Available()
}

// Column is one sealed immutable relation-column row.
type Column struct {
	name     schema.Key
	id       model.ColumnID
	relation schema.Key
	typeID   model.TypeID
}

func (column Column) Available() bool {
	return column.name.Available() && column.id.Available() && column.relation.Available() && column.typeID.Available()
}

// Name returns the authored local column label.
func (column Column) Name() schema.Key { return column.name }

// ID returns the owner-issued relation-column identity.
func (column Column) ID() model.ColumnID { return column.id }

// Token returns the column's owner-issued content token.
func (column Column) Token() identity.ContentID { return column.id.Content() }

// Relation returns the local relation label that owns this column.
func (column Column) Relation() schema.Key { return column.relation }

// Type returns the complete owner-issued semantic type identity.
func (column Column) Type() model.TypeID { return column.typeID }

// Key is one sealed immutable ordered key-vector row.
type Key struct {
	name     schema.Key
	id       model.KeyID
	relation schema.Key
	columns  []schema.Key
}

func (key Key) Available() bool {
	return key.name.Available() && key.id.Available() && key.relation.Available()
}

// Name returns the authored local key label.
func (key Key) Name() schema.Key { return key.name }

// ID returns the owner-issued key identity.
func (key Key) ID() model.KeyID { return key.id }

// Token returns the key's owner-issued content token.
func (key Key) Token() identity.ContentID { return key.id.Content() }

// Relation returns the local relation label that owns this key.
func (key Key) Relation() schema.Key { return key.relation }

// Columns returns the key's ordered local column labels.
func (key Key) Columns() []schema.Key { return cloneLabels(key.columns) }

// Scope is one sealed immutable decision-scope row.
type Scope struct {
	name       schema.Key
	id         model.ScopeID
	dimensions []schema.Key
	region     region.Region
}

func (scope Scope) Available() bool {
	return scope.name.Available() && scope.id.Available() && scope.region.Available() && !scope.region.IsFalse()
}

// Name returns the authored local scope label.
func (scope Scope) Name() schema.Key { return scope.name }

// ID returns the owner-issued scope identity.
func (scope Scope) ID() model.ScopeID { return scope.id }

// Token returns the scope's owner-issued content token.
func (scope Scope) Token() identity.ContentID { return scope.id.Content() }

// Dimensions returns the scope's authored ordered column labels.
func (scope Scope) Dimensions() []schema.Key { return cloneLabels(scope.dimensions) }

// Region returns the owner-declared Boolean scope formula.  The returned
// Region is immutable and carries its canonical identity and graph by value.
func (scope Scope) Region() region.Region { return scope.region }

// Denominator is one sealed immutable relation/key universe row.
type Denominator struct {
	name      schema.Key
	relation  schema.Key
	key       schema.Key
	reference model.DenominatorRef
}

func (denominator Denominator) Available() bool {
	return denominator.name.Available() && denominator.relation.Available() && denominator.key.Available() && denominator.reference.Available()
}

// Name returns the authored local denominator label.
func (denominator Denominator) Name() schema.Key { return denominator.name }

// Relation returns the local relation label closed by this denominator.
func (denominator Denominator) Relation() schema.Key { return denominator.relation }

// Key returns the local key label closed by this denominator.
func (denominator Denominator) Key() schema.Key { return denominator.key }

// Reference returns the canonical owner-issued relation/key universe.
func (denominator Denominator) Reference() model.DenominatorRef { return denominator.reference }

func cloneRelation(relation Relation) Relation {
	relation.columns = cloneLabels(relation.columns)
	relation.keys = cloneLabels(relation.keys)
	relation.addressing = cloneAddresses(relation.addressing)
	return relation
}

func cloneKey(key Key) Key {
	key.columns = cloneLabels(key.columns)
	return key
}

func cloneScope(scope Scope) Scope {
	scope.dimensions = cloneLabels(scope.dimensions)
	return scope
}
