package composition

// CanonicalActivationFamily admits one immutable semantic capability. It has
// no caller-programmable descriptor grammar: such a grammar can encode an
// Application×Target matrix and would become a second topology authority.
// Conditional semantic cases therefore use separate domain-declared family
// semantics and structural Rules.
func CanonicalActivationFamily(family ActivationFamily) (ActivationFamily, bool) {
	if !family.Semantic.Available() {
		return ActivationFamily{}, false
	}
	return ActivationFamily{Semantic: family.Semantic}, true
}

func sameActivationFamily(left, right ActivationFamily) bool {
	return left.Semantic == right.Semantic
}
