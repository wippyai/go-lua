package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// FreezeDynamicReadAddress exposes the canonical visibility-owned address
// freezer without exposing this authority's resolver. Concrete and guarded
// dynamic reads therefore cannot acquire different candidate key sets.
func (a *PathSemanticAuthority) FreezeDynamicReadAddress(point cfg.Point, path pathdom.Path) (visibility.DynamicReadAddress, bool) {
	if !a.Valid() {
		return visibility.DynamicReadAddress{}, false
	}
	return visibility.FreezeDynamicReadAddress(a.resolver.KeySpace(), a.resolver, point, path)
}
