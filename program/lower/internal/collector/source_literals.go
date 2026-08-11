package collector

import (
	"math"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Nil, Bool, Integer, FloatBits, and String each mint one fresh authored
// literal occurrence. Equal payloads remain distinct Terms.
func (l SourceLiterals) Nil(span source.Span, owner Term) Term {
	c := l.collector
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyNil, span)
	if term == 0 {
		return 0
	}
	c.source.nil = append(c.source.nil, source.NilLiteral{Owner: owner})
	return term
}

func (l SourceLiterals) Bool(span source.Span, owner Term, value bool) Term {
	c := l.collector
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyBool, span)
	if term == 0 {
		return 0
	}
	c.source.bool = append(c.source.bool, source.BoolLiteral{Owner: owner, Value: value})
	return term
}

func (l SourceLiterals) Integer(span source.Span, owner Term, value int64) Term {
	c := l.collector
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyInteger, span)
	if term == 0 {
		return 0
	}
	c.source.integer = append(c.source.integer, source.IntegerLiteral{Owner: owner, Value: value})
	return term
}

func (l SourceLiterals) FloatBits(span source.Span, owner Term, bits uint64) Term {
	c := l.collector
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyFloat, span)
	if term == 0 {
		return 0
	}
	c.source.float = append(c.source.float, source.FloatLiteral{Owner: owner, Bits: bits})
	return term
}

// Float is a convenience for lowerer code that has already decoded a parser
// float. FloatBits remains the exact-authority operation.
func (l SourceLiterals) Float(span source.Span, owner Term, value float64) Term {
	return l.FloatBits(span, owner, math.Float64bits(value))
}

func (l SourceLiterals) String(span source.Span, owner Term, value string) Term {
	c := l.collector
	if !validOwner(c, owner) {
		return 0
	}
	term := c.mint(keyspace.FamilyString, span)
	if term == 0 {
		return 0
	}
	c.source.string = append(c.source.string, source.StringLiteral{Owner: owner, Value: string([]byte(value))})
	return term
}

// exactLiteral returns the raw storable candidate represented by a Source
// scalar literal. Nil has no storable exact-key payload, and NaN is rejected
// rather than smuggled into the denominator. The returned value is not a Key.
func (l SourceLiterals) exactLiteral(term Term) (keyspace.LiteralValue, bool) {
	c := l.collector
	if c == nil || c.err != nil || !validTermInCounts(c, term) {
		return keyspace.LiteralValue{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		if ordinal > uint32(len(c.source.bool)) {
			return keyspace.LiteralValue{}, false
		}
		return keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: c.source.bool[ordinal-1].Value}, true
	case keyspace.FamilyInteger:
		if ordinal > uint32(len(c.source.integer)) {
			return keyspace.LiteralValue{}, false
		}
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: c.source.integer[ordinal-1].Value}, true
	case keyspace.FamilyFloat:
		if ordinal > uint32(len(c.source.float)) {
			return keyspace.LiteralValue{}, false
		}
		value := keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: c.source.float[ordinal-1].Bits}
		return value, validRawExactCandidate(value)
	case keyspace.FamilyString:
		if ordinal > uint32(len(c.source.string)) {
			return keyspace.LiteralValue{}, false
		}
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: c.source.string[ordinal-1].Value}, true
	default:
		return keyspace.LiteralValue{}, false
	}
}

// unaryNegExact applies the closed static FieldExact UnaryNeg grammar to an
// already-authored scalar literal. The caller supplies the Unary term only
// for its proof context; no recursive expression walk or second authority is
// introduced here. Integer minimum is represented as a float exactly as the
// Source/Flow semantic law requires.
func (l SourceLiterals) unaryNegExact(unary, operand Term) (keyspace.LiteralValue, bool) {
	c := l.collector
	if c == nil || c.err != nil || keyspace.TermFamily(unary) != keyspace.FamilyUnary ||
		!validTermInCounts(c, unary) {
		return keyspace.LiteralValue{}, false
	}
	value, ok := l.exactLiteral(operand)
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
func (l SourceLiterals) exactCandidate(term, unaryOperand Term) (keyspace.LiteralValue, bool) {
	if keyspace.TermFamily(term) == keyspace.FamilyUnary {
		return l.unaryNegExact(term, unaryOperand)
	}
	return l.exactLiteral(term)
}
