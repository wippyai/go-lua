package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// relationGraphKeyAt converts a value or len(value) path operand into the key
// form used by State's relational-constraint graph.
func relationGraphKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path, isLength bool) (pathdom.PathKey, bool) {
	if path.Symbol == 0 {
		return "", false
	}
	key := visibility.RootOrVisibleKeyAt(resolver, point, path)
	if key == "" {
		return "", false
	}
	if isLength {
		return state.LengthRelKey(key), true
	}
	return key, true
}
