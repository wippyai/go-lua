package effect

import "reflect"

// Label is one atomic function effect.
//
// CapabilityID answers the audited capability the label is classified as, using
// the dotted ID the capability catalog authors. Every label states its own, so
// the classification lives beside the type it classifies and no central switch
// re-derives it from the Go type. The empty string means the label carries no
// classifiable payload, which only a Return with an absent transform can be.
type Label interface {
	CapabilityID() string
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
