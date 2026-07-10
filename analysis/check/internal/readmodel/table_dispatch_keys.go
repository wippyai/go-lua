package readmodel

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r Reader) tableDispatchKeysAt(point cfg.Point, table path.Path) (map[string]bool, SourceSpan, bool) {
	if r.result == nil {
		return nil, SourceSpan{}, false
	}
	keys, span, ok := r.result.TableDispatchKeysAt(point, table, r.parents...)
	return keys, sourceSpanFromBody(span), ok
}
