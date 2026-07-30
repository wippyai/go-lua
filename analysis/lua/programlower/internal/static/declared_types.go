package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program"
)

// DeclareCellType attaches one already-lowered authored type to its exact
// lexical Cell. Only declared Cell types use this relation; schema annotations,
// casts, and callable returns have distinct source semantics.
func (w *Writer) DeclareCellType(host program.Term, expr ast.TypeExpr, target program.Term) error {
	if w == nil || w.builder == nil || expr == nil || host == 0 || target == 0 {
		return fmt.Errorf("programlower: invalid declared Cell type")
	}
	if w.builder.DeclareCellType(w.span(expr), host, target) == 0 {
		return fmt.Errorf("programlower: could not attach declared Cell type")
	}
	return nil
}

// DeclareImplicitSelfType materializes the binder's receiver-type evidence on
// a synthetic method self Cell. The syntax has no authored type expression,
// so its declaration reference and diagnostic span are supplied explicitly.
func (w *Writer) DeclareImplicitSelfType(host program.Term, span program.Span, decl bind.TypeDecl) error {
	if w == nil || w.builder == nil || host == 0 || decl.ID == 0 || decl.Name == "" {
		return fmt.Errorf("programlower: invalid implicit self declared type")
	}
	if decl.Kind != bind.TypeDeclAlias && decl.Kind != bind.TypeDeclParam {
		return fmt.Errorf("programlower: unsupported implicit self receiver declaration %q", decl.Name)
	}
	target, ok := w.Host(decl)
	if !ok || target == 0 {
		return fmt.Errorf("programlower: unavailable implicit self receiver declaration %q", decl.Name)
	}
	ref := w.builder.TypeRef(span, "", decl.Name, target)
	if ref == 0 {
		return fmt.Errorf("programlower: could not create implicit self type reference")
	}
	if w.builder.DeclareCellType(span, host, ref) == 0 {
		return fmt.Errorf("programlower: could not attach implicit self declared type")
	}
	return nil
}
