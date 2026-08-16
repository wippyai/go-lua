package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
)

// PublishTypePublications attaches binder-authorized static export metadata to
// an already constructed Assign. It performs no target evaluation and creates
// no Read, Lens, Call, or value occurrence: the ordinary assignment vertical
// has already established the runtime operation.
func (w *Writer) PublishTypePublications(stmt *ast.AssignStmt, assign keyspace.Term) error {
	if w == nil || w.binding == nil || w.scopes == nil || stmt == nil || assign == 0 {
		return fmt.Errorf("lualower: invalid static type publication statement")
	}
	for _, publication := range w.binding.StaticTypePublications(stmt) {
		index := int(publication.Index)
		if index < 0 || index >= len(stmt.Lhs) || index >= len(stmt.Rhs) {
			return fmt.Errorf("lualower: invalid static type publication index %d", publication.Index)
		}
		if len(publication.Source) == 0 {
			return fmt.Errorf("lualower: incomplete static type publication at index %d", index)
		}
		root, err := w.publicationRoot(stmt.Rhs[index], publication.Source)
		if err != nil {
			return fmt.Errorf("lualower: static type publication root %d: %w", index, err)
		}
		target, err := w.PublicationRef(w.span(stmt.Rhs[index]), publication.Source, root, publication.Alias)
		if err != nil {
			return fmt.Errorf("lualower: static type publication target %d: %w", index, err)
		}
		if w.static.Type(w.span(stmt), assign, uint32(index), target) == 0 {
			return fmt.Errorf("lualower: could not create static type publication at index %d", index)
		}
	}
	return nil
}

func (w *Writer) publicationRoot(expr ast.Expr, source []string) (keyspace.Term, error) {
	if len(source) <= 1 {
		return 0, nil
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil || attr.KeySyntax != ast.AttrKeyDot {
		return 0, fmt.Errorf("invalid qualified publication source")
	}
	ident, ok := attr.Object.(*ast.IdentExpr)
	if !ok || ident == nil || ident.Value != source[0] {
		return 0, fmt.Errorf("invalid publication root")
	}
	return w.publicationCell(ident)
}

func (w *Writer) publicationCell(ident *ast.IdentExpr) (keyspace.Term, error) {
	if ident == nil {
		return 0, fmt.Errorf("invalid publication identifier")
	}
	id, ok := w.binding.SymbolOf(ident)
	if !ok || id == 0 {
		return 0, fmt.Errorf("binder has no symbol for publication identifier")
	}
	if cell, visible := w.scopes.Resolve(id); visible {
		return cell, nil
	}
	identity, global := w.binding.GlobalIdentity(ident)
	if !global || !identity.Matches(ident.Value) {
		return 0, fmt.Errorf("publication root is not a visible global")
	}
	cell := w.flow.Global(identity)
	if cell == 0 {
		return 0, fmt.Errorf("could not select publication global")
	}
	return cell, nil
}
