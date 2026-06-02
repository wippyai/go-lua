package callbackenv

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// GlobalBinding is one callback-scoped global binding after an EnvOverlay name
// has been lowered to the callback body's graph symbol.
type GlobalBinding struct {
	Symbol cfg.SymbolID
	Type   typ.Type
}
