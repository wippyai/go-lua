package value

import (
	"reflect"
	"unsafe"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FactTypeEqual compares stored fact types, including function metadata.
//
// typ.TypeEquals intentionally compares function call shapes and ignores
// effects, specs, and refinements. Interprocedural fact equality needs the
// stronger relation because those fields are part of the abstract value stored
// in a fact slot.
func FactTypeEqual(a, b typ.Type) bool {
	if !typ.TypeEquals(a, b) {
		return false
	}
	return factTypeMetadataEqual(a, b, nil)
}

type factTypePair struct {
	a uintptr
	b uintptr
}

func factTypeMetadataEqual(a, b typ.Type, seen map[factTypePair]bool) bool {
	a = unwrapFactTransparent(a)
	b = unwrapFactTransparent(b)
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind() != b.Kind() {
		return false
	}
	if needsFactTypeCycleCheck(a.Kind()) {
		ap := factTypePointer(a)
		bp := factTypePointer(b)
		if ap != 0 && bp != 0 {
			pair := factTypePair{a: ap, b: bp}
			if seen == nil {
				seen = make(map[factTypePair]bool)
			}
			if seen[pair] {
				return true
			}
			seen[pair] = true
		}
	}

	switch left := a.(type) {
	case *typ.Optional:
		right, ok := b.(*typ.Optional)
		return ok && factTypeMetadataEqual(left.Inner, right.Inner, seen)
	case *typ.Union:
		right, ok := b.(*typ.Union)
		if !ok || len(left.Members) != len(right.Members) {
			return false
		}
		for i, member := range left.Members {
			if !factTypeMetadataEqual(member, right.Members[i], seen) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		right, ok := b.(*typ.Intersection)
		if !ok || len(left.Members) != len(right.Members) {
			return false
		}
		for i, member := range left.Members {
			if !factTypeMetadataEqual(member, right.Members[i], seen) {
				return false
			}
		}
		return true
	case *typ.Tuple:
		right, ok := b.(*typ.Tuple)
		if !ok || len(left.Elements) != len(right.Elements) {
			return false
		}
		for i, elem := range left.Elements {
			if !factTypeMetadataEqual(elem, right.Elements[i], seen) {
				return false
			}
		}
		return true
	case *typ.Array:
		right, ok := b.(*typ.Array)
		return ok && factTypeMetadataEqual(left.Element, right.Element, seen)
	case *typ.Map:
		right, ok := b.(*typ.Map)
		return ok &&
			factTypeMetadataEqual(left.Key, right.Key, seen) &&
			factTypeMetadataEqual(left.Value, right.Value, seen)
	case *typ.Record:
		right, ok := b.(*typ.Record)
		if !ok || left.Open != right.Open || len(left.Fields) != len(right.Fields) {
			return false
		}
		for i, field := range left.Fields {
			other := right.Fields[i]
			if field.Name != other.Name || field.Optional != other.Optional || field.Readonly != other.Readonly {
				return false
			}
			if !factTypeMetadataEqual(field.Type, other.Type, seen) {
				return false
			}
		}
		if left.HasMapComponent() != right.HasMapComponent() {
			return false
		}
		if left.HasMapComponent() {
			return factTypeMetadataEqual(left.MapKey, right.MapKey, seen) &&
				factTypeMetadataEqual(left.MapValue, right.MapValue, seen)
		}
		return true
	case *typ.Function:
		right, ok := b.(*typ.Function)
		return ok && factFunctionEqual(left, right, seen)
	case *typ.Generic:
		right, ok := b.(*typ.Generic)
		if !ok || left.Name != right.Name || len(left.TypeParams) != len(right.TypeParams) {
			return false
		}
		for i, tp := range left.TypeParams {
			if !factTypeParamEqual(tp, right.TypeParams[i], seen) {
				return false
			}
		}
		if left.Name != "" {
			return true
		}
		return factTypeMetadataEqual(left.Body, right.Body, seen)
	case *typ.Instantiated:
		right, ok := b.(*typ.Instantiated)
		if !ok || len(left.TypeArgs) != len(right.TypeArgs) {
			return false
		}
		if !factTypeMetadataEqual(left.Generic, right.Generic, seen) {
			return false
		}
		for i, arg := range left.TypeArgs {
			if !factTypeMetadataEqual(arg, right.TypeArgs[i], seen) {
				return false
			}
		}
		return true
	case *typ.Recursive:
		right, ok := b.(*typ.Recursive)
		if !ok || left.Name != right.Name {
			return false
		}
		if left.ID == right.ID {
			return true
		}
		return factTypeMetadataEqual(left.Body, right.Body, seen)
	case *typ.Sum:
		right, ok := b.(*typ.Sum)
		if !ok || left.Name != right.Name || len(left.Variants) != len(right.Variants) {
			return false
		}
		for i, variant := range left.Variants {
			other := right.Variants[i]
			if variant.Tag != other.Tag || len(variant.Types) != len(other.Types) {
				return false
			}
			for j, t := range variant.Types {
				if !factTypeMetadataEqual(t, other.Types[j], seen) {
					return false
				}
			}
		}
		return true
	case *typ.Interface:
		right, ok := b.(*typ.Interface)
		if !ok || left.Name != right.Name || len(left.Methods) != len(right.Methods) {
			return false
		}
		for i, method := range left.Methods {
			other := right.Methods[i]
			if method.Name != other.Name || !factFunctionEqual(method.Type, other.Type, seen) {
				return false
			}
		}
		return true
	case *typ.FieldAccess:
		right, ok := b.(*typ.FieldAccess)
		return ok && left.Field == right.Field &&
			factTypeMetadataEqual(left.Base, right.Base, seen)
	case *typ.IndexAccess:
		right, ok := b.(*typ.IndexAccess)
		return ok &&
			factTypeMetadataEqual(left.Base, right.Base, seen) &&
			factTypeMetadataEqual(left.Index, right.Index, seen)
	case *typ.Meta:
		right, ok := b.(*typ.Meta)
		return ok && factTypeMetadataEqual(left.Of, right.Of, seen)
	default:
		return true
	}
}

func unwrapFactTransparent(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = unwrap.Alias(t)
		annotated, ok := t.(*typ.Annotated)
		if !ok {
			return t
		}
		if annotated.Inner == nil || annotated.Inner == t {
			return annotated.Inner
		}
		t = annotated.Inner
	}
	return nil
}

func factFunctionEqual(left, right *typ.Function, seen map[factTypePair]bool) bool {
	if left == nil || right == nil {
		return left == right
	}
	if !effectInfoEqual(left.Effects, right.Effects) ||
		!specInfoEqual(left.Spec, right.Spec) ||
		!refinementInfoEqual(left.Refinement, right.Refinement) {
		return false
	}
	if len(left.TypeParams) != len(right.TypeParams) ||
		len(left.Params) != len(right.Params) ||
		len(left.Returns) != len(right.Returns) {
		return false
	}
	for i, tp := range left.TypeParams {
		if !factTypeParamEqual(tp, right.TypeParams[i], seen) {
			return false
		}
	}
	for i, param := range left.Params {
		other := right.Params[i]
		if param.Optional != other.Optional {
			return false
		}
		if !factTypeMetadataEqual(param.Type, other.Type, seen) {
			return false
		}
	}
	if (left.Variadic == nil) != (right.Variadic == nil) {
		return false
	}
	if left.Variadic != nil && !factTypeMetadataEqual(left.Variadic, right.Variadic, seen) {
		return false
	}
	for i, ret := range left.Returns {
		if !factTypeMetadataEqual(ret, right.Returns[i], seen) {
			return false
		}
	}
	return true
}

func factTypeParamEqual(left, right *typ.TypeParam, seen map[factTypePair]bool) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Name == right.Name &&
		factTypeMetadataEqual(left.Constraint, right.Constraint, seen)
}

func effectInfoEqual(left, right typ.EffectInfo) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equals(right)
}

func specInfoEqual(left, right typ.SpecInfo) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equals(right)
}

func refinementInfoEqual(left, right typ.RefinementInfo) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equals(right)
}

func needsFactTypeCycleCheck(k kind.Kind) bool {
	switch k {
	case kind.Union, kind.Intersection, kind.Record, kind.Function,
		kind.Generic, kind.Instantiated, kind.Interface, kind.Recursive,
		kind.Sum:
		return true
	default:
		return false
	}
}

func factTypePointer(t typ.Type) uintptr {
	switch tt := t.(type) {
	case *typ.Union:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Intersection:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Record:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Function:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Generic:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Instantiated:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Interface:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Recursive:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Sum:
		return uintptr(unsafe.Pointer(tt))
	}
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}
	return v.Pointer()
}
