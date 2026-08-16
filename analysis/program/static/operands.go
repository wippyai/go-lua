package static

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// compactOperands owns Static's exact operand sidecars. It does not inspect
// Flow claim kinds, reconstruct values, or derive annotations: those would
// duplicate Flow/Source authority. The later joint seal closes those cross-
// owner relations.
func compactOperands(component *Component, counts [keyspace.FamilyCount]uint32, input OperandsInput) error {
	store := &component.operands
	// The temporary dense table makes duplicate detection and later O(1)
	// lookup exact. The retained semantic relation below remains sparse.
	targets := make([]keyspace.Term, counts[keyspace.FamilyValueClaim])
	for _, row := range input.Claim {
		if !hasFamily(counts, row.Claim, keyspace.FamilyValueClaim) || !staticrole.Node(counts, row.Target) {
			return errors.New("program/static: invalid claim target")
		}
		ordinal := keyspace.TermOrdinal(row.Claim) - 1
		if targets[ordinal] != 0 {
			return errors.New("program/static: duplicate claim target")
		}
		targets[ordinal] = row.Target
	}
	store.claimTargets = targets
	for ordinal, target := range targets {
		if target != 0 {
			store.claims = append(store.claims, claimTargetRow{
				claim:  keyspace.MakeTerm(keyspace.FamilyValueClaim, uint32(ordinal+1)),
				target: target,
			})
		}
	}
	for _, row := range input.TypeValue {
		if !validTypeValueTarget(component, counts, row.Target) {
			return errors.New("program/static: invalid runtime type target")
		}
		store.typeValues = append(store.typeValues, row.Target)
	}
	for _, row := range input.Annotation {
		if !validAnnotation(counts, row) {
			return errors.New("program/static: invalid annotation")
		}
		store.annotations = append(store.annotations, row)
	}
	if !buildAnnotationCSR(store) {
		return errors.New("program/static: oversized annotation index")
	}
	return nil
}

func validTypeValueTarget(component *Component, counts [keyspace.FamilyCount]uint32, target keyspace.Term) bool {
	switch keyspace.TermFamily(target) {
	case keyspace.FamilyTypePrimitive:
		if !hasFamily(counts, target, keyspace.FamilyTypePrimitive) {
			return false
		}
		return component.types.primitive[keyspace.TermOrdinal(target)-1].Kind.RuntimeLoadable()
	case keyspace.FamilyTypeRef:
		if !hasFamily(counts, target, keyspace.FamilyTypeRef) {
			return false
		}
		row := component.references.rows[keyspace.TermOrdinal(target)-1]
		return row.resolution == TypeRefDeclaration &&
			(keyspace.TermFamily(row.target) == keyspace.FamilyTypeAlias || keyspace.TermFamily(row.target) == keyspace.FamilyTypeInterface) &&
			validTerm(counts, row.target)
	default:
		return false
	}
}

func validAnnotation(counts [keyspace.FamilyCount]uint32, row Annotation) bool {
	return staticrole.ScopeHandle(counts, row.Scope) &&
		staticrole.AnnotationTarget(counts, row.Target) &&
		row.Name != 0 && hasFamily(counts, row.Values, keyspace.FamilyValues)
}

func annotationTargetPresent(component *Component, target keyspace.Term) bool {
	if component == nil || keyspace.TermOrdinal(target) == 0 {
		return false
	}
	ordinal := keyspace.TermOrdinal(target)
	if keyspace.TermFamily(target) == keyspace.FamilyTypeField {
		return uint64(ordinal) <= uint64(len(component.types.field))
	}
	if !staticNodeFamily(keyspace.TermFamily(target)) {
		return false
	}
	return uint64(ordinal) <= uint64(staticNodeCount(component, keyspace.TermFamily(target)))
}

