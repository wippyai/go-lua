package readmodel

import "github.com/wippyai/go-lua/analysis/check/body"

func projectBodyOccurrences[From, To any](
	r Reader,
	visit func(To) bool,
	each func(*body.Result, func(From) bool) bool,
	project func(Reader, From) To,
) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	return each(r.result, func(occ From) bool {
		return visit(project(r, occ))
	})
}
