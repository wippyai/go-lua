package semanticsource

import (
	"errors"
	"sort"
)

var (
	// ErrInvalidDefinition rejects a zero, forged, or malformed definition.
	ErrInvalidDefinition = errors.New("semantic source: invalid relation definition")
	// ErrInvalidSchema rejects a schema that is malformed or was not issued by
	// same-package generated definitions.
	ErrInvalidSchema = errors.New("semantic source: invalid schema")
	// ErrDuplicateDefinition rejects two declarations for one Token.
	ErrDuplicateDefinition = errors.New("semantic source: duplicate relation definition")
	// ErrMissingFacetParent rejects a facet whose primary relation is absent.
	ErrMissingFacetParent = errors.New("semantic source: missing primary relation for facet")
	// ErrRevisionConflict rejects different revisions declared for one Origin.
	// An Origin and all of its facets share exactly one generated revision.
	ErrRevisionConflict = errors.New("semantic source: multiple revisions for origin")
)

const (
	relationDefinitionSeal uint64 = 0x9BDF_3187_8C17_77A1
	schemaSeal             uint64 = 0x53D4_E8A2_7160_BC19
)

// RelationDef is a generated declaration of one source relation. Its fields
// are private so callers may carry and compare definitions but cannot mint
// definitions outside this package.
type RelationDef struct {
	token Token
	seal  uint64
}

// Token reports the definition's canonical relation identity.
func (d RelationDef) Token() Token { return d.token }

func (d RelationDef) valid() bool {
	return d.seal == relationDefinitionSeal && d.token.valid()
}

// Schema is the immutable generated denominator for one catalog publication.
// The private owned slice prevents callers from changing the expected measure
// after a Publisher is initialized.
type Schema struct {
	definitions []RelationDef
	seal        uint64
}

// Definition resolves one exact generated catalog definition by its numeric
// origin and facet. It never creates a relation: an unknown or zero pair is
// rejected.
func Definition(origin Origin, facet Facet) (RelationDef, bool) {
	return catalogDefinition(origin, facet)
}

// Count includes primary and facet definitions, including definitions whose
// admitted cardinality claim is later zero.
func (s Schema) Count() int { return len(s.definitions) }

// DefinitionAt returns one canonical token-sorted definition.
func (s Schema) DefinitionAt(index int) (RelationDef, bool) {
	if !s.valid() || index < 0 || index >= len(s.definitions) {
		return RelationDef{}, false
	}
	return s.definitions[index], true
}

// Definitions returns a detached canonical token-sorted copy.
func (s Schema) Definitions() []RelationDef {
	if !s.valid() {
		return nil
	}
	return append([]RelationDef(nil), s.definitions...)
}

func (s Schema) valid() bool {
	return s.validationError() == nil
}

func (s Schema) validationError() error {
	if s.seal != schemaSeal || len(s.definitions) == 0 {
		return ErrInvalidSchema
	}
	var currentOrigin Origin
	var currentRevision Revision
	for index, definition := range s.definitions {
		if !definition.valid() {
			return ErrInvalidDefinition
		}
		if index != 0 {
			order := compareToken(s.definitions[index-1].token, definition.token)
			if order == 0 {
				return ErrDuplicateDefinition
			}
			if order > 0 {
				return ErrInvalidSchema
			}
		}
		if definition.token.origin != currentOrigin {
			currentOrigin = definition.token.origin
			currentRevision = definition.token.revision
		} else if definition.token.revision != currentRevision {
			return ErrRevisionConflict
		}
		if !definition.token.Primary() && !s.has(definition.token.parent()) {
			return ErrMissingFacetParent
		}
	}
	return nil
}

func (s Schema) has(token Token) bool {
	index := sort.Search(len(s.definitions), func(index int) bool {
		return compareToken(s.definitions[index].token, token) >= 0
	})
	return index < len(s.definitions) && s.definitions[index].token == token
}

// issuedDefinition and issuedSchema are deliberately package-private. The
// generated catalog is the only producer of usable definition/schema values.
func issuedDefinition(token Token) RelationDef {
	return RelationDef{token: token, seal: relationDefinitionSeal}
}

func issuedSchema(definitions ...RelationDef) Schema {
	owned := append([]RelationDef(nil), definitions...)
	sort.Slice(owned, func(left, right int) bool {
		return compareToken(owned[left].token, owned[right].token) < 0
	})
	schema := Schema{definitions: owned, seal: schemaSeal}
	return schema
}
