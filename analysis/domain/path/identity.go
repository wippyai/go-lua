package path

// IsEmpty returns true if the path has no identity (no Root and no Symbol).
func (p Path) IsEmpty() bool { return p.Root == "" && p.Symbol == 0 }

// HasSymbol returns true if this path has a resolved symbol identity.
func (p Path) HasSymbol() bool { return p.Symbol != 0 }

// Equal compares path identity before comparing the static suffix. Symbol
// identity is authoritative: if either path has a symbol, both Symbol and
// Version must match. Placeholder paths without symbols fall back to Root.
func (p Path) Equal(other Path) bool {
	if p.Symbol != 0 || other.Symbol != 0 {
		if p.Symbol != other.Symbol {
			return false
		}
		if p.Version != other.Version {
			return false
		}
	} else if p.Root != other.Root {
		return false
	}

	if len(p.Segments) != len(other.Segments) {
		return false
	}

	for i := range p.Segments {
		a := p.Segments[i]
		b := other.Segments[i]

		if a.Kind != b.Kind || a.Name != b.Name || a.Index != b.Index {
			return false
		}
	}

	return true
}

// Less provides stable canonical ordering. Two symbol-backed paths sort by
// Symbol and Version; all other comparisons fall back to Root before suffix
// segments break ties.
func (p Path) Less(other Path) bool {
	if p.Symbol != 0 && other.Symbol != 0 {
		if p.Symbol != other.Symbol {
			return p.Symbol < other.Symbol
		}
		if p.Version != other.Version {
			return p.Version < other.Version
		}
	} else if p.Root != other.Root {
		return p.Root < other.Root
	}

	if len(p.Segments) != len(other.Segments) {
		return len(p.Segments) < len(other.Segments)
	}

	for i := range p.Segments {
		a := p.Segments[i]
		b := other.Segments[i]

		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}

		if a.Name != b.Name {
			return a.Name < b.Name
		}

		if a.Index != b.Index {
			return a.Index < b.Index
		}
	}

	return false
}
