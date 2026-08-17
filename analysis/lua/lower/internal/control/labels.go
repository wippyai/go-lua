package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// PredeclareLabel allocates one addressable source label before the Body is
// traversed, so both forward and outward Gotos can carry their final typed
// target without an unresolved Program relation.
func (w *Writer) PredeclareLabel(
	stmt *ast.LabelStmt,
	span source.Span,
	owner keyspace.Term,
) error {
	if stmt == nil {
		return fmt.Errorf("lualower: cannot predeclare nil Label")
	}
	if w.labels == nil {
		w.labels = make(map[*ast.LabelStmt]labelState)
	}
	if _, exists := w.labels[stmt]; exists {
		return fmt.Errorf("lualower: duplicate Label predeclaration")
	}
	label := w.flow.Label(span, owner)
	if label == 0 {
		return fmt.Errorf("lualower: could not create Label")
	}
	w.labels[stmt] = labelState{term: label}
	return nil
}

// Label returns one predeclared Label at its authored source turn. The caller
// appends the existing identity to the active Body's sole source sequence.
func (w *Writer) Label(stmt *ast.LabelStmt) (keyspace.Term, error) {
	state, ok := w.labels[stmt]
	if !ok || state.term == 0 {
		return 0, fmt.Errorf("lualower: Label was not predeclared")
	}
	if state.placed {
		return 0, fmt.Errorf("lualower: Label occurred twice")
	}
	state.placed = true
	w.labels[stmt] = state
	w.labelCount++
	return state.term, nil
}

// Goto records one resolved non-local transfer. Label names and resolution
// remain transient binder concerns; Program receives only the exact Label.
func (w *Writer) Goto(
	span source.Span,
	owner keyspace.Term,
	target *ast.LabelStmt,
) (keyspace.Term, error) {
	state, ok := w.labels[target]
	if !ok || state.term == 0 {
		return 0, fmt.Errorf("lualower: Goto target was not predeclared")
	}
	term := w.flow.Goto(span, owner, state.term)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower goto")
	}
	return term, nil
}

// Predeclare publishes the addressable Labels in one already-entered lexical
// Body. The binder owns which declarations are invalid; a duplicate never
// receives a second Program Label.
func (w *Writer) Predeclare(stmts []ast.Stmt, body keyspace.Term) error {
	if err := w.ready(); err != nil {
		return err
	}
	if body == 0 || body != w.scopes.Owner() {
		return fmt.Errorf("lualower: Label predeclaration crossed Body boundary")
	}
	for _, stmt := range stmts {
		label, ok := stmt.(*ast.LabelStmt)
		if !ok {
			continue
		}
		if label == nil {
			return fmt.Errorf("lualower: absent Label")
		}
		if issue, invalid := w.issues[label]; invalid && issue.Kind == bind.ControlIssueDuplicateLabel {
			continue
		}
		if err := w.PredeclareLabel(label, w.span(label), body); err != nil {
			return err
		}
	}
	return nil
}
