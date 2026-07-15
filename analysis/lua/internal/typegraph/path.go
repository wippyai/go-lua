// Package typegraph provides allocation-free shallow traversal state for exact
// recursive type queries at the Lua semantic lowering boundary.
package typegraph

import "github.com/wippyai/go-lua/analysis/type/typ"

type Node struct {
	Type     typ.Type
	Position int
}

type Path struct {
	inline   [8]Node
	inlineN  uint8
	overflow map[Node]struct{}
}

func (p *Path) Enter(t typ.Type, position int) bool {
	n := Node{Type: t, Position: position}
	for i := range p.inlineN {
		if p.inline[i] == n {
			return false
		}
	}
	if _, ok := p.overflow[n]; ok {
		return false
	}
	if p.inlineN < uint8(len(p.inline)) {
		p.inline[p.inlineN] = n
		p.inlineN++
		return true
	}
	if p.overflow == nil {
		p.overflow = make(map[Node]struct{})
	}
	p.overflow[n] = struct{}{}
	return true
}

func (p *Path) Leave(t typ.Type, position int) {
	n := Node{Type: t, Position: position}
	if _, ok := p.overflow[n]; ok {
		delete(p.overflow, n)
		return
	}
	for i := int(p.inlineN) - 1; i >= 0; i-- {
		if p.inline[i] != n {
			continue
		}
		last := int(p.inlineN) - 1
		p.inline[i] = p.inline[last]
		p.inline[last] = Node{}
		p.inlineN--
		return
	}
}
