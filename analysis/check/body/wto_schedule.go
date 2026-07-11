package body

import (
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// compileWTOPlan prepares the immutable generic schedule with the body. State
// and its lane catalog remain entirely outside the plan.
func compileWTOPlan(graph cfg.Graph, schedule transfer.Schedule) *solve.WTOPlan[cfg.Point] {
	if graph == nil || schedule == transfer.ScheduleFIFO {
		return nil
	}
	cells := append([]cfg.Point(nil), graph.RPO()...)
	return solve.NewWTOPlan(cells, func(point cfg.Point) []cfg.Point {
		return cfg.SuccessorsReadOnly(graph, point)
	})
}
