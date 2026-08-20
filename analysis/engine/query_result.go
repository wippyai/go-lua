package engine

// frozenValue is one published, transitively immutable result value. It is
// never copied on the borrowed read path: a caller that needs an owned value
// asks the typed freezer for one through an explicit detachment.
type frozenValue interface {
	equal(frozenValue) bool
	fingerprint() uint64
	rowPresent() bool
}

type typedFrozenValue[R any] struct {
	value  R
	freeze FrozenResult[R]
}

func (value *typedFrozenValue[R]) equal(other frozenValue) bool {
	right, ok := other.(*typedFrozenValue[R])
	return ok && value != nil && right != nil && value.freeze.Equal(value.value, right.value)
}
func (value *typedFrozenValue[R]) fingerprint() uint64 {
	if value == nil {
		return 0
	}
	return value.freeze.Fingerprint(value.value)
}

func (value *typedFrozenValue[R]) rowPresent() bool {
	return value != nil && value.freeze.Present(value.value)
}
