package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekindof"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RuntimeKindFromType returns concrete Lua runtime-kind evidence for t. The
// canonical mapping lives in axis/runtimekindof so the typewitness reducer and
// this projection share one implementation.
func RuntimeKindFromType(t typ.Type) (runtimekind.Value, bool) {
	return runtimekindof.RuntimeKindFromType(t)
}
