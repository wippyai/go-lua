package refinement

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/presence"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// IsRefinableAnnotation reports whether an explicit annotation should be
// treated as a soft placeholder that call-site/contextual hints may refine.
//
// Canonical rule: explicit top types (`any`, `unknown`) are authoritative and
// must not be rewritten by hints. Structural soft placeholders like `{any}` or
// `any[]` remain refinable.
func IsRefinableAnnotation(t Type) bool {
	if t == nil {
		return false
	}
	if t.Kind().IsPlaceholder() {
		return false
	}
	return isRefinableStructuralAnnotation(t, NewGuard())
}

// IsClosedUnionAnnotation reports whether a declared annotation is a multi-member
// union whose members are all concrete (no placeholder/any/unknown at the top
// level of any member). Such an annotation carries a closed discriminant domain
// that flow narrowing must preserve at the variable's root even if the union
// has refinable slots deep inside member fields.
func IsClosedUnionAnnotation(t Type) bool {
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
		if presence.AbsentOrUnknown(member) || member.Kind().IsPlaceholder() {
			return false
		}
	}
	return true
}

// PruneLessPreciseRefinableUnionMembers removes refinable structural
// placeholder members from a union when another member carries comparable,
// strictly more precise evidence for the same runtime shape.
func PruneLessPreciseRefinableUnionMembers(t Type, morePrecise MorePreciseFunc, normalizeUnion func(...Type) Type) Type {
	u, ok := t.(*Union)
	if !ok || len(u.Members) < 2 || morePrecise == nil {
		return t
	}
	keep := make([]Type, 0, len(u.Members))
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
	return NewUnion(keep...)
}

// closedUnionOf unwraps Annotated/Alias/Optional layers to expose an underlying
// Union, mirroring the unwrap.Union helper without the import.
func closedUnionOf(t Type) *Union {
	for {
		switch v := UnwrapAnnotated(t).(type) {
		case *Union:
			return v
		case *Alias:
			t = v.Target
		case *Optional:
			t = v.Inner
		case *Instantiated:
			t = expandClosedUnionInstantiatedBody(v)
		default:
			return nil
		}
	}
}

func expandClosedUnionInstantiatedBody(inst *Instantiated) Type {
	if inst == nil || inst.Generic == nil || inst.Generic.Body == nil ||
		len(inst.Generic.TypeParams) != len(inst.TypeArgs) {
		return inst
	}
	params := inst.Generic.TypeParams
	args := inst.TypeArgs
	return Rewrite(inst.Generic.Body, func(node Type) (Type, bool) {
		tp, ok := node.(*TypeParam)
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

func isRefinableStructuralAnnotation(t Type, guard recursion.Guard) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	switch v := t.(type) {
	case *Alias:
		return isRefinableStructuralAnnotation(v.UnaliasedTarget(), next)
	case *Optional:
		return annotationSlotRefinable(v.Inner, next)
	case *Array:
		return annotationSlotRefinable(v.Element, next)
	case *Map:
		return annotationSlotRefinable(v.Key, next) || annotationSlotRefinable(v.Value, next)
	case *ReadonlyMap:
		return annotationSlotRefinable(v.Key, next) || annotationSlotRefinable(v.Value, next)
	case *Record:
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
	case *Tuple:
		for _, elem := range v.Elements {
			if annotationSlotRefinable(elem, next) {
				return true
			}
		}
		return false
	case *Union:
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

func annotationSlotRefinable(t Type, guard recursion.Guard) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t.Kind().IsPlaceholder() {
		return true
	}
	return isRefinableStructuralAnnotation(t, guard)
}
