package body

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// LocalAssignmentPresentation is syntax-owned display data for one local
// assignment obligation.
type LocalAssignmentPresentation struct {
	SourceLabel     string
	ExpectedLabel   string
	SourceSpan      SourceSpan
	DeclarationSpan SourceSpan
}

// OrdinaryAssignmentPresentation is syntax-owned display data for an ordinary
// assignment obligation.
type OrdinaryAssignmentPresentation struct {
	TargetLabel   string
	SourceLabel   string
	SourceSpan    SourceSpan
	TargetSpan    SourceSpan
	DynamicTarget bool
}

// LocalAssignmentPresentationFor returns syntax-owned display data for a local
// assignment fact. Semantic proof data remains outside body.
func LocalAssignmentPresentationFor(fact LocalAssignmentFact) LocalAssignmentPresentation {
	sourceExpr := fact.Expr
	if sourceExpr == nil {
		sourceExpr = fact.Source.Expr
	}
	return LocalAssignmentPresentation{
		SourceLabel:     AssignmentSourceLabel(sourceExpr),
		ExpectedLabel:   TypeAnnotationLabel(fact.Type),
		SourceSpan:      sourceSpanFromAST(ast.SpanOf(sourceExpr)),
		DeclarationSpan: sourceSpanFromAST(ast.SpanOf(fact.Type)),
	}
}

// OrdinaryAssignmentPresentationFor returns syntax-owned display data for an
// ordinary assignment fact.
func OrdinaryAssignmentPresentationFor(fact OrdinaryAssignmentFact) OrdinaryAssignmentPresentation {
	return OrdinaryAssignmentPresentation{
		TargetLabel:   AssignmentSourceLabel(fact.Target),
		SourceLabel:   AssignmentSourceLabel(fact.Value),
		SourceSpan:    sourceSpanFromAST(ast.SpanOf(fact.Value)),
		TargetSpan:    sourceSpanFromAST(ast.SpanOf(fact.Target)),
		DynamicTarget: ordinaryAssignmentDynamicTarget(fact.Target),
	}
}

func (r *Result) LocalAssignmentExpectedType(point cfg.Point, fact LocalAssignmentFact) (typ.Type, bool) {
	if r == nil || fact.Type == nil || r.TypeResolver() == nil {
		return nil, false
	}
	return r.TypeResolver().Type(fact.Type)
}

func ordinaryAssignmentDynamicTarget(target ast.Expr) bool {
	attr, ok := assignmentTargetAttrExpr(target)
	if !ok || attr == nil || attr.KeySyntax != ast.AttrKeyIndex || attr.Key == nil {
		return false
	}
	switch attr.Key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return false
	default:
		return true
	}
}

// AssignmentSourceLabel returns the compact source label used by assignment
// diagnostics.
func AssignmentSourceLabel(expr ast.Expr) string {
	return assignmentSourceLabelDepth(expr, 0)
}

func assignmentSourceLabelDepth(expr ast.Expr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := assignmentSourceLabelDepth(e.Object, depth+1)
		key := AssignmentAttrKeyLabel(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return assignmentCallLabelDepth(e, depth+1)
	case *ast.CastExpr:
		return assignmentSourceLabelDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		return assignmentSourceLabelDepth(e.Expr, depth+1)
	default:
		return ""
	}
}

func assignmentCallLabelDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth || expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := assignmentSourceLabelDepth(expr.Receiver, depth+1)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := assignmentSourceLabelDepth(expr.Func, depth+1)
	if name == "" {
		return ""
	}
	return name + "(...)"
}

// AssignmentAttrKeyLabel returns the compact key suffix used in assignment
// labels and nilable-access evidence.
func AssignmentAttrKeyLabel(expr *ast.AttrGetExpr) string {
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		if name := ast.KeyName(expr.Key); name != "" {
			return "." + name
		}
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			return "[" + strconv.Quote(key.Value) + "]"
		case *ast.NumberExpr:
			return "[" + key.Value + "]"
		case *ast.IdentExpr:
			return "[" + key.Value + "]"
		}
	}
	if name := ast.KeyName(expr.Key); name != "" {
		return "." + name
	}
	return ""
}

// TypeAnnotationLabel returns a compact user-facing label for a type
// annotation syntax node.
func TypeAnnotationLabel(expr ast.TypeExpr) string {
	switch e := expr.(type) {
	case *ast.TypeRefExpr:
		return strings.Join(e.Path, ".")
	case *ast.PrimitiveTypeExpr:
		return e.Name
	case *ast.OptionalTypeExpr:
		if inner := TypeAnnotationLabel(e.Inner); inner != "" {
			return inner + "?"
		}
	case *ast.GenericTypeExpr:
		base := TypeAnnotationLabel(e.Base)
		if base == "" || len(e.Args) == 0 {
			return base
		}
		args := make([]string, 0, len(e.Args))
		for _, arg := range e.Args {
			label := TypeAnnotationLabel(arg)
			if label == "" {
				return base
			}
			args = append(args, label)
		}
		return base + "<" + strings.Join(args, ", ") + ">"
	case *ast.FunctionTypeExpr:
		params := make([]string, 0, len(e.Params)+1)
		for _, param := range e.Params {
			label := TypeAnnotationLabel(param.Type)
			if label == "" {
				return ""
			}
			if param.Name != "" {
				label = param.Name + ": " + label
			}
			params = append(params, label)
		}
		if e.Variadic != nil {
			label := TypeAnnotationLabel(e.Variadic)
			if label == "" {
				return ""
			}
			params = append(params, "...: "+label)
		}
		returns := "()"
		if len(e.Returns) == 1 {
			returns = TypeAnnotationLabel(e.Returns[0])
			if returns == "" {
				return ""
			}
		} else if len(e.Returns) > 1 {
			parts := make([]string, 0, len(e.Returns))
			for _, ret := range e.Returns {
				label := TypeAnnotationLabel(ret)
				if label == "" {
					return ""
				}
				parts = append(parts, label)
			}
			returns = strings.Join(parts, ", ")
		}
		return "fun(" + strings.Join(params, ", ") + ") -> " + returns
	}
	return ""
}
