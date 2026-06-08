package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// SymbolProductEnv builds the ProductDomain environment for reducing
// constraints against one symbol-rooted product value.
func SymbolProductEnv(
	sym cfg.SymbolID,
	base product.AbstractValue,
	facts PointFacts,
	resolver narrow.Resolver,
) (constraint.Env, constraint.PathKey) {
	rootKey := SymbolPathKey(sym, nil)
	env := constraint.Env{
		Resolver: resolver,
		ResolvePath: func(path constraint.Path) constraint.PathKey {
			return StablePathKey(path)
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			addr, ok := StableAddressFromCanonicalKey(key)
			if !ok {
				return nil
			}
			keySym, ok := addr.Symbol()
			if !ok || keySym != sym {
				return nil
			}
			segments := addr.Segments()
			if len(segments) == 0 {
				return product.ProjectValueOrUnknown(base)
			}
			if t, ok := ProductMemberPathType(base, segments); ok {
				return t
			}
			if t, ok := facts.PathType(constraint.Path{Symbol: sym, Segments: segments}); ok {
				return t
			}
			return nil
		},
	}
	return env, rootKey
}

// ProductMemberPathType projects the type at a structured member/index suffix
// under base.
func ProductMemberPathType(base product.AbstractValue, segments []constraint.Segment) (typ.Type, bool) {
	value, ok := ProductMemberPathValue(base, segments)
	if !ok || value.IsZero() {
		return nil, false
	}
	t := product.ProjectValueOrUnknown(value)
	return t, !typ.IsAbsentOrUnknown(t)
}

// ProductMemberPathValue projects the product value at a structured
// member/index suffix under base.
func ProductMemberPathValue(base product.AbstractValue, segments []constraint.Segment) (product.AbstractValue, bool) {
	if base.IsZero() {
		return product.AbstractValue{}, false
	}
	cur := base
	for _, seg := range segments {
		member, ok := value.MemberFromSegment(seg)
		if !ok {
			return product.AbstractValue{}, false
		}
		next, ok := product.MemberOf(cur, member)
		if !ok || next.IsZero() {
			return product.AbstractValue{}, false
		}
		cur = next
	}
	return cur, true
}

// ProductWithMemberPath overlays value at a structured member/index suffix
// under base, rebuilding enclosing records as needed.
func ProductWithMemberPath(base product.AbstractValue, segments []constraint.Segment, val product.AbstractValue) product.AbstractValue {
	if len(segments) == 0 {
		return base
	}
	member, ok := value.MemberFromSegment(segments[0])
	if !ok {
		return base
	}
	if len(segments) == 1 {
		return product.WithMember(base, member, val)
	}
	child, ok := product.MemberOf(base, member)
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	}
	updated := ProductWithMemberPath(child, segments[1:], val)
	if updated.IsZero() {
		return base
	}
	return product.WithMember(base, member, updated)
}

// ProductWithOnlyMemberPath builds a minimal product value whose only known
// member suffix is segments and whose leaf value is val.
func ProductWithOnlyMemberPath(segments []constraint.Segment, val product.AbstractValue) product.AbstractValue {
	if len(segments) == 0 {
		return val
	}
	if valueIsBottom(val) {
		return product.Domain.Bottom()
	}
	member, ok := value.MemberFromSegment(segments[0])
	if !ok {
		return product.Domain.Bottom()
	}
	child := ProductWithOnlyMemberPath(segments[1:], val)
	if valueIsBottom(child) {
		return product.Domain.Bottom()
	}
	return product.WithMember(product.FromType(typ.NewRecord().Build()), member, child)
}

// ProductDomainHasNarrowingForSymbol reports whether domain has any narrowed
// fact rooted at sym.
func ProductDomainHasNarrowingForSymbol(domain *ProductDomain, sym cfg.SymbolID) bool {
	if domain == nil || sym == 0 {
		return false
	}
	for key := range domain.Type.Narrowed {
		if StableAddressKeyHasSymbol(key, sym) {
			return true
		}
	}
	for key := range domain.Shape.Narrowed {
		if StableAddressKeyHasSymbol(key, sym) {
			return true
		}
	}
	return false
}

// StableAddressKeyHasSymbol reports whether key denotes a path rooted at sym.
func StableAddressKeyHasSymbol(key constraint.PathKey, sym cfg.SymbolID) bool {
	root, ok := StableAddressOfSymbol(sym, nil)
	if !ok {
		return false
	}
	return StableAddressKeyHasPrefix(key, root)
}
