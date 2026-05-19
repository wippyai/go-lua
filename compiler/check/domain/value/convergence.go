package value

import (
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// NormalizeFactType canonicalizes one type before it is stored in an
// interprocedural fact slot.
func NormalizeFactType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return fn
	}
	return typ.PruneSoftUnionMembers(t)
}

// WidenForConvergence applies the finite-height approximation needed for
// higher-order structural growth.
func WidenForConvergence(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if !HasHigherOrderGrowthRisk(t) {
		return t
	}
	return subtype.WidenForInference(t)
}

// WidenFunctionForConvergence applies convergence widening to a function type.
func WidenFunctionForConvergence(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	if widened, ok := WidenForConvergence(fn).(*typ.Function); ok {
		return widened
	}
	return fn
}

// JoinPrecise merges non-function value facts inside one analysis iteration.
func JoinPrecise(existing, candidate typ.Type) typ.Type {
	existing = NormalizeFactType(existing)
	candidate = NormalizeFactType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

// MergeForConvergence merges non-function value facts at a fixpoint boundary.
func MergeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = NormalizeFactType(existing)
	candidate = NormalizeFactType(candidate)
	if existing == nil {
		return WidenForConvergence(candidate)
	}
	if candidate == nil {
		return WidenForConvergence(existing)
	}
	existing = WidenForConvergence(existing)
	candidate = WidenForConvergence(candidate)
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if unwrap.IsNilType(existing) && !unwrap.IsNilType(candidate) {
		return candidate
	}
	if unwrap.IsNilType(candidate) && !unwrap.IsNilType(existing) {
		return existing
	}
	if typ.IsAny(existing) || typ.IsUnknown(existing) {
		return existing
	}
	if typ.IsAny(candidate) || typ.IsUnknown(candidate) {
		return candidate
	}
	if ElidesOptional(candidate, existing) {
		return candidate
	}
	if ExtendsRecord(candidate, existing) && !ContainsNestedStructuralShape(candidate, existing) {
		return candidate
	}
	if refines, _ := RefinesFalsyMapKey(candidate, existing); refines {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

// UnsafePrecisionDrop reports whether merged lost a previously possible branch
// from prev while appearing as a subtype refinement.
func UnsafePrecisionDrop(prev, merged typ.Type) bool {
	if prev == nil || merged == nil || typ.TypeEquals(prev, merged) {
		return false
	}
	if ElidesOptional(merged, prev) {
		return false
	}
	if refines, _ := RefinesFalsyMapKey(merged, prev); refines {
		return false
	}
	if typ.IsAny(prev) || typ.IsUnknown(prev) {
		return true
	}

	switch p := UnwrapStructuralShape(prev).(type) {
	case *typ.Union:
		if unionStrictMemberSubset(merged, p) {
			return true
		}
		if subtype.IsSubtype(merged, p) && !subtype.IsSubtype(p, merged) {
			return true
		}
	case *typ.Record:
		m, ok := UnwrapStructuralShape(merged).(*typ.Record)
		if !ok {
			break
		}
		for _, pf := range p.Fields {
			mf := m.GetField(pf.Name)
			if mf != nil && UnsafePrecisionDrop(pf.Type, mf.Type) {
				return true
			}
		}
		if p.HasMapComponent() && m.HasMapComponent() && UnsafePrecisionDrop(p.MapValue, m.MapValue) {
			return true
		}
	case *typ.Array:
		if m, ok := UnwrapStructuralShape(merged).(*typ.Array); ok {
			return UnsafePrecisionDrop(p.Element, m.Element)
		}
	case *typ.Map:
		if m, ok := UnwrapStructuralShape(merged).(*typ.Map); ok {
			return UnsafePrecisionDrop(p.Key, m.Key) || UnsafePrecisionDrop(p.Value, m.Value)
		}
	case *typ.Tuple:
		m, ok := UnwrapStructuralShape(merged).(*typ.Tuple)
		if !ok || len(p.Elements) != len(m.Elements) {
			break
		}
		for i := range p.Elements {
			if UnsafePrecisionDrop(p.Elements[i], m.Elements[i]) {
				return true
			}
		}
	case *typ.Function:
		m, ok := UnwrapStructuralShape(merged).(*typ.Function)
		if !ok {
			break
		}
		for i := 0; i < len(p.Params) && i < len(m.Params); i++ {
			if UnsafePrecisionDrop(p.Params[i].Type, m.Params[i].Type) {
				return true
			}
		}
		for i := 0; i < len(p.Returns) && i < len(m.Returns); i++ {
			if UnsafePrecisionDrop(p.Returns[i], m.Returns[i]) {
				return true
			}
		}
	}

	if subtype.IsSubtype(merged, prev) && !subtype.IsSubtype(prev, merged) && !ExtendsRecord(merged, prev) {
		return true
	}
	return false
}

func unionStrictMemberSubset(candidate typ.Type, baseline *typ.Union) bool {
	if baseline == nil {
		return false
	}
	candidateMembers := UnionMembers(candidate)
	if len(candidateMembers) == 0 {
		candidateMembers = []typ.Type{candidate}
	}
	if len(candidateMembers) >= len(baseline.Members) {
		return false
	}
	for _, member := range candidateMembers {
		found := false
		for _, baseMember := range baseline.Members {
			if typ.TypeEquals(member, baseMember) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
