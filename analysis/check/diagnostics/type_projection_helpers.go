package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func projectionHasNil(t typ.Type) bool {
	return typevalue.ProjectionHasNil(t)
}

func projectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}
