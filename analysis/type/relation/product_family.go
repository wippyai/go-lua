package relation

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type precisionCompareFunc func(candidate, baseline typ.Type) (strict bool, comparable bool)

// productFamilyHash returns a stable structural family hash for product-domain
// relations that must compare recursive products coinductively without
// unfolding them by concrete node identity.
func productFamilyHash(t typ.Type) uint64 {
	return productFamilyHashWithCache(t, nil)
}

// productFamilyHashWithCache returns productFamilyHash while reusing caller-owned
// per-type hash cache entries. A nil cache is valid and disables caching.
func productFamilyHashWithCache(t typ.Type, cache map[typ.Type]uint64) uint64 {
	return productFamilyHashSeen(t, make(map[uintptr]bool), cache)
}

// sameProductFamily reports whether two recursive product observations describe
// the same fixed-point family under relation-owned structural family hashing.
// Callers that need equal-precision semantics should use
// sameProductFamilyWithPrecision.
func sameProductFamily(a, b typ.Type) bool {
	return sameProductFamilyWithPrecision(a, b, ComparePrecision)
}

// sameProductFamilyWithPrecision reports whether two recursive product
// observations describe the same fixed-point family with equal precision.
func sameProductFamilyWithPrecision(a, b typ.Type, compare precisionCompareFunc) bool {
	return sameProductFamilyWithCompare(a, b, compare, nil)
}

// sameProductFamilyWithPrecisionAndCache is sameProductFamilyWithPrecision using
// a caller-owned product-family hash cache.
func sameProductFamilyWithPrecisionAndCache(a, b typ.Type, compare precisionCompareFunc, cache map[typ.Type]uint64) bool {
	return sameProductFamilyWithCompare(a, b, compare, cache)
}

func sameProductFamilyWithCompare(a, b typ.Type, compare precisionCompareFunc, cache map[typ.Type]uint64) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !typ.ContainsRecursive(a) && !typ.ContainsRecursive(b) {
		return false
	}
	if productFamilyHashWithCache(a, cache) != productFamilyHashWithCache(b, cache) {
		return false
	}
	if compare == nil {
		return true
	}
	aStrict, aComparable := compare(a, b)
	if !aComparable || aStrict {
		return false
	}
	bStrict, bComparable := compare(b, a)
	return bComparable && !bStrict
}

