package factapply

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// PublishCallReturnPresenceRow prepares the immutable row relation and applies
// its canonical concrete adapter. Guarded execution consumes the same plan at
// the registered coordinate-family capability instead of rebuilding State.
func (a *PathSemanticAuthority) PublishCallReturnPresenceRow(
	ctx context.Context,
	reg *axis.Registry,
	point cfg.Point,
	targets []CallReturnPresenceRowTarget,
	input state.State,
) (state.State, error) {
	if ctx == nil {
		return input, fmt.Errorf("factapply: call return row requires exact path authority")
	}
	plan, err := a.PrepareCallReturnPresenceRow(reg, point, targets)
	if err != nil {
		return input, err
	}
	return plan.ApplyConcrete(ctx, input, input)
}
