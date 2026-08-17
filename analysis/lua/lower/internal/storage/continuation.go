package storage

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Run completes exactly one storage-private continuation. Expression children
// are explicitly handed to source's expression inbox; Values is a direct
// dependency because it owns exact Lua list adjustment.
func (w *Writer) Run() error {
	if w == nil || w.stack == nil || len(w.steps) == 0 {
		return fmt.Errorf("lualower: missing storage continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]
	switch current.kind {
	case stepExpression:
		return w.runExpression(current)
	case stepTarget:
		return w.runTarget(current)
	case stepFinishLensBase:
		return w.finishLensBase(current)
	case stepFinishLens:
		return w.finishLens(current)
	case stepTargets:
		return w.runTargets(current)
	case stepAppendTarget:
		term, _ := w.stack.Result()
		return w.RememberTarget(current.span, term)
	case stepFinishAssign:
		values, _ := w.stack.Result()
		term, err := w.Assign(current.span, current.owner, current.mark, values, current.assign)
		if err != nil {
			return err
		}
		if err := w.lexical.Append(term); err != nil {
			return err
		}
		w.stack.SetResult(term, false)
		return nil
	default:
		return fmt.Errorf("lualower: invalid storage continuation %d", current.kind)
	}
}

func (w *Writer) schedule(current step) {
	w.steps = append(w.steps, current)
	w.stack.Push(continuation.Store)
}

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: w.sourceName}
	}
	span, ok := coord.Build(w.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}

// Clean reports whether every storage-owned continuation and scratch range
// completed before Program sealing.
func (w *Writer) Clean() bool {
	return w != nil && len(w.steps) == 0 && len(w.targets) == 0 &&
		len(w.targetSpans) == 0 && len(w.tableKeys) == 0 &&
		len(w.tableKinds) == 0 && len(w.tableFields) == 0
}

type stepKind uint8

const (
	stepExpression stepKind = iota + 1
	stepTarget
	stepFinishLensBase
	stepFinishLens
	stepTargets
	stepAppendTarget
	stepFinishAssign
)

type step struct {
	kind        stepKind
	expr        ast.Expr
	attr        *ast.AttrGetExpr
	assign      *ast.AssignStmt
	owner       keyspace.Term
	span        source.Span
	keySpan     source.Span
	read        bool
	index       int
	targetSpans []source.Span
	mark        TargetMark
	base        keyspace.Term
}
