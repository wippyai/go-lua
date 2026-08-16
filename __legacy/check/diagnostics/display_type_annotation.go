package diagnostics

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/__legacy/analysis/lua/exprdisplay"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (d diagnosticDisplay) AnnotationOrType(annotation ast.TypeExpr, fallback typ.Type) string {
	if s, ok := formatTypeAnnotation(annotation); ok && s != "" {
		return s
	}
	return d.Type(fallback)
}

func formatTypeAnnotation(expr ast.TypeExpr) (string, bool) {
	return formatTypeAnnotationDepth(expr, 0)
}

func formatTypeAnnotationDepth(expr ast.TypeExpr, depth int) (string, bool) {
	if expr == nil {
		return "", false
	}
	if depth > typ.DefaultRecursionDepth {
		return "...", true
	}
	nextDepth := depth + 1
	switch e := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		if e.Name == "" {
			return "", false
		}
		return e.Name, true
	case *ast.TypeRefExpr:
		if len(e.Path) == 0 {
			return "", false
		}
		return strings.Join(e.Path, "."), true
	case *ast.GenericTypeExpr:
		base, ok := formatTypeAnnotationDepth(e.Base, nextDepth)
		if !ok {
			return "", false
		}
		args, ok := formatTypeAnnotationListDepth(e.Args, ", ", nextDepth)
		if !ok {
			return "", false
		}
		return base + "<" + args + ">", true
	case *ast.OptionalTypeExpr:
		inner, ok := formatTypeAnnotationDepth(e.Inner, nextDepth)
		if !ok {
			return "", false
		}
		return maybeParenthesizeOptionalInner(e.Inner, inner) + "?", true
	case *ast.UnionTypeExpr:
		return formatTypeAnnotationListDepth(e.Types, " | ", nextDepth)
	case *ast.IntersectionTypeExpr:
		return formatTypeAnnotationListDepth(e.Types, " & ", nextDepth)
	case *ast.ArrayTypeExpr:
		elem, ok := formatTypeAnnotationDepth(e.Element, nextDepth)
		if !ok {
			return "", false
		}
		if e.Readonly {
			return "readonly {" + elem + "}", true
		}
		return "{" + elem + "}", true
	case *ast.MapTypeExpr:
		key, ok := formatTypeAnnotationDepth(e.Key, nextDepth)
		if !ok {
			return "", false
		}
		value, ok := formatTypeAnnotationDepth(e.Value, nextDepth)
		if !ok {
			return "", false
		}
		prefix := "{"
		if e.Readonly {
			prefix = "readonly {"
		}
		return prefix + "[" + key + "]: " + value + "}", true
	case *ast.RecordTypeExpr:
		return formatRecordTypeAnnotationDepth(e, nextDepth)
	case *ast.FunctionTypeExpr:
		return formatFunctionTypeAnnotationDepth(e, nextDepth)
	case *ast.LiteralTypeExpr:
		return formatLiteralTypeAnnotation(e.Value)
	case *ast.MetaTypeExpr:
		inner, ok := formatTypeAnnotationDepth(e.Inner, nextDepth)
		if !ok {
			return "", false
		}
		return "type<" + inner + ">", true
	case *ast.SelfTypeExpr:
		return "self", true
	case *ast.TupleTypeExpr:
		elems, ok := formatTypeAnnotationListDepth(e.Elements, ", ", nextDepth)
		if !ok {
			return "", false
		}
		return "(" + elems + ")", true
	case *ast.AssertsTypeExpr:
		if e.NarrowTo == nil {
			return "asserts " + e.ParamName, true
		}
		narrow, ok := formatTypeAnnotationDepth(e.NarrowTo, nextDepth)
		if !ok {
			return "", false
		}
		return "asserts " + e.ParamName + " is " + narrow, true
	case *ast.TypeOfExpr:
		name := exprdisplay.NameOK(e.Expr)
		if name == "" {
			name = "..."
		}
		return "typeof(" + name + ")", true
	case *ast.KeyOfExpr:
		inner, ok := formatTypeAnnotationDepth(e.Inner, nextDepth)
		if !ok {
			return "", false
		}
		return "keyof " + parenthesizeTypeOperatorInner(e.Inner, inner), true
	case *ast.IndexAccessExpr:
		object, ok := formatTypeAnnotationDepth(e.Object, nextDepth)
		if !ok {
			return "", false
		}
		index, ok := formatTypeAnnotationDepth(e.Index, nextDepth)
		if !ok {
			return "", false
		}
		return parenthesizeTypeOperatorInner(e.Object, object) + "[" + index + "]", true
	case *ast.ConditionalTypeExpr:
		check, ok := formatTypeAnnotationDepth(e.Check, nextDepth)
		if !ok {
			return "", false
		}
		extends, ok := formatTypeAnnotationDepth(e.Extends, nextDepth)
		if !ok {
			return "", false
		}
		thenType, ok := formatTypeAnnotationDepth(e.Then, nextDepth)
		if !ok {
			return "", false
		}
		elseType, ok := formatTypeAnnotationDepth(e.Else, nextDepth)
		if !ok {
			return "", false
		}
		return check + " extends " + extends + " ? " + thenType + " : " + elseType, true
	default:
		return "", false
	}
}

