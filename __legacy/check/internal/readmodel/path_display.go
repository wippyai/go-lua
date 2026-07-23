package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

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

func (r Reader) displayPathCanonical(p path.Path) string {
	if p.IsEmpty() {
		return ""
	}
	root := p.Root
	if r.result != nil {
		root = p.DisplayRoot(r.result.SymbolName)
	}
	if root == "" && p.Symbol != 0 {
		root = "$sym" + strconv.FormatUint(uint64(p.Symbol), 10)
	}
	return root + segment.FormatSegments(p.Segments)
}
