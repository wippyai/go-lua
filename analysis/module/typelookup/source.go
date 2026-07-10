// Package typelookup exposes exact module type-definition metadata for
// annotation rehydration across module boundaries.
package typelookup

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Source is a narrow read view over named type definitions exported by module
// manifests. It intentionally does not read runtime module export values.
type Source struct {
	Manifests []*manifest.Manifest
}

// ResolveTypeRef resolves a qualified type path such as protocol.User against
// exact module manifest paths.
func (s Source) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) == 1 {
		return s.lookupUnqualified(path[0])
	}
	if len(path) < 2 {
		return nil, false
	}
	modulePath := strings.Join(path[:len(path)-1], ".")
	return s.Lookup(modulePath, path[len(path)-1])
}

// ResolveTypeRefWithModulePrefix resolves suffix under an already canonical
// module path prefix. It lets import-alias resolvers delegate qualified lookup
// without rebuilding a synthetic full path only for Source to split it again.
func (s Source) ResolveTypeRefWithModulePrefix(modulePrefix string, suffix []string) (typ.Type, bool) {
	if modulePrefix == "" || len(suffix) == 0 {
		return nil, false
	}
	if len(suffix) == 1 {
		return s.Lookup(modulePrefix, suffix[0])
	}
	modulePath := modulePrefixWithSuffix(modulePrefix, suffix[:len(suffix)-1])
	return s.Lookup(modulePath, suffix[len(suffix)-1])
}

// Lookup returns the named type exported by an exact module path. Later
// manifests override earlier ones, matching signature and export lookup order.
func (s Source) Lookup(modulePath, name string) (typ.Type, bool) {
	if modulePath == "" || name == "" {
		return nil, false
	}
	for i := len(s.Manifests) - 1; i >= 0; i-- {
		m := s.Manifests[i]
		if m == nil || m.Path != modulePath || m.Types == nil {
			continue
		}
		t := m.Types[name]
		if t == nil {
			continue
		}
		return m.ScopeType(t), true
	}
	return nil, false
}

func (s Source) lookupUnqualified(name string) (typ.Type, bool) {
	if name == "" {
		return nil, false
	}
	var out typ.Type
	found := false
	for i := len(s.Manifests) - 1; i >= 0; i-- {
		m := s.Manifests[i]
		if m == nil || m.Types == nil {
			continue
		}
		t := m.Types[name]
		if t == nil {
			continue
		}
		if found {
			return nil, false
		}
		out = m.ScopeType(t)
		found = true
	}
	return out, found
}

func modulePrefixWithSuffix(prefix string, suffix []string) string {
	if len(suffix) == 0 {
		return prefix
	}
	var b strings.Builder
	b.Grow(len(prefix) + 1 + len(suffix)*8)
	b.WriteString(prefix)
	for _, part := range suffix {
		b.WriteByte('.')
		b.WriteString(part)
	}
	return b.String()
}
