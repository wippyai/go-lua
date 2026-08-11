package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Values records one authored fixed prefix and optional final open tail.
// Terms are kept in the Values vertical's dense pool; the range is the only
// relation between a Values row and its fixed members.
func (w FlowValuesWriter) Values(span source.Span, owner keyspace.Term, fixed []keyspace.Term, tail keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Values owner")
	}
	for _, term := range fixed {
		if !flowrole.ValueOccurrence(c.counts, term) {
			return rejectTermMutationf(c, "program/lower/collector: invalid Values fixed operand")
		}
	}
	if tail != 0 && !flowrole.OpenOccurrence(c.counts, tail) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Values tail")
	}
	r, ok := appendTerms(&c.flow.values.valueTerms, fixed)
	if !ok {
		return rejectTermMutationf(c, "program/lower/collector: Values fixed operand range overflow")
	}
	term := c.mint(keyspace.FamilyValues, span)
	if term == 0 {
		return 0
	}
	c.flow.values.values = append(c.flow.values.values, flow.Value{Owner: owner, Fixed: r, Tail: tail})
	return term
}
