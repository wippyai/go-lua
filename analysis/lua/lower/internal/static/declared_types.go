package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DeclareCellType attaches one already-lowered authored type to its exact
// lexical Cell. Only declared Cell types use this relation; schema annotations,
// casts, and callable returns have distinct source semantics.
func (w *Writer) DeclareCellType(host keyspace.Term, expr ast.TypeExpr, target keyspace.Term) error {
	if w == nil || !validTypeExpr(expr) {
		return fmt.Errorf("lualower: invalid declared Cell type")
	}
	return w.DeclareCellTypeAt(host, w.typeSpan(expr), target)
}

// DeclareCellTypeAt attaches one declared Cell type using the span captured
// when the source request was scheduled. Delayed lowering never recomputes
// this source boundary from ambient lexical state.
func (w *Writer) DeclareCellTypeAt(host keyspace.Term, span source.Span, target keyspace.Term) error {
	if w == nil || host == 0 || target == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid declared Cell type")
	}
	if w.static.DeclaredType(span, host, target) == 0 {
		return fmt.Errorf("lualower: could not attach declared Cell type")
	}
	return nil
}

// DeclareImplicitSelfType materializes the binder's receiver-type evidence on
// a synthetic method self Cell. The syntax has no authored type expression,
// so its declaration reference and diagnostic span are supplied explicitly.
func (w *Writer) DeclareImplicitSelfType(host keyspace.Term, span source.Span, decl bind.TypeDecl) error {
	if w == nil || host == 0 || decl.ID == 0 || decl.Name == "" {
		return fmt.Errorf("lualower: invalid implicit self declared type")
	}
	if decl.Kind != bind.TypeDeclAlias && decl.Kind != bind.TypeDeclInterface && decl.Kind != bind.TypeDeclParam {
		return fmt.Errorf("lualower: unsupported implicit self receiver declaration %q", decl.Name)
	}
	target, ok := w.Host(decl)
	if !ok || target == 0 {
		return fmt.Errorf("lualower: unavailable implicit self receiver declaration %q", decl.Name)
	}
	ref := w.static.Declaration(span, []string{decl.Name}, 0, target)
	if ref == 0 {
		return fmt.Errorf("lualower: could not create implicit self type reference")
	}
	if w.static.DeclaredType(span, host, ref) == 0 {
		return fmt.Errorf("lualower: could not attach implicit self declared type")
	}
	return nil
}
