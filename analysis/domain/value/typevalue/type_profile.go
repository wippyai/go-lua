package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RuntimeTypeProfile summarizes type-derived facts about a product value
// without exposing the underlying type object to engine packages.
type RuntimeTypeProfile struct {
	TopLevelGradual bool
	ContainsGradual bool
	RuntimeKind     runtimekind.Value
	HasRuntimeKind  bool
}

// RuntimeTypeProfileOf reconstructs the value's type and reports the gradual
// and runtime-kind properties needed by engine-level source reconciliation.
func RuntimeTypeProfileOf(reg *axis.Registry, cache *Cache, value product.Value) (RuntimeTypeProfile, bool) {
	if cache != nil {
		return cache.runtimeTypeProfileOf(reg, value)
	}
	t, ok := TypeOf(reg, value)
	if !ok || t == nil {
		return RuntimeTypeProfile{}, false
	}
	return runtimeTypeProfileForType(t, nil), true
}

func (c *Cache) runtimeTypeProfileOf(reg *axis.Registry, value product.Value) (RuntimeTypeProfile, bool) {
	if c == nil {
		return RuntimeTypeProfileOf(reg, nil, value)
	}
	key := typeProfileCacheKey{reg: reg, value: value}
	if cached, ok := c.typeProfiles[key]; ok {
		return cached.profile, cached.ok
	}
	t, ok := c.TypeOf(reg, value)
	if !ok || t == nil {
		c.rememberTypeProfile(key, RuntimeTypeProfile{}, false)
		return RuntimeTypeProfile{}, false
	}
	profile := runtimeTypeProfileForType(t, c)
	c.rememberTypeProfile(key, profile, true)
	return profile, true
}

func (c *Cache) rememberTypeProfile(key typeProfileCacheKey, profile RuntimeTypeProfile, ok bool) {
	if c == nil {
		return
	}
	if c.typeProfiles == nil {
		c.typeProfiles = make(map[typeProfileCacheKey]cachedTypeProfile)
	}
	c.typeProfiles[key] = cachedTypeProfile{profile: profile, ok: ok}
}

func runtimeTypeProfileForType(t typ.Type, cache *Cache) RuntimeTypeProfile {
	kinds, kindsOK := RuntimeKindFromType(t)
	return RuntimeTypeProfile{
		TopLevelGradual: typ.IsAny(t) || typ.IsUnknown(t),
		ContainsGradual: typ.ContainsAny(t) || containsUnknownTypeCached(cache, t),
		RuntimeKind:     kinds,
		HasRuntimeKind:  kindsOK,
	}
}

func containsUnknownTypeCached(cache *Cache, t typ.Type) bool {
	if cache == nil || t == nil {
		result, _ := containsUnknownTypeScan(t)
		return result
	}
	if cached, ok := cache.unknownTypes[t]; ok && !cached.open {
		return cached.value
	}
	result, open := containsUnknownTypeScan(t)
	if !open {
		if cache.unknownTypes == nil {
			cache.unknownTypes = make(map[typ.Type]cachedContainsUnknown)
		}
		cache.unknownTypes[t] = cachedContainsUnknown{value: result}
	}
	return result
}

func containsUnknownTypeScan(t typ.Type) (bool, bool) {
	var scan unknownTypeScan
	return scan.contains(t), scan.open
}

type unknownTypeScan struct {
	seen map[typ.Type]struct{}
	open bool
}

func (s *unknownTypeScan) contains(t typ.Type) bool {
	if s == nil {
		return false
	}
	t = typ.UnwrapTransparentWrappers(t)
	if t == nil {
		return false
	}
	if typ.IsUnknown(t) {
		return true
	}
	if rec, ok := t.(*typ.Recursive); ok && rec.Body == nil {
		s.open = true
		return false
	}
	if s.seen == nil {
		s.seen = make(map[typ.Type]struct{})
	}
	if _, ok := s.seen[t]; ok {
		return false
	}
	s.seen[t] = struct{}{}
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return s.contains(child)
	})
}
