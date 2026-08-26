package authority

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

const catalogDigestDomain = "wippy.analysis/relation/schema/authority/catalog/v1"

const declarationDigestDomain = "wippy.analysis/relation/schema/authority/declaration/v1"

func digestDeclaration(declaration Declaration) identity.ContentID {
	parts := make([][]byte, 0, 10+len(declaration.relations)+len(declaration.columns)+len(declaration.keys)+len(declaration.scopes)+len(declaration.denominators))
	parts = append(parts, []byte("relations"), uintBytes(uint64(len(declaration.relations))))
	for _, relation := range declaration.relations {
		parts = append(parts, declarationRelationBytes(relation))
	}
	parts = append(parts, []byte("columns"), uintBytes(uint64(len(declaration.columns))))
	for _, column := range declaration.columns {
		parts = append(parts, declarationColumnBytes(column))
	}
	parts = append(parts, []byte("keys"), uintBytes(uint64(len(declaration.keys))))
	for _, key := range declaration.keys {
		parts = append(parts, declarationKeyBytes(key))
	}
	parts = append(parts, []byte("scopes"), uintBytes(uint64(len(declaration.scopes))))
	for _, scope := range declaration.scopes {
		parts = append(parts, declarationScopeBytes(scope))
	}
	parts = append(parts, []byte("denominators"), uintBytes(uint64(len(declaration.denominators))))
	for _, denominator := range declaration.denominators {
		parts = append(parts, declarationDenominatorBytes(denominator))
	}
	return derive(declarationDigestDomain, parts...)
}

func declarationRelationBytes(relation RelationSpec) []byte {
	parts := make([][]byte, 0, 4+2*len(relation.Addressing))
	parts = append(parts, []byte(relation.Name), identityBytes(relation.Token), []byte(relation.Scope), []byte{boolByte(relation.Publication.Available())})
	if relation.Publication.Available() {
		parts = append(parts, []byte(relation.Publication))
	}
	for _, address := range relation.Addressing {
		parts = append(parts, []byte{byte(address.Coordinate)}, []byte(address.Column))
	}
	return identityBytes(derive("wippy.analysis/relation/schema/authority/declaration/relation/v1", parts...))
}

func declarationColumnBytes(column ColumnSpec) []byte {
	return identityBytes(derive("wippy.analysis/relation/schema/authority/declaration/column/v1", []byte(column.Name), identityBytes(column.Token), []byte(column.Relation), identityBytes(column.Type.Owner().Content()), identityBytes(column.Type.Content())))
}

func declarationKeyBytes(key KeySpec) []byte {
	parts := make([][]byte, 0, 3+len(key.Columns))
	parts = append(parts, []byte(key.Name), identityBytes(key.Token), []byte(key.Relation))
	for _, column := range key.Columns {
		parts = append(parts, []byte(column))
	}
	return identityBytes(derive("wippy.analysis/relation/schema/authority/declaration/key/v1", parts...))
}

func declarationScopeBytes(scope ScopeSpec) []byte {
	parts := make([][]byte, 0, 3+len(scope.Dimensions))
	parts = append(parts, []byte(scope.Name), identityBytes(scope.Token), identityBytes(scope.Region.Identity()))
	for _, dimension := range scope.Dimensions {
		parts = append(parts, []byte(dimension))
	}
	return identityBytes(derive("wippy.analysis/relation/schema/authority/declaration/scope/v2", parts...))
}

func declarationDenominatorBytes(denominator DenominatorSpec) []byte {
	return identityBytes(derive("wippy.analysis/relation/schema/authority/declaration/denominator/v1", []byte(denominator.Name), []byte(denominator.Relation), []byte(denominator.Key)))
}

func digestCatalog(catalog Catalog) identity.ContentID {
	parts := make([][]byte, 0, 3+len(catalog.relations)+len(catalog.columns)+len(catalog.keys)+len(catalog.scopes)+len(catalog.denominators))
	parts = append(parts, uintBytes(uint64(catalog.owner.Entry.Surface)), []byte(catalog.owner.Entry.Key), identityBytes(catalog.owner.Token))
	for _, relation := range catalog.relations {
		parts = append(parts, relationBytes(relation))
	}
	for _, column := range catalog.columns {
		parts = append(parts, columnBytes(column))
	}
	for _, key := range catalog.keys {
		parts = append(parts, keyBytes(key))
	}
	for _, scope := range catalog.scopes {
		parts = append(parts, scopeBytes(scope))
	}
	for _, denominator := range catalog.denominators {
		parts = append(parts, denominatorBytes(denominator))
	}
	return derive(catalogDigestDomain, parts...)
}

func relationBytes(relation Relation) []byte {
	parts := make([][]byte, 0, 5+len(relation.columns)+len(relation.keys)+2*len(relation.addressing))
	parts = append(parts, []byte(relation.name), identityBytes(relation.id.Owner().Content()), identityBytes(relation.id.Content()), []byte(relation.scope), []byte{boolByte(relation.publication.Available())})
	if relation.publication.Available() {
		parts = append(parts, []byte(relation.publication))
	}
	for _, column := range relation.columns {
		parts = append(parts, []byte(column))
	}
	for _, key := range relation.keys {
		parts = append(parts, []byte(key))
	}
	for _, address := range relation.addressing {
		parts = append(parts, []byte{byte(address.Coordinate)}, []byte(address.Column))
	}
	return identityBytes(derive("wippy.analysis/relation/schema/authority/relation/v1", parts...))
}

func columnBytes(column Column) []byte {
	return identityBytes(derive("wippy.analysis/relation/schema/authority/column/v1", []byte(column.name), identityBytes(column.id.Owner().Content()), identityBytes(column.id.Content()), []byte(column.relation), identityBytes(column.typeID.Owner().Content()), identityBytes(column.typeID.Content())))
}

func keyBytes(key Key) []byte {
	parts := make([][]byte, 0, 4+len(key.columns))
	parts = append(parts, []byte(key.name), identityBytes(key.id.Owner().Content()), identityBytes(key.id.Content()), []byte(key.relation))
	for _, column := range key.columns {
		parts = append(parts, []byte(column))
	}
	return identityBytes(derive("wippy.analysis/relation/schema/authority/key/v1", parts...))
}

func scopeBytes(scope Scope) []byte {
	parts := make([][]byte, 0, 4+len(scope.dimensions))
	parts = append(parts, []byte(scope.name), identityBytes(scope.id.Owner().Content()), identityBytes(scope.id.Content()), identityBytes(scope.region.Identity()))
	for _, dimension := range scope.dimensions {
		parts = append(parts, []byte(dimension))
	}
	return identityBytes(derive("wippy.analysis/relation/schema/authority/scope/v2", parts...))
}

func denominatorBytes(denominator Denominator) []byte {
	return identityBytes(derive("wippy.analysis/relation/schema/authority/denominator/v1", []byte(denominator.name), []byte(denominator.relation), []byte(denominator.key), identityBytes(denominator.reference.Relation().Owner().Content()), identityBytes(denominator.reference.Relation().Content()), identityBytes(denominator.reference.Key().Content())))
}

func derive(domain string, parts ...[]byte) identity.ContentID {
	value, ok := identity.DeriveContentID(domain, parts...)
	if !ok {
		return identity.ContentID{}
	}
	return value
}

func identityBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func uintBytes(value uint64) []byte {
	var result [8]byte
	binary.BigEndian.PutUint64(result[:], value)
	return result[:]
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
