package inspect

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

func unwrapTransparent(t typ.Type) typ.Type {
	return typ.UnwrapTransparentWrappers(t)
}
