package flow

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
)

// VariantOriginFamily returns a deterministic family id for a finite variant
// provenance relation over target.field.
func VariantOriginFamily(target constraint.Path, field string) uint64 {
	h := internal.HashCombine(internal.FnvString("variant-origin"), target.Hash())
	return internal.HashCombine(h, internal.FnvString(field))
}

// VariantFieldPathRelationKind describes an observed relation between a
// variant result field and the path that produced one of its cases.
type VariantFieldPathRelationKind uint8

const (
	VariantFieldPathEquals VariantFieldPathRelationKind = iota + 1
	VariantFieldPathNotEquals
)

// VariantFieldPathRelation projects an observed field/path relation into the
// complete constraint set that represents it, including variant provenance.
type VariantFieldPathRelation struct {
	Origins []VariantFieldOrigin
	Target  constraint.Path
	Field   string
	Source  constraint.Path
	Kind    VariantFieldPathRelationKind
}

// VariantFieldPathRelationConstraints is the canonical projection for field
// relation observations. Consumers should not assemble the base field relation
// and variant-origin case facts separately.
func VariantFieldPathRelationConstraints(relation VariantFieldPathRelation) []constraint.Constraint {
	switch relation.Kind {
	case VariantFieldPathEquals:
		out := []constraint.Constraint{constraint.FieldEqualsPath{
			Target: relation.Target,
			Field:  relation.Field,
			Value:  relation.Source,
		}}
		return append(out, variantFieldOriginEqualityConstraints(relation.Origins, relation.Target, relation.Field, relation.Source)...)
	case VariantFieldPathNotEquals:
		out := []constraint.Constraint{constraint.FieldNotEqualsPath{
			Target: relation.Target,
			Field:  relation.Field,
			Value:  relation.Source,
		}}
		return append(out, variantFieldOriginNotEqualityConstraints(relation.Origins, relation.Target, relation.Field, relation.Source)...)
	default:
		return nil
	}
}

func variantFieldOriginEqualityConstraints(origins []VariantFieldOrigin, target constraint.Path, field string, source constraint.Path) []constraint.Constraint {
	return variantFieldOriginConstraints(origins, target, field, source, true)
}

func variantFieldOriginNotEqualityConstraints(origins []VariantFieldOrigin, target constraint.Path, field string, source constraint.Path) []constraint.Constraint {
	return variantFieldOriginConstraints(origins, target, field, source, false)
}

func variantFieldOriginConstraints(origins []VariantFieldOrigin, target constraint.Path, field string, source constraint.Path, equals bool) []constraint.Constraint {
	if len(origins) == 0 || target.IsEmpty() || source.IsEmpty() || field == "" {
		return nil
	}
	var out []constraint.Constraint
	for _, origin := range origins {
		if origin.Field != field ||
			!VariantOriginPathMatches(origin.Target, target) ||
			!VariantOriginPathMatches(origin.Source, source) ||
			origin.OriginFamily == 0 {
			continue
		}
		if equals {
			out = append(out, constraint.VariantCaseEquals{
				Target:       origin.Target,
				OriginFamily: origin.OriginFamily,
				CaseIndex:    origin.CaseIndex,
			})
			continue
		}
		out = append(out, constraint.VariantCaseNotEquals{
			Target:       origin.Target,
			OriginFamily: origin.OriginFamily,
			CaseIndex:    origin.CaseIndex,
		})
	}
	return out
}

// VariantOriginPathMatches accepts exact stable paths while allowing a
// version-agnostic origin or actual path to match its versioned counterpart.
func VariantOriginPathMatches(origin, actual constraint.Path) bool {
	if origin.IsEmpty() || actual.IsEmpty() {
		return false
	}
	if origin.Symbol != 0 || actual.Symbol != 0 {
		if origin.Symbol != actual.Symbol {
			return false
		}
		if origin.Version != 0 && actual.Version != 0 && origin.Version != actual.Version {
			return false
		}
	} else if origin.Root != actual.Root {
		return false
	}
	if len(origin.Segments) != len(actual.Segments) {
		return false
	}
	for i := range origin.Segments {
		if origin.Segments[i] != actual.Segments[i] {
			return false
		}
	}
	return true
}
