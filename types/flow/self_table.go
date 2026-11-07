package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// selfTableName names the recursive types of tables that store themselves.
const selfTableName = "self"

// foldSelfEmbedding returns the type of the table at target after a write of
// value, whose computed type is updated, when the table had type old.
//
// A table that stores itself, directly (t[k] = t, table.insert(t, t)) or
// through other tables (a[1] = b; b[1] = a), is read as the value of its own
// write. Every solver iteration then embeds the table's previous type in the
// next one, T_(n+1) = widen(T_n, T_n), and the chain never stabilizes. When
// the written value can hold the table, the nested occurrences of old in
// updated are approximations of the table itself; they fold into the
// recursion variable of the table, giving mu X. updated[old := X], the limit
// of that chain. Each table path keeps one recursion identity for the solve,
// so the folds of successive iterations are snapshots of one recursive type
// and an iteration that adds nothing new reproduces it.
//
// An empty table carries no structure for an approximation to stand for, so
// a write into it waits for the next iteration, which sees the table's first
// shape.
func (s *Solution) foldSelfEmbedding(target constraint.Path, value ValueSource, old, updated typ.Type) typ.Type {
	if updated == nil || old == nil || isEmptyRecordNoMapType(old) || !s.valueMayHoldTable(value, target.Symbol) {
		return updated
	}
	identity := s.selfTableIdentity(target)
	return typ.FoldApproximationsInto(identity, updated, func(node typ.Type) bool {
		if rec, ok := node.(*typ.Recursive); ok && rec.ID == identity.ID {
			return true
		}
		return typ.TypeEquals(node, old)
	})
}

// writtenTable is the table a write stores into, as the written value sees it.
// Lua evaluates the value before the store, but a value that reads the table
// itself holds the same table, so its type is the table's type after the
// write: t[k] = t gives t the type mu X. widen(T, X). The value reads the
// table as its recursion variable, and the written type binds that variable.
type writtenTable struct {
	path     constraint.Path
	identity *typ.Recursive
	read     bool
}

// writtenTableAt returns the written table at path.
func (s *Solution) writtenTableAt(path constraint.Path) *writtenTable {
	if path.Symbol == 0 {
		return nil
	}
	return &writtenTable{path: path, identity: s.selfTableIdentity(path)}
}

// readBy reports whether a read of path reads the written table, and records
// the read.
func (w *writtenTable) readBy(path constraint.Path) bool {
	if w == nil || path.Symbol != w.path.Symbol || len(path.Segments) != len(w.path.Segments) {
		return false
	}
	for i, seg := range path.Segments {
		if seg != w.path.Segments[i] {
			return false
		}
	}
	w.read = true
	return true
}

// bind closes the type of the written table over its recursion variable when
// the written value read the table.
func (w *writtenTable) bind(t typ.Type) typ.Type {
	if w == nil || !w.read || t == nil {
		return t
	}
	return typ.BindRecursiveSnapshot(w.identity, t)
}

// selfTableIdentity returns the recursion identity of the table at path.
func (s *Solution) selfTableIdentity(path constraint.Path) *typ.Recursive {
	key := constraint.Path{Root: path.Root, Symbol: path.Symbol, Segments: path.Segments}.Key()
	if identity := s.selfTables[key]; identity != nil {
		return identity
	}
	if s.selfTables == nil {
		s.selfTables = make(map[constraint.PathKey]*typ.Recursive)
	}
	identity := typ.NewRecursivePlaceholder(selfTableName)
	s.selfTables[key] = identity
	return identity
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
