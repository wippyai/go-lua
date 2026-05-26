package domain

import (
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
)

func joinNarrowedTypes(left, right typ.Type) typ.Type {
	return typejoin.Types(left, right)
}
