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
)

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
