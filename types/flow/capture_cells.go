package flow

import (
	"cmp"
	"fmt"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// CaptureCell is one abstract lexical cell captured by closures.
//
// The symbol is the stable binding identity assigned by CFG construction. The
// value is the reduced product value currently known for that mutable cell.
type CaptureCell struct {
	Symbol cfg.SymbolID
	Value  product.AbstractValue
}

// CaptureCells is a deterministic finite-map lattice for closure-captured
// mutable locations. An absent symbol denotes product.Bottom(), not Lua nil.
//
// Lua closures capture locations, not snapshots of values. Therefore captured
// upvalues are ordinary abstract state: reads consume a cell value, writes update
// the cell value, and call summaries expose cell effects. This carrier is the
// foundational domain used to move capture precision out of driver re-solve
// loops and into the canonical product fixed point.
type CaptureCells struct {
	top     bool
	entries []CaptureCell
}

// CaptureCellsKey is an exact comparable key for a captured-cell store. It is
// interned by semantic CaptureCells equality, not by hash alone: hashes select a
// bucket and equality resolves collisions. This keeps SummaryQ keys Salsa/db
// friendly without relying on lossy string formatting or unstable map identity.
type CaptureCellsKey struct {
	n *captureCellsKeyNode
}

type captureCellsKeyNode struct {
	cells CaptureCells
	hash  uint64
}

type captureCellsKeyInterner struct {
	mu      sync.RWMutex
	buckets map[uint64][]*captureCellsKeyNode
}

var canonicalCaptureCellKeys = &captureCellsKeyInterner{buckets: make(map[uint64][]*captureCellsKeyNode)}

// ResetCanonicalCaptureCellKeys clears the comparable-key interner for one
// checker analysis scope.
func ResetCanonicalCaptureCellKeys() {
	canonicalCaptureCellKeys.mu.Lock()
	defer canonicalCaptureCellKeys.mu.Unlock()
	canonicalCaptureCellKeys.buckets = make(map[uint64][]*captureCellsKeyNode)
}

// CaptureCellsOf constructs a canonical finite cell map from entries. Duplicate
// symbols are joined; zero, bottom, and symbol-0 entries are dropped.
func CaptureCellsOf(entries []CaptureCell) CaptureCells {
	return canonicalCaptureCells(entries, product.Domain.Join)
}

// CaptureCellsTop returns the greatest capture-cell state.
func CaptureCellsTop() CaptureCells {
	return CaptureCells{top: true}
}

// Entries returns a copy of the sorted finite entries. Top has no finite entry
// representation and returns nil.
func (c CaptureCells) Entries() []CaptureCell {
	if c.top || len(c.entries) == 0 {
		return nil
	}
	return append([]CaptureCell(nil), c.entries...)
}

// HasFiniteEntries reports whether c has at least one concrete captured cell.
// Top is not finite, so callers that need to know whether an unknown effect can
// target a known current cell should decide how to handle Top explicitly.
func (c CaptureCells) HasFiniteEntries() bool {
	return !c.top && len(c.entries) > 0
}

// Project keeps only the requested symbols. The symbol list may be unsorted or
// contain duplicates; the result is canonical and deterministic.
func (c CaptureCells) Project(symbols []cfg.SymbolID) CaptureCells {
	if c.top || len(c.entries) == 0 || len(symbols) == 0 {
		if c.top && len(symbols) > 0 {
			entries := make([]CaptureCell, 0, len(symbols))
			for _, sym := range sortedUniqueSymbols(symbols) {
				entries = append(entries, CaptureCell{Symbol: sym, Value: product.Domain.Top()})
			}
			return CaptureCellsOf(entries)
		}
		return CaptureCells{}
	}
	want := sortedUniqueSymbols(symbols)
	var out []CaptureCell
	i, j := 0, 0
	for i < len(c.entries) && j < len(want) {
		switch {
		case c.entries[i].Symbol < want[j]:
			i++
		case want[j] < c.entries[i].Symbol:
			j++
		default:
			out = append(out, c.entries[i])
			i++
			j++
		}
	}
	return CaptureCellsOf(out)
}

// ProjectPaths keeps the captured cells and product members named by projection.
// A root path keeps the full cell. A nested path rebuilds the root value with
// only the requested static member path, avoiding whole-object context keys when
// a callee observes only a few fields.
func (c CaptureCells) ProjectPaths(projection ReferencePathProjection) CaptureCells {
	requests := projection.captureCellProjection()
	if len(requests) == 0 {
		return CaptureCells{}
	}
	if c.top {
		entries := make([]CaptureCell, 0, len(requests))
		for _, req := range requests {
			entries = append(entries, CaptureCell{Symbol: req.symbol, Value: product.Domain.Top()})
		}
		return CaptureCellsOf(entries)
	}
	if len(c.entries) == 0 {
		return CaptureCells{}
	}
	var out []CaptureCell
	for i, j := 0, 0; i < len(c.entries) && j < len(requests); {
		entry := c.entries[i]
		req := requests[j]
		switch {
		case entry.Symbol < req.symbol:
			i++
			continue
		case req.symbol < entry.Symbol:
			j++
			continue
		}
		if req.full {
			out = append(out, entry)
			i++
			j++
			continue
		}
		projected := product.Domain.Bottom()
		for _, segs := range req.segments {
			valueAtPath, ok := ProductMemberPathValue(entry.Value, segs)
			if !ok || valueAtPath.IsZero() {
				continue
			}
			projected = product.Domain.Join(projected, ProductWithOnlyMemberPath(segs, valueAtPath))
		}
		if !projected.IsZero() && !product.Domain.Equal(projected, product.Domain.Bottom()) {
			out = append(out, CaptureCell{Symbol: entry.Symbol, Value: projected})
		}
		i++
		j++
	}
	return CaptureCellsOf(out)
}

// WithStaticMembers overlays point-local static-member facts onto captured cell
// root values. Summary projection uses this to carry exact member facts through
// the reference context without teaching summary how static-member facts are
// stored or rebased.
func (c CaptureCells) WithStaticMembers(facts StaticMemberFacts) CaptureCells {
	if c.top || facts.IsBottom() || len(facts.entries) == 0 {
		return c
	}
	out := c
	for _, entry := range c.entries {
		next := captureCellWithStaticMembers(entry.Symbol, entry.Value, facts)
		if next.IsZero() || product.Domain.Equal(next, entry.Value) {
			continue
		}
		out = out.With(entry.Symbol, next)
	}
	return out
}

func captureCellWithStaticMembers(sym cfg.SymbolID, av product.AbstractValue, facts StaticMemberFacts) product.AbstractValue {
	if sym == 0 || av.IsZero() {
		return av
	}
	root, ok := StableAddressOfSymbol(sym, nil)
	if !ok {
		return av
	}
	out := av
	for _, fact := range facts.AddressEntriesUnder(root) {
		segments := fact.Address.Segments()
		if len(segments) == 0 || fact.Value.IsZero() {
			continue
		}
		next := ProductWithMemberPath(out, segments, fact.Value)
		if !next.IsZero() {
			out = next
		}
	}
	return out
}

// IsTop reports whether c is the greatest capture-cell state.
func (c CaptureCells) IsTop() bool { return c.top }

// Value returns the cell value for sym. For absent finite entries it returns
// product.Bottom() and ok=false; for Top it returns product.Top() and ok=true.
func (c CaptureCells) Value(sym cfg.SymbolID) (product.AbstractValue, bool) {
	if sym == 0 {
		return product.Domain.Bottom(), false
	}
	if c.top {
		return product.Domain.Top(), true
	}
	idx, ok := findCaptureCell(c.entries, sym)
	if !ok {
		return product.Domain.Bottom(), false
	}
	return c.entries[idx].Value, true
}

// With returns c with sym strongly updated to v. Updating a finite cell to
// Bottom removes it, preserving the absent-is-bottom canonical form.
func (c CaptureCells) With(sym cfg.SymbolID, v product.AbstractValue) CaptureCells {
	if c.top {
		return c
	}
	next := c.Entries()
	idx, ok := findCaptureCell(next, sym)
	switch {
	case sym == 0 || valueIsBottom(v):
		if ok {
			next = append(next[:idx], next[idx+1:]...)
		}
	case ok:
		next[idx].Value = v
	default:
		next = append(next, CaptureCell{Symbol: sym, Value: v})
	}
	return canonicalCaptureCells(next, product.Domain.Join)
}

// Format renders c deterministically for law-test diagnostics and journal notes.
func (c CaptureCells) Format() string {
	if c.top {
		return "⊤"
	}
	if len(c.entries) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(c.entries))
	for _, e := range c.entries {
		parts = append(parts, fmt.Sprintf("%d:%s", e.Symbol, e.Value.ProjectValue()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// Key returns an exact comparable key for c. Equal CaptureCells return the same
// key handle, so Go comparable key equality matches CaptureCellsDomain.Equal.
func (c CaptureCells) Key() CaptureCellsKey {
	return internCaptureCellsKey(c)
}

// Cells returns the immutable store represented by k. The zero key denotes
// CaptureCells bottom.
func (k CaptureCellsKey) Cells() CaptureCells {
	if k.n == nil {
		return CaptureCells{}
	}
	return k.n.cells
}

// Format renders the keyed store deterministically.
func (k CaptureCellsKey) Format() string {
	return k.Cells().Format()
}

type captureCellPathRequest struct {
	symbol   cfg.SymbolID
	full     bool
	segments [][]constraint.Segment
}

type captureCellPathProjection []captureCellPathRequest

func captureCellProjectionOfAddresses(addresses referenceAddressProjection) captureCellPathProjection {
	requests := make(captureCellPathProjection, 0, len(addresses.Exact)+len(addresses.Subtrees))
	addAddress := func(addr StableAddress) {
		sym, ok := addr.Symbol()
		if !ok || sym == 0 {
			return
		}
		segments := addr.suffix.segments
		idx, ok := findCaptureCellPathRequest(requests, sym)
		if !ok {
			requests = append(requests, captureCellPathRequest{symbol: sym})
			idx = len(requests) - 1
		}
		req := requests[idx]
		if len(segments) == 0 {
			req.full = true
			req.segments = nil
			requests[idx] = req
			return
		}
		if !req.full && !captureCellRequestHasSegments(req, segments) {
			req.segments = append(req.segments, segments)
		}
		requests[idx] = req
	}
	for _, addr := range addresses.Exact {
		addAddress(addr)
	}
	for _, addr := range addresses.Subtrees {
		addAddress(addr)
	}
	if len(requests) == 0 {
		return nil
	}
	sortCaptureCellPathProjection(requests)
	return requests
}

func findCaptureCellPathRequest(requests captureCellPathProjection, sym cfg.SymbolID) (int, bool) {
	for i, req := range requests {
		if req.symbol == sym {
			return i, true
		}
	}
	return 0, false
}

func captureCellRequestHasSegments(req captureCellPathRequest, segments []constraint.Segment) bool {
	for _, existing := range req.segments {
		if captureCellSegmentsEqual(existing, segments) {
			return true
		}
	}
	return false
}

func captureCellSegmentsEqual(left, right []constraint.Segment) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sortCaptureCellPathProjection(requests captureCellPathProjection) {
	for i := 1; i < len(requests); i++ {
		for j := i; j > 0 && requests[j].symbol < requests[j-1].symbol; j-- {
			requests[j], requests[j-1] = requests[j-1], requests[j]
		}
	}
}

// CaptureCellsDomain is the finite-map lattice over captured lexical cells.
var CaptureCellsDomain = lattice.Lattice[CaptureCells]{
	Bottom: func() CaptureCells {
		return CaptureCells{}
	},
	Top: CaptureCellsTop,
	Equal: func(a, b CaptureCells) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i].Symbol != b.entries[i].Symbol ||
				!product.Domain.Equal(a.entries[i].Value, b.entries[i].Value) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b CaptureCells) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		return captureCellsPointwise(a, b, product.Domain.LessOrEq)
	},
	Join: func(a, b CaptureCells) CaptureCells {
		if a.top || b.top {
			return CaptureCellsTop()
		}
		if len(a.entries) == 0 {
			return canonicalCaptureCellsValue(b)
		}
		if len(b.entries) == 0 {
			return canonicalCaptureCellsValue(a)
		}
		return combineCaptureCells(a, b, product.Domain.Join)
	},
	Meet: nil,
	Widen: func(prev, next CaptureCells) CaptureCells {
		if prev.top || next.top {
			return CaptureCellsTop()
		}
		return combineCaptureCells(prev, next, product.Domain.Widen)
	},
}

func findCaptureCell(entries []CaptureCell, sym cfg.SymbolID) (int, bool) {
	idx, ok := binarySearchCaptureCell(entries, sym)
	return idx, ok
}

func binarySearchCaptureCell(entries []CaptureCell, sym cfg.SymbolID) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].Symbol < sym {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo].Symbol == sym
}

