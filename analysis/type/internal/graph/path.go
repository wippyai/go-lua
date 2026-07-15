// Package graph provides allocation-free small traversal state for exact type
// graph algorithms outside the core typ package.
package graph

import "github.com/wippyai/go-lua/analysis/type/typ"

// Path tracks nodes on the current DFS path. Ordinary shallow traversals stay
// entirely inline; only paths deeper than eight distinct nodes allocate.
type Path struct {
	inline   [8]typ.Type
	inlineN  uint8
	overflow map[typ.Type]struct{}
}

// Pair is one ordered type-graph relation node.
type Pair struct {
	Left  typ.Type
	Right typ.Type
}

// PairPath is the ordered-pair counterpart of Path.
type PairPath struct {
	inline   [8]Pair
	inlineN  uint8
	overflow map[Pair]struct{}
}

// Enter records an ordered pair and reports false on a current-path repeat.
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

// Leave removes an ordered pair from the current path.
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

// Enter records t and reports false when t is already on the current path.
func (p *Path) Enter(t typ.Type) bool {
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
		p.overflow = make(map[typ.Type]struct{})
	}
	p.overflow[t] = struct{}{}
	return true
}

// Leave removes t from the current path.
func (p *Path) Leave(t typ.Type) {
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
