package signature

func cloneTypestateRequirements(in []TypestateRequirement) []TypestateRequirement {
	if len(in) == 0 {
		return nil
	}
	out := make([]TypestateRequirement, len(in))
	for i, requirement := range in {
		out[i] = requirement
		out[i].Target = requirement.Target.Clone()
	}
	return out
}

func equalTypestateRequirements(a, b []TypestateRequirement) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Protocol != b[i].Protocol || a[i].State != b[i].State || !a[i].Target.Equal(b[i].Target) {
			return false
		}
	}
	return true
}
