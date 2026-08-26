package authority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Declaration is the immutable, owner-independent authored content that an
// owner commits before it has an EntryID. It is deliberately separate from a
// sealed Catalog: its digest can participate in EntryContent, while Seal
// later attaches the exact owner fence and issues local model identities.
type Declaration struct {
	relations    []RelationSpec
	columns      []ColumnSpec
	keys         []KeySpec
	scopes       []ScopeSpec
	denominators []DenominatorSpec
	digest       identity.ContentID
	sealed       bool
}

// NewDeclaration validates the five authored vectors and their complete local
// reference graph, copies every nested vector, and computes their
// deterministic owner-independent content digest. Seal therefore only needs
// to attach the owner fence and issue nominal model identities.
func NewDeclaration(relations []RelationSpec, columns []ColumnSpec, keys []KeySpec, scopes []ScopeSpec, denominators []DenominatorSpec) (Declaration, bool) {
	relationNames := make(map[schema.Key]struct{}, len(relations))
	columnNames := make(map[schema.Key]struct{}, len(columns))
	keyNames := make(map[schema.Key]struct{}, len(keys))
	scopeNames := make(map[schema.Key]struct{}, len(scopes))
	denominatorNames := make(map[schema.Key]struct{}, len(denominators))
	seenTokens := make(map[identity.ContentID]struct{}, len(relations)+len(columns)+len(keys)+len(scopes))
	if !validRelationSpecs(relations, relationNames, seenTokens) ||
		!validColumnSpecs(columns, columnNames, seenTokens) ||
		!validKeySpecs(keys, keyNames, seenTokens) ||
		!validScopeSpecs(scopes, scopeNames, seenTokens) ||
		!validDenominatorSpecs(denominators, denominatorNames) ||
		!declarationGraphValid(relations, columns, keys, scopes, denominators) {
		return Declaration{}, false
	}

	result := Declaration{
		relations:    cloneRelationSpecs(relations),
		columns:      cloneColumnSpecs(columns),
		keys:         cloneKeySpecs(keys),
		scopes:       cloneScopeSpecs(scopes),
		denominators: cloneDenominatorSpecs(denominators),
		sealed:       true,
	}
	result.digest = digestDeclaration(result)
	if !result.digest.Available() {
		return Declaration{}, false
	}
	return result, true
}

// Available reports whether this is a complete immutable authored
// declaration, including the valid empty declaration.
func (declaration Declaration) Available() bool {
	return declaration.sealed && declaration.digest.Available()
}

// Digest returns the deterministic authored content identity. It contains no
// owner EntryReference or owner token; those are attached only by Seal.
func (declaration Declaration) Digest() identity.ContentID {
	if !declaration.Available() {
		return identity.ContentID{}
	}
	return declaration.digest
}

// Relations returns a deep defensive copy of authored relation declarations.
func (declaration Declaration) Relations() []RelationSpec {
	if !declaration.Available() || declaration.relations == nil {
		return nil
	}
	return cloneRelationSpecs(declaration.relations)
}

// Columns returns a defensive copy of authored column declarations.
func (declaration Declaration) Columns() []ColumnSpec {
	if !declaration.Available() || declaration.columns == nil {
		return nil
	}
	return cloneColumnSpecs(declaration.columns)
}

// Keys returns a deep defensive copy of authored key declarations.
func (declaration Declaration) Keys() []KeySpec {
	if !declaration.Available() || declaration.keys == nil {
		return nil
	}
	return cloneKeySpecs(declaration.keys)
}

// Scopes returns a deep defensive copy of authored scope declarations.
func (declaration Declaration) Scopes() []ScopeSpec {
	if !declaration.Available() || declaration.scopes == nil {
		return nil
	}
	return cloneScopeSpecs(declaration.scopes)
}

// Denominators returns a defensive copy of authored denominator declarations.
func (declaration Declaration) Denominators() []DenominatorSpec {
	if !declaration.Available() || declaration.denominators == nil {
		return nil
	}
	return cloneDenominatorSpecs(declaration.denominators)
}

