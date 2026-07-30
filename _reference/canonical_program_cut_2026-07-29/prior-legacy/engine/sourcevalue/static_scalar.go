package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// StaticScalarValue materializes the context-independent ValueSource subset.
// It is the shared algebra kernel used by concrete source resolution and the
// symbolic transformer compiler. False means the source needs state, call,
// heap, expression-sidecar, or vararg semantics from the contextual resolver.
func StaticScalarValue(reg *axis.Registry, source factflow.ValueSource) (product.Value, bool) {
	if reg == nil || !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return product.Value{}, false
	}
	switch source.Kind {
	case factflow.ValueSourceNil:
		return typevalue.Nil(reg), true
	case factflow.ValueSourceLiteral:
		switch source.LiteralKind {
		case factflow.ValueSourceLiteralBool:
			return typevalue.LiteralBool(reg, source.Bool), true
		case factflow.ValueSourceLiteralInteger:
			return typevalue.LiteralInt(reg, source.Int), true
		case factflow.ValueSourceLiteralNumber:
			return typevalue.LiteralNumber(reg, source.Float), true
		case factflow.ValueSourceLiteralString:
			return typevalue.LiteralString(reg, source.String), true
		}
	}
	return product.Value{}, false
}
