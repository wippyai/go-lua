package typ

func knownAnyFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsAny(f.Type) {
			return true
		}
	}
	return false
}

func knownNeverFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsNever(f.Type) {
			return true
		}
	}
	return false
}

func knownTypeParamFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsTypeParam(f.Type) {
			return true
		}
	}
	return false
}

func knownInstantiatedFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsInstantiated(f.Type) {
			return true
		}
	}
	return false
}

func knownRecursiveFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsRecursive(f.Type) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveFields(fields []Field) bool {
	for _, f := range fields {
		if knownContainsOpenRecursive(f.Type) {
			return true
		}
	}
	return false
}

func knownAnyTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsAny(p.Constraint) {
			return true
		}
	}
	return false
}

func knownNeverTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsNever(p.Constraint) {
			return true
		}
	}
	return false
}

func knownTypeParamTypeParams(params []*TypeParam) bool {
	return len(params) > 0
}

func knownInstantiatedTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsInstantiated(p.Constraint) {
			return true
		}
	}
	return false
}

func knownRecursiveTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsRecursive(p.Constraint) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveTypeParams(params []*TypeParam) bool {
	for _, p := range params {
		if p != nil && knownContainsOpenRecursive(p.Constraint) {
			return true
		}
	}
	return false
}
