package factflow

import "github.com/wippyai/go-lua/analysis/domain/value/product"

// ValueOverlay describes a source expression whose value is resolved from an
// inner source and then conjunctively overlaid with a product value.
type ValueOverlay struct {
	source  ValueSource
	overlay product.Value
}

// NewValueOverlay creates a source-value overlay sidecar for an expression.
func NewValueOverlay(source ValueSource, overlay product.Value) ValueOverlay {
	return ValueOverlay{
		source:  source,
		overlay: overlay,
	}
}

// Source returns the inner value source.
func (o ValueOverlay) Source() ValueSource { return o.source }

// Overlay returns the product value met onto the resolved inner source value.
func (o ValueOverlay) Overlay() product.Value { return o.overlay }

func (o ValueOverlay) copy() ValueOverlay { return o }

func copyValueOverlayMap(in map[ExprRef]ValueOverlay) map[ExprRef]ValueOverlay {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ValueOverlay, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}
