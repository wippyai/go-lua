package defaults

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
)

// SparseSpecs returns the default sparse value-axis bundle in stable registry
// order. Presence is a product core lane and is intentionally excluded.
func SparseSpecs() []axis.ErasedSpec {
	return []axis.ErasedSpec{
		variantorigin.Spec().Erase(),
		identity.Spec().Erase(),
		runtimekind.Spec().Erase(),
		escape.Spec().Erase(),
		ownership.Spec().Erase(),
		evidence.Spec().Erase(),
		assertion.Spec().Erase(),
	}
}
