package assembly

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
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
	if err := c.source.AddExact(raw); err != nil {
		c.fail(err)
		return false
	}
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

// fillReservedImport writes one pre-censused Import span. Import ordinals are
// not minted during visitation and therefore remain stable under traversal
// order. The returned Term is the reserved canonical identity.

func (c *Collector) fillReservedImport(ordinal uint32, span source.Span) Term {
	if c == nil || c.err != nil || c.terminal || ordinal == 0 || ordinal > c.counts[keyspace.FamilyImport] ||
		ordinal-1 >= uint32(len(c.spans[keyspace.FamilyImport])) ||
		!validSpan(c, span) {
		if c != nil && c.err == nil {
			c.fail(errors.New("program/lower/collector: invalid reserved Import"))
		}
		return 0
	}
	at := ordinal - 1
	if !c.source.FillImport(ordinal, span) {
		c.fail(errors.New("program/lower/collector: reserved Import filled more than once"))
		return 0
	}
	c.spans[keyspace.FamilyImport][at] = span
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
	row, ok := c.source.BodyAt(int(ordinal))
	return ok && row.Body == body
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
	case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall,
		keyspace.FamilyBranch, keyspace.FamilyLoop, keyspace.FamilyReturn,
		keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyLabel:
		return c.flow.OwnerAt(keyspace.TermFamily(term), index)
	case keyspace.FamilyControlFault:
		if row, ok := c.source.FaultAt(index); ok {
			return row.Owner, true
		}
	case keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface:
		return c.static.OwnerAt(keyspace.TermFamily(term), index)
	}
	return 0, false
}
