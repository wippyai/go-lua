package projection

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// These narrow read surfaces are shared by non-diagnostic summary lanes. They
// carry solved facts only; none exposes binder, AST, or solver authority.
type callSiteViewReader interface {
	CallSiteView(cfg.Point) (factflow.CallSiteView, bool)
}

type callOutcomeAtReader interface {
	CallOutcomeAt(cfg.Point) (callpayload.CallOutcome, bool)
}

type sourceValueBeforeBoundaryReader interface {
	SourceValueBeforeBoundary(cfg.Point, factflow.ValueSource) (product.Value, bool)
}

type pathValueAtBoundaryReader interface {
	PathValueAtBoundary(cfg.Point, pathdom.Path) (product.Value, bool)
}

type diagnosticOutputReader interface {
	DiagnosticOutput() callpayload.DiagnosticOutput
}

// projectDiagnosticOutput converts the canonical body-level diagnostic tuple
// into the older public Summary carriers. It performs no syntax scan and owns
// no alternate diagnostic semantics.
func projectDiagnosticOutput(
	reg *axis.Registry,
	output callpayload.DiagnosticOutput,
	paramCount int,
) ([]product.Value, []summary.CapturedPathObligation, []summary.ParamSinkExposure, error) {
	if reg == nil || paramCount < 0 {
		return nil, nil, nil, fmt.Errorf("projection: diagnostic output requires registry and parameter arity")
	}
	output = output.Normalize(reg)
	if !output.Valid(reg) {
		return nil, nil, nil, fmt.Errorf("projection: malformed diagnostic output")
	}

	var params []product.Value
	if paramCount != 0 {
		params = make([]product.Value, paramCount)
		for index := range params {
			params[index] = product.Top()
		}
	}
	for _, obligation := range output.ParamObligations {
		if obligation.ParamIndex < 0 || obligation.ParamIndex >= paramCount {
			return nil, nil, nil, fmt.Errorf("projection: diagnostic parameter %d is outside arity %d", obligation.ParamIndex, paramCount)
		}
		if !summary.UsefulParamObligation(reg, obligation.Value) {
			continue
		}
		params[obligation.ParamIndex] = product.Meet(reg, params[obligation.ParamIndex], obligation.Value)
	}

	captured := make([]summary.CapturedPathObligation, 0, len(output.PathObligations))
	for _, obligation := range output.PathObligations {
		stable, ok := pathaddr.StableOfPath(obligation.Path)
		if !ok {
			return nil, nil, nil, fmt.Errorf("projection: diagnostic path obligation has no stable address")
		}
		if !summary.UsefulParamObligation(reg, obligation.Value) {
			continue
		}
		captured = append(captured, summary.CapturedPathObligation{Path: stable.StableKey(), Value: obligation.Value})
	}

	exposures := make([]summary.ParamSinkExposure, 0, len(output.ParamExposures))
	for _, exposure := range output.ParamExposures {
		source, ok := pathaddr.RootPlaceholderKeyFromPath(exposure.Source)
		if !ok {
			return nil, nil, nil, fmt.Errorf("projection: diagnostic exposure source %s is not an exact parameter root", exposure.Source)
		}
		if product.Equal(reg, exposure.Contract, product.Bottom(reg)) || product.Equal(reg, exposure.Contract, product.Top()) {
			return nil, nil, nil, fmt.Errorf("projection: diagnostic exposure for %s has no exact contract", exposure.Source)
		}
		// Summary predates exposure kinds. Source and Contract remain exact;
		// application selects the existing record-widening behavior.
		exposures = append(exposures, summary.ParamSinkExposure{Source: source, Contract: exposure.Contract})
	}
	return params, captured, exposures, nil
}

func callSiteViewAt(result ResultReader, point cfg.Point) (factflow.CallSiteView, bool) {
	reader, ok := result.(callSiteViewReader)
	if !ok {
		return factflow.CallSiteView{}, false
	}
	return reader.CallSiteView(point)
}

func hasCallSiteView(result ResultReader) bool {
	_, ok := result.(callSiteViewReader)
	return ok
}

func rootAssignmentAt(result ResultReader, point cfg.Point) (factflow.RootAssignment, bool) {
	reader, ok := result.(rootAssignmentReader)
	if !ok {
		return factflow.RootAssignment{}, false
	}
	return reader.RootAssignment(point)
}

func ordinaryRootAssignmentAt(result ResultReader, point cfg.Point) (factflow.RootAssignment, bool) {
	assignment, ok := rootAssignmentAt(result, point)
	if !ok || assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
		return factflow.RootAssignment{}, false
	}
	return assignment, true
}

func pathAssignmentAt(result ResultReader, point cfg.Point) (factflow.PathAssignment, bool) {
	reader, ok := result.(pathAssignmentReader)
	if !ok {
		return factflow.PathAssignment{}, false
	}
	return reader.PathAssignment(point)
}

func pathDescendantInvalidationAt(result ResultReader, point cfg.Point) (factflow.PathDescendantInvalidation, bool) {
	reader, ok := result.(pathDescendantInvalidationReader)
	if !ok {
		return factflow.PathDescendantInvalidation{}, false
	}
	return reader.PathDescendantInvalidation(point)
}

func callSiteBindings(result ResultReader, site factflow.CallSiteView) []pathdom.Path {
	pathReader, ok := result.(expressionPathRefReader)
	if !ok {
		return nil
	}
	var bindings []pathdom.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = appendPathBinding(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		sourcePath, ok := valueSourcePath(result, pathReader, source)
		if ok && !sourcePath.IsEmpty() {
			bindings = appendPathBinding(bindings, index+offset, sourcePath)
		}
		return true
	})
	return bindings
}

func appendPathBinding(bindings []pathdom.Path, index int, value pathdom.Path) []pathdom.Path {
	if index < 0 || value.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, pathdom.Path{})
	}
	bindings[index] = value
	return bindings
}

var _ diagnosticOutputReader = ResultReader(nil)
