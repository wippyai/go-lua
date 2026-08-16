// Package importlookup exposes exact module export metadata for import/require
// rehydration.
package importlookup

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

// Source is a narrow read view over module export types. It intentionally
// performs exact path lookup only.
type Source struct {
	Manifests []*manifest.Manifest
}

// LookupExport returns the export type for an exact module path.
func (s Source) LookupExport(path string) (typ.Type, bool) {
	if path == "" {
		return nil, false
	}
	for i := len(s.Manifests) - 1; i >= 0; i-- {
		m := s.Manifests[i]
		if m == nil || m.Path != path || m.Export == nil {
			continue
		}
		return m.ScopeType(m.Export), true
	}
	return nil, false
}
