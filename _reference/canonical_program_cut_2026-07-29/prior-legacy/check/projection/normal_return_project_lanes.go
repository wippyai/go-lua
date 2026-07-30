package projection

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// normalReturnSupplementContext owns only syntax/read-model evidence that a
// final State cannot contain. State-derived evidence is converted once by
// callboundary.NormalReturnFactsFromProjectedState before these supplements
// run.
type normalReturnSupplementContext struct {
	reg      *axis.Registry
	result   ResultReader
	exit     state.State
	params   []path.Path
	ks       *keyspace.KeySpace
	boundary boundaryPathProjector
}

type normalReturnSupplement struct {
	lane    callboundary.NormalReturnFactLaneID
	project func(normalReturnSupplementContext, *callboundary.NormalReturnFacts)
}

var normalReturnProjectLanes = []normalReturnSupplement{
	{callboundary.LanePathInvalidations, supplementNormalReturnPathInvalidations},
	{callboundary.LanePersistentPathWrites, supplementNormalReturnPersistentPathWrites},
	{callboundary.LanePathStaticMemberDeltas, supplementNormalReturnPathStaticMemberDeltas},
	{callboundary.LaneDynamicIndexFacts, supplementNormalReturnDynamicIndexFacts},
	{callboundary.LaneStoreRelations, supplementNormalReturnStoreRelations},
	{callboundary.LaneLifecycleFacts, supplementNormalReturnLifecycleFacts},
}

func supplementNormalReturnPathInvalidations(ctx normalReturnSupplementContext, out *callboundary.NormalReturnFacts) {
	out.PathInvalidations = append(out.PathInvalidations, projectAssignmentPathInvalidations(ctx.result, ctx.boundary)...)
}

func supplementNormalReturnPersistentPathWrites(ctx normalReturnSupplementContext, out *callboundary.NormalReturnFacts) {
	out.PersistentPathWrites = append(out.PersistentPathWrites, projectAssignmentPersistentPathWrites(ctx.reg, ctx.result, ctx.exit)...)
	out.PersistentPathWrites = append(out.PersistentPathWrites, projectCallOutcomePersistentPathWrites(ctx.result, ctx.params)...)
}

func supplementNormalReturnPathStaticMemberDeltas(ctx normalReturnSupplementContext, out *callboundary.NormalReturnFacts) {
	out.PathStaticMemberDeltas = append(out.PathStaticMemberDeltas, projectAssignmentPathStaticMemberDeltas(ctx.reg, ctx.result, ctx.exit, ctx.params, ctx.boundary)...)
}

func supplementNormalReturnDynamicIndexFacts(ctx normalReturnSupplementContext, out *callboundary.NormalReturnFacts) {
	out.DynamicIndexFacts = append(out.DynamicIndexFacts, projectAssignmentDynamicIndexFacts(ctx.reg, ctx.result, ctx.params)...)
}

func supplementNormalReturnStoreRelations(ctx normalReturnSupplementContext, out *callboundary.NormalReturnFacts) {
	out.StoreRelations = append(out.StoreRelations, projectAssignmentStoreRelations(ctx.result, ctx.params)...)
}

func supplementNormalReturnLifecycleFacts(ctx normalReturnSupplementContext, out *callboundary.NormalReturnFacts) {
	out.LifecycleFacts = append(out.LifecycleFacts, projectCallOutcomeLifecycleFacts(ctx.result, ctx.params)...)
}