func canonicalCaptureCells(entries []CaptureCell, merge func(product.AbstractValue, product.AbstractValue) product.AbstractValue) CaptureCells {
	if len(entries) == 0 {
		return CaptureCells{}
	}
	out := append([]CaptureCell(nil), entries...)
	sortCaptureCells(out)
	dst := out[:0]
	for _, e := range out {
		if e.Symbol == 0 || valueIsBottom(e.Value) {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Symbol == e.Symbol {
			dst[len(dst)-1].Value = merge(dst[len(dst)-1].Value, e.Value)
			if valueIsBottom(dst[len(dst)-1].Value) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		dst = append(dst, e)
	}
	return CaptureCells{entries: append([]CaptureCell(nil), dst...)}
}

func canonicalCaptureCellsValue(c CaptureCells) CaptureCells {
	if c.top {
		return CaptureCellsTop()
	}
	if len(c.entries) == 0 {
		return CaptureCells{}
	}
	for i, entry := range c.entries {
		if entry.Symbol == 0 || valueIsBottom(entry.Value) {
			return canonicalCaptureCells(c.entries, product.Domain.Join)
		}
		if i > 0 && c.entries[i-1].Symbol >= entry.Symbol {
			return canonicalCaptureCells(c.entries, product.Domain.Join)
		}
	}
	return c
}

func sortCaptureCells(entries []CaptureCell) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && cmp.Compare(entries[j].Symbol, entries[j-1].Symbol) < 0; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func sortedUniqueSymbols(symbols []cfg.SymbolID) []cfg.SymbolID {
	if len(symbols) == 0 {
		return nil
	}
	out := append([]cfg.SymbolID(nil), symbols...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	dst := out[:0]
	for _, sym := range out {
		if sym == 0 {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1] == sym {
			continue
		}
		dst = append(dst, sym)
	}
	return dst
}

func combineCaptureCells(a, b CaptureCells, op func(product.AbstractValue, product.AbstractValue) product.AbstractValue) CaptureCells {
	var out []CaptureCell
	i, j := 0, 0
	bottom := product.Domain.Bottom()
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Symbol < b.entries[j].Symbol):
			out = append(out, CaptureCell{Symbol: a.entries[i].Symbol, Value: op(a.entries[i].Value, bottom)})
			i++
		case i >= len(a.entries) || b.entries[j].Symbol < a.entries[i].Symbol:
			out = append(out, CaptureCell{Symbol: b.entries[j].Symbol, Value: op(bottom, b.entries[j].Value)})
			j++
		default:
			out = append(out, CaptureCell{Symbol: a.entries[i].Symbol, Value: op(a.entries[i].Value, b.entries[j].Value)})
			i++
			j++
		}
	}
	return canonicalCaptureCells(out, product.Domain.Join)
}

