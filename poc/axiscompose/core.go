// Package axiscompose is an isolated experiment in descriptor-driven product
// lattices. It is intentionally not used by the production State domain.
package axiscompose

import (
	"fmt"
	"hash/fnv"
	"math/bits"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// AxisID is the stable name of one product coordinate.
type AxisID string

// Polarity documents whether facts accumulate by may-union or must-
// intersection. Lattice operations, rather than this label, define semantics.
type Polarity uint8

const (
	May Polarity = iota
	Must
)

// Spec is the typed registration surface for one axis.
type Spec[T any] struct {
	ID       AxisID
	Polarity Polarity
	Domain   lattice.Lattice[T]
	Boundary *Boundary[T]
	Hash     func(T) uint64
}

// Handle is a typed reference to a registered axis.
type Handle[T any] struct {
	catalog *Catalog
	id      AxisID
}

// ID returns the registered axis name.
func (h Handle[T]) ID() AxisID { return h.id }

type erasedSpec struct {
	id       AxisID
	polarity Polarity
	bottom   func() any
	top      func() any
	equal    func(any, any) bool
	same     func(any, any) bool
	lessOrEq func(any, any) bool
	join     func(any, any) any
	widen    func(any, any) any
	hash     func(any) uint64
	boundary *erasedBoundary
}

// Catalog owns axis registration order. Schemas are sealed selections from it.
type Catalog struct {
	specs []erasedSpec
	byID  map[AxisID]int
}

// Register verifies and adds one typed axis.
func Register[T any](c *Catalog, spec Spec[T]) (Handle[T], error) {
	if c == nil {
		return Handle[T]{}, fmt.Errorf("axiscompose: nil catalog")
	}
	if spec.ID == "" {
		return Handle[T]{}, fmt.Errorf("axiscompose: empty axis id")
	}
	if spec.Domain.Bottom == nil || spec.Domain.Top == nil || spec.Domain.Equal == nil ||
		spec.Domain.LessOrEq == nil || spec.Domain.Join == nil || spec.Domain.Widen == nil {
		return Handle[T]{}, fmt.Errorf("axiscompose: axis %q has incomplete lattice", spec.ID)
	}
	if c.byID == nil {
		c.byID = make(map[AxisID]int)
	}
	if _, exists := c.byID[spec.ID]; exists {
		return Handle[T]{}, fmt.Errorf("axiscompose: duplicate axis %q", spec.ID)
	}
	e := erasedSpec{
		id:       spec.ID,
		polarity: spec.Polarity,
		bottom:   func() any { return spec.Domain.Bottom() },
		top:      func() any { return spec.Domain.Top() },
		equal:    func(a, b any) bool { return spec.Domain.Equal(a.(T), b.(T)) },
		lessOrEq: func(a, b any) bool { return spec.Domain.LessOrEq(a.(T), b.(T)) },
		join:     func(a, b any) any { return spec.Domain.Join(a.(T), b.(T)) },
		widen:    func(a, b any) any { return spec.Domain.Widen(a.(T), b.(T)) },
	}
	if spec.Domain.Same != nil {
		e.same = func(a, b any) bool { return spec.Domain.Same(a.(T), b.(T)) }
	}
	if spec.Hash != nil {
		e.hash = func(v any) uint64 { return spec.Hash(v.(T)) }
	} else {
		e.hash = func(v any) uint64 {
			h := fnv.New64a()
			_, _ = fmt.Fprint(h, v)
			return h.Sum64()
		}
	}
	if spec.Boundary != nil {
		e.boundary = eraseBoundary(spec.Boundary)
	}
	c.byID[spec.ID] = len(c.specs)
	c.specs = append(c.specs, e)
	return Handle[T]{catalog: c, id: spec.ID}, nil
}

// MustRegister is the panic-on-error registration helper used by tests/benches.
func MustRegister[T any](c *Catalog, spec Spec[T]) Handle[T] {
	h, err := Register(c, spec)
	if err != nil {
		panic(err)
	}
	return h
}

// Schema is one immutable selected product layout in canonical catalog order.
type Schema struct {
	catalog *Catalog
	specs   []erasedSpec
	byID    map[AxisID]int
}

// Seal validates selected IDs and constructs a canonical schema.
func (c *Catalog) Seal(selected ...AxisID) (*Schema, error) {
	if c == nil {
		return nil, fmt.Errorf("axiscompose: nil catalog")
	}
	want := make(map[AxisID]struct{}, len(selected))
	for _, id := range selected {
		if _, ok := c.byID[id]; !ok {
			return nil, fmt.Errorf("axiscompose: unknown axis %q", id)
		}
		if _, duplicate := want[id]; duplicate {
			return nil, fmt.Errorf("axiscompose: duplicate selected axis %q", id)
		}
		want[id] = struct{}{}
	}
	s := &Schema{catalog: c, byID: make(map[AxisID]int, len(want))}
	for _, spec := range c.specs {
		if _, ok := want[spec.id]; !ok {
			continue
		}
		s.byID[spec.id] = len(s.specs)
		s.specs = append(s.specs, spec)
	}
	return s, nil
}

// IDs returns selected IDs in canonical operation order.
func (s *Schema) IDs() []AxisID {
	if s == nil {
		return nil
	}
	out := make([]AxisID, len(s.specs))
	for i := range s.specs {
		out[i] = s.specs[i].id
	}
	return out
}

// Len returns the selected axis count.
func (s *Schema) Len() int {
	if s == nil {
		return 0
	}
	return len(s.specs)
}

// Stamp is a solve-local content identity token.
type Stamp uint64

// Arena owns stamps. States from different arenas cannot use stamp shortcuts.
type Arena struct {
	next         Stamp
	bottomStamps map[*Schema][]Stamp
}

func (a *Arena) fresh() Stamp {
	if a.next == ^Stamp(0) {
		panic("axiscompose: stamp overflow")
	}
	a.next++
	if a.next == 0 {
		a.next++
	}
	return a.next
}

func (a *Arena) bottoms(s *Schema) []Stamp {
	if a.bottomStamps == nil {
		a.bottomStamps = make(map[*Schema][]Stamp)
	}
	if got := a.bottomStamps[s]; got != nil {
		return got
	}
	out := make([]Stamp, s.Len())
	for i := range out {
		out[i] = a.fresh()
	}
	a.bottomStamps[s] = out
	return out
}

type slot struct {
	value any
	stamp Stamp
}

// State is an immutable product value for one schema and arena.
type State struct {
	schema *Schema
	arena  *Arena
	slots  []slot
}

// Bottom constructs the canonical bottom spelling for a schema in an arena.
func Bottom(arena *Arena, schema *Schema) State {
	if arena == nil || schema == nil {
		panic("axiscompose: Bottom requires arena and schema")
	}
	stamps := arena.bottoms(schema)
	slots := make([]slot, schema.Len())
	for i, spec := range schema.specs {
		slots[i] = slot{value: spec.bottom(), stamp: stamps[i]}
	}
	return State{schema: schema, arena: arena, slots: slots}
}

func compatible(a, b State) {
	if a.schema == nil || a.schema != b.schema || a.arena == nil || a.arena != b.arena {
		panic("axiscompose: incompatible state schema or arena")
	}
}

// Get reads a typed selected coordinate. False means the axis is not selected.
func Get[T any](s State, h Handle[T]) (T, bool) {
	var zero T
	if s.schema == nil || h.catalog == nil || s.schema.catalog != h.catalog {
		return zero, false
	}
	i, ok := s.schema.byID[h.id]
	if !ok {
		return zero, false
	}
	return s.slots[i].value.(T), true
}

// Put returns s with one coordinate changed. Semantic no-ops preserve stamps.
func Put[T any](arena *Arena, s State, h Handle[T], value T) State {
	if arena == nil || s.arena != arena || s.schema == nil || s.schema.catalog != h.catalog {
		panic("axiscompose: Put requires matching arena and catalog")
	}
	i, ok := s.schema.byID[h.id]
	if !ok {
		return s
	}
	spec := s.schema.specs[i]
	if spec.equal(s.slots[i].value, value) {
		return s
	}
	out := State{schema: s.schema, arena: arena, slots: append([]slot(nil), s.slots...)}
	out.slots[i] = slot{value: value, stamp: arena.fresh()}
	return out
}

// Reconfigure explicitly projects s into another schema from the same catalog.
// Shared coordinates preserve values/stamps; added coordinates start at bottom.
func Reconfigure(arena *Arena, s State, target *Schema) State {
	if arena == nil || s.arena != arena || s.schema == nil || target == nil || s.schema.catalog != target.catalog {
		panic("axiscompose: Reconfigure requires matching arena and catalog")
	}
	out := Bottom(arena, target)
	for id, oldIndex := range s.schema.byID {
		if newIndex, ok := target.byID[id]; ok {
			out.slots[newIndex] = s.slots[oldIndex]
		}
	}
	return out
}

// Equal reports semantic product equality. Stamps are not semantic.
func Equal(a, b State) bool {
	compatible(a, b)
	for i, spec := range a.schema.specs {
		if a.slots[i].stamp == b.slots[i].stamp {
			continue
		}
		if !spec.equal(a.slots[i].value, b.slots[i].value) {
			return false
		}
	}
	return true
}

// Mask is a scalable selected-axis bit set.
type Mask struct {
	schema *Schema
	words  []uint64
}

func newMask(schema *Schema) Mask {
	return Mask{schema: schema, words: make([]uint64, (schema.Len()+63)/64)}
}

func (m *Mask) set(i int) { m.words[i/64] |= uint64(1) << uint(i%64) }

// Has reports whether selected slot i is present.
func (m Mask) Has(i int) bool {
	return i >= 0 && i/64 < len(m.words) && m.words[i/64]&(uint64(1)<<uint(i%64)) != 0
}

// Count returns the number of selected bits.
func (m Mask) Count() int {
	n := 0
	for _, word := range m.words {
		n += bits.OnesCount64(word)
	}
	return n
}

// Empty reports whether no bit is selected.
func (m Mask) Empty() bool { return m.Count() == 0 }

// ChangeMask returns the safe pair-relative mask of unequal content stamps.
func ChangeMask(a, b State) Mask {
	compatible(a, b)
	out := newMask(a.schema)
	for i := range a.slots {
		if a.slots[i].stamp != b.slots[i].stamp {
			out.set(i)
		}
	}
	return out
}

// LessOrEq compares only coordinates whose content stamps differ.
func LessOrEq(a, b State) bool {
	ok, _ := lessOrEq(a, b, true)
	return ok
}

// LessOrEqBaseline is the full-scan reference implementation.
func LessOrEqBaseline(a, b State) bool {
	ok, _ := lessOrEq(a, b, false)
	return ok
}

// LessOrEqScans returns the result and number of semantic lane comparisons.
func LessOrEqScans(a, b State) (bool, int) { return lessOrEq(a, b, true) }

func lessOrEq(a, b State, masked bool) (bool, int) {
	compatible(a, b)
	scans := 0
	for i, spec := range a.schema.specs {
		if masked && a.slots[i].stamp == b.slots[i].stamp {
			continue
		}
		scans++
		if !spec.lessOrEq(a.slots[i].value, b.slots[i].value) {
			return false, scans
		}
	}
	return true, scans
}

// Join returns the pointwise join and preserves an operand stamp when equal.
func Join(arena *Arena, a, b State) State { return combine(arena, a, b, false) }

// Widen returns the pointwise widening with the same stamp discipline.
func Widen(arena *Arena, prev, next State) State { return combine(arena, prev, next, true) }

func combine(arena *Arena, a, b State, widening bool) State {
	compatible(a, b)
	if arena == nil || arena != a.arena {
		panic("axiscompose: combine requires matching arena")
	}
	out := Bottom(arena, a.schema)
	for i, spec := range a.schema.specs {
		av, bv := a.slots[i], b.slots[i]
		if av.stamp == bv.stamp {
			out.slots[i] = av
			continue
		}
		var value any
		if widening {
			value = spec.widen(av.value, bv.value)
		} else {
			value = spec.join(av.value, bv.value)
		}
		switch {
		case spec.equal(value, av.value):
			out.slots[i] = av
		case spec.equal(value, bv.value):
			out.slots[i] = bv
		default:
			out.slots[i] = slot{value: value, stamp: arena.fresh()}
		}
	}
	return out
}

// SemanticDigest excludes stamps and records only schema IDs and lane values.
func SemanticDigest(s State) uint64 {
	if s.schema == nil {
		return 0
	}
	h := fnv.New64a()
	for i, spec := range s.schema.specs {
		_, _ = h.Write([]byte(spec.id))
		_, _ = h.Write([]byte{0})
		v := spec.hash(s.slots[i].value)
		for shift := 0; shift < 64; shift += 8 {
			_, _ = h.Write([]byte{byte(v >> shift)})
		}
	}
	return h.Sum64()
}
