package assembly

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (c *Collector) DeclareFunction(span source.Span, owner keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyFunction, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitFunction(c.counts, term, owner); err != nil {
		c.fail(err)
		return 0
	}
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
func (c *Collector) FillFunction(function, body keyspace.Term, formals []keyspace.Term, vararg keyspace.Term, captures []programflow.Capture) bool {
	if !mutationReady(c) {
		return false
	}
	sourceCellsAlreadyOrdered := false
	for _, formal := range formals {
		if sourceCellAlreadyOrdered(c, formal) {
			sourceCellsAlreadyOrdered = true
			break
		}
	}
	if err := c.flow.AdmitFunctionFill(c.counts, function, body, vararg, formals, captures, sourceCellsAlreadyOrdered); err != nil {
		c.fail(err)
		return false
	}
	if !c.FunctionFormals(function, formals) {
		return false
	}
	return true
}

func (c *Collector) SetFunctionGenerics(function keyspace.Term, params []keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) {
		return rejectMutationf(c, "program/lower/collector: invalid Function generic owner")
	}
	// Type parameters are Static children. Admit the exact current terms
	// before touching the sidecar so a future/wrong-family child cannot be
	// deferred to Publish.
	if !staticTypeParamsForOwner(c, function, params) {
		return false
	}
	if err := c.static.FunctionContractGenerics(function, params); err != nil {
		c.fail(err)
		return false
	}
	return true
}

func (c *Collector) SetFunctionReturns(function keyspace.Term, known bool, returns []keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) {
		return rejectMutationf(c, "program/lower/collector: invalid Function return owner")
	}
	if !staticExistingNodeTerms(c, returns) {
		return false
	}
	if err := c.static.FunctionContractReturns(function, known, returns); err != nil {
		c.fail(err)
		return false
	}
	return true
}
