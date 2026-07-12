// Package semanticplanalloc is an isolated representation experiment for
// immutable symbolic transformers. It is not imported by the analyzer.
package semanticplanalloc

import (
	"fmt"
	"slices"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// EffectKind is deliberately an integer in the packed representation. Human
// names belong in cold metadata, not in every effect cell.
type EffectKind uint8

const (
	EffectInvalidateRoot EffectKind = iota + 1
	EffectWriteValue
	EffectInvalidatePath
	EffectRelatePaths
	EffectInvalidateHeap
	EffectClearDynamicIndex
	EffectClearKeyMembership
	EffectClearLenFloor
	EffectCopyUserLattice
)

type EffectSpec struct {
	Kind  EffectKind
	Phase uint8
}

// LaneSpec is explicit even for an unaffected lane. Consequently, adding a
// catalog lane without adding an adapter makes compilation fail closed.
type LaneSpec struct {
	Lane    state.LaneID
	Covered bool
	Effects []EffectSpec
}

type Registry struct {
	lanes    []state.LaneID
	adapters map[state.LaneID][]EffectSpec
	covered  uint64
}

func NewRegistry(lanes []state.LaneID, specs []LaneSpec) (*Registry, error) {
	if len(lanes) > 64 {
		return nil, fmt.Errorf("semanticplanalloc: at most 64 lanes")
	}
	known := make(map[state.LaneID]uint8, len(lanes))
	for i, lane := range lanes {
		if _, duplicate := known[lane]; duplicate {
			return nil, fmt.Errorf("semanticplanalloc: duplicate catalog lane %q", lane)
		}
		known[lane] = uint8(i)
	}
	r := &Registry{lanes: append([]state.LaneID(nil), lanes...), adapters: make(map[state.LaneID][]EffectSpec, len(specs))}
	for _, spec := range specs {
		ordinal, ok := known[spec.Lane]
		if !ok {
			return nil, fmt.Errorf("semanticplanalloc: orphan lane %q", spec.Lane)
		}
		if _, duplicate := r.adapters[spec.Lane]; duplicate {
			return nil, fmt.Errorf("semanticplanalloc: duplicate adapter %q", spec.Lane)
		}
		if !spec.Covered {
			continue
		}
		r.covered |= uint64(1) << ordinal
		r.adapters[spec.Lane] = append([]EffectSpec(nil), spec.Effects...)
	}
	return r, nil
}

func DefaultRegistry() *Registry {
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	effects := map[state.LaneID][]EffectSpec{
		state.LaneValues:            {{EffectInvalidateRoot, 10}, {EffectWriteValue, 40}},
		state.LanePathEvidence:      {{EffectInvalidatePath, 30}, {EffectRelatePaths, 50}},
		state.LaneHeapTableIdentity: {{EffectInvalidateHeap, 20}},
		state.LaneDynamicIndex:      {{EffectClearDynamicIndex, 30}},
		state.LaneKeyMemberships:    {{EffectClearKeyMembership, 30}},
		state.LaneLenFloors:         {{EffectClearLenFloor, 30}},
		state.LaneUserLattices:      {{EffectCopyUserLattice, 60}},
	}
	specs := make([]LaneSpec, len(lanes))
	for i, lane := range lanes {
		specs[i] = LaneSpec{Lane: lane, Covered: true, Effects: effects[lane]}
	}
	r, err := NewRegistry(lanes, specs)
	if err != nil {
		panic(err)
	}
	return r
}

type termID uint16

const (
	targetTerm termID = iota
	sourceTerm
	termCount
)

type termSpec struct {
	root      uint16
	suffixOff uint16
	suffixLen uint16
}

// effectCell is compact and contains no paths, strings, interfaces, or slices.
// The same two immutable terms are shared by every lane effect.
type effectCell struct {
	target      termID
	source      termID
	laneOrdinal uint8
	kind        EffectKind
	phase       uint8
}

// Transformer owns its packed immutable slices. None are exposed.
type Transformer struct {
	lanes    []state.LaneID
	suffixes []segment.Segment
	terms    [termCount]termSpec
	effects  []effectCell
	required uint64
	covered  uint64
}

func (r *Registry) CompilePathAssignment(target, source pathdom.Path) (Transformer, bool) {
	if r == nil || target.IsEmpty() || source.IsEmpty() || len(target.Segments) == 0 {
		return Transformer{}, false
	}
	if len(target.Segments)+len(source.Segments) > int(^uint16(0)) {
		return Transformer{}, false
	}
	effectCount := 0
	for _, lane := range r.lanes {
		effectCount += len(r.adapters[lane])
	}
	t := Transformer{
		lanes:    r.lanes,
		required: requiredMask(len(r.lanes)),
		covered:  r.covered,
		suffixes: make([]segment.Segment, 0, len(target.Segments)+len(source.Segments)),
		effects:  make([]effectCell, 0, effectCount),
	}
	t.terms[targetTerm] = t.appendTerm(0, target.Segments)
	t.terms[sourceTerm] = t.appendTerm(1, source.Segments)
	for ordinal, lane := range r.lanes {
		for _, effect := range r.adapters[lane] {
			t.effects = append(t.effects, effectCell{
				target: targetTerm, source: sourceTerm, laneOrdinal: uint8(ordinal), kind: effect.Kind, phase: effect.Phase,
			})
		}
	}
	slices.SortStableFunc(t.effects, func(a, b effectCell) int {
		if a.phase != b.phase {
			return int(a.phase) - int(b.phase)
		}
		return int(a.laneOrdinal) - int(b.laneOrdinal)
	})
	return t, t.covered == t.required
}

func (t *Transformer) appendTerm(root uint16, suffix []segment.Segment) termSpec {
	off := len(t.suffixes)
	t.suffixes = append(t.suffixes, suffix...)
	return termSpec{root: root, suffixOff: uint16(off), suffixLen: uint16(len(suffix))}
}

func requiredMask(n int) uint64 {
	if n == 64 {
		return ^uint64(0)
	}
	return uint64(1)<<n - 1
}

// Bindings is borrowed for a Cursor's lifetime. Roots and values use dense
// transformer slots, avoiding string PathKey construction and hash lookup.
type Bindings struct {
	Roots     []pathdom.Path
	Values    []product.Value
	ValueMask uint64
}

// PathView is a non-owning concatenation. An execution adapter should resolve
// it directly to its lane-specific key rather than materializing a Path.
type PathView struct {
	Base   pathdom.Path
	Suffix []segment.Segment
}

type BoundEffect struct {
	Lane   state.LaneID
	Kind   EffectKind
	Phase  uint8
	Target PathView
	Source PathView
	Value  product.Value
}

type Cursor struct {
	transformer *Transformer
	bindings    *Bindings
	next        int
}

// Bind validates the complete transaction and returns a stack-sized cursor.
// Successful Bind and Cursor.Next perform no allocation.
func (t *Transformer) Bind(bindings *Bindings) (Cursor, bool) {
	if t == nil || bindings == nil || t.covered != t.required || len(bindings.Roots) < int(termCount) || len(bindings.Values) < int(termCount) {
		return Cursor{}, false
	}
	for i := 0; i < int(termCount); i++ {
		if bindings.Roots[i].IsEmpty() {
			return Cursor{}, false
		}
	}
	if bindings.ValueMask&(uint64(1)<<sourceTerm) == 0 {
		return Cursor{}, false
	}
	return Cursor{transformer: t, bindings: bindings}, true
}

func (c *Cursor) Next() (BoundEffect, bool) {
	if c == nil || c.transformer == nil || c.next >= len(c.transformer.effects) {
		return BoundEffect{}, false
	}
	effect := c.transformer.effects[c.next]
	c.next++
	return BoundEffect{
		Lane: c.transformer.lanes[effect.laneOrdinal], Kind: effect.kind, Phase: effect.phase,
		Target: c.termView(effect.target), Source: c.termView(effect.source), Value: c.bindings.Values[effect.source],
	}, true
}

func (c *Cursor) termView(id termID) PathView {
	term := c.transformer.terms[id]
	return PathView{
		Base:   c.bindings.Roots[term.root],
		Suffix: c.transformer.suffixes[int(term.suffixOff) : int(term.suffixOff)+int(term.suffixLen)],
	}
}
