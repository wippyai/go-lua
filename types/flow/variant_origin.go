package flow

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
)

// VariantOriginFamily returns a deterministic family id for a finite variant
// provenance relation over target.field.
func VariantOriginFamily(target constraint.Path, field string) uint64 {
	h := internal.HashCombine(internal.FnvString("variant-origin"), target.Hash())
	return internal.HashCombine(h, internal.FnvString(field))
}
