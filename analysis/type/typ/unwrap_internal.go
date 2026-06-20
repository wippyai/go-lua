package typ

// UnwrapTransparentWrappers peels every transparent Annotated wrapper and
// returns the first non-annotated type. It is the canonical transparent-wrapper
// unwrap shared by traversal helpers across the type packages.
func UnwrapTransparentWrappers(t Type) Type {
	for {
		ann, ok := t.(*Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}

func unwrapTransparentWrappers(t Type) Type {
	return UnwrapTransparentWrappers(t)
}

func unwrapAnnotatedOrNil(t Type) Type {
	if t == nil {
		return nil
	}
	return unwrapAnnotated(t)
}
