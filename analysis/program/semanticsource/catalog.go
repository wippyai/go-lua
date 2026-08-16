package semanticsource

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
)

var (
	// ErrInvalidDefinition rejects a zero, forged, or malformed definition.
	ErrInvalidDefinition = errors.New("semantic source: invalid relation definition")
	// ErrInvalidSchema rejects an absent or malformed sealed Program schema.
	ErrInvalidSchema = errors.New("semantic source: invalid schema")
)

const relationDefinitionSeal uint64 = 0x9BDF_3187_8C17_77A1

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

// Declare mints the token for one generated origin/facet pair. The generated
// origin revision fence is the only revision source; Declare does not state
// that the pair belongs to the complete relation denominator. The sealed
// ProgramSchema owns that membership decision.
func Declare(origin Origin, facet Facet) (RelationDef, bool) {
	revision, ok := revisionForOrigin(origin)
	if !ok || origin == 0 || revision == 0 {
		return RelationDef{}, false
	}
	return RelationDef{token: issuedToken(origin, facet, revision), seal: relationDefinitionSeal}, true
}

// schemaDefinitions snapshots the complete declaration table exposed by one
// sealed ProgramSchema. The table is the only denominator: callers never
// reconstruct it from the generated alphabet or maintain a second catalog.
func schemaDefinitions(schema ProgramSchema) ([]RelationDef, map[Token]struct{}, error) {
	if schema == nil || schema.Count() <= 0 || !schema.SchemaDigest().Available() {
		return nil, nil, ErrInvalidSchema
	}
	definitions := make([]RelationDef, schema.Count())
	seen := make(map[Token]struct{}, schema.Count())
	for index := range definitions {
		definition, ok := schema.DefinitionAt(index)
		if !ok || !definition.valid() {
			return nil, nil, ErrInvalidSchema
		}
		token := definition.Token()
		expected, declared := Declare(token.Origin(), token.Facet())
		if !declared || expected != definition {
			return nil, nil, ErrInvalidSchema
		}
		resolved, found := schema.Definition(token.Origin(), token.Facet())
		if !found || resolved != definition {
			return nil, nil, ErrInvalidSchema
		}
		if _, duplicate := seen[token]; duplicate {
			return nil, nil, ErrInvalidSchema
		}
		seen[token] = struct{}{}
		definitions[index] = definition
	}
	for _, definition := range definitions {
		token := definition.Token()
		if token.Primary() {
			continue
		}
		parent, ok := Declare(token.Origin(), 0)
		if !ok {
			return nil, nil, ErrInvalidSchema
		}
		if _, exists := seen[parent.Token()]; !exists {
			return nil, nil, ErrInvalidSchema
		}
	}
	return definitions, seen, nil
}

// ProgramSchema is the narrow read surface exported by the one sealed
// relation table. Program and its children depend on this primitive contract,
// never on the schema package or a second generated catalog.
type ProgramSchema interface {
	Count() int
	DefinitionAt(index int) (RelationDef, bool)
	Definition(origin Origin, facet Facet) (RelationDef, bool)
	SchemaDigest() identity.ContentID
}
