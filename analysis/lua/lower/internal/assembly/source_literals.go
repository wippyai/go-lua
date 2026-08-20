package assembly

import (
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Nil, Bool, Integer, FloatBits, and String each mint one fresh authored
// literal occurrence. Equal payloads remain distinct Terms.
func (c *Collector) Nil(span source.Span, owner keyspace.Term) keyspace.Term {
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyNil, span)
	if term == 0 {
		return 0
	}
	c.source.Nil = append(c.source.Nil, source.NilLiteral{Owner: owner})
	return term
}

func (c *Collector) Bool(span source.Span, owner keyspace.Term, value bool) keyspace.Term {
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyBool, span)
	if term == 0 {
		return 0
	}
	c.source.Bool = append(c.source.Bool, source.BoolLiteral{Owner: owner, Value: value})
	return term
}

func (c *Collector) Integer(span source.Span, owner keyspace.Term, value int64) keyspace.Term {
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyInteger, span)
	if term == 0 {
		return 0
	}
	c.source.Integer = append(c.source.Integer, source.IntegerLiteral{Owner: owner, Value: value})
	return term
}

func (c *Collector) FloatBits(span source.Span, owner keyspace.Term, bits uint64) keyspace.Term {
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyFloat, span)
	if term == 0 {
		return 0
	}
	c.source.Float = append(c.source.Float, source.FloatLiteral{Owner: owner, Bits: bits})
	return term
}

// Float is a convenience for lowerer code that has already decoded a parser
// float. FloatBits remains the exact-authority operation.
func (c *Collector) Float(span source.Span, owner keyspace.Term, value float64) keyspace.Term {
	return c.FloatBits(span, owner, math.Float64bits(value))
}

func (c *Collector) String(span source.Span, owner keyspace.Term, value string) keyspace.Term {
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyString, span)
	if term == 0 {
		return 0
	}
	c.source.String = append(c.source.String, source.StringLiteral{Owner: owner, Value: string([]byte(value))})
	return term
}

// exactLiteral returns the raw storable candidate represented by a Source
// scalar literal. Nil has no storable exact-key payload, and NaN is rejected
// rather than smuggled into the denominator. The returned value is not a Key.
func (c *Collector) exactLiteral(term keyspace.Term) (keyspace.LiteralValue, bool) {
	if c == nil || c.err != nil || !validTermInCounts(c, term) {
		return keyspace.LiteralValue{}, false
	}
	if term == 0 {
		return keyspace.LiteralValue{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.LiteralValue{}, false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		if int(ordinal) <= len(c.source.Bool) {
			return keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: c.source.Bool[ordinal-1].Value}, true
		}
	case keyspace.FamilyInteger:
		if int(ordinal) <= len(c.source.Integer) {
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: c.source.Integer[ordinal-1].Value}, true
		}
	case keyspace.FamilyFloat:
		if int(ordinal) <= len(c.source.Float) {
			value := keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: c.source.Float[ordinal-1].Bits}
			return value, validRawExactCandidate(value)
		}
	case keyspace.FamilyString:
		if int(ordinal) <= len(c.source.String) {
			return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: c.source.String[ordinal-1].Value}, true
		}
	}
	return keyspace.LiteralValue{}, false
}

// unaryNegExact applies the closed static FieldExact UnaryNeg grammar to an
// already-authored scalar literal. The caller supplies the Unary term only
// for its proof context; no recursive expression walk or second authority is
// introduced here. Integer minimum is represented as a float exactly as the
// Source/Flow semantic law requires.
func (c *Collector) unaryNegExact(unary, operand keyspace.Term) (keyspace.LiteralValue, bool) {
	if c == nil || c.err != nil || keyspace.TermFamily(unary) != keyspace.FamilyUnary ||
		!validTermInCounts(c, unary) {
		return keyspace.LiteralValue{}, false
	}
	value, ok := c.exactLiteral(operand)
	if !ok {
		return keyspace.LiteralValue{}, false
	}
	switch value.Kind {
	case keyspace.LiteralInteger:
		if value.Integer == math.MinInt64 {
			return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-float64(value.Integer))}, true
		}
		value.Integer = -value.Integer
		return value, true
	case keyspace.LiteralFloat:
		value.FloatBits = math.Float64bits(-math.Float64frombits(value.FloatBits))
		return value, validRawExactCandidate(value)
	default:
		return keyspace.LiteralValue{}, false
	}
}

// exactCandidate is a compact Flow-facing spelling: scalar terms are
// resolved directly; UnaryNeg terms must provide their already-authored
// operand. It never inspects or infers Flow rows.
func (c *Collector) sourceExactCandidate(term, unaryOperand keyspace.Term) (keyspace.LiteralValue, bool) {
	if keyspace.TermFamily(term) == keyspace.FamilyUnary {
		return c.unaryNegExact(term, unaryOperand)
	}
	return c.exactLiteral(term)
}
