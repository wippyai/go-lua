package authority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
)

func validOwner(owner Owner) (model.OwnerID, bool) {
	if !owner.Available() {
		return model.OwnerID{}, false
	}
	value, ok := model.IssueOwnerID(owner.Token)
	return value, ok
}

func declarationClaimsToken(declaration Declaration, token identity.ContentID) bool {
	for _, relation := range declaration.relations {
		if relation.Token == token {
			return true
		}
	}
	for _, column := range declaration.columns {
		if column.Token == token {
			return true
		}
	}
	for _, key := range declaration.keys {
		if key.Token == token {
			return true
		}
	}
	for _, scope := range declaration.scopes {
		if scope.Token == token {
			return true
		}
	}
	return false
}

// declarationGraphValid is the one owner-independent proof of all local
// cross-row references. Seal only attaches an owner and issues identities from
// this already-proven graph.
func declarationGraphValid(relations []RelationSpec, columns []ColumnSpec, keys []KeySpec, scopes []ScopeSpec, denominators []DenominatorSpec) bool {
	relationsByName := make(map[schema.Key]RelationSpec, len(relations))
	columnsByName := make(map[schema.Key]ColumnSpec, len(columns))
	keysByName := make(map[schema.Key]KeySpec, len(keys))
	scopesByName := make(map[schema.Key]ScopeSpec, len(scopes))
	for _, relation := range relations {
		relationsByName[relation.Name] = relation
	}
	for _, column := range columns {
		if _, relationKnown := relationsByName[column.Relation]; !relationKnown {
			return false
		}
		columnsByName[column.Name] = column
	}
	for _, key := range keys {
		if _, relationKnown := relationsByName[key.Relation]; !relationKnown {
			return false
		}
		for _, columnName := range key.Columns {
			column, columnKnown := columnsByName[columnName]
			if !columnKnown || column.Relation != key.Relation {
				return false
			}
		}
		keysByName[key.Name] = key
	}
	for _, scope := range scopes {
		for _, dimension := range scope.Dimensions {
			if _, known := columnsByName[dimension]; !known {
				return false
			}
		}
		scopesByName[scope.Name] = scope
	}
	for _, relation := range relations {
		if _, scopeKnown := scopesByName[relation.Scope]; !scopeKnown {
			return false
		}
		for _, address := range relation.Addressing {
			column, columnKnown := columnsByName[address.Column]
			if !columnKnown || column.Relation != relation.Name {
				return false
			}
		}
		if relation.Publication.Available() {
			key, keyKnown := keysByName[relation.Publication]
			if !keyKnown || key.Relation != relation.Name {
				return false
			}
		}
	}
	for _, denominator := range denominators {
		if _, relationKnown := relationsByName[denominator.Relation]; !relationKnown {
			return false
		}
		key, keyKnown := keysByName[denominator.Key]
		if !keyKnown || key.Relation != denominator.Relation {
			return false
		}
	}
	return true
}

func duplicateLabels(labels []schema.Key) bool {
	seen := make(map[schema.Key]struct{}, len(labels))
	for _, label := range labels {
		if _, exists := seen[label]; exists {
			return true
		}
		seen[label] = struct{}{}
	}
	return false
}
