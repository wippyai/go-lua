package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Values records one authored fixed prefix and optional final open tail.
// Terms are kept in the Values vertical's dense pool; the range is the only
// relation between a Values row and its fixed members.
func (c *Collector) Values(span source.Span, owner keyspace.Term, fixed []keyspace.Term, tail keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyValues, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitValues(c.counts, term, owner, fixed, tail); err != nil {
		c.fail(err)
		return 0
	}
	return term
}
