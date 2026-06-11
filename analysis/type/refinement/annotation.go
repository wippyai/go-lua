package refinement

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// IsRefinableAnnotation reports whether an explicit annotation should be
// treated as a soft placeholder that call-site/contextual hints may refine.
//
// Canonical rule: explicit top types (`any`, `unknown`) are authoritative and
// must not be rewritten by hints. Structural soft placeholders like `{any}` or
// `any[]` remain refinable.
func IsRefinableAnnotation(t typ.Type) bool {
	if t == nil {
		return false
	}
	if t.Kind().IsPlaceholder() {
		return false
	}
	return isRefinableStructuralAnnotation(t, typ.NewGuard())
}

// IsClosedUnionAnnotation reports whether a declared annotation is a multi-member
// union whose members are all concrete (no placeholder/any/unknown at the top
// level of any member). Such an annotation carries a closed discriminant domain
// that flow narrowing must preserve at the variable's root even if the union
// has refinable slots deep inside member fields.
func IsClosedUnionAnnotation(t typ.Type) bool {
	if t == nil {
		return false
	}
	u := closedUnionOf(t)
	if u == nil || len(u.Members) < 2 {
		return false
	}
	for _, member := range u.Members {
		if member == nil {
			return false
		}
		if typ.AbsentOrUnknown(member) || member.Kind().IsPlaceholder() {
			return false
		}
	}
	return true
}

// PruneLessPreciseRefinableUnionMembers removes refinable structural
// placeholder members from a union when another member carries comparable,
// strictly more precise evidence for the same runtime shape.
func PruneLessPreciseRefinableUnionMembers(t typ.Type, morePrecise MorePreciseFunc, normalizeUnion func(...typ.Type) typ.Type) typ.Type {
	u, ok := t.(*typ.Union)
	if !ok || len(u.Members) < 2 || morePrecise == nil {
		return t
	}
	keep := make([]typ.Type, 0, len(u.Members))
	for i, member := range u.Members {
		if member == nil {
			continue
		}
		if !IsRefinableAnnotation(member) {
			keep = append(keep, member)
			continue
		}
		dominated := false
		for j, candidate := range u.Members {
			if i == j || candidate == nil {
				continue
			}
			if morePrecise(candidate, member) {
				dominated = true
				break
			}
		}
		if !dominated {
			keep = append(keep, member)
		}
	}
	if len(keep) == 0 {
		return t
	}
	if len(keep) == len(u.Members) {
		return t
	}
	if len(keep) == 1 {
		return keep[0]
	}
	if normalizeUnion != nil {
		return normalizeUnion(keep...)
	}
	return typ.NewUnion(keep...)
}

// closedUnionOf unwraps Annotated/Alias/Optional layers to expose an
// underlying Union for refinement-local pruning.
func closedUnionOf(t typ.Type) *typ.Union {
	for {
		switch v := unwrap.Annotated(t).(type) {
		case *typ.Union:
			return v
		case *typ.Alias:
			t = v.Target
		case *typ.Optional:
			t = v.Inner
		case *typ.Instantiated:
			t = expandClosedUnionInstantiatedBody(v)
		default:
			return nil
		}
	}
}

func expandClosedUnionInstantiatedBody(inst *typ.Instantiated) typ.Type {
	if inst == nil || inst.Generic == nil || inst.Generic.Body == nil ||
		len(inst.Generic.TypeParams) != len(inst.TypeArgs) {
		return inst
	}
	params := inst.Generic.TypeParams
	args := inst.TypeArgs
	return typ.Rewrite(inst.Generic.Body, func(node typ.Type) (typ.Type, bool) {
		tp, ok := node.(*typ.TypeParam)
		if !ok {
			return nil, false
		}
		for i, param := range params {
			if param == nil || args[i] == nil {
				continue
			}
			if tp == param || tp.Equals(param) {
				return args[i], true
			}
		}
		return nil, false
	})
}

func isRefinableStructuralAnnotation(t typ.Type, guard recursion.Guard) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	switch v := t.(type) {
	case *typ.Alias:
		return isRefinableStructuralAnnotation(v.UnaliasedTarget(), next)
	case *typ.Optional:
		return annotationSlotRefinable(v.Inner, next)
	case *typ.Array:
		return annotationSlotRefinable(v.Element, next)
	case *typ.Map:
		return annotationSlotRefinable(v.Key, next) || annotationSlotRefinable(v.Value, next)
	case *typ.ReadonlyMap:
		return annotationSlotRefinable(v.Key, next) || annotationSlotRefinable(v.Value, next)
	case *typ.Record:
		if v.Open && len(v.Fields) == 0 && !v.HasMapComponent() {
			return true
		}
		if v.HasMapComponent() &&
			(annotationSlotRefinable(v.MapKey, next) || annotationSlotRefinable(v.MapValue, next)) {
			return true
		}
		for _, field := range v.Fields {
			if annotationSlotRefinable(field.Type, next) {
				return true
			}
		}
		return false
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if annotationSlotRefinable(elem, next) {
				return true
			}
		}
		return false
	case *typ.Union:
		for _, member := range v.Members {
			if annotationSlotRefinable(member, next) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func annotationSlotRefinable(t typ.Type, guard recursion.Guard) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	if t.Kind().IsPlaceholder() {
		return true
	}
	return isRefinableStructuralAnnotation(t, guard)
}
