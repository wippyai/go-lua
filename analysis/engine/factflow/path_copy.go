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

func copyExpressionPathMap(in map[ExprRef]path.Path) map[ExprRef]path.Path {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]path.Path, len(in))
	for expr, p := range in {
		out[expr] = copyPath(p)
	}
	return out
}
