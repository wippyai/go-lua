package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// localStep is lexical-private ordinary-local construction state. It carries
// the exact visibility transaction from reservation through Bind; no other
// vertical may reconstruct or complete that transaction.
type localStep struct {
	owner        keyspace.Term
	span         source.Span
	stmt         *ast.LocalAssignStmt
	cellMark     int
	reservedMark int
	index        int
	kind         localStepKind
}

type localStepKind uint8

const (
	localDeclaredTypes localStepKind = iota + 1
	localBind
)

type reservedCell struct {
	id   bind.Symbol
	cell keyspace.Term
}

// Lua's ordinary local boundary is preserved exactly: Cells are reserved and
// may receive declared types, but their binder identities stay invisible until
// the initializer Values have completed and the Bind succeeds.
func (b *Bodies) ScheduleLocal(stmt *ast.LocalAssignStmt, owner keyspace.Term, span source.Span) error {
	if b == nil || b.collector == nil || b.binding == nil || b.modules == nil || b.phases == nil || b.values == nil || b.statics == nil || stmt == nil {
		return fmt.Errorf("lualower: invalid lexical local authority")
	}
	if owner == 0 || owner != b.Owner() || span.File == "" || len(stmt.Names) == 0 || len(stmt.Types) > len(stmt.Names) {
		return fmt.Errorf("lualower: invalid local declaration")
	}
	cellMark, reservedMark := b.CellMark(), len(b.reserved)
	for index := range stmt.Names {
		id, ok := b.binding.LocalSymbolAt(stmt, index)
		if !ok || id == 0 || b.Has(id) {
			return fmt.Errorf("lualower: invalid binder symbol for local slot %d", index)
		}
		if _, err := b.reserve(id, b.nameSpan(stmt, index)); err != nil {
			return fmt.Errorf("lualower: could not reserve local Cell %d: %w", index, err)
		}
	}
	b.localSteps = append(b.localSteps, localStep{
		kind:         localDeclaredTypes,
		owner:        owner,
		span:         span,
		stmt:         stmt,
		cellMark:     cellMark,
		reservedMark: reservedMark,
	})
	b.phases.Push(continuation.Lexical)
	return nil
}

// Run advances one lexical-private ordinary-local continuation. Its only
// external work uses the direct Values and typed Static capabilities injected
// at construction; source never reconstructs this transaction.
func (b *Bodies) Run() error {
	if b == nil || b.phases == nil || len(b.localSteps) == 0 {
		return fmt.Errorf("lualower: missing lexical continuation")
	}
	last := len(b.localSteps) - 1
	current := b.localSteps[last]
	b.localSteps = b.localSteps[:last]
	if current.owner == 0 || current.owner != b.Owner() {
		return fmt.Errorf("lualower: lexical local continuation crossed Body boundary")
	}

	switch current.kind {
	case localDeclaredTypes:
		return b.runLocalDeclaredTypes(current)
	case localBind:
		values, open := b.phases.Result()
		if values == 0 || open {
			return fmt.Errorf("lualower: invalid local initializer Values")
		}
		if err := b.bindReserved(current.cellMark, current.reservedMark, current.span, values); err != nil {
			return err
		}
		if len(current.stmt.Names) != 1 {
			return nil
		}
		id, ok := b.binding.LocalSymbolAt(current.stmt, 0)
		if !ok || id == 0 {
			return fmt.Errorf("lualower: missing one-cell local binder identity")
		}
		cell, ok := b.Resolve(id)
		if !ok || cell == 0 {
			return fmt.Errorf("lualower: missing one-cell local Cell")
		}
		return b.modules.AttachAlias(current.stmt, cell)
	default:
		return fmt.Errorf("lualower: invalid lexical local continuation %d", current.kind)
	}
}

func (b *Bodies) runLocalDeclaredTypes(current localStep) error {
	stmt := current.stmt
	if stmt == nil || current.index < 0 || current.index > len(stmt.Names) {
		return fmt.Errorf("lualower: invalid local declared-type cursor")
	}
	if current.index == len(stmt.Names) {
		current.kind = localBind
		b.pushLocal(current)
		return b.values.ScheduleValues(stmt.Exprs, current.owner, current.span)
	}

	index := current.index
	current.index++
	if index >= len(stmt.Types) || stmt.Types[index] == nil {
		b.pushLocal(current)
		return nil
	}
	declared := stmt.Types[index]
	id, ok := b.binding.LocalSymbolAt(stmt, index)
	if !ok || id == 0 {
		return fmt.Errorf("lualower: binder has no symbol for typed local slot %d", index)
	}
	bound, ok := b.binding.SymbolTypeAnnotation(id)
	if !ok || bound != declared {
		return fmt.Errorf("lualower: binder has mismatched declared type for local slot %d", index)
	}
	host, ok := b.reservedCell(current.reservedMark, index)
	if !ok || host == 0 {
		return fmt.Errorf("lualower: missing reserved Cell for typed local slot %d", index)
	}
	b.pushLocal(current)
	return b.statics.PushDeclaredCell(declared, host, current.owner, b.span(declared))
}

