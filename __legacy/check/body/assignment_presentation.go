package body

import (
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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

// AssignmentTargetKey returns a stable key for a local assignment target.
func AssignmentTargetKey(fact LocalAssignmentFact) string {
	if fact.HasSymbol && fact.Symbol != 0 {
		return "sym:" + strconv.FormatUint(uint64(fact.Symbol), 10)
	}
	return "local:" + fact.Name + ":" + strconv.Itoa(fact.Index)
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

// AssignmentSourcePathKey returns the canonical path key for an assignment
// source expression when lowering proved one.
func (r *Result) AssignmentSourcePathKey(expr ast.Expr) string {
	if r == nil || expr == nil {
		return ""
	}
	return assignmentPathKey(r.ExpressionPath(expr))
}

// OrdinaryAssignmentTargetPathKey returns the canonical path key for an
// ordinary assignment target. For member writes, the containing object path is
// used as a stable fallback when the full target path is unavailable.
func (r *Result) OrdinaryAssignmentTargetPathKey(fact OrdinaryAssignmentFact) string {
	if r == nil || fact.Target == nil {
		return ""
	}
	if key := assignmentPathKey(r.ExpressionPath(fact.Target)); key != "" {
		return key
	}
	if attr, ok := fact.Target.(*ast.AttrGetExpr); ok && attr.Object != nil {
		return assignmentPathKey(r.ExpressionPath(attr.Object))
	}
	return ""
}

// AssignmentSourceReadProvenPresent reports whether the assignment source read
// was proven present before the assignment boundary.
func (r *Result) AssignmentSourceReadProvenPresent(point cfg.Point, expr ast.Expr) bool {
	if r == nil || expr == nil {
		return false
	}
	return r.ExpressionReadProvenPresentBeforeBoundary(point, expr)
}

// AssignmentSourceIndexedReadAt reports whether an assignment source needs an
// indexed-read validation proof at point. Direct bracket sources count unless
// the exact slot is already proven present; bracket parents count only when the
// solved member-read facts show that parent can miss.
func (r *Result) AssignmentSourceIndexedReadAt(point cfg.Point, expr ast.Expr) bool {
	if r == nil || expr == nil {
		return false
	}
	if r.assignmentIndexedAccessCanMiss(point, expr, true) {
		return true
	}
	for _, evidence := range r.AssignmentNilableAccessEvidence(point, expr) {
		if strings.HasPrefix(evidence.Access, "[") {
			return true
		}
	}
	return false
}

func (r *Result) assignmentIndexedAccessCanMiss(point cfg.Point, expr ast.Expr, sourceRoot bool) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return false
	}
	if attr.KeySyntax == ast.AttrKeyIndex &&
		!r.ExpressionReadProvenPresentBeforeBoundary(point, attr) &&
		(sourceRoot || r.MemberReadCanMiss(point, attr)) {
		return true
	}
	return r.assignmentIndexedAccessCanMiss(point, attr.Object, false)
}

func assignmentPathKey(p pathdom.Path, ok bool) string {
	if !ok || p.IsEmpty() {
		return ""
	}
	return "path:" + p.String()
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
	case *ast.LogicalOpExpr:
		return assignmentBinaryLabel(e.Operator, e.Lhs, e.Rhs, depth+1)
	case *ast.TableExpr:
		if len(e.Fields) == 0 {
			return "{}"
		}
		return "{...}"
	default:
		return ""
	}
}

func assignmentBinaryLabel(operator string, lhs, rhs ast.Expr, depth int) string {
	left := assignmentSourceLabelDepth(lhs, depth+1)
	right := assignmentSourceLabelDepth(rhs, depth+1)
	if left == "" {
		return right
	}
	if right == "" || operator == "" {
		return left
	}
	return left + " " + operator + " " + right
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
