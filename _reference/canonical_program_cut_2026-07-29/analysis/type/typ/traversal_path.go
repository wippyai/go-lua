package typ

// typePath is the allocation-free small-path cycle set used by core linear
// wrapper traversals. Only paths deeper than eight distinct nodes allocate.
type typePath struct {
	inline   [8]Type
	inlineN  uint8
	overflow map[Type]struct{}
}

func (p *typePath) enter(t Type) bool {
	if t == nil {
		return true
	}
	for i := range p.inlineN {
		if p.inline[i] == t {
			return false
		}
	}
	if _, ok := p.overflow[t]; ok {
		return false
	}
	if p.inlineN < uint8(len(p.inline)) {
		p.inline[p.inlineN] = t
		p.inlineN++
		return true
	}
	if p.overflow == nil {
		p.overflow = make(map[Type]struct{})
	}
	p.overflow[t] = struct{}{}
	return true
}

func (p *typePath) leave(t Type) {
	if t == nil {
		return
	}
	if _, ok := p.overflow[t]; ok {
		delete(p.overflow, t)
		return
	}
	for i := int(p.inlineN) - 1; i >= 0; i-- {
		if p.inline[i] != t {
			continue
		}
		last := int(p.inlineN) - 1
		p.inline[i] = p.inline[last]
		p.inline[last] = nil
		p.inlineN--
		return
	}
}
