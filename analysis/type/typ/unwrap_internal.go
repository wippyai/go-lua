package typ

// UnwrapTransparentWrappers peels every transparent Annotated wrapper and
// returns the first non-annotated type. It is the canonical transparent-wrapper
// unwrap shared by traversal helpers across the type packages.
func UnwrapTransparentWrappers(t Type) Type {
	var path typePath
	for {
		ann, ok := t.(*Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t || !path.enter(ann) {
			return t
		}
		t = ann.Inner
	}
}

// UnwrapStructuralWrappers returns the structural view beneath every
// transparent Annotated and Alias wrapper. Wrapper kinds may alternate, so
// neither UnwrapTransparentWrappers nor Alias.UnaliasedTarget is sufficient
// by itself.
//
// Malformed nil links and wrapper cycles terminate at the first node that
// cannot be peeled. The small-path detector keeps ordinary wrapper chains
// allocation-free; only paths deeper than its inline storage allocate.
func UnwrapStructuralWrappers(t Type) Type {
	var path typePath
	for {
		t = UnwrapTransparentWrappers(t)
		alias, ok := t.(*Alias)
		if !ok || alias == nil || alias.Target == nil || !path.enter(alias) {
			return t
		}
		t = alias.Target
	}
}

func unwrapAnnotatedOrNil(t Type) Type {
	if t == nil {
		return nil
	}
	return unwrapAnnotated(t)
}
