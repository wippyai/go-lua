// Package transfer contains the neutral context vocabulary shared by semantic
// operations. Fixed-point scheduling and execution belong to engine/solve.
package transfer

import (
	"context"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// InitialState supplies an explicit starting state for a point.
type InitialState func(point cfg.Point) (state.State, bool)

// Result maps each reachable CFG point to its input state.
type Result map[cfg.Point]state.State

// NodeContext is the generic context passed to node semantic operations.
type NodeContext struct {
	Context  context.Context
	Session  *cancellation.Session
	Graph    cfg.Graph
	Registry *axis.Registry
	Point    cfg.Point
	Node     *cfg.Node
	Read     func(cfg.Point) state.State
}

// EdgeContext is the generic context passed to edge semantic operations.
type EdgeContext struct {
	Context  context.Context
	Session  *cancellation.Session
	Graph    cfg.Graph
	Registry *axis.Registry
	Edge     cfg.Edge
	HasCond  bool
	Read     func(cfg.Point) state.State
}
