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
//
// A recursive product family is compared by canonical-family identity: both
// operands hash-cons to one canonical representative through the metadata-
// sensitive verifier (CanonicalRecursiveFamily, which uses factTypeMetadataEqual),
// so the same family is the same node pointer. This makes fact-slot equality the
// kernel of the family hash by construction, so the inter-procedural fixpoint
// detects a fixed point on recursive summaries instead of re-deriving a hash-equal
// but Equal-distinct representative forever.
func FactTypeEqual(a, b typ.Type) bool {
	a = canonicalFactType(a)
	b = canonicalFactType(b)
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return a == b
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		if canonicalizableRecursiveFamily(a) && canonicalizableRecursiveFamily(b) {
			return CanonicalRecursiveFamily(a) == CanonicalRecursiveFamily(b)
		}
		// One side is an unsealed placeholder or a recursive family wrapped in a
		// non-canonicalizable carrier (any/unknown). The canonical-family path does
		// not apply; the metadata-sensitive verifier (which treats two nil-body
		// recursives as equal) still answers exactly.
		return factTypeMetadataEqual(a, b, nil)
	}
	if !typ.TypeEquals(a, b) {
		return false
	}
	return factTypeMetadataEqual(a, b, nil)
}

type factTypePair struct {
	a uintptr
	b uintptr
}

// factFamilyPair keys the coinductive bisimulation hypothesis on the structural
// product-family fingerprint of each side, so the verifier ties off on a
// recursive back-edge even when the two observations are distinct freshly-built
// nodes (distinct pointers each fixpoint iteration). Pointer-keyed cycle guards
// alone cannot close that case: two unfoldings of one family carry no shared
// pointer, so without the family guard the verifier would descend forever down
// the growing structure and report distinct families that are coinductively
// equal.
type factFamilyPair struct {
	a uint64
	b uint64
}

func factTypeMetadataEqual(a, b typ.Type, seen map[factTypePair]bool) bool {
	return factTypeMetadataEqualGuarded(a, b, seen, nil)
}

