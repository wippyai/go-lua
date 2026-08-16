// Package typegraph provides allocation-free traversal state for exact
// recursive type-graph queries across analysis layers.
package typegraph

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

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
	if t == nil {
		return true
	}
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
	if t == nil {
		return
	}
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

type Pair struct {
	Left  typ.Type
	Right typ.Type
}

type PairPath struct {
	inline   [8]Pair
	inlineN  uint8
	overflow map[Pair]struct{}
}

func (p *PairPath) Enter(left, right typ.Type) bool {
	pair := Pair{Left: left, Right: right}
	for i := range p.inlineN {
		if p.inline[i] == pair {
			return false
		}
	}
	if _, ok := p.overflow[pair]; ok {
		return false
	}
	if p.inlineN < uint8(len(p.inline)) {
		p.inline[p.inlineN] = pair
		p.inlineN++
		return true
	}
	if p.overflow == nil {
		p.overflow = make(map[Pair]struct{})
	}
	p.overflow[pair] = struct{}{}
	return true
}

func (p *PairPath) Leave(left, right typ.Type) {
	pair := Pair{Left: left, Right: right}
	if _, ok := p.overflow[pair]; ok {
		delete(p.overflow, pair)
		return
	}
	for i := int(p.inlineN) - 1; i >= 0; i-- {
		if p.inline[i] != pair {
			continue
		}
		last := int(p.inlineN) - 1
		p.inline[i] = p.inline[last]
		p.inline[last] = Pair{}
		p.inlineN--
		return
	}
}
