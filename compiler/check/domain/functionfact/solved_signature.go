package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

type solvedSignatureInput struct {
	graph          *cfg.Graph
	evidence       api.FlowEvidence
	flowSolution   *flow.Solution
	source         *typ.Function
	moduleBindings *bind.BindingTable
	observer       observation.Projector
}

// SolvedSignatureFromResult projects the solved function signature from a full
// analysis result without consulting mutable synthesis state.
func SolvedSignatureFromResult(result *api.FuncResult, fn *ast.FunctionExpr) *typ.Function {
	if result == nil {
		return nil
	}
	return solvedSignatureFromInput(solvedSignatureInput{
		graph:          result.Graph,
		evidence:       result.Evidence,
		flowSolution:   result.FlowSolution,
		source:         result.SourceSignature,
		moduleBindings: result.ModuleBindings,
		observer:       observation.FromFuncResult(result, nil),
	}, fn)
}

// SolvedSignatureFromView projects the solved function signature from the
// stable analysis view used by nested processing.
func SolvedSignatureFromView(result *api.FuncAnalysisView, fn *ast.FunctionExpr) *typ.Function {
	if result == nil {
		return nil
	}
	return solvedSignatureFromInput(solvedSignatureInput{
		graph:        result.Graph,
		evidence:     result.Evidence,
		flowSolution: result.FlowSolution,
		source:       result.SourceSignature,
		observer:     observation.FromAnalysisView(result, nil),
	}, fn)
}

func solvedSignatureFromInput(input solvedSignatureInput, fn *ast.FunctionExpr) *typ.Function {
	if fn == nil {
		return nil
	}
	fnType := input.source
	if fnType == nil {
		fnType = typ.Func().Build()
	}
	if len(fn.ReturnTypes) == 0 {
		if observed := abstractreturns.ObservedSummary(input.graph, input.evidence.Returns, input.flowSolution, input.observer); len(observed) > 0 {
			if aligned := typjoin.WithReturns(fnType, observed); aligned != nil {
				fnType = aligned
			}
		}
	}
	fnType = attachSolvedCallbackOverlaySpec(fnType, input)
	return erreffect.AttachInferredErrorReturnSpec(fnType, input.evidence, input.flowSolution, input.observer)
}

func attachSolvedCallbackOverlaySpec(fnType *typ.Function, input solvedSignatureInput) *typ.Function {
	if fnType == nil || input.graph == nil {
		return fnType
	}
	overlays := InferCallbackEnvOverlays(
		input.graph,
		input.evidence,
		input.graph.ParamSlotsReadOnly(),
		input.observer.TypeOf,
		input.moduleBindings,
	)
	if len(overlays) == 0 {
		return fnType
	}
	return AttachCallbackEnvOverlays(fnType, overlays)
}
