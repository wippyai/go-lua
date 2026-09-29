package typ

// MinRequiredArgs returns the minimum positional argument count required to call f.
//
// Lua arguments are positional, so a required parameter after an optional parameter
// still requires all earlier positions to be present.
func MinRequiredArgs(f *Function) int {
	if f == nil || len(f.Params) == 0 {
		return 0
	}
	lastRequired := -1
	for i, p := range f.Params {
		if !ParamMayBeAbsent(p) {
			lastRequired = i
		}
	}
	if lastRequired < 0 {
		return 0
	}
	return lastRequired + 1
}

// ParamMayBeAbsent reports whether a call may omit the parameter: it has a
// default or is declared nilable (T?). A parameter typed unknown or any is
// still required.
func ParamMayBeAbsent(p Param) bool {
	if p.Optional {
		return true
	}
	if p.Type == nil {
		return false
	}
	_, nilable := SplitNilableFieldType(p.Type)
	return nilable
}
