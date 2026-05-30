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
	if recordsHaveConflictingRequiredLiterals(ar, br) || recordsHaveDisjointPayload(ar, br) {
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

// RecordWidthDiffer reports whether a and b are two records the structural join
// merges (JoinRecordShape admits them) whose field sets differ, so their least
// upper bound optionalizes the non-shared fields. Width-covering does not make
// the covering record the LUB here, so callers must route such a pair through the
// optionalizing structural join rather than returning either operand whole.
func RecordWidthDiffer(a, b typ.Type) bool {
	ar, okA := unwrap.Alias(a).(*typ.Record)
	br, okB := unwrap.Alias(b).(*typ.Record)
	if !okA || !okB || ar == nil || br == nil {
		return false
	}
	if recordsHaveConflictingRequiredLiterals(ar, br) || recordsHaveDisjointPayload(ar, br) {
		return false
	}
	if recordsAreRecursiveAlternatives(ar, br) {
		return false
	}
	return recordAddsField(ar, br) || recordAddsField(br, ar)
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
		zzMTlog(a, b, nil)
		return nil
	}
	if FactTypeEqual(a, b) {
		return a
	}
	out := join(a, b)
	zzMTlog(a, b, out)
	return out
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
	// A field absent from the missing branch's top level but reachable through
	// its __index prototype (the Lua method-resolution path) is still accessible
	// there, so the two records are the same class observed as a self view
	// ({m, __index}) and a split view ({__index: {m}}). Keep it required and join
	// against the reachable type instead of optionalizing the surface.
	if reached, ok := fieldReachableViaIndex(missingBranch, name); ok {
		field.Type = join(field.Type, reached)
		return field
	}
	field.Optional = true
	if missingBranch != nil && missingBranch.HasMapComponent() &&
		subtype.IsSubtype(typ.LiteralString(name), missingBranch.MapKey) {
		field.Type = join(field.Type, missingBranch.MapValue)
	}
	return field
}

// fieldReachableViaIndex reports the type of name reachable through rec's
// __index prototype chain, the Lua method-resolution path. It follows __index
// fields whose value is a record, bounded against self-cycles, so a method that
// lives in the prototype of a split-view class record counts as present.
func fieldReachableViaIndex(rec *typ.Record, name string) (typ.Type, bool) {
	const indexField = "__index"
	seen := make(map[*typ.Record]bool, 4)
	for rec != nil && !seen[rec] {
		seen[rec] = true
		idx := rec.GetField(indexField)
		if idx == nil {
			return nil, false
		}
		proto, ok := recordUnderRecursive(idx.Type)
		if !ok || proto == nil {
			return nil, false
		}
		if f := proto.GetField(name); f != nil {
			return f.Type, true
		}
		rec = proto
	}
	return nil, false
}

// recordUnderRecursive unwraps a record from a transparent alias or a sealed
// recursive family body, so an __index prototype tied into a class mu still
// exposes its method surface for reachability.
func recordUnderRecursive(t typ.Type) (*typ.Record, bool) {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Record:
		return v, true
	case *typ.Recursive:
		if v != nil && v.Body != nil && v.Body != typ.Type(v) {
			return recordUnderRecursive(v.Body)
		}
	}
	return nil, false
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
	return typ.RecordsConflictOnLiteralDiscriminant(a, b)
}

// recordsHaveDisjointPayload reports whether two records are distinct tagged
// variants: one carries a required literal field the other lacks, and their
// non-literal payloads are mutually disjoint. The asymmetric literal alone is just
// data (config records with differing literal keys collapse), and disjoint payloads
// alone are partial records that optionalize on merge; only their combination
// (for example {kind, content} versus {tool, arguments}) marks a partition.
func recordsHaveDisjointPayload(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return false
	}
	if !recordHasAsymmetricRequiredLiteral(a, b) {
		return false
	}
	return recordRequiredPayloadMissingFrom(a, b) && recordRequiredPayloadMissingFrom(b, a)
}

// recordHasAsymmetricRequiredLiteral reports whether either record carries a
// required literal field the other does not require, the structural sign that a
// variant tag is present on one alternative only.
func recordHasAsymmetricRequiredLiteral(a, b *typ.Record) bool {
	return recordHasRequiredLiteralMissingFrom(a, b) || recordHasRequiredLiteralMissingFrom(b, a)
}

func recordHasRequiredLiteralMissingFrom(src, dst *typ.Record) bool {
	for _, field := range src.Fields {
		if field.Optional || !isLiteralType(field.Type) {
			continue
		}
		other := dst.GetField(field.Name)
		if other == nil || other.Optional {
			return true
		}
	}
	return false
}

// recordRequiredPayloadMissingFrom reports whether src requires a non-literal field
// dst lacks entirely. Such mutual absence is a disjoint payload rather than additive
// width that optionalizes on merge.
func recordRequiredPayloadMissingFrom(src, dst *typ.Record) bool {
	for _, field := range src.Fields {
		if field.Optional || isLiteralType(field.Type) {
			continue
		}
		if dst.GetField(field.Name) == nil {
			return true
		}
	}
	return false
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