func formatTypeAnnotationListDepth(exprs []ast.TypeExpr, sep string, depth int) (string, bool) {
	parts := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		part, ok := formatTypeAnnotationDepth(expr, depth+1)
		if !ok {
			return "", false
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, sep), true
}

func formatRecordTypeAnnotationDepth(expr *ast.RecordTypeExpr, depth int) (string, bool) {
	if expr == nil {
		return "", false
	}
	fields := make([]string, 0, len(expr.Fields))
	for _, field := range expr.Fields {
		fieldType, ok := formatTypeAnnotationDepth(field.Type, depth+1)
		if !ok {
			return "", false
		}
		name := field.Name
		if field.Optional {
			name += "?"
		}
		fields = append(fields, name+": "+fieldType)
	}
	prefix := "{"
	if expr.Readonly {
		prefix = "readonly {"
	}
	return prefix + strings.Join(fields, ", ") + "}", true
}

func formatFunctionTypeAnnotationDepth(expr *ast.FunctionTypeExpr, depth int) (string, bool) {
	if expr == nil {
		return "", false
	}
	params := make([]string, 0, len(expr.Params)+1)
	for _, param := range expr.Params {
		paramType, ok := formatTypeAnnotationDepth(param.Type, depth+1)
		if !ok {
			return "", false
		}
		if param.Name != "" {
			params = append(params, param.Name+": "+paramType)
		} else {
			params = append(params, paramType)
		}
	}
	if expr.Variadic != nil {
		variadic, ok := formatTypeAnnotationDepth(expr.Variadic, depth+1)
		if !ok {
			return "", false
		}
		params = append(params, "...: "+variadic)
	}
	returns, ok := formatTypeAnnotationReturnsDepth(expr.Returns, depth+1)
	if !ok {
		return "", false
	}
	typeParams, ok := formatTypeParamAnnotations(expr.TypeParams, depth+1)
	if !ok {
		return "", false
	}
	name := "fun"
	if typeParams != "" {
		name += "<" + typeParams + ">"
	}
	return name + "(" + strings.Join(params, ", ") + ") -> " + returns, true
}

func formatTypeAnnotationReturnsDepth(exprs []ast.TypeExpr, depth int) (string, bool) {
	if len(exprs) == 0 {
		return "()", true
	}
	if len(exprs) == 1 {
		return formatTypeAnnotationDepth(exprs[0], depth+1)
	}
	return formatTypeAnnotationListDepth(exprs, ", ", depth+1)
}

func formatTypeParamAnnotations(params []ast.TypeParamExpr, depth int) (string, bool) {
	if len(params) == 0 {
		return "", true
	}
	parts := make([]string, 0, len(params))
	for _, param := range params {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			return "", false
		}
		if param.Constraint != nil {
			constraint, ok := formatTypeAnnotationDepth(param.Constraint, depth+1)
			if !ok {
				return "", false
			}
			name += ": " + constraint
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", "), true
}

func formatLiteralTypeAnnotation(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	default:
		return "", false
	}
}

func maybeParenthesizeOptionalInner(expr ast.TypeExpr, rendered string) string {
	switch expr.(type) {
	case *ast.UnionTypeExpr, *ast.IntersectionTypeExpr, *ast.FunctionTypeExpr, *ast.TupleTypeExpr, *ast.ConditionalTypeExpr:
		return "(" + rendered + ")"
	default:
		return rendered
	}
}

func parenthesizeTypeOperatorInner(expr ast.TypeExpr, rendered string) string {
	switch expr.(type) {
	case *ast.UnionTypeExpr, *ast.IntersectionTypeExpr, *ast.FunctionTypeExpr, *ast.TupleTypeExpr, *ast.ConditionalTypeExpr:
		return "(" + rendered + ")"
	default:
		return rendered
	}
}
