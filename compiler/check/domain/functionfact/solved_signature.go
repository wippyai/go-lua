package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	returns "github.com/wippyai/go-lua/compiler/check/domain/returns"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

type solvedSignatureInput struct {
	graph          *cfg.Graph
	evidence       api.FlowEvidence
	flowOps        api.FlowOps
	source         *typ.Function
	moduleBindings *bind.BindingTable
	observer       observation.Projector
	returnSynth    returns.ExprSynth
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
		flowOps:        result.SolvedFlow(),
		source:         result.SourceSignature,
		moduleBindings: result.ModuleBindings,
		observer:       observation.FromSolvedObservationState(result.ObservationState(), nil),
		returnSynth:    result.SolvedSynth(),
	}, fn)
}

// SolvedSignatureFromView projects the solved function signature from the
// stable analysis view used by nested processing.
func SolvedSignatureFromView(result *api.FuncAnalysisView, fn *ast.FunctionExpr) *typ.Function {
	if result == nil {
		return nil
	}
	return solvedSignatureFromInput(solvedSignatureInput{
		graph:       result.Graph,
		evidence:    result.Evidence,
		flowOps:     result.SolvedFlow(),
		source:      result.SourceSignature,
		observer:    observation.FromAnalysisView(result, nil),
		returnSynth: result.SolvedSynth(),
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
		returnSynth := input.returnSynth
		if returnSynth == nil {
			returnSynth = input.observer
		}
		if observed := returns.ObservedSummary(input.graph, input.evidence.Returns, input.flowOps, returnSynth); len(observed) > 0 {
			if aligned := typjoin.WithReturns(fnType, observed); aligned != nil {
				fnType = aligned
			}
		}
	}
	fnType = attachSolvedCallbackOverlaySpec(fnType, input)
	return erreffect.AttachInferredErrorReturnSpec(fnType, input.evidence, input.flowOps, input.observer)
}

func attachSolvedCallbackOverlaySpec(fnType *typ.Function, input solvedSignatureInput) *typ.Function {
	if fnType == nil || input.graph == nil {
		return fnType
	}
	overlays := callbackenv.Infer(
		input.graph,
		input.evidence,
		input.graph.ParamSlotsReadOnly(),
		input.observer.TypeOf,
		input.moduleBindings,
	)
	if len(overlays) == 0 {
		return fnType
	}
	return attachCallbackEnvOverlays(fnType, overlays)
}

func attachCallbackEnvOverlays(fnType *typ.Function, overlays callbackenv.Overlays) *typ.Function {
	if fnType == nil || len(overlays) == 0 {
		return fnType
	}
	spec := cloneContractSpec(fnType)
	for _, param := range overlays {
		if len(param.Overlay) == 0 {
			continue
		}
		cb := spec.GetCallback(param.ParamIndex).Clone()
		if cb == nil {
			cb = &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}
		}
		cb.EnvOverlay = callbackenv.MergeIntoContractOverlay(cb.EnvOverlay, param.Overlay)
		spec.WithCallback(param.ParamIndex, cb)
	}
	return rebuildFunctionWithSpec(fnType, spec)
}
