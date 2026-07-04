package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func projectionHasNil(t typ.Type) bool {
	return readmodel.ProjectionHasNil(t)
}

func projectionWithoutNil(t typ.Type) typ.Type {
	return readmodel.ProjectionWithoutNil(t)
}
