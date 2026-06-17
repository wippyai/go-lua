package typ

// MapMembers applies f to each member and returns a copy-on-change slice along
// with whether any member changed. When nothing changes it returns (in, false)
// so callers can preserve the original aggregate type instead of rebuilding an
// identical one. It is the canonical "distribute a transform over a type's
// member slice" helper shared by union, intersection, and tuple rewriting.
func MapMembers(in []Type, f func(Type) Type) ([]Type, bool) {
	var out []Type
	for i, m := range in {
		mapped := f(m)
		if mapped == m {
			continue
		}
		if out == nil {
			out = make([]Type, len(in))
			copy(out, in)
		}
		out[i] = mapped
	}
	if out == nil {
		return in, false
	}
	return out, true
}
