package assembly

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Name and List retain source spelling only. Their canonical exact Key is
// resolved later from the live Source Preimage by dependent freeze passes.
func (c *Collector) Name(span source.Span, owner keyspace.Term, text string) keyspace.Term {
	if !validOwner(c, owner) || text == "" {
		if c != nil && !c.terminal {
			c.fail(errors.New("program/lower/collector: invalid source name key"))
		}
		return 0
	}
	if !c.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}) {
		return 0
	}
	term := c.mint(keyspace.FamilyKey, span)
	if term == 0 {
		return 0
	}
	c.source.AddKey(source.NameKey(owner, text))
	return term
}

func (c *Collector) List(span source.Span, owner keyspace.Term, ordinal int64) keyspace.Term {
	if !validOwner(c, owner) || ordinal <= 0 {
		if c != nil && !c.terminal {
			c.fail(errors.New("program/lower/collector: invalid source list key"))
		}
		return 0
	}
	if !c.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: ordinal}) {
		return 0
	}
	term := c.mint(keyspace.FamilyKey, span)
	if term == 0 {
		return 0
	}
	c.source.AddKey(source.ListKey(owner, ordinal))
	return term
}
