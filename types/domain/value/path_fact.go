package value

import (
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReconcilePathFactWithDeclaredRead reconciles path-sensitive evidence for an
// expression with the declared type obtained by reading that same expression.
// This is a precision/evidence-family relation, not a general subtype proof:
// full structural subtyping over recursive products is too expensive for this
// hot path and is not the semantic owner for same-expression fact selection.
func ReconcilePathFactWithDeclaredRead(narrowed, declared typ.Type) (typ.Type, bool) {
	if narrowed == nil || declared == nil {
		return narrowed, true
	}
	if alias, ok := declared.(*typ.Alias); ok {
		target := alias.UnaliasedTarget()
		reconciled, ok := ReconcilePathFactWithDeclaredRead(narrowed, target)
		if !ok || reconciled == nil {
			return reconciled, ok
		}
		if typ.TypeEquals(reconciled, target) {
			return declared, true
		}
		return typ.NewAlias(alias.Name, reconciled), true
	}
	narrowed = unwrap.Alias(narrowed)
	declared = unwrap.Alias(declared)
	if narrowed == nil || declared == nil || declared.Kind().IsPlaceholder() {
		return narrowed, true
	}
	if reconciled, ok := reconcileDeclaredProductContract(narrowed, declared); ok {
		return reconciled, true
	}
	if samePathFactFamily(narrowed, declared) {
		return narrowed, true
	}
	declaredNonNil, nilable := SplitNilable(declared)
	if nilable && declaredNonNil != nil && !typ.IsNever(declaredNonNil) {
		declaredNonNil = unwrap.Alias(declaredNonNil)
		if reconciled, ok := reconcileDeclaredProductContract(narrowed, declaredNonNil); ok {
			return reconciled, true
		}
		if samePathFactFamily(narrowed, declaredNonNil) {
			return narrowed, true
		}
		if samePathFactFamily(declaredNonNil, narrowed) {
			// The declared read permits nil; when the narrowed flow value itself
			// carries nil, that presence is authoritative and must survive the
			// reconciliation rather than collapsing to the non-nil family core.
			if _, narrowedNilable := SplitNilable(narrowed); narrowedNilable {
				return declared, true
			}
			return declaredNonNil, true
		}
		if unwrap.Function(declaredNonNil) != nil && unwrap.Function(narrowed) != nil {
			return declaredNonNil, true
		}
		if subtype.IsSubtype(narrowed, declaredNonNil) {
			return narrowed, true
		}
	}
	if unwrap.Function(declared) != nil && unwrap.Function(narrowed) != nil {
		return declared, true
	}
	if declaredReadCoveredByUnion(narrowed, declared) {
		return declared, true
	}
	if reconciled, ok := reconcileNarrowedAgainstUnionDeclared(narrowed, declared); ok {
		return reconciled, true
	}
	return nil, false
}

// reconcileNarrowedAgainstUnionDeclared handles the case where the declared
// read is a union and the narrowed flow evidence is a single concrete carrier
// (e.g. a record literal-shape) that matches exactly one union member by
// product contract. The narrowed value is then reconciled against that
// member: a narrowed value of one union member is a sound observation of the
// declared union read.
func reconcileNarrowedAgainstUnionDeclared(narrowed, declared typ.Type) (typ.Type, bool) {
	u := unwrap.Union(declared)
	if u == nil {
		return nil, false
	}
	if unwrap.Union(narrowed) != nil {
		return nil, false
	}
	var matched typ.Type
	var matchedReconciled typ.Type
	for _, member := range u.Members {
		memberCore := unwrap.Alias(member)
		if memberCore == nil {
			continue
		}
		if reconciled, ok := reconcileDeclaredProductContract(narrowed, memberCore); ok {
			if matched != nil {
				return nil, false
			}
			matched = member
			matchedReconciled = reconciled
			continue
		}
		if samePathFactFamily(narrowed, memberCore) {
			if matched != nil {
				return nil, false
			}
			matched = member
			matchedReconciled = narrowed
		}
	}
	if matched == nil {
		return nil, false
	}
	return matchedReconciled, true
}

func reconcileDeclaredProductContract(candidate, declared typ.Type) (typ.Type, bool) {
	candidate = unwrap.Alias(candidate)
	declared = unwrap.Alias(declared)
	if candidate == nil || declared == nil {
		return nil, false
	}
	if opt, ok := declared.(*typ.Optional); ok {
		return reconcileDeclaredProductContract(candidate, opt.Inner)
	}
	if rec, ok := declared.(*typ.Recursive); ok {
		if rec.Body == nil || rec.Body == rec {
			return nil, false
		}
		if _, ok := reconcileDeclaredProductContract(candidate, rec.Body); ok {
			return declared, true
		}
		return nil, false
	}
	candidateRecord, ok := candidate.(*typ.Record)
	if !ok {
		return nil, false
	}
	declaredRecord, ok := declared.(*typ.Record)
	if !ok {
		return nil, false
	}
	return reconcileDeclaredRecordContract(candidateRecord, declaredRecord)
}

func reconcileDeclaredRecordContract(candidate, declared *typ.Record) (typ.Type, bool) {
	if candidate == nil || declared == nil {
		return nil, false
	}
	builder := typ.NewRecord().SetOpen(candidate.Open && declared.Open)
	if candidate.Metatable != nil {
		builder.Metatable(candidate.Metatable)
	} else if declared.Metatable != nil {
		builder.Metatable(declared.Metatable)
	}
	switch {
	case candidate.HasMapComponent() && declared.HasMapComponent():
		key := typ.JoinPreferNonSoft(candidate.MapKey, declared.MapKey)
		value := typ.JoinPreferNonSoft(candidate.MapValue, declared.MapValue)
		builder.MapComponent(key, value)
	case candidate.HasMapComponent():
		builder.MapComponent(candidate.MapKey, candidate.MapValue)
	case declared.HasMapComponent():
		builder.MapComponent(declared.MapKey, declared.MapValue)
	}

	added := map[string]bool{}
	declaredNames := make(map[string]struct{}, len(declared.Fields))
	for _, declaredField := range declared.Fields {
		declaredNames[declaredField.Name] = struct{}{}
	}
	partialOverlay := candidate.Open && !declared.Open
	changed := false
	for _, declaredField := range declared.Fields {
		candidateField := candidate.GetField(declaredField.Name)
		if candidateField == nil {
			if !partialOverlay && !declaredFieldCanBeAbsent(declaredField) {
				return nil, false
			}
			addRecordField(builder, declaredField)
			added[declaredField.Name] = true
			changed = true
			continue
		}
		merged, ok := reconcileDeclaredFieldContract(candidateField.Type, declaredField.Type)
		if !ok {
			return nil, false
		}
		field := typ.Field{
			Name:     declaredField.Name,
			Type:     merged,
			Optional: candidateField.Optional && declaredField.Optional,
			Readonly: candidateField.Readonly || declaredField.Readonly,
		}
		addRecordField(builder, field)
		added[field.Name] = true
		if !typ.TypeEquals(merged, candidateField.Type) ||
			field.Optional != candidateField.Optional ||
			field.Readonly != candidateField.Readonly {
			changed = true
		}
	}
	for _, field := range candidate.Fields {
		if added[field.Name] {
			continue
		}
		if partialOverlay {
			if _, declared := declaredNames[field.Name]; !declared {
				return nil, false
			}
		}
		addRecordField(builder, field)
		changed = true
	}
	if !changed {
		return candidate, true
	}
	return builder.Build(), true
}

func reconcileDeclaredFieldContract(candidate, declared typ.Type) (typ.Type, bool) {
	if candidate == nil {
		if typeCanBeAbsent(declared) {
			return declared, true
		}
		return nil, false
	}
	if declared == nil || typ.TypeEquals(candidate, declared) {
		return candidate, true
	}
	if merged, ok := reconcileDeclaredProductContract(candidate, declared); ok {
		return merged, true
	}
	declaredNonNil, nilable := SplitNilable(declared)
	if nilable && declaredNonNil != nil {
		if merged, ok := reconcileDeclaredProductContract(candidate, declaredNonNil); ok {
			return merged, true
		}
	}
	if subtype.IsSubtype(candidate, declared) || samePathFactFamily(candidate, declared) {
		return candidate, true
	}
	if nilable && declaredNonNil != nil {
		if subtype.IsSubtype(candidate, declaredNonNil) || samePathFactFamily(candidate, declaredNonNil) {
			return candidate, true
		}
	}
	return nil, false
}

func declaredFieldCanBeAbsent(field typ.Field) bool {
	return field.Optional || typeCanBeAbsent(field.Type)
}

func typeCanBeAbsent(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) {
		return true
	}
	_, nilable := SplitNilable(t)
	return nilable
}

