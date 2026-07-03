// Package visibility adapts precomputed SSA visibility to state key resolution.
package visibility

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

var _ VersionSource = (*Table)(nil)

type lookup struct {
	point  cfg.Point
	symbol symbol.ID
}

// Table stores the SSA version visible for each CFG point and symbol.
type Table struct {
	visible map[lookup]ssa.Version
	input   map[lookup]ssa.Version
}

// NewTable returns a visibility table cloned from a point/symbol map.
//
// The map key's symbol is authoritative. Version.Root is preserved only as
// display text, and Version.Symbol is normalized to the map key when stored.
func NewTable(visible map[cfg.Point]map[symbol.ID]ssa.Version) *Table {
	t := newTableWithCapacity(visibleEntryCount(visible))
	for point, bySymbol := range visible {
		for sym, version := range bySymbol {
			t.set(point, sym, version)
		}
	}
	return t
}

func newTableWithCapacity(capacity int) *Table {
	if capacity <= 0 {
		return &Table{}
	}
	return &Table{visible: make(map[lookup]ssa.Version, capacity)}
}

func visibleEntryCount(visible map[cfg.Point]map[symbol.ID]ssa.Version) int {
	total := 0
	for _, bySymbol := range visible {
		total += len(bySymbol)
	}
	return total
}

// VisibleVersion returns the precomputed version visible at point for sym.
// Missing entries return the zero Version.
func (t *Table) VisibleVersion(point cfg.Point, sym symbol.ID) ssa.Version {
	if t == nil || sym == 0 {
		return ssa.Version{}
	}
	return t.visible[lookup{point: point, symbol: sym}]
}

// VisibleVersionBefore returns the version visible at point before point-local
// definitions are applied. Tables without input snapshots fall back to
// VisibleVersion, matching explicit test builders and compatibility tables.
func (t *Table) VisibleVersionBefore(point cfg.Point, sym symbol.ID) ssa.Version {
	if t == nil || sym == 0 {
		return ssa.Version{}
	}
	if t.input != nil {
		return t.input[lookup{point: point, symbol: sym}]
	}
	return t.VisibleVersion(point, sym)
}

func (t *Table) set(point cfg.Point, sym symbol.ID, version ssa.Version) {
	if sym == 0 || version.ID <= 0 {
		return
	}
	if t.visible == nil {
		t.visible = make(map[lookup]ssa.Version)
	}
	version.Symbol = sym
	t.visible[lookup{point: point, symbol: sym}] = version
}

func (t *Table) setInput(point cfg.Point, sym symbol.ID, version ssa.Version) {
	if sym == 0 || version.ID <= 0 {
		return
	}
	if t.input == nil {
		t.input = make(map[lookup]ssa.Version)
	}
	version.Symbol = sym
	t.input[lookup{point: point, symbol: sym}] = version
}

// Builder creates explicit visibility tables without computing CFG flow.
type Builder struct {
	visible map[lookup]ssa.Version
	next    map[symbol.ID]int
}

// NewBuilder creates an empty explicit visibility builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// Define allocates a new version for sym and makes it visible at point.
func (b *Builder) Define(point cfg.Point, sym symbol.ID, root string) ssa.Version {
	if b == nil || sym == 0 {
		return ssa.Version{}
	}
	if b.next == nil {
		b.next = make(map[symbol.ID]int)
	}
	id := b.next[sym] + 1
	version := ssa.Version{Root: root, Symbol: sym, ID: id}
	b.SetVisible(point, sym, version)
	return version
}

// SetVisible records version as the visible value for sym at point.
//
// The sym argument is the binding identity used for lookup. Version.Root is
// preserved for display, and Version.Symbol is normalized to sym.
func (b *Builder) SetVisible(point cfg.Point, sym symbol.ID, version ssa.Version) {
	if b == nil || sym == 0 || version.ID <= 0 {
		return
	}
	if b.visible == nil {
		b.visible = make(map[lookup]ssa.Version)
	}
	version.Symbol = sym
	b.visible[lookup{point: point, symbol: sym}] = version

	if b.next == nil {
		b.next = make(map[symbol.ID]int)
	}
	if version.ID > b.next[sym] {
		b.next[sym] = version.ID
	}
}

// Build returns a table with the builder's current explicit visibility.
func (b *Builder) Build() *Table {
	if b == nil || len(b.visible) == 0 {
		return &Table{}
	}
	t := &Table{visible: make(map[lookup]ssa.Version, len(b.visible))}
	for key, version := range b.visible {
		t.visible[key] = version
	}
	return t
}