func (b *Bodies) pushLocal(current localStep) {
	b.localSteps = append(b.localSteps, current)
	b.phases.Push(continuation.Lexical)
}

// reserve creates one ordinary-local Cell without installing its binder
// identity. It is deliberately private so ScheduleLocal is the only ordinary
// local route.
func (b *Bodies) reserve(id bind.Symbol, span source.Span) (keyspace.Term, error) {
	if id == 0 {
		return 0, fmt.Errorf("lualower: cannot reserve zero binder symbol")
	}
	if _, exists := b.active[id]; exists {
		return 0, fmt.Errorf("lualower: duplicate active binder symbol")
	}
	if _, exists := b.reservedIDs[id]; exists {
		return 0, fmt.Errorf("lualower: duplicate reserved binder symbol")
	}
	name := b.binding.Name(id)
	if name == "" {
		return 0, fmt.Errorf("lualower: binder Cell %d has no authored spelling", id)
	}
	cell := b.collector.Cell(span, b.Owner(), name)
	if cell == 0 {
		return 0, fmt.Errorf("lualower: could not reserve Cell")
	}
	b.appendCell(cell)
	b.reserved = append(b.reserved, reservedCell{id: id, cell: cell})
	if b.reservedIDs == nil {
		b.reservedIDs = make(map[bind.Symbol]struct{})
	}
	b.reservedIDs[id] = struct{}{}
	return cell, nil
}

// Bind completes the Function-owned recursive-local declaration group. Ordinary
// locals use ScheduleLocal's private bindReserved transaction instead.
func (b *Bodies) Bind(mark int, span source.Span, values keyspace.Term) error {
	if mark < 0 || mark > b.cellLen {
		return fmt.Errorf("lualower: invalid local Cell mark")
	}
	owner := b.Owner()
	term := b.collector.Bind(span, owner, b.cellSlice()[mark:], values)
	b.truncateCells(mark)
	if term == 0 {
		return fmt.Errorf("lualower: could not lower local declaration")
	}
	b.source = append(b.source, sourceItem{term: term})
	return nil
}

// reservedCell returns one Cell in the private ordinary-local transaction.
func (b *Bodies) reservedCell(mark, index int) (keyspace.Term, bool) {
	at := mark + index
	if mark < 0 || index < 0 || at < mark || at >= len(b.reserved) {
		return 0, false
	}
	return b.reserved[at].cell, true
}

// RetainedCell returns one Cell in the active local/formal construction range.
func (b *Bodies) RetainedCell(mark, index int) (keyspace.Term, bool) {
	at := mark + index
	cells := b.cellSlice()
	if mark < 0 || index < 0 || at < mark || at >= len(cells) {
		return 0, false
	}
	return cells[at], true
}

// bindReserved publishes one ordinary-local group after its initializer and
// declared types were lowered. Installation happens only after Collector Bind
// succeeds, preserving Lua's initializer visibility boundary.
func (b *Bodies) bindReserved(cellMark, reservedMark int, span source.Span, values keyspace.Term) error {
	if cellMark < 0 || cellMark > b.cellLen || reservedMark < 0 || reservedMark > len(b.reserved) {
		return fmt.Errorf("lualower: invalid reserved local mark")
	}
	cells := b.cellSlice()[cellMark:]
	reserved := b.reserved[reservedMark:]
	if len(cells) != len(reserved) {
		return fmt.Errorf("lualower: reserved local Cell range mismatch")
	}
	for index, item := range reserved {
		if item.id == 0 || item.cell != cells[index] || b.Has(item.id) {
			return fmt.Errorf("lualower: invalid reserved local Cell")
		}
		if _, exists := b.reservedIDs[item.id]; !exists {
			return fmt.Errorf("lualower: missing reserved binder symbol")
		}
	}
	owner := b.Owner()
	term := b.collector.Bind(span, owner, cells, values)
	if term == 0 {
		return fmt.Errorf("lualower: could not lower reserved local declaration")
	}
	for _, item := range reserved {
		b.install(item.id, item.cell)
		delete(b.reservedIDs, item.id)
	}
	b.truncateCells(cellMark)
	b.reserved = b.reserved[:reservedMark]
	b.source = append(b.source, sourceItem{term: term})
	return nil
}
