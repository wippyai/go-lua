package valuesource

import (
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Literal returns the authored literal payload for one canonical term. Type
// values and all malformed or out-of-range terms are not literal payloads.
func Literal(input *program.Program, term keyspace.Term) (keyspace.Family, keyspace.LiteralValue, bool) {
	if input == nil || !input.Available() {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	index := int(ordinal - 1)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		issued, _, ok := input.Source().Literals().Nils().At(index)
		return keyspace.FamilyNil, keyspace.LiteralValue{}, ok && issued == term
	case keyspace.FamilyBool:
		issued, _, value, ok := input.Source().Literals().Bools().At(index)
		return keyspace.FamilyBool, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, ok && issued == term
	case keyspace.FamilyInteger:
		issued, _, value, ok := input.Source().Literals().Integers().At(index)
		return keyspace.FamilyInteger, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, ok && issued == term
	case keyspace.FamilyFloat:
		issued, _, value, ok := input.Source().Literals().Floats().At(index)
		return keyspace.FamilyFloat, keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: value}, ok && issued == term
	case keyspace.FamilyString:
		issued, _, value, ok := input.Source().Literals().Strings().At(index)
		return keyspace.FamilyString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, ok && issued == term
	default:
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
}
