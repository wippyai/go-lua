// Package typegraph provides allocation-free shallow traversal state for exact
// recursive type-graph queries used by engine transfer boundaries.
package typegraph

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

type Path struct {
	inline   [8]typ.Type
	inlineN  uint8
	overflow map[typ.Type]struct{}
}

func (p *Path) Enter(t typ.Type) bool {
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
		p.overflow = make(map[typ.Type]struct{})
	}
	p.overflow[t] = struct{}{}
	return true
}

func (p *Path) Leave(t typ.Type) {
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
