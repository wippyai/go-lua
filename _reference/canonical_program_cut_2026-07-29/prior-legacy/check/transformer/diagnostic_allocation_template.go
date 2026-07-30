package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

func diagnosticContainsAllocationTemplate(reg *axis.Registry, value callpayload.DiagnosticOutput) bool {
	if reg == nil {
		return true
	}
	contains := func(value product.Value) bool {
		term, exact := product.Get(reg, value, identity.Key).Term()
		if !exact {
			return false
		}
		_, allocation := term.Allocation()
		return allocation
	}
	for _, obligation := range value.ParamObligations {
		if contains(obligation.Value) {
			return true
		}
	}
	for _, obligation := range value.PathObligations {
		if contains(obligation.Value) {
			return true
		}
	}
	for _, exposure := range value.ParamExposures {
		if contains(exposure.Contract) {
			return true
		}
	}
	return false
}
