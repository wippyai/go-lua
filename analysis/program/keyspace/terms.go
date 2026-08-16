package keyspace

// Family is the stable closed family of canonical Program Terms. The numeric
// values are part of Program identity and artifact replay; they are never
// declaration-order or package-registration ordinals.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyNil
	FamilyBool
	FamilyInteger
	FamilyFloat
	FamilyString
	FamilyValues
	FamilyLensExact
	FamilyLensKey
	FamilyReturn
	FamilyBreak
	FamilyLabel
	FamilyGoto
	FamilyBody
	FamilyCell
	FamilyRead
	FamilyVararg
	FamilyUnary
	FamilyBinary
	FamilySelect
	FamilyBind
	FamilyAssign
	FamilyFunction
	FamilyCall
	FamilyBranch
	FamilyLoop
	FamilyTable
	FamilyKey
	FamilyTypeAlias
	FamilyTypeInterface
	FamilyTypeParam
	FamilyTypePrimitive
	FamilyTypeLiteral
	FamilyTypeOptional
	FamilyTypeUnion
	FamilyTypeIntersection
	FamilyTypeRef
	FamilyTypeGeneric
	FamilyTypeArray
	FamilyTypeMap
	FamilyTypeRecord
	FamilyTypeField
	FamilyTypeFunction
	FamilyTypeAsserts
	FamilyDeclaredType
	FamilyTypePublication
	FamilyTypeValue
	FamilyValueClaim
	FamilyAnnotation
	FamilyTypeOf
	FamilyTypeKeyOf
	FamilyTypeIndexAccess
	FamilyTypeConditional
	FamilyWrite
	FamilyTableField
	FamilyOutcome
	FamilyControlFault
	FamilyImport
	FamilyCount
)

const (
	termFamilyBits = 8
	termFamilyMask = uint32(1<<termFamilyBits - 1)
	// MaxTermOrdinal is the largest dense one-based ordinal representable by
	// one canonical Term family.
	MaxTermOrdinal = uint32(1<<(32-termFamilyBits) - 1)
)

// MakeTerm returns the canonical Term for one valid family and one-based dense
// ordinal. Invalid families, zero ordinals, and overflow fail closed as zero.
func MakeTerm(family Family, ordinal uint32) Term {
	if family <= FamilyInvalid || family >= FamilyCount || ordinal == 0 || ordinal > MaxTermOrdinal {
		return 0
	}
	return Term(ordinal<<termFamilyBits | uint32(family))
}

// TermFamily returns the encoded family. Zero and malformed families are
// reported as FamilyInvalid.
func TermFamily(term Term) Family {
	family := Family(uint32(term) & termFamilyMask)
	if term == 0 || TermOrdinal(term) == 0 || family <= FamilyInvalid || family >= FamilyCount {
		return FamilyInvalid
	}
	return family
}

// ValidTerm reports exact family membership and an ordinal within count.
func ValidTerm(term Term, family Family, count int) bool {
	ordinal := TermOrdinal(term)
	return count >= 0 && TermFamily(term) == family && ordinal != 0 && uint64(ordinal) <= uint64(count)
}

// TermOrdinalFits reports whether count can be represented by one Term family.
func TermOrdinalFits(count int) bool {
	return count >= 0 && uint64(count) <= uint64(MaxTermOrdinal)
}
