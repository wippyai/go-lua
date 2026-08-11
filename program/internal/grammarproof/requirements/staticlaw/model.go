// Package staticlaw owns exact source-to-Program laws for the closed static
// syntax family. It uses Program's existing sealed static authority; it does
// not create another type tree or an analysis type vocabulary.
package staticlaw

// Family is one parser-authored static constructor with a distinct public
// Program relation.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyPrimitive
	FamilyLiteral
	FamilyOptional
	FamilyUnion
	FamilyIntersection
	FamilyTypeRef
	FamilyGeneric
	FamilyArray
	FamilyMap
	FamilyRecord
	FamilySignature
	FamilyAssertion
	FamilyTypeOf
	FamilyKeyOf
	FamilyIndexAccess
	FamilyConditional
	FamilyAnnotated
)

// Requirements is the independent semantic denominator. Every row is a
// parser-reachable static constructor, not a fixture-derived Program count.
func Requirements() []Family {
	return []Family{
		FamilyPrimitive,
		FamilyLiteral,
		FamilyOptional,
		FamilyUnion,
		FamilyIntersection,
		FamilyTypeRef,
		FamilyGeneric,
		FamilyArray,
		FamilyMap,
		FamilyRecord,
		FamilySignature,
		FamilyAssertion,
		FamilyTypeOf,
		FamilyKeyOf,
		FamilyIndexAccess,
		FamilyConditional,
		FamilyAnnotated,
	}
}
