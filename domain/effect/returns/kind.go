package returns

// ReturnTypeKind identifies the vocabulary variant of a return transform. It
// classifies value and pointer spellings, including typed nil pointers; use the
// As... helpers when the concrete non-nil payload is required.
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

// KindOfReturnType reports the vocabulary variant of transform.
func KindOfReturnType(transform ReturnType) ReturnTypeKind {
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
