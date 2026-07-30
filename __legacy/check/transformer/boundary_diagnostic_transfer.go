package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

// materializeBoundaryPrefixDiagnostics uses the reachable diagnostic identity
// for suspension. A prefix contributes facts and MaySuspend events, but cannot
// revoke a later whole-path certification merely because it is not a terminal.
func materializeBoundaryPrefixDiagnostics(body *relationProgramBody, contribution semanticContribution) (callpayload.DiagnosticOutput, error) {
	return materializeBoundaryDiagnosticOutput(body, true, contribution.maySuspend,
		contribution.paramObligations, contribution.pathObligations, contribution.paramExposures)
}

func materializeBoundaryOutcomeDiagnostics(body *relationProgramBody, outcome boundaryOutcomeTuple) (callpayload.DiagnosticOutput, error) {
	return materializeBoundaryDiagnosticOutput(body, outcome.suspensionKnown, outcome.maySuspend,
		outcome.paramObligations, outcome.pathObligations, outcome.paramExposures)
}

// materializeBoundaryDiagnosticOutput closes arena-owned lexical syntax into
// the canonical body-relative diagnostic lattice value. It is a transfer of
// the sole relation equation: no State is inspected and no summary/evaluator
// is invoked. Reachability is owned by the coordinate leaf carrying the value.
func materializeBoundaryDiagnosticOutput(
	body *relationProgramBody,
	suspensionKnown, maySuspend bool,
	paramObligations []callpayload.CallParamObligation,
	pathObligations []boundaryPathObligationTerm,
	paramExposures []boundaryParamExposureTerm,
) (callpayload.DiagnosticOutput, error) {
	if body == nil || body.relation.arena == nil ||
		!validBoundaryParamObligations(body.relation.arena.reg, body.relation.shape.Params, paramObligations) ||
		!validBoundaryPathObligations(body.relation.arena, body.relation.shape, pathObligations) ||
		!validBoundaryParamExposures(body.relation.arena, body.relation.shape, paramExposures) {
		return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: boundary diagnostic materialization is malformed")
	}
	out := callpayload.DiagnosticOutput{
		SuspensionKnown:  suspensionKnown,
		MaySuspend:       maySuspend,
		ParamObligations: cloneBoundaryParamObligations(paramObligations),
	}
	for _, obligation := range pathObligations {
		path, exact := bodyRelativeBoundaryDiagnosticPath(body, obligation.path)
		if !exact {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: boundary path obligation has no body-relative path")
		}
		out.PathObligations = append(out.PathObligations, callpayload.CallPathObligation{Path: path, Value: obligation.value})
	}
	for _, exposure := range paramExposures {
		path, exact := bodyRelativeBoundaryDiagnosticPath(body, exposure.source)
		if !exact || !path.IsPlaceholder() {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: boundary parameter exposure has no parameter-relative path")
		}
		out.ParamExposures = append(out.ParamExposures, callpayload.CallParamExposure{Source: path, Contract: exposure.contract, Kind: exposure.kind})
	}
	return out.Normalize(body.relation.arena.reg), nil
}

// liftBoundaryApplicationDiagnostics is the typed Apply transfer for the
// diagnostic component of the sole tuple-mu carrier. The child value is
// target-relative; the result is caller-boundary-relative and may be lifted
// again by an enclosing Apply. Local/rvalue arguments have no outward lexical
// identity and are intentionally discharged only at their own call point.
func liftBoundaryApplicationDiagnostics(caller, target *relationProgramBody, frame *linkedRelationFrame, child callpayload.DiagnosticOutput) (callpayload.DiagnosticOutput, error) {
	if caller == nil || target == nil || caller.relation.arena == nil || target.relation.arena == nil || frame == nil ||
		!frame.valid() || frame.owner != caller.variable || frame.target != target.variable || !child.Valid(target.relation.arena.reg) {
		return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: boundary diagnostic Apply transfer is unowned")
	}
	out := callpayload.DiagnosticOutput{SuspensionKnown: child.SuspensionKnown, MaySuspend: child.MaySuspend}
	appendObligation := func(targetPath pathdom.Path, obligation callpayload.CallParamObligation) error {
		callerPath, exact := linkedCallerBoundaryDiagnosticPath(caller, target, frame, targetPath)
		if !exact {
			// The exact local call outcome still owns this requirement. A literal,
			// local temporary, or otherwise pathless argument has no contract to
			// publish through the caller's lexical boundary.
			return nil
		}
		if callerPath.IsPlaceholder() && len(callerPath.Segments) == 0 {
			obligation.ParamIndex = callerPath.PlaceholderIndex()
			out.ParamObligations = append(out.ParamObligations, obligation)
			return nil
		}
		out.PathObligations = append(out.PathObligations, callpayload.CallPathObligation{Path: callerPath, Value: obligation.Value})
		return nil
	}
	for _, obligation := range child.ParamObligations {
		if obligation.ParamIndex < 0 || uint32(obligation.ParamIndex) >= target.relation.shape.Params {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: child parameter obligation is outside target shape")
		}
		if err := appendObligation(pathdom.NewPlaceholder(obligation.ParamIndex), obligation); err != nil {
			return callpayload.DiagnosticOutput{}, err
		}
	}
	for _, obligation := range child.PathObligations {
		callerPath, exact := linkedCallerBoundaryDiagnosticPath(caller, target, frame, obligation.Path)
		if !exact {
			continue
		}
		if callerPath.IsPlaceholder() && len(callerPath.Segments) == 0 {
			out.ParamObligations = append(out.ParamObligations, callpayload.CallParamObligation{ParamIndex: callerPath.PlaceholderIndex(), Value: obligation.Value})
		} else {
			out.PathObligations = append(out.PathObligations, callpayload.CallPathObligation{Path: callerPath, Value: obligation.Value})
		}
	}
	for _, exposure := range child.ParamExposures {
		if !exposure.Source.IsPlaceholder() || exposure.Source.PlaceholderIndex() < 0 || uint32(exposure.Source.PlaceholderIndex()) >= target.relation.shape.Params {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: child parameter exposure is outside target shape")
		}
		callerPath, exact := linkedCallerBoundaryDiagnosticPath(caller, target, frame, exposure.Source)
		if !exact || !callerPath.IsPlaceholder() {
			// Capture/global mutation is already carried by exact State boundary
			// transport. CallParamExposure is strictly explicit-param-relative;
			// do not invent a second PathExposure vocabulary.
			continue
		}
		exposure.Source = callerPath
		out.ParamExposures = append(out.ParamExposures, exposure)
	}
	return out.Normalize(caller.relation.arena.reg), nil
}
