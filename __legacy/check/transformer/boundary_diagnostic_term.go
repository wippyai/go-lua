package transformer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// boundaryPathObligationTerm is diagnostic-only pre-call evidence over one
// symbolic boundary path. Path stays a PathTerm until the enclosing lexical
// frame is bound, so capture and parameter roots use the same call-frame
// substitution as State rather than retaining producer-local symbols.
type boundaryPathObligationTerm struct {
	path  PathTerm
	value product.Value
}

// boundaryParamExposureTerm is one caller-visible mutable-view exposure. The
// source remains a symbolic boundary path for the same reason as an obligation;
// contract and kind are the canonical CallParamExposure payload fields.
type boundaryParamExposureTerm struct {
	source   PathTerm
	contract product.Value
	kind     factflow.CovariantExposureKind
}

func cloneBoundaryParamObligations(in []callpayload.CallParamObligation) []callpayload.CallParamObligation {
	return append([]callpayload.CallParamObligation(nil), in...)
}

func cloneBoundaryPathObligations(in []boundaryPathObligationTerm) []boundaryPathObligationTerm {
	return append([]boundaryPathObligationTerm(nil), in...)
}

func cloneBoundaryParamExposures(in []boundaryParamExposureTerm) []boundaryParamExposureTerm {
	return append([]boundaryParamExposureTerm(nil), in...)
}

func validBoundaryParamObligations(reg *axis.Registry, width uint32, in []callpayload.CallParamObligation) bool {
	if len(in) != 0 && (callpayload.CallOutcome{ParamObligations: cloneBoundaryParamObligations(in)}).Empty() {
		return false
	}
	for _, obligation := range in {
		if obligation.ParamIndex < 0 || uint32(obligation.ParamIndex) >= width || !usefulBoundaryDiagnosticValue(reg, obligation.Value) {
			return false
		}
	}
	return true
}

func validBoundaryPathObligations(arena *Arena, shape Shape, in []boundaryPathObligationTerm) bool {
	if arena == nil {
		return false
	}
	for _, obligation := range in {
		if !arena.validPath(obligation.path, shape) || !usefulBoundaryDiagnosticValue(arena.reg, obligation.value) {
			return false
		}
	}
	if len(in) != 0 {
		probe := callpayload.CallOutcome{PathObligations: []callpayload.CallPathObligation{{
			Path: pathdom.NewPlaceholder(0), Value: in[0].value,
		}}}
		if probe.Empty() {
			return false
		}
	}
	return true
}

func validBoundaryParamExposures(arena *Arena, shape Shape, in []boundaryParamExposureTerm) bool {
	if arena == nil {
		return false
	}
	for _, exposure := range in {
		if !arena.validPath(exposure.source, shape) || !product.BelongsToRegistry(arena.reg, exposure.contract) {
			return false
		}
	}
	if len(in) != 0 {
		probe := callpayload.CallOutcome{ParamExposures: []callpayload.CallParamExposure{{
			Source: pathdom.NewPlaceholder(0), Contract: in[0].contract, Kind: in[0].kind,
		}}}
		if probe.Empty() {
			return false
		}
	}
	return true
}

func usefulBoundaryDiagnosticValue(reg *axis.Registry, value product.Value) bool {
	return reg != nil && product.BelongsToRegistry(reg, value) &&
		!product.Equal(reg, value, product.Top()) && !product.Equal(reg, value, product.Bottom(reg))
}
