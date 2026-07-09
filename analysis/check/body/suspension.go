package body

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// PointMaySuspend reports whether executing point may suspend the current
// frame. It is exact for recognized scheduler-controlled operations and
// conservative for calls whose callee surface is not suspension-certified.
func (r *Result) PointMaySuspend(point cfg.Point) bool {
	if r == nil || !r.PointNormallyReachable(point) {
		return false
	}
	if len(r.ChannelSelects(point)) != 0 {
		return true
	}
	if !r.HasCallSite(point) {
		return false
	}
	outcome, ok := r.CallOutcomeAt(point)
	if !ok || !outcome.SuspensionKnown {
		return true
	}
	return outcome.MaySuspend
}