func productFamilyHashSeen(t typ.Type, active map[uintptr]bool, cache map[typ.Type]uint64) (out uint64) {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return 0
	}
	if alias, ok := t.(*typ.Alias); ok {
		return productFamilyHashSeen(alias.UnaliasedTarget(), active, cache)
	}
	if rec, ok := t.(*typ.Recursive); ok {
		return hash.HashCombine(uint64(kind.Recursive), hash.FnvString(rec.Name))
	}
	if cache != nil {
		if cached, ok := cache[t]; ok {
			return cached
		}
		defer func() {
			cache[t] = out
		}()
	}

	ptr := nodeid.Pointer(t)
	if ptr != 0 {
		if active[ptr] {
			return hash.HashCombine(uint64(kind.Recursive), hash.FnvString("$cycle"))
		}
		active[ptr] = true
		defer delete(active, ptr)
	}

	switch v := t.(type) {
	case *typ.Optional:
		return hash.HashCombine(uint64(kind.Optional), productFamilyMemberHash(v.Inner, active, cache))
	case *typ.Union:
		h := hash.HashCombine(uint64(kind.Union), uint64(len(v.Members)))
		for _, member := range v.Members {
			h = hash.HashCombine(h, productFamilyMemberHash(member, active, cache))
		}
		return h
	case *typ.Intersection:
		h := hash.HashCombine(uint64(kind.Intersection), uint64(len(v.Members)))
		for _, member := range v.Members {
			h = hash.HashCombine(h, productFamilyMemberHash(member, active, cache))
		}
		return h
	case *typ.Record:
		h := hash.HashCombine(uint64(kind.Record), boolProductFamilyHash(v.Open))
		h = hash.HashCombine(h, boolProductFamilyHash(v.HasMapComponent()))
		if v.HasMapComponent() {
			h = hash.HashCombine(h, productFamilyMemberHash(v.MapKey, active, cache))
			h = hash.HashCombine(h, productFamilyMemberHash(v.MapValue, active, cache))
		}
		if v.Metatable != nil {
			h = hash.HashCombine(h, productFamilyMemberHash(v.Metatable, active, cache))
		}
		h = hash.HashCombine(h, uint64(len(v.Fields)))
		for _, field := range v.Fields {
			h = hash.HashCombine(h, hash.FnvString(field.Name))
			h = hash.HashCombine(h, boolProductFamilyHash(field.Optional))
			h = hash.HashCombine(h, boolProductFamilyHash(field.Readonly))
			h = hash.HashCombine(h, productFamilyTerminalHash(field.Type, cache))
		}
		h = hash.HashCombine(h, uint64(len(v.StaticMembers)))
		for _, member := range v.StaticMembers {
			h = hash.HashCombine(h, uint64(member.Kind))
			h = hash.HashCombine(h, hash.FnvString(member.Name))
			h = hash.HashCombine(h, uint64(member.Index))
			h = hash.HashCombine(h, boolProductFamilyHash(member.Optional))
			h = hash.HashCombine(h, boolProductFamilyHash(member.Readonly))
			h = hash.HashCombine(h, productFamilyTerminalHash(member.Type, cache))
		}
		return h
	case *typ.Array:
		return hash.HashCombine(uint64(kind.Array), productFamilyMemberHash(v.Element, active, cache))
	case *typ.Map:
		h := hash.HashCombine(uint64(kind.Map), productFamilyMemberHash(v.Key, active, cache))
		return hash.HashCombine(h, productFamilyMemberHash(v.Value, active, cache))
	case *typ.ReadonlyMap:
		h := hash.HashCombine(uint64(kind.ReadonlyMap), productFamilyMemberHash(v.Key, active, cache))
		return hash.HashCombine(h, productFamilyMemberHash(v.Value, active, cache))
	case *typ.Tuple:
		h := hash.HashCombine(uint64(kind.Tuple), uint64(len(v.Elements)))
		for _, elem := range v.Elements {
			h = hash.HashCombine(h, productFamilyMemberHash(elem, active, cache))
		}
		return h
	case *typ.Function:
		h := hash.HashCombine(uint64(kind.Function), uint64(len(v.TypeParams)))
		for _, param := range v.TypeParams {
			h = hash.HashCombine(h, productFamilyMemberHash(param, active, cache))
		}
		h = hash.HashCombine(h, uint64(len(v.Params)))
		for _, param := range v.Params {
			h = hash.HashCombine(h, boolProductFamilyHash(param.Optional))
			h = hash.HashCombine(h, productFamilyMemberHash(param.Type, active, cache))
		}
		if v.Variadic != nil {
			h = hash.HashCombine(h, 1)
			h = hash.HashCombine(h, productFamilyMemberHash(v.Variadic, active, cache))
		}
		h = hash.HashCombine(h, uint64(len(v.Returns)))
		for _, ret := range v.Returns {
			h = hash.HashCombine(h, productFamilyMemberHash(ret, active, cache))
		}
		return h
	case *typ.Instantiated:
		h := hash.HashCombine(uint64(kind.Instantiated), productFamilyMemberHash(v.Generic, active, cache))
		for _, arg := range v.TypeArgs {
			h = hash.HashCombine(h, productFamilyMemberHash(arg, active, cache))
		}
		return h
	case *typ.Generic:
		h := hash.HashCombine(uint64(kind.Generic), hash.FnvString(v.Name))
		for _, param := range v.TypeParams {
			h = hash.HashCombine(h, productFamilyMemberHash(param, active, cache))
		}
		if v.Name == "" && v.Body != nil {
			h = hash.HashCombine(h, productFamilyMemberHash(v.Body, active, cache))
		}
		return h
	case *typ.TypeParam:
		h := hash.HashCombine(uint64(kind.TypeParam), hash.FnvString(v.Name))
		if v.Constraint != nil {
			h = hash.HashCombine(h, productFamilyMemberHash(v.Constraint, active, cache))
		}
		return h
	case *typ.Meta:
		return hash.HashCombine(uint64(kind.Meta), productFamilyMemberHash(v.Of, active, cache))
	case *typ.Interface:
		h := hash.HashCombine(uint64(kind.Interface), hash.FnvString(v.Name))
		for _, method := range v.Methods {
			h = hash.HashCombine(h, hash.FnvString(method.Name))
			h = hash.HashCombine(h, productFamilyMemberHash(method.Type, active, cache))
		}
		return h
	default:
		return t.Hash()
	}
}

func productFamilyMemberHash(t typ.Type, active map[uintptr]bool, cache map[typ.Type]uint64) (out uint64) {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return 0
	}
	if alias, ok := t.(*typ.Alias); ok {
		return productFamilyMemberHash(alias.UnaliasedTarget(), active, cache)
	}
	if rec, ok := t.(*typ.Recursive); ok {
		return hash.HashCombine(uint64(kind.Recursive), hash.FnvString(rec.Name))
	}
	if cache != nil {
		if cached, ok := cache[t]; ok {
			return cached
		}
		defer func() {
			cache[t] = out
		}()
	}
	if !typ.ContainsRecursive(t) {
		return typ.EqualityHash(t)
	}
	return productFamilyTerminalHash(t, cache)
}

func productFamilyTerminalHash(t typ.Type, cache map[typ.Type]uint64) (out uint64) {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return 0
	}
	if alias, ok := t.(*typ.Alias); ok {
		return productFamilyTerminalHash(alias.UnaliasedTarget(), cache)
	}
	if rec, ok := t.(*typ.Recursive); ok {
		return hash.HashCombine(uint64(kind.Recursive), hash.FnvString(rec.Name))
	}
	if cache != nil {
		if cached, ok := cache[t]; ok {
			return cached
		}
		defer func() {
			cache[t] = out
		}()
	}
	if !typ.ContainsRecursive(t) {
		return typ.EqualityHash(t)
	}
	return hash.HashCombine(uint64(t.Kind()), hash.FnvString("$recursive-family"))
}

func normalizeNilType(t typ.Type) typ.Type {
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

func boolProductFamilyHash(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}
