package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func pathMembershipSourceStateKeyAt(resolver *visibility.Resolver, point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	if resolver == nil || p.IsEmpty() {
		return "", false
	}
	if substitutedRootPath(p) {
		return visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
	}
	return visibility.AddressAt(resolver, point, p).VisibleStateKey()
}
