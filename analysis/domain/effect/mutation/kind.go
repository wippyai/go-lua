package mutation

// TransformKind identifies the vocabulary variant of a type transform. It
// classifies value and pointer spellings, including typed nil pointers; use the
// As... helpers when the concrete non-nil payload is required.
type TransformKind uint8

const (
	TransformUnknown TransformKind = iota
	TransformUnchanged
	TransformElementUnion
	TransformContainerElementUnion
	TransformToArray
	transformKindLimit
)

// TransformKindCount is the size of the closed vocabulary. The ordinals are
// dense from TransformUnchanged, so a consumer indexes by kind without a lookup.
const TransformKindCount = int(transformKindLimit) - 1

// Valid reports membership in the closed vocabulary. TransformUnknown is the
// answer for a transform outside it and is not a member.
func (kind TransformKind) Valid() bool {
	return kind > TransformUnknown && kind < transformKindLimit
}

// TransformKinds is the vocabulary catalog in ordinal order. It is the one
// enumeration of the variants this package owns, so a consumer that visits,
// serializes, or declares every variant projects it instead of restating the
// member list. The catalog is returned by value and costs no allocation to
// range over.
func TransformKinds() [TransformKindCount]TransformKind {
	return [TransformKindCount]TransformKind{
		TransformUnchanged, TransformElementUnion,
		TransformContainerElementUnion, TransformToArray,
	}
}

// KindOfTransform reports the vocabulary variant of transform.
func KindOfTransform(transform TypeTransform) TransformKind {
	switch transform.(type) {
	case Unchanged, *Unchanged:
		return TransformUnchanged
	case ElementUnion, *ElementUnion:
		return TransformElementUnion
	case ContainerElementUnion, *ContainerElementUnion:
		return TransformContainerElementUnion
	case ToArray, *ToArray:
		return TransformToArray
	default:
		return TransformUnknown
	}
}

// AsUnchanged returns the concrete Unchanged transform for value and non-nil
// pointer spellings. Typed nil pointers are treated as absent.
func AsUnchanged(t TypeTransform) (Unchanged, bool) {
	switch tt := t.(type) {
	case Unchanged:
		return tt, true
	case *Unchanged:
		if tt != nil {
			return *tt, true
		}
	}
	return Unchanged{}, false
}

// AsElementUnion returns the concrete ElementUnion transform for value and
// non-nil pointer spellings. Typed nil pointers are treated as absent.
func AsElementUnion(t TypeTransform) (ElementUnion, bool) {
	switch tt := t.(type) {
	case ElementUnion:
		return tt, true
	case *ElementUnion:
		if tt != nil {
			return *tt, true
		}
	}
	return ElementUnion{}, false
}

// AsContainerElementUnion returns the concrete ContainerElementUnion transform
// for value and non-nil pointer spellings. Typed nil pointers are treated as
// absent.
func AsContainerElementUnion(t TypeTransform) (ContainerElementUnion, bool) {
	switch tt := t.(type) {
	case ContainerElementUnion:
		return tt, true
	case *ContainerElementUnion:
		if tt != nil {
			return *tt, true
		}
	}
	return ContainerElementUnion{}, false
}

// AsToArray returns the concrete ToArray transform for value and non-nil
// pointer spellings. Typed nil pointers are treated as absent.
func AsToArray(t TypeTransform) (ToArray, bool) {
	switch tt := t.(type) {
	case ToArray:
		return tt, true
	case *ToArray:
		if tt != nil {
			return *tt, true
		}
	}
	return ToArray{}, false
}
