package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func pathMembershipSourceStateKeysAt(resolver *visibility.Resolver, point cfg.Point, p pathdom.Path) []pathaddr.StateKey {
	if resolver == nil || p.IsEmpty() {
		return nil
	}
	address := visibility.AddressAt(resolver, point, p)
	var out []pathaddr.StateKey
	if visible, ok := address.VisibleStateKey(); ok {
		out = append(out, visible)
	}
	if rootless, ok := address.RootOrVisibleStateKey(); ok && substitutedRootPath(p) && !stateKeyIn(out, rootless) {
		out = append(out, rootless)
	}
	return out
}
