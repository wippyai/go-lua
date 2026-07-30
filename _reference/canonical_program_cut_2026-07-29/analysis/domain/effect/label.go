package effect

import "reflect"

// Label is one atomic function effect.
type Label interface {
	EffectLabel()
	String() string
	Equals(other Label) bool
}

// NormalizeLabel returns the value-owned form of a label. Effect rows store
// labels as values so selectors, equality, hashing, and manifest roundtrips
// all observe the same concrete label shape.
func NormalizeLabel(label Label) Label {
	if label == nil {
		return nil
	}
	v := reflect.ValueOf(label)
	if v.Kind() != reflect.Pointer {
		return label
	}
	if v.IsNil() {
		return nil
	}
	if normalized, ok := v.Elem().Interface().(Label); ok {
		return normalized
	}
	return label
}
