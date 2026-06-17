package factflow

import "github.com/wippyai/go-lua/analysis/domain/path"

func copyExpressionPathMap(in map[ExprRef]path.Path) map[ExprRef]path.Path {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]path.Path, len(in))
	for expr, p := range in {
		out[expr] = p.Clone()
	}
	return out
}
