package path

// IsEmpty returns true if the path has no identity (no Root and no Symbol).
func (p Path) IsEmpty() bool { return p.Root == "" && p.Symbol == 0 }

// HasSymbol returns true if this path has a resolved symbol identity.
func (p Path) HasSymbol() bool { return p.Symbol != 0 }

// Equal returns true if two paths have the same identity.
// Symbol-based identity takes precedence when available.
func (p Path) Equal(other Path) bool {
	// Symbol-based identity is primary when available
	if p.Symbol != 0 || other.Symbol != 0 {
		// If either has a symbol, both must have the same symbol
		if p.Symbol != other.Symbol {
			return false
		}
		if p.Version != other.Version {
			return false
		}
	} else {
		// Neither has symbol (placeholder paths) - use Root for identity
		if p.Root != other.Root {
			return false
		}
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

// Less provides a stable ordering for canonicalization.
// Compares by Symbol first when both are set, otherwise by Root.
func (p Path) Less(other Path) bool {
	// Compare by Symbol when both have it
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
