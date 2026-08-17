package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

type bindingSummarySurfaceReceipt interface {
	boundTopologySummarySurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, composition.Key, bool)
}

func validateSummarySurfaceReceipt(receipt bindingSummarySurfaceReceipt, state *schemaBindingState, authority *schemaBindingAuthority, surface equation.Surface) bool {
	receiptState, receiptAuthority, factor, normalizer, ok := receipt.boundTopologySummarySurfaceReceipt()
	return ok && receiptState == state && receiptAuthority == authority && surface.Available() && surface.Factor == factor && surface.Form == equation.SurfaceReadSummary && surface.Semantic == normalizer && surface.Normalizer == normalizer && surface.Mode == equation.TargetModeNone
}
