package transfer

// Value is the keyless two-point membership fact for the complete Transfer
// family. The correlated semantic coordinate is carried only by Key; every
// absent key uses the same constant Default Value.
type Value struct {
	owner   *algebra
	present bool
}

func (algebra Algebra) Default() (Value, bool) {
	if !algebra.valid() {
		return Value{}, false
	}
	return Value{owner: algebra.owner}, true
}

func (algebra Algebra) Present() (Value, bool) {
	if !algebra.valid() {
		return Value{}, false
	}
	return Value{owner: algebra.owner, present: true}, true
}

func (value Value) valid() bool { return value.owner != nil }
func (algebra Algebra) owns(value Value) bool {
	return algebra.valid() && value.owner == algebra.owner && value.valid()
}

func (algebra Algebra) Equal(left, right Value) bool {
	return algebra.owns(left) && algebra.owns(right) && left.present == right.present
}

func (algebra Algebra) LessOrEq(left, right Value) bool {
	return algebra.owns(left) && algebra.owns(right) && (!left.present || right.present)
}

// Join is membership union. Foreign values fail closed rather than becoming a
// bottom or top value under an unrelated family.
func (algebra Algebra) Join(left, right Value) (Value, bool) {
	if !algebra.owns(left) || !algebra.owns(right) {
		return Value{}, false
	}
	return Value{owner: algebra.owner, present: left.present || right.present}, true
}

func (algebra Algebra) Widen(previous, next Value) (Value, bool) { return algebra.Join(previous, next) }
