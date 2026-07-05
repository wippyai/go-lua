package readmodel

import "github.com/wippyai/go-lua/analysis/domain/path"

func (r Reader) displayPath(p path.Path) string {
	if p.IsEmpty() {
		return ""
	}
	display := p.Clone()
	if r.result != nil {
		display.Root = p.DisplayRoot(r.result.SymbolName)
	}
	return display.String()
}
