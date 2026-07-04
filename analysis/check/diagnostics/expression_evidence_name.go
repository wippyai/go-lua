package diagnostics

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func spanWithEvidenceName(span diagnostic.Span, sourceName string) diagnostic.Span {
	if !span.Valid() || sourceName == "" || sourceName == unknownSourceName || hasUsefulEnd(span) || !simpleEvidenceSpanName(sourceName) {
		return span
	}
	span.EndLine = span.StartLine
	span.EndCol = span.StartCol + len(sourceName)
	return span
}

func simpleEvidenceSpanName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func hasUsefulEnd(span diagnostic.Span) bool {
	return span.EndLine == span.StartLine && span.EndCol > span.StartCol
}

func sameStart(a, b diagnostic.Span) bool {
	return a.StartLine == b.StartLine && a.StartCol == b.StartCol
}

func exprEvidenceName(expr ast.Expr) string {
	if name := exprEvidenceNameOK(expr); name != "" {
		return name
	}
	return unknownSourceName
}

func exprEvidenceNameOK(expr ast.Expr) string {
	return exprEvidenceNameOKDepth(expr, 0)
}

func exprEvidenceNameOKDepth(expr ast.Expr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := exprEvidenceNameOKDepth(e.Object, depth+1)
		key := attrKeyEvidenceName(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return callEvidenceNameOKDepth(e, depth+1)
	case *ast.CastExpr:
		return exprEvidenceNameOKDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		return exprEvidenceNameOKDepth(e.Expr, depth+1)
	default:
		return ""
	}
}

func callEvidenceNameOKDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth || expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := exprEvidenceNameOKDepth(expr.Receiver, depth+1)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := exprEvidenceNameOKDepth(expr.Func, depth+1)
	if name == "" {
		return ""
	}
	return name + "(...)"
}

func assignmentTargetAttr(target ast.Expr) (*ast.AttrGetExpr, bool) {
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		return t, true
	case *ast.CastExpr:
		return assignmentTargetAttr(t.Expr)
	case *ast.NonNilAssertExpr:
		return nil, false
	default:
		return nil, false
	}
}

func requiredFieldPath(targetName, fieldName string) string {
	if fieldName == "" {
		return targetName
	}
	field := requiredFieldPathSegment(fieldName)
	if targetName == "" || targetName == unknownSourceName {
		return field
	}
	if field[0] == '[' {
		return targetName + field
	}
	return targetName + "." + field
}

func requiredFieldPathSegment(fieldName string) string {
	if luaDotFieldName(fieldName) {
		return fieldName
	}
	return "[" + strconv.Quote(fieldName) + "]"
}

func luaDotFieldName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func attrKeyEvidenceName(expr *ast.AttrGetExpr) string {
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
