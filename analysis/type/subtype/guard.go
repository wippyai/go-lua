package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

func stopDepthPair(sub, super typ.Type, depth int) bool {
	return typ.DepthExceeded(depth) || sub == nil || super == nil
}
