package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func (c callEntryProjector) expectedArgType(point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type {
	if c.program == nil || c.program.driver == nil || c.graph == nil || c.transfer == nil || info == nil || info.Call == nil || in == nil || argIdx < 0 {
		return nil
	}
	frame, ok := c.typer.callBoundaryFrame(info.Call, c.transfer.ProductCallContext(in, info.Call), productCallOutcomeOptions{})
	if !ok {
		return nil
	}
	return frame.expectedArgType(info, c.forceMethodReceiver(point, info), argIdx)
}

func (c callEntryProjector) forceMethodReceiver(point cfg.Point, info *cfg.CallInfo) bool {
	if c.program == nil || c.graph == nil || info == nil || info.Call == nil {
		return false
	}
	ref, ok := c.program.refByGraph(c.graph)
	if !ok {
		return false
	}
	return callsite.ForceMethodReceiverAtPoint(c.graph.Bindings(), c.graph, c.program.inputs[ref].Evidence, point, info.Call)
}