// SelectPathObservation chooses the canonical read observation for one
// expression path when both solved value-flow state and condition-proof state
// are available. Condition proofs may refine solved flow, but a proof-only
// placeholder must not erase a more precise solved product.
func SelectPathObservation(solved, proof, declared typ.Type) (typ.Type, bool) {
	solved, solvedOK := reconcileUsablePathObservation(solved, declared)
	proof, proofOK := reconcileUsablePathObservation(proof, declared)
	switch {
	case !solvedOK && !proofOK:
		return nil, false
	case !solvedOK:
		return proof, true
	case !proofOK:
		return solved, true
	}
	if typ.TypeEquals(proof, solved) {
		return solved, true
	}
	if typ.MorePrecise(proof, solved) {
		return proof, true
	}
	if typ.MorePrecise(solved, proof) {
		return solved, true
	}
	proofSubSolved := subtype.IsSubtype(proof, solved)
	solvedSubProof := subtype.IsSubtype(solved, proof)
	switch {
	case proofSubSolved && !solvedSubProof:
		return proof, true
	case solvedSubProof && !proofSubSolved:
		return solved, true
	default:
		return proof, true
	}
}

// SelectSourceProjection chooses the value carried by a same-source assignment
// projection and a solved source read. The projection is emitted only for
// higher-order source evidence such as callpoint function values and call
// return slots; it is not a target annotation. That makes the projection the
// authoritative source when the plain path/call read is stale or cannot encode
// capture-sensitive evidence, while still preserving a strictly more precise
// solved read when both observations belong to the same family.
func SelectSourceProjection(solved, projected typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(projected) {
		return solved
	}
	if typ.IsAbsentOrUnknown(solved) {
		return projected
	}
	if typ.TypeEquals(projected, solved) {
		return projected
	}
	if typ.MorePrecise(solved, projected) {
		return solved
	}
	if typ.MorePrecise(projected, solved) {
		return projected
	}
	projectedSubSolved := subtype.IsSubtype(projected, solved)
	solvedSubProjected := subtype.IsSubtype(solved, projected)
	switch {
	case solvedSubProjected && !projectedSubSolved:
		return solved
	case projectedSubSolved && !solvedSubProjected:
		return projected
	default:
		return projected
	}
}

