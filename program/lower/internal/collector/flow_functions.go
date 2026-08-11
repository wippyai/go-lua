package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func (w FlowFunctionsWriter) DeclareFunction(span source.Span, owner keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Function owner")
	}
	term := c.mint(keyspace.FamilyFunction, span)
	if term == 0 {
		return 0
	}
	c.flow.functions.functions = append(c.flow.functions.functions, flow.Function{Owner: owner})
	// Function declaration atomically coordinates its executable row with the
	// one Static contract sidecar; Static remains the sidecar owner.
	if err := c.static.FunctionContractPlaceholder(term); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// FillFunction attaches executable body, optional vararg Cell, captures, and
// Source formal order. Static contract rows are filled through the explicit
// operations below, each as one atomic collector coordination with Static.
func (w FlowFunctionsWriter) FillFunction(function, body keyspace.Term, formals []keyspace.Term, vararg keyspace.Term, captures []flow.Capture) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) || !validFamilyTerm(c, body, keyspace.FamilyBody) || function == body || int(keyspace.TermOrdinal(function)) > len(c.flow.functions.functions) {
		return rejectMutationf(c, "program/lower/collector: invalid Function fill")
	}
	row := &c.flow.functions.functions[keyspace.TermOrdinal(function)-1]
	if row.Body != 0 {
		return rejectMutationf(c, "program/lower/collector: Function filled twice")
	}
	if vararg != 0 && !localCellInBodyAdmission(c, vararg, body) {
		return rejectMutationf(c, "program/lower/collector: invalid Function vararg")
	}
	seenFormals := make(map[keyspace.Term]struct{}, len(formals))
	for _, formal := range formals {
		// Formals are activation cells owned by the function body. A census
		// family check alone would allow a current-but-foreign Cell to defer
		// the ownership failure until Source/Flow freeze.
		if !localCellInBodyAdmission(c, formal, body) {
			return rejectMutationf(c, "program/lower/collector: invalid Function formal")
		}
		if _, duplicate := seenFormals[formal]; duplicate || sourceCellAlreadyOrdered(c, formal) {
			return rejectMutationf(c, "program/lower/collector: duplicate Function formal Cell")
		}
		seenFormals[formal] = struct{}{}
	}
	for _, capture := range captures {
		if !localCellInBodyAdmission(c, capture.Inner, body) || !localCellAdmission(c, capture.Outer) || c.flow.storage.cells[keyspace.TermOrdinal(capture.Outer)-1].Body == body {
			return rejectMutationf(c, "program/lower/collector: invalid Function capture")
		}
	}
	for index, capture := range captures {
		for _, previous := range captures[:index] {
			if previous.Inner == capture.Inner || previous.Outer == capture.Outer {
				return rejectMutationf(c, "program/lower/collector: duplicate Function capture")
			}
		}
	}
	r, ok := rangeFor(len(c.flow.functions.captures), len(captures))
	if !ok {
		return rejectMutationf(c, "program/lower/collector: Function capture range overflow")
	}
	c.flow.functions.captures = append(c.flow.functions.captures, captures...)
	row.Body, row.Vararg, row.Captures = body, vararg, r
	if !c.Source().Order().FunctionFormals(function, formals) {
		return false
	}
	return true
}

func (w FlowFunctionsWriter) SetFunctionGenerics(function keyspace.Term, params []keyspace.Term) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) {
		return rejectMutationf(c, "program/lower/collector: invalid Function generic owner")
	}
	// Type parameters are Static children. Admit the exact current terms
	// before touching the sidecar so a future/wrong-family child cannot be
	// deferred to Prepare.
	if !staticTypeParamsForOwner(StaticRoot{collector: c}, function, params) {
		return false
	}
	if err := c.static.FunctionContractGenerics(function, params); err != nil {
		c.fail(err)
		return false
	}
	return true
}

func (w FlowFunctionsWriter) SetFunctionReturns(function keyspace.Term, known bool, returns []keyspace.Term) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) {
		return rejectMutationf(c, "program/lower/collector: invalid Function return owner")
	}
	if !staticExistingNodeTerms(StaticRoot{collector: c}, returns) {
		return false
	}
	if err := c.static.FunctionContractReturns(function, known, returns); err != nil {
		c.fail(err)
		return false
	}
	return true
}
