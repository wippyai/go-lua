package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ScheduleVararg owns vararg identity because the Cell is lexical evidence
// from the active function boundary. Source dispatches the exact Comma3 AST
// case here; eval never reconstructs binder-backed function scope. The caller
// must name the currently active Body, preserving the same ownership boundary
// as ordinary local scheduling.
func (b *Bodies) ScheduleVararg(expr *ast.Comma3Expr, owner keyspace.Term, span source.Span) error {
	if b == nil || b.collector == nil || b.binding == nil || b.phases == nil || expr == nil || owner == 0 {
		return fmt.Errorf("lualower: invalid lexical vararg authority")
	}
	if len(b.frames) == 0 || owner != b.Owner() {
		return fmt.Errorf("lualower: vararg request crossed Body boundary")
	}
	cell, err := b.Vararg(span)
	if err != nil {
		return err
	}
	term := b.collector.Vararg(span, owner, cell)
	if term == 0 {
		return fmt.Errorf("lualower: could not create Vararg")
	}
	b.phases.SetResult(term, !expr.AdjustRet)
	return nil
}

// Vararg resolves the active function boundary's exact vararg Cell from the
// construction's sole binder authority. The chunk has one implicit Cell,
// minted lazily only when authored source uses "...".
func (b *Bodies) Vararg(span source.Span) (keyspace.Term, error) {
	if b == nil || b.binding == nil || len(b.frames) == 0 {
		return 0, fmt.Errorf("lualower: vararg expression outside Body")
	}
	function := b.frames[len(b.frames)-1].function
	if function == nil {
		if b.chunkVararg != 0 {
			return b.chunkVararg, nil
		}
		entry := b.frames[0].body
		if b.collector == nil || b.collector.Entry() != entry {
			return 0, fmt.Errorf("lualower: chunk vararg requires the entry Body")
		}
		cell := b.collector.Cell(span, entry, "...")
		if cell == 0 {
			return 0, fmt.Errorf("lualower: could not create chunk vararg Cell")
		}
		b.chunkVararg = cell
		return cell, nil
	}
	id, ok := b.binding.VarargSymbol(function)
	if !ok {
		return 0, fmt.Errorf("lualower: vararg expression in non-vararg function")
	}
	cell, ok := b.Resolve(id)
	if !ok {
		return 0, fmt.Errorf("lualower: missing vararg Cell")
	}
	return cell, nil
}
