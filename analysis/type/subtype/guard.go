package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

func stopDepthPair(sub, super typ.Type, _ int) bool {
	return sub == nil || super == nil
}