// staticNodeCount maps the closed static-node vocabulary to its owning store.
// It is an immutable query boundary, not a generic node representation.
func staticNodeCount(component *Component, family keyspace.Family) int {
	switch family {
	case keyspace.FamilyTypePrimitive:
		return len(component.types.primitive)
	case keyspace.FamilyTypeLiteral:
		return len(component.types.literal)
	case keyspace.FamilyTypeOptional:
		return len(component.types.optional)
	case keyspace.FamilyTypeUnion:
		return len(component.types.union)
	case keyspace.FamilyTypeIntersection:
		return len(component.types.intersection)
	case keyspace.FamilyTypeRef:
		return len(component.references.rows)
	case keyspace.FamilyTypeGeneric:
		return len(component.types.generic)
	case keyspace.FamilyTypeArray:
		return len(component.types.array)
	case keyspace.FamilyTypeMap:
		return len(component.types.mapType)
	case keyspace.FamilyTypeRecord:
		return len(component.types.record)
	case keyspace.FamilyTypeFunction:
		return len(component.signatures.functions)
	case keyspace.FamilyTypeAsserts:
		return len(component.signatures.assertions)
	case keyspace.FamilyTypeOf:
		return len(component.operators.typeOf)
	case keyspace.FamilyTypeKeyOf:
		return len(component.operators.keyOf)
	case keyspace.FamilyTypeIndexAccess:
		return len(component.operators.indexAccess)
	case keyspace.FamilyTypeConditional:
		return len(component.operators.conditional)
	default:
		return 0
	}
}

type annotationIndexRow struct {
	target keyspace.Term
	term   keyspace.Term
}

// buildAnnotationCSR constructs only a direct-query acceleration structure.
// Its order is stable by target Term, then authored Annotation ordinal.
func buildAnnotationCSR(store *operandsStore) bool {
	rows := make([]annotationIndexRow, len(store.annotations))
	for index, row := range store.annotations {
		rows[index] = annotationIndexRow{
			target: row.Target,
			term:   keyspace.MakeTerm(keyspace.FamilyAnnotation, uint32(index+1)),
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].target != rows[right].target {
			return rows[left].target < rows[right].target
		}
		return rows[left].term < rows[right].term
	})
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].target == rows[start].target {
			end++
		}
		if uint64(len(store.annotationTerms))+uint64(end-start) > uint64(^uint32(0)) {
			return false
		}
		store.annotationTargets = append(store.annotationTargets, rows[start].target)
		from := uint32(len(store.annotationTerms))
		for _, row := range rows[start:end] {
			store.annotationTerms = append(store.annotationTerms, row.term)
		}
		store.annotationRanges = append(store.annotationRanges, poolRange{Start: from, End: uint32(len(store.annotationTerms))})
		start = end
	}
	return true
}

// emitOperandsContainment retains Flow terms solely as opaque parents of the
// authored Static operand they already identify. Annotation is metadata, not
// containment, and therefore deliberately emits no edge.
func emitOperandsContainment(component *Component, check *containment) bool {
	for ordinal, target := range component.operands.claimTargets {
		if target != 0 && !check.attach(keyspace.MakeTerm(keyspace.FamilyValueClaim, uint32(ordinal+1)), target) {
			return false
		}
	}
	for ordinal, target := range component.operands.typeValues {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeValue, uint32(ordinal+1)), target) {
			return false
		}
	}
	return true
}

// writeOperandsContent owns the sparse ClaimTarget relation and the two
// dense sidecars. CSR rows and dense claim lookup are derived indexes, so are
// intentionally excluded.
func writeOperandsContent(writer *framing.Writer, store operandsStore) error {
	if err := writer.Count(uint64(len(store.claims))); err != nil {
		return err
	}
	for _, row := range store.claims {
		if err := writer.Uint(uint64(row.claim)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.typeValues))); err != nil {
		return err
	}
	for _, target := range store.typeValues {
		if err := writer.Uint(uint64(target)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.annotations))); err != nil {
		return err
	}
	for _, row := range store.annotations {
		if err := writer.Uint(uint64(row.Scope)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Values)); err != nil {
			return err
		}
	}
	return nil
}
