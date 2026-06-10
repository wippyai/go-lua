package effect

func Union(r1, r2 Row) Row {
	if r1.IsUnknown() || r2.IsUnknown() {
		return Unknown
	}

	labels := append([]Label{}, r1.Labels...)
	for _, l := range r2.Labels {
		if !containsLabelEquals(labels, l) {
			labels = append(labels, l)
		}
	}

	var tail *Var
	switch {
	case r1.Tail != nil && r2.Tail != nil:
		if r1.Tail.Name == r2.Tail.Name {
			tail = r1.Tail
		} else {
			tail = &Var{Name: r1.Tail.Name + "+" + r2.Tail.Name}
		}
	case r1.Tail != nil:
		tail = r1.Tail
	case r2.Tail != nil:
		tail = r2.Tail
	}

	return Row{Labels: labels, Tail: tail}
}

func containsLabelEquals(labels []Label, l Label) bool {
	for _, existing := range labels {
		if existing.Equals(l) {
			return true
		}
	}
	return false
}

func Intersect(r1, r2 Row) Row {
	if r1.Pure() || r2.Pure() {
		return Empty
	}

	var labels []Label
	for _, l := range r1.Labels {
		for _, l2 := range r2.Labels {
			if l.Equals(l2) {
				labels = append(labels, l)
				break
			}
		}
	}

	var tail *Var
	if r1.Tail != nil && r2.Tail != nil && r1.Tail.Name == r2.Tail.Name {
		tail = r1.Tail
	}
	return Row{Labels: labels, Tail: tail}
}

func Subset(r1, r2 Row) bool {
	if r1.Pure() {
		return true
	}
	if r2.IsUnknown() {
		return true
	}
	if r1.IsUnknown() {
		return false
	}
	for _, l := range r1.Labels {
		found := false
		for _, l2 := range r2.Labels {
			if l.Equals(l2) {
				found = true
				break
			}
		}
		if !found && !r2.IsOpen() {
			return false
		}
	}
	return true
}

func Open(name string, labels ...Label) Row {
	return Row{Labels: labels, Tail: &Var{Name: name}}
}
