package assembly

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

var errModuleRequestEmpty = errors.New("program/lower/collector: empty Module Import request")

// Import is the assembly-core orchestration boundary for Module observation.
// It proves the Call's exact first String request through Flow and Source,
// admits that request to Source's sole exact denominator, fills the reserved
// Import span, then submits the complete typed row to the Module owner.
func (c *Collector) Import(ordinal int, span source.Span, call keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	if ordinal < 0 || uint64(ordinal) >= uint64(c.counts[keyspace.FamilyImport]) ||
		!validTermInCounts(c, call) || keyspace.TermFamily(call) != keyspace.FamilyCall {
		rejectMutation(c, errors.New("program/lower/collector: invalid Module Import observation"))
		return 0
	}
	request, ok := c.moduleRequestTerm(call)
	if !ok {
		c.fail(errors.New("program/lower/collector: Module Import Call has no exact first String request"))
		return 0
	}
	raw, ok := c.exactLiteral(request)
	if !ok || raw.Kind != keyspace.LiteralString {
		c.fail(errors.New("program/lower/collector: Module Import request is not a Source String"))
		return 0
	}
	if raw.String == "" {
		c.fail(errModuleRequestEmpty)
		return 0
	}
	if !c.addExact(raw) {
		return 0
	}
	term := c.fillReservedImport(uint32(ordinal+1), span)
	if term == 0 {
		return 0
	}
	if err := c.module.Set(ordinal, term, call, request); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// SetImportAlias is the second and final Module orchestration boundary. Core
// proves the alias Cell against the live census; the Module owner records the
// one-shot authored alias state and never receives another owner store.
func (c *Collector) SetImportAlias(importTerm, alias keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if keyspace.TermFamily(importTerm) != keyspace.FamilyImport ||
		keyspace.TermOrdinal(importTerm) == 0 || keyspace.TermOrdinal(importTerm) > c.counts[keyspace.FamilyImport] ||
		(alias != 0 && !validFamilyTerm(c, alias, keyspace.FamilyCell)) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid module alias"))
	}
	slot := int(keyspace.TermOrdinal(importTerm) - 1)
	if err := c.module.SetAlias(slot, alias); err != nil {
		c.fail(err)
		return false
	}
	return true
}
