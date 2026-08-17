package returns

import "github.com/wippyai/go-lua/domain/effect/capability"

// ReturnTypeKind identifies the vocabulary variant of a return transform. It
// classifies the value and pointer spellings of a transform that is present;
// use the As... helpers to reach the concrete payload behind a classified term.
type ReturnTypeKind uint8

const (
	ReturnTypeUnknown ReturnTypeKind = iota
	ReturnTypeSameAs
	ReturnTypeElementOf
	ReturnTypeOptionalElementOf
	ReturnTypeCallbackReturn
	ReturnTypeArrayOfCallbackReturn
	ReturnTypeTypeProjection
	ReturnTypeConditionalType
	returnTypeKindLimit
)

// ReturnTypeKindCount is the size of the closed vocabulary. The ordinals are
// dense from ReturnTypeSameAs, so a consumer indexes by kind without a lookup.
const ReturnTypeKindCount = int(returnTypeKindLimit) - 1

// Valid reports membership in the closed vocabulary. ReturnTypeUnknown is the
// answer for a transform outside it and is not a member.
func (kind ReturnTypeKind) Valid() bool {
	return kind > ReturnTypeUnknown && kind < returnTypeKindLimit
}

// ReturnTypeKinds is the vocabulary catalog in ordinal order. It is the one
// enumeration of the variants this package owns, so a consumer that visits,
// serializes, or declares every variant projects it instead of restating the
// member list. The catalog is returned by value and costs no allocation to
// range over.
func ReturnTypeKinds() [ReturnTypeKindCount]ReturnTypeKind {
	return [ReturnTypeKindCount]ReturnTypeKind{
		ReturnTypeSameAs, ReturnTypeElementOf, ReturnTypeOptionalElementOf,
		ReturnTypeCallbackReturn, ReturnTypeArrayOfCallbackReturn,
		ReturnTypeTypeProjection, ReturnTypeConditionalType,
	}
}

// returnTypeCapabilityIDs classifies each variant as the capability the catalog
// audits it under, indexed by the variant's dense ordinal. The kind enumeration
// above is the vocabulary, so the classification rides it rather than restating
// the member list; a variant added without a capability lands on the empty
// string and the bijection law reports it.
var returnTypeCapabilityIDs = [returnTypeKindLimit]string{
	ReturnTypeSameAs:                capability.ReturnsReturnSameAs,
	ReturnTypeElementOf:             capability.ReturnsReturnElementOf,
	ReturnTypeOptionalElementOf:     capability.ReturnsReturnOptionalElementOf,
	ReturnTypeCallbackReturn:        capability.ReturnsReturnCallbackReturn,
	ReturnTypeArrayOfCallbackReturn: capability.ReturnsReturnArrayOfCallbackReturn,
	ReturnTypeTypeProjection:        capability.ReturnsReturnTypeProjection,
	ReturnTypeConditionalType:       capability.ReturnsReturnConditionalType,
}

// CapabilityID answers the audited capability this variant is classified as. A
// kind outside the closed vocabulary answers the empty string.
func (kind ReturnTypeKind) CapabilityID() string {
	if !kind.Valid() {
		return ""
	}
	return returnTypeCapabilityIDs[kind]
}

// KindOfReturnType reports the vocabulary variant of transform. A term outside
// the vocabulary, including an absent one, answers ReturnTypeUnknown: a typed
// nil pointer names a variant in its Go type but carries no transform, so the
// classification consults the package's absence rule first and answers absent
// for it, as every accessor and the equality do.
func KindOfReturnType(transform ReturnType) ReturnTypeKind {
	if IsNilReturnType(transform) {
		return ReturnTypeUnknown
	}
	switch transform.(type) {
	case SameAs, *SameAs:
		return ReturnTypeSameAs
	case ElementOf, *ElementOf:
		return ReturnTypeElementOf
	case OptionalElementOf, *OptionalElementOf:
		return ReturnTypeOptionalElementOf
	case CallbackReturn, *CallbackReturn:
		return ReturnTypeCallbackReturn
	case ArrayOfCallbackReturn, *ArrayOfCallbackReturn:
		return ReturnTypeArrayOfCallbackReturn
	case TypeProjection, *TypeProjection:
		return ReturnTypeTypeProjection
	case ConditionalType, *ConditionalType:
		return ReturnTypeConditionalType
	default:
		return ReturnTypeUnknown
	}
}
