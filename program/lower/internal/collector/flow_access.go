package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// exactCandidate derives raw exact-key provenance from an authored scalar
// Term. UnaryNeg closes over its already-authored operand; no AST walk or
// normalization occurs in the Access vertical.
func (w FlowAccessWriter) exactCandidate(term keyspace.Term) (keyspace.LiteralValue, bool) {
	c := w.collector
	if c == nil || !validTermInCounts(c, term) {
		return keyspace.LiteralValue{}, false
	}
	operand := Term(0)
	if keyspace.TermFamily(term) == keyspace.FamilyUnary {
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || ordinal > uint32(len(c.flow.operators.unaries)) {
			return keyspace.LiteralValue{}, false
		}
		unary := c.flow.operators.unaries[ordinal-1]
		if unary.Op != kind.UnaryNeg {
			return keyspace.LiteralValue{}, false
		}
		operand = unary.Operand
	}
	return c.Source().Literals().exactCandidate(term, operand)
}

// LensExact records a static Name or exact scalar source operand. The raw
// candidate is derived from the already-authored key Term; callers cannot
// provide a second payload that could disagree with Source provenance.
func (w FlowAccessWriter) LensExact(span source.Span, owner, base, key keyspace.Term, fieldKind kind.FieldKind) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !flowrole.ValueOccurrence(c.counts, base) ||
		!flowrole.FieldSourceFamily(c.counts, key, fieldKind) ||
		(fieldKind != kind.FieldName && fieldKind != kind.FieldExact) {
		return rejectTermMutationf(c, "program/lower/collector: invalid exact Lens admission")
	}
	if fieldKind == kind.FieldExact {
		candidate, present := w.exactCandidate(key)
		if !present {
			return rejectTermMutationf(c, "program/lower/collector: exact Lens key has no exact candidate")
		}
		if !w.addExactCandidate(candidate) {
			return 0
		}
	}
	term := c.mint(keyspace.FamilyLensExact, span)
	if term == 0 {
		return 0
	}
	c.flow.access.exactLenses = append(c.flow.access.exactLenses, flow.ExactLens{
		Owner: owner, Base: base, Source: key, Kind: fieldKind,
	})
	return term
}

// LensKey records an evaluated base followed by an evaluated dynamic key.
func (w FlowAccessWriter) LensKey(span source.Span, owner, base, key keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !flowrole.ValueOccurrence(c.counts, base) || !flowrole.ValueOccurrence(c.counts, key) {
		return rejectTermMutationf(c, "program/lower/collector: invalid dynamic Lens admission")
	}
	term := c.mint(keyspace.FamilyLensKey, span)
	if term == 0 {
		return 0
	}
	c.flow.access.dynamicLenses = append(c.flow.access.dynamicLenses, flow.DynamicLens{Owner: owner, Base: base, Key: key})
	return term
}

// addExactCandidate admits raw authored provenance directly to Source's sole
// exact denominator. Flow retains no parallel candidate row or map. NaN has
// no storable exact identity and is therefore an intentional no-op.
func (w FlowAccessWriter) addExactCandidate(raw keyspace.LiteralValue) bool {
	c := w.collector
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
