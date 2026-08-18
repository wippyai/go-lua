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
		if !publication.Valid() {
			return fmt.Errorf("lualower: invalid static type publication evidence")
		}
		index := int(publication.Index)
		if index < 0 || index >= len(stmt.Lhs) || index >= len(stmt.Rhs) {
			return fmt.Errorf("lualower: invalid static type publication index %d", publication.Index)
		}
		if len(publication.Source) == 0 {
			return fmt.Errorf("lualower: incomplete static type publication at index %d", index)
		}
		var root keyspace.Term
		if len(publication.Source) > 1 {
			if cell, visible := w.scopes.Resolve(publication.Root); visible {
				root = cell
			} else {
				identity, global := w.binding.GlobalIdentityOf(publication.Root)
				if !global || !identity.Matches(publication.Source[0]) {
					return fmt.Errorf("lualower: static type publication root %d is not a visible global", index)
				}
				root = w.flow.Global(identity)
			}
			if root == 0 {
				return fmt.Errorf("lualower: could not materialize static type publication root %d", index)
			}
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
