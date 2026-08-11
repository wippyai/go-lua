package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func (w FlowCallsWriter) DeclareCall(span source.Span, owner, callee, receiver, actuals keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !flowrole.ValueOccurrence(c.counts, callee) || (receiver != 0 && !methodCalleeAdmission(c, owner, callee, receiver)) || !validFamilyTerm(c, actuals, keyspace.FamilyValues) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Call admission")
	}
	term := c.mint(keyspace.FamilyCall, span)
	if term == 0 {
		return 0
	}
	c.flow.calls.calls = append(c.flow.calls.calls, flow.Call{Owner: owner, Callee: callee, Receiver: receiver, Actuals: actuals})
	// Call declaration atomically coordinates its executable row with the one
	// Static contract sidecar; Static remains the sidecar owner.
	if err := c.static.CallContractPlaceholder(term); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// methodCalleeAdmission proves the row-local method-call shape. The generic
// ValueOccurrence role only says that receiver is evaluable; it cannot prove
// that callee is the matching named lens Read for this owner and receiver.
func methodCalleeAdmission(c *Collector, owner, callee, receiver keyspace.Term) bool {
	if c == nil || !flowrole.ValueOccurrence(c.counts, receiver) || !validFamilyTerm(c, callee, keyspace.FamilyRead) {
		return false
	}
	readOrdinal := keyspace.TermOrdinal(callee)
	if readOrdinal == 0 || uint64(readOrdinal) > uint64(len(c.flow.storage.reads)) {
		return false
	}
	read := c.flow.storage.reads[readOrdinal-1]
	if read.Owner != owner || !validFamilyTerm(c, read.Source, keyspace.FamilyLensExact) {
		return false
	}
	lensOrdinal := keyspace.TermOrdinal(read.Source)
	if lensOrdinal == 0 || uint64(lensOrdinal) > uint64(len(c.flow.access.exactLenses)) {
		return false
	}
	lens := c.flow.access.exactLenses[lensOrdinal-1]
	return lens.Owner == owner && lens.Base == receiver && lens.Kind == kind.FieldName
}

// SetCallTypeArgs atomically fills Call's Static sidecar. Flow retains no
// duplicate type-argument pool.
func (w FlowCallsWriter) SetCallTypeArgs(call keyspace.Term, arguments []keyspace.Term) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, call, keyspace.FamilyCall) {
		return rejectMutationf(c, "program/lower/collector: invalid Call type-argument owner")
	}
	if !staticExistingNodeTerms(StaticRoot{collector: c}, arguments) {
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
func (w FlowCallsWriter) moduleRequestTerm(call keyspace.Term) (keyspace.Term, bool) {
	c := w.collector
	if c == nil || !validFamilyTerm(c, call, keyspace.FamilyCall) {
		return 0, false
	}
	if keyspace.TermOrdinal(call) > uint32(len(c.flow.calls.calls)) {
		return 0, false
	}
	callRow := c.flow.calls.calls[keyspace.TermOrdinal(call)-1]
	values, ok := valuesRowAdmission(c, callRow.Actuals)
	if !ok || values.Tail != 0 || values.Fixed.Start > values.Fixed.End || values.Fixed.End-values.Fixed.Start != 1 {
		return 0, false
	}
	if uint64(values.Fixed.End) > uint64(len(c.flow.values.valueTerms)) {
		return 0, false
	}
	request := c.flow.values.valueTerms[values.Fixed.Start]
	if !validFamilyTerm(c, request, keyspace.FamilyString) {
		return 0, false
	}
	return request, true
}
