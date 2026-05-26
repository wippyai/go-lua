package product

import "github.com/wippyai/go-lua/types/typ"

// vector.go provides the slice-level admission and egress boundary for
// interprocedural carriers (api.FunctionFact parameter/return vectors and the
// captured/constructor field maps). A carrier stores []AbstractValue; producers
// lift their computed typ.Type vectors here at admission and consumers project
// them back at egress. The per-element conversion is FromType / ProjectValue, so
// the round-trip is the value-domain lossless inverse the carriers rely on.

// LiftVector lifts a typ.Type evidence vector into the interned AbstractValue
// carrier vector. A nil slot lifts to the zero AbstractValue so an unoccupied
// evidence slot survives the round-trip distinct from an interned Bottom; the
// egress ProjectVector maps the zero handle back to a nil slot.
func LiftVector(types []typ.Type) []AbstractValue {
	if len(types) == 0 {
		return nil
	}
	out := make([]AbstractValue, len(types))
	for i, t := range types {
		if t == nil {
			continue
		}
		out[i] = FromType(t)
	}
	return out
}

// ProjectVector projects a carrier AbstractValue vector back to its structural
// typ.Type evidence vector. The zero AbstractValue (an unoccupied slot) projects
// to a nil type so an absent evidence slot survives the round-trip.
func ProjectVector(values []AbstractValue) []typ.Type {
	if len(values) == 0 {
		return nil
	}
	out := make([]typ.Type, len(values))
	for i, v := range values {
		if v.IsZero() {
			continue
		}
		out[i] = v.ProjectValue()
	}
	return out
}

// LiftFieldMap lifts a fieldName->typ.Type map into the interned carrier field
// map at admission. A nil-typed slot lifts to the zero AbstractValue.
func LiftFieldMap(fields map[string]typ.Type) map[string]AbstractValue {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]AbstractValue, len(fields))
	for name, t := range fields {
		if t == nil {
			continue
		}
		out[name] = FromType(t)
	}
	return out
}

// ProjectFieldMap projects a carrier field map back to its structural
// fieldName->typ.Type map at egress. A zero-handle slot projects to a nil type.
func ProjectFieldMap(fields map[string]AbstractValue) map[string]typ.Type {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(fields))
	for name, v := range fields {
		if v.IsZero() {
			continue
		}
		out[name] = v.ProjectValue()
	}
	return out
}

// EqualVector reports whether two carrier vectors are element-wise product-Equal.
// The zero AbstractValue (absent slot) is equal only to another zero handle, so
// occupancy is part of the comparison.
func EqualVector(a, b []AbstractValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].IsZero() || b[i].IsZero() {
			if a[i].IsZero() != b[i].IsZero() {
				return false
			}
			continue
		}
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
