package projection

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// ValueTypeWithPresence projects a solved product value into the best structural
// type for user-facing checks, applying the value presence lane to the result.
func ValueTypeWithPresence(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if reg == nil || typeValues == nil {
		return nil, false
	}
	return proof.New(reg, typeValues).ValueTypeWithPresence(value)
}
