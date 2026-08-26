package authority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Catalog is one immutable owner-local attachment. It retains no lookup map;
// rows are exposed in the exact declaration order supplied to NewCatalog.
type Catalog struct {
	owner        Owner
	ownerID      model.OwnerID
	relations    []Relation
	columns      []Column
	keys         []Key
	scopes       []Scope
	denominators []Denominator
	digest       identity.ContentID
	sealed       bool
}

// sealCatalog attaches the owner and issues identities from a Declaration
// whose raw graph has already been validated and privately copied. It does no
// second graph validation.
func sealCatalog(owner Owner, declaration Declaration) (Catalog, bool) {
	ownerID, ok := validOwner(owner)
	if !ok {
		return Catalog{}, false
	}
	if declarationClaimsToken(declaration, owner.Token) {
		return Catalog{}, false
	}

	// These maps are construction indexes only. They never cross the
	// immutable attachment boundary and are discarded after issuance.
	relations := declaration.relations
	columns := declaration.columns
	keys := declaration.keys
	scopes := declaration.scopes
	denominators := declaration.denominators
	relationByName := make(map[schema.Key]int, len(relations))
	keyByName := make(map[schema.Key]int, len(keys))
	for index, relation := range relations {
		relationByName[relation.Name] = index
	}
	for index, key := range keys {
		keyByName[key.Name] = index
	}

	sealedScopes := make([]Scope, len(scopes))
	for index, spec := range scopes {
		id, issued := model.IssueScopeID(ownerID, spec.Token)
		if !issued {
			return Catalog{}, false
		}
		sealedScopes[index] = Scope{name: spec.Name, id: id, dimensions: cloneLabels(spec.Dimensions), region: spec.Region}
	}

	sealedRelations := make([]Relation, len(relations))
	for index, spec := range relations {
		id, issued := model.IssueRelationID(ownerID, spec.Token)
		if !issued {
			return Catalog{}, false
		}
		sealedRelations[index] = Relation{
			name:        spec.Name,
			id:          id,
			scope:       spec.Scope,
			addressing:  cloneAddresses(spec.Addressing),
			publication: spec.Publication,
		}
	}

	sealedColumns := make([]Column, len(columns))
	for index, spec := range columns {
		relationIndex := relationByName[spec.Relation]
		relationID := sealedRelations[relationIndex].id
		id, issued := model.IssueColumnID(relationID, spec.Token)
		if !issued {
			return Catalog{}, false
		}
		// Type is already a complete, owner-fenced model identity. It is
		// carried exactly; its content token is not part of this catalog's
		// local token-uniqueness namespace.
		sealedColumns[index] = Column{name: spec.Name, id: id, relation: spec.Relation, typeID: spec.Type}
	}

	// Relation membership is stated once by the ordered column declarations.
	// No duplicate membership field is accepted on RelationSpec.
	for index := range sealedColumns {
		column := &sealedColumns[index]
		relationIndex := relationByName[column.relation]
		sealedRelations[relationIndex].columns = append(sealedRelations[relationIndex].columns, column.name)
	}

	sealedKeys := make([]Key, len(keys))
	for index, spec := range keys {
		relationIndex := relationByName[spec.Relation]
		relationID := sealedRelations[relationIndex].id
		id, issued := model.IssueKeyID(relationID, spec.Token)
		if !issued {
			return Catalog{}, false
		}
		sealedKeys[index] = Key{name: spec.Name, id: id, relation: spec.Relation, columns: cloneLabels(spec.Columns)}
	}

	// Relation key membership is stated once by the ordered key declarations.
	for index := range sealedKeys {
		key := &sealedKeys[index]
		relationIndex := relationByName[key.relation]
		sealedRelations[relationIndex].keys = append(sealedRelations[relationIndex].keys, key.name)
	}

	sealedDenominators := make([]Denominator, len(denominators))
	for index, spec := range denominators {
		relationIndex := relationByName[spec.Relation]
		keyIndex := keyByName[spec.Key]
		reference, referenceOK := model.NewDenominatorRef(sealedRelations[relationIndex].id, sealedKeys[keyIndex].id)
		if !referenceOK {
			return Catalog{}, false
		}
		sealedDenominators[index] = Denominator{name: spec.Name, relation: spec.Relation, key: spec.Key, reference: reference}
	}

	result := Catalog{
		owner:        owner,
		ownerID:      ownerID,
		relations:    sealedRelations,
		columns:      sealedColumns,
		keys:         sealedKeys,
		scopes:       sealedScopes,
		denominators: sealedDenominators,
		sealed:       true,
	}
	result.digest = digestCatalog(result)
	if !result.digest.Available() {
		return Catalog{}, false
	}
	return result, true
}