func captureCellsPointwise(a, b CaptureCells, pred func(product.AbstractValue, product.AbstractValue) bool) bool {
	i, j := 0, 0
	bottom := product.Domain.Bottom()
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Symbol < b.entries[j].Symbol):
			if !pred(a.entries[i].Value, bottom) {
				return false
			}
			i++
		case i >= len(a.entries) || b.entries[j].Symbol < a.entries[i].Symbol:
			if !pred(bottom, b.entries[j].Value) {
				return false
			}
			j++
		default:
			if !pred(a.entries[i].Value, b.entries[j].Value) {
				return false
			}
			i++
			j++
		}
	}
	return true
}

func valueIsBottom(v product.AbstractValue) bool {
	return v.IsZero() || product.Domain.Equal(v, product.Domain.Bottom())
}

func internCaptureCellsKey(c CaptureCells) CaptureCellsKey {
	cells := canonicalCaptureCells(c.Entries(), product.Domain.Join)
	if c.top {
		cells = CaptureCellsTop()
	}
	h := captureCellsKeyHash(cells)

	canonicalCaptureCellKeys.mu.RLock()
	if existing, ok := lookupCaptureCellsKey(canonicalCaptureCellKeys.buckets[h], cells); ok {
		canonicalCaptureCellKeys.mu.RUnlock()
		return CaptureCellsKey{n: existing}
	}
	canonicalCaptureCellKeys.mu.RUnlock()

	canonicalCaptureCellKeys.mu.Lock()
	defer canonicalCaptureCellKeys.mu.Unlock()
	if existing, ok := lookupCaptureCellsKey(canonicalCaptureCellKeys.buckets[h], cells); ok {
		return CaptureCellsKey{n: existing}
	}
	node := &captureCellsKeyNode{cells: cells, hash: h}
	canonicalCaptureCellKeys.buckets[h] = append(canonicalCaptureCellKeys.buckets[h], node)
	return CaptureCellsKey{n: node}
}

func lookupCaptureCellsKey(bucket []*captureCellsKeyNode, cells CaptureCells) (*captureCellsKeyNode, bool) {
	for _, node := range bucket {
		if CaptureCellsDomain.Equal(node.cells, cells) {
			return node, true
		}
	}
	return nil, false
}

func captureCellsKeyHash(c CaptureCells) uint64 {
	h := internal.FnvString("flow.CaptureCellsKey")
	if c.top {
		return internal.HashCombine(h, 1)
	}
	h = internal.HashCombine(h, 0)
	for _, entry := range c.entries {
		h = internal.HashCombine(h, uint64(entry.Symbol))
		h = internal.HashCombine(h, entry.Value.Hash())
	}
	return h
}
