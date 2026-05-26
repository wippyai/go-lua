package value

import (
	"sort"

	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// JoinRecordShape joins compatible record observations with the caller's slot
// join law. This keeps parameter/effect domains from accidentally inheriting
// return-slot semantics for fields such as any-vs-unknown.
func JoinRecordShape(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	ar, okA := unwrap.Alias(a).(*typ.Record)
	br, okB := unwrap.Alias(b).(*typ.Record)
	if !okA || !okB || ar == nil || br == nil {
		return nil, false
	}
	if recordsHaveConflictingRequiredLiterals(ar, br) || recordsHaveAsymmetricRequiredLiteral(ar, br) {
		return nil, false
	}
	if recordsAreRecursiveAlternatives(ar, br) {
		return nil, false
	}

	builder := typ.NewRecord()
	if ar.Open || br.Open {
		builder.SetOpen(true)
	}
	if mt := joinedRecordMetatable(ar.Metatable, br.Metatable, join); mt != nil {
		builder.Metatable(mt)
	}
	mapKey, mapValue, hasMap := joinedRecordMapComponent(ar, br, join)
	if hasMap {
		builder.MapComponent(mapKey, mapValue)
	}

	leftFields := fieldsByName(ar)
	rightFields := fieldsByName(br)
	names := make([]string, 0, len(leftFields)+len(rightFields))
	seen := make(map[string]struct{}, len(leftFields)+len(rightFields))
	for name := range leftFields {
		names = append(names, name)
		seen[name] = struct{}{}
	}
	for name := range rightFields {
		if _, ok := seen[name]; ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		lf, hasLeft := leftFields[name]
		rf, hasRight := rightFields[name]
		field := joinRecordFieldWithMapEvidence(name, lf, hasLeft, ar, rf, hasRight, br, join)
		addRecordField(builder, field)
	}

	return builder.Build(), true
}

// RecordExtensionUpperBound admits a record extension as the convergence upper
// bound only when doing so is monotone for the value evidence lattice. Plain
// construction histories can grow from {} to {field = T}, but once a baseline
// carries optional fields it represents observed absence from an alternative
// branch; a later required field must be joined structurally instead of erasing
// that absence.
func RecordExtensionUpperBound(a, b typ.Type) (typ.Type, bool) {
	if recordExtensionIsConvergenceUpperBound(a, b) {
		return a, true
	}
	if recordExtensionIsConvergenceUpperBound(b, a) {
		return b, true
	}
	return nil, false
}

func recordExtensionIsConvergenceUpperBound(candidate, baseline typ.Type) bool {
	if !ExtendsRecord(candidate, baseline) {
		return false
	}
	cr, okC := unwrap.Alias(candidate).(*typ.Record)
	br, okB := unwrap.Alias(baseline).(*typ.Record)
	if !okC || !okB || cr == nil || br == nil {
		return true
	}
	// A genuine record-construction extension adds fields ({} -> {field = T}); the
	// added fields are what make the candidate the upper bound. When the candidate
	// adds no fields it is the same record shape with differing field evidence,
	// which the merge joins field-wise so per-field convergence (including the
	// self-embedding fold on a cyclic field such as Bus.__index = Bus) runs instead
	// of admitting a deeper unfolding wholesale.
	if !recordAddsField(cr, br) {
		return false
	}
	return !recordExtensionErasesAlternativeAbsence(cr, br)
}

// recordAddsField reports whether candidate carries a field absent from baseline.
func recordAddsField(candidate, baseline *typ.Record) bool {
	baselineFields := fieldsByName(baseline)
	for _, field := range candidate.Fields {
		if _, ok := baselineFields[field.Name]; !ok {
			return true
		}
	}
	return false
}

func recordExtensionErasesAlternativeAbsence(candidate, baseline *typ.Record) bool {
	if candidate == nil || baseline == nil {
		return false
	}
	baselineFields := fieldsByName(baseline)
	for _, field := range candidate.Fields {
		base, ok := baselineFields[field.Name]
		if !ok {
			if recordHasOptionalEvidence(baseline) && !field.Optional && !unwrap.IsOptionalLike(field.Type) {
				return true
			}
			continue
		}
		if base.Optional && !field.Optional && !unwrap.IsOptionalLike(field.Type) {
			return true
		}
	}
	return false
}

func recordHasOptionalEvidence(rec *typ.Record) bool {
	if rec == nil {
		return false
	}
	for _, field := range rec.Fields {
		if field.Optional || unwrap.IsOptionalLike(field.Type) {
			return true
		}
	}
	return false
}

func joinedRecordMetatable(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) typ.Type {
	if a == nil || b == nil {
		return nil
	}
	if FactTypeEqual(a, b) {
		return a
	}
	return join(a, b)
}

func joinedRecordMapComponent(a, b *typ.Record, join func(typ.Type, typ.Type) typ.Type) (typ.Type, typ.Type, bool) {
	switch {
	case a.HasMapComponent() && b.HasMapComponent():
		return joinMapKey(a.MapKey, b.MapKey, join), join(a.MapValue, b.MapValue), true
	case a.HasMapComponent():
		return a.MapKey, a.MapValue, true
	case b.HasMapComponent():
		return b.MapKey, b.MapValue, true
	default:
		return nil, nil, false
	}
}

func fieldsByName(r *typ.Record) map[string]typ.Field {
	out := make(map[string]typ.Field, len(r.Fields))
	for _, field := range r.Fields {
		out[field.Name] = field
	}
	return out
}

func joinRecordField(
	name string,
	left typ.Field,
	hasLeft bool,
	right typ.Field,
	hasRight bool,
	join func(typ.Type, typ.Type) typ.Type,
) typ.Field {
	switch {
	case hasLeft && hasRight:
		return typ.Field{
			Name:     name,
			Type:     join(left.Type, right.Type),
			Optional: left.Optional || right.Optional,
			Readonly: left.Readonly && right.Readonly,
		}
	case hasLeft:
		left.Optional = true
		return left
	default:
		right.Optional = true
		return right
	}
}

func joinRecordFieldWithMapEvidence(
	name string,
	left typ.Field,
	hasLeft bool,
	leftRecord *typ.Record,
	right typ.Field,
	hasRight bool,
	rightRecord *typ.Record,
	join func(typ.Type, typ.Type) typ.Type,
) typ.Field {
	if hasLeft && hasRight {
		return joinRecordField(name, left, true, right, true, join)
	}
	if hasLeft {
		return fieldWithMissingBranchMapEvidence(name, left, rightRecord, join)
	}
	return fieldWithMissingBranchMapEvidence(name, right, leftRecord, join)
}

func fieldWithMissingBranchMapEvidence(
	name string,
	field typ.Field,
	missingBranch *typ.Record,
	join func(typ.Type, typ.Type) typ.Type,
) typ.Field {
	field.Optional = true
	if missingBranch != nil && missingBranch.HasMapComponent() &&
		subtype.IsSubtype(typ.LiteralString(name), missingBranch.MapKey) {
		field.Type = join(field.Type, missingBranch.MapValue)
	}
	return field
}

func addRecordField(builder *typ.RecordBuilder, field typ.Field) {
	switch {
	case field.Optional && field.Readonly:
		builder.OptReadonlyField(field.Name, field.Type)
	case field.Optional:
		builder.OptField(field.Name, field.Type)
	case field.Readonly:
		builder.ReadonlyField(field.Name, field.Type)
	default:
		builder.Field(field.Name, field.Type)
	}
}

func recordsHaveConflictingRequiredLiterals(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return false
	}
	for _, left := range a.Fields {
		if left.Optional {
			continue
		}
		right := b.GetField(left.Name)
		if right == nil || right.Optional {
			continue
		}
		if typ.IsDiscriminantLiteralField(left.Name) && requiredLiteralsConflict(left.Type, right.Type) {
			return true
		}
	}
	return false
}

