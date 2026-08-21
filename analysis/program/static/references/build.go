package references

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/rows"
)

// Build validates and seals the authored TypeRef relation.
func Build(input Input, counts [keyspace.FamilyCount]uint32) (Table, error) {
	var keys rows.PoolBuilder[keyspace.Key]
	ref := rows.NewTableBuilder[TypeRefRow](keyspace.FamilyTypeRef)
	for _, row := range input.TypeRef {
		if !validTypeRef(counts, row) {
			return Table{}, errors.New("program/static/references: invalid type reference")
		}
		sourceSpan, ok := keys.Append(row.Source)
		if !ok {
			return Table{}, errors.New("program/static/references: oversized type reference source")
		}
		canonicalSpan, ok := keys.Append(row.Canonical)
		if !ok {
			return Table{}, errors.New("program/static/references: oversized type reference canonical path")
		}
		sealed := TypeRefRow{
			Resolution: row.Resolution, Target: row.Target, Root: row.Root,
			Source: sourceSpan, Canonical: canonicalSpan,
		}
		if _, ok := ref.Append(sealed); !ok {
			return Table{}, errors.New("program/static/references: oversized type reference table")
		}
	}
	return Table{ref: ref.Seal(), keys: keys.Seal()}, nil
}

func validTypeRef(counts [keyspace.FamilyCount]uint32, row TypeRef) bool {
	if !validKeys(row.Source, 1) || !validRoot(counts, row.Source, row.Root) {
		return false
	}
	switch row.Resolution {
	case Unresolved:
		return row.Target == 0 && len(row.Canonical) == 0
	case Declaration:
		return staticrole.TypeReferenceTarget(counts, row.Target) && len(row.Canonical) == 0
	case CanonicalPath:
		return row.Target == 0 && validKeys(row.Canonical, 1)
	default:
		return false
	}
}

// validRoot states the one qualification law: a single-segment spelling is
// unqualified and carries no root, and a longer one is rooted in a Cell.
func validRoot(counts [keyspace.FamilyCount]uint32, source []keyspace.Key, root keyspace.Term) bool {
	if len(source) == 1 {
		return root == 0
	}
	return keyspace.ValidTerm(root, keyspace.FamilyCell, int(counts[keyspace.FamilyCell]))
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
