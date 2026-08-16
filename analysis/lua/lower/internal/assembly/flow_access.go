package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// exactCandidate derives raw exact-key provenance from an authored scalar
// Term. UnaryNeg closes over its already-authored operand; no AST walk or
// normalization occurs in the Access vertical.
func (c *Collector) exactCandidate(term keyspace.Term) (keyspace.LiteralValue, bool) {
	if c == nil || !validTermInCounts(c, term) {
		return keyspace.LiteralValue{}, false
	}
	operand := Term(0)
	if keyspace.TermFamily(term) == keyspace.FamilyUnary {
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 {
			return keyspace.LiteralValue{}, false
		}
		unary, ok := c.flow.UnaryAt(int(ordinal - 1))
		if !ok {
			return keyspace.LiteralValue{}, false
		}
		if unary.Op != kind.UnaryNeg {
			return keyspace.LiteralValue{}, false
		}
		operand = unary.Operand
	}
	return c.sourceExactCandidate(term, operand)
}

// LensExact records a static Name or exact scalar source operand. The raw
// candidate is derived from the already-authored key Term; callers cannot
// provide a second payload that could disagree with Source provenance.
func (c *Collector) LensExact(span source.Span, owner, base, key keyspace.Term, fieldKind kind.FieldKind) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	var candidatePresent bool
	if fieldKind == kind.FieldExact {
		candidate, present := c.exactCandidate(key)
		if !present {
			return rejectTermMutationf(c, "program/lower/collector: exact Lens key has no exact candidate")
		}
		candidatePresent = true
		if !c.addExactCandidate(candidate) {
			return 0
		}
	}
	term := c.mint(keyspace.FamilyLensExact, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitExactLens(c.counts, term, owner, base, key, fieldKind, candidatePresent); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// LensKey records an evaluated base followed by an evaluated dynamic key.
func (c *Collector) LensKey(span source.Span, owner, base, key keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyLensKey, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitDynamicLens(c.counts, term, owner, base, key); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// addExactCandidate admits raw authored provenance directly to Source's sole
// exact denominator. Flow retains no parallel candidate row or map. NaN has
// no storable exact identity and is therefore an intentional no-op.
func (c *Collector) addExactCandidate(raw keyspace.LiteralValue) bool {
	if !mutationReady(c) {
		return false
	}
	// NaN is an authored exact operand but has no lawful Source atom.
	if raw.Kind == keyspace.LiteralFloat && !validRawExactCandidate(raw) {
		return true
	}
	if !validRawExactCandidate(raw) {
		return rejectMutationf(c, "program/lower/collector: invalid Flow exact candidate")
	}
	return c.addExact(raw)
}
