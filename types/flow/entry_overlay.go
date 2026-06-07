package flow

// OverlayCaptureCells returns a closure-entry cell store where live captured
// locations override allocation-time snapshot values. Lua closures capture
// mutable locations, so a snapshot is a fallback, not an immutable fact.
func OverlayCaptureCells(base, live CaptureCells) CaptureCells {
	if base.IsTop() || live.IsTop() {
		if live.IsTop() {
			return live
		}
		return base
	}
	out := base
	for _, entry := range live.Entries() {
		out = out.With(entry.Symbol, entry.Value)
	}
	return out
}

// OverlayFunctionRefs returns a closure-entry function-ref store where live
// captured paths override allocation-time snapshot identities.
func OverlayFunctionRefs(base, live FunctionRefs) FunctionRefs {
	if FunctionRefsDomain.Equal(base, FunctionRefsDomain.Top()) ||
		FunctionRefsDomain.Equal(live, FunctionRefsDomain.Top()) {
		if FunctionRefsDomain.Equal(live, FunctionRefsDomain.Top()) {
			return live
		}
		return base
	}
	out := FunctionRefsDomain.Join(base, nil)
	for path, set := range live {
		if set.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(path)
		if !ok {
			continue
		}
		out = WithFunctionRefAddress(out, addr, set)
	}
	return out
}

// OverlayClosureRefs returns a closure-entry closure-ref store where live
// captured paths override allocation-time snapshot closure environments.
func OverlayClosureRefs(base, live ClosureRefs) ClosureRefs {
	if ClosureRefsDomain.Equal(base, ClosureRefsDomain.Top()) ||
		ClosureRefsDomain.Equal(live, ClosureRefsDomain.Top()) {
		if ClosureRefsDomain.Equal(live, ClosureRefsDomain.Top()) {
			return live
		}
		return base
	}
	out := ClosureRefsDomain.Join(base, nil)
	for path, set := range live {
		if set.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(path)
		if !ok {
			continue
		}
		out = WithClosureRefAddress(out, addr, set)
	}
	return out
}