func recordsHaveAsymmetricRequiredLiteral(a, b *typ.Record) bool {
	return recordHasRequiredLiteralMissingFrom(a, b) || recordHasRequiredLiteralMissingFrom(b, a)
}

func recordHasRequiredLiteralMissingFrom(src, dst *typ.Record) bool {
	if src == nil || dst == nil {
		return false
	}
	for _, field := range src.Fields {
		if field.Optional || !typ.IsDiscriminantLiteralField(field.Name) || !isLiteralType(field.Type) {
			continue
		}
		other := dst.GetField(field.Name)
		if other == nil || other.Optional {
			return true
		}
	}
	return false
}

func requiredLiteralsConflict(a, b typ.Type) bool {
	al, okA := unwrap.Alias(a).(*typ.Literal)
	bl, okB := unwrap.Alias(b).(*typ.Literal)
	return okA && okB && al.Base == bl.Base && !typ.TypeEquals(al, bl)
}

func isLiteralType(t typ.Type) bool {
	_, ok := unwrap.Alias(t).(*typ.Literal)
	return ok
}

func recordsAreRecursiveAlternatives(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return false
	}
	return recordContainsEquivalentField(a, b) || recordContainsEquivalentField(b, a)
}

func recordContainsEquivalentField(container, target *typ.Record) bool {
	for _, field := range container.Fields {
		if typ.SameNodeOrAcyclicEqual(field.Type, target) || ContainsEquivalent(field.Type, target) {
			return true
		}
	}
	if container.HasMapComponent() &&
		(typ.SameNodeOrAcyclicEqual(container.MapValue, target) || ContainsEquivalent(container.MapValue, target)) {
		return true
	}
	if container.Metatable != nil &&
		(typ.SameNodeOrAcyclicEqual(container.Metatable, target) || ContainsEquivalent(container.Metatable, target)) {
		return true
	}
	return false
}
