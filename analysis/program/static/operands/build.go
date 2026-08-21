package operands

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/internal/rows"
)

// Build validates and seals Static's exact operand sidecars. It consumes the
// already-sealed Types and References tables as stage inputs: whether a
// runtime type target is loadable is a fact those owners publish, and this
// vertical must not re-derive it from their storage.
func Build(input Input, counts [keyspace.FamilyCount]uint32, types statictypes.Table, refs staticrefs.Table) (Table, error) {
	// The dense table makes duplicate detection and later O(1) lookup exact.
	// The retained semantic relation below remains sparse.
	targets := make([]keyspace.Term, int(counts[keyspace.FamilyValueClaim]))
	for _, row := range input.Claim {
		if !hasFamily(counts, row.Claim, keyspace.FamilyValueClaim) || !staticrole.Node(counts, row.Target) {
			return Table{}, errors.New("program/static/operands: invalid claim target")
		}
		ordinal := keyspace.TermOrdinal(row.Claim) - 1
		if targets[ordinal] != 0 {
			return Table{}, errors.New("program/static/operands: duplicate claim target")
		}
		targets[ordinal] = row.Target
	}
	var claim rows.RowsBuilder[ClaimTarget]
	for ordinal, target := range targets {
		if target == 0 {
			continue
		}
		row := ClaimTarget{Claim: keyspace.MakeTerm(keyspace.FamilyValueClaim, uint32(ordinal+1)), Target: target}
		if _, ok := claim.Append(row); !ok {
			return Table{}, errors.New("program/static/operands: oversized claim table")
		}
	}
	typeValue := rows.NewTableBuilder[keyspace.Term](keyspace.FamilyTypeValue)
	for _, row := range input.TypeValue {
		if !validTypeValueTarget(counts, types, refs, row.Target) {
			return Table{}, errors.New("program/static/operands: invalid runtime type target")
		}
		if _, ok := typeValue.Append(row.Target); !ok {
			return Table{}, errors.New("program/static/operands: oversized runtime type target table")
		}
	}

	annotation := rows.NewTableBuilder[Annotation](keyspace.FamilyAnnotation)
	for _, row := range input.Annotation {
		if !validAnnotation(counts, row) {
			return Table{}, errors.New("program/static/operands: invalid annotation")
		}
		if _, ok := annotation.Append(row); !ok {
			return Table{}, errors.New("program/static/operands: oversized annotation table")
		}
	}
	annotations := annotation.Seal()
	index, ok := buildAnnotationIndex(annotations)
	if !ok {
		return Table{}, errors.New("program/static/operands: oversized annotation index")
	}
	return Table{
		claim:      claim.Seal(),
		typeValue:  typeValue.Seal(),
		annotation: annotations,
		index:      index,
	}, nil
}

// validTypeValueTarget admits exactly the targets a runtime type singleton can
// name. The primitive case reads the Types owner's published row; the
// reference case reads the References owner's published disposition.
func validTypeValueTarget(counts [keyspace.FamilyCount]uint32, types statictypes.Table, refs staticrefs.Table, target keyspace.Term) bool {
	switch keyspace.TermFamily(target) {
	case keyspace.FamilyTypePrimitive:
		row, ok := types.Primitive(target)
		return ok && row.Kind.RuntimeLoadable()
	case keyspace.FamilyTypeRef:
		row, ok := refs.Ref(target)
		if !ok || row.Resolution != staticrefs.Declaration {
			return false
		}
		family := keyspace.TermFamily(row.Target)
		return (family == keyspace.FamilyTypeAlias || family == keyspace.FamilyTypeInterface) &&
			keyspace.TermOrdinal(row.Target) != 0 && keyspace.TermOrdinal(row.Target) <= counts[family]
	default:
		return false
	}
}

func validAnnotation(counts [keyspace.FamilyCount]uint32, row Annotation) bool {
	return staticrole.ScopeHandle(counts, row.Scope) &&
		staticrole.AnnotationTarget(counts, row.Target) &&
		row.Name != 0 && hasFamily(counts, row.Values, keyspace.FamilyValues)
}

// indexRow is construction-only pairing used to order the query index.
type indexRow struct{ target, term keyspace.Term }

// buildAnnotationIndex constructs only a direct-query acceleration structure.
// Its order is stable by target Term, then authored Annotation ordinal.
func buildAnnotationIndex(annotations rows.Table[Annotation]) (AnnotationIndex, bool) {
	pairs := make([]indexRow, 0, annotations.Count())
	for term, row := range annotations.Terms() {
		pairs = append(pairs, indexRow{target: row.Target, term: term})
	}
	sort.Slice(pairs, func(left, right int) bool {
		if pairs[left].target != pairs[right].target {
			return pairs[left].target < pairs[right].target
		}
		return pairs[left].term < pairs[right].term
	})
	var targets []keyspace.Term
	var windows []rows.Span
	var terms rows.PoolBuilder[keyspace.Term]
	for start := 0; start < len(pairs); {
		end := start + 1
		for end < len(pairs) && pairs[end].target == pairs[start].target {
			end++
		}
		group := make([]keyspace.Term, 0, end-start)
		for _, pair := range pairs[start:end] {
			group = append(group, pair.term)
		}
		window, ok := terms.Append(group)
		if !ok {
			return AnnotationIndex{}, false
		}
		targets = append(targets, pairs[start].target)
		windows = append(windows, window)
		start = end
	}
	return AnnotationIndex{
		targets: rows.NewRows(targets),
		windows: rows.NewRows(windows),
		terms:   terms.Seal(),
	}, true
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}