// Available reports whether this is a complete sealed attachment, including
// the valid empty attachment.
func (catalog Catalog) Available() bool {
	return catalog.sealed && catalog.owner.Available() && catalog.ownerID.Available() && catalog.digest.Available()
}

// Owner returns the exact owner fence retained by this attachment.
func (catalog Catalog) Owner() Owner {
	if !catalog.Available() {
		return Owner{}
	}
	return catalog.owner
}

// OwnerID returns the model owner identity used to issue local rows.
func (catalog Catalog) OwnerID() model.OwnerID {
	if !catalog.Available() {
		return model.OwnerID{}
	}
	return catalog.ownerID
}

// Digest returns the deterministic content identity of the complete
// owner-local attachment. Authored order is part of the declaration content.
func (catalog Catalog) Digest() identity.ContentID {
	if !catalog.Available() {
		return identity.ContentID{}
	}
	return catalog.digest
}

// Relations returns a deep defensive copy in authored order.
func (catalog Catalog) Relations() []Relation {
	if !catalog.Available() || catalog.relations == nil {
		return nil
	}
	result := make([]Relation, len(catalog.relations))
	for index, relation := range catalog.relations {
		result[index] = cloneRelation(relation)
	}
	return result
}

// Columns returns a defensive copy in authored order.
func (catalog Catalog) Columns() []Column {
	if !catalog.Available() || catalog.columns == nil {
		return nil
	}
	return append([]Column(nil), catalog.columns...)
}

// Keys returns a deep defensive copy in authored order.
func (catalog Catalog) Keys() []Key {
	if !catalog.Available() || catalog.keys == nil {
		return nil
	}
	result := make([]Key, len(catalog.keys))
	for index, key := range catalog.keys {
		result[index] = cloneKey(key)
	}
	return result
}

// Scopes returns a deep defensive copy in authored order.
func (catalog Catalog) Scopes() []Scope {
	if !catalog.Available() || catalog.scopes == nil {
		return nil
	}
	result := make([]Scope, len(catalog.scopes))
	for index, scope := range catalog.scopes {
		result[index] = cloneScope(scope)
	}
	return result
}

// Denominators returns a defensive copy in authored order.
func (catalog Catalog) Denominators() []Denominator {
	if !catalog.Available() || catalog.denominators == nil {
		return nil
	}
	return append([]Denominator(nil), catalog.denominators...)
}

func (catalog Catalog) RelationCount() int {
	if !catalog.Available() {
		return 0
	}
	return len(catalog.relations)
}

func (catalog Catalog) ColumnCount() int {
	if !catalog.Available() {
		return 0
	}
	return len(catalog.columns)
}

func (catalog Catalog) KeyCount() int {
	if !catalog.Available() {
		return 0
	}
	return len(catalog.keys)
}

func (catalog Catalog) ScopeCount() int {
	if !catalog.Available() {
		return 0
	}
	return len(catalog.scopes)
}

func (catalog Catalog) DenominatorCount() int {
	if !catalog.Available() {
		return 0
	}
	return len(catalog.denominators)
}

// RelationAt returns one relation at its authored position.
func (catalog Catalog) RelationAt(index int) (Relation, bool) {
	if !catalog.Available() || index < 0 || index >= len(catalog.relations) {
		return Relation{}, false
	}
	return cloneRelation(catalog.relations[index]), true
}

