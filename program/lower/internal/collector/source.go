package collector

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// mint is the sole authored Term allocator.  Source's Outcome family is
// derived by Flow's finalization and therefore cannot be minted here.  A
// failed allocation poisons the cursor; callers cannot continue with a
// partially authoritative construction.
func (c *Collector) mint(family keyspace.Family, span source.Span) Term {
	if c == nil {
		return 0
	}
	if c.err != nil || c.terminal {
		return 0
	}
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		family == keyspace.FamilyOutcome || family == keyspace.FamilyImport {
		c.fail(fmt.Errorf("program/lower/collector: invalid authored family %d", family))
		return 0
	}
	if !validSpan(c, span) {
		c.fail(errors.New("program/lower/collector: invalid source span"))
		return 0
	}
	ordinal := uint64(c.counts[family]) + 1
	if ordinal > uint64(keyspace.MaxTermOrdinal) {
		c.fail(fmt.Errorf("program/lower/collector: family %d ordinal overflow", family))
		return 0
	}
	term := keyspace.MakeTerm(family, uint32(ordinal))
	if term == 0 {
		c.fail(fmt.Errorf("program/lower/collector: failed to mint family %d", family))
		return 0
	}
	c.counts[family] = uint32(ordinal)
	c.spans[family] = append(c.spans[family], span)
	return term
}

// addExact records a raw exact-key candidate.  It deliberately performs no
// normalization, deduplication, sorting, or numeric Key assignment.  Source
// Build owns that quotient and is the sole place where Key handles are made.
func (c *Collector) addExact(raw keyspace.LiteralValue) bool {
	if c == nil || c.err != nil || c.terminal {
		return false
	}
	if !validRawExactCandidate(raw) {
		c.fail(errors.New("program/lower/collector: invalid exact-key candidate"))
		return false
	}
	c.source.exact = append(c.source.exact, cloneLiteral(raw))
	return true
}

func validRawExactCandidate(value keyspace.LiteralValue) bool {
	switch value.Kind {
	case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
		return true
	case keyspace.LiteralFloat:
		return !math.IsNaN(math.Float64frombits(value.FloatBits))
	default:
		return false
	}
}

func cloneLiteral(value keyspace.LiteralValue) keyspace.LiteralValue {
	if value.String != "" {
		// Force ownership of the payload even when the caller's string was
		// assembled from a mutable byte buffer.
		value.String = string([]byte(value.String))
	}
	return value
}

// fillReservedImport writes one pre-censused Import span. Import ordinals are
// not minted during visitation and therefore remain stable under traversal
// order. The returned Term is the reserved canonical identity.

func (w ModuleRoot) fillReservedImport(ordinal uint32, span source.Span) Term {
	c := w.collector
	if c == nil || c.err != nil || c.terminal || ordinal == 0 || ordinal > c.counts[keyspace.FamilyImport] ||
		ordinal-1 >= uint32(len(c.spans[keyspace.FamilyImport])) ||
		!validSpan(c, span) {
		if c != nil && c.err == nil {
			c.fail(errors.New("program/lower/collector: invalid reserved Import"))
		}
		return 0
	}
	at := ordinal - 1
	if at >= uint32(len(c.source.importFilled)) || c.source.importFilled[at] {
		c.fail(errors.New("program/lower/collector: reserved Import filled more than once"))
		return 0
	}
	c.spans[keyspace.FamilyImport][at] = span
	c.source.importFilled[at] = true
	return keyspace.MakeTerm(keyspace.FamilyImport, ordinal)
}

func validOwner(c *Collector, owner Term) bool {
	if c == nil || c.err != nil || !validBody(c, owner) {
		if c != nil && c.err == nil && !c.terminal {
			c.fail(errors.New("program/lower/collector: invalid Body owner"))
		}
		return false
	}
	return true
}

func validBody(c *Collector, body Term) bool {
	if c == nil || c.err != nil || c.terminal || !validBodyTerm(body) || keyspace.TermOrdinal(body) > c.counts[keyspace.FamilyBody] {
		return false
	}
	ordinal := keyspace.TermOrdinal(body) - 1
	return uint64(ordinal) < uint64(len(c.source.bodies)) && uint64(ordinal) < uint64(len(c.source.filled)) &&
		c.source.bodies[ordinal].Body == body
}

func validFamilyTerm(c *Collector, term Term, family keyspace.Family) bool {
	return c != nil && c.err == nil && !c.terminal && keyspace.TermFamily(term) == family &&
		keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= c.counts[family]
}

// sourceDirectFamily is the exact family boundary for authored Body roots.
// It mirrors Source's canonical direct-body denominator without admitting a
// term merely because its ordinal is present in the Collector census.
func sourceDirectFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyLabel, keyspace.FamilyControlFault,
		keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface:
		return true
	default:
		return false
	}
}

func validDirectBodyTerm(c *Collector, body, term Term) bool {
	if c == nil || !validBody(c, body) || !validTermInCounts(c, term) || !sourceDirectFamily(keyspace.TermFamily(term)) {
		return false
	}
	if keyspace.TermFamily(term) == keyspace.FamilyBody {
		// A nested lexical Body is a direct source root of its enclosing Body,
		// but a Body cannot contain itself. The completed Flow/source forest
		// proves which existing Body is the lexical child; admission must not
		// fabricate a parallel parent owner here.
		return term != body
	}
	owner, ok := sourceDirectTermOwner(c, term)
	return ok && owner == body
}

// sourceDirectTermOwner is the one owner-role authority used by SetBody.
// Every direct family with a row-local Body owner is resolved here, so the
// admission boundary cannot drift through several subtly different switches.
func sourceDirectTermOwner(c *Collector, term Term) (Term, bool) {
	if c == nil || !validTermInCounts(c, term) {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	index := int(ordinal - 1)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBind:
		if index < len(c.flow.storage.binds) {
			return c.flow.storage.binds[index].Owner, true
		}
	case keyspace.FamilyAssign:
		if index < len(c.flow.storage.assigns) {
			return c.flow.storage.assigns[index].Owner, true
		}
	case keyspace.FamilyCall:
		if index < len(c.flow.calls.calls) {
			return c.flow.calls.calls[index].Owner, true
		}
	case keyspace.FamilyBranch:
		if index < len(c.flow.control.branches) {
			return c.flow.control.branches[index].Owner, true
		}
	case keyspace.FamilyLoop:
		if index < len(c.flow.control.loops) {
			return c.flow.control.loops[index].Owner, true
		}
	case keyspace.FamilyReturn:
		if index < len(c.flow.control.returns) {
			return c.flow.control.returns[index].Owner, true
		}
	case keyspace.FamilyBreak:
		if index < len(c.flow.control.breaks) {
			return c.flow.control.breaks[index].Owner, true
		}
	case keyspace.FamilyGoto:
		if index < len(c.flow.control.gotos) {
			return c.flow.control.gotos[index].Owner, true
		}
	case keyspace.FamilyLabel:
		if index < len(c.flow.control.labels) {
			return c.flow.control.labels[index].Owner, true
		}
	case keyspace.FamilyControlFault:
		if index < len(c.source.faults) {
			return c.source.faults[index].Owner, true
		}
	case keyspace.FamilyTypeAlias:
		if index < len(c.static.aliases) {
			return c.static.aliases[index].owner, true
		}
	case keyspace.FamilyTypeInterface:
		if index < len(c.static.interfaces) {
			return c.static.interfaces[index].owner, true
		}
	}
	return 0, false
}
