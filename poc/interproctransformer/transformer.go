// Package interproctransformer is an isolated proof that one guarded lexical
// boundary transformer can replace repeated exact-context callee body solves.
// It is deliberately disconnected from production program solving.
package interproctransformer

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// GuardedBoundary is the compiled-once semantic content of the accepted POC
// slice: eq(a,b) returns true and guarantees a==b; neq(a,b) returns false.
// No concrete State, caller path, placement, heap identity, or sidecar is kept.
type GuardedBoundary struct{}

// Compile rejects capabilities whose identities/effects cannot be represented
// by this boundary. The explicit booleans make widening the POC fail closed.
type CompileRequest struct {
	Recursive, Heap, Placement, StateSensitiveSidecars bool
}

func Compile(req CompileRequest) (GuardedBoundary, error) {
	if req.Recursive || req.Heap || req.Placement || req.StateSensitiveSidecars {
		return GuardedBoundary{}, fmt.Errorf("interproctransformer: contextual capability")
	}
	return GuardedBoundary{}, nil
}

// Specialize evaluates the guarded boundary for one caller binding. It is the
// intended replacement for solving the callee CFG with that binding.
func (GuardedBoundary) Specialize(reg *axis.Registry, left, right product.Value) summary.Summary {
	equal := product.Equal(reg, left, right)
	out := summary.Summary{
		Returns:            []product.Value{typevalue.LiteralBool(reg, equal)},
		NormalReturnParams: []product.Value{left, right},
		ReturnConditionParamRefinements: []summary.ReturnConditionParamRefinement{{
			ReturnIndex: 0, ReturnValue: equal, Target: pathdom.NewPlaceholder(0), Value: left,
		}},
	}
	if equal {
		out.NormalReturnParamEqualities = []summary.ParamEquality{{Left: 0, Right: 1}}
	}
	return summary.Normalize(reg, out)
}

// Lower specializes the existing Summary vocabulary into the existing generic
// CallOutcome vocabulary for precisely the lanes admitted by Compile.
func Lower(sum summary.Summary) (callpayload.CallOutcome, error) {
	value := reflect.ValueOf(sum)
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		switch typ.Field(i).Name {
		case "Returns", "NormalReturnParams", "NormalReturnParamEqualities", "ReturnConditionParamRefinements":
			continue
		}
		field := value.Field(i)
		nonempty := false
		switch field.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.String:
			nonempty = field.Len() != 0
		default:
			nonempty = !field.IsZero()
		}
		if nonempty {
			return callpayload.CallOutcome{}, fmt.Errorf("interproctransformer: unsupported summary field %s", typ.Field(i).Name)
		}
	}
	out := callpayload.CallOutcome{PostReturnAuthority: true, SuspensionKnown: true}
	for i, value := range sum.Returns {
		out.Results = append(out.Results, callpayload.CallResult{Index: i, Value: value})
	}
	for i, value := range sum.NormalReturnParams {
		out.ParamPathRefinements = append(out.ParamPathRefinements, callpayload.CallParamPathRefinement{
			Path: pathdom.NewPlaceholder(i), Value: value,
		})
	}
	for _, eq := range sum.NormalReturnParamEqualities {
		out.ParamPathRelations = append(out.ParamPathRelations, callpayload.CallParamPathRelation{
			Kind: callpayload.CallPathRelationEqual,
			Left: pathdom.NewPlaceholder(eq.Left), Right: pathdom.NewPlaceholder(eq.Right),
		})
	}
	for _, refinement := range sum.ReturnConditionParamRefinements {
		out.ReturnConditionRefinements = append(out.ReturnConditionRefinements, callpayload.CallReturnConditionRefinement{
			ReturnIndex: refinement.ReturnIndex, ReturnValue: refinement.ReturnValue,
			Target: refinement.Target.Clone(), Value: refinement.Value,
		})
	}
	return out, nil
}

// ApplyCaller runs the provider-independent production ordinary and edge
// seams. Result-slot publication remains explicit because it is intentionally
// outside ResolvedCallOutcomeOrdinaryEffects.
func ApplyCaller(reg *axis.Registry, graph cfg.Graph, facts factflow.Facts, resolver *visibility.Resolver, call, branch cfg.Point, entry state.State, outcome callpayload.CallOutcome) transfer.Result {
	site, _ := facts.CallSiteView(call)
	return transfer.Run(transfer.Config{
		Graph: graph, Registry: reg, EntryState: entry,
		NodeTransfer: func(ctx transfer.NodeContext, in state.State) state.State {
			if ctx.Point != call {
				return in
			}
			applied := factapply.ApplyResolvedCallOutcomeOrdinaryEffects(factapply.ResolvedCallOutcomeOrdinaryEffectsRequest{
				Context: ctx, Facts: facts, Resolver: resolver, Output: in, Site: site, Outcome: outcome,
			})
			if !applied.Applied {
				return in
			}
			out := applied.Output
			for _, result := range outcome.Results {
				out = out.WriteReturnSlot(reg, result.Index, result.Value)
			}
			return out
		},
		EdgeTransfer: func(ctx transfer.EdgeContext, out state.State) state.State {
			if ctx.Edge.From != branch {
				return out
			}
			return factapply.ApplyResolvedCallOutcomeEdge(factapply.ResolvedCallOutcomeEdgeRequest{
				Context: ctx, Facts: facts, Resolver: resolver, Output: out,
				CallPoint: call, Site: site, Outcome: outcome,
			})
		},
	})
}
