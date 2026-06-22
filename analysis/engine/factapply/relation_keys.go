package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// relationGraphKeyAt converts a value or len(value) path operand into the key
// form used by State's relational-constraint graph.
func relationGraphKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path, isLength bool) (state.RelOperand, bool) {
	if path.Symbol == 0 {
		return state.RelOperand{}, false
	}
	key, ok := visibility.RootOrVisibleStateKeyAt(resolver, point, path)
	if !ok {
		return state.RelOperand{}, false
	}
	if isLength {
		return state.RelLengthOperand(key), true
	}
	return state.RelValueOperand(key), true
}