func factTypeMetadataEqualGuarded(a, b typ.Type, seen map[factTypePair]bool, seenFamily map[factFamilyPair]bool) bool {
	a = canonicalFactType(a)
	b = canonicalFactType(b)
	a = unwrapFactTransparent(a)
	b = unwrapFactTransparent(b)
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind() != b.Kind() {
		return false
	}
	if typ.ContainsRecursive(a) && typ.ContainsRecursive(b) {
		pair := factFamilyPair{a: typ.ProductFamilyHash(a), b: typ.ProductFamilyHash(b)}
		if seenFamily == nil {
			seenFamily = make(map[factFamilyPair]bool)
		}
		if seenFamily[pair] {
			return true
		}
		seenFamily[pair] = true
		defer delete(seenFamily, pair)
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
	same := func(x, y typ.Type) bool {
		return factTypeMetadataEqualGuarded(x, y, seen, seenFamily)
	}

	switch left := a.(type) {
	case *typ.Optional:
		right, ok := b.(*typ.Optional)
		return ok && same(left.Inner, right.Inner)
	case *typ.Union:
		right, ok := b.(*typ.Union)
		if !ok || len(left.Members) != len(right.Members) {
			return false
		}
		for i, member := range left.Members {
			if !same(member, right.Members[i]) {
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
			if !same(member, right.Members[i]) {
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
			if !same(elem, right.Elements[i]) {
				return false
			}
		}
		return true
	case *typ.Array:
		right, ok := b.(*typ.Array)
		return ok && same(left.Element, right.Element)
	case *typ.Map:
		right, ok := b.(*typ.Map)
		return ok &&
			same(left.Key, right.Key) &&
			same(left.Value, right.Value)
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
			if !same(field.Type, other.Type) {
				return false
			}
		}
		if left.HasMapComponent() != right.HasMapComponent() {
			return false
		}
		if left.HasMapComponent() {
			if !same(left.MapKey, right.MapKey) ||
				!same(left.MapValue, right.MapValue) {
				return false
			}
		}
		return same(left.Metatable, right.Metatable)
	case *typ.Function:
		right, ok := b.(*typ.Function)
		return ok && factFunctionEqual(left, right, seen, seenFamily)
	case *typ.Generic:
		right, ok := b.(*typ.Generic)
		if !ok || left.Name != right.Name || len(left.TypeParams) != len(right.TypeParams) {
			return false
		}
		for i, tp := range left.TypeParams {
			if !factTypeParamEqual(tp, right.TypeParams[i], seen, seenFamily) {
				return false
			}
		}
		if left.Name != "" {
			return true
		}
		return same(left.Body, right.Body)
	case *typ.Instantiated:
		right, ok := b.(*typ.Instantiated)
		if !ok || len(left.TypeArgs) != len(right.TypeArgs) {
			return false
		}
		if !same(left.Generic, right.Generic) {
			return false
		}
		for i, arg := range left.TypeArgs {
			if !same(arg, right.TypeArgs[i]) {
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
		return same(left.Body, right.Body)
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
				if !same(t, other.Types[j]) {
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
			if method.Name != other.Name || !factFunctionEqual(method.Type, other.Type, seen, seenFamily) {
				return false
			}
		}
		return true
	case *typ.FieldAccess:
		right, ok := b.(*typ.FieldAccess)
		return ok && left.Field == right.Field &&
			same(left.Base, right.Base)
	case *typ.IndexAccess:
		right, ok := b.(*typ.IndexAccess)
		return ok &&
			same(left.Base, right.Base) &&
			same(left.Index, right.Index)
	case *typ.Meta:
		right, ok := b.(*typ.Meta)
		return ok && same(left.Of, right.Of)
	default:
		return typ.TypeEquals(a, b)
	}
}

func unwrapFactTransparent(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = canonicalFactType(t)
		if t == nil {
			return nil
		}
		t = unwrap.Alias(t)
		t = canonicalFactType(t)
		if t == nil {
			return nil
		}
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

func canonicalFactType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	v := reflect.ValueOf(t)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	return t
}

func factFunctionEqual(left, right *typ.Function, seen map[factTypePair]bool, seenFamily map[factFamilyPair]bool) bool {
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
	// Recursive functions are compared field-by-field below, never short-circuited
	// by SameProductFamily. SameProductFamily is the precision relation, which
	// treats a recursive family reached as a member by name only; two distinct
	// same-named families (a Store class per module) carry the same member-level
	// fingerprint, so the shortcut would judge fun()->StoreA equal to fun()->StoreB
	// and let the value interner collapse them. The structural descent plus the
	// coinductive family/pointer guards terminate on genuine same-family unfoldings
	// while keeping distinct families apart.
	same := func(x, y typ.Type) bool {
		return factTypeMetadataEqualGuarded(x, y, seen, seenFamily)
	}
	for i, tp := range left.TypeParams {
		if !factTypeParamEqual(tp, right.TypeParams[i], seen, seenFamily) {
			return false
		}
	}
	for i, param := range left.Params {
		other := right.Params[i]
		if param.Optional != other.Optional {
			return false
		}
		if !same(param.Type, other.Type) {
			return false
		}
	}
	if (left.Variadic == nil) != (right.Variadic == nil) {
		return false
	}
	if left.Variadic != nil && !same(left.Variadic, right.Variadic) {
		return false
	}
	for i, ret := range left.Returns {
		if !same(ret, right.Returns[i]) {
			return false
		}
	}
	return true
}

func factTypeParamEqual(left, right *typ.TypeParam, seen map[factTypePair]bool, seenFamily map[factFamilyPair]bool) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Name == right.Name &&
		factTypeMetadataEqualGuarded(left.Constraint, right.Constraint, seen, seenFamily)
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
