package static

import (
	"fmt"

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
