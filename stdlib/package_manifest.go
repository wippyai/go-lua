package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func packageDeclaration() declaration {
	return declaration{
		signatures: map[string]declaredFunction{
			"loadlib": authored(typ.Func().
				Param("libname", typ.String).Param("funcname", typ.String).
				Returns(typ.Never).Build()),
			"seeall": openAuthored("stdlib.package.seeall.environment", typ.Func().
				Param("module", typ.BuiltinTableTopMarker()).Build(),
				mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: mutation.Unchanged{}}),
		},
		values: map[string]typ.Type{
			"preload": typ.NewMap(typ.String, typ.Any),
			"loaders": typ.NewArray(typ.Any),
			"loaded":  typ.NewMap(typ.String, typ.Any),
			"path":    typ.String,
			"cpath":   typ.String,
			"config":  typ.String,
		},
	}
}
