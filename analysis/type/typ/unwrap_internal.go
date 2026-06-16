package typ

func unwrapTransparentWrappers(t Type) Type {
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

func unwrapAnnotatedOrNil(t Type) Type {
	if t == nil {
		return nil
	}
	return unwrapAnnotated(t)
}
