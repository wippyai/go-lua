package body

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (c *checker) signatureNameForCall(bindings *bind.Result) effectlowering.SignatureNameFunc {
	return func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
		result := Result{bindings: bindings}
		return result.stableCalleeName(call.CalleeSymbol(), call.CalleePath())
	}
}

func (r *Result) stableCalleeName(callee symbol.ID, calleePath path.Path) (string, bool) {
	if r == nil || r.bindings == nil {
		return "", false
	}
	root := callee
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	if root == 0 {
		return "", false
	}
	kind, ok := r.bindings.Kind(root)
	if !ok || kind != symbol.Global {
		return "", false
	}
	name := r.bindings.Name(root)
	if name == "" {
		return "", false
	}
	if len(calleePath.Segments) == 0 {
		return name, true
	}
	var b strings.Builder
	b.WriteString(name)
	for _, seg := range calleePath.Segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name == "" {
				return "", false
			}
			b.WriteByte('.')
			b.WriteString(seg.Name)
		default:
			return "", false
		}
	}
	return b.String(), true
}
