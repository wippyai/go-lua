package typevalue

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func normalize(t typ.Type) typ.Type {
	return unwrap.Annotated(unwrap.NormalizeNil(t))
}
