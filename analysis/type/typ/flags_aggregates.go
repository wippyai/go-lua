package typ

func knownAny(types ...Type) bool {
	for _, t := range types {
		if knownContainsAny(t) {
			return true
		}
	}
	return false
}

func knownNever(types ...Type) bool {
	for _, t := range types {
		if knownContainsNever(t) {
			return true
		}
	}
	return false
}

func knownTypeParam(types ...Type) bool {
	for _, t := range types {
		if knownContainsTypeParam(t) {
			return true
		}
	}
	return false
}

func knownInstantiated(types ...Type) bool {
	for _, t := range types {
		if knownContainsInstantiated(t) {
			return true
		}
	}
	return false
}

func knownRecursive(types ...Type) bool {
	for _, t := range types {
		if knownContainsRecursive(t) {
			return true
		}
	}
	return false
}

func knownOpenRecursive(types ...Type) bool {
	for _, t := range types {
		if knownContainsOpenRecursive(t) {
			return true
		}
	}
	return false
}

func knownAnyParams(params []Param) bool {
	for _, p := range params {
		if knownContainsAny(p.Type) {
			return true
		}
	}
	return false
}

func knownNeverParams(params []Param) bool {
	for _, p := range params {
		if knownContainsNever(p.Type) {
			return true
		}
	}
	return false
}

func knownTypeParamParams(params []Param) bool {
	for _, p := range params {
		if knownContainsTypeParam(p.Type) {
			return true
		}
	}
	return false
}

func knownInstantiatedParams(params []Param) bool {
	for _, p := range params {
		if knownContainsInstantiated(p.Type) {
			return true
		}
	}
	return false
}

func knownRecursiveParams(params []Param) bool {
	for _, p := range params {
		if knownContainsRecursive(p.Type) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveParams(params []Param) bool {
	for _, p := range params {
		if knownContainsOpenRecursive(p.Type) {
			return true
		}
	}
	return false
}
