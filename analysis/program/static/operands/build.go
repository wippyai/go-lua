package operands

import (
	"errors"

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
		if !keyspace.ValidTerm(row.Claim, keyspace.FamilyValueClaim, int(counts[keyspace.FamilyValueClaim])) || !staticrole.Node(counts, row.Target) {
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
	return Table{
		claim:      claim.Seal(),
		typeValue:  typeValue.Seal(),
		annotation: annotation.Seal(),
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
		row.Name != 0 && keyspace.ValidTerm(row.Values, keyspace.FamilyValues, int(counts[keyspace.FamilyValues]))
}