func reconcileUsablePathObservation(observed, declared typ.Type) (typ.Type, bool) {
	reconciled, ok := ReconcilePathFactWithDeclaredRead(observed, declared)
	if !ok || typ.IsAbsentOrUnknown(reconciled) {
		return nil, false
	}
	return reconciled, true
}

func declaredReadCoveredByUnion(narrowed, declared typ.Type) bool {
	u := unwrap.Union(narrowed)
	if u == nil || declared == nil {
		return false
	}
	declaredNonNil, nilable := SplitNilable(declared)
	for _, member := range u.Members {
		member = unwrap.Alias(member)
		if samePathFactFamily(member, declared) || samePathFactFamily(declared, member) {
			return true
		}
		if nilable && declaredNonNil != nil {
			if samePathFactFamily(member, declaredNonNil) || samePathFactFamily(declaredNonNil, member) {
				return true
			}
		}
	}
	return false
}

func samePathFactFamily(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil {
		return candidate == baseline
	}
	if typ.TypeEquals(candidate, baseline) {
		return true
	}
	if typ.ContainsRecursive(candidate) && typ.ContainsRecursive(baseline) && SameEvidenceFamily(candidate, baseline) {
		return true
	}
	if _, ok := typ.ComparePrecision(candidate, baseline); ok {
		return true
	}
	return false
}
