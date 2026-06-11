package typeannotation

import (
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// Annotations lowers AST annotations when all arguments are literal payloads
// representable by analysis/type/annotation.Annotation.
func Annotations(exprs []ast.AnnotationExpr) ([]annotation.Annotation, bool) {
	if len(exprs) == 0 {
		return nil, true
	}
	out := make([]annotation.Annotation, 0, len(exprs))
	for _, expr := range exprs {
		if expr.Name == "" || len(expr.Args) > 1 {
			return nil, false
		}
		ann := annotation.Annotation{Name: expr.Name}
		if len(expr.Args) == 1 {
			arg, ok := annotationArg(expr.Args[0])
			if !ok {
				return nil, false
			}
			ann.Arg = arg
		}
		out = append(out, ann)
	}
	return out, true
}

func annotationArg(expr ast.Expr) (any, bool) {
	switch e := expr.(type) {
	case *ast.StringExpr:
		return e.Value, true
	case *ast.TrueExpr:
		return true, true
	case *ast.FalseExpr:
		return false, true
	case *ast.NumberExpr:
		if i, ok := numparse.ParseIntegerLiteral(e.Value); ok {
			return i, true
		}
		if f, ok := numparse.ParseFloatLiteral(e.Value); ok {
			return f, true
		}
		return nil, false
	default:
		return nil, false
	}
}
