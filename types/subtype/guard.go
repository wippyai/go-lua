package subtype

import "github.com/wippyai/go-lua/types/typ"

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || typ.DepthExceeded(depth)
}
