package typ

func knownAnyStaticMembers(members []StaticMember) bool {
	for _, m := range members {
		if knownAny(m.Type) {
			return true
		}
	}
	return false
}

func knownNeverStaticMembers(members []StaticMember) bool {
	for _, m := range members {
		if knownNever(m.Type) {
			return true
		}
	}
	return false
}

func knownTypeParamStaticMembers(members []StaticMember) bool {
	for _, m := range members {
		if knownTypeParam(m.Type) {
			return true
		}
	}
	return false
}

func knownInstantiatedStaticMembers(members []StaticMember) bool {
	for _, m := range members {
		if knownInstantiated(m.Type) {
			return true
		}
	}
	return false
}

func knownRecursiveStaticMembers(members []StaticMember) bool {
	for _, m := range members {
		if knownRecursive(m.Type) {
			return true
		}
	}
	return false
}

func knownOpenRecursiveStaticMembers(members []StaticMember) bool {
	for _, m := range members {
		if knownOpenRecursive(m.Type) {
			return true
		}
	}
	return false
}
