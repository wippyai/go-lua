package program

// declaredTypeRow attaches one authored declared type to its exact lexical Cell.
// It is deliberately limited to lexical/formal/vararg Cells: callable returns,
// casts, and schema annotations have different source semantics.
type declaredTypeRow struct{ host, target Term }

// DeclareCellType records one authored declared type for a lexical Cell.
func (b *Builder) DeclareCellType(span Span, host, target Term) Term {
	if !b.require(b.lexicalCell(host) && b.staticTypeNode(target)) {
		return 0
	}
	b.declaredTypes = append(b.declaredTypes, declaredTypeRow{host: host, target: target})
	term := b.mint(tagDeclaredType, span, b.familyIndex(len(b.declaredTypes)))
	if term == 0 {
		b.declaredTypes = b.declaredTypes[:len(b.declaredTypes)-1]
	}
	return term
}

// validateDeclaredTypes attaches declared-type roots to their exact Cell hosts.
// A Cell may carry at most one authored declaration.
func (b *Builder) validateDeclaredTypes(attach func(parent, child Term) bool) bool {
	seen := make([]bool, len(b.cells))
	for index, row := range b.declaredTypes {
		declared := makeTerm(tagDeclaredType, uint32(index+1))
		if !b.lexicalCell(row.host) || seen[row.host.index()-1] || !attach(declared, row.target) {
			return false
		}
		seen[row.host.index()-1] = true
	}
	return true
}

// declaredTypeLookup derives the sealed dense Cell-to-declaration index. It is
// kept separate from relation ownership so downstream users never scan rows.
func (b *Builder) declaredTypeLookup() ([]Term, bool) {
	lookup := make([]Term, len(b.cells)+1)
	for index, row := range b.declaredTypes {
		declared := makeTerm(tagDeclaredType, uint32(index+1))
		if !b.lexicalCell(row.host) || lookup[row.host.index()] != 0 {
			return nil, false
		}
		lookup[row.host.index()] = declared
	}
	return lookup, true
}

// DeclaredType reports the exact lexical Cell and authored static type attached
// by one declared type annotation.
func (p *Program) DeclaredType(term Term) (host, target Term, ok bool) {
	if !p.has(term, tagDeclaredType) {
		return 0, 0, false
	}
	r := p.declaredTypes[term.index()-1]
	return r.host, r.target, true
}

func (p *Program) DeclaredTypeCount() int {
	if p == nil {
		return 0
	}
	return len(p.declaredTypes)
}

func (p *Program) DeclaredTypeAt(index int) (Term, bool) {
	return familyTerm(tagDeclaredType, p.DeclaredTypeCount(), index)
}

// CellDeclaredType returns the one authored declaration for cell, if present.
// It is a direct sealed lookup, not a scan over DeclaredType rows.
func (p *Program) CellDeclaredType(cell Term) (Term, bool) {
	if !p.has(cell, tagCell) || int(cell.index()) >= len(p.cellDeclaredTypes) {
		return 0, false
	}
	declared := p.cellDeclaredTypes[cell.index()]
	return declared, declared != 0
}
