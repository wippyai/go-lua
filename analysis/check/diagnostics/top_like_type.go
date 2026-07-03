package diagnostics

import "github.com/wippyai/go-lua/analysis/type/typ"

func topLikeType(t typ.Type) bool {
	return t == nil || typ.IsAny(t) || typ.IsUnknown(t)
}
