package manifest

import (
	"sort"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// TypeDeclaration is one provider-owned named type projected into the
// catalogue's exact qualified namespace. Type is already scoped through its
// defining manifest, so consumers do not need to reconstruct module paths or
// rewrite references themselves.
type TypeDeclaration struct {
	Name string
	Type typ.Type
}

// TypeDeclarations returns every provider-declared named type under its exact
// qualified spelling, in deterministic name order. The result is a fresh
// enumeration; the catalogue keeps the only provider declaration authority.
func (c *Catalogue) TypeDeclarations() []TypeDeclaration {
	if c == nil {
		return nil
	}
	var out []TypeDeclaration
	for _, item := range c.providers {
		for name, declaration := range item.manifest.Types {
			out = append(out, TypeDeclaration{
				Name: qualify(item.manifest.Path, name),
				Type: item.manifest.ScopeType(declaration),
			})
		}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].Name < out[right].Name })
	return out
}
