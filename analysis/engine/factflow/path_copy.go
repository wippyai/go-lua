package factflow

import "github.com/wippyai/go-lua/analysis/domain/path"

func copyPath(p path.Path) path.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
}
