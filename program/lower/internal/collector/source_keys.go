package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Name and List retain source spelling only. Their canonical exact Key is
// resolved later from the live Source Preimage by dependent freeze passes.
func (k SourceKeys) Name(span source.Span, owner Term, text string) Term {
	c := k.collector
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
	c.source.keys = append(c.source.keys, source.NameKey(owner, text))
	return term
}

func (k SourceKeys) List(span source.Span, owner Term, ordinal int64) Term {
	c := k.collector
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
	c.source.keys = append(c.source.keys, source.ListKey(owner, ordinal))
	return term
}