// Seal attaches the exact owner fence and issues the local model identities.
// No owner identity is derived from Declaration.Digest; the owner token is
// supplied explicitly by the sealed owner entry.
func (declaration Declaration) Seal(owner Owner) (Catalog, bool) {
	if !declaration.Available() {
		return Catalog{}, false
	}
	return sealCatalog(owner, declaration)
}

func validRelationSpecs(specs []RelationSpec, names map[schema.Key]struct{}, tokens map[identity.ContentID]struct{}) bool {
	for _, spec := range specs {
		if !spec.Available() || !claimDeclarationName(names, spec.Name) || !claimDeclarationToken(tokens, spec.Token) || duplicateAddresses(spec.Addressing) {
			return false
		}
	}
	return true
}

func validColumnSpecs(specs []ColumnSpec, names map[schema.Key]struct{}, tokens map[identity.ContentID]struct{}) bool {
	for _, spec := range specs {
		if !spec.Available() || !claimDeclarationName(names, spec.Name) || !claimDeclarationToken(tokens, spec.Token) {
			return false
		}
	}
	return true
}

func validKeySpecs(specs []KeySpec, names map[schema.Key]struct{}, tokens map[identity.ContentID]struct{}) bool {
	for _, spec := range specs {
		if !spec.Available() || !claimDeclarationName(names, spec.Name) || !claimDeclarationToken(tokens, spec.Token) || duplicateLabels(spec.Columns) {
			return false
		}
	}
	return true
}

func validScopeSpecs(specs []ScopeSpec, names map[schema.Key]struct{}, tokens map[identity.ContentID]struct{}) bool {
	for _, spec := range specs {
		if !spec.Available() || !claimDeclarationName(names, spec.Name) || !claimDeclarationToken(tokens, spec.Token) || duplicateLabels(spec.Dimensions) {
			return false
		}
	}
	return true
}

func validDenominatorSpecs(specs []DenominatorSpec, names map[schema.Key]struct{}) bool {
	for _, spec := range specs {
		if !spec.Available() || !claimDeclarationName(names, spec.Name) {
			return false
		}
	}
	return true
}

func claimDeclarationName(seen map[schema.Key]struct{}, name schema.Key) bool {
	if _, exists := seen[name]; exists {
		return false
	}
	seen[name] = struct{}{}
	return true
}

func claimDeclarationToken(seen map[identity.ContentID]struct{}, token identity.ContentID) bool {
	if _, exists := seen[token]; exists {
		return false
	}
	seen[token] = struct{}{}
	return true
}

func duplicateAddresses(addresses []Address) bool {
	coordinates := make(map[Coordinate]struct{}, len(addresses))
	columns := make(map[schema.Key]struct{}, len(addresses))
	for _, address := range addresses {
		if _, exists := coordinates[address.Coordinate]; exists {
			return true
		}
		coordinates[address.Coordinate] = struct{}{}
		if _, exists := columns[address.Column]; exists {
			return true
		}
		columns[address.Column] = struct{}{}
	}
	return false
}

func cloneRelationSpecs(specs []RelationSpec) []RelationSpec {
	if specs == nil {
		return nil
	}
	result := make([]RelationSpec, len(specs))
	for index, spec := range specs {
		result[index] = spec
		result[index].Addressing = cloneAddresses(spec.Addressing)
	}
	return result
}

func cloneColumnSpecs(specs []ColumnSpec) []ColumnSpec {
	if specs == nil {
		return nil
	}
	return append([]ColumnSpec(nil), specs...)
}

func cloneKeySpecs(specs []KeySpec) []KeySpec {
	if specs == nil {
		return nil
	}
	result := make([]KeySpec, len(specs))
	for index, spec := range specs {
		result[index] = spec
		result[index].Columns = cloneLabels(spec.Columns)
	}
	return result
}

func cloneScopeSpecs(specs []ScopeSpec) []ScopeSpec {
	if specs == nil {
		return nil
	}
	result := make([]ScopeSpec, len(specs))
	for index, spec := range specs {
		result[index] = spec
		result[index].Dimensions = cloneLabels(spec.Dimensions)
	}
	return result
}

func cloneDenominatorSpecs(specs []DenominatorSpec) []DenominatorSpec {
	if specs == nil {
		return nil
	}
	return append([]DenominatorSpec(nil), specs...)
}