// ColumnAt returns one column at its authored position.
func (catalog Catalog) ColumnAt(index int) (Column, bool) {
	if !catalog.Available() || index < 0 || index >= len(catalog.columns) {
		return Column{}, false
	}
	return catalog.columns[index], true
}

// KeyAt returns one key at its authored position.
func (catalog Catalog) KeyAt(index int) (Key, bool) {
	if !catalog.Available() || index < 0 || index >= len(catalog.keys) {
		return Key{}, false
	}
	return cloneKey(catalog.keys[index]), true
}

// ScopeAt returns one scope at its authored position.
func (catalog Catalog) ScopeAt(index int) (Scope, bool) {
	if !catalog.Available() || index < 0 || index >= len(catalog.scopes) {
		return Scope{}, false
	}
	return cloneScope(catalog.scopes[index]), true
}

// DenominatorAt returns one denominator at its authored position.
func (catalog Catalog) DenominatorAt(index int) (Denominator, bool) {
	if !catalog.Available() || index < 0 || index >= len(catalog.denominators) {
		return Denominator{}, false
	}
	return catalog.denominators[index], true
}

// RelationByName resolves one owner-local relation label to the exact
// owner-issued relation. Labels are construction vocabulary only: callers
// receive the sealed nominal identity and cannot issue a relation of their
// own.
func (catalog Catalog) RelationByName(name schema.Key) (Relation, bool) {
	if !catalog.Available() || !name.Available() {
		return Relation{}, false
	}
	for _, relation := range catalog.relations {
		if relation.name == name {
			return cloneRelation(relation), true
		}
	}
	return Relation{}, false
}

// ColumnByName resolves one owner-local column label to its exact sealed row.
// It is an owner attachment lookup only: it neither resolves cross-owner
// vocabulary nor issues a replacement column identity.
func (catalog Catalog) ColumnByName(name schema.Key) (Column, bool) {
	if !catalog.Available() || !name.Available() {
		return Column{}, false
	}
	for _, column := range catalog.columns {
		if column.name == name {
			return column, true
		}
	}
	return Column{}, false
}

// KeyByName resolves one owner-local key label to its exact sealed row.
func (catalog Catalog) KeyByName(name schema.Key) (Key, bool) {
	if !catalog.Available() || !name.Available() {
		return Key{}, false
	}
	for _, key := range catalog.keys {
		if key.name == name {
			return cloneKey(key), true
		}
	}
	return Key{}, false
}

// ScopeByName resolves one owner-local scope label to its exact sealed row.
func (catalog Catalog) ScopeByName(name schema.Key) (Scope, bool) {
	if !catalog.Available() || !name.Available() {
		return Scope{}, false
	}
	for _, scope := range catalog.scopes {
		if scope.name == name {
			return cloneScope(scope), true
		}
	}
	return Scope{}, false
}

// DenominatorByName resolves one owner-local denominator label to its exact
// sealed relation/key universe row.
func (catalog Catalog) DenominatorByName(name schema.Key) (Denominator, bool) {
	if !catalog.Available() || !name.Available() {
		return Denominator{}, false
	}
	for _, denominator := range catalog.denominators {
		if denominator.name == name {
			return denominator, true
		}
	}
	return Denominator{}, false
}

// IssueRow adopts one content token through the named relation's exact owner
// fence. The catalog does not derive the token: the owner of the base-row
// population supplies it once from that population's canonical key values.
// This keeps row identity issuance at the relation owner while leaving key
// encoding and enumeration policy outside the schema vocabulary.
func (catalog Catalog) IssueRow(relationName schema.Key, token identity.ContentID) (model.RowID, bool) {
	relation, ok := catalog.RelationByName(relationName)
	if !ok || !token.Available() {
		return model.RowID{}, false
	}
	return model.IssueRowID(relation.ID(), token)
}
