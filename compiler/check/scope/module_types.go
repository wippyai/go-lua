package scope

import (
	"sort"

	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// WithModuleTypes returns s extended with the types that the given module
// manifests define, bound under their unqualified names. This is the type
// namespace an embedder exposes for its builtin modules, so Channel<T> and
// channel.Channel<T> both name the channel module's type.
//
// A name stays unbound when modules define it with different types; code
// then names it module-qualified. Names already bound in s keep their
// binding.
func (s *State) WithModuleTypes(manifests []*io.Manifest) *State {
	if s == nil {
		s = New()
	}
	sorted := make([]*io.Manifest, 0, len(manifests))
	for _, m := range manifests {
		if m != nil {
			sorted = append(sorted, m)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	bound := make(map[string]typ.Type)
	conflicting := make(map[string]bool)
	for _, m := range sorted {
		types := m.AllTypes()
		names := make([]string, 0, len(types))
		for name := range types {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			t, _ := m.LookupType(name)
			if prev, ok := bound[name]; ok && prev != t && !typ.TypeEquals(prev, t) {
				conflicting[name] = true
				continue
			}
			if _, ok := bound[name]; !ok {
				bound[name] = t
			}
		}
	}

	names := make([]string, 0, len(bound))
	for name := range bound {
		names = append(names, name)
	}
	sort.Strings(names)
	out := s
	for _, name := range names {
		if conflicting[name] {
			continue
		}
		if _, ok := out.LookupType(name); ok {
			continue
		}
		out = out.WithType(name, bound[name])
	}
	return out
}
