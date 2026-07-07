package body

import "github.com/wippyai/go-lua/analysis/domain/path"

// DisplayPath formats p with body-local symbol names when available.
func (r *Result) DisplayPath(p path.Path) string {
	if p.IsEmpty() {
		return ""
	}
	display := p.Clone()
	if r != nil {
		display.Root = p.DisplayRoot(r.SymbolName)
	}
	return display.String()
}
