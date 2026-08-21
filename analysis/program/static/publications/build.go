package publications

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/internal/rows"
)

// slot is the construction-only write-pair identity used to reject a repeated
// publication of the same Assign position. It is not retained.
type slot struct {
	assign keyspace.Term
	pair   uint32
}

// Build validates and seals the authored publication relation. It consumes
// the already-sealed References table as a stage input: a publication target
// must be a reference whose binder disposition means something beyond the
// local spelling, and References is the owner that decides that.
func Build(input Input, counts [keyspace.FamilyCount]uint32, refs staticrefs.Table) (Table, error) {
	seen := make(map[slot]struct{}, len(input.Type))
	publication := rows.NewTableBuilder[Publication](keyspace.FamilyTypePublication)
	for _, row := range input.Type {
		if !keyspace.ValidTerm(row.Assign, keyspace.FamilyAssign, int(counts[keyspace.FamilyAssign])) || !resolvedTarget(refs, row.Target) {
			return Table{}, errors.New("program/static/publications: invalid type publication")
		}
		position := slot{assign: row.Assign, pair: row.Pair}
		if _, duplicate := seen[position]; duplicate {
			return Table{}, errors.New("program/static/publications: duplicate type publication pair")
		}
		seen[position] = struct{}{}
		if _, ok := publication.Append(row); !ok {
			return Table{}, errors.New("program/static/publications: oversized publication table")
		}
	}
	return Table{publication: publication.Seal()}, nil
}

// resolvedTarget accepts only the binder dispositions whose target is
// meaningful beyond the local spelling. An unresolved name is never made
// public merely because an Assign happens to contain one.
func resolvedTarget(refs staticrefs.Table, target keyspace.Term) bool {
	row, ok := refs.Ref(target)
	return ok && row.Resolved()
}
