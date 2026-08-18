package assembly

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (c *Collector) DeclareCall(span source.Span, owner, callee, receiver, actuals keyspace.Term, name string) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyCall, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitCall(c.counts, term, owner, callee, receiver, actuals); err != nil {
		c.fail(err)
		return 0
	}
	if name != "" && !c.source.AddCallSpelling(term, name) {
		c.fail(errors.New("program/lower/collector: could not attach Call spelling"))
		return 0
	}
	// Call declaration atomically coordinates its executable row with the one
	// Static contract sidecar; Static remains the sidecar owner.
	if err := c.static.CallContractPlaceholder(term); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// SetCallTypeArgs atomically fills Call's Static sidecar. Flow retains no
// duplicate type-argument pool.
func (c *Collector) SetCallTypeArgs(call keyspace.Term, arguments []keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, call, keyspace.FamilyCall) {
		return rejectMutationf(c, "program/lower/collector: invalid Call type-argument owner")
	}
	if !staticExistingNodeTerms(c, arguments) {
		return false
	}
	if err := c.static.CallContractArguments(call, arguments); err != nil {
		c.fail(err)
		return false
	}
	return true
}

// moduleRequestTerm is the narrow Values witness used at the binder-censused
// Module observation boundary. It accepts exactly one fixed member and no open
// tail, then returns that already-authored String Term. No other caller may
// infer a Module request from generic Call/Values shape.
func (c *Collector) moduleRequestTerm(call keyspace.Term) (keyspace.Term, bool) {
	return c.flow.ModuleRequest(c.counts, call)
}
