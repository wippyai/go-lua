package static

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// compactReferences owns the authored TypeRef relation: its source spelling,
// its optional canonical path, and the binder disposition that separates the
// two. Resolution to a declaration target is authored, never inferred here.
func compactReferences(component *Component, counts [keyspace.FamilyCount]uint32, input ReferencesInput) error {
	store := &component.references
	for _, row := range input.TypeRef {
		if !validTypeRef(counts, row) {
			return errors.New("program/static: invalid type reference")
		}
		source, ok := appendKeys(&store.source, row.Source)
		if !ok {
			return errors.New("program/static: oversized type reference source")
		}
		canonical, ok := appendKeys(&store.canonical, row.Canonical)
		if !ok {
			return errors.New("program/static: oversized type reference canonical path")
		}
		store.rows = append(store.rows, typeRefRow{
			resolution: row.Resolution,
			target:     row.Target,
			root:       row.Root,
			source:     source,
			canonical:  canonical,
		})
	}
	return nil
}

func validTypeRef(counts [keyspace.FamilyCount]uint32, row TypeRef) bool {
	if !validKeys(row.Source, 1) || !validTypeRefRoot(counts, row.Source, row.Root) {
		return false
	}
	switch row.Resolution {
	case TypeRefUnresolved:
		return row.Target == 0 && len(row.Canonical) == 0
	case TypeRefDeclaration:
		return staticrole.TypeReferenceTarget(counts, row.Target) && len(row.Canonical) == 0
	case TypeRefCanonicalPath:
		return row.Target == 0 && validKeys(row.Canonical, 1)
	default:
		return false
	}
}

func validTypeRefRoot(counts [keyspace.FamilyCount]uint32, source []keyspace.Key, root keyspace.Term) bool {
	if len(source) == 1 {
		return root == 0
	}
	return hasFamily(counts, root, keyspace.FamilyCell)
}

func validKeys(keys []keyspace.Key, minimum int) bool {
	if len(keys) < minimum {
		return false
	}
	for _, key := range keys {
		if key == 0 {
			return false
		}
	}
	return true
}

func appendKeys(pool *[]keyspace.Key, values []keyspace.Key) (poolRange, bool) {
	start := len(*pool)
	if uint64(start)+uint64(len(values)) > uint64(math.MaxUint32) {
		return poolRange{}, false
	}
	*pool = append(*pool, values...)
	return poolRange{Start: uint32(start), End: uint32(len(*pool))}, true
}
