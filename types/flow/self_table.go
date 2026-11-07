package flow

import (
	"slices"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// selfTableName names the recursive types of tables that store themselves.
const selfTableName = "self"

// A table that stores itself, directly (t[k] = t, table.insert(t, t)) or
// through other tables (a[1] = b; b[1] = a), is read as the value of its own
// write. Typing that read structurally embeds the table's previous type in
// the next one, T_(n+1) = widen(T_n, T_n), and the chain never stabilizes.
//
// Each such table instead has a recursion identity: one per table path and
// allocation site, the root assignments that reach the write. A value that
// reads the written table reads its recursion variable, and every written
// type of the table is bound to its identity, so the references to the table
// held anywhere in the type, also inside other tables' recursive types,
// denote the table: t[k] = t gives t the type mu X. widen(T, X). A write that
// adds nothing new reproduces the same snapshot and the solve converges.

// writtenTable is the table a write stores into, as the written value sees
// it. Lua evaluates the value before the store, but a value that reads the
// table itself holds the same table, so its type is the table's type after
// the write.
type writtenTable struct {
	path     constraint.Path
	identity *typ.Recursive
}

// writtenTableAt returns the table at path written at p.
func (s *Solution) writtenTableAt(p cfg.Point, path constraint.Path) *writtenTable {
	if path.Symbol == 0 {
		return nil
	}
	return &writtenTable{path: path, identity: s.tableIdentityAt(p, path)}
}

// readBy reports whether a read of path reads the written table.
func (w *writtenTable) readBy(path constraint.Path) bool {
	if w == nil || path.Symbol != w.path.Symbol || len(path.Segments) != len(w.path.Segments) {
		return false
	}
	for i, seg := range path.Segments {
		if seg != w.path.Segments[i] {
			return false
		}
	}
	return true
}

// bindWrittenTable returns updated, the type of the table at target after a
// write of value at p, bound to the table's recursion identity when the value
// can hold the table.
func (s *Solution) bindWrittenTable(p cfg.Point, target constraint.Path, value ValueSource, updated typ.Type) typ.Type {
	if updated == nil || !s.valueMayHoldTable(value, target.Symbol) {
		return updated
	}
	identity := s.tableIdentityAt(p, target)
	if snapshot, ok := updated.(*typ.Recursive); ok && snapshot.ID == identity.ID && snapshot.Body != nil {
		updated = snapshot.Body
	}
	return typ.BindRecursiveSnapshotThrough(identity, typ.JoinSnapshotUnions(updated))
}

// tableIdentityAt returns the recursion identity of the table at path seen
// at p.
func (s *Solution) tableIdentityAt(p cfg.Point, path constraint.Path) *typ.Recursive {
	var key strings.Builder
	key.WriteString(string(constraint.Path{Symbol: path.Symbol, Segments: path.Segments}.Key()))
	for _, site := range s.allocationSitesAt(p, path.Symbol) {
		key.WriteByte('#')
		key.WriteString(strconv.Itoa(int(site)))
	}
	if identity := s.selfTables[key.String()]; identity != nil {
		return identity
	}
	if s.selfTables == nil {
		s.selfTables = make(map[string]*typ.Recursive)
	}
	identity := typ.NewRecursivePlaceholder(selfTableName)
	s.selfTables[key.String()] = identity
	return identity
}

// allocationSitesAt returns the root assignments of sym that reach p, in
// point order. A symbol without root assignments in the graph, such as a
// parameter or a captured variable, has none.
func (s *Solution) allocationSitesAt(p cfg.Point, sym cfg.SymbolID) []cfg.Point {
	reach, ok := s.allocationReach[sym]
	if !ok {
		reach = s.computeAllocationReach(sym)
		if s.allocationReach == nil {
			s.allocationReach = make(map[cfg.SymbolID]map[cfg.Point][]cfg.Point)
		}
		s.allocationReach[sym] = reach
	}
	return reach[p]
}

// computeAllocationReach maps each point to the root assignments of sym
// that reach it without another root assignment of sym between.
func (s *Solution) computeAllocationReach(sym cfg.SymbolID) map[cfg.Point][]cfg.Point {
	defs := make(map[cfg.Point]bool)
	for _, assign := range s.inputs.Assignments {
		if assign.TargetPath.Symbol == sym && len(assign.TargetPath.Segments) == 0 {
			defs[assign.Point] = true
		}
	}
	reach := make(map[cfg.Point][]cfg.Point)
	graph := s.inputs.Graph
	if graph == nil {
		return reach
	}
	sites := make([]cfg.Point, 0, len(defs))
	for def := range defs {
		sites = append(sites, def)
	}
	slices.Sort(sites)
	for _, def := range sites {
		seen := make(map[cfg.Point]bool)
		stack := append([]cfg.Point(nil), graph.Successors(def)...)
		for len(stack) > 0 {
			point := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[point] {
				continue
			}
			seen[point] = true
			reach[point] = append(reach[point], def)
			if defs[point] {
				continue
			}
			stack = append(stack, graph.Successors(point)...)
		}
	}
	return reach
}

// writtenSelfAncestor returns the path of the table a write into the table at
// path stores into. A path whose current type is a reference to the table of
// one of its ancestor paths, as t[1] in t[1] = t, names that same table.
func (s *Solution) writtenSelfAncestor(p cfg.Point, path constraint.Path, current typ.Type) constraint.Path {
	rec, ok := current.(*typ.Recursive)
	if !ok {
		return path
	}
	for cut := 0; cut < len(path.Segments); cut++ {
		ancestor := constraint.Path{Root: path.Root, Symbol: path.Symbol, Segments: path.Segments[:cut]}
		if s.tableIdentityAt(p, ancestor).ID == rec.ID {
			return ancestor
		}
	}
	return path
}

// valueMayHoldTable reports whether a value read from value can hold the
// table of symbol table: the value reads that table, or a table into which
// writes store values that reach it.
func (s *Solution) valueMayHoldTable(value ValueSource, table cfg.SymbolID) bool {
	if table == 0 {
		return false
	}
	holds := s.tableHoldGraph()
	seen := make(map[cfg.SymbolID]bool)
	pending := valueSourceSymbols(value, nil)
	for len(pending) > 0 {
		sym := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if sym == table {
			return true
		}
		if seen[sym] {
			continue
		}
		seen[sym] = true
		pending = append(pending, holds[sym]...)
	}
	return false
}

// tableHoldGraph maps each table symbol to the symbols whose values the
// indexer and mutator writes of the solve store into it.
func (s *Solution) tableHoldGraph() map[cfg.SymbolID][]cfg.SymbolID {
	if s.tableHolds != nil {
		return s.tableHolds
	}
	holds := make(map[cfg.SymbolID][]cfg.SymbolID)
	for _, ia := range s.inputs.IndexerAssignments {
		if ia.Symbol != 0 {
			holds[ia.Symbol] = valueSourceSymbols(ia.ValueSource, holds[ia.Symbol])
		}
	}
	for _, tm := range s.inputs.TableMutatorAssignments {
		if tm.Target.Symbol != 0 {
			holds[tm.Target.Symbol] = valueSourceSymbols(tm.ValueSource, holds[tm.Target.Symbol])
		}
	}
	s.tableHolds = holds
	return holds
}

// valueSourceSymbols appends to out the symbols whose values value reads.
func valueSourceSymbols(value ValueSource, out []cfg.SymbolID) []cfg.SymbolID {
	if value.ValuePath.Symbol != 0 {
		out = append(out, value.ValuePath.Symbol)
	}
	if value.MapElementSource != nil && value.MapElementSource.MapPath.Symbol != 0 {
		out = append(out, value.MapElementSource.MapPath.Symbol)
	}
	for _, field := range value.ValueFields {
		out = valueSourceSymbols(field.ValueSource, out)
	}
	for _, element := range value.ValueElements {
		out = valueSourceSymbols(element, out)
	}
	return out
}
